package types

import (
	"testing"

	"github.com/bytedance/sonic"
)

func TestParseSymbol(t *testing.T) {
	tests := []struct {
		str    string
		symbol *Symbol
		err    error
	}{
		{
			str: "BTC/USDT",
			symbol: &Symbol{
				Base:  "BTC",
				Quote: "USDT",
				Type:  MarketTypeSpot,
			},
			err: nil,
		},
		{
			str: "btc/usdt:future",
			symbol: &Symbol{
				Base:  "BTC",
				Quote: "USDT",
				Type:  MarketTypeFuture,
			},
			err: nil,
		},
	}
	for _, test := range tests {
		symbol, err := ParseSymbol(test.str)
		if err != nil {
			t.Errorf("failed to parse symbol: %v", err)
			continue
		}
		if symbol.Base != test.symbol.Base {
			t.Errorf("expected base to be %s, got %s", test.symbol.Base, symbol.Base)
		}
		if symbol.Quote != test.symbol.Quote {
			t.Errorf("expected quote to be %s, got %s", test.symbol.Quote, symbol.Quote)
		}
		if symbol.Type != test.symbol.Type {
			t.Errorf("expected type to be %s, got %s", test.symbol.Type, symbol.Type)
		}
	}
}

func TestSymbolMarshal(t *testing.T) {
	symbol := Symbol{
		Base:  "BTC",
		Quote: "USDT",
		Type:  MarketTypeSpot,
	}
	marshal, err := sonic.Marshal(symbol)
	if err != nil {
		t.Errorf("failed to marshal symbol: %v", err)
	}
	t.Logf("marshal: %s", string(marshal))
	var unmarshal Symbol
	err = sonic.Unmarshal(marshal, &unmarshal)
	if err != nil {
		t.Errorf("failed to unmarshal symbol: %v", err)
	}
	if unmarshal.Base != symbol.Base {
		t.Errorf("expected base to be %s, got %s", symbol.Base, unmarshal.Base)
	}
	if unmarshal.Quote != symbol.Quote {
		t.Errorf("expected quote to be %s, got %s", symbol.Quote, unmarshal.Quote)
	}
}