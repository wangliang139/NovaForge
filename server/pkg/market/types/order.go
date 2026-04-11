package types

import (
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// PlaceOrderInput 统一的下单入参（屏蔽交易所差异）。
// - 对于现货：Side 可忽略，仅使用 IsBuy 决定 buy/sell。
// - 对于合约：Side 用于决定 posSide/positionSide（LONG/SHORT），IsBuy 决定 side（BUY/SELL）。
// - Quantity/QuoteQty：现货市价单可支持 quote 计价下单；合约目前仅支持 Quantity。
type PlaceOrderInput struct {
	Symbol ctypes.Symbol

	Side  ctypes.PositionSide
	IsBuy bool

	OrderType ctypes.OrderType

	Price    *decimal.Decimal
	Quantity *decimal.Decimal
	QuoteQty *decimal.Decimal

	// ClientOrderID 透传到交易所（clientOrderId / clOrdId）。
	// 不同交易所对长度/字符集要求不同，这里不做强校验。
	ClientOrderID *ctypes.OrderId

	TimeInForce *ctypes.TimeInForce

	ReduceOnly    *bool
	ClosePosition *bool
}

type PlaceOrderResult struct {
	OrderID       ctypes.OrderId
	ClientOrderID ctypes.OrderId
	Status        ctypes.OrderStatus
}
