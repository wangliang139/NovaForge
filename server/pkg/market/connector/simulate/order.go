package simulate

import (
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

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
	Status        OrderStatus
	CreatedAt     time.Time
	LastUpdatedAt time.Time
	RejectReason  string
}

// PlaceOrderResult is returned from Engine.PlaceOrder.
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
