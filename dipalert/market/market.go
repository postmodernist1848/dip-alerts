package market

import (
	"context"
	"fmt"
	"time"
)

type Candle struct {
	Close  float64
	Closed time.Time
}

type Snapshot struct {
	Market    string
	Source    string
	Currency  string
	BestAsk   float64
	QuoteTime time.Time
	Candles   []Candle
}

type Provider interface {
	Snapshot(ctx context.Context, since, now time.Time) (Snapshot, error)
}

func RollingHigh(snapshot Snapshot, since, now time.Time) (float64, time.Time, error) {
	var high float64
	var at time.Time
	for _, candle := range snapshot.Candles {
		if candle.Closed.Before(since) || candle.Closed.After(now) || candle.Close <= 0 {
			continue
		}
		if candle.Close > high {
			high, at = candle.Close, candle.Closed
		}
	}
	if high == 0 {
		return 0, time.Time{}, fmt.Errorf("%s returned no completed candles in the requested window", snapshot.Source)
	}
	return high, at, nil
}
