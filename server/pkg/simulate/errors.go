package simulate

import "errors"

var (
	// ErrSeqGap means delta.PrevSeqId does not match the last applied sequence id.
	ErrSeqGap = errors.New("market: sequence gap (prev seq mismatch)")
	// ErrNotInitialized is returned when ApplyDelta is called before a successful ApplySnapshot.
	ErrNotInitialized = errors.New("market: depth not initialized (apply snapshot first)")

	ErrUnknownSymbol       = errors.New("market: unknown symbol")
	ErrInvalidQty          = errors.New("market: invalid quantity")
	ErrInvalidPrice        = errors.New("market: invalid price")
	ErrBelowMinQty         = errors.New("market: below min quantity")
	ErrBelowMinNotional    = errors.New("market: below min notional")
	ErrInsufficientBalance = errors.New("market: insufficient balance")
	ErrReduceOnly          = errors.New("market: reduce-only violation")
	ErrInvalidIntent       = errors.New("market: invalid open/close for side and position")
	ErrLeverage            = errors.New("market: leverage out of range or insufficient margin")
	ErrOrderNotFound       = errors.New("market: order not found")
	ErrInstrumentKind      = errors.New("market: instrument kind mismatch")
	ErrInvalidAccount      = errors.New("market: invalid account id")
)
