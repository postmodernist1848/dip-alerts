package cifra

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/postmodernist1848/dip-alerts/dipalert/market"
)

type Client struct {
	BaseURL, APIKey, APISecret string
	Ticker                     string
	HTTPClient                 *http.Client
	Now                        func() time.Time
}

func New(baseURL, key, secret string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 8 * time.Second}
	}
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: key, APISecret: secret, Ticker: "USDT-RUB.IMEX", HTTPClient: httpClient, Now: time.Now}
}

func (c *Client) Snapshot(ctx context.Context, since, now time.Time) (market.Snapshot, error) {
	quoteRaw, err := c.signed(ctx, "getStockQuotesJson", map[string]any{"tickers": c.Ticker})
	if err != nil {
		return market.Snapshot{}, fmt.Errorf("Cifra quote: %w", err)
	}
	ask, quoteTime, err := parseQuote(quoteRaw, now)
	if err != nil {
		return market.Snapshot{}, fmt.Errorf("Cifra quote: %w", err)
	}
	candleRaw, err := c.signed(ctx, "getHloc", map[string]any{
		"id":           c.Ticker,
		"count":        -1,
		"date_from":    since.UTC().Format("02.01.2006 15:04"),
		"date_to":      now.UTC().Format("02.01.2006 15:04"),
		"timeframe":    60,
		"intervalMode": "ClosedRay",
	})
	if err != nil {
		return market.Snapshot{}, fmt.Errorf("Cifra candles: %w", err)
	}
	candles, err := parseCandles(candleRaw)
	if err != nil {
		return market.Snapshot{}, fmt.Errorf("Cifra candles: %w", err)
	}
	return market.Snapshot{Market: c.Ticker, Source: "Cifra Markets", Currency: "RUB", BestAsk: ask, QuoteTime: quoteTime, Candles: candles}, nil
}

func (c *Client) signed(ctx context.Context, command string, values map[string]any) ([]byte, error) {
	body, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	timestamp := strconv.FormatInt(c.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(c.APISecret))
	mac.Write(body)
	mac.Write([]byte(timestamp))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/"+command, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-NtApi-PublicKey", c.APIKey)
	req.Header.Set("X-NtApi-Timestamp", timestamp)
	req.Header.Set("X-NtApi-Sig", hex.EncodeToString(mac.Sum(nil)))
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(raw))
		contentType := strings.ToLower(resp.Header.Get("Content-Type"))
		if strings.Contains(contentType, "text/html") || strings.Contains(strings.ToLower(message), "cloudflare") {
			if strings.Contains(strings.ToLower(message), "cloudflare") {
				return nil, fmt.Errorf("HTTP %d (Cloudflare block)", resp.StatusCode)
			}
			return nil, fmt.Errorf("HTTP %d (HTML response)", resp.StatusCode)
		}
		if len(message) > 300 {
			message = message[:300] + "…"
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, message)
	}
	var envelope struct {
		Code   *int   `json:"code"`
		ErrMsg string `json:"errMsg"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Code != nil && *envelope.Code != 0 {
		message := strings.TrimSpace(envelope.ErrMsg)
		if message == "" {
			message = "unspecified API error"
		}
		return nil, fmt.Errorf("API code %d: %s", *envelope.Code, message)
	}
	return raw, nil
}

func parseQuote(raw []byte, fallback time.Time) (float64, time.Time, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, time.Time{}, err
	}
	objects := collectObjects(value)
	for _, object := range objects {
		ask := number(object, "bap", "bestAsk", "best_ask", "ask")
		if ask <= 0 {
			continue
		}
		at := fallback.UTC()
		if text, ok := stringValue(object, "ltt", "time", "timestamp"); ok {
			if parsed, err := parseTime(text); err == nil {
				at = parsed
			}
		}
		return ask, at, nil
	}
	return 0, time.Time{}, fmt.Errorf("best ask not found in response")
}

func parseCandles(raw []byte) ([]market.Candle, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	if data, ok := root["data"].(map[string]any); ok {
		root = data
	}
	xs := tickerArray(root["xSeries"])
	hloc := tickerArray(root["hloc"])
	if len(xs) == 0 || len(hloc) == 0 {
		return nil, fmt.Errorf("xSeries/hloc arrays not found")
	}
	count := len(xs)
	if len(hloc) < count {
		count = len(hloc)
	}
	result := make([]market.Candle, 0, count)
	for i := 0; i < count; i++ {
		row, ok := hloc[i].([]any)
		if !ok || len(row) < 4 {
			continue
		}
		closePrice := asFloat(row[3])
		if closePrice <= 0 {
			continue
		}
		closed, err := parseTime(fmt.Sprint(xs[i]))
		if err != nil {
			continue
		}
		result = append(result, market.Candle{Close: closePrice, Closed: closed.Add(time.Hour)})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no valid hourly candles")
	}
	return result, nil
}

func collectObjects(value any) []map[string]any {
	var result []map[string]any
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			result = append(result, typed)
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return result
}

func number(object map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if v, ok := object[key]; ok {
			if n := asFloat(v); n != 0 {
				return n
			}
		}
	}
	return 0
}
func asFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case string:
		n, _ := strconv.ParseFloat(v, 64)
		return n
	case json.Number:
		n, _ := v.Float64()
		return n
	}
	return 0
}
func stringValue(object map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if v, ok := object[key]; ok {
			return fmt.Sprint(v), true
		}
	}
	return "", false
}
func parseTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		if n < 1e12 {
			n *= 1000
		}
		return time.UnixMilli(n).UTC(), nil
	}
	if n, err := strconv.ParseFloat(value, 64); err == nil {
		millis := int64(n)
		if n < 1e12 {
			millis = int64(n * 1000)
		}
		return time.UnixMilli(millis).UTC(), nil
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC(), nil
	}
	moscow := time.FixedZone("MSK", 3*60*60)
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, moscow); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
}

func tickerArray(value any) []any {
	if direct, ok := value.([]any); ok {
		return direct
	}
	if keyed, ok := value.(map[string]any); ok {
		for _, candidate := range keyed {
			if items, ok := candidate.([]any); ok {
				return items
			}
		}
	}
	return nil
}
