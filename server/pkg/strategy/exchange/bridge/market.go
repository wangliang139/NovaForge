package bridge

import (
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

// MarketEvent gateway -> matching engine（同步调用版本，仅保留撮合所需字段）。
type MarketEvent struct {
	Ts time.Time

	Exchange ctypes.Exchange
	Symbol   ctypes.Symbol

	// Phase 表示 bar 的阶段，用于区分 open/close 两个时间点的撮合语义。
	Phase MarketPhase

	Open   decimal.Decimal
	High   decimal.Decimal
	Low    decimal.Decimal
	Close  decimal.Decimal
	Volume decimal.Decimal
}

type MarketPhase string

const (
	MarketPhaseOpen  MarketPhase = "open"
	MarketPhaseClose MarketPhase = "close"
)
