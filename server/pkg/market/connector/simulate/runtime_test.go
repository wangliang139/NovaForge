package simulate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestAccountEventLeverageToMessage(t *testing.T) {
	sym := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture)
	rt := &VenueRuntime{Exchange: ctypes.ExchangeBinance}
	ev := AccountEvent{
		accountID: "acc1",
		symbol:    Symbol(sym.String()),
		kind:      AccountEventTypeLeverage,
		leverage:  &LeverageChange{leverage: 20, leverageSide: ctypes.PositionSideLong},
	}
	msg := rt.accountEventToMessage(ev)
	require.NotNil(t, msg)
	require.NotNil(t, msg.SymbolLeverage)
	require.Equal(t, 20, msg.SymbolLeverage.Leverage)
	require.Equal(t, ctypes.PositionSideLong, msg.SymbolLeverage.Side)
	require.True(t, msg.SymbolLeverage.Symbol.Equal(sym))
}

func TestPerpSlotToPositionsUpdateFullyFlatHedgeEmitsSnapshot(t *testing.T) {
	sym := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture)
	require.True(t, sym.IsValid())
	slot := &PerpSlot{Mode: PositionModeHedge}
	now := time.Now().UTC()
	pu := perpSlotToPositionsUpdate(ctypes.ExchangeBinance, "acc1", sym, slot, PositionModeHedge, "e1", now)
	require.NotNil(t, pu, "fully flat hedge must still publish snapshot so close flow clears positions downstream")
	require.Len(t, pu.Positions, 2)
	require.Equal(t, ctypes.PositionSideLong, pu.Positions[0].Side)
	require.Equal(t, ctypes.PositionSideShort, pu.Positions[1].Side)
}
