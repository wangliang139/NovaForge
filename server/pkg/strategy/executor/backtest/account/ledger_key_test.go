package account

import (
	"testing"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestResolveWalletTypePrefersSignal(t *testing.T) {
	spot := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)
	got := resolveWalletType(ctypes.ExchangeBinance, &spot, ctypes.WalletTypeFuture)
	if got != ctypes.WalletTypeFuture {
		t.Fatalf("expected signal wallet type, got %s", got)
	}
}

func TestResolveWalletTypeFallsBackToSymbol(t *testing.T) {
	spot := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)
	got := resolveWalletType(ctypes.ExchangeBinance, &spot, "")
	if got != ctypes.WalletTypeSpot {
		t.Fatalf("expected spot wallet, got %s", got)
	}

	future := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture)
	got = resolveWalletType(ctypes.ExchangeBinance, &future, "")
	if got != ctypes.WalletTypeFuture {
		t.Fatalf("expected future wallet, got %s", got)
	}
}

func TestLedgerKeyForSymbolOKXUsesTrade(t *testing.T) {
	spot := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)
	key := ledgerKeyForSymbol(ctypes.ExchangeOkx, spot, "USDT")
	if key.WalletType != ctypes.WalletTypeTrade || key.Asset != "USDT" {
		t.Fatalf("unexpected key: %+v", key)
	}

	future := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture)
	key = ledgerKeyForSymbol(ctypes.ExchangeOkx, future, "USDT")
	if key.WalletType != ctypes.WalletTypeTrade {
		t.Fatalf("OKX future should map to trade wallet, got %s", key.WalletType)
	}
}

func TestNewLedgerKeyNormalizesAsset(t *testing.T) {
	key := newLedgerKey(ctypes.WalletTypeSpot, " usdt ")
	if key.Asset != "USDT" {
		t.Fatalf("expected normalized asset USDT, got %s", key.Asset)
	}
}
