package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/postmodernist1848/dip-alert/internal/app"
	"github.com/postmodernist1848/dip-alert/internal/config"
	"github.com/postmodernist1848/dip-alert/internal/state"
)

func main() {
	c := config.Config{CifraAPIKey: strings.TrimSpace(os.Getenv("CIFRA_API_KEY")), CifraAPISecret: strings.TrimSpace(os.Getenv("CIFRA_API_SECRET")), CifraAPIBase: strings.TrimRight(strings.TrimSpace(os.Getenv("CIFRA_API_BASE")), "/")}
	if c.CifraAPIBase == "" {
		c.CifraAPIBase = "https://tradernet.com/api"
	}
	if c.CifraAPIKey == "" || c.CifraAPISecret == "" {
		fmt.Fprintln(os.Stderr, "CIFRA_API_KEY and CIFRA_API_SECRET are required")
		os.Exit(2)
	}
	results, err := app.Engine(c, state.NewMemory(), nil, nil).Run(context.Background(), false)
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"results": results, "ok": err == nil})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
