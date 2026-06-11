package account

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/clock"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func newTestAccount(exchange ctypes.Exchange) *account {
	clk := clock.NewBacktestClock(time.Unix(100, 0))
	return NewAccount(string(exchange), AccountConfig{Exchange: exchange}, nil, clk)
}

func applySnapshot(t *testing.T, acc *account, exchange ctypes.Exchange, symbol ctypes.Symbol, wt ctypes.WalletType, asset string, free decimal.Decimal) {
	t.Helper()
	ctx := context.Background()
	err := acc.ApplyBalanceSnapshot(ctx, &stypes.BalanceSignal{
		BaseSignal: stypes.BaseSignal{
			Exchange:  ptr(exchange),
			Symbol:    &symbol,
			AccountID: ptr(string(exchange)),
			Ts:        time.Unix(100, 0),
		},
		WalletType: wt,
		Asset:      asset,
		Free:       free,
		Frozen:     decimal.Zero,
	})
	if err != nil {
		t.Fatalf("apply snapshot: %v", err)
	}
}

func TestOKXSharedUSDTBetweenSpotAndFuture(t *testing.T) {
	ctx := context.Background()
	acc := newTestAccount(ctypes.ExchangeOkx)
	spotSym := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)
	futureSym := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture)

	applySnapshot(t, acc, ctypes.ExchangeOkx, spotSym, ctypes.WalletTypeTrade, "USDT", decimal.RequireFromString("5000"))

	spotAsset, err := acc.GetAsset(ctx, "okx", spotSym, "USDT")
	if err != nil {
		t.Fatalf("get spot asset: %v", err)
	}
	futureAsset, err := acc.GetAsset(ctx, "okx", futureSym, "USDT")
	if err != nil {
		t.Fatalf("get future asset: %v", err)
	}

	if !spotAsset.Balance.Equal(decimal.RequireFromString("5000")) || !futureAsset.Balance.Equal(decimal.RequireFromString("5000")) {
		t.Fatalf("OKX should share USDT pool: spot=%s future=%s", spotAsset.Balance, futureAsset.Balance)
	}
	if spotAsset.WalletType != ctypes.WalletTypeTrade || futureAsset.WalletType != ctypes.WalletTypeTrade {
		t.Fatalf("unexpected wallet types: spot=%s future=%s", spotAsset.WalletType, futureAsset.WalletType)
	}

	if err := acc.FreezeFunds(ctx, "okx", futureSym, "USDT", decimal.RequireFromString("200"), nil); err != nil {
		t.Fatalf("freeze on future symbol: %v", err)
	}

	spotAsset, err = acc.GetAsset(ctx, "okx", spotSym, "USDT")
	if err != nil {
		t.Fatalf("get spot after freeze: %v", err)
	}
	if !spotAsset.Locked.Equal(decimal.RequireFromString("200")) {
		t.Fatalf("freeze on future should affect shared pool: locked=%s", spotAsset.Locked)
	}
}

func TestBinanceSpotFutureUSDTIsolated(t *testing.T) {
	ctx := context.Background()
	acc := newTestAccount(ctypes.ExchangeBinance)
	spotSym := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)
	futureSym := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture)

	applySnapshot(t, acc, ctypes.ExchangeBinance, spotSym, ctypes.WalletTypeSpot, "USDT", decimal.RequireFromString("1000"))
	applySnapshot(t, acc, ctypes.ExchangeBinance, futureSym, ctypes.WalletTypeFuture, "USDT", decimal.RequireFromString("2000"))

	spotAsset, err := acc.GetAsset(ctx, "binance", spotSym, "USDT")
	if err != nil {
		t.Fatalf("get spot asset: %v", err)
	}
	futureAsset, err := acc.GetAsset(ctx, "binance", futureSym, "USDT")
	if err != nil {
		t.Fatalf("get future asset: %v", err)
	}

	if !spotAsset.Balance.Equal(decimal.RequireFromString("1000")) {
		t.Fatalf("spot balance=%s", spotAsset.Balance)
	}
	if !futureAsset.Balance.Equal(decimal.RequireFromString("2000")) {
		t.Fatalf("future balance=%s", futureAsset.Balance)
	}
	if spotAsset.WalletType != ctypes.WalletTypeSpot || futureAsset.WalletType != ctypes.WalletTypeFuture {
		t.Fatalf("wallet types spot=%s future=%s", spotAsset.WalletType, futureAsset.WalletType)
	}

	if err := acc.FreezeFunds(ctx, "binance", spotSym, "USDT", decimal.RequireFromString("100"), nil); err != nil {
		t.Fatalf("freeze spot: %v", err)
	}
	futureAsset, err = acc.GetAsset(ctx, "binance", futureSym, "USDT")
	if err != nil {
		t.Fatalf("get future after spot freeze: %v", err)
	}
	if !futureAsset.Locked.IsZero() {
		t.Fatalf("spot freeze should not affect future wallet: locked=%s", futureAsset.Locked)
	}
}

func TestApplyBalanceDeltaResolvesWalletFromSymbol(t *testing.T) {
	ctx := context.Background()
	acc := newTestAccount(ctypes.ExchangeBinance)
	spotSym := ctypes.NewSymbol("ETH", "USDT", ctypes.MarketTypeSpot)

	err := acc.ApplyBalanceDelta(ctx, &stypes.BalanceDeltaSignal{
		BaseSignal: stypes.BaseSignal{
			Exchange:  ptr(ctypes.ExchangeBinance),
			Symbol:    &spotSym,
			AccountID: ptr("binance"),
			Ts:        time.Unix(100, 0),
		},
		Asset: "USDT",
		Free:  decimal.RequireFromString("300"),
	})
	if err != nil {
		t.Fatalf("apply delta: %v", err)
	}

	asset, err := acc.GetAsset(ctx, "binance", spotSym, "USDT")
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	if !asset.Balance.Equal(decimal.RequireFromString("300")) || asset.WalletType != ctypes.WalletTypeSpot {
		t.Fatalf("delta routed to wrong wallet: balance=%s wt=%s", asset.Balance, asset.WalletType)
	}

	futureSym := ctypes.NewSymbol("ETH", "USDT", ctypes.MarketTypeFuture)
	futureAsset, err := acc.GetAsset(ctx, "binance", futureSym, "USDT")
	if err != nil {
		t.Fatalf("get future asset: %v", err)
	}
	if !futureAsset.Balance.IsZero() {
		t.Fatalf("delta should not touch future wallet: %s", futureAsset.Balance)
	}
}

func TestGetBalanceReturnsDistinctWalletTypes(t *testing.T) {
	ctx := context.Background()
	acc := newTestAccount(ctypes.ExchangeBinance)
	spotSym := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)
	futureSym := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture)

	applySnapshot(t, acc, ctypes.ExchangeBinance, spotSym, ctypes.WalletTypeSpot, "USDT", decimal.RequireFromString("100"))
	applySnapshot(t, acc, ctypes.ExchangeBinance, futureSym, ctypes.WalletTypeFuture, "USDT", decimal.RequireFromString("200"))

	balances, err := acc.GetBalance(ctx, "binance")
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}

	var spotUSDT, futureUSDT decimal.Decimal
	for _, b := range balances {
		if b.Code != "USDT" {
			continue
		}
		switch b.WalletType {
		case ctypes.WalletTypeSpot:
			spotUSDT = b.Balance
		case ctypes.WalletTypeFuture:
			futureUSDT = b.Balance
		}
	}
	if !spotUSDT.Equal(decimal.RequireFromString("100")) || !futureUSDT.Equal(decimal.RequireFromString("200")) {
		t.Fatalf("unexpected balances: spot=%s future=%s", spotUSDT, futureUSDT)
	}
}

func TestFuturePositionStoredOnWalletLedger(t *testing.T) {
	ctx := context.Background()
	acc := newTestAccount(ctypes.ExchangeOkx)
	futureSym := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture)
	ex := ctypes.ExchangeOkx

	err := acc.ApplyPosition(ctx, &stypes.PositionSignal{
		BaseSignal: stypes.BaseSignal{
			Exchange:  &ex,
			Symbol:    &futureSym,
			AccountID: ptr("okx"),
			Ts:        time.Unix(100, 0),
		},
		Side:       ctypes.PositionSideLong,
		Qty:        decimal.RequireFromString("0.5"),
		EntryPrice: decimal.RequireFromString("40000"),
	})
	if err != nil {
		t.Fatalf("apply position: %v", err)
	}

	pos, err := acc.GetPosition(ctx, "okx", futureSym, ctypes.PositionSideLong)
	if err != nil {
		t.Fatalf("get position: %v", err)
	}
	if !pos.Amount.Equal(decimal.RequireFromString("0.5")) {
		t.Fatalf("unexpected position qty: %s", pos.Amount)
	}

	// 同一 trade 钱包 ledger 上仍可入账 USDT
	applySnapshot(t, acc, ctypes.ExchangeOkx, futureSym, ctypes.WalletTypeTrade, "USDT", decimal.RequireFromString("1000"))
	asset, err := acc.GetAsset(ctx, "okx", futureSym, "USDT")
	if err != nil {
		t.Fatalf("get margin asset: %v", err)
	}
	if !asset.Balance.Equal(decimal.RequireFromString("1000")) {
		t.Fatalf("margin balance=%s", asset.Balance)
	}
}
