package simulate

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func ValidatePlaceOrderInputBasic(input mdtypes.PlaceOrderInput) error {
	if !input.Symbol.IsValid() {
		return fmt.Errorf("simulate: invalid symbol")
	}
	if input.Quantity == nil || input.Quantity.Sign() <= 0 {
		return fmt.Errorf("simulate: invalid quantity")
	}
	if input.OrderType == ctypes.OrderTypeLimit && (input.Price == nil || input.Price.Sign() <= 0) {
		return fmt.Errorf("simulate: limit requires price")
	}
	return nil
}

func ValidatePlaceOrderByMarketRules(ctx context.Context, input mdtypes.PlaceOrderInput, market *ctypes.Market) error {
	_ = ctx
	rules := market.Rules
	if input.OrderType == ctypes.OrderTypeLimit {
		if err := validatePriceRules(*input.Price, rules); err != nil {
			return err
		}
	}
	if err := validateQuantityRules(*input.Quantity, rules); err != nil {
		return err
	}
	if input.OrderType == ctypes.OrderTypeLimit {
		if err := validateNotionalRules(*input.Price, *input.Quantity, rules); err != nil {
			return err
		}
	}
	return nil
}

func validatePriceRules(price decimal.Decimal, rules ctypes.MarketRules) error {
	if rules.TickSize.Sign() > 0 && !isStepAligned(price, rules.TickSize) {
		return ErrInvalidPrice
	}
	return nil
}

func validateQuantityRules(qty decimal.Decimal, rules ctypes.MarketRules) error {
	if qty.LessThan(rules.MinQuantity) {
		return ErrBelowMinQty
	}
	if rules.LotSize.Sign() > 0 && !isStepAligned(qty, rules.LotSize) {
		return ErrInvalidQty
	}
	return nil
}

func validateNotionalRules(price, qty decimal.Decimal, rules ctypes.MarketRules) error {
	if price.Mul(qty).LessThan(rules.MinNotional) {
		return ErrBelowMinNotional
	}
	return nil
}

func isStepAligned(v, step decimal.Decimal) bool {
	if step.IsZero() {
		return true
	}
	q := v.Div(step)
	return q.Equal(q.Truncate(0))
}
