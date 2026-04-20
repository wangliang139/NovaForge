package simulate

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
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

	rt *VenueRuntime

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

func (e *Engine) WithRuntime(rt *VenueRuntime) *Engine {
	e.rt = rt
	return e
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

// SeedOpenOrder bootstraps open orders from persistence. Limit orders are added as resting (no immediate match).
// Market orders execute remaining qty against public depth like PlaceOrder; depth must exist (e.g. after ensureSymbolInitialized).
// onNew/onComplete are optional; for market orders they match PlaceOrder semantics (onComplete runs after e.mu is released).
func (e *Engine) SeedOpenOrder(accountID string, po Order) error {
	if po.OrderType == OrderTypeMarket {
		return e.seedOpenMarketOrder(accountID, po)
	}
	if po.OrderType != OrderTypeLimit {
		return fmt.Errorf("simulate: seed open order unsupported type")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	ins, ok := e.ins[po.Symbol]
	if !ok {
		return ErrUnknownSymbol
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

func (e *Engine) seedOpenMarketOrder(accountID string, po Order) error {
	e.mu.Lock()
	ins, ok := e.ins[po.Symbol]
	if !ok {
		e.mu.Unlock()
		return ErrUnknownSymbol
	}
	book := e.getBook(accountID, po.Symbol)
	if po.ID != "" {
		if ex, ok2 := book.GetOrder(po.ID); ok2 && ex.Status == OrderStatusFilled {
			e.mu.Unlock()
			return nil
		}
	}
	rem := FloorToStep(po.QtyRemaining, ins.QtyStep)
	if rem.Sign() <= 0 {
		e.mu.Unlock()
		return fmt.Errorf("simulate: seed market requires positive remaining qty")
	}
	if e.depths[po.Symbol] == nil {
		e.mu.Unlock()
		return ErrNotInitialized
	}
	req := PlaceOrderRequest{
		AccountID:     accountID,
		Symbol:        po.Symbol,
		OrderType:     OrderTypeMarket,
		Side:          po.Side,
		Intent:        po.Intent,
		ReduceOnly:    po.ReduceOnly,
		Leverage:      po.Leverage,
		PosSide:       po.PosSide,
		Price:         decimal.Zero,
		Qty:           rem,
		ClientOrderID: po.ClientOrderID,
		OrderID:       po.ID,
		Source:        po.Source,
	}
	res := e.placeOrderMuLocked(req, e.now())
	e.mu.Unlock()
	if res.Order.Status == OrderStatusRejected {
		return fmt.Errorf("simulate: seed market order rejected: %s", res.Order.RejectReason)
	}
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

func (e *Engine) validateOneWayPerp(o *Order, ins *Instrument, pos Position) error {
	if ins.Kind != KindPerp {
		return nil
	}
	switch o.Intent {
	case IntentOpen:
		if o.Leverage < 1 || (ins.LeverageMax > 0 && o.Leverage > ins.LeverageMax) {
			return ErrLeverage
		}
		if o.ReduceOnly {
			return ErrInvalidIntent
		}
		if o.Side == SideBuy && pos.Qty.Sign() < 0 {
			return ErrInvalidIntent
		}
		if o.Side == SideSell && pos.Qty.Sign() > 0 {
			return ErrInvalidIntent
		}
	case IntentClose:
		if !o.ReduceOnly {
			return ErrInvalidIntent
		}
		if o.Side == SideSell {
			if pos.Qty.Sign() <= 0 {
				return ErrInvalidIntent
			}
			if o.QtyOriginal.GreaterThan(pos.Qty) {
				return ErrReduceOnly
			}
		} else if o.Side == SideBuy {
			if pos.Qty.Sign() >= 0 {
				return ErrInvalidIntent
			}
			if o.QtyOriginal.GreaterThan(pos.Qty.Abs()) {
				return ErrReduceOnly
			}
		}
	}
	return nil
}

func (e *Engine) validateHedgePerp(o *Order, ins *Instrument, slot *PerpSlot) error {
	if !o.PosSide.Valid() {
		return ErrInvalidIntent
	}
	isBuy := o.Side == SideBuy
	var legQty decimal.Decimal
	switch o.PosSide {
	case ctypes.PositionSideLong:
		legQty = slot.Long.Qty
	case ctypes.PositionSideShort:
		legQty = slot.Short.Qty
	}
	if err := ValidateHedgeOrder(o.PosSide, isBuy, o.ReduceOnly, legQty); err != nil {
		return err
	}
	opening := HedgeOpen(o.PosSide, isBuy)
	if opening {
		if o.Leverage < 1 || (ins.LeverageMax > 0 && o.Leverage > ins.LeverageMax) {
			return ErrLeverage
		}
	} else {
		if o.QtyOriginal.GreaterThan(legQty) {
			return ErrReduceOnly
		}
	}
	return nil
}

func (e *Engine) rejectOrder(o *Order, reason string, ts time.Time) *PlaceOrderResult {
	book := e.getBook(o.AccountID, o.Symbol)
	now := ts.UTC()

	o.Status = OrderStatusRejected
	o.RejectReason = reason
	o.LastUpdatedAt = now

	book.PutOrderRecord(o)
	return &PlaceOrderResult{Order: *o, Fills: nil, FeePaid: decimal.Zero}
}

// PlaceOrderCompleteFunc runs after PlaceOrder commits ledger and releases e.mu.
// Implementations must not call back into Engine (deadlock risk).
type PlaceOrderCompleteFunc func(before, after AccountSnapshot, res *PlaceOrderResult)

// PlaceOrderNewFunc is invoked after the working order is created (StatusNew) and before matching; e.mu is held.
// Implementations must not call back into Engine (deadlock risk).
type PlaceOrderNewFunc func(order Order)

// PlaceOrder validates, matches against public depth, updates ledger, rests remainder.
// If onNew is non-nil, it runs once the order record is accepted (NEW) and before any fill simulation.
// If onComplete is non-nil, it is invoked after mutations with before/after account snapshots (lock released).
func (e *Engine) PlaceOrder(_ context.Context, req PlaceOrderRequest) {
	ts := e.now()
	e.mu.Lock()
	_ = e.placeOrderMuLocked(req, ts)
	e.mu.Unlock()
}

// placeOrderMuLocked runs the matching/ledger path; e.mu must be held.
func (e *Engine) placeOrderMuLocked(req PlaceOrderRequest, ts time.Time) *PlaceOrderResult {
	accountID := req.AccountID
	orderID := req.OrderID
	if orderID == "" {
		orderID = e.genOrderID(accountID)
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
		Price:         req.Price,
		QtyOriginal:   req.Qty,
		QtyRemaining:  req.Qty,
		Status:        OrderStatusNew,
		CreatedAt:     now,
		LastUpdatedAt: now,
		Source:        req.Source,
	}
	res := e.doPlaceOrder(order, ts)
	after := e.accountSnapshotLocked(req.AccountID, req.Symbol)
	if e.rt != nil {
		// 订单快照
		e.rt.enqueueAccountPublish(AccountEvent{
			accountID: accountID,
			symbol:    req.Symbol,
			kind:      AccountEventTypeOrder,
			order:     order,
		})
		// 资产事件
		e.rt.enqueueAccountPublish(AccountEvent{
			accountID: accountID,
			symbol:    req.Symbol,
			kind:      AccountEventTypeBalance,
			balance:   &after,
		})
		// 持仓事件
		e.rt.enqueueAccountPublish(AccountEvent{
			accountID: accountID,
			symbol:    req.Symbol,
			kind:      AccountEventTypePosition,
			position:  &after.Slot,
		})
	}
	return res
}

// doPlaceOrder runs the matching/ledger path; e.mu must be held.
func (e *Engine) doPlaceOrder(o *Order, ts time.Time) *PlaceOrderResult {
	accountID := o.AccountID

	if e.rt != nil {
		e.rt.enqueueAccountPublish(AccountEvent{
			accountID: o.AccountID,
			symbol:    o.Symbol,
			kind:      AccountEventTypeOrder,
			order:     o,
		})
	}

	ins, ok := e.ins[o.Symbol]
	if !ok {
		return e.rejectOrder(o, ErrUnknownSymbol.Error(), ts)
	}
	d, ok := e.depths[o.Symbol]
	if !ok || d == nil {
		return e.rejectOrder(o, ErrNotInitialized.Error(), ts)
	}
	book := e.getBook(accountID, o.Symbol)

	mode := e.modeForUnlock(accountID)
	slot := e.ledger.EnsurePerpSlot(accountID, o.Symbol, mode)
	if slot.Mode != mode {
		// slot existed with different mode — should not happen if SetPerpMode used
		slot.Mode = mode
	}

	if ins.Kind == KindPerp {
		if mode == PositionModeHedge {
			if err := e.validateHedgePerp(o, ins, slot); err != nil {
				return e.rejectOrder(o, err.Error(), ts)
			}
		} else {
			pos := slot.Net
			if err := e.validateOneWayPerp(o, ins, pos); err != nil {
				return e.rejectOrder(o, err.Error(), ts)
			}
		}
	}

	qty := FloorToStep(o.QtyOriginal, ins.QtyStep)
	price := o.Price
	if o.OrderType == OrderTypeLimit {
		price = FloorToTick(o.Price, ins.PriceTick)
	}
	if err := ins.ValidateOrderParams(price, qty, o.OrderType == OrderTypeLimit); err != nil {
		return e.rejectOrder(o, err.Error(), ts)
	}

	var allFills []Fill
	var feeSum decimal.Decimal
	var feeBaseSum decimal.Decimal // 现货买入：累计以 base 计的手续费（用于订单展示与 attach）
	takerBps := ins.TakerFeeBps
	lev := o.Leverage
	if lev <= 0 {
		lev = int32(DefaultSimulateLeverage)
	}

	now := ts.UTC()
	switch ins.Kind {
	case KindSpot:
		switch o.OrderType {
		case OrderTypeMarket:
			ref := midPrice(d)
			if err := ins.ValidateMarketQty(qty, ref); err != nil {
				return e.rejectPlacedOrder(o, book, err.Error(), now)
			}
			var fills []Fill
			var notional decimal.Decimal
			if o.Side == SideBuy {
				fills, _, notional = SimulateMarketBuy(d, qty)
			} else {
				fills, _, notional = SimulateMarketSell(d, qty)
			}
			if len(fills) == 0 {
				o.Status = OrderStatusRejected
				o.RejectReason = "no liquidity"
				book.PutOrderRecord(o)
				return &PlaceOrderResult{Order: *o, Fills: nil, FeePaid: decimal.Zero}
			}
			filledQty := totalFilledQty(fills)
			fee := FeeNotional(notional, takerBps)
			if o.Side == SideBuy {
				if err := e.ledger.ApplySpotBuy(accountID, ins, fills, fee); err != nil {
					return e.rejectPlacedOrder(o, book, err.Error(), now)
				}
				feeBaseSum = feeBaseSum.Add(SpotFeeBaseFromQuote(notional, filledQty, fee))
			} else {
				if err := e.ledger.ApplySpotSell(accountID, ins, fills, fee); err != nil {
					return e.rejectPlacedOrder(o, book, err.Error(), now)
				}
			}
			allFills = fills
			feeSum = fee
			fillOrderImmediate(o, fills, notional, filledQty, now)

		case OrderTypeLimit:
			if err := ins.ValidateOrderParams(price, qty, true); err != nil {
				return e.rejectPlacedOrder(o, book, err.Error(), now)
			}
			var fills []Fill
			var notional decimal.Decimal
			if o.Side == SideBuy {
				ba, _, okA := d.BestAsk()
				if okA && !ba.GreaterThan(price) {
					fills, o.QtyRemaining, notional = SimulateLimitBuy(d, price, qty)
				}
			} else {
				bb, _, okB := d.BestBid()
				if okB && !bb.LessThan(price) {
					fills, o.QtyRemaining, notional = SimulateLimitSell(d, price, qty)
				}
			}
			if len(fills) > 0 {
				filledQty := totalFilledQty(fills)
				fee := FeeNotional(notional, takerBps)
				if o.Side == SideBuy {
					if err := e.ledger.ApplySpotBuy(accountID, ins, fills, fee); err != nil {
						return e.rejectPlacedOrder(o, book, err.Error(), now)
					}
					feeBaseSum = feeBaseSum.Add(SpotFeeBaseFromQuote(notional, filledQty, fee))
				} else {
					if err := e.ledger.ApplySpotSell(accountID, ins, fills, fee); err != nil {
						return e.rejectPlacedOrder(o, book, err.Error(), now)
					}
				}
				allFills = append(allFills, fills...)
				feeSum = feeSum.Add(fee)
				partialFillOrder(o, fills, notional, filledQty, now)
			}
			if o.QtyRemaining.Sign() > 0 {
				book.AddResting(o)
			} else if len(fills) > 0 {
				o.Status = OrderStatusFilled
			} else {
				book.AddResting(o)
			}
		}

	case KindPerp:
		switch o.OrderType {
		case OrderTypeMarket:
			ref := midPrice(d)
			if err := ins.ValidateMarketQty(qty, ref); err != nil {
				return e.rejectPlacedOrder(o, book, err.Error(), now)
			}
			var fills []Fill
			var notional decimal.Decimal
			if o.Side == SideBuy {
				fills, _, notional = SimulateMarketBuy(d, qty)
			} else {
				fills, _, notional = SimulateMarketSell(d, qty)
			}
			if len(fills) == 0 {
				o.Status = OrderStatusRejected
				o.RejectReason = "no liquidity"
				book.PutOrderRecord(o)
				return &PlaceOrderResult{Order: *o, Fills: nil, FeePaid: decimal.Zero}
			}
			filledQty := totalFilledQty(fills)
			fee := FeeNotional(notional, takerBps)
			if err := e.applyPerpFills(accountID, ins, o, fills, fee, lev, slot); err != nil {
				return e.rejectPlacedOrder(o, book, err.Error(), now)
			}
			allFills = fills
			feeSum = fee
			fillOrderImmediate(o, fills, notional, filledQty, now)

		case OrderTypeLimit:
			if err := ins.ValidateOrderParams(price, qty, true); err != nil {
				return e.rejectPlacedOrder(o, book, err.Error(), now)
			}
			var fills []Fill
			var notional decimal.Decimal
			if o.Side == SideBuy {
				ba, _, okA := d.BestAsk()
				if okA && !ba.GreaterThan(price) {
					fills, o.QtyRemaining, notional = SimulateLimitBuy(d, price, qty)
				}
			} else {
				bb, _, okB := d.BestBid()
				if okB && !bb.LessThan(price) {
					fills, o.QtyRemaining, notional = SimulateLimitSell(d, price, qty)
				}
			}
			if len(fills) > 0 {
				filledQty := totalFilledQty(fills)
				fee := FeeNotional(notional, takerBps)
				if err := e.applyPerpFills(accountID, ins, o, fills, fee, lev, slot); err != nil {
					return e.rejectPlacedOrder(o, book, err.Error(), now)
				}
				allFills = append(allFills, fills...)
				feeSum = feeSum.Add(fee)
				partialFillOrder(o, fills, notional, filledQty, now)
			}
			if o.QtyRemaining.Sign() > 0 {
				book.AddResting(o)
			} else if len(fills) > 0 {
				o.Status = OrderStatusFilled
			} else {
				book.AddResting(o)
			}
		}
	}

	attachOrderFeeMeta(o, ins, feeSum, feeBaseSum)
	if o.Status == OrderStatusFilled || o.Status == OrderStatusRejected {
		book.PutOrderRecord(o)
	}
	return &PlaceOrderResult{Order: *o, Fills: allFills, FeePaid: o.FeePaid}
}

func quoteRealizedPnLForPerpOrder(slot *PerpSlot, o *Order, fills []Fill) decimal.Decimal {
	if slot == nil || len(fills) == 0 {
		return decimal.Zero
	}
	if slot.Mode == PositionModeHedge {
		isBuy := o.Side == SideBuy
		if HedgeOpen(o.PosSide, isBuy) {
			return decimal.Zero
		}
		switch o.PosSide {
		case ctypes.PositionSideLong:
			return QuoteRealizedPnLHedgeCloseLong(slot.Long, fills)
		case ctypes.PositionSideShort:
			return QuoteRealizedPnLHedgeCloseShort(slot.Short, fills)
		default:
			return decimal.Zero
		}
	}
	if o.Intent != IntentClose {
		return decimal.Zero
	}
	if o.Side == SideSell {
		return QuoteRealizedPnLOneWayCloseLong(slot.Net, fills)
	}
	return QuoteRealizedPnLOneWayCloseShort(slot.Net, fills)
}

func (e *Engine) applyPerpFills(accountID string, ins *Instrument, o *Order, fills []Fill, fee decimal.Decimal, lev int32, slot *PerpSlot) error {
	// RealizedPnl 按订单累计（限价多次吃单路径会多次进入此处）
	o.RealizedPnl = o.RealizedPnl.Add(quoteRealizedPnLForPerpOrder(slot, o, fills))
	if slot.Mode == PositionModeHedge {
		isBuy := o.Side == SideBuy
		opening := HedgeOpen(o.PosSide, isBuy)
		switch o.PosSide {
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
	switch o.Intent {
	case IntentOpen:
		if o.Side == SideBuy {
			return e.ledger.ApplyPerpOpenLong(accountID, o.Symbol, ins, fills, fee, lev, slot)
		}
		return e.ledger.ApplyPerpOpenShort(accountID, o.Symbol, ins, fills, fee, lev, slot)
	case IntentClose:
		if o.Side == SideSell {
			return e.ledger.ApplyPerpCloseLong(accountID, o.Symbol, ins, fills, fee, slot)
		}
		return e.ledger.ApplyPerpCloseShort(accountID, o.Symbol, ins, fills, fee, slot)
	default:
		return ErrInvalidIntent
	}
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

// attachOrderFeeMeta sets cumulative fee on the wire Order when there are fills (negative = paid).
// feeSumQuote: 按 bps×名义累计的 quote 手续费额；feeBaseSum: 现货买入时累计的 base 手续费量（与 ledger 一致）。
func attachOrderFeeMeta(o *Order, ins *Instrument, feeSumQuote, feeBaseSum decimal.Decimal) {
	if o == nil || ins == nil || o.QtyFilled.Sign() <= 0 {
		return
	}
	if ins.Kind == KindSpot && o.Side == SideBuy {
		o.FeePaid = feeBaseSum.Neg()
		o.FeeAsset = string(ins.Base)
		return
	}
	o.FeePaid = feeSumQuote.Neg()
	o.FeeAsset = string(ins.Quote)
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
func (e *Engine) OnDepthUpdated(sym Symbol) {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.onDepthUpdatedUnlocked(sym)
	if err != nil {
		log.Error().Str("symbol", string(sym)).Msg("simulate: onDepthUpdated failed")
		return
	}
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
					fq := totalFilledQty(ev.Fills)
					fb := SpotFeeBaseFromQuote(notional, fq, fee)
					ev.Order.FeePaid = ev.Order.FeePaid.Add(fb.Neg())
					ev.Order.FeeAsset = string(ins.Base)
				} else {
					if err := e.ledger.ApplySpotSell(ev.Order.AccountID, ins, ev.Fills, fee); err != nil {
						return all, err
					}
					ev.Order.FeePaid = ev.Order.FeePaid.Add(fee.Neg())
					if ev.Order.FeeAsset == "" {
						ev.Order.FeeAsset = string(ins.Quote)
					}
				}
			case KindPerp:
				if err := e.applyPerpFills(ev.Order.AccountID, ins, ev.Order, ev.Fills, fee, ev.Order.Leverage, slot); err != nil {
					return all, err
				}
				ev.Order.FeePaid = ev.Order.FeePaid.Add(fee.Neg())
				if ev.Order.FeeAsset == "" {
					ev.Order.FeeAsset = string(ins.Quote)
				}
			}

			if e.rt != nil {
				after := e.accountSnapshotLocked(ev.Order.AccountID, ev.Order.Symbol)
				// 发布订单事件
				e.rt.enqueueAccountPublish(AccountEvent{
					accountID: ev.Order.AccountID,
					symbol:    ev.Order.Symbol,
					kind:      AccountEventTypeOrder,
					order:     ev.Order,
				})
				// 发布资产事件
				e.rt.enqueueAccountPublish(AccountEvent{
					accountID: ev.Order.AccountID,
					symbol:    ev.Order.Symbol,
					kind:      AccountEventTypeBalance,
					balance:   &after,
				})
				// 发布持仓事件
				e.rt.enqueueAccountPublish(AccountEvent{
					accountID: ev.Order.AccountID,
					symbol:    ev.Order.Symbol,
					kind:      AccountEventTypePosition,
					position:  &after.Slot,
				})
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

// ListAllOpenOrders lists open orders for account across all symbols that have a resting book.
func (e *Engine) ListAllOpenOrders(accountID string) []Order {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []Order
	for key, book := range e.books {
		if key.AccountID != accountID {
			continue
		}
		out = append(out, book.ListOpenOrders()...)
	}
	return out
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

// PlaceLiquidationMarket runs a reduce-only market order against public depth (e.g. forced liquidation).
// Caller must set req.Source to OrderSourceLiquidation. e.mu is acquired internally.
func (e *Engine) PlaceLiquidationMarket(req PlaceOrderRequest) *PlaceOrderResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.placeOrderMuLocked(req, e.now())
}

// forceCloseOneWayAtMarkSynthetic closes one-way net at mark without the order book (insurance-style fallback).
func (e *Engine) forceCloseOneWayAtMarkSynthetic(accountID string, sym Symbol, mark decimal.Decimal) *PlaceOrderResult {
	if !mark.GreaterThan(decimal.Zero) {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	ins, ok := e.ins[sym]
	if !ok || ins == nil || ins.Kind != KindPerp {
		return nil
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
	var fills []Fill
	var rp decimal.Decimal
	if pos.Qty.Sign() > 0 {
		fills = []Fill{{Price: mark, Size: qtyAbs}}
		rp = QuoteRealizedPnLOneWayCloseLong(pos, fills)
		if err := e.ledger.ApplyPerpCloseLong(accountID, sym, ins, fills, fee, slot); err != nil {
			return nil
		}
	} else {
		fills = []Fill{{Price: mark, Size: qtyAbs}}
		rp = QuoteRealizedPnLOneWayCloseShort(pos, fills)
		if err := e.ledger.ApplyPerpCloseShort(accountID, sym, ins, fills, fee, slot); err != nil {
			return nil
		}
	}
	now := e.now().UTC()
	side := SideSell
	if pos.Qty.Sign() < 0 {
		side = SideBuy
	}
	lev := pos.Leverage
	if lev <= 0 {
		lev = int32(DefaultSimulateLeverage)
	}
	o := Order{
		ID:            e.genOrderID(accountID),
		AccountID:     accountID,
		Symbol:        sym,
		OrderType:     OrderTypeMarket,
		Side:          side,
		Intent:        IntentClose,
		ReduceOnly:    true,
		Leverage:      lev,
		QtyOriginal:   qtyAbs,
		QtyRemaining:  decimal.Zero,
		QtyFilled:     qtyAbs,
		AvgFillPrice:  mark,
		FeePaid:       fee.Neg(),
		FeeAsset:      string(ins.Quote),
		RealizedPnl:   rp,
		Status:        OrderStatusFilled,
		CreatedAt:     now,
		LastUpdatedAt: now,
		Source:        ctypes.OrderSourceLiquidation,
	}
	return &PlaceOrderResult{Order: o, Fills: fills, FeePaid: fee.Neg()}
}

// forceCloseHedgeLegAtMarkSynthetic closes one hedge leg at mark without the order book (fallback).
func (e *Engine) forceCloseHedgeLegAtMarkSynthetic(accountID string, sym Symbol, side ctypes.PositionSide, mark decimal.Decimal) *PlaceOrderResult {
	if !mark.GreaterThan(decimal.Zero) {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	ins, ok := e.ins[sym]
	if !ok || ins == nil || ins.Kind != KindPerp {
		return nil
	}
	mode := e.modeForUnlock(accountID)
	slot := e.ledger.EnsurePerpSlot(accountID, sym, mode)
	if slot.Mode != PositionModeHedge {
		return nil
	}
	now := e.now().UTC()
	var fills []Fill
	var fee decimal.Decimal
	var o Order
	switch side {
	case ctypes.PositionSideLong:
		if slot.Long.Qty.IsZero() {
			return nil
		}
		legSnap := slot.Long
		q := slot.Long.Qty
		notional := mark.Mul(q)
		fee = FeeNotional(notional, ins.TakerFeeBps)
		fills = []Fill{{Price: mark, Size: q}}
		rpLong := QuoteRealizedPnLHedgeCloseLong(legSnap, fills)
		if err := e.ledger.ApplyHedgeCloseLong(accountID, ins, fills, fee, slot); err != nil {
			return nil
		}
		lev := legSnap.Leverage
		if lev <= 0 {
			lev = int32(DefaultSimulateLeverage)
		}
		o = Order{
			ID:            e.genOrderID(accountID),
			AccountID:     accountID,
			Symbol:        sym,
			OrderType:     OrderTypeMarket,
			Side:          SideSell,
			Intent:        IntentClose,
			ReduceOnly:    true,
			Leverage:      lev,
			PosSide:       ctypes.PositionSideLong,
			QtyOriginal:   q,
			QtyRemaining:  decimal.Zero,
			QtyFilled:     q,
			AvgFillPrice:  mark,
			FeePaid:       fee.Neg(),
			FeeAsset:      string(ins.Quote),
			RealizedPnl:   rpLong,
			Status:        OrderStatusFilled,
			CreatedAt:     now,
			LastUpdatedAt: now,
			Source:        ctypes.OrderSourceLiquidation,
		}
	case ctypes.PositionSideShort:
		if slot.Short.Qty.IsZero() {
			return nil
		}
		legSnap := slot.Short
		q := slot.Short.Qty
		notional := mark.Mul(q)
		fee = FeeNotional(notional, ins.TakerFeeBps)
		fills = []Fill{{Price: mark, Size: q}}
		rpSh := QuoteRealizedPnLHedgeCloseShort(legSnap, fills)
		if err := e.ledger.ApplyHedgeCloseShort(accountID, ins, fills, fee, slot); err != nil {
			return nil
		}
		lev := legSnap.Leverage
		if lev <= 0 {
			lev = int32(DefaultSimulateLeverage)
		}
		o = Order{
			ID:            e.genOrderID(accountID),
			AccountID:     accountID,
			Symbol:        sym,
			OrderType:     OrderTypeMarket,
			Side:          SideBuy,
			Intent:        IntentClose,
			ReduceOnly:    true,
			Leverage:      lev,
			PosSide:       ctypes.PositionSideShort,
			QtyOriginal:   q,
			QtyRemaining:  decimal.Zero,
			QtyFilled:     q,
			AvgFillPrice:  mark,
			FeePaid:       fee.Neg(),
			FeeAsset:      string(ins.Quote),
			RealizedPnl:   rpSh,
			Status:        OrderStatusFilled,
			CreatedAt:     now,
			LastUpdatedAt: now,
			Source:        ctypes.OrderSourceLiquidation,
		}
	default:
		return nil
	}
	return &PlaceOrderResult{Order: o, Fills: fills, FeePaid: fee.Neg()}
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
	return e.accountSnapshotLocked(accountID, sym)
}

// accountSnapshotLocked is like AccountSnapshot but caller must hold e.mu.
func (e *Engine) accountSnapshotLocked(accountID string, sym Symbol) AccountSnapshot {
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

// applyLeverageStateLocked updates per-symbol leverage map and rescales slot margin when instr is perp.
// Caller must hold e.mu.
func (e *Engine) applyLeverageStateLocked(accountID string, sym Symbol, lev int) error {
	if e.leverages == nil {
		e.leverages = make(map[accountLevKey]int)
	}
	ins := e.ins[sym]
	if ins != nil && ins.Kind == KindPerp {
		if err := e.ledger.SyncPerpSlotLeverage(accountID, sym, ins, int32(lev)); err != nil {
			return err
		}
	}
	e.leverages[accountLevKey{accountID, sym}] = lev
	return nil
}

func (e *Engine) leverageForAccountSymbolLocked(accountID string, sym Symbol) int {
	if v, ok := e.leverages[accountLevKey{accountID, sym}]; ok && v > 0 {
		return v
	}
	return DefaultSimulateLeverage
}

func (e *Engine) publishLeverageChangeLocked(accountID string, sym Symbol, lev int) {
	if e.rt == nil {
		return
	}
	for _, side := range []ctypes.PositionSide{ctypes.PositionSideLong, ctypes.PositionSideShort} {
		e.rt.enqueueAccountPublish(AccountEvent{
			accountID: accountID,
			symbol:    sym,
			kind:      AccountEventTypeLeverage,
			leverage:  &LeverageChange{leverage: lev, leverageSide: side},
		})
	}
}

// SetLeverage stores leverage for (account, symbol), syncs perp slot margin, and publishes when leverage changes.
func (e *Engine) SetLeverage(accountID string, sym Symbol, lev int) (int, error) {
	if lev < 1 {
		return 0, fmt.Errorf("simulate: invalid leverage")
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	oldLev := e.leverageForAccountSymbolLocked(accountID, sym)
	if oldLev == lev {
		return lev, nil
	}

	ins := e.ins[sym]
	if ins != nil && ins.Kind == KindSpot {
		return 0, fmt.Errorf("simulate: spot does not support leverage")
	}

	if err := e.applyLeverageStateLocked(accountID, sym, lev); err != nil {
		return 0, err
	}

	e.publishLeverageChangeLocked(accountID, sym, lev)
	return lev, nil
}

// MergeSymbolLeverages sets leverage for multiple symbols (does not delete keys omitted from levs).
// Does not emit account stream events (used when hydrating from persistence).
func (e *Engine) MergeSymbolLeverages(accountID string, levs map[Symbol]int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(levs) == 0 {
		return
	}
	for sym, lev := range levs {
		if lev <= 0 {
			continue
		}
		ins := e.ins[sym]
		if ins != nil && ins.Kind != KindPerp {
			continue
		}
		if err := e.applyLeverageStateLocked(accountID, sym, lev); err != nil {
			log.Warn().Err(err).Str("symbol", string(sym)).Msg("simulate: merge leverage skipped")
			continue
		}
	}
}

// Leverage returns configured leverage or DefaultSimulateLeverage.
func (e *Engine) Leverage(accountID string, sym Symbol) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.leverageForAccountSymbolLocked(accountID, sym)
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

// fundingQuoteWalletDelta returns quote wallet delta (positive = receive) for Binance/OKX USDT-M style funding:
// notional uses mark × contract multiplier × qty; long pays shorts when rate > 0.
func fundingQuoteWalletDelta(slot *PerpSlot, mark, rate, mult decimal.Decimal) decimal.Decimal {
	if rate.IsZero() || !mark.GreaterThan(decimal.Zero) || mult.Sign() <= 0 || slot == nil {
		return decimal.Zero
	}
	m := mark.Mul(mult).Mul(rate)
	switch slot.Mode {
	case PositionModeHedge:
		return slot.Short.Qty.Sub(slot.Long.Qty).Mul(m)
	default:
		return slot.Net.Qty.Neg().Mul(m)
	}
}

// settleFunding applies funding to all accounts with a perp slot on sym; publishes balance snapshots only.
func (e *Engine) settleFunding(sym Symbol, mark, rate decimal.Decimal) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ins := e.ins[sym]
	if ins == nil || ins.Kind != KindPerp {
		return
	}
	if !mark.GreaterThan(decimal.Zero) || rate.IsZero() {
		return
	}
	mult := DefaultContractMultiplier(ins.ContractMultiplier)
	wt := ins.WalletType()
	quote := ins.Quote
	ids := e.ledger.ListAccountIDs()
	var changed []string
	for _, aid := range ids {
		st, ok := e.ledger.GetPerpSlot(aid, sym)
		if !ok {
			continue
		}
		delta := fundingQuoteWalletDelta(&st, mark, rate, mult)
		if delta.IsZero() {
			continue
		}
		e.ledger.ApplyQuoteDelta(aid, wt, quote, delta)
		changed = append(changed, aid)
	}
	if len(changed) == 0 || e.rt == nil {
		return
	}
	for _, aid := range changed {
		after := e.accountSnapshotLocked(aid, sym)
		e.rt.enqueueAccountPublish(AccountEvent{
			accountID: aid,
			symbol:    sym,
			kind:      AccountEventTypeBalance,
			balance:   &after,
		})
	}
}
