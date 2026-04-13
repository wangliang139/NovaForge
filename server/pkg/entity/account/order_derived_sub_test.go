package account

import (
	"testing"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestFilterPositionsByExchange(t *testing.T) {
	ex := ctypes.ExchangeBinance
	other := ctypes.ExchangeOkx
	pos := []*ctypes.Position{
		{Exchange: ex, Symbol: ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture)},
		nil,
		{Exchange: other, Symbol: ctypes.NewSymbol("ETH", "USDT", ctypes.MarketTypeFuture)},
	}
	out := filterPositionsByExchange(pos, ex)
	if len(out) != 1 || out[0].Symbol.Base != "BTC" {
		t.Fatalf("got %#v", out)
	}
}
