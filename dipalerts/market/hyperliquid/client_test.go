package hyperliquid

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSnapshotResolvesSpotAndParsesData(t *testing.T) {
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var request map[string]any
		_ = json.NewDecoder(r.Body).Decode(&request)
		var body string
		switch request["type"] {
		case "spotMeta":
			body = `{"tokens":[{"index":0,"name":"USDC"},{"index":1,"name":"UBTC"}],"universe":[{"name":"@1","tokens":[1,0]}]}`
		case "l2Book":
			body = `{"time":1788775200000,"levels":[[{"px":"99900"}],[{"px":"100100"}]]}`
		case "candleSnapshot":
			body = `[{"t":1788768000000,"T":1788771599999,"c":"100000"}]`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	c := New(httpClient)
	c.URL = "https://example.invalid"
	snapshot, err := c.Snapshot(t.Context(), now.Add(-7*24*time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.BestAsk != 100100 || len(snapshot.Candles) != 1 || snapshot.Candles[0].Close != 100000 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
