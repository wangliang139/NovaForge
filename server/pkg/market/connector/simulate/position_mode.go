package simulate

import (
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// PositionMode selects one-way (net) or hedge (long+short legs) perps.
type PositionMode int

const (
	PositionModeOneWay PositionMode = iota
	PositionModeHedge
)

// HedgeOpen returns true if this (position side, buy/sell) combination opens/adds to that leg.
func HedgeOpen(posSide ctypes.PositionSide, isBuy bool) bool {
	switch posSide {
	case ctypes.PositionSideLong:
		return isBuy
	case ctypes.PositionSideShort:
		return !isBuy
	default:
		return false
	}
}

// HedgeClose returns true if this combination closes/reduces that leg.
func HedgeClose(posSide ctypes.PositionSide, isBuy bool) bool {
	return !HedgeOpen(posSide, isBuy)
}

// ValidateHedgeOrder checks reduce-only and opening/closing direction for hedge-mode perp orders.
func ValidateHedgeOrder(posSide ctypes.PositionSide, isBuy bool, reduceOnly bool, legQty decimal.Decimal) error {
	if !posSide.Valid() {
		return ErrInvalidIntent
	}
	isClose := HedgeClose(posSide, isBuy)
	if reduceOnly && !isClose {
		return ErrInvalidIntent
	}
	if !reduceOnly && isClose && legQty.Sign() <= 0 {
		return ErrInvalidIntent
	}
	return nil
}
