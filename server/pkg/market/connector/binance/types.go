package binance

import (
	"strconv"
	"time"

	binance "github.com/adshao/go-binance/v2"
	futures "github.com/adshao/go-binance/v2/futures"
	portfolio "github.com/adshao/go-binance/v2/portfolio"
	portfolio_pro "github.com/adshao/go-binance/v2/portfolio_pro"
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type Account struct {
	VipLevel int // VIP等级

	IsSpotEnabled   bool // 是否开启现货交易
	IsFutureEnabled bool // 是否开启期货交易
	IsPortfolioMode bool // 是否开启统一账户模式

	ApiKeyPermission *binance.APIKeyPermission // API权限

	SpotAccount      *binance.Account       // 现货账户信息
	PortfolioAccount *portfolio_pro.Account // 统一账户信息

	FutureAccount    *futures.AccountV3 // 合约账户信息
	FutureAcctConfig *FutureAcctConfig  // 合约账户配置
}

func (a Account) IsPortfolioEnabled() bool {
	if a.ApiKeyPermission == nil {
		return false
	}
	if a.IsPortfolioMode && a.ApiKeyPermission.EnablePortfolioMarginTrading {
		return true
	}
	return false
}

func (a Account) ToTypesAccount(exchange ctypes.Exchange) *ctypes.AccountBo {
	uid := strconv.FormatInt(a.SpotAccount.UID, 10)
	account := &ctypes.AccountBo{
		Exchange:        exchange,
		Uid:             uid,
		IsSpotEnabled:   false,
		IsFutureEnabled: false,
	}
	if a.ApiKeyPermission == nil {
		return account
	}
	isSpotEnabled := a.IsSpotEnabled && a.ApiKeyPermission.EnableSpotAndMarginTrading
	isFutureEnabled := false
	if a.IsFutureEnabled {
		if a.IsPortfolioMode && a.ApiKeyPermission.EnablePortfolioMarginTrading {
			isFutureEnabled = true
		} else if a.ApiKeyPermission.EnableFutures {
			isFutureEnabled = true
		}
	}

	account.IsSpotEnabled = isSpotEnabled
	account.IsFutureEnabled = isFutureEnabled
	return account
}

type Balance struct {
	account *Account // 账户信息

	AssetBalance       []*binance.WalletBalance        // 按账户汇总（净资产）
	SpotAccount        *binance.Account                // 现货账户信息
	FutureAccount      *futures.AccountV3              // 合约账户信息
	FundingBalance     []binance.FundingAsset          // 资金账户余额
	PortfolioBalance   []*portfolio_pro.AccountBalance // 统一账户余额
	PortfolioUmBalance *portfolio.UMAccountDetailV2    // 统一账户UM余额

	UpdatedTs time.Time
}

func (b Balance) ToTypesBalance() *ctypes.Balance {
	assets := make([]*ctypes.AssetBo, 0)
	for _, asset := range b.FundingBalance {
		_free := decimal.RequireFromString(asset.Free)
		_locked := decimal.RequireFromString(asset.Locked)
		_freezed := decimal.RequireFromString(asset.Freeze)
		_total := _free.Add(_locked).Add(_freezed)
		assets = append(assets, &ctypes.AssetBo{
			WalletType: ctypes.WalletTypeFund,
			Code:       asset.Asset,
			Balance:    _total,
			Locked:     _locked.Add(_freezed),
			UpdatedTs:  b.UpdatedTs,
		})
	}
	for _, asset := range b.SpotAccount.Balances {
		_free := decimal.RequireFromString(asset.Free)
		_locked := decimal.RequireFromString(asset.Locked)

		assets = append(assets, &ctypes.AssetBo{
			WalletType: ctypes.WalletTypeSpot,
			Code:       asset.Asset,
			Balance:    _free.Add(_locked),
			Locked:     decimal.Zero, // 币安资产的冻结/解冻由订单快照事件推导而来，无需重复落库
			UpdatedTs:  b.UpdatedTs,
		})
	}

	if b.account.IsPortfolioMode {
		// lockedMargins := make(map[string]decimal.Decimal)
		// for _, asset := range b.PortfolioUmBalance.Assets {
		// 	lockedMargins[asset.Asset] = decimal.RequireFromString(asset.InitialMargin)
		// }
		for _, asset := range b.PortfolioBalance {
			_umTotal := decimal.RequireFromString(asset.UmWalletBalance)

			// _margin := decimal.Zero
			// if _, ok := lockedMargins[asset.Asset]; ok {
			// 	_margin = lockedMargins[asset.Asset]
			// }
			// _locked := _crossMarginLocked.Add(_margin)

			assets = append(assets, &ctypes.AssetBo{
				WalletType: ctypes.WalletTypeFuture,
				Code:       asset.Asset,
				Balance:    _umTotal,
				Locked:     decimal.Zero,
				UpdatedTs:  b.UpdatedTs,
			})

			_marginTotal := decimal.RequireFromString(asset.CrossMarginAsset)
			_marginBorrowed := decimal.RequireFromString(asset.CrossMarginBorrowed)
			_marginLocked := decimal.RequireFromString(asset.CrossMarginLocked)
			assets = append(assets, &ctypes.AssetBo{
				WalletType: ctypes.WalletTypeMargin,
				Code:       asset.Asset,
				Balance:    _marginTotal.Sub(_marginBorrowed),
				Locked:     _marginLocked,
				UpdatedTs:  b.UpdatedTs,
			})
		}
	} else {
		for _, asset := range b.FutureAccount.Assets {
			_total := decimal.RequireFromString(asset.WalletBalance)
			// _free := decimal.RequireFromString(asset.AvailableBalance)
			assets = append(assets, &ctypes.AssetBo{
				WalletType: ctypes.WalletTypeFuture,
				Code:       asset.Asset,
				Balance:    _total,
				Locked:     decimal.Zero, // 币安资产的冻结/解冻由订单快照事件推导而来，无需重复落库
				UpdatedTs:  b.UpdatedTs,
			})
		}
	}

	return &ctypes.Balance{
		Assets: assets,
	}
}

type Position interface {
	Symbol() ctypes.Symbol
	Side() ctypes.PositionSide
	UpdateTime() time.Time
	Amount() decimal.Decimal
	Notional() decimal.Decimal

	Extra() map[string]any
}

type FuturePosition struct {
	symbol           ctypes.Symbol
	side             ctypes.PositionSide // 持仓方向
	amount           decimal.Decimal     // 持仓数量
	entryPrice       decimal.Decimal     // 开仓价格
	markPrice        decimal.Decimal     // 标记价格
	liquidationPrice decimal.Decimal     // 强平价格
	notional         decimal.Decimal     // 名义价值
	leverage         int                 // 杠杆倍数
	initialMargin    decimal.Decimal     // 初始保证金
	maintMargin      decimal.Decimal     // 维持保证金
	unRealizedProfit decimal.Decimal     // 未实现收益
	updateTime       time.Time           // 更新时间
}

func (p FuturePosition) Symbol() ctypes.Symbol {
	return p.symbol
}

func (p FuturePosition) Side() ctypes.PositionSide {
	return p.side
}

func (p FuturePosition) Amount() decimal.Decimal {
	return p.amount
}

func (p FuturePosition) Notional() decimal.Decimal {
	return p.notional
}

func (p FuturePosition) UpdateTime() time.Time {
	return p.updateTime
}

func (p FuturePosition) Extra() map[string]any {
	return map[string]any{
		"entryPrice":       p.entryPrice,
		"markPrice":        p.markPrice,
		"liquidationPrice": p.liquidationPrice,
		"leverage":         p.leverage,
		"initialMargin":    p.initialMargin,
		"maintMargin":      p.maintMargin,
		"unRealizedProfit": p.unRealizedProfit,
	}
}

func (p FuturePosition) ToTypesPosition() *ctypes.Position {
	return &ctypes.Position{
		Symbol:           p.symbol,
		Side:             p.side,
		Amount:           p.amount,
		EntryPrice:       p.entryPrice,
		MarkPrice:        p.markPrice,
		LiquidationPrice: p.liquidationPrice,
		Notional:         p.notional,
		Leverage:         p.leverage,
		InitialMargin:    p.initialMargin,
		MaintMargin:      p.maintMargin,
		UnRealizedProfit: p.unRealizedProfit,
		UpdatedTs:        p.updateTime,
	}
}

type FutureAcctConfig struct {
	CanDeposit        bool  // 是否开启存款权限
	CanTrade          bool  // 是否开启交易权限
	CanWithdraw       bool  // 是否开启提现权限
	DualSidePosition  bool  // 是否开启双向持仓权限
	FeeTier           int   // 手续费等级
	MultiAssetsMargin bool  // 是否开启多资产保证金模式
	UpdateTime        int64 // 更新时间
}

type Interval string

const (
	Interval1s  Interval = "1s"
	Interval1m  Interval = "1m"
	Interval3m  Interval = "3m"
	Interval5m  Interval = "5m"
	Interval15m Interval = "15m"
	Interval30m Interval = "30m"
	Interval1h  Interval = "1h"
	Interval2h  Interval = "2h"
	Interval4h  Interval = "4h"
	Interval6h  Interval = "6h"
	Interval8h  Interval = "8h"
	Interval12h Interval = "12h"
	Interval1d  Interval = "1d"
	Interval3d  Interval = "3d"
	Interval1w  Interval = "1w"
	Interval1M  Interval = "1M"
)

func (i Interval) String() string {
	return string(i)
}

func (i Interval) Valid() bool {
	switch i {
	case Interval1m, Interval3m, Interval5m, Interval15m, Interval30m, Interval1h, Interval2h, Interval4h, Interval6h, Interval8h, Interval12h, Interval1d, Interval3d, Interval1w, Interval1M:
		return true
	}
	return false
}

var MapSpotOrderType2Types = map[string]ctypes.OrderType{
	"LIMIT":              ctypes.OrderTypeLimit,
	"MARKET":             ctypes.OrderTypeMarket,
	"LIMIT_MAKER":        ctypes.OrderTypeLimit,
	"STOP":               ctypes.OrderTypeLimit,
	"STOP_MARKET":        ctypes.OrderTypeMarket,
	"TAKE_PROFIT":        ctypes.OrderTypeLimit,
	"TAKE_PROFIT_MARKET": ctypes.OrderTypeMarket,
}

var MapSpotOrderType2AlgoType = map[string]ctypes.AlgoType{
	"LIMIT":              ctypes.AlgoTypeNone,
	"MARKET":             ctypes.AlgoTypeNone,
	"LIMIT_MAKER":        ctypes.AlgoTypeNone,
	"STOP":               ctypes.AlgoTypeConditional,
	"STOP_MARKET":        ctypes.AlgoTypeConditional,
	"TAKE_PROFIT":        ctypes.AlgoTypeConditional,
	"TAKE_PROFIT_MARKET": ctypes.AlgoTypeConditional,
}

var MapFutureOrderType2Types = map[string]ctypes.OrderType{
	"LIMIT":                ctypes.OrderTypeLimit,
	"MARKET":               ctypes.OrderTypeMarket,
	"STOP":                 ctypes.OrderTypeLimit,
	"STOP_MARKET":          ctypes.OrderTypeMarket,
	"TAKE_PROFIT":          ctypes.OrderTypeLimit,
	"TAKE_PROFIT_MARKET":   ctypes.OrderTypeMarket,
	"TRAILING_STOP_MARKET": ctypes.OrderTypeMarket,
	"LIQUIDATION":          ctypes.OrderTypeMarket,
}

var MapFutureOrderType2AlgoType = map[string]ctypes.AlgoType{
	"LIMIT":                ctypes.AlgoTypeNone,
	"MARKET":               ctypes.AlgoTypeNone,
	"STOP":                 ctypes.AlgoTypeConditional,
	"STOP_MARKET":          ctypes.AlgoTypeConditional,
	"TAKE_PROFIT":          ctypes.AlgoTypeConditional,
	"TAKE_PROFIT_MARKET":   ctypes.AlgoTypeConditional,
	"TRAILING_STOP_MARKET": ctypes.AlgoTypeTrailing,
	"LIQUIDATION":          ctypes.AlgoTypeNone,
}

type SymbolConfig struct {
	Symbol           string
	IsIsolated       bool
	IsAutoAddMargin  bool
	Leverage         int
	MaxNotionalValue decimal.Decimal
}

var MapOrderStatus2Types = map[string]ctypes.OrderStatus{
	"NEW":              ctypes.OrderStatusNew,
	"PENDING":          ctypes.OrderStatusPending,
	"PARTIALLY_FILLED": ctypes.OrderStatusPartialDone,
	"FILLED":           ctypes.OrderStatusDone,
	"CANCELED":         ctypes.OrderStatusCanceled,
	"CANCELLED":        ctypes.OrderStatusCanceled,
	"PENDING_CANCEL":   ctypes.OrderStatusCanceled,
	"REJECTED":         ctypes.OrderStatusRejected,
	"EXPIRED":          ctypes.OrderStatusExpired,
	"EXPIRED_IN_MATCH": ctypes.OrderStatusExpired,
	"TRIGGERING":       ctypes.OrderStatusWorking,
	"TRIGGERED":        ctypes.OrderStatusWorking,
	"FINISHED":         ctypes.OrderStatusDone,
}

var MapAlgoOrderStatus2Types = map[string]ctypes.OrderStatus{
	"NEW":              ctypes.OrderStatusNew,
	"PENDING":          ctypes.OrderStatusPending,
	"PARTIALLY_FILLED": ctypes.OrderStatusPartialDone,
	"FILLED":           ctypes.OrderStatusDone,
	"CANCELED":         ctypes.OrderStatusCanceled,
	"CANCELLED":        ctypes.OrderStatusCanceled,
	"PENDING_CANCEL":   ctypes.OrderStatusCanceled,
	"REJECTED":         ctypes.OrderStatusRejected,
	"EXPIRED":          ctypes.OrderStatusExpired,
	"EXPIRED_IN_MATCH": ctypes.OrderStatusExpired,
}

// LeverageBracket define leverage bracket
type LeverageBracket struct {
	Symbol       string    `json:"symbol"`
	NotionalCoef string    `json:"notionalCoef"` // 用户bracket相对默认bracket的倍数，仅在和交易对默认不一样时显示
	Brackets     []Bracket `json:"brackets"`
}

// Bracket define bracket info
type Bracket struct {
	Bracket          int     `json:"bracket"`          // Notional bracket
	InitialLeverage  int     `json:"initialLeverage"`  // Max initial leverage for this bracket
	NotionalCap      float64 `json:"notionalCap"`      // Cap notional of this bracket
	NotionalFloor    float64 `json:"notionalFloor"`    // Notional threshold of this bracket
	MaintMarginRatio float64 `json:"maintMarginRatio"` // Maintenance ratio for this bracket
	Cum              float64 `json:"cum"`              // Auxiliary number for quick calculation
}
