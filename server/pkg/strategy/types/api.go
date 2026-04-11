package types

import (
	"github.com/wangliang139/NovaForge/server/pkg/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// CreateStrategyRequest 创建策略请求
type CreateStrategyRequest struct {
	Name        string
	Code        string
	Description string
	Params      []*StrategyParam
	Signals     []*SignalDefinition
}

type CreateStrategyResponse struct {
	Strategy *Strategy `json:"strategy,omitempty"`
}

// UpdateStrategyRequest 更新策略请求
type UpdateStrategyRequest struct {
	// 全量覆盖更新：无论是否修改，调用方应传入所有字段；字段为空表示清空。
	Id          string
	Version     string
	Name        string
	Description string
	Code        string
	Params      []*StrategyParam
	Signals     []*SignalDefinition
}

// StrategyFilter 策略过滤条件
type StrategyFilter struct {
	Id             *string
	Version        *string
	Status         *StrategyStatus
	Name           *string
	CreatedAtStart *int64
	CreatedAtEnd   *int64
}

type UpdateStrategyResponse struct {
	Strategy *Strategy `json:"strategy,omitempty"`
}

// ---- Strategy CRUD ----

type GetStrategyRequest struct {
	ID      string  `json:"id"`
	Version *string `json:"version,omitempty"`
}

type GetStrategyResponse struct {
	Strategy *Strategy `json:"strategy,omitempty"`
}

type ListStrategiesRequest struct {
	Offset         int64           `json:"offset"`
	Limit          int64           `json:"limit"`
	ID             *string         `json:"id,omitempty"`
	Version        *string         `json:"version,omitempty"`
	Status         *StrategyStatus `json:"status,omitempty"`
	Name           *string         `json:"name,omitempty"`
	CreatedAtStart *int64          `json:"createdAtStart,omitempty"`
	CreatedAtEnd   *int64          `json:"createdAtEnd,omitempty"`
}

type ListStrategiesResponse struct {
	Strategies []*Strategy `json:"strategies,omitempty"`
	Count      int64       `json:"count"`
}

type CountStrategiesRequest struct{}

type CountStrategiesResponse struct {
	Count int64 `json:"count"`
}

type DeleteStrategyRequest struct {
	ID string `json:"id"`
}

type DeleteStrategyResponse struct{}

type ActiveStrategyRequest struct {
	ID string `json:"id"`
}

type ActiveStrategyResponse struct{}

type InactiveStrategyRequest struct {
	ID string `json:"id"`
}

type InactiveStrategyResponse struct{}

// ---- Datasource ----

type CreateDatasourceRequest struct {
	Type        SignalType      `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Exchange    *types.Exchange `json:"exchange,omitempty"`
	Symbol      *string         `json:"symbol,omitempty"`
	Props       *string         `json:"props,omitempty"` // JSON string
	StartTs     int64           `json:"startTs"`
	EndTs       int64           `json:"endTs"`
}

type CreateDatasourceResponse struct {
	Datasource *types.DataSource `json:"datasource,omitempty"`
	Inserted   bool              `json:"inserted"`
}

type ListDatasourcesRequest struct {
	Offset   int64           `json:"offset"`
	Limit    int64           `json:"limit"`
	Type     *SignalType     `json:"type,omitempty"`
	Exchange *types.Exchange `json:"exchange,omitempty"`
	Symbol   *string         `json:"symbol,omitempty"`
}

type ListDatasourcesResponse struct {
	Datasources []*types.DataSource `json:"datasources,omitempty"`
	Count       int64               `json:"count"`
}

type DeleteDatasourceRequest struct {
	ID int32 `json:"id"`
}

type DeleteDatasourceResponse struct{}

type CreateBotRequest struct {
	StrategyID  string         `json:"strategyId"`
	StrategyVer string         `json:"strategyVer"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Mode        BotMode        `json:"mode"`
	Exchange    types.Exchange `json:"exchange"`
	Symbols     []string       `json:"symbols"`
	AccountID   string         `json:"accountId"`
	Config      string         `json:"config"`
}

type CreateBotResponse struct {
	Bot *Bot `json:"bot,omitempty"`
}

type UpdateBotRequest struct {
	ID          int32    `json:"id"`
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Symbols     []string `json:"symbols,omitempty"`
	Config      string   `json:"config,omitempty"`
}

type UpdateBotResponse struct {
	Bot *Bot `json:"bot,omitempty"`
}

type ListBotsRequest struct {
	Limit          int32           `json:"limit"`
	Offset         int32           `json:"offset"`
	ID             *int32          `json:"id,omitempty"`
	StrategyID     *string         `json:"strategyId,omitempty"`
	Mode           *BotMode        `json:"mode,omitempty"`
	Status         *BotStatus      `json:"status,omitempty"`
	Exchange       *types.Exchange `json:"exchange,omitempty"`
	AccountID      *string         `json:"accountId,omitempty"`
	Name           *string         `json:"name,omitempty"`
	CreatedAtStart *int64          `json:"createdAtStart,omitempty"`
	CreatedAtEnd   *int64          `json:"createdAtEnd,omitempty"`
}

type ListBotsResponse struct {
	Bots       []*Bot `json:"bots,omitempty"`
	TotalCount int32  `json:"totalCount"`
}

type CountBotsRequest struct {
	Status *BotStatus `json:"status,omitempty"`
}

type CountBotsResponse struct {
	Count int32 `json:"count"`
}

type GetBotRequest struct {
	ID int32 `json:"id"`
}

type GetBotResponse struct {
	Bot *Bot `json:"bot,omitempty"`
}

type StartBotRequest struct {
	ID int32 `json:"id"`
}

type StartBotResponse struct {
	Success bool `json:"success"`
}

type StopBotRequest struct {
	ID int32 `json:"id"`
}

type StopBotResponse struct {
	Success bool `json:"success"`
}

type DeleteBotRequest struct {
	ID int32 `json:"id"`
}

type DeleteBotResponse struct {
	Success bool `json:"success"`
}

type UpgradeBotRequest struct {
	ID int32 `json:"id"`
}

type UpgradeBotResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Bot     *Bot   `json:"bot,omitempty"`
}

type GetBotBalanceRequest struct {
	BotID        int32             `json:"botId"`
	WalletType   *types.WalletType `json:"walletType,omitempty"`
	Asset        *string           `json:"asset,omitempty"`
	WithNotional *bool             `json:"withNotional,omitempty"`
}

type GetBotBalanceResponse struct {
	Balance *types.Balance `json:"balance,omitempty"`
}

type GetBotPositionsRequest struct {
	BotID      int32             `json:"botId"`
	MarketType *types.MarketType `json:"marketType,omitempty"`
	Symbol     *string           `json:"symbol,omitempty"`
}

type GetBotPositionsResponse struct {
	Positions []*types.Position `json:"positions,omitempty"`
}

type BotPortfolioAsset struct {
	Exchange   types.Exchange   `json:"exchange"`
	WalletType types.WalletType `json:"walletType"`
	Asset      string           `json:"asset"`
	Free       string           `json:"free"`
	Frozen     string           `json:"frozen"`
	UpdatedTs  int64            `json:"updatedTs"`
}

type BotPortfolioPosition struct {
	Exchange   types.Exchange     `json:"exchange"`
	Symbol     string             `json:"symbol"`
	MarketType types.MarketType   `json:"marketType"`
	Side       types.PositionSide `json:"side"`
	Qty        string             `json:"qty"`
	AvgPrice   string             `json:"avgPrice"`
	UpdatedTs  int64              `json:"updatedTs"`
	Leverage   int32              `json:"leverage"`
}

type BotPortfolio struct {
	Assets    []BotPortfolioAsset    `json:"assets"`
	Positions []BotPortfolioPosition `json:"positions"`
	Ts        int64                  `json:"ts"`
}

type GetBotStateRequest struct {
	BotID int32 `json:"botId"`
}

type GetBotStateResponse struct {
	BotStatus           BotStatus     `json:"botStatus"`
	ExecutorStatus      string        `json:"executorStatus,omitempty"`
	RunErr              string        `json:"runErr,omitempty"`
	JsRunnerStatus      string        `json:"jsRunnerStatus,omitempty"`
	LastSignalTs        int64         `json:"lastSignalTs,omitempty"`
	SignalAvgDurationMs int64         `json:"signalAvgDurationMs,omitempty"`
	SignalAvgLatencyMs  int64         `json:"signalAvgLatencyMs,omitempty"`
	Portfolio           *BotPortfolio `json:"portfolio,omitempty"`
}

type ListBotOrdersRequest struct {
	BotID       int32               `json:"botId"`
	Page        int32               `json:"page"`
	Size        int32               `json:"size"`
	Symbol      *string             `json:"symbol,omitempty"`
	OrderType   *types.OrderType    `json:"orderType,omitempty"`
	OrderSource *types.OrderSource  `json:"orderSource,omitempty"`
	Statuses    []types.OrderStatus `json:"statuses,omitempty"`
}

type ListBotOrdersResponse struct {
	Orders     []*types.Order `json:"orders,omitempty"`
	TotalCount int32          `json:"totalCount"`
}

type ListBotLedgerRequest struct {
	BotID      int32             `json:"botId"`
	Page       int32             `json:"page"`
	Size       int32             `json:"size"`
	WalletType *types.WalletType `json:"walletType,omitempty"`
	Asset      *string           `json:"asset,omitempty"`
	StartTs    *int64            `json:"startTs,omitempty"`
	EndTs      *int64            `json:"endTs,omitempty"`
}

type ListBotLedgerResponse struct {
	Ledgers    []*types.Ledger `json:"ledgers,omitempty"`
	TotalCount int32           `json:"totalCount"`
}

type ListBotEquityRequest struct {
	BotID   int32 `json:"botId"`
	StartTs int64 `json:"startTs"`
	EndTs   int64 `json:"endTs"`
}

type ListBotEquityResponse struct {
	Points []*types.Equity `json:"points,omitempty"`
}

type MetricsDimension string

const (
	MetricsDimensionUnspecified MetricsDimension = ""
	MetricsDimensionAccount     MetricsDimension = "account"
	MetricsDimensionSymbol      MetricsDimension = "symbol"
)

type SymbolMetrics struct {
	Symbol                string  `json:"symbol"`
	Exchange              string  `json:"exchange"`
	Cagr                  float64 `json:"cagr"`
	Sharpe                float64 `json:"sharpe"`
	Sortino               float64 `json:"sortino"`
	MaxDrawdown           float64 `json:"maxDrawdown"`
	TimeUnderWaterSeconds int64   `json:"timeUnderWaterSeconds"`
	Calmar                float64 `json:"calmar"`
	RollingSharpe         float64 `json:"rollingSharpe"`
	WinRate               float64 `json:"winRate"`
	ProfitFactor          float64 `json:"profitFactor"`
	FeeRatio              float64 `json:"feeRatio"`
	MaxConsecutiveLoss    int32   `json:"maxConsecutiveLoss"`
	AvgSlippageBps        float64 `json:"avgSlippageBps"`
}

type QueryBotMetricsRequest struct {
	BotID     int32            `json:"botId"`
	Dimension MetricsDimension `json:"dimension,omitempty"`
	Symbol    *ctypes.Symbol   `json:"symbol,omitempty"`
	StartTs   *int64           `json:"startTs,omitempty"`
	EndTs     *int64           `json:"endTs,omitempty"`
}

type QueryBotMetricsResponse struct {
	AccountID             string           `json:"accountId"`
	BotID                 int32            `json:"botId"`
	Dimension             MetricsDimension `json:"dimension"`
	SymbolsFilter         string           `json:"symbolsFilter,omitempty"`
	StartTs               int64            `json:"startTs"`
	EndTs                 int64            `json:"endTs"`
	Cagr                  float64          `json:"cagr"`
	Sharpe                float64          `json:"sharpe"`
	Sortino               float64          `json:"sortino"`
	MaxDrawdown           float64          `json:"maxDrawdown"`
	TimeUnderWaterSeconds int64            `json:"timeUnderWaterSeconds"`
	Calmar                float64          `json:"calmar"`
	RollingSharpe         float64          `json:"rollingSharpe"`
	WinRate               float64          `json:"winRate"`
	ProfitFactor          float64          `json:"profitFactor"`
	FeeRatio              float64          `json:"feeRatio"`
	MaxConsecutiveLoss    int32            `json:"maxConsecutiveLoss"`
	AvgSlippageBps        float64          `json:"avgSlippageBps"`
	Symbols               []SymbolMetrics  `json:"symbols,omitempty"`
}

type ListBotLogsRequest struct {
	BotID     int32   `json:"botId"`
	Limit     int32   `json:"limit"`
	Cursor    *string `json:"cursor,omitempty"`
	StartTime *int64  `json:"startTime,omitempty"`
	EndTime   *int64  `json:"endTime,omitempty"`
	Level     *string `json:"level,omitempty"`
}

type BotLogEntry struct {
	ID        int64  `json:"id"`
	BotID     int32  `json:"botId"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Ts        int64  `json:"ts"`
	CreatedAt int64  `json:"createdAt"`
}

type ListBotLogsResponse struct {
	Logs       []BotLogEntry `json:"logs"`
	NextCursor *string       `json:"nextCursor,omitempty"`
}

type QueryBotSignalFlowRequest struct {
	BotID      int32       `json:"botId"`
	Limit      int32       `json:"limit"`
	SignalType *SignalType `json:"signalType,omitempty"`
	StartTsMs  *int64      `json:"startTsMs,omitempty"`
	StartID    *int64      `json:"startId,omitempty"`
}

type BotSignalRecord struct {
	ID           int64  `json:"id"`
	BotID        int32  `json:"botId"`
	AccountID    string `json:"accountId"`
	Exchange     string `json:"exchange"`
	Stream       string `json:"stream"`
	Topic        string `json:"topic"`
	EventKind    string `json:"eventKind"`
	TsMs         int64  `json:"tsMs"`
	InboundAtMs  int64  `json:"inboundAtMs"`
	OutboundAtMs int64  `json:"outboundAtMs"`
	ReceiveAtMs  int64  `json:"receiveAtMs"`
	IngestAtMs   int64  `json:"ingestAtMs"`
	PayloadJSON  string `json:"payloadJson"`
}

type QueryBotSignalFlowResponse struct {
	Events []BotSignalRecord `json:"events"`
	NextID int64             `json:"nextId"`
}

type GetBotStatsRequest struct {
	StartTs int64  `json:"startTs"`
	EndTs   int64  `json:"endTs"`
	BotID   *int32 `json:"botId,omitempty"`
}

type BotSignalStats struct {
	BotID        int32   `json:"botId"`
	Stream       string  `json:"stream"`
	EventCount   int64   `json:"eventCount"`
	AvgLatencyMs float64 `json:"avgLatencyMs"`
	MaxLatencyMs float64 `json:"maxLatencyMs"`
}

type GetBotStatsResponse struct {
	Stats []BotSignalStats `json:"stats,omitempty"`
}

type GenerateStrategyRequest struct {
	Query string `json:"query"`
}

type GenerateStrategyResponse struct {
	SessionID string `json:"sessionId"`
	Content   string `json:"content"`
}

type ListBotSignalFlowEventsResponse struct {
	Events []BotSignalRecord `json:"events"`
	NextID int64             `json:"nextId"`
}
