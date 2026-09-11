package hyperliquid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/postmodernist1848/dip-alerts/dipalerts/market"
)

const defaultURL = "https://api.hyperliquid.xyz/info"

type Client struct {
	URL        string
	HTTPClient *http.Client
}

func New(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 8 * time.Second}
	}
	return &Client{URL: defaultURL, HTTPClient: httpClient}
}

func (c *Client) Snapshot(ctx context.Context, since, now time.Time) (market.Snapshot, error) {
	coin, err := c.resolveSpot(ctx)
	if err != nil {
		return market.Snapshot{}, err
	}
	var book struct {
		Time   int64 `json:"time"`
		Levels [][]struct {
			Px string `json:"px"`
		} `json:"levels"`
	}
	if err := c.post(ctx, map[string]any{"type": "l2Book", "coin": coin}, &book); err != nil {
		return market.Snapshot{}, fmt.Errorf("Hyperliquid order book: %w", err)
	}
	if len(book.Levels) < 2 || len(book.Levels[1]) == 0 {
		return market.Snapshot{}, fmt.Errorf("Hyperliquid returned no UBTC/USDC asks")
	}
	ask, err := strconv.ParseFloat(book.Levels[1][0].Px, 64)
	if err != nil || ask <= 0 {
		return market.Snapshot{}, fmt.Errorf("Hyperliquid returned invalid best ask %q", book.Levels[1][0].Px)
	}
	var raw []struct {
		OpenTime  int64  `json:"t"`
		CloseTime int64  `json:"T"`
		Close     string `json:"c"`
	}
	payload := map[string]any{"type": "candleSnapshot", "req": map[string]any{
		"coin": coin, "interval": "1h", "startTime": since.UnixMilli(), "endTime": now.UnixMilli(),
	}}
	if err := c.post(ctx, payload, &raw); err != nil {
		return market.Snapshot{}, fmt.Errorf("Hyperliquid candles: %w", err)
	}
	candles := make([]market.Candle, 0, len(raw))
	for _, item := range raw {
		closePrice, parseErr := strconv.ParseFloat(item.Close, 64)
		if parseErr != nil || closePrice <= 0 {
			continue
		}
		closed := time.UnixMilli(item.CloseTime)
		if item.CloseTime == 0 {
			closed = time.UnixMilli(item.OpenTime).Add(time.Hour)
		}
		if !closed.After(now) {
			candles = append(candles, market.Candle{Close: closePrice, Closed: closed.UTC()})
		}
	}
	return market.Snapshot{Market: "UBTC/USDC", Source: "Hyperliquid", Currency: "USDC", BestAsk: ask, QuoteTime: time.UnixMilli(book.Time).UTC(), Candles: candles}, nil
}

func (c *Client) resolveSpot(ctx context.Context) (string, error) {
	var meta struct {
		Tokens []struct {
			Index int    `json:"index"`
			Name  string `json:"name"`
		} `json:"tokens"`
		Universe []struct {
			Name   string `json:"name"`
			Tokens []int  `json:"tokens"`
		} `json:"universe"`
	}
	if err := c.post(ctx, map[string]any{"type": "spotMeta"}, &meta); err != nil {
		return "", fmt.Errorf("Hyperliquid spot metadata: %w", err)
	}
	names := make(map[int]string, len(meta.Tokens))
	for _, token := range meta.Tokens {
		names[token.Index] = token.Name
	}
	for _, pair := range meta.Universe {
		if len(pair.Tokens) == 2 && names[pair.Tokens[0]] == "UBTC" && names[pair.Tokens[1]] == "USDC" {
			return pair.Name, nil
		}
	}
	return "", fmt.Errorf("Hyperliquid UBTC/USDC spot market was not found")
}

func (c *Client) post(ctx context.Context, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
