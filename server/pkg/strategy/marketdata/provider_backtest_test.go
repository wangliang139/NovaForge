package marketdata

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestBacktestMarketProviderAcceptsHistoricalKline(t *testing.T) {
	ctx := context.Background()
	p := NewBacktestMarketProvider(ctypes.ExchangeBinance, "USDT")

	ex := ctypes.ExchangeBinance
	sym := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)
	ts := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	openMs := ts.UnixMilli()

	sig := &stypes.KlineSignal{
		BaseSignal: stypes.BaseSignal{
			Exchange: &ex,
			Symbol:   &sym,
			Ts:       ts,
		},
		Interval: ctypes.Interval1m,
		Open:     decimal.RequireFromString("100"),
		High:     decimal.RequireFromString("110"),
		Low:      decimal.RequireFromString("90"),
		Close:    decimal.RequireFromString("105"),
		Volume:   decimal.RequireFromString("1"),
		OpenTs:   openMs,
		IsClosed: true,
	}

	if err := p.OnEvent(ctx, sig); err != nil {
		t.Fatalf("OnEvent: %v", err)
	}

	px, err := p.GetLastPrice(ctx, ex, sym)
	if err != nil {
		t.Fatalf("GetLastPrice: %v", err)
	}
	if !px.Equal(decimal.RequireFromString("105")) {
		t.Fatalf("expected close 105, got %s", px)
	}
}
