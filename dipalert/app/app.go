package app

import (
	"net/http"
	"time"

	"github.com/postmodernist1848/dip-alerts/dipalert/alert"
	"github.com/postmodernist1848/dip-alerts/dipalert/config"
	"github.com/postmodernist1848/dip-alerts/dipalert/market/cifra"
	"github.com/postmodernist1848/dip-alerts/dipalert/market/hyperliquid"
	"github.com/postmodernist1848/dip-alerts/dipalert/state"
	"github.com/postmodernist1848/dip-alerts/dipalert/telegram"
)

func Engine(c config.Config, store state.Store, sender telegram.Sender, client *http.Client) *alert.Engine {
	return &alert.Engine{Store: store, Sender: sender, Watches: []alert.Watch{
		{ID: "usdt-rub", Provider: cifra.New(c.CifraAPIBase, c.CifraAPIKey, c.CifraAPISecret, client), Lookback: 14 * 24 * time.Hour, Tiers: []alert.Tier{{Name: "−3%", Drawdown: -0.03}, {Name: "−5%", Drawdown: -0.05}}, Schedule: alert.RUBSchedule},
		{ID: "btc-usdc", Provider: hyperliquid.New(client), Lookback: 7 * 24 * time.Hour, Tiers: []alert.Tier{{Name: "−5%", Drawdown: -0.05}, {Name: "−10%", Drawdown: -0.10}}, Schedule: alert.BTCSchedule},
	}}
}
