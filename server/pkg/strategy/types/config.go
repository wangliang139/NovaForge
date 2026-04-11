package types

import (
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

// RunMode 表示一次策略执行“运行模式”。
type RunMode string

const (
	RunModeLive     RunMode = "live"     // 实盘
	RunModePaper    RunMode = "paper"    // 模拟盘
	RunModeBacktest RunMode = "backtest" // 回测
)

// RootConfig 顶层配置：上游统一传入这一份。
// 设计原则：
// - StrategyParams 仅用于 JS runtime.WithParams（策略参数），不承载撮合/风控/手续费等基础设施配置
// - 各子模块配置强类型化；若上游来源仍是 map/json，可在上游或入口处做 parse/merge
type RootConfig struct {
	Mode RunMode

	// Global 全局默认（跨模块共享的“基准”配置）
	Global GlobalConfig

	// Exchange 交易所维度配置（可按 exchange 进一步细分）
	Exchange ExchangeConfig

	// Account 账户维度配置（含杠杆、精度、初始资金等）
	Account AccountConfig

	// Strategy 策略维度配置（JS 沙箱/运行时/策略参数）
	Strategy StrategyConfig

	// Backtest 仅在回测模式使用
	Backtest *BacktestConfig

	// Live 仅在实盘模式使用
	Live *LiveConfig

	// Paper 仅在模拟盘模式使用
	Paper *PaperConfig
}

/********** Global **********/

type GlobalConfig struct {
	// 估值与“附加 price kline source”等需要的基准货币与优先交易所
	BaseCurrency string          // 默认 "USDT"
	BaseExchange ctypes.Exchange // 默认 "binance"

	// 控制策略 console.log 缓存条数（回测结果里 console logs 也会用到）
	ConsoleLogMaxCache int // 默认 1000

	// 信号订阅（目前 entity.go 里用 env STRATEGY_*）
	PubSub PubSubConfig
}

type PubSubConfig struct {
	Enabled bool

	NatsServers  []string
	NatsName     string
	NatsUsername string
	NatsPassword string

	TopicPrefix string // 默认 "md"
}

/********** Exchange（交易所维度） **********/

type ExchangeConfig struct {
	// Sim 回测撮合/延迟/滑点/手续费等“交易所模拟”配置
	// - 映射到 exchange/matching.MatchingConfig
	// - 同时给 order.Config 提供默认费率/冻结因子，避免重复配置
	Sim ExchangeSimConfig

	// PerExchange 可选：对不同交易所覆盖 Sim 的部分字段
	PerExchange map[ctypes.Exchange]ExchangeSimConfig
}

type ExchangeSimConfig struct {
	// 手续费率（spot）
	SpotMakerCommissionRate float64
	SpotTakerCommissionRate float64

	// 手续费率（future）
	FutureMakerCommissionRate float64
	FutureTakerCommissionRate float64

	// 兼容旧字段：当 Maker/Taker 为 0 时可回退到 CommissionRate（matching.MatchingConfig 有该字段）
	CommissionRate float64

	// 市价单滑点率（matching.SlippageRate）
	SlippageRate float64

	// 市价单冻结因子（matching.MarketOrderFreezeFactor / order.Config.MarketOrderFreezeFactor）
	MarketOrderFreezeFactor float64

	// 交易所/撮合延迟（matching.ExchangeDelay；FillDelay 仅兼容历史）
	ExchangeDelay time.Duration
	FillDelay     time.Duration
}

/********** Account（账户维度） **********/

type AccountConfig struct {
	// 资产精度（account.AccountConfig.AssetPrecision，<=0 则默认 18）
	AssetPrecision int

	// 杠杆配置（按 account + 标的 + 仓位方向）
	// key 推荐：accountID + ":" + exSymbolKey + ":" + side
	Leverage LeverageConfig

	// 初始资金（主要给回测/模拟盘；实盘一般来自交易所账户同步）
	InitialFunds InitialFundsConfig
}

type LeverageConfig struct {
	// Default 默认杠杆（未命中 Overrides 时使用；<=0 表示“由系统默认/不设置”）
	Default int

	// Overrides 精确覆盖：accountId/exSymbol/side -> leverage
	Overrides map[string]int
}

type InitialFundsConfig struct {
	// 按 accountId + exSymbolKey 维度设置初始 base/quote（兼容现有 BacktestSymbol 的 BaseAssetQty/QuoteAssetQty）
	// key 推荐：accountID + ":" + exSymbolKey
	PerSymbol map[string]InitialSymbolFunds
}

type InitialSymbolFunds struct {
	BaseAssetQty  decimal.Decimal
	QuoteAssetQty decimal.Decimal
}

/********** Strategy（策略维度） **********/

type StrategyConfig struct {
	// JS 沙箱（engine.Sandbox）
	Sandbox SandboxConfig

	// 策略参数：透传给 JS runtime.WithParams
	StrategyParams map[string]any
}

type SandboxConfig struct {
	MaxMemoryBytes int64
	MaxCPU         time.Duration
	AllowedAPIs    []string
	BlockedAPIs    []string
}

/********** Backtest（回测模式） **********/

type BacktestConfig2 struct {
	// 时间范围（types.BacktestConfig.StartTime/EndTime）
	StartTime time.Time
	EndTime   time.Time

	// 标的配置（types.BacktestConfig.Symbols）
	Symbols []BacktestSymbol

	// 信号绑定（Entity.buildSignalSources 目前从 RunBacktestInput.Signals + strategy.Signals 推导 sources）
	Signals []SignalBinding

	// 数据源（如果上游已经构建好 Source，也允许直接传入，跳过内部推导）
	Sources []Source

	// 时间线/事件编排（timeline.SorterConfig + ExternalMerger policy）
	Timeline BacktestTimelineConfig

	// Order 引擎（order.Config 的强类型入口）
	Order BacktestOrderConfig

	// 风控（risk.Config 的强类型入口）
	Risk BacktestRiskConfig
}

type BacktestTimelineConfig struct {
	// SorterConfig 等价于 timeline/sorter.SorterConfig 的语义表达
	SignalTypePriority map[SignalType]int
	// scopePriority 用函数不利于配置化，这里先用“命名策略”或“固定映射”表达
	ScopePriorityMode string // e.g. "default" / "accountFirst" / "symbolFirst"

	// ExternalMerger 的错误策略（FailFast/Degrade）
	// ExternalErrorPolicy ErrorPolicy
}

type BacktestOrderConfig struct {
	// 允许交易的标的列表（可空；空则由 Symbols 派生）
	AllowedSymbols []ctypes.ExSymbolKey

	// 若不填，可从 Exchange.Sim 派生默认值
	SpotMakerCommissionRate   float64
	SpotTakerCommissionRate   float64
	FutureMakerCommissionRate float64
	FutureTakerCommissionRate float64

	MarketOrderFreezeFactor float64
}

type BacktestRiskConfig struct {
	// 对应 risk.Config
	MaxPositionPerSymbol decimal.Decimal
	MaxTotalPosition     decimal.Decimal
	MaxOrderSize         decimal.Decimal
	MaxDailyLoss         decimal.Decimal
	MaxLeverage          decimal.Decimal
	MaxConcentration     float64
}

/********** Live / Paper（实盘/模拟盘） **********/

type LiveConfig struct {
	// 预留：实盘执行链路（Bot executor）未来会用到
	// - 例如：信号订阅过滤（只订阅 bot 需要的 symbol）、下单限速、风控来源等
}

type PaperConfig struct {
	// 预留：模拟盘如果要“走撮合”或“走行情驱动撮合”，可复用 ExchangeSim + BacktestOrder/Risk 的子配置
}
