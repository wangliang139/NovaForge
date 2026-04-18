package simulate

import (
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// Ticker is a lightweight top-level price update used by simulator.
type Ticker struct {
	Exchange Exchange        `json:"exchange,omitempty"`
	Symbol   Symbol          `json:"symbol,omitempty"`
	Last     decimal.Decimal `json:"last,omitempty"`
	Mark     decimal.Decimal `json:"mark,omitempty"`
	Index    decimal.Decimal `json:"index,omitempty"`
	Ts       time.Time       `json:"ts,omitempty"`
}

// TickerStore caches latest ticker by symbol.
type TickerStore struct {
	mu   sync.RWMutex
	data map[Symbol]Ticker
}

func NewTickerStore() *TickerStore {
	return &TickerStore{data: make(map[Symbol]Ticker)}
}

func (s *TickerStore) Update(t Ticker) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.data[t.Symbol]
	if ok {
		// Keep ticker time monotonic in replay/event scenarios.
		if !t.Ts.IsZero() && t.Ts.Before(prev.Ts) {
			return
		}
		// Keep latest non-zero mark/index when source message lacks them.
		if !t.Mark.GreaterThan(decimal.Zero) {
			t.Mark = prev.Mark
		}
		if !t.Index.GreaterThan(decimal.Zero) {
			t.Index = prev.Index
		}
		if !t.Last.GreaterThan(decimal.Zero) {
			t.Last = prev.Last
		}
	}
	s.data[t.Symbol] = t
}

func (s *TickerStore) Get(sym Symbol) (Ticker, bool) {
	if s == nil {
		return Ticker{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.data[sym]
	return t, ok
}
