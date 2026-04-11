package types

import (
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

type BalanceView struct {
	Asset    string
	Free     decimal.Decimal
	Frozen   decimal.Decimal
	UpdateAt int64
}

type PositionView struct {
	Symbol        ctypes.ExSymbol
	Side          ctypes.PositionSide
	Leverage      int
	Qty           decimal.Decimal
	AvgPrice      decimal.Decimal
	UnrealizedPnL decimal.Decimal
	UpdateAt      int64
}

type PortfolioSnapshot struct {
	Positions map[ctypes.PositionKey]PositionView
	Balances  map[ctypes.AssetKey]BalanceView
	Ts        int64
}
