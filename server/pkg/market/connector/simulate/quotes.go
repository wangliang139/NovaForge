package simulate

import (
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// QuoteTick caches last/mark/index per symbol (thread-safe).
type QuoteTick struct {
	Exchange Exchange
	Symbol   Symbol
	Last     decimal.Decimal
	Mark     decimal.Decimal
	Index    decimal.Decimal
	Ts       time.Time
}

// QuoteCache stores latest quote per symbol.
type QuoteCache struct {
	mu   sync.RWMutex
	data map[Symbol]QuoteTick
}

// NewQuoteCache creates an empty cache.
func NewQuoteCache() *QuoteCache {
	return &QuoteCache{data: make(map[Symbol]QuoteTick)}
}

// Update merges a tick (monotonic ts; preserve non-zero fields).
func (s *QuoteCache) Update(t QuoteTick) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.data[t.Symbol]
	if ok {
		if !t.Ts.IsZero() && t.Ts.Before(prev.Ts) {
			return
		}
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

// Get returns the latest tick.
func (s *QuoteCache) Get(sym Symbol) (QuoteTick, bool) {
	if s == nil {
		return QuoteTick{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.data[sym]
	return t, ok
}

// MarkIndexService is a thin facade over QuoteCache for risk modules.
type MarkIndexService struct {
	q *QuoteCache
}

// NewMarkIndexService wraps a quote cache.
func NewMarkIndexService(q *QuoteCache) *MarkIndexService {
	return &MarkIndexService{q: q}
}

// MarkForRisk returns mark if positive, else last, else zero.
func (m *MarkIndexService) MarkForRisk(sym Symbol) (decimal.Decimal, time.Time, bool) {
	if m == nil || m.q == nil {
		return decimal.Zero, time.Time{}, false
	}
	tk, ok := m.q.Get(sym)
	if !ok {
		return decimal.Zero, time.Time{}, false
	}
	mp := tk.Mark
	if !mp.GreaterThan(decimal.Zero) {
		mp = tk.Last
	}
	if !mp.GreaterThan(decimal.Zero) {
		return decimal.Zero, time.Time{}, false
	}
	return mp, tk.Ts, true
}
