package simulate

import (
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// MatchEvent describes a resting order matched against public depth (maker).
type MatchEvent struct {
	Order *SimOrder
	Fills []Fill
}

// SimBook holds user resting limit orders for one symbol (not shown on public L2).
type SimBook struct {
	mu     sync.Mutex
	symbol Symbol
	nowFn  func() time.Time

	bidLevels map[string][]*SimOrder
	askLevels map[string][]*SimOrder
	byID      map[string]*SimOrder
}

// NewSimBook creates an empty book for symbol.
func NewSimBook(sym Symbol, nowFn func() time.Time) *SimBook {
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	return &SimBook{
		symbol:    sym,
		nowFn:     nowFn,
		bidLevels: make(map[string][]*SimOrder),
		askLevels: make(map[string][]*SimOrder),
		byID:      make(map[string]*SimOrder),
	}
}

func priceKey(p decimal.Decimal) string {
	return p.String()
}

// PutOrderRecord stores any terminal or active order for GetOrder (e.g. fully filled market orders).
func (b *SimBook) PutOrderRecord(o *SimOrder) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.byID[o.ID] = o
}

// AddResting inserts a limit order (must have Remaining > 0).
func (b *SimBook) AddResting(o *SimOrder) {
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

// Cancel removes a resting order by id. Returns false if not found or not cancelable.
func (b *SimBook) Cancel(orderID string) (*SimOrder, bool) {
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

// removeFromLevels removes a resting order from price queues but keeps it in byID for lookup.
func (b *SimBook) removeFromLevels(o *SimOrder) {
	pk := priceKey(o.Price)
	if o.Side == SideBuy {
		b.bidLevels[pk] = removePtrFromSlice(b.bidLevels[pk], o)
		if len(b.bidLevels[pk]) == 0 {
			delete(b.bidLevels, pk)
		}
	} else {
		b.askLevels[pk] = removePtrFromSlice(b.askLevels[pk], o)
		if len(b.askLevels[pk]) == 0 {
			delete(b.askLevels, pk)
		}
	}
}

func removePtrFromSlice(s []*SimOrder, x *SimOrder) []*SimOrder {
	out := s[:0]
	for _, p := range s {
		if p != x {
			out = append(out, p)
		}
	}
	return out
}

func collectBidsSorted(levels map[string][]*SimOrder) []*SimOrder {
	var all []*SimOrder
	for _, q := range levels {
		all = append(all, q...)
	}
	sort.Slice(all, func(i, j int) bool {
		pi := all[i].Price
		pj := all[j].Price
		if !pi.Equal(pj) {
			return pi.GreaterThan(pj)
		}
		return all[i].CreatedAt.Before(all[j].CreatedAt)
	})
	return all
}

func collectAsksSorted(levels map[string][]*SimOrder) []*SimOrder {
	var all []*SimOrder
	for _, q := range levels {
		all = append(all, q...)
	}
	sort.Slice(all, func(i, j int) bool {
		pi := all[i].Price
		pj := all[j].Price
		if !pi.Equal(pj) {
			return pi.LessThan(pj)
		}
		return all[i].CreatedAt.Before(all[j].CreatedAt)
	})
	return all
}

// OnDepth matches resting orders against a snapshot of public depth (read-only on depth arg;
// uses an internal working clone that is mutated across sequential matches).
func (b *SimBook) OnDepth(depth *MarketDepth) []MatchEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	working := depth.Clone()
	var events []MatchEvent

	for _, o := range collectBidsSorted(b.bidLevels) {
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
		var filled decimal.Decimal
		for _, f := range fills {
			filled = filled.Add(f.Size)
		}
		working.ConsumeAskFills(fills)
		events = append(events, MatchEvent{Order: o, Fills: fills})
		b.applyFillsUnlocked(o, fills)
	}

	// reset working from depth again for asks (bids may have consumed — public depth unchanged in reality;
	// for maker matching we must re-clone from original depth for ask side vs bid side interaction).
	// Correct approach: single working clone from start, process bids then asks sequentially on same working.
	// We already mutated working for bids — asks (sells) hit bids, so continue same working.
	for _, o := range collectAsksSorted(b.askLevels) {
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

func (b *SimBook) applyFillsUnlocked(o *SimOrder, fills []Fill) {
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
func (b *SimBook) GetOrder(id string) (SimOrder, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	o, ok := b.byID[id]
	if !ok {
		return SimOrder{}, false
	}
	cp := *o
	return cp, true
}

// ListOpenOrders returns resting orders (copy).
func (b *SimBook) ListOpenOrders() []SimOrder {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]SimOrder, 0, len(b.byID))
	for _, o := range b.byID {
		if o.Status == OrderStatusNew || o.Status == OrderStatusPartiallyFilled {
			cp := *o
			out = append(out, cp)
		}
	}
	return out
}
