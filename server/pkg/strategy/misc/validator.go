package misc

import (
	"fmt"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// ValidateMarketStatus 校验市场状态是否为 TRADING
func ValidateMarketStatus(market *ctypes.Market) error {
	if market == nil {
		return fmt.Errorf("market is nil")
	}
	if market.Status != ctypes.MarketStatusTrading {
		return fmt.Errorf("market status is %s, expected TRADING", market.Status)
	}
	return nil
}

// ValidateOrderType 校验订单类型是否支持
func ValidateOrderType(market *ctypes.Market, orderType ctypes.OrderType) error {
	if market == nil {
		return fmt.Errorf("market is nil")
	}
	// 如果 SupportOrderTypes 为空，假设支持所有类型（向后兼容）
	if len(market.OrderTypeRules) == 0 {
		return nil
	}
	// 检查 SupportOrderTypes 中是否包含该订单类型
	for _, ot := range market.OrderTypeRules {
		if ot.OrderType == orderType {
			return nil
		}
	}
	return fmt.Errorf("order type %s is not supported", orderType)
}

// GetOrderTypeRules 获取订单类型对应的规则（分层聚合）。
// 具体订单类型的规则优先于通用规则；若订单类型规则中某参数为空/零值，则用通用 Market.Rules 中对应参数填充。
func GetOrderTypeRules(market *ctypes.Market, orderType ctypes.OrderType) *ctypes.MarketRules {
	if market == nil {
		return nil
	}
	generic := &market.Rules
	var specific *ctypes.MarketRules
	for _, ot := range market.OrderTypeRules {
		if ot.OrderType == orderType {
			specific = &ot.Rules
			break
		}
	}
	if specific == nil {
		return generic
	}
	return mergeMarketRules(specific, generic)
}

// mergeMarketRules 合并规则：以 base 为准，base 中为空/零值的字段用 fallback 填充。
func mergeMarketRules(base, fallback *ctypes.MarketRules) *ctypes.MarketRules {
	out := &ctypes.MarketRules{}
	// MaxOrderNum
	if base.MaxOrderNum > 0 {
		out.MaxOrderNum = base.MaxOrderNum
	} else {
		out.MaxOrderNum = fallback.MaxOrderNum
	}
	// MinPrice, MaxPrice, TickSize
	out.MinPrice = pickDecimal(base.MinPrice, fallback.MinPrice)
	out.MaxPrice = pickDecimal(base.MaxPrice, fallback.MaxPrice)
	out.TickSize = pickDecimal(base.TickSize, fallback.TickSize)
	// MinQuantity, MaxQuantity, LotSize
	out.MinQuantity = pickDecimal(base.MinQuantity, fallback.MinQuantity)
	out.MaxQuantity = pickDecimal(base.MaxQuantity, fallback.MaxQuantity)
	out.LotSize = pickDecimal(base.LotSize, fallback.LotSize)
	// MinNotional, MaxNotional
	out.MinNotional = pickDecimal(base.MinNotional, fallback.MinNotional)
	out.MaxNotional = pickDecimal(base.MaxNotional, fallback.MaxNotional)
	return out
}

func pickDecimal(primary, fallback decimal.Decimal) decimal.Decimal {
	if !primary.IsZero() {
		return primary
	}
	return fallback
}

// ValidateMarketFilters 校验市场过滤器
// openOrderCount 是当前该标的的挂单数量（NEW 和 PARTIAL_DONE 状态）
func ValidateMarketFilters(market *ctypes.Market, orderType ctypes.OrderType, price *decimal.Decimal, qty decimal.Decimal, openOrderCount int) error {
	if market == nil {
		return fmt.Errorf("market is nil")
	}

	// 获取订单类型对应的规则
	rules := GetOrderTypeRules(market, orderType)
	if rules == nil {
		return fmt.Errorf("no rules found for order type %s", orderType)
	}

	// 价格校验（仅对限价单）
	if orderType == ctypes.OrderTypeLimit {
		if price == nil {
			return fmt.Errorf("price is required for limit order")
		}
		adjustedPrice := NormalizeSymbolPrice(*price, orderType, market)
		if !adjustedPrice.Equal(*price) {
			return fmt.Errorf("price %s is not a multiple of tick size %s", price.String(), rules.TickSize.String())
		}
		if !rules.MinPrice.IsZero() && price.LessThan(rules.MinPrice) {
			return fmt.Errorf("price %s is less than min price %s", price.String(), rules.MinPrice.String())
		}
		if !rules.MaxPrice.IsZero() && price.GreaterThan(rules.MaxPrice) {
			return fmt.Errorf("price %s is greater than max price %s", price.String(), rules.MaxPrice.String())
		}
	}

	// 数量范围校验
	if !rules.MinQuantity.IsZero() && qty.LessThan(rules.MinQuantity) {
		return fmt.Errorf("quantity %s is less than min quantity %s", qty.String(), rules.MinQuantity.String())
	}
	if !rules.MaxQuantity.IsZero() && qty.GreaterThan(rules.MaxQuantity) {
		return fmt.Errorf("quantity %s is greater than max quantity %s", qty.String(), rules.MaxQuantity.String())
	}

	// 数量步长校验（调整数量到 LotSize 的倍数）
	if !rules.LotSize.IsZero() {
		adjustedQty := NormalizeBaseAssetQty(qty, orderType, market)
		if !adjustedQty.Equal(qty) {
			return fmt.Errorf("quantity %s is not a multiple of lot size %s", qty.String(), rules.LotSize.String())
		}
	}

	// 名义价值校验
	if price != nil && price.GreaterThan(decimal.Zero) {
		notional := price.Mul(qty)
		if !rules.MinNotional.IsZero() && notional.LessThan(rules.MinNotional) {
			return fmt.Errorf("notional %s is less than min notional %s", notional.String(), rules.MinNotional.String())
		}
		if !rules.MaxNotional.IsZero() && notional.GreaterThan(rules.MaxNotional) {
			return fmt.Errorf("notional %s is greater than max notional %s", notional.String(), rules.MaxNotional.String())
		}
	}

	// 最大挂单数校验（使用通用规则）
	if market.Rules.MaxOrderNum > 0 {
		if openOrderCount >= market.Rules.MaxOrderNum {
			return fmt.Errorf("open order count %d exceeds max order num %d", openOrderCount, market.Rules.MaxOrderNum)
		}
	}

	return nil
}
