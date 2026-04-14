package order

import (
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/internal/consts"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func NormalizeSymbolPrice(price decimal.Decimal, orderType ctypes.OrderType, market ctypes.Market) decimal.Decimal {
	precision := consts.DefaultAssetPrecision
	var tickSize *decimal.Decimal
	rules := getOrderTypeRules(&market, orderType)
	if rules != nil && !rules.TickSize.IsZero() {
		tickSize = &rules.TickSize
	} else if !market.Rules.TickSize.IsZero() {
		tickSize = &market.Rules.TickSize
	}
	if tickSize != nil && !tickSize.IsZero() {
		q := price.Div(*tickSize)
		qFloor := decimal.NewFromInt(q.IntPart())
		price = qFloor.Mul(*tickSize)
	}
	if market.PricePrecision >= 0 {
		precision = market.PricePrecision
	}
	return price.Round(int32(precision))
}

func NormalizeBaseAssetQty(qty decimal.Decimal, orderType ctypes.OrderType, market ctypes.Market) decimal.Decimal {
	precision := consts.DefaultAssetPrecision
	var lotSize *decimal.Decimal
	rules := getOrderTypeRules(&market, orderType)
	if rules != nil && !rules.LotSize.IsZero() {
		lotSize = &rules.LotSize
	} else if !market.Rules.LotSize.IsZero() {
		lotSize = &market.Rules.LotSize
	}
	if lotSize != nil && !lotSize.IsZero() {
		q := qty.Div(*lotSize)
		qFloor := decimal.NewFromInt(q.IntPart())
		qty = qFloor.Mul(*lotSize)
	}
	if market.BaseAssetPrecision >= 0 {
		precision = market.BaseAssetPrecision
	}
	return qty.Round(int32(precision))
}
