package simulate

import (
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// InstrumentKind distinguishes spot and perpetual-style contracts.
type InstrumentKind int

const (
	KindSpot InstrumentKind = iota
	KindPerp
)

// Asset identifies a collateral or coin (e.g. "USDT", "BTC").
type Asset string

// Instrument defines trading constraints for a symbol.
type Instrument struct {
	Symbol Symbol
	Kind   InstrumentKind

	Base  Asset
	Quote Asset

	Exchange ctypes.Exchange
	Market   ctypes.MarketType

	PriceTick decimal.Decimal
	QtyStep   decimal.Decimal

	MinQty      decimal.Decimal
	MinNotional decimal.Decimal

	ContractMultiplier decimal.Decimal

	MakerFeeBps int64
	TakerFeeBps int64

	LeverageMax int32
}

// WalletType resolves the wallet bucket for this instrument's balances.
func (ins *Instrument) WalletType() ctypes.WalletType {
	if ins == nil {
		return ctypes.WalletTypeTrade
	}
	if ins.Exchange.IsValid() && ins.Market.Valid() {
		return ctypes.GetWalletType(ins.Exchange, ins.Market)
	}
	switch ins.Kind {
	case KindPerp:
		return ctypes.WalletTypeFuture
	default:
		return ctypes.WalletTypeSpot
	}
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

// ValidateMarketQty checks quantity and estimated notional against reference price.
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
