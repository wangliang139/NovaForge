package account

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestSplitProportionalDelta_112_to_25_25_50(t *testing.T) {
	delta := decimal.NewFromInt(100)
	subs := []SubWeight{
		{SubAccountID: "subA", W: decimal.NewFromInt(1)},
		{SubAccountID: "subB", W: decimal.NewFromInt(1)},
	}
	wUnalloc := decimal.NewFromInt(2)

	toSub, parent, err := SplitProportionalDelta(delta, subs, wUnalloc)
	if err != nil {
		t.Fatal(err)
	}
	if !toSub["subA"].Equal(decimal.NewFromInt(25)) || !toSub["subB"].Equal(decimal.NewFromInt(25)) {
		t.Fatalf("subs: got %#v want 25 each", toSub)
	}
	if !parent.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("parent: got %s want 50", parent)
	}
	var sum decimal.Decimal
	for _, v := range toSub {
		sum = sum.Add(v)
	}
	if !sum.Add(parent).Equal(delta) {
		t.Fatalf("conservation: sum(children)+parent=%s want %s", sum.Add(parent), delta)
	}
}

func TestSplitProportionalDelta_mergeSameSubID(t *testing.T) {
	delta := decimal.NewFromInt(100)
	subs := []SubWeight{
		{SubAccountID: "subA", W: decimal.NewFromInt(1)},
		{SubAccountID: "subA", W: decimal.NewFromInt(1)},
		{SubAccountID: "subB", W: decimal.NewFromInt(2)},
	}
	wUnalloc := decimal.NewFromInt(4) // W = 2+2+4 = 8, A gets 100*2/8=25, B gets 100*2/8=25, parent 50
	toSub, parent, err := SplitProportionalDelta(delta, subs, wUnalloc)
	if err != nil {
		t.Fatal(err)
	}
	if !toSub["subA"].Equal(decimal.NewFromInt(25)) || !toSub["subB"].Equal(decimal.NewFromInt(25)) {
		t.Fatalf("got %#v", toSub)
	}
	if !parent.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("parent %s", parent)
	}
}

func TestSplitProportionalDelta_zeroDenominator(t *testing.T) {
	_, _, err := SplitProportionalDelta(decimal.NewFromInt(1), nil, decimal.Zero)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSplitProportionalDeltaRoundLastChild_conserves(t *testing.T) {
	delta := decimal.RequireFromString("100.001")
	subs := []SubWeight{
		{SubAccountID: "a", W: decimal.NewFromInt(1)},
		{SubAccountID: "b", W: decimal.NewFromInt(1)},
	}
	wUnalloc := decimal.NewFromInt(2)
	toSub, parent, err := SplitProportionalDeltaRoundLastChild(delta, subs, wUnalloc, 2)
	if err != nil {
		t.Fatal(err)
	}
	var sum decimal.Decimal
	for _, v := range toSub {
		sum = sum.Add(v)
	}
	if !sum.Add(parent).Equal(delta) {
		t.Fatalf("conservation failed: children+parent=%s delta=%s", sum.Add(parent), delta)
	}
}
