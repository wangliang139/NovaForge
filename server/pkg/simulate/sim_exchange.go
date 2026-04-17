package simulate

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"
)

// SimExchange is a minimal simulated exchange: instruments, shared public depth per symbol,
// user SimBook, and portfolio (spot + one-way perp).
type SimExchange struct {
	mu sync.Mutex

	ins    map[Symbol]*Instrument
	depths map[Symbol]*MarketDepth
	books  map[accountSymbolKey]*SimBook
	port   *Portfolio
	nowFn  func() time.Time

	nextOrderID int64
}

// SimExchangeOption customizes simulator runtime behavior.
type SimExchangeOption func(*SimExchange)

type accountSymbolKey struct {
	AccountID string
	Symbol    Symbol
}

// WithNowFn injects a deterministic clock source for replay/testing.
func WithNowFn(nowFn func() time.Time) SimExchangeOption {
	return func(e *SimExchange) {
		if nowFn != nil {
			e.nowFn = nowFn
		}
	}
}

// NewSimExchange creates an exchange with an empty portfolio.
func NewSimExchange(opts ...SimExchangeOption) *SimExchange {
	nowFn := func() time.Time { return time.Now().UTC() }
	e := &SimExchange{
		ins:    make(map[Symbol]*Instrument),
		depths: make(map[Symbol]*MarketDepth),
		books:  make(map[accountSymbolKey]*SimBook),
		port:   NewPortfolio(),
		nowFn:  nowFn,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func (e *SimExchange) now() time.Time {
	if e.nowFn == nil {
		return time.Now().UTC()
	}
	return e.nowFn().UTC()
}

func normalizeAccountID(accountID string) (string, error) {
	if accountID == "" {
		return "", ErrInvalidAccount
	}
	return accountID, nil
}

func (e *SimExchange) InitBalances(accountID string, bals map[Asset]decimal.Decimal) error {
	if len(bals) == 0 {
		return nil
	}
	var err error
	accountID, err = normalizeAccountID(accountID)
	if err != nil {
		return err
	}
	for asset, amount := range bals {
		e.port.SetBalance(accountID, asset, amount)
	}
	return nil
}

func (e *SimExchange) InitPosition(accountID string, sym Symbol, pos Position) error {
	accountID, err := normalizeAccountID(accountID)
	if err != nil {
		return err
	}
	e.port.SetPosition(accountID, sym, pos)
	return nil
}

// Portfolio returns the underlying portfolio (same pointer; external code should not mutate without care).
func (e *SimExchange) Portfolio() *Portfolio { return e.port }

// RegisterInstrument registers symbol metadata and allocates a SimBook for the symbol.
func (e *SimExchange) RegisterInstrument(ins *Instrument) error {
	if ins == nil {
		return ErrUnknownSymbol
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ins[ins.Symbol] = ins
	return nil
}

// BindDepth attaches the public L2 maintained elsewhere (e.g. via Coordinator) to a symbol.
func (e *SimExchange) BindDepth(sym Symbol, d *MarketDepth) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.ins[sym]; !ok {
		return ErrUnknownSymbol
	}
	e.depths[sym] = d
	return nil
}

func (e *SimExchange) getBook(accountID string, sym Symbol) *SimBook {
	key := accountSymbolKey{AccountID: accountID, Symbol: sym}
	b := e.books[key]
	if b == nil {
		b = NewSimBook(sym, e.now)
		e.books[key] = b
	}
	return b
}

func (e *SimExchange) genOrderID() string {
	n := atomic.AddInt64(&e.nextOrderID, 1)
	return fmt.Sprintf("o%d", n)
}

func (e *SimExchange) getDepth(sym Symbol) *MarketDepth {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.depths[sym]
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

func (e *SimExchange) validateRequest(req *PlaceOrderRequest, ins *Instrument, pos Position) error {
	if req == nil {
		return ErrInvalidQty
	}
	if ins.Kind == KindSpot {
		// Spot ignores Intent / ReduceOnly.
	}
	if ins.Kind == KindPerp {
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
	}
	return nil
}

// PlaceOrder validates, matches against public depth (shadow), updates portfolio, and rests the remainder.
func (e *SimExchange) PlaceOrder(ctx context.Context, req PlaceOrderRequest) (*PlaceOrderResult, error) {
	return e.placeOrderAt(ctx, req, e.now())
}

// PlaceOrderAt is deterministic placement entry used by replay/event-pump.
func (e *SimExchange) PlaceOrderAt(ctx context.Context, req PlaceOrderRequest, ts time.Time) (*PlaceOrderResult, error) {
	return e.placeOrderAt(ctx, req, ts)
}

func (e *SimExchange) placeOrderAt(ctx context.Context, req PlaceOrderRequest, ts time.Time) (*PlaceOrderResult, error) {
	_ = ctx
	e.mu.Lock()
	defer e.mu.Unlock()

	ins, ok := e.ins[req.Symbol]
	if !ok {
		return nil, ErrUnknownSymbol
	}
	d, ok := e.depths[req.Symbol]
	if !ok || d == nil {
		return nil, ErrNotInitialized
	}
	accountID, err := normalizeAccountID(req.AccountID)
	if err != nil {
		return nil, err
	}
	book := e.getBook(accountID, req.Symbol)

	pos, _ := e.port.Position(accountID, req.Symbol)
	if err := e.validateRequest(&req, ins, pos); err != nil {
		return nil, err
	}

	qty := FloorToStep(req.Qty, ins.QtyStep)
	price := req.Price
	if req.OrderType == OrderTypeLimit {
		price = FloorToTick(req.Price, ins.PriceTick)
	}
	if err := ins.ValidateOrderParams(price, qty, req.OrderType == OrderTypeLimit); err != nil {
		return nil, err
	}

	now := ts.UTC()
	order := &SimOrder{
		ID:            e.genOrderID(),
		AccountID:     accountID,
		ClientOrderID: req.ClientOrderID,
		Symbol:        req.Symbol,
		OrderType:     req.OrderType,
		Side:          req.Side,
		Intent:        req.Intent,
		ReduceOnly:    req.ReduceOnly,
		Leverage:      req.Leverage,
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

	switch ins.Kind {
	case KindSpot:
		switch req.OrderType {
		case OrderTypeMarket:
			ref := midPrice(d)
			if err := ins.ValidateMarketQty(qty, ref); err != nil {
				return nil, err
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
				if err := e.port.ApplySpotBuy(accountID, ins, fills, fee); err != nil {
					return nil, err
				}
			} else {
				if err := e.port.ApplySpotSell(accountID, ins, fills, fee); err != nil {
					return nil, err
				}
			}
			allFills = fills
			feeSum = fee
			fillOrderImmediate(order, fills, notional, filledQty, now)

		case OrderTypeLimit:
			if err := ins.ValidateOrderParams(price, qty, true); err != nil {
				return nil, err
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
					if err := e.port.ApplySpotBuy(accountID, ins, fills, fee); err != nil {
						return nil, err
					}
				} else {
					if err := e.port.ApplySpotSell(accountID, ins, fills, fee); err != nil {
						return nil, err
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
		lev := req.Leverage
		if lev <= 0 {
			lev = 1
		}
		switch req.OrderType {
		case OrderTypeMarket:
			ref := midPrice(d)
			if err := ins.ValidateMarketQty(qty, ref); err != nil {
				return nil, err
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
			if err := e.applyPerpFills(accountID, ins, &req, fills, fee, lev); err != nil {
				return nil, err
			}
			allFills = fills
			feeSum = fee
			fillOrderImmediate(order, fills, notional, filledQty, now)

		case OrderTypeLimit:
			if err := ins.ValidateOrderParams(price, qty, true); err != nil {
				return nil, err
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
				if err := e.applyPerpFills(accountID, ins, &req, fills, fee, lev); err != nil {
					return nil, err
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
	// Resting orders already in byID via AddResting.
	cp := *order
	return &PlaceOrderResult{Order: cp, Fills: allFills, FeePaid: feeSum}, nil
}

func totalFilledQty(fills []Fill) decimal.Decimal {
	var q decimal.Decimal
	for _, f := range fills {
		q = q.Add(f.Size)
	}
	return q
}

func fillOrderImmediate(o *SimOrder, fills []Fill, notional, filledQty decimal.Decimal, now time.Time) {
	o.QtyFilled = filledQty
	o.QtyRemaining = decimal.Zero
	o.AvgFillPrice = AveragePrice(notional, filledQty)
	o.Status = OrderStatusFilled
	o.LastUpdatedAt = now.UTC()
}

func partialFillOrder(o *SimOrder, fills []Fill, notional, filledQty decimal.Decimal, now time.Time) {
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

func (e *SimExchange) applyPerpFills(accountID string, ins *Instrument, req *PlaceOrderRequest, fills []Fill, fee decimal.Decimal, lev int32) error {
	switch req.Intent {
	case IntentOpen:
		if req.Side == SideBuy {
			return e.port.ApplyPerpOpenLong(accountID, req.Symbol, ins, fills, fee, lev)
		}
		return e.port.ApplyPerpOpenShort(accountID, req.Symbol, ins, fills, fee, lev)
	case IntentClose:
		if req.Side == SideSell {
			return e.port.ApplyPerpCloseLong(accountID, req.Symbol, ins, fills, fee)
		}
		return e.port.ApplyPerpCloseShort(accountID, req.Symbol, ins, fills, fee)
	default:
		return ErrInvalidIntent
	}
}

// OnDepthUpdated should be called after each public depth snapshot/delta apply; it matches resting orders (maker).
func (e *SimExchange) OnDepthUpdated(sym Symbol) ([]MatchEvent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	d, ok := e.depths[sym]
	if !ok || d == nil {
		return nil, ErrNotInitialized
	}
	ins, ok := e.ins[sym]
	if !ok {
		return nil, ErrUnknownSymbol
	}
	allEvents := make([]MatchEvent, 0)
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
			req := PlaceOrderRequest{
				AccountID:  ev.Order.AccountID,
				Symbol:     sym,
				Side:       ev.Order.Side,
				Intent:     ev.Order.Intent,
				ReduceOnly: ev.Order.ReduceOnly,
				Leverage:   ev.Order.Leverage,
			}
			switch ins.Kind {
			case KindSpot:
				if ev.Order.Side == SideBuy {
					if err := e.port.ApplySpotBuy(ev.Order.AccountID, ins, ev.Fills, fee); err != nil {
						return allEvents, err
					}
				} else {
					if err := e.port.ApplySpotSell(ev.Order.AccountID, ins, ev.Fills, fee); err != nil {
						return allEvents, err
					}
				}
			case KindPerp:
				lev := ev.Order.Leverage
				if lev <= 0 {
					lev = 1
				}
				if err := e.applyPerpFills(ev.Order.AccountID, ins, &req, ev.Fills, fee, lev); err != nil {
					return allEvents, err
				}
			}
			allEvents = append(allEvents, ev)
		}
	}
	return allEvents, nil
}

// CancelOrder cancels a resting order by id.
func (e *SimExchange) CancelOrder(ctx context.Context, accountID string, sym Symbol, orderID string) error {
	return e.CancelOrderAt(ctx, accountID, sym, orderID, e.now())
}

func (e *SimExchange) CancelOrderAt(ctx context.Context, accountID string, sym Symbol, orderID string, ts time.Time) error {
	_ = ctx
	var err error
	accountID, err = normalizeAccountID(accountID)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.getBook(accountID, sym)
	_, ok := b.Cancel(orderID)
	if !ok {
		return ErrOrderNotFound
	}
	_ = ts
	return nil
}

// GetOrder returns a copy of the order if known to the symbol book.
func (e *SimExchange) GetOrder(accountID string, sym Symbol, orderID string) (SimOrder, bool) {
	accountID, err := normalizeAccountID(accountID)
	if err != nil {
		return SimOrder{}, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.getBook(accountID, sym)
	return b.GetOrder(orderID)
}

func (e *SimExchange) ListOpenOrders(accountID string, sym Symbol) []SimOrder {
	accountID, err := normalizeAccountID(accountID)
	if err != nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.getBook(accountID, sym)
	return b.ListOpenOrders()
}

func (e *SimExchange) GetBalances(accountID string) map[Asset]decimal.Decimal {
	accountID, err := normalizeAccountID(accountID)
	if err != nil {
		return nil
	}
	return e.port.Balances(accountID)
}

func (e *SimExchange) GetPosition(accountID string, sym Symbol) (Position, bool) {
	accountID, err := normalizeAccountID(accountID)
	if err != nil {
		return Position{}, false
	}
	return e.port.Position(accountID, sym)
}
