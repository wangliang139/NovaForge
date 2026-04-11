package types

// SignalScope 信号作用域
type SignalScope string

const (
	SignalScopeSymbol   SignalScope = "symbol"   // Symbol 级别：每个 symbol 需要独立数据源（动态，根据回测时的 symbols）
	SignalScopeTarget   SignalScope = "target"   // Target 级别：针对策略中指定的具体 symbol（固定）
	SignalScopeExchange SignalScope = "exchange" // Exchange 级别：每个交易所只需要一个数据源
	SignalScopeStrategy SignalScope = "strategy" // Strategy 级别：整个策略只需要一个数据源
)

func (s SignalScope) String() string {
	return string(s)
}

func (s SignalScope) Valid() bool {
	switch s {
	case SignalScopeSymbol, SignalScopeTarget, SignalScopeExchange, SignalScopeStrategy:
		return true
	}
	return false
}

// SignalType 信号类型
type SignalType string

const (
	SignalTypeKline     SignalType = "kline"
	SignalTypeTrade     SignalType = "trade"
	SignalTypeDepth     SignalType = "depth"
	SignalTypeTicker    SignalType = "ticker"
	SignalTypeMarkPrice SignalType = "mark_price"
	SignalTypeSocial    SignalType = "social"
	SignalTypeTimer     SignalType = "timer"
	SignalTypeOrder     SignalType = "order"
	SignalTypePosition  SignalType = "position"
	SignalTypeBalance   SignalType = "balance"
	SignalTypeFill      SignalType = "fill"
	SignalTypeLeverage  SignalType = "leverage"
	SignalTypeRisk      SignalType = "risk"
	SignalTypeSystem    SignalType = "system"
	SignalTypeTest      SignalType = "test"
	// 指数价格、资金费率 等
)

func (t SignalType) String() string {
	return string(t)
}

func (t SignalType) Valid() bool {
	switch t {
	case SignalTypeKline, SignalTypeTrade, SignalTypeDepth,
		SignalTypeTicker, SignalTypeMarkPrice, SignalTypeSocial, SignalTypeTimer,
		SignalTypeOrder, SignalTypePosition, SignalTypeBalance,
		SignalTypeFill, SignalTypeLeverage, SignalTypeRisk, SignalTypeSystem, SignalTypeTest:
		return true
	}
	return false
}

func (t SignalType) IsMarketSignal() bool {
	switch t {
	case SignalTypeKline, SignalTypeTrade, SignalTypeDepth, SignalTypeTicker, SignalTypeMarkPrice:
		return true
	}
	return false
}

func (t SignalType) IsAccountSignal() bool {
	switch t {
	case SignalTypeBalance, SignalTypePosition, SignalTypeFill, SignalTypeLeverage, SignalTypeRisk:
		return true
	}
	return false
}

func (t SignalType) IsSymbolSignal() bool {
	switch t {
	case SignalTypeKline, SignalTypeTrade, SignalTypeDepth, SignalTypeTicker, SignalTypeMarkPrice:
		return true
	}
	return false
}

type SignalKind string

const (
	// 市场数据类
	SignalKindKline             SignalKind = "kline"
	SignalKindTrade             SignalKind = "trade"
	SignalKindDepth             SignalKind = "depth"
	SignalKindTicker            SignalKind = "ticker"
	SignalKindMarkPrice         SignalKind = "mark_price"
	SignalKindFundingRate       SignalKind = "funding_rate"
	SignalKindFundingSettlement SignalKind = "funding_settlement"

	// 订单意图类
	SignalKindPlaceIntent  SignalKind = "place_intent"
	SignalKindCancelIntent SignalKind = "cancel_intent"

	// 订单生命周期类
	SignalKindOrderLifecycle SignalKind = "order_lifecycle" // 订单生命周期（包含完整订单信息，由各生命周期事件触发）
	SignalKindOrderSnapshot  SignalKind = "order_snapshot"  // 订单快照（包含完整订单信息，由各生命周期事件触发）

	// 成交类
	SignalKindFill SignalKind = "fill" // 成交事件（部分成交/全部成交）

	// 账户 / 资产类
	SignalKindBalanceSnapshot SignalKind = "balance_snapshot" // 资金快照（包含完整资金信息，由各资金变更事件触发）
	SignalKindBalanceChanged  SignalKind = "balance_changed"
	SignalKindLeverageChanged SignalKind = "leverage_changed"

	// 投资组合 / 估值类
	SignalKindPositionSnapshot     SignalKind = "position_snapshot"
	SignalKindUnrealizedPnLUpdated SignalKind = "unrealized_pnl_updated"
	SignalKindEquityUpdated        SignalKind = "equity_updated"

	SignalKindSocial SignalKind = "social"
	SignalKindTimer  SignalKind = "timer"

	SignalKindSystem SignalKind = "system"
	SignalKindRisk   SignalKind = "risk"
	SignalKindTest   SignalKind = "test"
)
