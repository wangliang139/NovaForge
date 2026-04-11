package matching

import (
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

// orderEntry 订单簿条目（只保存撮合必需的最小信息）
type orderEntry struct {
	OrderID     ctypes.OrderId
	Price       decimal.Decimal
	RemainQty   decimal.Decimal // 剩余数量
	IsBuy       bool
	Ts          time.Time
	TimeInForce ctypes.TimeInForce
	Status      ctypes.OrderStatus // 新增：订单状态，减少对 orderCache 的查询
}

type orderBook struct {
	// 市价单：按时间顺序（FIFO）
	marketBuys  []*orderEntry
	marketSells []*orderEntry

	// 限价单：按价优/时优
	limitBuys  []*orderEntry // price desc, ts asc
	limitSells []*orderEntry // price asc, ts asc

	// 挂单计数器（NEW 和 PARTIAL_DONE 状态）
	openOrderCount int
}

type marketBar struct {
	Open   decimal.Decimal
	High   decimal.Decimal
	Low    decimal.Decimal
	Close  decimal.Decimal
	Volume decimal.Decimal // base volume
}
