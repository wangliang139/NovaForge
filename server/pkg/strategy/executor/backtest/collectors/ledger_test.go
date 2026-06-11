package collectors

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestLedgerCollectorBalanceDeltaAndSnapshot(t *testing.T) {
	c := NewLedgerCollector()
	ex := ctypes.ExchangeBinance
	sym := ctypes.Symbol{Base: "ETH", Quote: "USDT", Type: ctypes.MarketTypeFuture}
	aid := "binance"
	ts := time.Unix(1_700_000_000, 0)

	c.OnBalanceSnapshot(&stypes.BalanceSignal{
		BaseSignal: stypes.BaseSignal{
			Exchange:  &ex,
			Symbol:    &sym,
			AccountID: &aid,
			Ts:        ts,
		},
		WalletType: ctypes.WalletTypeTrade,
		Asset:      "USDT",
		Free:       decimal.RequireFromString("1000"),
		Frozen:     decimal.Zero,
	})

	c.OnBalanceDelta(&stypes.BalanceDeltaSignal{
		BaseSignal: stypes.BaseSignal{
			Exchange:  &ex,
			Symbol:    &sym,
			AccountID: &aid,
			Ts:        ts.Add(time.Second),
		},
		WalletType: ctypes.WalletTypeTrade,
		Asset:      "USDT",
		Free:       decimal.RequireFromString("-0.1"),
	})

	entries := c.GetLedgers()
	require.GreaterOrEqual(t, len(entries), 2)
	require.Equal(t, ctypes.LedgerReasonSnapshot, entries[0].Type)
	require.Equal(t, ctypes.LedgerReasonFill, entries[1].Type)
}
