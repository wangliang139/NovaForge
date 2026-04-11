package portfolio

import (
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
)

type PositionStore struct {
	Symbol        ctypes.ExSymbol
	Side          ctypes.PositionSide
	Qty           decimal.Decimal
	AvgPrice      decimal.Decimal
	UnrealizedPnL decimal.Decimal
	UpdateAt      int64
}

func (p PositionStore) ApplySnapshot(e stypes.PositionSignal) PositionStore {
	if e.GetTimestamp().UnixNano() < p.UpdateAt {
		return p
	}

	// 语义：快照覆盖
	p.Side = e.Side
	p.Qty = e.Qty
	if p.Qty.IsZero() {
		p.AvgPrice = decimal.Zero
		p.UpdateAt = e.GetTimestamp().UnixNano()
		return p
	}

	p.AvgPrice = e.EntryPrice
	p.UpdateAt = e.GetTimestamp().UnixNano()
	return p
}
