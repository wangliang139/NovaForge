package account

import (
	"testing"

	"github.com/shopspring/decimal"
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

func TestAllocateFieldAmongSubs_spotBaseRemainderToMax(t *testing.T) {
	m := &ctypes.Market{BaseAssetPrecision: 0, QuoteAssetPrecision: 2}
	shares := []subShare{{id: "a", s: decimal.RequireFromString("0.3")}, {id: "b", s: decimal.RequireFromString("0.3")}}
	maxSub := "b"
	// sumShares=0.6, sumTicks=100*0.6=60; a floor(30)=30, b gets 60-30=30
	got := allocateFieldAmongSubs(decimal.NewFromInt(100), shares, maxSub, false, scaleFieldBaseQty, m)
	if !got["a"].Equal(decimal.NewFromInt(30)) || !got["b"].Equal(decimal.NewFromInt(30)) {
		t.Fatalf("got %#v", got)
	}
}

func TestAllocateFieldAmongSubs_futureLotRemainderToMax(t *testing.T) {
	lot := decimal.NewFromInt(1)
	m := &ctypes.Market{BaseAssetPrecision: 8, Rules: ctypes.MarketRules{LotSize: lot}}
	shares := []subShare{{id: "a", s: decimal.RequireFromString("0.31")}, {id: "b", s: decimal.RequireFromString("0.31")}}
	maxSub := "b"
	// t_a=3.1 floor lot 3, sumTicks=10*0.62=6.2, b=6.2-3=3.2
	got := allocateFieldAmongSubs(decimal.NewFromInt(10), shares, maxSub, true, scaleFieldBaseQty, m)
	if !got["a"].Equal(decimal.NewFromInt(3)) || !got["b"].Equal(decimal.RequireFromString("3.2")) {
		t.Fatalf("got %#v", got)
	}
}

func TestAllocateFieldAmongSubs_moneyDefaultPrecision(t *testing.T) {
	m := (*ctypes.Market)(nil)
	shares := []subShare{{id: "a", s: decimal.RequireFromString("0.25")}, {id: "b", s: decimal.RequireFromString("0.25")}}
	maxSub := "b"
	got := allocateFieldAmongSubs(decimal.NewFromInt(4), shares, maxSub, false, scaleFieldMoney, m)
	if !got["a"].Equal(decimal.NewFromInt(1)) || !got["b"].Equal(decimal.NewFromInt(1)) {
		t.Fatalf("fee split got %#v", got)
	}
}

func TestOrderMatchedWeightsToSubFanoutShares_reservesParentWhenSubsSumBelowOriginal(t *testing.T) {
	weights := map[string]decimal.Decimal{
		"subA": decimal.NewFromInt(1),
		"subB": decimal.NewFromInt(2),
	}
	// T=3, P=10 → 子合计份额 0.3，与「未分配由父吸收」一致
	got, err := orderMatchedWeightsToSubFanoutShares(weights, decimal.NewFromInt(10))
	if err != nil {
		t.Fatal(err)
	}
	if !got["subA"].Equal(decimal.RequireFromString("0.1")) || !got["subB"].Equal(decimal.RequireFromString("0.2")) {
		t.Fatalf("got %#v", got)
	}
	var sum decimal.Decimal
	for _, v := range got {
		sum = sum.Add(v)
	}
	if !sum.Equal(decimal.RequireFromString("0.3")) {
		t.Fatalf("sum child shares %s want 0.3", sum)
	}
}

func TestOrderMatchedWeightsToSubFanoutShares_sameAsNormalizeWhenSubsCoverParent(t *testing.T) {
	weights := map[string]decimal.Decimal{
		"subA": decimal.NewFromInt(1),
		"subB": decimal.NewFromInt(2),
	}
	got, err := orderMatchedWeightsToSubFanoutShares(weights, decimal.NewFromInt(3))
	if err != nil {
		t.Fatal(err)
	}
	wantA := decimal.NewFromInt(1).Div(decimal.NewFromInt(3))
	wantB := decimal.NewFromInt(2).Div(decimal.NewFromInt(3))
	if !got["subA"].Equal(wantA) || !got["subB"].Equal(wantB) {
		t.Fatalf("got subA=%s subB=%s want %s %s", got["subA"], got["subB"], wantA, wantB)
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
