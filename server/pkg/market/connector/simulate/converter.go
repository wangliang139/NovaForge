package simulate

import (
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func toSimSide(isBuy bool) Side {
	if isBuy {
		return SideBuy
	}
	return SideSell
}

func toSimIntent(input mdtypes.PlaceOrderInput) ContractIntent {
	if lo.FromPtr(input.ReduceOnly) {
		return IntentClose
	}
	return IntentOpen
}

func marketTypeToInstrumentKind(mt ctypes.MarketType) InstrumentKind {
	if mt == ctypes.MarketTypeFuture {
		return KindPerp
	}
	return KindSpot
}

func placeOrderRequestFromInput(c *Connector, input mdtypes.PlaceOrderInput, market *ctypes.Market) PlaceOrderRequest {
	sym := toPaperSymbol(input.Symbol)
	mode := c.rt.Engine.AccountPositionMode(c.accountID)
	lev := int32(c.rt.Engine.Leverage(c.accountID, sym))
	req := PlaceOrderRequest{
		AccountID:     c.accountID,
		Symbol:        sym,
		OrderType:     OrderTypeMarket,
		Side:          toSimSide(input.IsBuy),
		Intent:        toSimIntent(input),
		ReduceOnly:    lo.FromPtr(input.ReduceOnly),
		Leverage:      lev,
		Price:         decimal.Zero,
		Qty:           *input.Quantity,
		ClientOrderID: string(lo.FromPtr(input.ClientOrderID)),
		Source:        ctypes.OrderSourceUser,
	}
	if input.OrderType == ctypes.OrderTypeLimit {
		req.OrderType = OrderTypeLimit
		if input.Price != nil {
			req.Price = *input.Price
		}
	}
	if market.Symbol.Type == ctypes.MarketTypeFuture && mode == PositionModeHedge {
		req.PosSide = input.Side
	}
	return req
}

func orderFromTypes(c *Connector, od *ctypes.Order, qtyRemaining decimal.Decimal) Order {
	sym := Symbol(od.Symbol.String())
	side := SideSell
	if od.IsBuy {
		side = SideBuy
	}
	intent := IntentOpen
	if od.ReduceOnly {
		intent = IntentClose
	}
	lev := int32(c.rt.Engine.Leverage(c.accountID, sym))
	if lev <= 0 {
		lev = int32(DefaultSimulateLeverage)
	}
	st := OrderStatusNew
	switch od.Status {
	case ctypes.OrderStatusPartialDone:
		st = OrderStatusPartiallyFilled
	}
	var posSide ctypes.PositionSide
	mode := c.rt.Engine.AccountPositionMode(c.accountID)
	if od.Symbol.Type == ctypes.MarketTypeFuture && mode == PositionModeHedge && od.Side.Valid() {
		posSide = od.Side
	}
	now := od.UpdatedTs
	if now.IsZero() {
		now = od.CreatedTs
	}
	src := od.Source
	if src == "" {
		src = ctypes.OrderSourceUser
	}
	simOT := OrderTypeLimit
	if od.OrderType == ctypes.OrderTypeMarket {
		simOT = OrderTypeMarket
	}
	o := Order{
		ID:            string(od.OrderID),
		AccountID:     c.accountID,
		ClientOrderID: string(od.ClientOrderID),
		Symbol:        sym,
		OrderType:     simOT,
		Side:          side,
		Intent:        intent,
		ReduceOnly:    od.ReduceOnly,
		Leverage:      lev,
		PosSide:       posSide,
		Price:         od.Price,
		QtyOriginal:   od.OriginalQty,
		QtyRemaining:  qtyRemaining,
		QtyFilled:     od.ExecutedQty,
		AvgFillPrice:  od.AvgPrice,
		Status:        st,
		CreatedAt:     od.CreatedTs,
		LastUpdatedAt: now,
		RejectReason:  od.RejectReason,
		Source:        src,
	}
	if od.Fee != nil {
		o.FeePaid = *od.Fee
	}
	if od.FeeAsset != nil && *od.FeeAsset != "" {
		o.FeeAsset = *od.FeeAsset
	}
	return o
}

func toTypesSymbol(symbol Symbol) ctypes.Symbol {
	s, _ := ctypes.ParseSymbol(string(symbol))
	return s
}

func toPaperSymbol(symbol ctypes.Symbol) Symbol {
	return Symbol(symbol.String())
}

func toTypesOrderType(tp OrderType) ctypes.OrderType {
	if tp == OrderTypeLimit {
		return ctypes.OrderTypeLimit
	}
	return ctypes.OrderTypeMarket
}

func toTypesOrderStatus(st OrderStatus) ctypes.OrderStatus {
	switch st {
	case OrderStatusNew:
		return ctypes.OrderStatusNew
	case OrderStatusPartiallyFilled:
		return ctypes.OrderStatusPartialDone
	case OrderStatusFilled:
		return ctypes.OrderStatusDone
	case OrderStatusCanceled:
		return ctypes.OrderStatusCanceled
	case OrderStatusRejected:
		return ctypes.OrderStatusRejected
	default:
		return ctypes.OrderStatusPending
	}
}

func toTypesOrder(exchange ctypes.Exchange, od *Order) *ctypes.Order {
	if od == nil {
		return nil
	}
	src := od.Source
	if src == "" {
		src = ctypes.OrderSourceUser
	}
	return &ctypes.Order{
		AccountID:        od.AccountID,
		Exchange:         exchange,
		Symbol:           toTypesSymbol(od.Symbol),
		OrderID:          ctypes.OrderId(od.ID),
		ClientOrderID:    ctypes.OrderId(od.ClientOrderID),
		OrderType:        toTypesOrderType(od.OrderType),
		AlgoType:         ctypes.AlgoTypeNone,
		TimeInForce:      ctypes.TimeInForceGTC,
		IsBuy:            od.Side == SideBuy,
		Price:            od.Price,
		OriginalQty:      od.QtyOriginal,
		ExecutedQty:      od.QtyFilled,
		AvgPrice:         od.AvgFillPrice,
		Status:           toTypesOrderStatus(od.Status),
		CreatedTs:        od.CreatedAt,
		UpdatedTs:        od.LastUpdatedAt,
		RejectReason:     od.RejectReason,
		ExecutedQuoteQty: od.QtyFilled.Mul(od.AvgFillPrice),
		Side:             od.PosSide,
		Source:           src,
		Fee:              &od.FeePaid,
		FeeAsset:         &od.FeeAsset,
	}
}
