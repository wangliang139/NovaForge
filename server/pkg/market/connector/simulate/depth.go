package simulate

import (
	"sync"

	rbtx "github.com/emirpasic/gods/examples/redblacktreeextended"
	rbt "github.com/emirpasic/gods/trees/redblacktree"
	"github.com/shopspring/decimal"
)

func priceComparator(a, b interface{}) int {
	return a.(decimal.Decimal).Cmp(b.(decimal.Decimal))
}

func newPriceTree() *rbtx.RedBlackTreeExtended {
	return &rbtx.RedBlackTreeExtended{Tree: rbt.NewWith(priceComparator)}
}

// MarketDepth is an aggregated L2 view (price -> size). It is safe for concurrent reads;
// writes are serialized by an internal mutex.
type MarketDepth struct {
	mu sync.RWMutex

	bids *rbtx.RedBlackTreeExtended
	asks *rbtx.RedBlackTreeExtended

	exchange Exchange
	symbol   Symbol

	lastSeqId int64
	init      bool
}

// NewMarketDepth creates an empty depth (ApplySnapshot must be called before ApplyDelta).
func NewMarketDepth() *MarketDepth {
	return &MarketDepth{
		bids: newPriceTree(),
		asks: newPriceTree(),
	}
}

// LastSeqID returns the sequence id of the last successfully applied snapshot or delta.
func (d *MarketDepth) LastSeqID() int64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastSeqId
}

// Exchange returns the venue from the last snapshot (if set).
func (d *MarketDepth) Exchange() Exchange {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.exchange
}

// Symbol returns the market from the last snapshot (if set).
func (d *MarketDepth) Symbol() Symbol {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.symbol
}

// ApplySnapshot fully replaces both sides of the book and sets lastSeqId to ob.SeqId.
// JSON nil slice for bids/asks clears that side (no levels). Non-nil replaces that side.
func (d *MarketDepth) ApplySnapshot(ob *OrderBook) error {
	if ob == nil {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.bids = newPriceTree()
	d.asks = newPriceTree()

	if ob.Bids != nil {
		for _, lvl := range ob.Bids {
			if err := d.upsertBidLocked(lvl); err != nil {
				return err
			}
		}
	}
	if ob.Asks != nil {
		for _, lvl := range ob.Asks {
			if err := d.upsertAskLocked(lvl); err != nil {
				return err
			}
		}
	}

	d.exchange = ob.Exchange
	d.symbol = ob.Symbol
	d.lastSeqId = ob.SeqId
	d.init = true
	return nil
}

// ApplyDelta patches only sides present in JSON: in Go, a missing field unmarshals to nil slice,
// meaning "no changes" for that side. An empty non-nil slice applies zero patches.
// Size == 0 removes the price level.
func (d *MarketDepth) ApplyDelta(ob *OrderBook) error {
	if ob == nil {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.init {
		return ErrNotInitialized
	}
	if ob.PrevSeqId != d.lastSeqId {
		return ErrSeqGap
	}

	if ob.Bids != nil {
		for _, lvl := range ob.Bids {
			if err := d.patchBidLocked(lvl); err != nil {
				return err
			}
		}
	}
	if ob.Asks != nil {
		for _, lvl := range ob.Asks {
			if err := d.patchAskLocked(lvl); err != nil {
				return err
			}
		}
	}

	d.lastSeqId = ob.SeqId
	return nil
}

func (d *MarketDepth) upsertBidLocked(lvl OrderBookLevel) error {
	if lvl.Price.Sign() <= 0 {
		return nil
	}
	if lvl.Size.Sign() <= 0 {
		return nil
	}
	d.bids.Put(lvl.Price, lvl.Size)
	return nil
}

func (d *MarketDepth) upsertAskLocked(lvl OrderBookLevel) error {
	if lvl.Price.Sign() <= 0 {
		return nil
	}
	if lvl.Size.Sign() <= 0 {
		return nil
	}
	d.asks.Put(lvl.Price, lvl.Size)
	return nil
}

func (d *MarketDepth) patchBidLocked(lvl OrderBookLevel) error {
	if lvl.Price.Sign() <= 0 {
		return nil
	}
	if lvl.Size.IsZero() {
		d.bids.Remove(lvl.Price)
		return nil
	}
	if lvl.Size.Sign() < 0 {
		return nil
	}
	d.bids.Put(lvl.Price, lvl.Size)
	return nil
}

func (d *MarketDepth) patchAskLocked(lvl OrderBookLevel) error {
	if lvl.Price.Sign() <= 0 {
		return nil
	}
	if lvl.Size.IsZero() {
		d.asks.Remove(lvl.Price)
		return nil
	}
	if lvl.Size.Sign() < 0 {
		return nil
	}
	d.asks.Put(lvl.Price, lvl.Size)
	return nil
}

// BestBid returns the best bid price and size, or false if none.
func (d *MarketDepth) BestBid() (price decimal.Decimal, size decimal.Decimal, ok bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.bids.Empty() {
		return decimal.Zero, decimal.Zero, false
	}
	it := d.bids.Iterator()
	if !it.Last() {
		return decimal.Zero, decimal.Zero, false
	}
	return it.Key().(decimal.Decimal), it.Value().(decimal.Decimal), true
}

// BestAsk returns the best ask price and size, or false if none.
func (d *MarketDepth) BestAsk() (price decimal.Decimal, size decimal.Decimal, ok bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.asks.Empty() {
		return decimal.Zero, decimal.Zero, false
	}
	it := d.asks.Iterator()
	if !it.First() {
		return decimal.Zero, decimal.Zero, false
	}
	return it.Key().(decimal.Decimal), it.Value().(decimal.Decimal), true
}

// WalkBids visits bid levels from best to worst (descending price).
func (d *MarketDepth) WalkBids(fn func(price decimal.Decimal, size decimal.Decimal) bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	it := d.bids.Iterator()
	if !it.Last() {
		return
	}
	for {
		if !fn(it.Key().(decimal.Decimal), it.Value().(decimal.Decimal)) {
			return
		}
		if !it.Prev() {
			return
		}
	}
}

// WalkAsks visits ask levels from best to worst (ascending price).
func (d *MarketDepth) WalkAsks(fn func(price decimal.Decimal, size decimal.Decimal) bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	it := d.asks.Iterator()
	if !it.First() {
		return
	}
	for {
		if !fn(it.Key().(decimal.Decimal), it.Value().(decimal.Decimal)) {
			return
		}
		if !it.Next() {
			return
		}
	}
}

// Clone returns a shallow copy of the top-of-book trees for use without holding the lock
// across simulations. Snapshot is consistent at one instant.
func (d *MarketDepth) Clone() *MarketDepth {
	d.mu.RLock()
	defer d.mu.RUnlock()

	nb := newPriceTree()
	na := newPriceTree()
	it := d.bids.Iterator()
	if it.Last() {
		for {
			nb.Put(it.Key(), it.Value())
			if !it.Prev() {
				break
			}
		}
	}
	it = d.asks.Iterator()
	if it.First() {
		for {
			na.Put(it.Key(), it.Value())
			if !it.Next() {
				break
			}
		}
	}

	return &MarketDepth{
		bids:      nb,
		asks:      na,
		exchange:  d.exchange,
		symbol:    d.symbol,
		lastSeqId: d.lastSeqId,
		init:      d.init,
	}
}

// ConsumeAskFills subtracts executed base size from ask levels (mutates d). Intended for
// working copies used when sequencing multiple shadow matches.
func (d *MarketDepth) ConsumeAskFills(fills []Fill) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, f := range fills {
		v, found := d.asks.Get(f.Price)
		if !found {
			continue
		}
		cur := v.(decimal.Decimal).Sub(f.Size)
		if cur.Sign() <= 0 {
			d.asks.Remove(f.Price)
		} else {
			d.asks.Put(f.Price, cur)
		}
	}
}

// ConsumeBidFills subtracts executed base size from bid levels (mutates d).
func (d *MarketDepth) ConsumeBidFills(fills []Fill) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, f := range fills {
		v, found := d.bids.Get(f.Price)
		if !found {
			continue
		}
		cur := v.(decimal.Decimal).Sub(f.Size)
		if cur.Sign() <= 0 {
			d.bids.Remove(f.Price)
		} else {
			d.bids.Put(f.Price, cur)
		}
	}
}
