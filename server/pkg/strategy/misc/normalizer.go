package misc

import (
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/internal/consts"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func NormalizeSymbolPrice(price decimal.Decimal, orderType ctypes.OrderType, market *ctypes.Market) decimal.Decimal {
	precision := consts.DefaultAssetPrecision
	if market == nil {
		return price.Round(int32(precision))
	}

	// 先按照 TickSize 归一化
	var tickSize *decimal.Decimal
	rules := getRulesByOrderType(orderType, market)
	if rules != nil && !rules.TickSize.IsZero() {
		tickSize = &rules.TickSize
	} else {
		rules = &market.Rules
		if !rules.TickSize.IsZero() {
			tickSize = &rules.TickSize
		}
	}
	if tickSize != nil && !tickSize.IsZero() {
		// price = floor(price / tickSize) * tickSize
		quotient := price.Div(*tickSize)
		quotientFloor := decimal.NewFromInt(quotient.IntPart())
		price = quotientFloor.Mul(*tickSize)
	}

	// 再按照 PricePrecision 归一化
	if market.PricePrecision >= 0 {
		precision = market.PricePrecision
	}

	return price.Round(int32(precision))
}

func NormalizeBaseAssetQty(qty decimal.Decimal, orderType ctypes.OrderType, market *ctypes.Market) decimal.Decimal {
	precision := consts.DefaultAssetPrecision
	if market == nil {
		return qty.Round(int32(precision))
	}

	// 先按照 LotSize 归一化
	var lotSize *decimal.Decimal
	rules := getRulesByOrderType(orderType, market)
	if rules != nil && !rules.LotSize.IsZero() {
		lotSize = &rules.LotSize
	} else {
		rules = &market.Rules
		if !rules.LotSize.IsZero() {
			lotSize = &rules.LotSize
		}
	}
	if lotSize != nil && !lotSize.IsZero() {
		// qty = floor(qty / lotSize) * lotSize
		quotient := qty.Div(*lotSize)
		quotientFloor := decimal.NewFromInt(quotient.IntPart())
		qty = quotientFloor.Mul(*lotSize)
	}

	// 再按照 BaseAssetPrecision 归一化
	if market.BaseAssetPrecision >= 0 {
		precision = market.BaseAssetPrecision
	}

	return qty.Round(int32(precision))
}

// NormalizeQuoteAssetQty 根据 market 的 QuoteAssetPrecision 归一化报价资产数量（金额）。
func NormalizeQuoteAssetQty(qty decimal.Decimal, market *ctypes.Market) decimal.Decimal {
	precision := consts.DefaultAssetPrecision
	if market == nil {
		return qty.Round(int32(precision))
	}
	if market.QuoteAssetPrecision >= 0 {
		precision = market.QuoteAssetPrecision
	}
	return qty.Round(int32(precision))
}

func getRulesByOrderType(orderType ctypes.OrderType, market *ctypes.Market) *ctypes.MarketRules {
	if market == nil {
		return nil
	}
	for _, ot := range market.OrderTypeRules {
		if ot.OrderType == orderType {
			return &ot.Rules
		}
	}
	return nil
}
