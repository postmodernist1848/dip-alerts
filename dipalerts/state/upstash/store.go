package upstash

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/postmodernist1848/dip-alerts/dipalerts/state"
)

type Store struct {
	url, token string
	client     *http.Client
}

func New(url, token string, client *http.Client) *Store {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &Store{url: strings.TrimRight(url, "/"), token: token, client: client}
}

func (s *Store) Load(ctx context.Context, marketName string) (state.MarketState, error) {
	result, err := s.command(ctx, "GET", key(marketName))
	if err != nil {
		return state.MarketState{}, err
	}
	if len(result) == 0 || bytes.Equal(result, []byte("null")) {
		return state.MarketState{Armed: map[string]bool{}}, nil
	}
	var encoded string
	if err := json.Unmarshal(result, &encoded); err != nil {
		return state.MarketState{}, fmt.Errorf("decode Redis value: %w", err)
	}
	var value state.MarketState
	if err := json.Unmarshal([]byte(encoded), &value); err != nil {
		return state.MarketState{}, fmt.Errorf("decode market state: %w", err)
	}
	if value.Armed == nil {
		value.Armed = map[string]bool{}
	}
	return value, nil
}

func (s *Store) Save(ctx context.Context, marketName string, value state.MarketState) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.command(ctx, "SET", key(marketName), string(raw))
	return err
}

func (s *Store) SaveLastCheck(ctx context.Context, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.command(ctx, "SET", "dip-alerts:v1:last-check", string(raw))
	return err
}

func (s *Store) LoadLastCheck(ctx context.Context) (json.RawMessage, error) {
	result, err := s.command(ctx, "GET", "dip-alerts:v1:last-check")
	if err != nil {
		return nil, err
	}
	if len(result) == 0 || bytes.Equal(result, []byte("null")) {
		return nil, nil
	}
	var encoded string
	if err := json.Unmarshal(result, &encoded); err != nil {
		return nil, fmt.Errorf("decode Redis value: %w", err)
	}
	if !json.Valid([]byte(encoded)) {
		return nil, fmt.Errorf("decode last check: invalid JSON")
	}
	return json.RawMessage(encoded), nil
}

func (s *Store) command(ctx context.Context, args ...string) (json.RawMessage, error) {
	body, _ := json.Marshal(args)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call Upstash Redis: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Upstash Redis HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode Upstash response: %w", err)
	}
	if envelope.Error != "" {
		return nil, fmt.Errorf("Upstash Redis: %s", envelope.Error)
	}
	return envelope.Result, nil
}

func key(marketName string) string { return "dip-alerts:v1:market:" + marketName }
