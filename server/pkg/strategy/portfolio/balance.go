package portfolio

import (
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

type BalanceStore struct {
	Asset      string
	WalletType ctypes.WalletType
	Free       decimal.Decimal
	Frozen     decimal.Decimal
	UpdateAt   int64
}

// Apply 应用资金变更增量（delta 语义）
func (b BalanceStore) ApplyDelta(e stypes.BalanceDeltaSignal) BalanceStore {
	// log.Info().Str("asset", b.Asset).Str("free", e.Free.String()).Str("frozen", e.Frozen.String()).Msg("ApplyDelta signal")
	if e.GetTimestamp().UnixNano() < b.UpdateAt {
		return b
	}

	// log.Info().Str("asset", b.Asset).Str("free", b.Free.String()).Str("frozen", b.Frozen.String()).Msg("ApplyDelta before")

	// BalanceSignal 为 delta 语义：Free 和 Frozen 是增量，需要累加
	b.Free = b.Free.Add(e.Free)
	b.Frozen = b.Frozen.Add(e.Frozen)

	// 防止负值
	// if b.Free.IsNegative() {
	// 	b.Free = decimal.Zero
	// }
	// if b.Frozen.IsNegative() {
	// 	b.Frozen = decimal.Zero
	// }

	b.UpdateAt = e.GetTimestamp().UnixNano()

	// log.Info().Str("asset", b.Asset).Str("free", b.Free.String()).Str("frozen", b.Frozen.String()).Msg("ApplyDelta result")
	return b
}

func (b BalanceStore) ApplySnapshot(e stypes.BalanceSignal) BalanceStore {
	if e.GetTimestamp().UnixNano() < b.UpdateAt {
		return b
	}
	b.Free = e.Free
	b.Frozen = e.Frozen
	b.UpdateAt = e.GetTimestamp().UnixNano()
	return b
}
