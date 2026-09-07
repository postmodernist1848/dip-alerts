package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/postmodernist1848/dip-alerts/dipalert/app"
	"github.com/postmodernist1848/dip-alerts/dipalert/config"
	"github.com/postmodernist1848/dip-alerts/dipalert/state"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	c := config.Config{
		CifraAPIKey:    strings.TrimSpace(os.Getenv("CIFRA_API_KEY")),
		CifraAPISecret: strings.TrimSpace(os.Getenv("CIFRA_API_SECRET")),
		CifraAPIBase:   strings.TrimRight(strings.TrimSpace(os.Getenv("CIFRA_API_BASE")), "/"),
	}
	if c.CifraAPIBase == "" {
		c.CifraAPIBase = "https://tradernet.by/api"
	}
	if c.CifraAPIKey == "" || c.CifraAPISecret == "" {
		http.Error(w, "market data configuration is unavailable", http.StatusServiceUnavailable)
		return
	}
	checkedAt := time.Now().UTC()
	results, err := app.Engine(c, state.NewMemory(), nil, nil).Run(r.Context(), false)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, s-maxage=60, stale-while-revalidate=30")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"checkedAt": checkedAt.Format(time.RFC3339),
		"ok":        err == nil,
		"results":   results,
	})
}
