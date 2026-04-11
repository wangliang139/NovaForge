package okx

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/mow/number"
	okx "github.com/wangliang139/okx-connector-go"
)

type Interval string

const (
	Interval1s  Interval = "1s"
	Interval1m  Interval = "1m"
	Interval3m  Interval = "3m"
	Interval5m  Interval = "5m"
	Interval15m Interval = "15m"
	Interval30m Interval = "30m"
	Interval1h  Interval = "1H"
	Interval2h  Interval = "2H"
	Interval4h  Interval = "4H"
	Interval6h  Interval = "6Hutc"
	Interval12h Interval = "12Hutc"
	Interval1d  Interval = "1Dutc"
	Interval2d  Interval = "2Dutc"
	Interval3d  Interval = "3Dutc"
	Interval5d  Interval = "5Dutc"
	Interval1w  Interval = "1Wutc"
	Interval1M  Interval = "1Mutc"
	Interval3M  Interval = "3Mutc"
)

func (i Interval) String() string {
	return string(i)
}

func (i Interval) Valid() bool {
	switch i {
	case Interval1s, Interval1m, Interval3m, Interval5m, Interval15m, Interval30m, Interval1h, Interval2h, Interval4h, Interval6h, Interval12h, Interval1d, Interval2d, Interval3d, Interval5d, Interval1w, Interval1M, Interval3M:
		return true
	}
	return false
}

func (i Interval) ToOkxKlineChannel() okx.KlineChannel {
	switch i {
	case Interval1s:
		return okx.KlineChannelCandle1s
	case Interval1m:
		return okx.KlineChannelCandle1m
	case Interval3m:
		return okx.KlineChannelCandle3m
	case Interval5m:
		return okx.KlineChannelCandle5m
	case Interval15m:
		return okx.KlineChannelCandle15m
	case Interval30m:
		return okx.KlineChannelCandle30m
	case Interval1h:
		return okx.KlineChannelCandle1H
	case Interval2h:
		return okx.KlineChannelCandle2H
	case Interval4h:
		return okx.KlineChannelCandle4H
	case Interval6h:
		return okx.KlineChannelCandle6Hutc
	case Interval12h:
		return okx.KlineChannelCandle12Hutc
	case Interval1d:
		return okx.KlineChannelCandle1Dutc
	case Interval2d:
		return okx.KlineChannelCandle2Dutc
	case Interval3d:
		return okx.KlineChannelCandle3Dutc
	case Interval5d:
		return okx.KlineChannelCandle5Dutc
	case Interval1w:
		return okx.KlineChannelCandle1Wutc
	case Interval1M:
		return okx.KlineChannelCandle1Mutc
	case Interval3M:
		return okx.KlineChannelCandle3Mutc
	}
	return okx.KlineChannel(i.String())
}

type Account struct {
	Uid          string // 当前请求的账户ID，账户uid和app上的一致
	MainUid      string // 当前请求的母账户ID，如果 uid = mainUid，代表当前账号为母账户；如果 uid != mainUid，代表当前账户为子账户。
	UserLevel    string // 当前在平台上真实交易量的用户等级，如 Lv1，代表普通用户等级。
	Level        string // 账户模式 1：现货模式，2：合约模式，3：跨币种保证金模式，4：组合保证金模式
	Type         string // 账户类型 0：母账户 1：普通子账户 2：资管子账户 5：托管交易子账户 - Copper 9：资管交易子账户 - Copper 12：托管交易子账户 - Komainu
	PositionMode string // 持仓模式 long_short_mode：开平仓模式（双向） net_mode：买卖模式（单向） 仅适用交割/永续合约

	RoleType        string   // 用户角色 0：普通用户 1：带单者 2：跟单者
	TraderInsts     []string // 当前账号已经设置的带单合约，仅适用于带单者
	SpotRoleType    string   // 现货跟单角色。0：普通用户；1：带单者；2：跟单者
	SpotTraderInsts []string // 当前账号已经设置的带单币对，仅适用于带单者

	ApiKeyPermission APIKeyPermission // API权限
}

func (a Account) ToTypesAccount(exchange ctypes.Exchange) *ctypes.AccountBo {
	isSpotEnabled := false
	if a.ApiKeyPermission.CanTrade() {
		isSpotEnabled = true
	}
	isFutureEnabled := false
	if a.Level != "1" && a.ApiKeyPermission.CanTrade() {
		isFutureEnabled = true
	}
	return &ctypes.AccountBo{
		Exchange:        exchange,
		Uid:             a.Uid,
		IsSpotEnabled:   isSpotEnabled,
		IsFutureEnabled: isFutureEnabled,
	}
}

type APIKeyPermission int8

func NewAPIKeyPermission(raw string) APIKeyPermission {
	result := APIKeyPermission(0)
	permissions := strings.Split(raw, ",")
	for _, permission := range permissions {
		switch strings.TrimSpace(permission) {
		case "read_only":
			result |= 1
		case "trade":
			result |= 2
		case "withdraw":
			result |= 4
		}
	}
	return result
}

func (p APIKeyPermission) CanRead() bool {
	return p&1 == 1
}

func (p APIKeyPermission) CanTrade() bool {
	return p&2 == 2
}

func (p APIKeyPermission) CanWithdraw() bool {
	return p&4 == 4
}

type Balance struct {
	FundingValues   *okx.FundingAssetValuation
	FundingBalances []*okx.FundingAssetBalance
	TradingBalances *okx.AccountBalance

	UpdatedTs time.Time
}

func (b Balance) ToTypesBalance() *ctypes.Balance {
	assets := make([]*ctypes.AssetBo, 0)
	for _, balance := range b.FundingBalances {
		_balance := decimal.RequireFromString(balance.Balance)
		_freezed := decimal.RequireFromString(balance.FrozenBalance)
		assets = append(assets, &ctypes.AssetBo{
			WalletType: ctypes.WalletTypeFund,
			Code:       balance.Ccy,
			Balance:    _balance,
			Locked:     _freezed,
			UpdatedTs:  b.UpdatedTs,
		})
	}

	for _, balance := range b.TradingBalances.Details {
		_balance := number.DecimalFromString(balance.CashBal)
		_freezed := number.DecimalFromString(balance.FrozenBal)
		_margin := number.DecimalFromString(balance.Imr)
		// okx 口径的冻结资产包含了保证金，需要减去保证金
		_locked := _freezed.Sub(_margin)
		if _locked.LessThan(decimal.Zero) {
			_locked = decimal.Zero
		}
		assets = append(assets, &ctypes.AssetBo{
			WalletType: ctypes.WalletTypeTrade,
			Code:       balance.Ccy,
			Balance:    _balance,
			Locked:     _locked,
			UpdatedTs:  b.UpdatedTs,
		})
	}
	return &ctypes.Balance{
		Assets: assets,
	}
}

func GetOrderType(order *okx.Order) ctypes.OrderType {
	switch order.OrdType {
	case "market": // 市价单
		return ctypes.OrderTypeMarket
	case "limit": // 限价单
		return ctypes.OrderTypeLimit
	case "post_only": // 只做maker单
		return ctypes.OrderTypeLimit
	case "fok": // 全部成交或立即取消
		return ctypes.OrderTypeLimit
	case "ioc": // 立即成交并取消剩余
		return ctypes.OrderTypeLimit
	case "optimal_limit_ioc": // 市价委托立即成交并取消剩余（仅适用交割、永续）
		return ctypes.OrderTypeLimit
	}
	return ctypes.OrderTypeUnknown
}

func GetAlgoType(order *okx.Order) ctypes.AlgoType {
	switch order.OrdType {
	case "market", "limit", "post_only", "fok", "ioc", "optimal_limit_ioc":
		return ctypes.AlgoTypeNone
	case "conditional": // 普通条件单
		return ctypes.AlgoTypeConditional
	case "trigger": // 计划委托单
		return ctypes.AlgoTypeConditional
	case "move_order_stop": // 移动止盈/止损单
		return ctypes.AlgoTypeTrailing
	case "oco": // 止盈止损单
		return ctypes.AlgoTypeOCO
	case "chase": // 追逐限价单
		return ctypes.AlgoTypeChase
	case "iceberg": // 冰山单
		return ctypes.AlgoTypeIceberg
	case "twap": // 时间加权单
		return ctypes.AlgoTypeTWAP
	}
	return ctypes.AlgoTypeUnknown
}

var MapOrderStatus2Types = map[string]ctypes.OrderStatus{
	"live":             ctypes.OrderStatusNew,
	"partially_filled": ctypes.OrderStatusPartialDone,
	"filled":           ctypes.OrderStatusDone,
	"canceled":         ctypes.OrderStatusCanceled,
	"mmp_canceled":     ctypes.OrderStatusCanceled,
}

// ConvertPriceWorkingType 转换 OKX 的触发价格类型到本地类型
func ConvertPriceWorkingType(okxType string) ctypes.PriceWorkingType {
	switch okxType {
	case "last":
		return ctypes.PriceWorkingTypeLatest
	case "index":
		return ctypes.PriceWorkingTypeIndex
	case "mark":
		return ctypes.PriceWorkingTypeMark
	default:
		return ctypes.PriceWorkingTypeLatest
	}
}

// GetOrderSource 根据 OKX 订单来源和种类判断订单来源
func GetOrderSource(order *okx.Order) ctypes.OrderSource {
	// 根据 source 字段判断
	switch order.Source {
	case "6": // 计划委托策略触发后的生成的普通单
		return ctypes.OrderSourceStrategy
	case "7": // 止盈止损策略触发后的生成的普通单
		return ctypes.OrderSourceStrategy
	default:
		// 默认为用户订单
		return ctypes.OrderSourceUser
	}
}
