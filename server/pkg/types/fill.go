package types

import (
	"time"

	"github.com/shopspring/decimal"
)

// Fill 表示订单的成交事件（账户级别成交）。
//
// 说明：
// - 该事件用于策略系统的 FillSignal/PositionSignal 等强一致链路。
// - trade_id 在不同交易所语义不同；当上游无法提供时，可由系统生成唯一值。
type Fill struct {
	Exchange Exchange `json:"exchange,omitempty"`
	Symbol   Symbol   `json:"symbol,omitempty"`

	AccountID string `json:"accountId,omitempty"`

	OrderID       OrderId `json:"orderId,omitempty"`
	ClientOrderID OrderId `json:"clientOrderId,omitempty"`
	TradeID       string  `json:"tradeId,omitempty"`

	Side  PositionSide `json:"side,omitempty"`
	IsBuy bool         `json:"isBuy,omitempty"`

	Qty   decimal.Decimal `json:"qty,omitempty"`
	Price decimal.Decimal `json:"price,omitempty"`

	// 手续费（正数），实际扣减由资金变更事件处理
	Fee      decimal.Decimal `json:"fee,omitempty"`
	FeeAsset string          `json:"feeAsset,omitempty"`

	// 已实现盈亏（可正可负），通常以 Quote/BaseCurrency 计价
	RealizedPnl decimal.Decimal `json:"realizedPnl,omitempty"`

	IsMaker bool      `json:"isMaker,omitempty"`
	Ts      time.Time `json:"ts,omitempty"`
}

