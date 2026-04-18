package simulate

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type accountSymbolKey struct {
	AccountID string
	Symbol    Symbol
}

type accountLevKey struct {
	AccountID string
	Symbol    Symbol
}

// Engine is the single-writer matching core (hold mu for all mutations).
type Engine struct {
	mu sync.Mutex

	ins    map[Symbol]*Instrument
	depths map[Symbol]*MarketDepth
	books  map[accountSymbolKey]*RestingBook
	ledger *Ledger
	nowFn  func() time.Time

	// per-account default position mode for new perp slots
	accountModes map[string]PositionMode
	leverages    map[accountLevKey]int
}

// NewEngine creates an engine with an empty ledger.
func NewEngine() *Engine {
	nowFn := func() time.Time { return time.Now().UTC() }
	return &Engine{
		ins:          make(map[Symbol]*Instrument),
		depths:       make(map[Symbol]*MarketDepth),
		books:        make(map[accountSymbolKey]*RestingBook),
		ledger:       NewLedger(),
		nowFn:        nowFn,
		accountModes: make(map[string]PositionMode),
		leverages:    make(map[accountLevKey]int),
	}
}

// WithNowFn sets clock source (tests / replay).
func (e *Engine) WithNowFn(nowFn func() time.Time) *Engine {
	if nowFn != nil {
		e.nowFn = nowFn
	}
	return e
}

func (e *Engine) now() time.Time {
	if e.nowFn == nil {
		return time.Now().UTC()
	}
	return e.nowFn().UTC()
}

// Ledger exposes the account ledger (read-only patterns should copy under Engine lock).
func (e *Engine) Ledger() *Ledger { return e.ledger }

// SetAccountPositionMode sets default mode for new perp slots (switching requires flat book via Ledger.SetPerpMode).
func (e *Engine) SetAccountPositionMode(accountID string, mode PositionMode) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.accountModes[accountID] = mode
}

// AccountPositionMode returns configured mode (default hedge / 双向).
func (e *Engine) AccountPositionMode(accountID string) PositionMode {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.modeForUnlock(accountID)
}

func (e *Engine) modeFor(accountID string) PositionMode {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.modeForUnlock(accountID)
}

func (e *Engine) modeForUnlock(accountID string) PositionMode {
	if m, ok := e.accountModes[accountID]; ok {
		return m
	}
	return PositionModeHedge
}

// RegisterInstrument registers symbol metadata.
func (e *Engine) RegisterInstrument(ins *Instrument) error {
	if ins == nil {
		return ErrUnknownSymbol
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := *ins
	e.ins[ins.Symbol] = &cp
	return nil
}

// BindDepth attaches shared L2 for a symbol.
func (e *Engine) BindDepth(sym Symbol, d *MarketDepth) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.ins[sym]; !ok {
		return ErrUnknownSymbol
	}
	e.depths[sym] = d
	return nil
}

// InitBalances seeds wallet balances.
func (e *Engine) InitBalances(accountID string, bals map[ctypes.WalletType]map[Asset]decimal.Decimal) {
	if len(bals) == 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for wt, by := range bals {
		for a, v := range by {
			e.ledger.SetBalance(accountID, wt, a, v)
		}
	}
}

// SeedLedgerOneWayNet seeds one-way net perp position under the engine lock.
func (e *Engine) SeedLedgerOneWayNet(accountID string, sym Symbol, pos Position) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ledger.SeedOneWayNet(accountID, sym, pos)
}

// SeedLedgerHedgeLeg seeds one hedge leg under the engine lock.
func (e *Engine) SeedLedgerHedgeLeg(accountID string, sym Symbol, side ctypes.PositionSide, leg PerpLeg) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ledger.MergeSeedHedgeLeg(accountID, sym, side, leg)
}

// SeedOpenOrder inserts a resting limit order (bootstrap / DB sync). Does not match against depth.
func (e *Engine) SeedOpenOrder(accountID string, po Order) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	ins, ok := e.ins[po.Symbol]
	if !ok {
		return ErrUnknownSymbol
	}
	if po.OrderType != OrderTypeLimit {
		return fmt.Errorf("simulate: seed open order requires limit type")
	}
	book := e.getBook(accountID, po.Symbol)
	if po.ID != "" {
		if ex, ok2 := book.GetOrder(po.ID); ok2 {
			if ex.Status == OrderStatusNew || ex.Status == OrderStatusPartiallyFilled {
				return nil
			}
		}
	}
	rem := FloorToStep(po.QtyRemaining, ins.QtyStep)
	if rem.Sign() <= 0 {
		return fmt.Errorf("simulate: seed requires positive remaining qty")
	}
	price := FloorToTick(po.Price, ins.PriceTick)
	if err := ins.ValidateOrderParams(price, rem, true); err != nil {
		return err
	}
	now := e.now().UTC()
	cp := po
	cp.AccountID = accountID
	cp.Symbol = po.Symbol
	cp.OrderType = OrderTypeLimit
	cp.Price = price
	cp.QtyRemaining = rem
	if cp.QtyOriginal.Sign() <= 0 {
		cp.QtyOriginal = rem.Add(cp.QtyFilled)
	} else {
		cp.QtyOriginal = FloorToStep(cp.QtyOriginal, ins.QtyStep)
	}
	if cp.QtyFilled.Sign() < 0 {
		cp.QtyFilled = decimal.Zero
	}
	if cp.Status != OrderStatusNew && cp.Status != OrderStatusPartiallyFilled {
		cp.Status = OrderStatusNew
	}
	if cp.ID == "" {
		cp.ID = e.genOrderID(accountID)
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.LastUpdatedAt = now
	book.AddResting(&cp)
	return nil
}

func (e *Engine) getBook(accountID string, sym Symbol) *RestingBook {
	key := accountSymbolKey{AccountID: accountID, Symbol: sym}
	b := e.books[key]
	if b == nil {
		b = NewRestingBook(sym, e.now)
		e.books[key] = b
	}
	return b
}

func (e *Engine) genOrderID(accountID string) string {
	return GenerateCompactID(accountID)
}

func midPrice(d *MarketDepth) decimal.Decimal {
	bb, _, okB := d.BestBid()
	ba, _, okA := d.BestAsk()
	if okB && okA {
		return bb.Add(ba).Div(decimal.NewFromInt(2))
	}
	if okB {
		return bb
	}
	if okA {
		return ba
	}
	return decimal.Zero
}

func (e *Engine) validateOneWayPerp(req *PlaceOrderRequest, ins *Instrument, pos Position) error {
	if ins.Kind != KindPerp {
		return nil
	}
	switch req.Intent {
	case IntentOpen:
		if req.Leverage < 1 || (ins.LeverageMax > 0 && req.Leverage > ins.LeverageMax) {
			return ErrLeverage
		}
		if req.ReduceOnly {
			return ErrInvalidIntent
		}
		if req.Side == SideBuy && pos.Qty.Sign() < 0 {
			return ErrInvalidIntent
		}
		if req.Side == SideSell && pos.Qty.Sign() > 0 {
			return ErrInvalidIntent
		}
	case IntentClose:
		if !req.ReduceOnly {
			return ErrInvalidIntent
		}
		if req.Side == SideSell {
			if pos.Qty.Sign() <= 0 {
				return ErrInvalidIntent
			}
			if req.Qty.GreaterThan(pos.Qty) {
				return ErrReduceOnly
			}
		} else if req.Side == SideBuy {
			if pos.Qty.Sign() >= 0 {
				return ErrInvalidIntent
			}
			if req.Qty.GreaterThan(pos.Qty.Abs()) {
				return ErrReduceOnly
			}
		}
	}
	return nil
}

func (e *Engine) validateHedgePerp(req *PlaceOrderRequest, ins *Instrument, slot *PerpSlot) error {
	if !req.PosSide.Valid() {
		return ErrInvalidIntent
	}
	isBuy := req.Side == SideBuy
	var legQty decimal.Decimal
	switch req.PosSide {
	case ctypes.PositionSideLong:
		legQty = slot.Long.Qty
	case ctypes.PositionSideShort:
		legQty = slot.Short.Qty
	}
	if err := ValidateHedgeOrder(req.PosSide, isBuy, req.ReduceOnly, legQty); err != nil {
		return err
	}
	opening := HedgeOpen(req.PosSide, isBuy)
	if opening {
		if req.Leverage < 1 || (ins.LeverageMax > 0 && req.Leverage > ins.LeverageMax) {
			return ErrLeverage
		}
	} else {
		if req.Qty.GreaterThan(legQty) {
			return ErrReduceOnly
		}
	}
	return nil
}

func (e *Engine) rejectOrder(accountID string, req *PlaceOrderRequest, orderID, reason string, ts time.Time) *PlaceOrderResult {
	book := e.getBook(accountID, req.Symbol)
	now := ts.UTC()
	qty := req.Qty
	if ins, ok := e.ins[req.Symbol]; ok {
		qty = FloorToStep(req.Qty, ins.QtyStep)
	}
	o := Order{
		ID:            orderID,
		AccountID:     accountID,
		ClientOrderID: req.ClientOrderID,
		Symbol:        req.Symbol,
		OrderType:     req.OrderType,
		Side:          req.Side,
		Intent:        req.Intent,
		ReduceOnly:    req.ReduceOnly,
		Leverage:      req.Leverage,
		PosSide:       req.PosSide,
		Price:         req.Price,
		QtyOriginal:   qty,
		QtyRemaining:  qty,
		Status:        OrderStatusRejected,
		RejectReason:  reason,
		CreatedAt:     now,
		LastUpdatedAt: now,
	}
	book.PutOrderRecord(&o)
	return &PlaceOrderResult{Order: o, Fills: nil, FeePaid: decimal.Zero}
}

// PlaceOrder validates, matches against public depth, updates ledger, rests remainder.
func (e *Engine) PlaceOrder(_ context.Context, req PlaceOrderRequest) (*PlaceOrderResult, error) {
	ts := e.now()
	e.mu.Lock()
	defer e.mu.Unlock()

	accountID := req.AccountID
	orderID := req.OrderID
	if orderID == "" {
		orderID = e.genOrderID(accountID)
	}

	ins, ok := e.ins[req.Symbol]
	if !ok {
		return e.rejectOrder(accountID, &req, orderID, ErrUnknownSymbol.Error(), ts), nil
	}
	d, ok := e.depths[req.Symbol]
	if !ok || d == nil {
		return e.rejectOrder(accountID, &req, orderID, ErrNotInitialized.Error(), ts), nil
	}
	book := e.getBook(accountID, req.Symbol)

	mode := e.modeForUnlock(accountID)
	slot := e.ledger.EnsurePerpSlot(accountID, req.Symbol, mode)
	if slot.Mode != mode {
		// slot existed with different mode — should not happen if SetPerpMode used
		slot.Mode = mode
	}

	if ins.Kind == KindPerp {
		if mode == PositionModeHedge {
			if err := e.validateHedgePerp(&req, ins, slot); err != nil {
				return e.rejectOrder(accountID, &req, orderID, err.Error(), ts), nil
			}
		} else {
			pos := slot.Net
			if err := e.validateOneWayPerp(&req, ins, pos); err != nil {
				return e.rejectOrder(accountID, &req, orderID, err.Error(), ts), nil
			}
		}
	}

	qty := FloorToStep(req.Qty, ins.QtyStep)
	price := req.Price
	if req.OrderType == OrderTypeLimit {
		price = FloorToTick(req.Price, ins.PriceTick)
	}
	if err := ins.ValidateOrderParams(price, qty, req.OrderType == OrderTypeLimit); err != nil {
		return e.rejectOrder(accountID, &req, orderID, err.Error(), ts), nil
	}

	now := ts.UTC()
	order := &Order{
		ID:            orderID,
		AccountID:     accountID,
		ClientOrderID: req.ClientOrderID,
		Symbol:        req.Symbol,
		OrderType:     req.OrderType,
		Side:          req.Side,
		Intent:        req.Intent,
		ReduceOnly:    req.ReduceOnly,
		Leverage:      req.Leverage,
		PosSide:       req.PosSide,
		Price:         price,
		QtyOriginal:   qty,
		QtyRemaining:  qty,
		Status:        OrderStatusNew,
		CreatedAt:     now,
		LastUpdatedAt: now,
	}

	var allFills []Fill
	var feeSum decimal.Decimal
	takerBps := ins.TakerFeeBps
	lev := req.Leverage
	if lev <= 0 {
		lev = 1
	}

	switch ins.Kind {
	case KindSpot:
		switch req.OrderType {
		case OrderTypeMarket:
			ref := midPrice(d)
			if err := ins.ValidateMarketQty(qty, ref); err != nil {
				return e.rejectPlacedOrder(order, book, err.Error(), now), nil
			}
			var fills []Fill
			var notional decimal.Decimal
			if req.Side == SideBuy {
				fills, _, notional = SimulateMarketBuy(d, qty)
			} else {
				fills, _, notional = SimulateMarketSell(d, qty)
			}
			if len(fills) == 0 {
				order.Status = OrderStatusRejected
				order.RejectReason = "no liquidity"
				book.PutOrderRecord(order)
				return &PlaceOrderResult{Order: *order, Fills: nil, FeePaid: decimal.Zero}, nil
			}
			filledQty := totalFilledQty(fills)
			fee := FeeNotional(notional, takerBps)
			if req.Side == SideBuy {
				if err := e.ledger.ApplySpotBuy(accountID, ins, fills, fee); err != nil {
					return e.rejectPlacedOrder(order, book, err.Error(), now), nil
				}
			} else {
				if err := e.ledger.ApplySpotSell(accountID, ins, fills, fee); err != nil {
					return e.rejectPlacedOrder(order, book, err.Error(), now), nil
				}
			}
			allFills = fills
			feeSum = fee
			fillOrderImmediate(order, fills, notional, filledQty, now)

		case OrderTypeLimit:
			if err := ins.ValidateOrderParams(price, qty, true); err != nil {
				return e.rejectPlacedOrder(order, book, err.Error(), now), nil
			}
			var fills []Fill
			var notional decimal.Decimal
			if req.Side == SideBuy {
				ba, _, okA := d.BestAsk()
				if okA && !ba.GreaterThan(price) {
					fills, order.QtyRemaining, notional = SimulateLimitBuy(d, price, qty)
				}
			} else {
				bb, _, okB := d.BestBid()
				if okB && !bb.LessThan(price) {
					fills, order.QtyRemaining, notional = SimulateLimitSell(d, price, qty)
				}
			}
			if len(fills) > 0 {
				filledQty := totalFilledQty(fills)
				fee := FeeNotional(notional, takerBps)
				if req.Side == SideBuy {
					if err := e.ledger.ApplySpotBuy(accountID, ins, fills, fee); err != nil {
						return e.rejectPlacedOrder(order, book, err.Error(), now), nil
					}
				} else {
					if err := e.ledger.ApplySpotSell(accountID, ins, fills, fee); err != nil {
						return e.rejectPlacedOrder(order, book, err.Error(), now), nil
					}
				}
				allFills = append(allFills, fills...)
				feeSum = feeSum.Add(fee)
				partialFillOrder(order, fills, notional, filledQty, now)
			}
			if order.QtyRemaining.Sign() > 0 {
				book.AddResting(order)
			} else if len(fills) > 0 {
				order.Status = OrderStatusFilled
			} else {
				book.AddResting(order)
			}
		}

	case KindPerp:
		switch req.OrderType {
		case OrderTypeMarket:
			ref := midPrice(d)
			if err := ins.ValidateMarketQty(qty, ref); err != nil {
				return e.rejectPlacedOrder(order, book, err.Error(), now), nil
			}
			var fills []Fill
			var notional decimal.Decimal
			if req.Side == SideBuy {
				fills, _, notional = SimulateMarketBuy(d, qty)
			} else {
				fills, _, notional = SimulateMarketSell(d, qty)
			}
			if len(fills) == 0 {
				order.Status = OrderStatusRejected
				order.RejectReason = "no liquidity"
				book.PutOrderRecord(order)
				return &PlaceOrderResult{Order: *order, Fills: nil, FeePaid: decimal.Zero}, nil
			}
			filledQty := totalFilledQty(fills)
			fee := FeeNotional(notional, takerBps)
			if err := e.applyPerpFills(accountID, ins, &req, fills, fee, lev, slot); err != nil {
				return e.rejectPlacedOrder(order, book, err.Error(), now), nil
			}
			allFills = fills
			feeSum = fee
			fillOrderImmediate(order, fills, notional, filledQty, now)

		case OrderTypeLimit:
			if err := ins.ValidateOrderParams(price, qty, true); err != nil {
				return e.rejectPlacedOrder(order, book, err.Error(), now), nil
			}
			var fills []Fill
			var notional decimal.Decimal
			if req.Side == SideBuy {
				ba, _, okA := d.BestAsk()
				if okA && !ba.GreaterThan(price) {
					fills, order.QtyRemaining, notional = SimulateLimitBuy(d, price, qty)
				}
			} else {
				bb, _, okB := d.BestBid()
				if okB && !bb.LessThan(price) {
					fills, order.QtyRemaining, notional = SimulateLimitSell(d, price, qty)
				}
			}
			if len(fills) > 0 {
				filledQty := totalFilledQty(fills)
				fee := FeeNotional(notional, takerBps)
				if err := e.applyPerpFills(accountID, ins, &req, fills, fee, lev, slot); err != nil {
					return e.rejectPlacedOrder(order, book, err.Error(), now), nil
				}
				allFills = append(allFills, fills...)
				feeSum = feeSum.Add(fee)
				partialFillOrder(order, fills, notional, filledQty, now)
			}
			if order.QtyRemaining.Sign() > 0 {
				book.AddResting(order)
			} else if len(fills) > 0 {
				order.Status = OrderStatusFilled
			} else {
				book.AddResting(order)
			}
		}
	}

	if order.Status == OrderStatusFilled || order.Status == OrderStatusRejected {
		book.PutOrderRecord(order)
	}
	cp := *order
	return &PlaceOrderResult{Order: cp, Fills: allFills, FeePaid: feeSum}, nil
}

func (e *Engine) applyPerpFills(accountID string, ins *Instrument, req *PlaceOrderRequest, fills []Fill, fee decimal.Decimal, lev int32, slot *PerpSlot) error {
	if slot.Mode == PositionModeHedge {
		isBuy := req.Side == SideBuy
		opening := HedgeOpen(req.PosSide, isBuy)
		switch req.PosSide {
		case ctypes.PositionSideLong:
			if opening {
				return e.ledger.ApplyHedgeOpenLong(accountID, ins, fills, fee, lev, slot)
			}
			return e.ledger.ApplyHedgeCloseLong(accountID, ins, fills, fee, slot)
		case ctypes.PositionSideShort:
			if opening {
				return e.ledger.ApplyHedgeOpenShort(accountID, ins, fills, fee, lev, slot)
			}
			return e.ledger.ApplyHedgeCloseShort(accountID, ins, fills, fee, slot)
		default:
			return ErrInvalidIntent
		}
	}
	switch req.Intent {
	case IntentOpen:
		if req.Side == SideBuy {
			return e.ledger.ApplyPerpOpenLong(accountID, req.Symbol, ins, fills, fee, lev, slot)
		}
		return e.ledger.ApplyPerpOpenShort(accountID, req.Symbol, ins, fills, fee, lev, slot)
	case IntentClose:
		if req.Side == SideSell {
			return e.ledger.ApplyPerpCloseLong(accountID, req.Symbol, ins, fills, fee, slot)
		}
		return e.ledger.ApplyPerpCloseShort(accountID, req.Symbol, ins, fills, fee, slot)
	default:
		return ErrInvalidIntent
	}
}

func (e *Engine) applyMakerPerpFills(accountID string, ins *Instrument, o *Order, fills []Fill, fee decimal.Decimal, slot *PerpSlot) error {
	req := PlaceOrderRequest{
		AccountID:  accountID,
		Symbol:     o.Symbol,
		Side:       o.Side,
		Intent:     o.Intent,
		ReduceOnly: o.ReduceOnly,
		Leverage:   o.Leverage,
		PosSide:    o.PosSide,
	}
	return e.applyPerpFills(accountID, ins, &req, fills, fee, o.Leverage, slot)
}

func totalFilledQty(fills []Fill) decimal.Decimal {
	var q decimal.Decimal
	for _, f := range fills {
		q = q.Add(f.Size)
	}
	return q
}

func fillOrderImmediate(o *Order, fills []Fill, notional, filledQty decimal.Decimal, now time.Time) {
	o.QtyFilled = filledQty
	o.QtyRemaining = decimal.Zero
	o.AvgFillPrice = AveragePrice(notional, filledQty)
	o.Status = OrderStatusFilled
	o.LastUpdatedAt = now.UTC()
}

func partialFillOrder(o *Order, fills []Fill, notional, filledQty decimal.Decimal, now time.Time) {
	o.QtyFilled = filledQty
	o.QtyRemaining = o.QtyRemaining.Sub(filledQty)
	if o.QtyRemaining.Sign() < 0 {
		o.QtyRemaining = decimal.Zero
	}
	o.AvgFillPrice = AveragePrice(notional, filledQty)
	o.Status = OrderStatusPartiallyFilled
	if o.QtyRemaining.IsZero() {
		o.Status = OrderStatusFilled
	}
	o.LastUpdatedAt = now.UTC()
}

func (e *Engine) rejectPlacedOrder(order *Order, book *RestingBook, reason string, now time.Time) *PlaceOrderResult {
	order.Status = OrderStatusRejected
	order.RejectReason = reason
	order.LastUpdatedAt = now.UTC()
	book.PutOrderRecord(order)
	return &PlaceOrderResult{Order: *order, Fills: nil, FeePaid: decimal.Zero}
}

// OnDepthUpdated matches resting orders after a depth commit.
func (e *Engine) OnDepthUpdated(sym Symbol) ([]MatchEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.onDepthUpdatedUnlocked(sym)
}

func (e *Engine) onDepthUpdatedUnlocked(sym Symbol) ([]MatchEvent, error) {
	d, ok := e.depths[sym]
	if !ok || d == nil {
		return nil, ErrNotInitialized
	}
	ins, ok := e.ins[sym]
	if !ok {
		return nil, ErrUnknownSymbol
	}
	var all []MatchEvent
	for key, book := range e.books {
		if key.Symbol != sym {
			continue
		}
		events := book.OnDepth(d)
		for _, ev := range events {
			var notional decimal.Decimal
			for _, f := range ev.Fills {
				notional = notional.Add(f.Price.Mul(f.Size))
			}
			fee := FeeNotional(notional, ins.MakerFeeBps)
			mode := e.modeForUnlock(ev.Order.AccountID)
			slot := e.ledger.EnsurePerpSlot(ev.Order.AccountID, sym, mode)
			switch ins.Kind {
			case KindSpot:
				if ev.Order.Side == SideBuy {
					if err := e.ledger.ApplySpotBuy(ev.Order.AccountID, ins, ev.Fills, fee); err != nil {
						return all, err
					}
				} else {
					if err := e.ledger.ApplySpotSell(ev.Order.AccountID, ins, ev.Fills, fee); err != nil {
						return all, err
					}
				}
			case KindPerp:
				if err := e.applyMakerPerpFills(ev.Order.AccountID, ins, ev.Order, ev.Fills, fee, slot); err != nil {
					return all, err
				}
			}
			all = append(all, ev)
		}
	}
	return all, nil
}

// ApplyDepthBook applies a public order book snapshot or delta and optionally runs maker matching.
func (e *Engine) ApplyDepthBook(cb *ctypes.OrderBook, matchResting bool) ([]MatchEvent, error) {
	if cb == nil || !cb.Symbol.IsValid() {
		return nil, nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	sym := Symbol(cb.Symbol.String())
	if _, ok := e.ins[sym]; !ok {
		return nil, ErrUnknownSymbol
	}
	d, ok := e.depths[sym]
	if !ok || d == nil {
		d = NewMarketDepth()
		e.depths[sym] = d
	}
	ob := orderBookFromTypes(cb)
	if ob.PrevSeqId > 0 && d.LastSeqID() > 0 {
		if err := d.ApplyDelta(lo.ToPtr(ob)); err != nil {
			_ = d.ApplySnapshot(lo.ToPtr(ob))
		}
	} else {
		_ = d.ApplySnapshot(lo.ToPtr(ob))
	}
	if !matchResting {
		return nil, nil
	}
	return e.onDepthUpdatedUnlocked(sym)
}

func orderBookFromTypes(book *ctypes.OrderBook) OrderBook {
	out := OrderBook{
		Symbol:    Symbol(book.Symbol.String()),
		Ts:        book.Ts,
		SeqId:     book.SeqId,
		PrevSeqId: book.PrevSeqId,
	}
	for _, bid := range book.Bids {
		out.Bids = append(out.Bids, OrderBookLevel{Price: bid.Price, Size: bid.Size})
	}
	for _, ask := range book.Asks {
		out.Asks = append(out.Asks, OrderBookLevel{Price: ask.Price, Size: ask.Size})
	}
	return out
}

// CancelOrder cancels a resting order.
func (e *Engine) CancelOrder(_ context.Context, accountID string, sym Symbol, orderID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.getBook(accountID, sym)
	_, ok := b.Cancel(orderID)
	if !ok {
		return ErrOrderNotFound
	}
	return nil
}

// GetOrder returns a copy if known.
func (e *Engine) GetOrder(accountID string, sym Symbol, orderID string) (Order, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.getBook(accountID, sym)
	return b.GetOrder(orderID)
}

// ListOpenOrders lists open orders for account+symbol.
func (e *Engine) ListOpenOrders(accountID string, sym Symbol) []Order {
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.getBook(accountID, sym)
	return b.ListOpenOrders()
}

// InstrumentBySymbol returns instrument metadata.
func (e *Engine) InstrumentBySymbol(sym Symbol) (Instrument, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ins, ok := e.ins[sym]
	if !ok || ins == nil {
		return Instrument{}, false
	}
	return *ins, true
}

// Depth returns shared depth if bound.
func (e *Engine) Depth(sym Symbol) (*MarketDepth, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	d, ok := e.depths[sym]
	return d, ok
}

// ForceClosePerpAtMark closes one-way net position at mark.
func (e *Engine) ForceClosePerpAtMark(accountID string, sym Symbol, mark decimal.Decimal) error {
	if !mark.GreaterThan(decimal.Zero) {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	ins, ok := e.ins[sym]
	if !ok || ins == nil || ins.Kind != KindPerp {
		return ErrUnknownSymbol
	}
	mode := e.modeForUnlock(accountID)
	slot := e.ledger.EnsurePerpSlot(accountID, sym, mode)
	if slot.Mode == PositionModeHedge {
		return nil
	}
	pos := slot.Net
	if pos.Qty.IsZero() {
		return nil
	}
	qtyAbs := pos.Qty.Abs()
	notional := mark.Mul(qtyAbs)
	fee := FeeNotional(notional, ins.TakerFeeBps)
	if pos.Qty.Sign() > 0 {
		fills := []Fill{{Price: mark, Size: qtyAbs}}
		return e.ledger.ApplyPerpCloseLong(accountID, sym, ins, fills, fee, slot)
	}
	fills := []Fill{{Price: mark, Size: qtyAbs}}
	return e.ledger.ApplyPerpCloseShort(accountID, sym, ins, fills, fee, slot)
}

// ForceCloseHedgeLegAtMark closes one hedge leg at mark.
func (e *Engine) ForceCloseHedgeLegAtMark(accountID string, sym Symbol, side ctypes.PositionSide, mark decimal.Decimal) error {
	if !mark.GreaterThan(decimal.Zero) {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	ins, ok := e.ins[sym]
	if !ok || ins == nil || ins.Kind != KindPerp {
		return ErrUnknownSymbol
	}
	mode := e.modeForUnlock(accountID)
	slot := e.ledger.EnsurePerpSlot(accountID, sym, mode)
	if slot.Mode != PositionModeHedge {
		return nil
	}
	switch side {
	case ctypes.PositionSideLong:
		if slot.Long.Qty.IsZero() {
			return nil
		}
		q := slot.Long.Qty
		notional := mark.Mul(q)
		fee := FeeNotional(notional, ins.TakerFeeBps)
		fills := []Fill{{Price: mark, Size: q}}
		return e.ledger.ApplyHedgeCloseLong(accountID, ins, fills, fee, slot)
	case ctypes.PositionSideShort:
		if slot.Short.Qty.IsZero() {
			return nil
		}
		q := slot.Short.Qty
		notional := mark.Mul(q)
		fee := FeeNotional(notional, ins.TakerFeeBps)
		fills := []Fill{{Price: mark, Size: q}}
		return e.ledger.ApplyHedgeCloseShort(accountID, ins, fills, fee, slot)
	default:
		return ErrInvalidIntent
	}
}

// NetPosition returns one-way position snapshot for compatibility.
func (e *Engine) NetPosition(accountID string, sym Symbol) (Position, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ledger.Position(accountID, sym)
}

// AccountSnapshot is a point-in-time view for account stream diffs.
type AccountSnapshot struct {
	Bal     map[BalanceKey]decimal.Decimal
	Slot    PerpSlot
	HasSlot bool
	Mode    PositionMode
}

// AccountSnapshot returns balances and perp slot under a single engine lock.
func (e *Engine) AccountSnapshot(accountID string, sym Symbol) AccountSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	bal := e.ledger.Balances(accountID)
	mode := e.modeForUnlock(accountID)
	slot, ok := e.ledger.GetPerpSlot(accountID, sym)
	if !ok {
		slot = PerpSlot{Mode: mode}
	}
	return AccountSnapshot{Bal: bal, Slot: slot, HasSlot: ok, Mode: mode}
}

// Balances returns a balance snapshot (copy).
func (e *Engine) Balances(accountID string) map[BalanceKey]decimal.Decimal {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ledger.Balances(accountID)
}

// SetLeverage stores leverage for (account, symbol).
func (e *Engine) SetLeverage(accountID string, sym Symbol, lev int) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.leverages == nil {
		e.leverages = make(map[accountLevKey]int)
	}
	e.leverages[accountLevKey{accountID, sym}] = lev
	return lev
}

// Leverage returns configured leverage or 1.
func (e *Engine) Leverage(accountID string, sym Symbol) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if v, ok := e.leverages[accountLevKey{accountID, sym}]; ok && v > 0 {
		return v
	}
	return 1
}

// AllSymbols returns registered instrument symbols.
func (e *Engine) AllSymbols() []Symbol {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Symbol, 0, len(e.ins))
	for s := range e.ins {
		out = append(out, s)
	}
	return out
}
