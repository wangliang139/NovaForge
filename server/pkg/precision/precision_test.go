package precision

import (
	"testing"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
)

func TestFinalizeForStorage_Round18(t *testing.T) {
	d := decimal.RequireFromString("1.1234567890123456789")
	got := FinalizeForStorage(d)
	want := decimal.RequireFromString("1.123456789012345679")
	if !got.Equal(want) {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestDecimalToPgNumeric_MatchesFinalizeAndUtils(t *testing.T) {
	d := decimal.RequireFromString("1.1234567890123456789")
	got := utils.Decimal.PgNumericToDecimal(DecimalToPgNumeric(d))
	want := utils.Decimal.PgNumericToDecimal(utils.Decimal.DecimalToPgNumeric(FinalizeForStorage(d)))
	if !got.Equal(want) {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestFeeFromNotional_NoEarlyTruncate(t *testing.T) {
	n := decimal.RequireFromString("100.1")
	r := decimal.RequireFromString("0.001")
	f := FeeFromNotional(n, r)
	// 高精度中间值，不在此处截断
	if !f.Equal(decimal.RequireFromString("0.1001")) {
		t.Fatalf("unexpected fee %s", f)
	}
}

func TestFinalizeOrderSnapshotForDB(t *testing.T) {
	f := decimal.RequireFromString("0.1001000000000000001")
	o := &ctypes.Order{
		Price:       decimal.RequireFromString("1.0000000000000000004"),
		OriginalQty: decimal.RequireFromString("2.0000000000000000009"),
		ExecutedQty: decimal.RequireFromString("1.5"),
		Fee:         &f,
	}
	FinalizeOrderSnapshotForDB(o)
	if !o.Price.Equal(decimal.RequireFromString("1.000000000000000000")) {
		t.Fatalf("price: %s", o.Price)
	}
	if !o.Fee.Equal(decimal.RequireFromString("0.100100000000000000")) {
		t.Fatalf("fee: %s", o.Fee)
	}
}
