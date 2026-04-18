package simulate

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// SeedAccountBalances replaces wallet balances from persisted assets (virtual accounts).
func (c *Connector) SeedAccountBalances(bals map[ctypes.WalletType]map[Asset]decimal.Decimal) error {
	c.rt.Engine.InitBalances(c.accountID, bals)
	return nil
}

// SeedAccountPositions seeds perp slots from DB snapshots (one-way net or hedge legs).
func (c *Connector) SeedAccountPositions(posMap map[ctypes.Symbol]ctypes.Position) error {
	mode := c.rt.Engine.AccountPositionMode(c.accountID)
	for sym, p := range posMap {
		if !sym.IsValid() || sym.Type != ctypes.MarketTypeFuture {
			continue
		}
		psym := Symbol(sym.String())
		lev := int32(p.Leverage)
		if lev <= 0 {
			lev = int32(c.rt.Engine.Leverage(c.accountID, psym))
		}
		if lev <= 0 {
			lev = 1
		}
		if mode == PositionModeHedge {
			if !p.Side.Valid() {
				continue
			}
			leg := PerpLeg{
				Qty:        p.Amount,
				EntryPrice: p.EntryPrice,
				UsedMargin: p.InitialMargin,
				Leverage:   lev,
			}
			c.rt.Engine.SeedLedgerHedgeLeg(c.accountID, psym, p.Side, leg)
			continue
		}
		var qty decimal.Decimal
		switch p.Side {
		case ctypes.PositionSideLong:
			qty = p.Amount
		case ctypes.PositionSideShort:
			qty = p.Amount.Neg()
		default:
			continue
		}
		if qty.IsZero() {
			continue
		}
		net := Position{
			Qty:        qty,
			EntryPrice: p.EntryPrice,
			UsedMargin: p.InitialMargin,
			Leverage:   lev,
		}
		c.rt.Engine.SeedLedgerOneWayNet(c.accountID, psym, net)
	}
	return nil
}

// SeedOpenOrders hydrates resting limit orders from DB open orders.
func (c *Connector) SeedOpenOrders(orders []*ctypes.Order) error {
	ctx := context.Background()
	for _, od := range orders {
		if od == nil || !od.Symbol.IsValid() {
			continue
		}
		if od.OrderType != ctypes.OrderTypeLimit {
			continue
		}
		switch od.Status {
		case ctypes.OrderStatusNew, ctypes.OrderStatusPartialDone, ctypes.OrderStatusWorking, ctypes.OrderStatusPending:
		default:
			continue
		}
		rem := od.OriginalQty.Sub(od.ExecutedQty)
		if rem.Sign() <= 0 {
			continue
		}
		market, err := c.GetMarket(ctx, od.Symbol)
		if err != nil {
			return fmt.Errorf("simulate: seed order market %s: %w", od.Symbol, err)
		}
		if market == nil {
			return fmt.Errorf("simulate: seed order unknown market %s", od.Symbol)
		}
		c.ensureInstrument(market)
		po := paperOrderFromTypes(c, od, rem)
		if err := c.rt.Engine.SeedOpenOrder(c.accountID, po); err != nil {
			return err
		}
	}
	return nil
}

// WarmSymbols ensures instrument registration and depth wiring for the given symbols.
func (c *Connector) WarmSymbols(ctx context.Context, symbols []ctypes.Symbol) {
	for _, sym := range symbols {
		if !sym.IsValid() {
			continue
		}
		_ = c.ensureSymbolInitialized(ctx, sym)
	}
}

func paperOrderFromTypes(c *Connector, od *ctypes.Order, qtyRemaining decimal.Decimal) Order {
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
		lev = 1
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
	return Order{
		ID:            string(od.OrderID),
		AccountID:     c.accountID,
		ClientOrderID: string(od.ClientOrderID),
		Symbol:        sym,
		OrderType:     OrderTypeLimit,
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
	}
}
