package types

import (
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

// BacktestConfig 回测配置
type BacktestConfig struct {
	StartTime    time.Time
	EndTime      time.Time
	Symbols      []*BacktestSymbol // 交易标的
	Sources      []Source         // 支持多数据源
	Params       map[string]any   // 回测参数（透传给策略）
	BaseCurrency string           // 统一记账货币，默认 "USDT"
	BaseExchange ctypes.Exchange  // 估值用价格的优先交易所（可选）
}

// BacktestResult 回测结果
type BacktestResult struct {
	ID             string    `json:"id"`                     // 回测唯一ID
	JobID          string    `json:"job_id,omitempty"`       // 回测任务ID（可选）
	StrategyID     string    `json:"strategy_id"`            // 策略ID
	StrategyVer    string    `json:"strategy_version"`       // 策略版本
	StartTime      time.Time `json:"start_time"`             // 回测起始时间
	EndTime        time.Time `json:"end_time"`               // 回测结束时间
	TimeCost       int64     `json:"time_cost"`              // 回测耗时（毫秒）
	InitialBalance string    `json:"initial_balance"`        // 初始总权益（BaseCurrency 计价，字符串避免精度丢失）
	FinalBalance   string    `json:"final_balance"`          // 结束时总权益（BaseCurrency 计价）
	TotalPnl       string    `json:"total_pnl"`              // 总盈亏（FinalBalance-InitialBalance）
	TotalTrades    int       `json:"total_trades"`           // 总成交笔数
	WinTrades      int       `json:"win_trades"`             // 盈利成交数
	LossTrades     int       `json:"loss_trades"`            // 亏损成交数
	WinRate        float64   `json:"win_rate"`               // 胜率（0~1）
	SharpeRatio    float64   `json:"sharpe_ratio,omitempty"` // 夏普比率
	MaxDrawdown    float64   `json:"max_drawdown,omitempty"` // 最大回撤

	// Data 承载结构化扩展信息（更适合长期演进，也更利于控制返回体大小）。
	// 约定：默认仅返回 summary + equity（轻量）；需要明细时再通过参数开关注入（例如订单/成交列表）。
	Data      *BacktestResultData `json:"data,omitempty"` // 回测详细结构化数据（如订单、成交、日志等，按需注入）
	CreatedAt time.Time           `json:"created_at"`     // 结果生成时间
}

// BacktestResultData 回测结果扩展数据（强类型）
type BacktestResultData struct {
	// Symbols 扁平化的逐标的摘要（最常用，便于前端展示/排序/筛选）
	Symbols []*SymbolSummary `json:"symbols,omitempty"`

	// Equity 权益曲线（用于计算回撤/夏普，也便于前端画图）
	Equity []EquityPoint `json:"equity,omitempty"`

	// ConsoleLogs 策略 console.log 输出的日志（受服务端缓存上限控制，避免返回体过大）
	ConsoleLogs []ConsoleLog `json:"console_logs,omitempty"`

	// Orders 所有订单记录（包括已成交、部分成交、已取消、已拒绝等所有状态）
	Orders []*ctypes.Order `json:"orders,omitempty"`

	// Trades 所有成交记录（包含已实现盈亏）
	Trades []*Trade `json:"trades,omitempty"`

	// Meta 预留扩展字段（尽量保持小体积；大对象请放到对象存储/分页接口）
	Meta map[string]any `json:"meta,omitempty"`
}

type EquityPoint struct {
	Ts            time.Time           `json:"ts"`
	TotalNetValue decimal.Decimal     `json:"total_net_value"`
	Symbols       []SymbolEquityPoint `json:"symbols"`
}

type SymbolEquityPoint struct {
	ExSymbol      ctypes.ExSymbol `json:"ex_symbol"`
	BaseNetValue  decimal.Decimal `json:"base_net_value"`
	QuoteNetValue decimal.Decimal `json:"quote_net_value"`
	BaseQty       decimal.Decimal `json:"base_qty"`
	QuoteQty      decimal.Decimal `json:"quote_qty"`
	PosQty        decimal.Decimal `json:"pos_qty"`
	AvgPx         decimal.Decimal `json:"avg_px"`
}

// ConsoleLog is one strategy console output entry.
type ConsoleLog struct {
	Ts      time.Time `json:"ts"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// Trade 成交记录（包含已实现盈亏）
type Trade struct {
	ExSymbol      ctypes.ExSymbol     `json:"ex_symbol"`
	OrderID       ctypes.OrderId      `json:"orderId,omitempty"`
	ClientOrderID ctypes.OrderId      `json:"clientOrderId,omitempty"`
	Side          ctypes.PositionSide `json:"side,omitempty"`
	IsBuy         bool                `json:"isBuy,omitempty"`
	Qty           decimal.Decimal     `json:"qty,omitempty"`
	Price         decimal.Decimal     `json:"price,omitempty"`
	Fee           decimal.Decimal     `json:"fee,omitempty"`
	Asset         string              `json:"asset,omitempty"` // 手续费扣除资产
	FeeInBase     decimal.Decimal     `json:"feeInBase,omitempty"`
	Numeraire     string              `json:"numeraire,omitempty"` // BaseCurrency（如 USDT）
	RealizedPnl   decimal.Decimal     `json:"realizedPnl"`         // 已实现盈亏（字符串表示用于避免浮点误差）
	Ts            time.Time           `json:"ts,omitempty"`
}

// SymbolSummary 单标的资金/仓位/盈亏汇总（字符串表示用于避免浮点误差，并便于 JSON 传输）
type SymbolSummary struct {
	ExSymbol ctypes.ExSymbol `json:"ex_symbol"`

	// 资金数据
	InitialBase  decimal.Decimal `json:"initialBase"`
	InitialQuote decimal.Decimal `json:"initialQuote"`
	FinalBase    decimal.Decimal `json:"finalBase"`
	FinalQuote   decimal.Decimal `json:"finalQuote"`

	PositionQty decimal.Decimal `json:"positionQty"`
	AvgPrice    decimal.Decimal `json:"avgPrice"`
	LastPrice   decimal.Decimal `json:"lastPrice"`

	InitialNet decimal.Decimal `json:"initialNet"`
	FinalNet   decimal.Decimal `json:"finalNet"`

	RealizedPnl   decimal.Decimal `json:"realizedPnl"`
	UnrealizedPnl decimal.Decimal `json:"unrealizedPnl"`
	NetPnl        decimal.Decimal `json:"netPnl"`

	// 按方向区分的盈亏统计（用于合约双向开仓）
	LongRealizedPnl    decimal.Decimal `json:"longRealizedPnl"`    // 多仓已实现盈亏
	ShortRealizedPnl   decimal.Decimal `json:"shortRealizedPnl"`   // 空仓已实现盈亏
	LongUnrealizedPnl  decimal.Decimal `json:"longUnrealizedPnl"`  // 多仓未实现盈亏
	ShortUnrealizedPnl decimal.Decimal `json:"shortUnrealizedPnl"` // 空仓未实现盈亏
	LongNetPnl         decimal.Decimal `json:"longNetPnl"`         // 多仓净盈亏
	ShortNetPnl        decimal.Decimal `json:"shortNetPnl"`        // 空仓净盈亏
	LongTrades         int             `json:"longTrades"`         // 多仓成交次数
	ShortTrades        int             `json:"shortTrades"`        // 空仓成交次数
}

// BacktestContext 描述一次回测任务上下文（不依赖Bot）
type BacktestContext struct {
	ID          string `json:"id"`
	StrategyID  string `json:"strategy_id"`
	StrategyVer string `json:"strategy_version"`
}

// BacktestSymbol 回测标的
type BacktestSymbol struct {
	Exchange      ctypes.Exchange
	Symbol        ctypes.Symbol
	BaseAssetQty  string
	QuoteAssetQty string
}

// RunBacktestInput 描述一次回测的请求参数
type RunBacktestInput struct {
	Context   BacktestContext
	StartTime time.Time
	EndTime   time.Time
	Symbols   []*BacktestSymbol
	Signals   []*SignalBinding
	Params    map[string]any
	// Strategy 可选的策略对象。如果提供，将直接使用此策略，跳过数据库查询。
	Strategy *Strategy
}

// ---- Backtest ----

type RunBacktestRequest struct {
	RunType   int32             `json:"runType"` // 1: 从库加载策略
	Strategy  *Strategy         `json:"strategy,omitempty"`
	Params    string            `json:"params,omitempty"`
	Symbols   []*BacktestSymbol `json:"symbols,omitempty"`
	Signals   []*SignalBinding  `json:"signals,omitempty"`
	StartTime int64             `json:"startTime"`
	EndTime   int64             `json:"endTime"`
}

type RunBacktestResponse struct {
	ID             string              `json:"id"`
	JobID          string              `json:"job_id,omitempty"`
	StrategyID     string              `json:"strategy_id"`
	StrategyVer    string              `json:"strategy_version"`
	StartTime      time.Time           `json:"start_time"`
	EndTime        time.Time           `json:"end_time"`
	TimeCost       int64               `json:"time_cost"`
	InitialBalance string              `json:"initial_balance"`
	FinalBalance   string              `json:"final_balance"`
	TotalPnl       string              `json:"total_pnl"`
	TotalTrades    int                 `json:"total_trades"`
	WinTrades      int                 `json:"win_trades"`
	LossTrades     int                 `json:"loss_trades"`
	WinRate        float64             `json:"win_rate"`
	SharpeRatio    float64             `json:"sharpe_ratio,omitempty"`
	MaxDrawdown    float64             `json:"max_drawdown,omitempty"`
	Data           *BacktestResultData `json:"data,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	Strategy       *Strategy           `json:"strategy,omitempty"`
}
