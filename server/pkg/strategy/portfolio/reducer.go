package portfolio

import (
	"context"

	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
)

type Reducer struct {
	positions map[ctypes.PositionKey]PositionStore
	balances  map[ctypes.AssetKey]BalanceStore
	// 杠杆缓存：按交易所 + 交易对存储
	leverages map[ctypes.PositionKey]int
}

func NewReducer() *Reducer {
	return &Reducer{
		positions: make(map[ctypes.PositionKey]PositionStore),
		balances:  make(map[ctypes.AssetKey]BalanceStore),
		leverages: make(map[ctypes.PositionKey]int),
	}
}

func (r *Reducer) Apply(ctx context.Context, sig stypes.Signal) error {
	var exSymbol ctypes.ExSymbol
	if sig.GetExchange() != nil && sig.GetSymbol() != nil {
		exSymbol = ctypes.NewExSymbol(*sig.GetExchange(), *sig.GetSymbol())
	}
	switch e := sig.(type) {
	case *stypes.PositionSignal:
		key := ctypes.PositionKey{
			ExSymbol: exSymbol,
			Side:     e.Side,
		}
		view := r.positions[key]
		view = view.ApplySnapshot(*e)
		view.Symbol = exSymbol
		r.positions[key] = view
	case *stypes.BalanceDeltaSignal:
		key := ctypes.AssetKey{
			Exchange:   *sig.GetExchange(),
			WalletType: e.WalletType,
			Asset:      e.Asset,
		}
		view := r.balances[key]
		view = view.ApplyDelta(*e)
		view.Asset = e.Asset
		view.WalletType = e.WalletType
		r.balances[key] = view
	case *stypes.BalanceSignal:
		key := ctypes.AssetKey{
			Exchange:   *sig.GetExchange(),
			WalletType: e.WalletType,
			Asset:      e.Asset,
		}
		view := r.balances[key]
		view = view.ApplySnapshot(*e)
		view.Asset = e.Asset
		view.WalletType = e.WalletType
		r.balances[key] = view
	case *stypes.LeverageChangedSignal:
		// 杠杆变更信号：直接覆盖当前值
		if sig.GetExchange() == nil || sig.GetSymbol() == nil {
			return nil
		}
		r.leverages[ctypes.PositionKey{
			ExSymbol: ctypes.NewExSymbol(*sig.GetExchange(), *sig.GetSymbol()),
			Side:     ctypes.PositionSideLong,
		}] = e.Leverage
		r.leverages[ctypes.PositionKey{
			ExSymbol: ctypes.NewExSymbol(*sig.GetExchange(), *sig.GetSymbol()),
			Side:     ctypes.PositionSideShort,
		}] = e.Leverage
	}
	return nil
}
