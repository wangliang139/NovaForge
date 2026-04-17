package simulate

import (
	"time"

	"github.com/shopspring/decimal"
)

// OrderType is market or limit.
type OrderType int

const (
	OrderTypeMarket OrderType = iota
	OrderTypeLimit
)

// Side is buy or sell (base perspective: buy acquires base).
type Side int

const (
	SideBuy Side = iota + 1
	SideSell
)

// ContractIntent distinguishes open and close on perps.
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

	// Perp only (ignored for spot)
	Intent     ContractIntent
	ReduceOnly bool
	Leverage   int32

	Price decimal.Decimal // limit; ignored for market
	Qty   decimal.Decimal

	ClientOrderID string
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

// SimOrder is a resting or completed simulated order record.
type SimOrder struct {
	ID            string
	AccountID     string
	ClientOrderID string
	Symbol        Symbol

	OrderType OrderType
	Side      Side

	Intent     ContractIntent
	ReduceOnly bool
	Leverage   int32

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

// PlaceOrderResult is returned from SimExchange.PlaceOrder.
type PlaceOrderResult struct {
	Order   SimOrder
	Fills   []Fill
	FeePaid decimal.Decimal // quote asset
}
