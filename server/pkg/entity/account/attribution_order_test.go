package account

import (
	"testing"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestFutureOpenPositionLikeDeriveOrderLocked(t *testing.T) {
	longOpen := ctypes.Order{
		Symbol: ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture),
		Side:   ctypes.PositionSideLong,
		IsBuy:  true,
	}
	if !futureOpenPositionLikeDeriveOrderLocked(longOpen) {
		t.Fatal("expected long+buy open")
	}
	longClose := ctypes.Order{
		Symbol: ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture),
		Side:   ctypes.PositionSideLong,
		IsBuy:  false,
	}
	if futureOpenPositionLikeDeriveOrderLocked(longClose) {
		t.Fatal("expected long+sell not open")
	}
	spot := ctypes.Order{Symbol: ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot), Side: ctypes.PositionSideLong, IsBuy: true}
	if futureOpenPositionLikeDeriveOrderLocked(spot) {
		t.Fatal("spot must not be future-open")
	}
}

func TestBuildSubRawDispatchesFromUnitShares_conserves112(t *testing.T) {
	ord := ctypes.Order{OrderID: "x", Symbol: ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)}
	subs := []SubWeight{
		{SubAccountID: "subA", W: decimal.NewFromInt(1)},
		{SubAccountID: "subB", W: decimal.NewFromInt(1)},
	}
	wUnalloc := decimal.NewFromInt(2)
	shares, parentAbs, err := SplitProportionalDelta(decimal.NewFromInt(1), subs, wUnalloc)
	if err != nil {
		t.Fatal(err)
	}
	if !parentAbs.Equal(decimal.RequireFromString("0.5")) {
		t.Fatalf("parent absorb unit share: got %s want 0.5", parentAbs)
	}
	disp := buildSubRawDispatchesFromUnitShares(ord, shares)
	if len(disp) != 2 {
		t.Fatalf("len %d", len(disp))
	}
	var sum decimal.Decimal
	for _, d := range disp {
		sum = sum.Add(d.Share)
		if d.Order.AccountID != d.SubAccountID {
			t.Fatalf("order account not rewritten: %+v", d)
		}
	}
	if !sum.Equal(decimal.RequireFromString("0.5")) {
		t.Fatalf("sum child shares %s want 0.5", sum)
	}
	if !sum.Add(parentAbs).Equal(decimal.NewFromInt(1)) {
		t.Fatalf("conservation: %s + %s", sum, parentAbs)
	}
}

func TestScaleOrderForShare(t *testing.T) {
	share := decimal.RequireFromString("0.25")
	fee := decimal.RequireFromString("4")
	pnl := decimal.RequireFromString("-8")
	ord := ctypes.Order{
		OriginalQty:      decimal.NewFromInt(100),
		ExecutedQty:      decimal.NewFromInt(40),
		OriginalQuoteQty: decimal.NewFromInt(200),
		ExecutedQuoteQty: decimal.NewFromInt(80),
		Fee:              &fee,
		RealizedPnl:      &pnl,
	}
	out := scaleOrderForShare(ord, share)
	if !out.OriginalQty.Equal(decimal.NewFromInt(25)) {
		t.Fatalf("OriginalQty %s", out.OriginalQty)
	}
	if !out.ExecutedQty.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("ExecutedQty %s", out.ExecutedQty)
	}
	if out.Fee == nil || !out.Fee.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("Fee %+v", out.Fee)
	}
	if out.RealizedPnl == nil || !out.RealizedPnl.Equal(decimal.NewFromInt(-2)) {
		t.Fatalf("Pnl %+v", out.RealizedPnl)
	}
}

func TestBuildSubRawDispatchesFromUnitShares_sortedIDs(t *testing.T) {
	ord := ctypes.Order{}
	shares := map[string]decimal.Decimal{"z": decimal.RequireFromString("0.1"), "a": decimal.RequireFromString("0.2")}
	d := buildSubRawDispatchesFromUnitShares(ord, shares)
	if len(d) != 2 || d[0].SubAccountID != "a" || d[1].SubAccountID != "z" {
		t.Fatalf("want sorted a,z got %#v", d)
	}
}
