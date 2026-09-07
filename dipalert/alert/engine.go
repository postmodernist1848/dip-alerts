package alert

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/postmodernist1848/dip-alerts/dipalert/market"
	"github.com/postmodernist1848/dip-alerts/dipalert/state"
	"github.com/postmodernist1848/dip-alerts/dipalert/telegram"
)

type Tier struct {
	Name     string
	Drawdown float64
}
type Schedule func(time.Time) (cycle string, next time.Time)
type Watch struct {
	ID       string
	Provider market.Provider
	Lookback time.Duration
	Tiers    []Tier
	Schedule Schedule
}
type Result struct {
	Market       string     `json:"market"`
	CurrentPrice float64    `json:"currentPrice,omitempty"`
	RollingHigh  float64    `json:"rollingHigh,omitempty"`
	HighAt       *time.Time `json:"highAt,omitempty"`
	Drawdown     float64    `json:"drawdown,omitempty"`
	Alerted      []string   `json:"alerted,omitempty"`
	Error        string     `json:"error,omitempty"`
}
type Engine struct {
	Store   state.Store
	Sender  telegram.Sender
	Watches []Watch
	Now     func() time.Time
}

func (e *Engine) Run(ctx context.Context, deliver bool) ([]Result, error) {
	now := time.Now().UTC()
	if e.Now != nil {
		now = e.Now().UTC()
	}
	results := make([]Result, 0, len(e.Watches))
	var failures []error
	for _, watch := range e.Watches {
		result, err := e.runWatch(ctx, watch, now, deliver)
		results = append(results, result)
		if err != nil {
			failures = append(failures, err)
		}
	}
	return results, errors.Join(failures...)
}

func (e *Engine) runWatch(ctx context.Context, watch Watch, now time.Time, deliver bool) (Result, error) {
	result := Result{Market: watch.ID}
	stored, err := e.Store.Load(ctx, watch.ID)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	snapshot, err := watch.Provider.Snapshot(ctx, now.Add(-watch.Lookback), now)
	if err != nil {
		result.Error = err.Error()
		if deliver && !stored.Failed {
			_ = e.Sender.Send(ctx, "⚠️ "+watch.ID+" data API failed; buying alerts are skipped.\n"+err.Error())
		}
		stored.Failed = true
		_ = e.Store.Save(ctx, watch.ID, stored)
		return result, err
	}
	if snapshot.QuoteTime.IsZero() || now.Sub(snapshot.QuoteTime) > 15*time.Minute || snapshot.QuoteTime.After(now.Add(time.Minute)) {
		err = fmt.Errorf("%s quote is stale (%s)", snapshot.Source, snapshot.QuoteTime.Format(time.RFC3339))
		result.Error = err.Error()
		stored.Failed = true
		_ = e.Store.Save(ctx, watch.ID, stored)
		return result, err
	}
	high, highAt, err := market.RollingHigh(snapshot, now.Add(-watch.Lookback), now)
	if err != nil {
		result.Error = err.Error()
		stored.Failed = true
		_ = e.Store.Save(ctx, watch.ID, stored)
		return result, err
	}
	drawdown := snapshot.BestAsk/high - 1
	result.CurrentPrice = snapshot.BestAsk
	result.RollingHigh = high
	result.HighAt = &highAt
	result.Drawdown = drawdown
	cycle, next := watch.Schedule(now)
	if stored.Cycle != cycle {
		stored.Cycle = cycle
		for _, tier := range watch.Tiers {
			stored.Armed[tier.Name] = true
		}
	}
	for _, tier := range watch.Tiers {
		if !stored.Armed[tier.Name] && drawdown > tier.Drawdown*0.6 {
			stored.Armed[tier.Name] = true
		}
	}
	if deliver && stored.Failed {
		_ = e.Sender.Send(ctx, "✅ "+watch.ID+" data API recovered.")
	}
	stored.Failed = false
	var crossed []Tier
	for _, tier := range watch.Tiers {
		if stored.Armed[tier.Name] && drawdown <= tier.Drawdown {
			crossed = append(crossed, tier)
		}
	}
	if len(crossed) > 0 {
		sort.Slice(crossed, func(i, j int) bool { return crossed[i].Drawdown > crossed[j].Drawdown })
		for _, tier := range crossed {
			result.Alerted = append(result.Alerted, tier.Name)
		}
		if deliver {
			if err := e.Sender.Send(ctx, formatAlert(snapshot, high, highAt, drawdown, crossed, next)); err != nil {
				result.Error = err.Error()
				return result, err
			}
		}
		for _, tier := range crossed {
			stored.Armed[tier.Name] = false
		}
	}
	if deliver {
		if err := e.Store.Save(ctx, watch.ID, stored); err != nil {
			result.Error = err.Error()
			return result, err
		}
	}
	return result, nil
}

func formatAlert(snapshot market.Snapshot, high float64, highAt time.Time, drawdown float64, crossed []Tier, next time.Time) string {
	names := make([]string, len(crossed))
	for i, tier := range crossed {
		names[i] = tier.Name
	}
	return fmt.Sprintf("📉 Dip alert: %s\nCrossed: %s\nBest ask: %.2f %s\nRolling high: %.2f %s (%s)\nDrawdown: %.2f%%\nNext DCA: %s\nSource: %s\n\nInformational alert only.", snapshot.Market, strings.Join(names, ", "), snapshot.BestAsk, snapshot.Currency, high, snapshot.Currency, highAt.Format("2006-01-02 15:04 UTC"), drawdown*100, next.In(moscow()).Format("2006-01-02"), snapshot.Source)
}

func BTCSchedule(now time.Time) (string, time.Time) {
	local := now.In(moscow())
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, moscow())
	days := (int(start.Weekday()) - int(time.Sunday) + 7) % 7
	start = start.AddDate(0, 0, -days)
	return start.Format("2006-01-02"), start.AddDate(0, 0, 7)
}

func RUBSchedule(now time.Time) (string, time.Time) {
	local := now.In(moscow())
	y, m, d := local.Date()
	var start, next time.Time
	if d >= 17 {
		start = time.Date(y, m, 17, 0, 0, 0, 0, moscow())
		next = time.Date(y, m+1, 3, 0, 0, 0, 0, moscow())
	} else if d >= 3 {
		start = time.Date(y, m, 3, 0, 0, 0, 0, moscow())
		next = time.Date(y, m, 17, 0, 0, 0, 0, moscow())
	} else {
		start = time.Date(y, m-1, 17, 0, 0, 0, 0, moscow())
		next = time.Date(y, m, 3, 0, 0, 0, 0, moscow())
	}
	return start.Format("2006-01-02"), next
}

func moscow() *time.Location {
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return time.FixedZone("MSK", 3*60*60)
	}
	return location
}
