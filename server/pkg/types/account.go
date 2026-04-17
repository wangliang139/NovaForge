package types

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type AccountBo struct {
	Exchange Exchange `json:"exchange,omitempty"`

	Uid string `json:"uid,omitempty"` // 账户ID

	IsSpotEnabled   bool `json:"isSpotEnabled,omitempty"`   // 是否开启现货交易
	IsFutureEnabled bool `json:"isFutureEnabled,omitempty"` // 是否开启期货交易
}

type Balance struct {
	Notional          decimal.Decimal `json:"notional,omitempty"`          // 当前总量对应的现金价值（USDT）
	UnRealizedProfit  decimal.Decimal `json:"unRealizedProfit,omitempty"`  // 未结算收益（USDT）
	Notional24HChange decimal.Decimal `json:"notional24HChange,omitempty"` // 24小时现金价值变动（USDT）

	Assets []*AssetBo `json:"assets,omitempty"` // 资产列表按币种汇总
}

type WalletType string

const (
	WalletTypeFund   WalletType = "fund"   // 资金账户
	WalletTypeTrade  WalletType = "trade"  // 交易账户 (OKX)
	WalletTypeSpot   WalletType = "spot"   // 现货账户 (Binance)
	WalletTypeFuture WalletType = "future" // 合约账户 (Binance)
	WalletTypeMargin WalletType = "margin" // 杠杆账户 (Binance)
)

func (a WalletType) Valid() bool {
	return a == WalletTypeFund || a == WalletTypeTrade || a == WalletTypeSpot || a == WalletTypeFuture || a == WalletTypeMargin
}

type AssetKey struct {
	Exchange   Exchange
	WalletType WalletType
	Asset      string
}

type AssetBo struct {
	AccountID  string     `json:"accountId,omitempty"`  // 账户ID
	WalletType WalletType `json:"walletType,omitempty"` // 钱包类型
	Code       string     `json:"code,omitempty"`       // 资产代码

	Balance decimal.Decimal `json:"balance,omitempty"` // 当前总量
	Locked  decimal.Decimal `json:"locked,omitempty"`  // 锁定数量

	Notional         decimal.Decimal `json:"notional,omitempty"`         // 当前总量对应的现金价值（USDT）
	UnRealizedProfit decimal.Decimal `json:"unRealizedProfit,omitempty"` // 未结算收益（币种单位）
	AvgPrice         decimal.Decimal `json:"avgPrice,omitempty"`         // 持仓均价（asset/USDT，来自 assets.avg_price）

	UpdatedTs time.Time `json:"updatedTs,omitempty"` // 更新时间
}

func (a *AssetBo) Free() decimal.Decimal {
	return a.Balance.Sub(a.Locked)
}

type Asset struct {
	AccountID  string     `json:"accountId,omitempty"`  // 账户ID
	WalletType WalletType `json:"walletType,omitempty"` // 钱包类型
	Code       string     `json:"code,omitempty"`       // 资产代码

	Balance       decimal.Decimal `json:"balance,omitempty"`       // 当前总量
	Frozened      decimal.Decimal `json:"frozened,omitempty"`      // 冻结数量
	OrderOccupied decimal.Decimal `json:"orderOccupied,omitempty"` // 订单占用数量
	AvgPrice      decimal.Decimal `json:"avgPrice,omitempty"`      // 持仓均价（asset/USDT）

	UpdatedTs time.Time `json:"updatedTs,omitempty"` // 更新时间
}

func (a *Asset) IsEmpty() bool {
	return a.Balance.IsZero() && a.Frozened.IsZero() && a.OrderOccupied.IsZero()
}

func (a *Asset) Locked() decimal.Decimal {
	return a.Frozened.Add(a.OrderOccupied)
}

// Ledger 资金流水
type Ledger struct {
	ID          int64           `json:"id,omitempty"`
	AccountID   string          `json:"accountId,omitempty"`
	Exchange    Exchange        `json:"exchange,omitempty"`
	Asset       string          `json:"asset,omitempty"`
	WalletType  WalletType      `json:"walletType,omitempty"`
	Total       decimal.Decimal `json:"total,omitempty"`
	Frozen      decimal.Decimal `json:"frozen,omitempty"`
	TotalDelta  decimal.Decimal `json:"totalDelta,omitempty"`
	FrozenDelta decimal.Decimal `json:"frozenDelta,omitempty"`
	Type        LedgerReason    `json:"type,omitempty"`
	Detail      json.RawMessage `json:"detail,omitempty"`
	IsEffective bool            `json:"isEffective,omitempty"`
	Ts          time.Time       `json:"ts,omitempty"`
	CreatedAt   time.Time       `json:"createdAt,omitempty"`
}

type SymbolLeverage struct {
	Exchange  Exchange     `json:"exchange,omitempty"`
	Symbol    Symbol       `json:"symbol,omitempty"`
	Side      PositionSide `json:"side,omitempty"` // 仓位方向
	Leverage  int          `json:"leverage,omitempty"`
	UpdatedTs time.Time    `json:"updatedTs,omitempty"` // 更新时间
}

type AssetEvent struct {
	WalletType WalletType `json:"walletType,omitempty"` // 钱包类型
	Code       string     `json:"code,omitempty"`       // 资产代码

	Balance *decimal.Decimal `json:"balance,omitempty"` // 当前总量
	Locked  *decimal.Decimal `json:"locked,omitempty"`  // 锁定数量

	UpdatedTs time.Time `json:"updatedTs,omitempty"` // 更新时间
}

type BalanceSnapshot struct {
	Scope  []WalletType  `json:"scope,omitempty"`  // 资产范围
	Assets []*AssetEvent `json:"assets,omitempty"` // 资产列表
}

type UpdateType string

const (
	UpdateTypeSnapshot  UpdateType = "snapshot"
	UpdateTypeIncrement UpdateType = "increment"
)

type BalanceUpdate struct {
	EventID string          `json:"eventId,omitempty"` // 事件ID
	Type    UpdateType      `json:"type,omitempty"`    // 更新类型
	Reason  LedgerReason    `json:"reason,omitempty"`  // 更新原因（规范化 code）
	Assets  []*AssetEvent   `json:"assets,omitempty"`  // 资产列表
	Detail  json.RawMessage `json:"detail,omitempty"`  // 事件详情
}

type PositionSnapshot struct {
	Positions []*Position `json:"positions,omitempty"` // 持仓列表
}

type PositionsUpdate struct {
	EventID   string      `json:"eventId,omitempty"`   // 事件ID
	Type      UpdateType  `json:"type,omitempty"`      // 更新类型
	Reason    string      `json:"reason,omitempty"`    // 更新原因
	Positions []*Position `json:"positions,omitempty"` // 持仓列表
}

type LedgerReason string

const (
	LedgerReasonSnapshot            LedgerReason = "SNAPSHOT"
	LedgerReasonDeposit             LedgerReason = "DEPOSIT"
	LedgerReasonWithdraw            LedgerReason = "WITHDRAW"
	LedgerReasonWithdrawReject      LedgerReason = "WITHDRAW_REJECT"
	LedgerReasonFill                LedgerReason = "FILL"
	LedgerReasonFundingFee          LedgerReason = "FUNDING_FEE"
	LedgerReasonDelivered           LedgerReason = "DELIVERED"
	LedgerReasonExercised           LedgerReason = "EXERCISED"
	LedgerReasonTransferred         LedgerReason = "TRANSFERRED"
	LedgerReasonLiquidation         LedgerReason = "LIQUIDATION"
	LedgerReasonClawBack            LedgerReason = "CLAW_BACK"
	LedgerReasonADL                 LedgerReason = "ADL"
	LedgerReasonAdjustment          LedgerReason = "ADJUSTMENT"
	LedgerReasonSetLeverage         LedgerReason = "SET_LEVERAGE"
	LedgerReasonInterestDeduction   LedgerReason = "INTEREST_DEDUCTION"
	LedgerReasonSettlement          LedgerReason = "SETTLEMENT"
	LedgerReasonInsuranceClear      LedgerReason = "INSURANCE_CLEAR"
	LedgerReasonAdminDeposit        LedgerReason = "ADMIN_DEPOSIT"
	LedgerReasonAdminWithdraw       LedgerReason = "ADMIN_WITHDRAW"
	LedgerReasonMarginTransfer      LedgerReason = "MARGIN_TRANSFER"
	LedgerReasonMarginTypeChange    LedgerReason = "MARGIN_TYPE_CHANGE"
	LedgerReasonAssetTransfer       LedgerReason = "ASSET_TRANSFER"
	LedgerReasonOptionsPremiumFee   LedgerReason = "OPTIONS_PREMIUM_FEE"
	LedgerReasonOptionsSettleProfit LedgerReason = "OPTIONS_SETTLE_PROFIT"
	LedgerReasonAutoExchange        LedgerReason = "AUTO_EXCHANGE"
	LedgerReasonCoinSwapDeposit     LedgerReason = "COIN_SWAP_DEPOSIT"
	LedgerReasonCoinSwapWithdraw    LedgerReason = "COIN_SWAP_WITHDRAW"

	LedgerReasonFundsFreeze         LedgerReason = "FUNDS_FREEZE"
	LedgerReasonFundsUnfreeze       LedgerReason = "FUNDS_UNFREEZE"
	LedgerReasonOrderMarginFreeze   LedgerReason = "ORDER_MARGIN_FREEZE"
	LedgerReasonOrderMarginUnfreeze LedgerReason = "ORDER_MARGIN_UNFREEZE"
)

func NormalizeLedgerReason(exchange Exchange, raw string) LedgerReason {
	if raw == "" {
		return LedgerReasonAdjustment
	}

	switch exchange {
	case ExchangeBinance, ExchangeBinanceTest:
		return normalizeBinanceLedgerReason(raw)
	case ExchangeOkx, ExchangeOkxTest:
		return normalizeOkxLedgerReason(raw)
	default:
		return LedgerReason(strings.ToUpper(strings.TrimSpace(raw)))
	}
}

func normalizeBinanceLedgerReason(raw string) LedgerReason {
	value := strings.ToUpper(strings.TrimSpace(raw))
	switch value {
	case "DEPOSIT":
		return LedgerReasonDeposit
	case "WITHDRAW":
		return LedgerReasonWithdraw
	case "ORDER":
		return LedgerReasonFill
	case "FUNDING_FEE":
		return LedgerReasonFundingFee
	case "WITHDRAW_REJECT":
		return LedgerReasonWithdrawReject
	case "ADJUSTMENT":
		return LedgerReasonAdjustment
	case "INSURANCE_CLEAR":
		return LedgerReasonInsuranceClear
	case "ADMIN_DEPOSIT":
		return LedgerReasonAdminDeposit
	case "ADMIN_WITHDRAW":
		return LedgerReasonAdminWithdraw
	case "MARGIN_TRANSFER":
		return LedgerReasonMarginTransfer
	case "MARGIN_TYPE_CHANGE":
		return LedgerReasonMarginTypeChange
	case "ASSET_TRANSFER":
		return LedgerReasonAssetTransfer
	case "OPTIONS_PREMIUM_FEE":
		return LedgerReasonOptionsPremiumFee
	case "OPTIONS_SETTLE_PROFIT":
		return LedgerReasonOptionsSettleProfit
	case "AUTO_EXCHANGE":
		return LedgerReasonAutoExchange
	case "COIN_SWAP_DEPOSIT":
		return LedgerReasonCoinSwapDeposit
	case "COIN_SWAP_WITHDRAW":
		return LedgerReasonCoinSwapWithdraw
	case "OUTBOUND ACCOUNT POSITION UPDATE":
		return LedgerReasonSnapshot
	case "BALANCE UPDATE":
		return LedgerReasonAdjustment
	case "EXTERNAL LOCK UPDATE":
		return LedgerReasonAdjustment
	default:
		return LedgerReason(value)
	}
}

func normalizeOkxLedgerReason(raw string) LedgerReason {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "snapshot":
		return LedgerReasonSnapshot
	case "delivered":
		return LedgerReasonDelivered
	case "exercised":
		return LedgerReasonExercised
	case "transferred":
		return LedgerReasonTransferred
	case "filled":
		return LedgerReasonFill
	case "liquidation":
		return LedgerReasonLiquidation
	case "claw_back":
		return LedgerReasonClawBack
	case "adl":
		return LedgerReasonADL
	case "funding_fee":
		return LedgerReasonFundingFee
	case "adjust_margin":
		return LedgerReasonAdjustment
	case "set_leverage":
		return LedgerReasonSetLeverage
	case "interest_deduction":
		return LedgerReasonInterestDeduction
	case "settlement":
		return LedgerReasonSettlement
	default:
		return LedgerReason(strings.ToUpper(value))
	}
}

func GetWalletType(exchange Exchange, marketType MarketType) WalletType {
	if exchange == ExchangeOkx || exchange == ExchangeOkxTest {
		return WalletTypeTrade
	}
	switch marketType {
	case MarketTypeFuture:
		return WalletTypeFuture
	case MarketTypeSpot:
		return WalletTypeSpot
	default:
		return WalletTypeFund
	}
}

type Equity struct {
	ID               int64           `json:"id,omitempty"`
	AccountID        string          `json:"accountId,omitempty"`
	Ts               time.Time       `json:"ts,omitempty"`
	Notional         decimal.Decimal `json:"notional,omitempty"`
	UnRealizedProfit decimal.Decimal `json:"unRealizedProfit,omitempty"`
	CreatedAt        time.Time       `json:"createdAt,omitempty"`
}

func ParseAssetCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

type Account struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Exchange    Exchange      `json:"exchange"`
	ApiKey      string        `json:"api_key"`
	ApiSecret   string        `json:"api_secret"`
	Passphrase  string        `json:"passphrase"`
	Tags        []string      `json:"tags"`
	Status      AccountStatus `json:"status"`
	Algorithm   AuthAlgorithm `json:"algorithm"`
	AccountType AccountType   `json:"account_type"`
	// ParentAccountID 仅 virtual_sub：指向父 real 账户
	ParentAccountID *string `json:"parent_account_id,omitempty"`
	// MultiBotMode 仅父 real 有意义；virtual / virtual_sub 恒为 false
	MultiBotMode bool      `json:"multi_bot_mode"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// 风控配置（新结构）
	Config *RiskConfig `json:"config,omitempty"`
}

type AccountStatus string

const (
	AccountStatusOnline  AccountStatus = "online"
	AccountStatusOffline AccountStatus = "offline"
)

func (s AccountStatus) Valid() bool {
	switch s {
	case AccountStatusOnline, AccountStatusOffline:
		return true
	}
	return false
}

type AuthAlgorithm string

const (
	AuthAlgorithmNone    AuthAlgorithm = "none"
	AuthAlgorithmHmac    AuthAlgorithm = "hmac"
	AuthAlgorithmEd25519 AuthAlgorithm = "ed25519"
	AuthAlgorithmRsa     AuthAlgorithm = "rsa"
)

func (a AuthAlgorithm) Valid() bool {
	switch a {
	case AuthAlgorithmNone, AuthAlgorithmHmac, AuthAlgorithmEd25519, AuthAlgorithmRsa:
		return true
	}
	return false
}

type AccountType string

const (
	AccountTypeReal       AccountType = "real"
	AccountTypeVirtual    AccountType = "virtual"
	AccountTypeVirtualSub AccountType = "virtual_sub"
)

func (t AccountType) Valid() bool {
	switch t {
	case AccountTypeReal, AccountTypeVirtual, AccountTypeVirtualSub:
		return true
	}
	return false
}

type CreateAccountInput struct {
	Name        string
	Exchange    Exchange
	ApiKey      string
	ApiSecret   string
	Passphrase  string
	Tags        []string
	Status      AccountStatus
	Algorithm   AuthAlgorithm
	AccountType AccountType

	InitialAssets []AssetInput

	// ParentAccountID 与 MultiBotMode 仅服务内部创建 virtual_sub / 父账户多 Bot 标记
	ParentAccountID *string
	MultiBotMode    *bool
}

// CreateVirtualSubAccountInput 在父账户 multi_bot_mode 下创建子账（不暴露为公开 GraphQL 创建入口）
type CreateVirtualSubAccountInput struct {
	ParentAccountID string
	BotName         string
	InitialAssets   []AssetInput
}

type AssetInput struct {
	Asset      string
	Total      string
	Frozen     string
	WalletType WalletType
}

type AccountState struct {
	Assets    []*AssetBo
	Positions []*Position
	Orders    []*Order
	Equity    decimal.Decimal
	DailyPnL  decimal.Decimal
}

type FundsFreezeType string

const (
	FundsFreezeTypeUnspecified FundsFreezeType = ""
	FundsFreezeTypeOrder       FundsFreezeType = "order"
)

type EventFlowStream string

const (
	EventFlowStreamUnspecified EventFlowStream = ""
	EventFlowStreamAccountRaw  EventFlowStream = "account_raw"
	EventFlowStreamAccount     EventFlowStream = "account"
	EventFlowStreamAll         EventFlowStream = "all"
)

// AccountAPIMetricsDimension 与 metrics.v1.MetricsDimension 取值对齐：1=账户 2=标的
type AccountAPIMetricsDimension int32

const (
	AccountAPIMetricsDimensionUnspecified AccountAPIMetricsDimension = 0
	AccountAPIMetricsDimensionAccount     AccountAPIMetricsDimension = 1
	AccountAPIMetricsDimensionSymbol      AccountAPIMetricsDimension = 2
)

// --- Request / Response ---

type UpdateAccountRequest struct {
	ID          string
	Name        string
	Exchange    Exchange
	ApiKey      string
	ApiSecret   string
	Passphrase  string
	Tags        []string
	Status      AccountStatus
	Algorithm   AuthAlgorithm
	AccountType AccountType
	// MultiBotMode 非 nil 时更新；仅父 real 允许为 true
	MultiBotMode *bool
}

// AccountUnallocatedAsset 父账户在共享多 Bot 模式下，某资产维度未分配数量（父快照 − 各子账初始分配之和）
type AccountUnallocatedAsset struct {
	Asset         string          `json:"asset"`
	WalletType    WalletType      `json:"wallet_type"`
	ParentTotal   decimal.Decimal `json:"parent_total"`
	SubsAllocated decimal.Decimal `json:"subs_allocated"`
	Unallocated   decimal.Decimal `json:"unallocated"`
}

type GetAccountUnallocatedAssetsRequest struct {
	ParentAccountID string
}

type GetAccountUnallocatedAssetsResponse struct {
	Items []*AccountUnallocatedAsset `json:"items"`
}

type MultiBotSubAccount struct {
	AccountID string `json:"accountId"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
}

type MultiBotAssetAllocation struct {
	Asset          string                     `json:"asset"`
	WalletType     WalletType                 `json:"walletType"`
	ParentTotal    decimal.Decimal            `json:"parentTotal"`
	SubAllocations map[string]decimal.Decimal `json:"subAllocations"`
	Unallocated    decimal.Decimal            `json:"unallocated"`
}

type MultiBotPositionAllocation struct {
	Symbol         string                     `json:"symbol"`
	Side           PositionSide               `json:"side"`
	ParentTotal    decimal.Decimal            `json:"parentTotal"`
	SubAllocations map[string]decimal.Decimal `json:"subAllocations"`
	Unallocated    decimal.Decimal            `json:"unallocated"`
}

type GetAccountMultiBotDetailsRequest struct {
	ParentAccountID string
}

type GetAccountMultiBotDetailsResponse struct {
	SubAccounts         []*MultiBotSubAccount         `json:"subAccounts"`
	AssetAllocations    []*MultiBotAssetAllocation    `json:"assetAllocations"`
	PositionAllocations []*MultiBotPositionAllocation `json:"positionAllocations"`
}

type UpdateAccountResponse struct {
	Account *Account
}

type QueryAccountsRequest struct {
	Id             *string
	Name           *string
	Tags           []string
	Status         *AccountStatus
	AccountType    *AccountType
	Exchange       *Exchange
	CreatedAtStart *int64
	CreatedAtEnd   *int64
	Offset         int64
	Limit          int64
}

type QueryAccountsResponse struct {
	Count    int64
	Accounts []*Account
}

type QueryRiskEventsRequest struct {
	AccountID string
	Limit     int64
	Offset    int64
}

type RiskEvent struct {
	ID          int64  `json:"id"`
	AccountID   string `json:"accountId"`
	Exchange    string `json:"exchange"`
	Rule        string `json:"rule"`
	RiskIndex   string `json:"riskIndex"`
	PayloadJSON string `json:"payloadJson"`
	CreatedAt   int64  `json:"createdAt"`
}

type QueryRiskEventsResponse struct {
	Events []*RiskEvent
}

type GetRiskIndexRequest struct {
	AccountID string
}

type GetRiskIndexResponse struct {
	RiskIndex string
}

type AmountLimitStrings struct {
	Amount string
	Ratio  string
}

type UpdateAccountRiskConfigRequest struct {
	AccountID                 string
	MaxOrderSize              string
	MaxLeverage               string
	MaxOrdersPerMinute        int32
	MinMaintenanceMarginRatio string
	RiskIndexThreshold        string
	RiskIndexAction           string
	CooldownSeconds           int32
	MaxPositionPerSymbol      *AmountLimitStrings
	MaxDailyLoss              *AmountLimitStrings
	MaxTotalNetExposure       *AmountLimitStrings
	MaxTotalGrossExposure     *AmountLimitStrings
}

type UpdateAccountRiskConfigResponse struct {
	Account *Account
}

type DeleteAccountRequest struct {
	Id string
}

type DeleteAccountResponse struct{}

type OnlineAccountRequest struct {
	Id string
}

type OnlineAccountResponse struct {
	Account *Account
}

type OfflineAccountRequest struct {
	Id string
}

type OfflineAccountResponse struct {
	Account *Account
}

type RefreshAccountSnapshotsRequest struct {
	AccountID string
}

type RefreshAccountSnapshotsResponse struct {
	Success bool
}

type QueryEquitysRequest struct {
	AccountID string
	StartTs   int64
	EndTs     int64
}

type QueryEquitysResponse struct {
	Equitys []*Equity
}

type QuerySymbolEquityRequest struct {
	AccountID string
	StartTs   int64
	EndTs     int64
	Exchange  string
	Symbol    string
}

type SymbolEquity struct {
	ID           int64
	AccountID    string
	Exchange     string
	Symbol       string
	NetValue     string
	BaseCurrency string
	Ts           int64
	CreatedAt    int64
}

type QuerySymbolEquityResponse struct {
	Items []*SymbolEquity
}

type GetAccountRequest struct {
	AccountID string
}

type GetAccountResponse struct {
	Account *AccountBo
}

type GetBalanceRequest struct {
	AccountID    string
	WalletType   *WalletType
	Asset        *string
	WithNotional *bool
}

type GetBalanceResponse struct {
	Balance *Balance
}

type GetPositionsRequest struct {
	AccountID  string
	MarketType *MarketType
	Symbol     *string
}

type GetPositionsResponse struct {
	Positions []*Position
}

type EstimateOrderRequest struct {
	AccountID string
	Symbol    string
	Side      PositionSide
	IsBuy     bool
	OrderType OrderType
	Price     string
	QuoteQty  string
	Leverage  int32
}

type EstimateOrderResponse struct {
	LiquidationPrice string
	Fee              string
	FeeAsset         string
	ExpectedPnl      string
}

type GetSymbolConfigRequest struct {
	AccountID string
	Symbol    string
}

type GetSymbolConfigResponse struct {
	Config *SymbolConfig
}

type QueryLedgersRequest struct {
	AccountID  string
	WalletType *WalletType
	Asset      *string
	StartTs    int64
	EndTs      int64
	Page       int32
	Size       int32
}

type QueryLedgersResponse struct {
	Ledgers    []*Ledger
	TotalCount int64
}

type SetLeverageRequest struct {
	AccountID string
	Symbol    string
	Leverage  int32
}

type SetLeverageResponse struct {
	Success  bool
	Leverage int32
}

type GetLeverageRequest struct {
	AccountID string
	Symbol    string
}

type GetLeverageResponse struct {
	Leverage int32
}

type FundsFreezeRequest struct {
	AccountID  string
	Symbol     string
	Asset      string
	Amount     string
	FreezeType FundsFreezeType
	Order      *Order
}

type FundsFreezeResponse struct {
	Success bool
}

type FundsUnfreezeRequest struct {
	AccountID  string
	Symbol     string
	Asset      string
	Amount     string
	FreezeType FundsFreezeType
	Order      *Order
}

type FundsUnfreezeResponse struct {
	Success bool
}

type QueryAccountEventFlowRequest struct {
	AccountID string
	Stream    EventFlowStream
	StartTsMs *int64
	StartID   *int64
	Limit     int32
}

type EventRecord struct {
	ID          int64  `json:"id"`
	AccountID   string `json:"accountId"`
	Exchange    string `json:"exchange"`
	Stream      string `json:"stream"`
	Topic       string `json:"topic"`
	EventKind   string `json:"eventKind"`
	TsMs        int64  `json:"tsMs"`
	ReceiveAtMs int64  `json:"receiveAtMs"`
	PublishAtMs int64  `json:"publishAtMs"`
	IngestAtMs  int64  `json:"ingestAtMs"`
	PayloadJSON string `json:"payloadJson"`
}

type QueryAccountEventFlowResponse struct {
	Events []*EventRecord
	NextID int64
}

type QueryAccountMetricsRequest struct {
	AccountID string
	Symbol    *string
	StartTs   *int64
	EndTs     *int64
	Dimension AccountAPIMetricsDimension
}

type AccountSymbolMetrics struct {
	Symbol                string
	Exchange              string
	Cagr                  float64
	Sharpe                float64
	Sortino               float64
	MaxDrawdown           float64
	TimeUnderWaterSeconds int64
	Calmar                float64
	WinRate               float64
	ProfitFactor          float64
	RollingSharpe         float64
	AvgSlippageBps        float64
	FeeRatio              float64
	MaxConsecutiveLoss    int32
}

type QueryAccountMetricsResponse struct {
	AccountID             string                     `json:"accountId"`
	Dimension             AccountAPIMetricsDimension `json:"dimension"`
	Cagr                  float64
	Sharpe                float64
	Sortino               float64
	MaxDrawdown           float64
	TimeUnderWaterSeconds int64
	Calmar                float64
	WinRate               float64
	ProfitFactor          float64
	RollingSharpe         float64
	AvgSlippageBps        float64
	FeeRatio              float64
	MaxConsecutiveLoss    int32
	StartTs               int64
	EndTs                 int64
	Symbols               []*AccountSymbolMetrics
}
