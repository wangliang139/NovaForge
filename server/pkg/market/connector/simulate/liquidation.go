package simulate

import (
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// LiquidationLossRatio triggers forced close when unrealized PnL <= -ratio * usedInitialMargin.
var LiquidationLossRatio = decimal.RequireFromString("0.9")

// LiquidationEngine evaluates per-leg margin (simplified isolated model).
type LiquidationEngine struct {
	eng *Engine
}

// NewLiquidationEngine creates a liquidation helper bound to an engine.
func NewLiquidationEngine(eng *Engine) *LiquidationEngine {
	return &LiquidationEngine{eng: eng}
}

// OnMark attempts liquidation via reduce-only market orders against public depth (Source=LIQUIDATION).
// If the market order is rejected for lack of liquidity, falls back to synthetic close at mark.
// Returns one *PlaceOrderResult per liquidation action (may be empty).
func (l *LiquidationEngine) OnMark(accountID string, sym Symbol, mark decimal.Decimal, onLiquidated func()) []*PlaceOrderResult {
	if l == nil || l.eng == nil || !mark.GreaterThan(decimal.Zero) {
		return nil
	}
	mode := l.eng.AccountPositionMode(accountID)
	if mode == PositionModeHedge {
		var out []*PlaceOrderResult
		if r := l.tryHedgeLeg(accountID, sym, ctypes.PositionSideLong, mark, onLiquidated); r != nil {
			out = append(out, r)
		}
		if r := l.tryHedgeLeg(accountID, sym, ctypes.PositionSideShort, mark, onLiquidated); r != nil {
			out = append(out, r)
		}
		return out
	}
	r := l.tryOneWay(accountID, sym, mark, onLiquidated)
	if r == nil {
		return nil
	}
	return []*PlaceOrderResult{r}
}

func (l *LiquidationEngine) tryOneWay(accountID string, sym Symbol, mark decimal.Decimal, onLiquidated func()) *PlaceOrderResult {
	pos, ok := l.eng.NetPosition(accountID, sym)
	if !ok || pos.Qty.IsZero() {
		return nil
	}
	var upnl decimal.Decimal
	if pos.Qty.Sign() > 0 {
		upnl = mark.Sub(pos.EntryPrice).Mul(pos.Qty)
	} else {
		upnl = pos.EntryPrice.Sub(mark).Mul(pos.Qty.Abs())
	}
	trigger := pos.UsedMargin.Mul(LiquidationLossRatio).Neg()
	if upnl.GreaterThan(trigger) {
		return nil
	}
	lev := pos.Leverage
	if lev <= 0 {
		lev = 1
	}
	qtyAbs := pos.Qty.Abs()
	side := SideSell
	if pos.Qty.Sign() < 0 {
		side = SideBuy
	}
	req := PlaceOrderRequest{
		AccountID:     accountID,
		Symbol:        sym,
		OrderType:     OrderTypeMarket,
		Side:          side,
		Intent:        IntentClose,
		ReduceOnly:    true,
		Leverage:      lev,
		Qty:           qtyAbs,
		ClientOrderID: "liq",
		OrderID:       GenerateCompactID(accountID),
		Source:        ctypes.OrderSourceLiquidation,
	}
	res := l.eng.PlaceLiquidationMarket(req)
	if res != nil && res.Order.Status == OrderStatusRejected && res.Order.RejectReason == "no liquidity" {
		return l.eng.forceCloseOneWayAtMarkSynthetic(accountID, sym, mark)
	}
	if res != nil && onLiquidated != nil {
		onLiquidated()
	}
	return res
}

func (l *LiquidationEngine) tryHedgeLeg(accountID string, sym Symbol, side ctypes.PositionSide, mark decimal.Decimal, onLiquidated func()) *PlaceOrderResult {
	slot, ok := l.eng.Ledger().GetPerpSlot(accountID, sym)
	if !ok || slot.Mode != PositionModeHedge {
		return nil
	}
	var qty, entry, used decimal.Decimal
	switch side {
	case ctypes.PositionSideLong:
		qty, entry, used = slot.Long.Qty, slot.Long.EntryPrice, slot.Long.UsedMargin
	case ctypes.PositionSideShort:
		qty, entry, used = slot.Short.Qty, slot.Short.EntryPrice, slot.Short.UsedMargin
	default:
		return nil
	}
	if qty.IsZero() || used.IsZero() {
		return nil
	}
	var upnl decimal.Decimal
	switch side {
	case ctypes.PositionSideLong:
		upnl = mark.Sub(entry).Mul(qty)
	case ctypes.PositionSideShort:
		upnl = entry.Sub(mark).Mul(qty)
	}
	trigger := used.Mul(LiquidationLossRatio).Neg()
	if upnl.GreaterThan(trigger) {
		return nil
	}
	var simSide Side
	var lev int32
	switch side {
	case ctypes.PositionSideLong:
		simSide = SideSell
		lev = slot.Long.Leverage
	case ctypes.PositionSideShort:
		simSide = SideBuy
		lev = slot.Short.Leverage
	}
	if lev <= 0 {
		lev = 1
	}
	req := PlaceOrderRequest{
		AccountID:     accountID,
		Symbol:        sym,
		OrderType:     OrderTypeMarket,
		Side:          simSide,
		Intent:        IntentClose,
		ReduceOnly:    true,
		Leverage:      lev,
		PosSide:       side,
		Qty:           qty,
		ClientOrderID: "liq",
		OrderID:       GenerateCompactID(accountID),
		Source:        ctypes.OrderSourceLiquidation,
	}
	res := l.eng.PlaceLiquidationMarket(req)
	if res != nil && res.Order.Status == OrderStatusRejected && res.Order.RejectReason == "no liquidity" {
		return l.eng.forceCloseHedgeLegAtMarkSynthetic(accountID, sym, side, mark)
	}
	if res != nil && onLiquidated != nil {
		onLiquidated()
	}
	return res
}
