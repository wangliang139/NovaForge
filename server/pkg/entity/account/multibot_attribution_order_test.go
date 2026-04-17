package account

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/internal/consts"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

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
		if d.SubAccountID != "subA" && d.SubAccountID != "subB" {
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

func TestAllocateFieldAmongSubs_spotBaseRemainderToMax(t *testing.T) {
	m := &ctypes.Market{BaseAssetPrecision: 0, QuoteAssetPrecision: 2}
	shares := []subShare{{id: "a", s: decimal.RequireFromString("0.3")}, {id: "b", s: decimal.RequireFromString("0.3")}}
	// sumShares=0.6, sumTicks=100*0.6=60; a floor(30)=30, b gets 60-30=30
	got := allocateFieldAmongSubs(decimal.NewFromInt(100), shares, false, scaleFieldBaseQty, m)
	if !got["a"].Equal(decimal.NewFromInt(30)) || !got["b"].Equal(decimal.NewFromInt(30)) {
		t.Fatalf("got %#v", got)
	}
}

func TestAllocateFieldAmongSubs_futureLotRemainderToMax(t *testing.T) {
	lot := decimal.NewFromInt(1)
	m := &ctypes.Market{BaseAssetPrecision: 8, Rules: ctypes.MarketRules{LotSize: lot}}
	shares := []subShare{{id: "a", s: decimal.RequireFromString("0.31")}, {id: "b", s: decimal.RequireFromString("0.31")}}
	// t_a=3.1 floor lot 3, sumTicks=10*0.62=6.2, b 余量也按 lot floor 为 3，尘量 0.2 由父吸收
	got := allocateFieldAmongSubs(decimal.NewFromInt(10), shares, true, scaleFieldBaseQty, m)
	if !got["a"].Equal(decimal.NewFromInt(3)) || !got["b"].Equal(decimal.NewFromInt(3)) {
		t.Fatalf("got %#v", got)
	}
	step := marketLotStepBase(m)
	for sid, qty := range got {
		ticks := qty.Div(step)
		if !ticks.Equal(ticks.Floor()) {
			t.Fatalf("sub %s qty=%s is not lot multiple %s", sid, qty, step)
		}
	}
	sumChild := got["a"].Add(got["b"])
	sumTicks := decimal.NewFromInt(10).Mul(decimal.RequireFromString("0.62"))
	if sumChild.GreaterThan(sumTicks) {
		t.Fatalf("child sum should not exceed sumTicks: child=%s sumTicks=%s", sumChild, sumTicks)
	}
	if !sumTicks.Sub(sumChild).Equal(decimal.RequireFromString("0.2")) {
		t.Fatalf("parent dust absorb mismatch, got %s", sumTicks.Sub(sumChild))
	}
}

func TestIsFutureOpenPositionOrder(t *testing.T) {
	tests := []struct {
		name string
		ord  ctypes.Order
		want bool
	}{
		{
			name: "long buy open",
			ord:  ctypes.Order{Side: ctypes.PositionSideLong, IsBuy: true},
			want: true,
		},
		{
			name: "short sell open",
			ord:  ctypes.Order{Side: ctypes.PositionSideShort, IsBuy: false},
			want: true,
		},
		{
			name: "long sell close",
			ord:  ctypes.Order{Side: ctypes.PositionSideLong, IsBuy: false},
			want: false,
		},
		{
			name: "short buy close",
			ord:  ctypes.Order{Side: ctypes.PositionSideShort, IsBuy: true},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFutureOpenPositionOrder(tt.ord); got != tt.want {
				t.Fatalf("isFutureOpenPositionOrder()=%v want %v", got, tt.want)
			}
		})
	}
}

func TestAllocateFieldAmongSubs_moneyDefaultPrecision(t *testing.T) {
	m := (*ctypes.Market)(nil)
	shares := []subShare{{id: "a", s: decimal.RequireFromString("0.25")}, {id: "b", s: decimal.RequireFromString("0.25")}}
	got := allocateFieldAmongSubs(decimal.NewFromInt(4), shares, false, scaleFieldMoney, m)
	if !got["a"].Equal(decimal.NewFromInt(1)) || !got["b"].Equal(decimal.NewFromInt(1)) {
		t.Fatalf("fee split got %#v", got)
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

func TestFanoutSharesStableAndScalingConservativeWithoutRemainderTopup(t *testing.T) {
	parent := ctypes.Order{
		OrderID:          "ord-regression",
		Symbol:           ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot),
		OriginalQty:      decimal.NewFromInt(1),
		ExecutedQty:      decimal.NewFromInt(1),
		OriginalQuoteQty: decimal.NewFromInt(1),
		ExecutedQuoteQty: decimal.NewFromInt(1),
	}
	unitShares := map[string]decimal.Decimal{
		"subA": decimal.RequireFromString("0.3333333333"),
		"subB": decimal.RequireFromString("0.3333333333"),
		"subC": decimal.RequireFromString("0.3333333333"),
	}

	disp := buildSubRawDispatchesFromUnitShares(parent, unitShares)
	if len(disp) != 3 {
		t.Fatalf("want 3 dispatches, got %d", len(disp))
	}
	for _, d := range disp {
		wantShare := unitShares[d.SubAccountID]
		if !d.Share.Equal(wantShare) {
			t.Fatalf("share changed for %s: got=%s want=%s", d.SubAccountID, d.Share, wantShare)
		}
	}

	e := &Entity{}
	scaled, err := e.buildScaledOrdersForMultiBotFanout(t.Context(), ctypes.ExchangeBinance, &parent, disp)
	if err != nil {
		t.Fatal(err)
	}
	if len(scaled) != 3 {
		t.Fatalf("want 3 scaled orders, got %d", len(scaled))
	}

	wantScaledQty := floorDecimalPlaces(decimal.RequireFromString("0.3333333333"), int32(consts.DefaultAssetPrecision))
	var sumScaled decimal.Decimal
	for sid, ord := range scaled {
		if !ord.OriginalQty.Equal(wantScaledQty) {
			t.Fatalf("sub %s original qty got=%s want=%s", sid, ord.OriginalQty, wantScaledQty)
		}
		theoretical := parent.OriginalQty.Mul(unitShares[sid])
		if ord.OriginalQty.GreaterThan(theoretical) {
			t.Fatalf("sub %s should not exceed theoretical share: got=%s theoretical=%s", sid, ord.OriginalQty, theoretical)
		}
		sumScaled = sumScaled.Add(ord.OriginalQty)
	}
	if !sumScaled.LessThan(parent.OriginalQty) {
		t.Fatalf("sum scaled should be conservative and less than parent: sum=%s parent=%s", sumScaled, parent.OriginalQty)
	}
}
