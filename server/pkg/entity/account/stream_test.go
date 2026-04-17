package account

import (
	"testing"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestClampVirtualSubFutureReduceDustToZero(t *testing.T) {
	e := &Entity{}
	acct := &ctypes.Account{AccountType: ctypes.AccountTypeVirtualSub}
	delta := &ctypes.Position{
		Symbol: ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture),
		Amount: decimal.RequireFromString("-0.01"),
	}

	nextQty := decimal.RequireFromString("0.0000000000000000001")
	got := e.clampVirtualSubFutureReduceDustToZero(t.Context(), acct, ctypes.ExchangeBinance, delta, nextQty)
	if !got.IsZero() {
		t.Fatalf("want dust qty clamped to zero, got %s", got)
	}
}

func TestClampVirtualSubFutureReduceDustToZero_OpenKeepsQty(t *testing.T) {
	e := &Entity{}
	acct := &ctypes.Account{AccountType: ctypes.AccountTypeVirtualSub}
	delta := &ctypes.Position{
		Symbol: ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture),
		Amount: decimal.RequireFromString("0.01"),
	}

	nextQty := decimal.RequireFromString("0.0000000000000000001")
	got := e.clampVirtualSubFutureReduceDustToZero(t.Context(), acct, ctypes.ExchangeBinance, delta, nextQty)
	if !got.Equal(nextQty) {
		t.Fatalf("want open increment keep qty %s, got %s", nextQty, got)
	}
}
