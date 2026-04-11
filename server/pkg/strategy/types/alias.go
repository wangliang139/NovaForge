package types

import (
	"github.com/wangliang139/llt-trade/server/pkg/types"
)

type (
	SignalType = types.SignalType
	SignalKind = types.SignalKind
)

var (
	SignalTypeKline     = types.SignalTypeKline
	SignalTypeTicker    = types.SignalTypeTicker
	SignalTypeTrade     = types.SignalTypeTrade
	SignalTypeDepth     = types.SignalTypeDepth
	SignalTypeMarkPrice = types.SignalTypeMarkPrice
	SignalTypeSocial    = types.SignalTypeSocial
	SignalTypeTimer     = types.SignalTypeTimer
	SignalTypeOrder     = types.SignalTypeOrder
	SignalTypeFill      = types.SignalTypeFill
	SignalTypePosition  = types.SignalTypePosition
	SignalTypeBalance   = types.SignalTypeBalance
	SignalTypeLeverage  = types.SignalTypeLeverage
	SignalTypeRisk      = types.SignalTypeRisk
	SignalTypeSystem    = types.SignalTypeSystem
)

var (
	SignalKindKline                = types.SignalKindKline
	SignalKindTrade                = types.SignalKindTrade
	SignalKindDepth                = types.SignalKindDepth
	SignalKindTicker               = types.SignalKindTicker
	SignalKindMarkPrice            = types.SignalKindMarkPrice
	SignalKindFundingRate          = types.SignalKindFundingRate
	SignalKindFundingSettlement    = types.SignalKindFundingSettlement
	SignalKindPlaceIntent          = types.SignalKindPlaceIntent
	SignalKindCancelIntent         = types.SignalKindCancelIntent
	SignalKindOrderLifecycle       = types.SignalKindOrderLifecycle
	SignalKindOrderSnapshot        = types.SignalKindOrderSnapshot
	SignalKindFill                 = types.SignalKindFill
	SignalKindBalanceSnapshot      = types.SignalKindBalanceSnapshot
	SignalKindBalanceChanged       = types.SignalKindBalanceChanged
	SignalKindLeverageChanged      = types.SignalKindLeverageChanged
	SignalKindUnrealizedPnLUpdated = types.SignalKindUnrealizedPnLUpdated
	SignalKindPositionSnapshot     = types.SignalKindPositionSnapshot
	SignalKindEquityUpdated        = types.SignalKindEquityUpdated
	SignalKindSystem               = types.SignalKindSystem
	SignalKindRisk                 = types.SignalKindRisk
	SignalKindSocial               = types.SignalKindSocial
	SignalKindTimer                = types.SignalKindTimer
	SignalKindTest                 = types.SignalKindTest
)
