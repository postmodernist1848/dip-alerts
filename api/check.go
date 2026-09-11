package handler

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/postmodernist1848/dip-alerts/dipalerts/alert"
	"github.com/postmodernist1848/dip-alerts/dipalerts/app"
	"github.com/postmodernist1848/dip-alerts/dipalerts/config"
	"github.com/postmodernist1848/dip-alerts/dipalerts/state/upstash"
	"github.com/postmodernist1848/dip-alerts/dipalerts/telegram"
)

var runtime struct {
	sync.Once
	engine *alert.Engine
	store  *upstash.Store
	secret string
	err    error
}

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	initialize()
	if runtime.err != nil {
		http.Error(w, "service configuration is unavailable", http.StatusServiceUnavailable)
		return
	}
	provided := strings.TrimPrefix(strings.TrimSpace(r.Header.Get("Authorization")), "Bearer ")
	if len(provided) != len(runtime.secret) || subtle.ConstantTimeCompare([]byte(provided), []byte(runtime.secret)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	results, err := runtime.engine.Run(r.Context(), true)
	checkedAt := time.Now().UTC()
	_ = runtime.store.SaveLastCheck(r.Context(), map[string]any{
		"checkedAt": checkedAt.Format(time.RFC3339),
		"ok":        err == nil,
		"results":   results,
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"checkedAt": checkedAt.Format(time.RFC3339), "results": results, "ok": err == nil})
}

func initialize() {
	runtime.Do(func() {
		c, err := config.Load()
		if err != nil {
			runtime.err = err
			return
		}
		runtime.secret = c.SchedulerSecret
		store := upstash.New(c.RedisURL, c.RedisToken, nil)
		runtime.store = store
		sender := telegram.New(c.TelegramToken, c.TelegramChatID, nil)
		runtime.engine = app.Engine(c, store, sender, nil)
	})
}
