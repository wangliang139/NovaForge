package simulate

import (
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// RestingBook holds user resting limit orders for one symbol (hidden from public L2).
type RestingBook struct {
	mu     sync.Mutex
	symbol Symbol
	nowFn  func() time.Time

	bidLevels map[string][]*Order
	askLevels map[string][]*Order
	byID      map[string]*Order
}

// NewRestingBook creates an empty book for symbol.
func NewRestingBook(sym Symbol, nowFn func() time.Time) *RestingBook {
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	return &RestingBook{
		symbol:    sym,
		nowFn:     nowFn,
		bidLevels: make(map[string][]*Order),
		askLevels: make(map[string][]*Order),
		byID:      make(map[string]*Order),
	}
}

func priceKey(p decimal.Decimal) string {
	return p.String()
}

// PutOrderRecord stores any terminal or active order for GetOrder.
func (b *RestingBook) PutOrderRecord(o *Order) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.byID[o.ID] = o
}

// AddResting inserts a limit order (must have Remaining > 0).
func (b *RestingBook) AddResting(o *Order) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.byID[o.ID] = o
	pk := priceKey(o.Price)
	if o.Side == SideBuy {
		b.bidLevels[pk] = append(b.bidLevels[pk], o)
	} else {
		b.askLevels[pk] = append(b.askLevels[pk], o)
	}
}

// Cancel removes a resting order by id.
func (b *RestingBook) Cancel(orderID string) (*Order, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	o, ok := b.byID[orderID]
	if !ok || (o.Status != OrderStatusNew && o.Status != OrderStatusPartiallyFilled) {
		return nil, false
	}
	b.removeFromLevels(o)
	o.Status = OrderStatusCanceled
	now := b.nowFn().UTC()
	o.LastUpdatedAt = now
	return o, true
}

func (b *RestingBook) removeFromLevels(o *Order) {
	pk := priceKey(o.Price)
	if o.Side == SideBuy {
		b.bidLevels[pk] = removeOrdPtrFromSlice(b.bidLevels[pk], o)
		if len(b.bidLevels[pk]) == 0 {
			delete(b.bidLevels, pk)
		}
	} else {
		b.askLevels[pk] = removeOrdPtrFromSlice(b.askLevels[pk], o)
		if len(b.askLevels[pk]) == 0 {
			delete(b.askLevels, pk)
		}
	}
}

func removeOrdPtrFromSlice(s []*Order, x *Order) []*Order {
	out := s[:0]
	for _, p := range s {
		if p != x {
			out = append(out, p)
		}
	}
	return out
}

func collectBidsSortedOrd(levels map[string][]*Order) []*Order {
	var all []*Order
	for _, q := range levels {
		all = append(all, q...)
	}
	sort.Slice(all, func(i, j int) bool {
		pi, pj := all[i].Price, all[j].Price
		if !pi.Equal(pj) {
			return pi.GreaterThan(pj)
		}
		return all[i].CreatedAt.Before(all[j].CreatedAt)
	})
	return all
}

func collectAsksSortedOrd(levels map[string][]*Order) []*Order {
	var all []*Order
	for _, q := range levels {
		all = append(all, q...)
	}
	sort.Slice(all, func(i, j int) bool {
		pi, pj := all[i].Price, all[j].Price
		if !pi.Equal(pj) {
			return pi.LessThan(pj)
		}
		return all[i].CreatedAt.Before(all[j].CreatedAt)
	})
	return all
}

// OnDepth matches resting orders against public depth (maker).
func (b *RestingBook) OnDepth(depth *MarketDepth) []MatchEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	working := depth.Clone()
	var events []MatchEvent

	for _, o := range collectBidsSortedOrd(b.bidLevels) {
		if o.Status != OrderStatusNew && o.Status != OrderStatusPartiallyFilled {
			continue
		}
		if o.QtyRemaining.Sign() <= 0 {
			continue
		}
		ba, _, ok := working.BestAsk()
		if !ok || ba.GreaterThan(o.Price) {
			continue
		}
		fills, _, _ := SimulateLimitBuy(working, o.Price, o.QtyRemaining)
		if len(fills) == 0 {
			continue
		}
		working.ConsumeAskFills(fills)
		events = append(events, MatchEvent{Order: o, Fills: fills})
		b.applyFillsUnlocked(o, fills)
	}

	for _, o := range collectAsksSortedOrd(b.askLevels) {
		if o.Status != OrderStatusNew && o.Status != OrderStatusPartiallyFilled {
			continue
		}
		if o.QtyRemaining.Sign() <= 0 {
			continue
		}
		bb, _, ok := working.BestBid()
		if !ok || bb.LessThan(o.Price) {
			continue
		}
		fills, _, _ := SimulateLimitSell(working, o.Price, o.QtyRemaining)
		if len(fills) == 0 {
			continue
		}
		working.ConsumeBidFills(fills)
		events = append(events, MatchEvent{Order: o, Fills: fills})
		b.applyFillsUnlocked(o, fills)
	}

	return events
}

func (b *RestingBook) applyFillsUnlocked(o *Order, fills []Fill) {
	var filled decimal.Decimal
	var notional decimal.Decimal
	for _, f := range fills {
		filled = filled.Add(f.Size)
		notional = notional.Add(f.Price.Mul(f.Size))
	}
	oldFilled := o.QtyFilled
	o.QtyFilled = o.QtyFilled.Add(filled)
	o.QtyRemaining = o.QtyRemaining.Sub(filled)
	if oldFilled.IsZero() {
		o.AvgFillPrice = notional.Div(filled)
	} else {
		o.AvgFillPrice = o.AvgFillPrice.Mul(oldFilled).Add(notional).Div(o.QtyFilled)
	}
	now := b.nowFn().UTC()
	o.LastUpdatedAt = now
	if o.QtyRemaining.IsZero() {
		o.Status = OrderStatusFilled
		b.removeFromLevels(o)
	} else {
		o.Status = OrderStatusPartiallyFilled
	}
}

// GetOrder returns a snapshot copy if exists.
func (b *RestingBook) GetOrder(id string) (Order, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	o, ok := b.byID[id]
	if !ok {
		return Order{}, false
	}
	cp := *o
	return cp, true
}

// ListOpenOrders returns resting orders (copy).
func (b *RestingBook) ListOpenOrders() []Order {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Order, 0, len(b.byID))
	for _, o := range b.byID {
		if o.Status == OrderStatusNew || o.Status == OrderStatusPartiallyFilled {
			cp := *o
			out = append(out, cp)
		}
	}
	return out
}
