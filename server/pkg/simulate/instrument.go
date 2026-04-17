package simulate

import (
	"github.com/shopspring/decimal"
)

// InstrumentKind distinguishes spot and perpetual-style contracts.
type InstrumentKind int

const (
	KindSpot InstrumentKind = iota
	KindPerp
)

// Asset identifies a collateral or coin (e.g. "USDT", "BTC").
type Asset string

// Instrument defines trading constraints for a symbol (precision, fees, kind).
type Instrument struct {
	Symbol Symbol

	Kind InstrumentKind

	Base  Asset
	Quote Asset

	PriceTick decimal.Decimal
	QtyStep   decimal.Decimal

	MinQty      decimal.Decimal
	MinNotional decimal.Decimal

	// ContractMultiplier converts contract size to base units; 1 for coin-margined per coin.
	ContractMultiplier decimal.Decimal

	// MakerFeeBps / TakerFeeBps are fee rates in basis points (1 bps = 0.01%).
	MakerFeeBps int64
	TakerFeeBps int64

	// LeverageMax caps leverage on perp opens (e.g. 125). Ignored for spot.
	LeverageMax int32
}

// DefaultContractMultiplier returns 1 if m is zero.
func DefaultContractMultiplier(m decimal.Decimal) decimal.Decimal {
	if m.IsZero() {
		return decimal.NewFromInt(1)
	}
	return m
}

// FloorToTick rounds price down to the nearest PriceTick multiple.
func FloorToTick(price, tick decimal.Decimal) decimal.Decimal {
	if tick.IsZero() || tick.Sign() <= 0 {
		return price
	}
	q := price.Div(tick).Floor()
	return q.Mul(tick)
}

// FloorToStep rounds quantity down to the nearest QtyStep multiple.
func FloorToStep(qty, step decimal.Decimal) decimal.Decimal {
	if step.IsZero() || step.Sign() <= 0 {
		return qty
	}
	q := qty.Div(step).Floor()
	return q.Mul(step)
}

// NormalizePriceQty applies instrument rounding (floor to tick/step).
func (ins *Instrument) NormalizePriceQty(price, qty decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	return FloorToTick(price, ins.PriceTick), FloorToStep(qty, ins.QtyStep)
}

// BaseQtyFromContracts returns base quantity for a contract count.
func (ins *Instrument) BaseQtyFromContracts(contracts decimal.Decimal) decimal.Decimal {
	m := DefaultContractMultiplier(ins.ContractMultiplier)
	return FloorToStep(contracts.Mul(m), ins.QtyStep)
}

// ValidateOrderParams checks positive qty and min qty / notional where applicable.
func (ins *Instrument) ValidateOrderParams(price, qty decimal.Decimal, isLimit bool) error {
	if qty.Sign() <= 0 {
		return ErrInvalidQty
	}
	if qty.LessThan(ins.MinQty) {
		return ErrBelowMinQty
	}
	if isLimit && price.Sign() <= 0 {
		return ErrInvalidPrice
	}
	if isLimit {
		n := price.Mul(qty)
		if n.LessThan(ins.MinNotional) {
			return ErrBelowMinNotional
		}
	}
	return nil
}

// ValidateMarketQty checks quantity and estimated notional against reference price (e.g. mid).
func (ins *Instrument) ValidateMarketQty(qty, refPrice decimal.Decimal) error {
	if qty.Sign() <= 0 {
		return ErrInvalidQty
	}
	if qty.LessThan(ins.MinQty) {
		return ErrBelowMinQty
	}
	if refPrice.Sign() > 0 {
		if qty.Mul(refPrice).LessThan(ins.MinNotional) {
			return ErrBelowMinNotional
		}
	}
	return nil
}

// FeeNotional returns fee charged on quote notional at given bps.
func FeeNotional(notional decimal.Decimal, feeBps int64) decimal.Decimal {
	if feeBps == 0 || notional.Sign() == 0 {
		return decimal.Zero
	}
	return notional.Mul(decimal.NewFromInt(feeBps)).Div(decimal.NewFromInt(10000))
}
