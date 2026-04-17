package account

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/internal/consts"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestAllocateFieldAmongSubs_NegativeParentSpotNoRemainderToMax(t *testing.T) {
	m := &ctypes.Market{BaseAssetPrecision: 1, QuoteAssetPrecision: 2}
	shares := []subShare{
		{id: "a", s: decimal.RequireFromString("0.3333333333")},
		{id: "b", s: decimal.RequireFromString("0.3333333333")},
	}
	parent := decimal.NewFromInt(-1)
	got := allocateFieldAmongSubs(parent, shares, false, scaleFieldBaseQty, m)
	// 非合约 base 按 DefaultAssetPrecision，与 m.BaseAssetPrecision 无关
	want := floorDecimalPlaces(decimal.RequireFromString("0.3333333333"), int32(consts.DefaultAssetPrecision)).Neg()
	if !got["a"].Equal(want) || !got["b"].Equal(want) {
		t.Fatalf("want both subs floor abs then neg to %s, got %#v", want, got)
	}
}

func TestAllocateFieldAmongSubs_SpotUsesDefaultAssetPrecision(t *testing.T) {
	m := &ctypes.Market{BaseAssetPrecision: 1, QuoteAssetPrecision: 2}
	shares := []subShare{
		{id: "a", s: decimal.RequireFromString("0.3333333333")},
		{id: "b", s: decimal.RequireFromString("0.3333333333")},
	}
	got := allocateFieldAmongSubs(decimal.NewFromInt(1), shares, false, scaleFieldBaseQty, m)
	want := floorDecimalPlaces(decimal.RequireFromString("0.3333333333"), int32(consts.DefaultAssetPrecision))
	if !got["a"].Equal(want) || !got["b"].Equal(want) {
		t.Fatalf("want both subs floor at DefaultAssetPrecision without topup, got %#v", got)
	}
}

func TestMultiBotSubScaledBaseBelowMinStep(t *testing.T) {
	lot := decimal.NewFromInt(1)
	futMkt := &ctypes.Market{BaseAssetPrecision: 8, Rules: ctypes.MarketRules{LotSize: lot}}
	spotMkt := &ctypes.Market{BaseAssetPrecision: 4, QuoteAssetPrecision: 2}

	tests := []struct {
		name string
		m    *ctypes.Market
		o    ctypes.Order
		want bool
	}{
		{
			name: "skip_zero_orig",
			m:    futMkt,
			o:    ctypes.Order{OriginalQty: decimal.Zero, ExecutedQty: decimal.Zero},
			want: true,
		},
		{
			name: "skip_negative_orig",
			m:    futMkt,
			o:    ctypes.Order{OriginalQty: decimal.NewFromInt(-1), ExecutedQty: decimal.Zero},
			want: true,
		},
		{
			name: "not_skip_positive_below_future_lot",
			m:    futMkt,
			o:    ctypes.Order{OriginalQty: decimal.RequireFromString("0.2"), ExecutedQty: decimal.Zero},
			want: false,
		},
		{
			name: "not_skip_positive_below_spot_step",
			m:    spotMkt,
			o: ctypes.Order{
				OriginalQty: decimal.RequireFromString("0.00001"),
				ExecutedQty: decimal.RequireFromString("0.00002"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if g := multiBotSubScaledBaseBelowMinStep(tt.o, tt.m); g != tt.want {
				t.Fatalf("got %v want %v", g, tt.want)
			}
		})
	}
}

func TestAllocateFieldAmongSubs_NegativeParentFutureLotFloor(t *testing.T) {
	lot := decimal.NewFromInt(1)
	m := &ctypes.Market{BaseAssetPrecision: 8, Rules: ctypes.MarketRules{LotSize: lot}}
	shares := []subShare{
		{id: "a", s: decimal.RequireFromString("0.25")},
		{id: "b", s: decimal.RequireFromString("0.25")},
		{id: "c", s: decimal.RequireFromString("0.25")},
		{id: "d", s: decimal.RequireFromString("0.25")},
	}
	parent := decimal.NewFromInt(-1)
	got := allocateFieldAmongSubs(parent, shares, true, scaleFieldBaseQty, m)
	for _, id := range []string{"a", "b", "c", "d"} {
		if !got[id].IsZero() {
			t.Fatalf("sub %s: want zero (|-0.25| floors to 0 lot), got %s", id, got[id])
		}
	}
}

func TestAllocateFieldAmongSubs_FutureNoRemainderToMax(t *testing.T) {
	lot := decimal.NewFromInt(1)
	m := &ctypes.Market{BaseAssetPrecision: 8, Rules: ctypes.MarketRules{LotSize: lot}}
	shares := []subShare{
		{id: "a", s: decimal.RequireFromString("0.5")},
		{id: "b", s: decimal.RequireFromString("0.5")},
	}

	got := allocateFieldAmongSubs(decimal.NewFromInt(1), shares, true, scaleFieldBaseQty, m)
	if !got["a"].Equal(decimal.Zero) || !got["b"].Equal(decimal.Zero) {
		t.Fatalf("want both subs floor to zero, got %#v", got)
	}
}

func TestBuildScaledOrdersForMultiBotFanout_AllSubsFloorWithoutTopup(t *testing.T) {
	e := &Entity{}
	parent := &ctypes.Order{
		OrderID:          "ord-1",
		Symbol:           ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot),
		OriginalQty:      decimal.NewFromInt(1),
		ExecutedQty:      decimal.NewFromInt(1),
		OriginalQuoteQty: decimal.NewFromInt(1),
		ExecutedQuoteQty: decimal.NewFromInt(1),
		Fee:              loPtrDecimal(decimal.NewFromInt(1)),
		RealizedPnl:      loPtrDecimal(decimal.NewFromInt(1)),
	}
	disp := buildSubRawDispatchesFromUnitShares(*parent, map[string]decimal.Decimal{
		"subA": decimal.RequireFromString("0.3333333333"),
		"subB": decimal.RequireFromString("0.3333333333"),
		"subC": decimal.RequireFromString("0.3333333333"),
	})

	scaled, err := e.buildScaledOrdersForMultiBotFanout(t.Context(), ctypes.ExchangeBinance, parent, disp)
	if err != nil {
		t.Fatal(err)
	}
	if len(scaled) != 3 {
		t.Fatalf("expected 3 scaled orders, got %d", len(scaled))
	}
	want := floorDecimalPlaces(decimal.RequireFromString("0.3333333333"), int32(consts.DefaultAssetPrecision))
	for sid, ord := range scaled {
		if !ord.OriginalQty.Equal(want) || !ord.ExecutedQty.Equal(want) {
			t.Fatalf("sub %s qty mismatch: orig=%s exec=%s want=%s", sid, ord.OriginalQty, ord.ExecutedQty, want)
		}
		if !ord.OriginalQuoteQty.Equal(want) || !ord.ExecutedQuoteQty.Equal(want) {
			t.Fatalf("sub %s quote mismatch: orig=%s exec=%s want=%s", sid, ord.OriginalQuoteQty, ord.ExecutedQuoteQty, want)
		}
		if ord.Fee == nil || !ord.Fee.Equal(want) {
			t.Fatalf("sub %s fee mismatch: got=%v want=%s", sid, ord.Fee, want)
		}
		if ord.RealizedPnl == nil || !ord.RealizedPnl.Equal(want) {
			t.Fatalf("sub %s pnl mismatch: got=%v want=%s", sid, ord.RealizedPnl, want)
		}
	}
}

func loPtrDecimal(v decimal.Decimal) *decimal.Decimal {
	return &v
}
