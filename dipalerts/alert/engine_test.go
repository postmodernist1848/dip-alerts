package alert

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/postmodernist1848/dip-alerts/dipalerts/market"
	"github.com/postmodernist1848/dip-alerts/dipalerts/state"
)

type fakeProvider struct {
	ask float64
	err error
	now time.Time
}

func (f *fakeProvider) Snapshot(context.Context, time.Time, time.Time) (market.Snapshot, error) {
	if f.err != nil {
		return market.Snapshot{}, f.err
	}
	return market.Snapshot{Market: "BTC", Source: "test", Currency: "USD", BestAsk: f.ask, QuoteTime: f.now, Candles: []market.Candle{{Close: 100, Closed: f.now.Add(-time.Hour)}}}, nil
}

type fakeSender struct{ messages []string }

func (f *fakeSender) Send(_ context.Context, text string) error {
	f.messages = append(f.messages, text)
	return nil
}

func TestTierAlertRecoveryAndCalendarReset(t *testing.T) {
	now := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	provider := &fakeProvider{ask: 94, now: now}
	sender := &fakeSender{}
	store := state.NewMemory()
	engine := &Engine{Store: store, Sender: sender, Now: func() time.Time { return now }, Watches: []Watch{{ID: "btc", Provider: provider, Lookback: 7 * 24 * time.Hour, Tiers: []Tier{{Name: "-5", Drawdown: -.05}, {Name: "-10", Drawdown: -.10}}, Schedule: BTCSchedule}}}
	run := func() Result {
		results, err := engine.Run(context.Background(), true)
		if err != nil {
			t.Fatal(err)
		}
		return results[0]
	}
	if got := run().Alerted; len(got) != 1 || got[0] != "-5" {
		t.Fatalf("first alert = %v", got)
	}
	if got := run().Alerted; len(got) != 0 {
		t.Fatalf("duplicate alert = %v", got)
	}
	provider.ask = 97.1 // above the -3% recovery boundary
	if got := run().Alerted; len(got) != 0 {
		t.Fatalf("recovery alert = %v", got)
	}
	provider.ask = 94
	if got := run().Alerted; len(got) != 1 || got[0] != "-5" {
		t.Fatalf("recross alert = %v", got)
	}
	now = now.AddDate(0, 0, 7)
	provider.now = now
	if got := run().Alerted; len(got) != 1 || got[0] != "-5" {
		t.Fatalf("calendar reset alert = %v", got)
	}
}

func TestDirectJumpCombinesTiers(t *testing.T) {
	now := time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)
	provider := &fakeProvider{ask: 89, now: now}
	sender := &fakeSender{}
	engine := &Engine{Store: state.NewMemory(), Sender: sender, Now: func() time.Time { return now }, Watches: []Watch{{ID: "btc", Provider: provider, Lookback: 7 * 24 * time.Hour, Tiers: []Tier{{Name: "-5", Drawdown: -.05}, {Name: "-10", Drawdown: -.10}}, Schedule: BTCSchedule}}}
	results, err := engine.Run(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(results[0].Alerted) != 2 || len(sender.messages) != 1 {
		t.Fatalf("result=%+v messages=%d", results[0], len(sender.messages))
	}
}

func TestFailureAlertSentOnceAndRecovery(t *testing.T) {
	now := time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)
	provider := &fakeProvider{err: errors.New("offline"), now: now}
	sender := &fakeSender{}
	engine := &Engine{Store: state.NewMemory(), Sender: sender, Now: func() time.Time { return now }, Watches: []Watch{{ID: "btc", Provider: provider, Lookback: time.Hour, Tiers: []Tier{{Name: "-5", Drawdown: -.05}}, Schedule: BTCSchedule}}}
	_, _ = engine.Run(context.Background(), true)
	_, _ = engine.Run(context.Background(), true)
	if len(sender.messages) != 1 || !strings.Contains(sender.messages[0], "failed") {
		t.Fatalf("failure messages=%v", sender.messages)
	}
	provider.err = nil
	provider.ask = 100
	if _, err := engine.Run(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(sender.messages) != 2 || !strings.Contains(sender.messages[1], "recovered") {
		t.Fatalf("recovery messages=%v", sender.messages)
	}
}

func TestSchedules(t *testing.T) {
	moscow, _ := time.LoadLocation("Europe/Moscow")
	cycle, next := RUBSchedule(time.Date(2026, 3, 1, 12, 0, 0, 0, moscow))
	if cycle != "2026-02-17" || next.Day() != 3 {
		t.Fatalf("rub cycle=%s next=%s", cycle, next)
	}
	cycle, next = BTCSchedule(time.Date(2026, 9, 9, 12, 0, 0, 0, moscow))
	if cycle != "2026-09-06" || next.Day() != 13 {
		t.Fatalf("btc cycle=%s next=%s", cycle, next)
	}
}
