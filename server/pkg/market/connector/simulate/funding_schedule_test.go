package simulate

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestFundingQuoteWalletDeltaOneWay(t *testing.T) {
	mark := decimal.NewFromInt(100)
	rate := decimal.RequireFromString("0.0001")
	mult := decimal.NewFromInt(1)
	// Long 2: pay 2*100*0.0001 = 0.02
	slot := &PerpSlot{Mode: PositionModeOneWay, Net: Position{Qty: decimal.NewFromInt(2)}}
	delta := fundingQuoteWalletDelta(slot, mark, rate, mult)
	require.True(t, delta.Equal(decimal.RequireFromString("-0.02")))
	// Short net -2: receive +0.02
	slot2 := &PerpSlot{Mode: PositionModeOneWay, Net: Position{Qty: decimal.NewFromInt(-2)}}
	delta2 := fundingQuoteWalletDelta(slot2, mark, rate, mult)
	require.True(t, delta2.Equal(decimal.RequireFromString("0.02")))
}

func TestFundingQuoteWalletDeltaHedge(t *testing.T) {
	mark := decimal.NewFromInt(50)
	rate := decimal.RequireFromString("0.0002")
	mult := decimal.NewFromInt(1)
	// m = 50 * 0.0002 = 0.01 per coin; short 1 long 2 -> (1-2)*0.01 = -0.01
	slot := &PerpSlot{
		Mode:  PositionModeHedge,
		Long:  PerpLeg{Qty: decimal.NewFromInt(2)},
		Short: PerpLeg{Qty: decimal.NewFromInt(1)},
	}
	delta := fundingQuoteWalletDelta(slot, mark, rate, mult)
	require.True(t, delta.Equal(decimal.RequireFromString("-0.01")))
}

func TestFundingQuoteWalletDeltaZeroRate(t *testing.T) {
	slot := &PerpSlot{Mode: PositionModeOneWay, Net: Position{Qty: decimal.NewFromInt(10)}}
	require.True(t, fundingQuoteWalletDelta(slot, decimal.NewFromInt(100), decimal.Zero, decimal.NewFromInt(1)).IsZero())
}

func TestFundingSettlementKeyUsesStaleExchangeTime(t *testing.T) {
	now := time.Date(2026, 5, 1, 8, 0, 2, 0, time.UTC)
	staleFundingTime := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	retryAt := now.Add(fundingRetryDelay)
	fr := &ctypes.FundingRate{NextFundingTime: staleFundingTime}

	require.True(t, fundingSettlementKey(now, now, fr).Equal(staleFundingTime))
	require.True(t, fundingSettlementKey(retryAt, retryAt, fr).Equal(staleFundingTime))
}

func TestFundingNextScheduleRetriesWhenExchangeTimeIsStale(t *testing.T) {
	now := time.Date(2026, 5, 1, 8, 0, 2, 0, time.UTC)
	staleFundingTime := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	fr := &ctypes.FundingRate{NextFundingTime: staleFundingTime}

	require.True(t, fundingNextSchedule(now, fr).Equal(now.Add(fundingRetryDelay)))
}

func TestEngineSettleFundingPublishesFundingFeeIncrement(t *testing.T) {
	rt := &VenueRuntime{Exchange: ctypes.ExchangeBinance}
	eng := NewEngine().WithRuntime(rt)
	_, paper := btcFutureSym()
	require.NoError(t, eng.RegisterInstrument(testPerpInstrument(paper)))
	eng.InitBalances("a1", map[ctypes.WalletType]map[Asset]decimal.Decimal{
		ctypes.WalletTypeFuture: {"USDT": decimal.NewFromInt(1000)},
	})
	eng.Ledger().SeedOneWayNet("a1", paper, Position{
		Qty:        decimal.NewFromInt(1),
		EntryPrice: decimal.NewFromInt(100),
		UsedMargin: decimal.NewFromInt(10),
		Leverage:   10,
	})
	ch := make(chan AccountEvent, 8)
	rt.accountPublishCh = ch
	eng.settleFunding(paper, decimal.NewFromInt(100), decimal.RequireFromString("0.01"))
	select {
	case ev := <-ch:
		require.Equal(t, AccountEventTypeBalance, ev.kind)
		require.Equal(t, "a1", ev.accountID)
		require.Nil(t, ev.balance)
		require.NotNil(t, ev.update)
		require.Equal(t, ctypes.UpdateTypeIncrement, ev.update.Type)
		require.Equal(t, ctypes.LedgerReasonFundingFee, ev.update.Reason)
		require.Len(t, ev.update.Assets, 1)
		require.Equal(t, ctypes.WalletTypeFuture, ev.update.Assets[0].WalletType)
		require.Equal(t, "USDT", ev.update.Assets[0].Code)
		require.NotNil(t, ev.update.Assets[0].Balance)
		require.True(t, ev.update.Assets[0].Balance.Equal(decimal.NewFromInt(-1)))

		msg := rt.accountEventToMessage(ev)
		require.NotNil(t, msg)
		require.NotNil(t, msg.BalanceUpdate)
		require.Equal(t, ctypes.UpdateTypeIncrement, msg.BalanceUpdate.Type)
		require.Equal(t, ctypes.LedgerReasonFundingFee, msg.BalanceUpdate.Reason)
	default:
		t.Fatal("expected balance event")
	}
	select {
	case <-ch:
		t.Fatal("unexpected extra event")
	default:
	}
}
