package state

import "context"

type MarketState struct {
	Cycle  string          `json:"cycle"`
	Armed  map[string]bool `json:"armed"`
	Failed bool            `json:"failed"`
}

type Store interface {
	Load(ctx context.Context, market string) (MarketState, error)
	Save(ctx context.Context, market string, value MarketState) error
}
