package simulate

import "errors"

var (
	ErrSeqGap              = errors.New("simulate: sequence gap")
	ErrNotInitialized      = errors.New("simulate: depth not initialized")
	ErrUnknownSymbol       = errors.New("simulate: unknown symbol")
	ErrInvalidQty          = errors.New("simulate: invalid quantity")
	ErrInvalidPrice        = errors.New("simulate: invalid price")
	ErrBelowMinQty         = errors.New("simulate: below min quantity")
	ErrBelowMinNotional    = errors.New("simulate: below min notional")
	ErrInsufficientBalance = errors.New("simulate: insufficient balance")
	ErrReduceOnly          = errors.New("simulate: reduce-only violation")
	ErrInvalidIntent       = errors.New("simulate: invalid open/close for side and position")
	ErrLeverage            = errors.New("simulate: leverage out of range")
	ErrOrderNotFound       = errors.New("simulate: order not found")
	ErrInvalidAccount      = errors.New("simulate: invalid account id")
	ErrPositionMode        = errors.New("simulate: position mode conflict")
	ErrQueueFull           = errors.New("simulate: order queue full")
)
