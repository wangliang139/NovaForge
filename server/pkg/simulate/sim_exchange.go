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

	ins     map[Symbol]*Instrument
	depths  map[Symbol]*MarketDepth
	books   map[Symbol]*SimBook
	port    *Portfolio

	nextOrderID int64
}

// NewSimExchange creates an exchange with an empty portfolio.
func NewSimExchange() *SimExchange {
	return &SimExchange{
		ins:    make(map[Symbol]*Instrument),
		depths: make(map[Symbol]*MarketDepth),
		books:  make(map[Symbol]*SimBook),
		port:   NewPortfolio(),
	}
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
	if _, ok := e.books[ins.Symbol]; !ok {
		e.books[ins.Symbol] = NewSimBook(ins.Symbol)
	}
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
	if e.books[sym] == nil {
		e.books[sym] = NewSimBook(sym)
	}
	return nil
}

func (e *SimExchange) genOrderID() string {
	n := atomic.AddInt64(&e.nextOrderID, 1)
	return fmt.Sprintf("o%d", n)
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
	book := e.books[req.Symbol]

	pos, _ := e.port.Position(req.Symbol)
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

	now := time.Now().UTC()
	order := &SimOrder{
		ID:            e.genOrderID(),
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
				if err := e.port.ApplySpotBuy(ins, fills, fee); err != nil {
					return nil, err
				}
			} else {
				if err := e.port.ApplySpotSell(ins, fills, fee); err != nil {
					return nil, err
				}
			}
			allFills = fills
			feeSum = fee
			fillOrderImmediate(order, fills, notional, filledQty)

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
					if err := e.port.ApplySpotBuy(ins, fills, fee); err != nil {
						return nil, err
					}
				} else {
					if err := e.port.ApplySpotSell(ins, fills, fee); err != nil {
						return nil, err
					}
				}
				allFills = append(allFills, fills...)
				feeSum = feeSum.Add(fee)
				partialFillOrder(order, fills, notional, filledQty)
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
			if err := e.applyPerpFills(ins, &req, fills, fee, lev); err != nil {
				return nil, err
			}
			allFills = fills
			feeSum = fee
			fillOrderImmediate(order, fills, notional, filledQty)

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
				if err := e.applyPerpFills(ins, &req, fills, fee, lev); err != nil {
					return nil, err
				}
				allFills = append(allFills, fills...)
				feeSum = feeSum.Add(fee)
				partialFillOrder(order, fills, notional, filledQty)
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

func fillOrderImmediate(o *SimOrder, fills []Fill, notional, filledQty decimal.Decimal) {
	o.QtyFilled = filledQty
	o.QtyRemaining = decimal.Zero
	o.AvgFillPrice = AveragePrice(notional, filledQty)
	o.Status = OrderStatusFilled
	o.LastUpdatedAt = time.Now().UTC()
}

func partialFillOrder(o *SimOrder, fills []Fill, notional, filledQty decimal.Decimal) {
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
	o.LastUpdatedAt = time.Now().UTC()
}

func (e *SimExchange) applyPerpFills(ins *Instrument, req *PlaceOrderRequest, fills []Fill, fee decimal.Decimal, lev int32) error {
	switch req.Intent {
	case IntentOpen:
		if req.Side == SideBuy {
			return e.port.ApplyPerpOpenLong(req.Symbol, ins, fills, fee, lev)
		}
		return e.port.ApplyPerpOpenShort(req.Symbol, ins, fills, fee, lev)
	case IntentClose:
		if req.Side == SideSell {
			return e.port.ApplyPerpCloseLong(req.Symbol, ins, fills, fee)
		}
		return e.port.ApplyPerpCloseShort(req.Symbol, ins, fills, fee)
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
	book := e.books[sym]
	events := book.OnDepth(d)
	for _, ev := range events {
		var notional decimal.Decimal
		for _, f := range ev.Fills {
			notional = notional.Add(f.Price.Mul(f.Size))
		}
		fee := FeeNotional(notional, ins.MakerFeeBps)
		req := PlaceOrderRequest{
			Symbol:     sym,
			Side:       ev.Order.Side,
			Intent:     ev.Order.Intent,
			ReduceOnly: ev.Order.ReduceOnly,
			Leverage:   ev.Order.Leverage,
		}
		switch ins.Kind {
		case KindSpot:
			if ev.Order.Side == SideBuy {
				if err := e.port.ApplySpotBuy(ins, ev.Fills, fee); err != nil {
					return events, err
				}
			} else {
				if err := e.port.ApplySpotSell(ins, ev.Fills, fee); err != nil {
					return events, err
				}
			}
		case KindPerp:
			lev := ev.Order.Leverage
			if lev <= 0 {
				lev = 1
			}
			if err := e.applyPerpFills(ins, &req, ev.Fills, fee, lev); err != nil {
				return events, err
			}
		}
	}
	return events, nil
}

// CancelOrder cancels a resting order by id.
func (e *SimExchange) CancelOrder(ctx context.Context, sym Symbol, orderID string) error {
	_ = ctx
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.books[sym]
	if b == nil {
		return ErrOrderNotFound
	}
	_, ok := b.Cancel(orderID)
	if !ok {
		return ErrOrderNotFound
	}
	return nil
}

// GetOrder returns a copy of the order if known to the symbol book.
func (e *SimExchange) GetOrder(sym Symbol, orderID string) (SimOrder, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.books[sym]
	if b == nil {
		return SimOrder{}, false
	}
	return b.GetOrder(orderID)
}

// ListOpenOrders lists resting orders for a symbol.
func (e *SimExchange) ListOpenOrders(sym Symbol) []SimOrder {
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.books[sym]
	if b == nil {
		return nil
	}
	return b.ListOpenOrders()
}

// GetBalances snapshots spot balances.
func (e *SimExchange) GetBalances() map[Asset]decimal.Decimal {
	return e.port.Balances()
}

// GetPosition returns perp position for symbol.
func (e *SimExchange) GetPosition(sym Symbol) (Position, bool) {
	return e.port.Position(sym)
}
