package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Sender interface {
	Send(ctx context.Context, text string) error
}

type Client struct {
	Token, ChatID string
	HTTPClient    *http.Client
}

func New(token, chatID string, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &Client{Token: token, ChatID: chatID, HTTPClient: client}
}

func (c *Client) Send(ctx context.Context, text string) error {
	body, _ := json.Marshal(map[string]any{"chat_id": c.ChatID, "text": text, "disable_web_page_preview": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+c.Token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("send Telegram message: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var envelope struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if json.Unmarshal(raw, &envelope) != nil || resp.StatusCode >= 300 || !envelope.OK {
		return fmt.Errorf("Telegram HTTP %d: %s", resp.StatusCode, strings.TrimSpace(envelope.Description))
	}
	return nil
}
