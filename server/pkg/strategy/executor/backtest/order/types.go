package order

import "github.com/shopspring/decimal"

// reservation 预留资金/数量（从 matching 包迁移）
type reservation struct {
	Asset          string
	Amount         decimal.Decimal
	IsFuture       bool            // 是否是期货订单
	IsOpenPosition bool            // 是否是开仓订单（仅期货有效）
	MarginAmount   decimal.Decimal // 保证金金额（仅期货开仓有效）
}
