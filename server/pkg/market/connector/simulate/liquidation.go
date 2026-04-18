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

// OnMark tries to liquidate positions for account+symbol at mark.
func (l *LiquidationEngine) OnMark(accountID string, sym Symbol, mark decimal.Decimal, onLiquidated func()) {
	if l == nil || l.eng == nil || !mark.GreaterThan(decimal.Zero) {
		return
	}
	mode := l.eng.AccountPositionMode(accountID)
	if mode == PositionModeHedge {
		l.tryLeg(accountID, sym, ctypes.PositionSideLong, mark, onLiquidated)
		l.tryLeg(accountID, sym, ctypes.PositionSideShort, mark, onLiquidated)
		return
	}
	pos, ok := l.eng.NetPosition(accountID, sym)
	if !ok || pos.Qty.IsZero() {
		return
	}
	var upnl decimal.Decimal
	if pos.Qty.Sign() > 0 {
		upnl = mark.Sub(pos.EntryPrice).Mul(pos.Qty)
	} else {
		upnl = pos.EntryPrice.Sub(mark).Mul(pos.Qty.Abs())
	}
	trigger := pos.UsedMargin.Mul(LiquidationLossRatio).Neg()
	if upnl.GreaterThan(trigger) {
		return
	}
	if err := l.eng.ForceClosePerpAtMark(accountID, sym, mark); err == nil && onLiquidated != nil {
		onLiquidated()
	}
}

func (l *LiquidationEngine) tryLeg(accountID string, sym Symbol, side ctypes.PositionSide, mark decimal.Decimal, onLiquidated func()) {
	slot, ok := l.eng.Ledger().GetPerpSlot(accountID, sym)
	if !ok || slot.Mode != PositionModeHedge {
		return
	}
	var qty, entry, used decimal.Decimal
	switch side {
	case ctypes.PositionSideLong:
		qty, entry, used = slot.Long.Qty, slot.Long.EntryPrice, slot.Long.UsedMargin
	case ctypes.PositionSideShort:
		qty, entry, used = slot.Short.Qty, slot.Short.EntryPrice, slot.Short.UsedMargin
	default:
		return
	}
	if qty.IsZero() || used.IsZero() {
		return
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
		return
	}
	if err := l.eng.ForceCloseHedgeLegAtMark(accountID, sym, side, mark); err == nil && onLiquidated != nil {
		onLiquidated()
	}
}
