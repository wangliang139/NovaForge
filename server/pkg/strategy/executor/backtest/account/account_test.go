package account

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	mb "github.com/wangliang139/NovaForge/server/pkg/strategy/infra/bus"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/clock"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type captureBus struct {
	last stypes.Signal
}

func (b *captureBus) Subscribe(mb.Handler, int, ...mb.Filter) (mb.SubscriptionID, error) {
	return "", nil
}

func (b *captureBus) Unsubscribe(mb.SubscriptionID) error {
	return nil
}

func (b *captureBus) Publish(_ context.Context, sig stypes.Signal) error {
	b.last = sig
	return nil
}

func (b *captureBus) Start(context.Context) error {
	return nil
}

func (b *captureBus) Stop(context.Context) error {
	return nil
}

func TestFreezeFundsPublishesSnapshotWithoutChangingTotalOnReplay(t *testing.T) {
	ctx := context.Background()
	bus := &captureBus{}
	clk := clock.NewBacktestClock(time.Unix(100, 0))
	acc := NewAccount("binance", AccountConfig{Exchange: ctypes.ExchangeBinance}, bus, clk)
	symbol := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)

	if err := acc.ApplyBalanceSnapshot(ctx, &stypes.BalanceSignal{
		BaseSignal: stypes.BaseSignal{
			Exchange:  ptr(ctypes.ExchangeBinance),
			Symbol:    &symbol,
			AccountID: ptr("binance"),
			Ts:        clk.Now(),
		},
		WalletType: ctypes.WalletTypeTrade,
		Asset:      "USDT",
		Free:       decimal.RequireFromString("1000"),
		Frozen:     decimal.Zero,
	}); err != nil {
		t.Fatalf("apply initial balance: %v", err)
	}

	if err := acc.FreezeFunds(ctx, "binance", symbol, "USDT", decimal.RequireFromString("100"), nil); err != nil {
		t.Fatalf("freeze funds: %v", err)
	}

	asset, err := acc.GetAsset(ctx, "binance", symbol, "USDT")
	if err != nil {
		t.Fatalf("get asset after freeze: %v", err)
	}
	if !asset.Balance.Equal(decimal.RequireFromString("1000")) || !asset.Locked.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("unexpected balance after freeze: balance=%s locked=%s", asset.Balance, asset.Locked)
	}

	snapshot, ok := bus.last.(*stypes.BalanceSignal)
	if !ok {
		t.Fatalf("expected BalanceSignal, got %T", bus.last)
	}
	if err := acc.ApplyBalanceSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("replay freeze snapshot: %v", err)
	}

	asset, err = acc.GetAsset(ctx, "binance", symbol, "USDT")
	if err != nil {
		t.Fatalf("get asset after replay: %v", err)
	}
	if !asset.Balance.Equal(decimal.RequireFromString("1000")) || !asset.Locked.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("snapshot replay changed totals: balance=%s locked=%s", asset.Balance, asset.Locked)
	}
}

func ptr[T any](v T) *T {
	return &v
}
