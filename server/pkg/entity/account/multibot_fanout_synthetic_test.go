package account

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestNewSyntheticAccountRawBalanceUpdateEnvelope(t *testing.T) {
	parent := "p1"
	sub := "s1"
	ex := ctypes.ExchangeBinance
	ts := time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)
	bu := &ctypes.BalanceUpdate{
		Type:   ctypes.UpdateTypeIncrement,
		Reason: ctypes.LedgerReasonFundingFee,
		Assets: []*ctypes.AssetEvent{
			{
				WalletType: ctypes.WalletTypeFuture,
				Code:       "USDT",
				Balance:    lo.ToPtr(decimal.RequireFromString("-0.1")),
				UpdatedTs:  ts,
			},
		},
	}
	env := newSyntheticAccountRawBalanceUpdateEnvelope(parent, ex, sub, bu)
	if env == nil || env.Payload == nil || env.Payload.BalanceUpdate == nil {
		t.Fatal("envelope")
	}
	if !env.Synthetic || env.SourceParentID != parent {
		t.Fatalf("meta %+v", env)
	}
	if env.Stream != ctypes.StreamTypeAccountRaw || *env.Account != sub {
		t.Fatal(env.Stream, env.Account)
	}
}

func TestNewSyntheticAccountRawSymbolLeverageEnvelope(t *testing.T) {
	parent := "p1"
	sub := "s1"
	ex := ctypes.ExchangeBinance
	sl := &ctypes.SymbolLeverage{
		Exchange:  ex,
		Symbol:    ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture),
		Side:      ctypes.PositionSideLong,
		Leverage:  5,
		UpdatedTs: time.Date(2026, 4, 13, 15, 0, 0, 0, time.UTC),
	}
	env := newSyntheticAccountRawSymbolLeverageEnvelope(parent, ex, sub, sl)
	if env == nil || env.Payload == nil || env.Payload.SymbolLeverage == nil {
		t.Fatal("envelope")
	}
	if !env.Synthetic || env.SourceParentID != parent {
		t.Fatalf("meta %+v", env)
	}
	if env.Stream != ctypes.StreamTypeAccountRaw || *env.Account != sub {
		t.Fatal("stream/account")
	}
}

func TestNewSyntheticAccountRawOrderEnvelope(t *testing.T) {
	parent := "parent-1"
	sub := "sub-1"
	ex := ctypes.ExchangeBinance
	ord := ctypes.Order{
		OrderID:   "oid1",
		AccountID: sub,
		Symbol:    ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot),
		UpdatedTs: time.Date(2026, 4, 13, 1, 2, 3, 0, time.UTC),
		ExecutedQty: decimal.NewFromInt(1),
	}
	env := newSyntheticAccountRawOrderEnvelope(parent, ex, sub, ord)
	if env == nil || env.Payload == nil || env.Payload.Order == nil {
		t.Fatal("envelope")
	}
	if !env.Synthetic || env.SourceParentID != parent {
		t.Fatalf("synthetic meta: %+v", env)
	}
	if env.Stream != ctypes.StreamTypeAccountRaw || *env.Account != sub {
		t.Fatalf("stream/account: %+v", env)
	}
	if env.Exchange != ex.String() {
		t.Fatalf("exchange %q", env.Exchange)
	}
}
