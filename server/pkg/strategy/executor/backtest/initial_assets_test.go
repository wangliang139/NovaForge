package backtest

import (
	"strings"
	"testing"

	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestValidateInitialAssetsForSymbols_SharedUSDTSpot(t *testing.T) {
	exchange := ctypes.ExchangeBinance
	symbols := []*stypes.BacktestSymbol{
		{Exchange: exchange, Symbol: ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)},
		{Exchange: exchange, Symbol: ctypes.NewSymbol("ETH", "USDT", ctypes.MarketTypeSpot)},
	}
	assets := []ctypes.AssetInput{
		{Asset: "USDT", WalletType: ctypes.WalletTypeSpot, Total: "1000", Frozen: "0"},
	}
	if err := ValidateInitialAssetsForSymbols(exchange, symbols, assets); err != nil {
		t.Fatalf("expected valid shared USDT pool, got %v", err)
	}
}

func TestValidateInitialAssetsForSymbols_MissingQuoteForFuture(t *testing.T) {
	exchange := ctypes.ExchangeBinance
	symbols := []*stypes.BacktestSymbol{
		{Exchange: exchange, Symbol: ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture)},
	}
	assets := []ctypes.AssetInput{
		{Asset: "BTC", WalletType: ctypes.WalletTypeFuture, Total: "1", Frozen: "0"},
	}
	err := ValidateInitialAssetsForSymbols(exchange, symbols, assets)
	if err == nil || !strings.Contains(err.Error(), "quote asset USDT") {
		t.Fatalf("expected missing quote error, got %v", err)
	}
}

func TestValidateInitialAssetsForSymbols_OKXTradeWallet(t *testing.T) {
	exchange := ctypes.ExchangeOkx
	symbols := []*stypes.BacktestSymbol{
		{Exchange: exchange, Symbol: ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)},
		{Exchange: exchange, Symbol: ctypes.NewSymbol("ETH", "USDT", ctypes.MarketTypeSpot)},
	}
	assets := []ctypes.AssetInput{
		{Asset: "USDT", WalletType: ctypes.WalletTypeTrade, Total: "5000", Frozen: "0"},
	}
	if err := ValidateInitialAssetsForSymbols(exchange, symbols, assets); err != nil {
		t.Fatalf("expected valid OKX trade wallet, got %v", err)
	}
}
