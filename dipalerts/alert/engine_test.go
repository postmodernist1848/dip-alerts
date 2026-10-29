package alert

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/postmodernist1848/dip-alerts/dipalerts/market"
	"github.com/postmodernist1848/dip-alerts/dipalerts/market/cifra"
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

func TestFailureAlertAfterTwelveRunsAndRecovery(t *testing.T) {
	now := time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)
	provider := &fakeProvider{err: errors.New("offline"), now: now}
	sender := &fakeSender{}
	engine := &Engine{Store: state.NewMemory(), Sender: sender, Now: func() time.Time { return now }, Watches: []Watch{{ID: "btc", Provider: provider, Lookback: time.Hour, Tiers: []Tier{{Name: "-5", Drawdown: -.05}}, Schedule: BTCSchedule}}}
	for range failureAlertThreshold - 1 {
		_, _ = engine.Run(context.Background(), true)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("early failure messages=%v", sender.messages)
	}
	_, _ = engine.Run(context.Background(), true)
	_, _ = engine.Run(context.Background(), true)
	if len(sender.messages) != 1 || !strings.Contains(sender.messages[0], "12 consecutive checks") {
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

func TestShortFailureDoesNotSendRecovery(t *testing.T) {
	now := time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)
	provider := &fakeProvider{err: errors.New("offline"), now: now}
	sender := &fakeSender{}
	engine := &Engine{Store: state.NewMemory(), Sender: sender, Now: func() time.Time { return now }, Watches: []Watch{{ID: "btc", Provider: provider, Lookback: time.Hour, Schedule: BTCSchedule}}}
	for range failureAlertThreshold - 1 {
		_, _ = engine.Run(context.Background(), true)
	}
	provider.err = nil
	provider.ask = 100
	if _, err := engine.Run(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("messages=%v", sender.messages)
	}
}

func TestReadOnlyFailureDoesNotChangeState(t *testing.T) {
	now := time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)
	store := state.NewMemory()
	engine := &Engine{Store: store, Sender: &fakeSender{}, Now: func() time.Time { return now }, Watches: []Watch{{ID: "btc", Provider: &fakeProvider{err: errors.New("offline")}, Lookback: time.Hour, Schedule: BTCSchedule}}}
	for range failureAlertThreshold {
		_, _ = engine.Run(context.Background(), false)
	}
	stored, err := store.Load(context.Background(), "btc")
	if err != nil {
		t.Fatal(err)
	}
	if stored.ConsecutiveFailures != 0 || stored.Failed {
		t.Fatalf("read-only run changed state: %+v", stored)
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

func TestCifraOvernightStalenessAndRecovery(t *testing.T) {
	for _, outlastsWindow := range []bool{false, true} {
		name := "recovers during quiet period"
		if outlastsWindow {
			name = "outage continues after cutoff"
		}
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 10, 5, 23, 15, 0, 0, time.UTC)
			provider := &fakeProvider{ask: 90, now: time.Date(2026, 10, 5, 22, 53, 54, 0, time.UTC)}
			sender := &fakeSender{}
			store := state.NewMemory()
			engine := &Engine{Store: store, Sender: sender, Now: func() time.Time { return now }, Watches: []Watch{{
				ID: "usdt-rub", Provider: provider, Lookback: 14 * 24 * time.Hour,
				Tiers: []Tier{{Name: "-3", Drawdown: -.03}}, Schedule: RUBSchedule,
				SuppressStaleAlert: cifra.SuppressOvernightStaleAlert,
			}}}
			const quietChecks = 15
			for range quietChecks {
				results, err := engine.Run(t.Context(), true)
				if err == nil || results[0].Error == "" || !results[0].FailureAlertSuppressed || len(results[0].Alerted) != 0 {
					t.Fatalf("stale quote was not visibly skipped: results=%+v err=%v", results, err)
				}
				now = now.Add(5 * time.Minute)
			}
			if len(sender.messages) != 0 {
				t.Fatalf("quiet period sent notifications: %v", sender.messages)
			}
			stored, _ := store.Load(t.Context(), "usdt-rub")
			if stored.Failed || stored.ConsecutiveFailures != 0 || stored.Cycle != "" || len(stored.Armed) != 0 {
				t.Fatalf("quiet failures changed alert state: %+v", stored)
			}
			if outlastsWindow {
				now = time.Date(2026, 10, 6, 2, 10, 0, 0, time.UTC)
				for range failureAlertThreshold - 1 {
					_, _ = engine.Run(t.Context(), true)
				}
				if len(sender.messages) != 0 {
					t.Fatalf("alert before 12 checks outside window: %v", sender.messages)
				}
				results, err := engine.Run(t.Context(), true)
				if err == nil || results[0].FailureAlertSuppressed || len(sender.messages) != 1 || !strings.Contains(sender.messages[0], "data API failed") {
					t.Fatalf("continuing outage was not alerted: results=%+v err=%v messages=%v", results, err, sender.messages)
				}
				_, _ = engine.Run(t.Context(), true)
				if len(sender.messages) != 1 {
					t.Fatalf("duplicate outage notification: %v", sender.messages)
				}
			}
			provider.now, provider.ask = now, 100
			if _, err := engine.Run(t.Context(), true); err != nil {
				t.Fatal(err)
			}
			wantMessages := 0
			if outlastsWindow {
				wantMessages = 2
			}
			if len(sender.messages) != wantMessages {
				t.Fatalf("recovery notifications: %v", sender.messages)
			}
			if outlastsWindow && !strings.Contains(sender.messages[1], "recovered") {
				t.Fatalf("missing recovery: %v", sender.messages)
			}
			stored, _ = store.Load(t.Context(), "usdt-rub")
			if stored.Failed || stored.ConsecutiveFailures != 0 {
				t.Fatalf("recovery did not reset failure state: %+v", stored)
			}
		})
	}
}

func TestCifraQuietPeriodSuppressesAllDataFailures(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	nightlyQuote := time.Date(2026, 10, 5, 22, 53, 54, 0, time.UTC)
	tests := []struct {
		name, marketID string
		quote          time.Time
		failure        error
	}{
		{"network or API error", "usdt-rub", nightlyQuote, errors.New("offline")},
		{"missing timestamp", "usdt-rub", time.Time{}, nil},
		{"future timestamp", "usdt-rub", now.Add(2 * time.Minute), nil},
		{"quote outside pattern", "usdt-rub", now.Add(-6 * time.Hour), nil},
		{"quote from previous night", "usdt-rub", nightlyQuote.Add(-24 * time.Hour), nil},
		{"another provider", "btc-usdc", nightlyQuote, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &fakeSender{}
			watch := Watch{ID: tt.marketID, Provider: &fakeProvider{ask: 100, now: tt.quote, err: tt.failure}, Lookback: 14 * 24 * time.Hour, Schedule: RUBSchedule}
			if tt.marketID == "usdt-rub" {
				watch.SuppressStaleAlert = cifra.SuppressOvernightStaleAlert
			}
			engine := &Engine{Store: state.NewMemory(), Sender: sender, Now: func() time.Time { return now }, Watches: []Watch{watch}}
			for range failureAlertThreshold {
				results, err := engine.Run(t.Context(), true)
				if err == nil || results[0].FailureAlertSuppressed != (tt.marketID == "usdt-rub") {
					t.Fatalf("unexpected suppression: results=%+v err=%v", results, err)
				}
			}
			if (tt.marketID == "usdt-rub" && len(sender.messages) != 0) || (tt.marketID != "usdt-rub" && len(sender.messages) != 1) {
				t.Fatalf("failure notifications: %v", sender.messages)
			}
		})
	}
}

func TestReadOnlyQuietPeriodDoesNotChangeState(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	store := state.NewMemory()
	sender := &fakeSender{}
	engine := &Engine{Store: store, Sender: sender, Now: func() time.Time { return now }, Watches: []Watch{{
		ID: "usdt-rub", Provider: &fakeProvider{ask: 90, now: time.Date(2026, 10, 5, 22, 53, 54, 0, time.UTC)},
		Lookback: 14 * 24 * time.Hour, Schedule: RUBSchedule, SuppressStaleAlert: cifra.SuppressOvernightStaleAlert,
	}}}
	results, err := engine.Run(t.Context(), false)
	if err == nil || !results[0].FailureAlertSuppressed {
		t.Fatalf("read-only result=%+v err=%v", results, err)
	}
	stored, _ := store.Load(t.Context(), "usdt-rub")
	if stored.ConsecutiveFailures != 0 || stored.Failed || len(sender.messages) != 0 {
		t.Fatalf("read-only run changed state: state=%+v messages=%v", stored, sender.messages)
	}
}
