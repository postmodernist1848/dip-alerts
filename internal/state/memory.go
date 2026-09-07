package state

import (
	"context"
	"sync"
)

type Memory struct {
	mu     sync.Mutex
	values map[string]MarketState
}

func NewMemory() *Memory { return &Memory{values: map[string]MarketState{}} }
func (m *Memory) Load(_ context.Context, name string) (MarketState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value := m.values[name]
	if value.Armed == nil {
		value.Armed = map[string]bool{}
	}
	return value, nil
}
func (m *Memory) Save(_ context.Context, name string, value MarketState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[name] = value
	return nil
}
