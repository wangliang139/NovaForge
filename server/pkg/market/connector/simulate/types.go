package simulate

import (
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// Exchange identifies a venue (opaque string for flexibility).
type Exchange string

// Symbol identifies a market (opaque string for flexibility).
type Symbol string

// OrderBookLevel is one aggregated price level on the L2 book.
type OrderBookLevel struct {
	Price decimal.Decimal `json:"price,omitempty"`
	Size  decimal.Decimal `json:"size,omitempty"`
}

// OrderBook is the wire DTO for both full snapshots and incremental updates.
type OrderBook struct {
	Exchange  Exchange         `json:"exchange,omitempty"`
	Symbol    Symbol           `json:"symbol,omitempty"`
	Bids      []OrderBookLevel `json:"bids,omitempty"`
	Asks      []OrderBookLevel `json:"asks,omitempty"`
	Ts        time.Time        `json:"ts,omitempty"`
	SeqId     int64            `json:"seqId,omitempty"`
	PrevSeqId int64            `json:"prevSeqId,omitempty"`
}

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

// OrderType is market or limit.
type OrderType int

const (
	OrderTypeMarket OrderType = iota
	OrderTypeLimit
)

// Side is buy or sell (base perspective).
type Side int

const (
	SideBuy Side = iota + 1
	SideSell
)

// ContractIntent distinguishes open and close on perps (one-way mode).
type ContractIntent int

const (
	IntentOpen ContractIntent = iota
	IntentClose
)

// PlaceOrderRequest is the unified wire for spot and perp orders.
type PlaceOrderRequest struct {
	AccountID string
	Symbol    Symbol

	OrderType OrderType
	Side      Side

	// One-way perp
	Intent     ContractIntent
	ReduceOnly bool
	Leverage   int32

	// Hedge perp: LONG or SHORT leg (required when account/symbol is in hedge mode).
	PosSide ctypes.PositionSide

	Price decimal.Decimal
	Qty   decimal.Decimal

	ClientOrderID string
	OrderID       string // optional pre-assigned id

	// Source identifies order origin for outbound events (empty => USER in toTypesOrder).
	Source ctypes.OrderSource
}

// OrderStatus for resting / completed orders.
type OrderStatus int

const (
	OrderStatusNew OrderStatus = iota
	OrderStatusPartiallyFilled
	OrderStatusFilled
	OrderStatusCanceled
	OrderStatusRejected
)

// Order is a resting or completed simulated order record.
type Order struct {
	ID            string
	AccountID     string
	ClientOrderID string
	Symbol        Symbol
	OrderType     OrderType
	Side          Side
	Intent        ContractIntent
	ReduceOnly    bool
	Leverage      int32
	PosSide       ctypes.PositionSide
	Price         decimal.Decimal
	QtyOriginal   decimal.Decimal
	QtyRemaining  decimal.Decimal
	QtyFilled     decimal.Decimal
	AvgFillPrice  decimal.Decimal
	// FeePaid：累计手续费支出（负数）；合约/现货卖为 quote；现货买为 base（与扣款资产一致）。
	FeePaid  decimal.Decimal
	FeeAsset string // 手续费币种：合约与现货卖为 quote；现货买为 base
	// RealizedPnl：合约平仓维度累计（同一订单多次撮合含 maker 分批时递加）；开仓为 0；不含手续费；现货不使用。
	RealizedPnl   decimal.Decimal
	Status        OrderStatus
	CreatedAt     time.Time
	LastUpdatedAt time.Time
	RejectReason  string

	Source ctypes.OrderSource
}

// PlaceOrderResult is the outcome of a simulated place; delivered via PlaceOrderCompleteFunc.
type PlaceOrderResult struct {
	Order   Order
	Fills   []Fill
	FeePaid decimal.Decimal
}

// MatchEvent describes a resting order matched against public depth (maker).
type MatchEvent struct {
	Order *Order
	Fills []Fill
}

type AccountEventType int

const (
	AccountEventTypeOrder AccountEventType = iota
	AccountEventTypeBalance
	AccountEventTypePosition
	AccountEventTypeLeverage
)

type LeverageChange struct {
	leverage     int
	leverageSide ctypes.PositionSide
}

type AccountEvent struct {
	accountID string
	symbol    Symbol // paper symbol; required for balance/position payloads (wire Symbol)
	kind      AccountEventType
	order     *Order
	balance   *AccountSnapshot
	position  *PerpSlot
	leverage  *LeverageChange
}
