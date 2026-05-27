package backtest

import (
	"strings"
	"testing"

	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestValidateSingleExchangeSymbolsAllowsMultipleSymbolsOnOneExchange(t *testing.T) {
	symbols := []*stypes.BacktestSymbol{
		{
			Exchange: ctypes.ExchangeBinance,
			Symbol:   ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot),
		},
		{
			Exchange: ctypes.ExchangeBinance,
			Symbol:   ctypes.NewSymbol("ETH", "USDT", ctypes.MarketTypeSpot),
		},
	}

	if err := ValidateSingleExchangeSymbols(symbols); err != nil {
		t.Fatalf("expected one-exchange symbols to be valid, got %v", err)
	}
}

func TestValidateSingleExchangeSymbolsRejectsMultipleExchanges(t *testing.T) {
	symbols := []*stypes.BacktestSymbol{
		{
			Exchange: ctypes.ExchangeBinance,
			Symbol:   ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot),
		},
		{
			Exchange: ctypes.ExchangeOkx,
			Symbol:   ctypes.NewSymbol("ETH", "USDT", ctypes.MarketTypeSpot),
		},
	}

	err := ValidateSingleExchangeSymbols(symbols)
	if err == nil {
		t.Fatal("expected multiple exchanges to be rejected")
	}
	if !strings.Contains(err.Error(), "only supports one exchange") {
		t.Fatalf("unexpected error: %v", err)
	}
}
