package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	TelegramToken   string
	TelegramChatID  string
	SchedulerSecret string
	CifraAPIKey     string
	CifraAPISecret  string
	CifraAPIBase    string
	RedisURL        string
	RedisToken      string
}

func Load() (Config, error) {
	c := Config{
		TelegramToken:   strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramChatID:  strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID")),
		SchedulerSecret: strings.TrimSpace(os.Getenv("SCHEDULER_SECRET")),
		CifraAPIKey:     strings.TrimSpace(os.Getenv("CIFRA_API_KEY")),
		CifraAPISecret:  strings.TrimSpace(os.Getenv("CIFRA_API_SECRET")),
		CifraAPIBase:    strings.TrimRight(strings.TrimSpace(os.Getenv("CIFRA_API_BASE")), "/"),
		RedisURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("KV_REST_API_URL")), "/"),
		RedisToken:      strings.TrimSpace(os.Getenv("KV_REST_API_TOKEN")),
	}
	if c.CifraAPIBase == "" {
		c.CifraAPIBase = "https://tradernet.by/api"
	}
	if c.TelegramToken == "" || c.TelegramChatID == "" || c.SchedulerSecret == "" ||
		c.CifraAPIKey == "" || c.CifraAPISecret == "" || c.RedisURL == "" || c.RedisToken == "" {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID, SCHEDULER_SECRET, CIFRA_API_KEY, CIFRA_API_SECRET, KV_REST_API_URL and KV_REST_API_TOKEN are required")
	}
	return c, nil
}
