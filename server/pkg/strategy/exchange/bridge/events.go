package bridge

import (
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

// ExchangeEvent 撮合引擎输出的“交易所语义事件”（不实现 strategy/types.Signal，避免混淆语义层次）。
type ExchangeEvent interface {
	GetTimestamp() time.Time
}

// ExchangeEventKind 交易所语义事件种类
type ExchangeEventKind string

const (
	ExchangeEventKindOrderAccepted ExchangeEventKind = "order_accepted"
	ExchangeEventKindOrderRejected ExchangeEventKind = "order_rejected"
	ExchangeEventKindOrderCanceled ExchangeEventKind = "order_canceled"
	ExchangeEventKindOrderExpired  ExchangeEventKind = "order_expired"
	ExchangeEventKindOrderDone     ExchangeEventKind = "order_done"

	ExchangeEventKindFill ExchangeEventKind = "fill"
)

// OrderEvent 订单状态变化（交易所语义）
//
// 说明：
// - ExchangeOrderID：撮合/交易所侧订单ID（可能为空，例如下单校验失败直接拒单）
// - ClientOrderID：策略侧订单ID（通常不为空）
type OrderEvent struct {
	Kind ExchangeEventKind
	Ts   time.Time

	Exchange  ctypes.Exchange
	Symbol    ctypes.Symbol
	AccountID string

	ExchangeOrderID ctypes.OrderId
	ClientOrderID   ctypes.OrderId

	Reason string
	Code   string
}

func (e OrderEvent) GetTimestamp() time.Time { return e.Ts }

// FillEvent 成交事件（交易所语义）
type FillEvent struct {
	Kind ExchangeEventKind
	Ts   time.Time

	Exchange  ctypes.Exchange
	Symbol    ctypes.Symbol
	AccountID string

	ExchangeOrderID ctypes.OrderId
	ClientOrderID   ctypes.OrderId

	Side  ctypes.PositionSide
	IsBuy bool

	Qty   decimal.Decimal
	Price decimal.Decimal
	Fee   decimal.Decimal
	Asset string
}

func (e FillEvent) GetTimestamp() time.Time { return e.Ts }
