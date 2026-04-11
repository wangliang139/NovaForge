package exchange

import bridge "github.com/wangliang139/llt-trade/server/pkg/strategy/exchange/bridge"

// 兼容层：对外保留 exchange 包下的事件类型名；实际实现下沉到 exchange/bridge，
// 避免 exchange(依赖 matching) <-> matching(依赖事件类型) 的 import cycle。

type (
	OrderEvent = bridge.OrderEvent
	FillEvent  = bridge.FillEvent
)

type (
	ExchangeEvent     = bridge.ExchangeEvent
	ExchangeEventKind = bridge.ExchangeEventKind
)

const (
	ExchangeEventKindOrderAccepted = bridge.ExchangeEventKindOrderAccepted
	ExchangeEventKindOrderRejected = bridge.ExchangeEventKindOrderRejected
	ExchangeEventKindOrderCanceled = bridge.ExchangeEventKindOrderCanceled
	ExchangeEventKindOrderExpired  = bridge.ExchangeEventKindOrderExpired
	ExchangeEventKindOrderDone     = bridge.ExchangeEventKindOrderDone
	ExchangeEventKindFill          = bridge.ExchangeEventKindFill
)

type (
	MarketEvent = bridge.MarketEvent
)
