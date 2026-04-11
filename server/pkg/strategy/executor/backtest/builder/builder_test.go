package builder

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/marketdata"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
)

func TestCalculateUnrealizedPnL_Spot(t *testing.T) {
	// 创建一个简单的 mock marketProvider
	mockProvider := &mockMarketProvider{
		prices: map[string]decimal.Decimal{
			"USDT": decimal.NewFromInt(1),
		},
	}

	builder := &ResultBuilder{
		marketProvider: mockProvider,
		baseCurrency:   "USDT",
		baseExchange:   ctypes.ExchangeBinance,
	}

	exSymbol := ctypes.NewExSymbol(ctypes.ExchangeBinance, ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot))

	// 持仓 1 BTC，成本价 10000 USDT，当前价 12000 USDT
	posQty := decimal.NewFromInt(1)
	avgPx := decimal.NewFromInt(10000)
	lastPx := decimal.NewFromInt(12000)

	unrealized := builder.calculateUnrealizedPnL(context.Background(), exSymbol, posQty, avgPx, lastPx)

	// 未实现盈亏应为 1 * (12000 - 10000) = 2000 USDT
	expected := decimal.NewFromInt(2000)
	if !unrealized.Equal(expected) {
		t.Errorf("未实现盈亏应为 %s，got %s", expected, unrealized)
	}
}

func TestCalculateUnrealizedPnL_Future_Long(t *testing.T) {
	mockProvider := &mockMarketProvider{
		prices: map[string]decimal.Decimal{
			"USDT": decimal.NewFromInt(1),
		},
	}

	builder := &ResultBuilder{
		marketProvider: mockProvider,
		baseCurrency:   "USDT",
		baseExchange:   ctypes.ExchangeBinance,
	}

	exSymbol := ctypes.NewExSymbol(ctypes.ExchangeBinance, ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture))

	// 多仓 1 BTC，成本价 10000 USDT，当前价 12000 USDT
	posQty := decimal.NewFromInt(1)
	avgPx := decimal.NewFromInt(10000)
	lastPx := decimal.NewFromInt(12000)

	unrealized := builder.calculateUnrealizedPnL(context.Background(), exSymbol, posQty, avgPx, lastPx)

	// 未实现盈亏应为 1 * (12000 - 10000) = 2000 USDT
	expected := decimal.NewFromInt(2000)
	if !unrealized.Equal(expected) {
		t.Errorf("未实现盈亏应为 %s，got %s", expected, unrealized)
	}
}

func TestCalculateUnrealizedPnL_Future_Short(t *testing.T) {
	mockProvider := &mockMarketProvider{
		prices: map[string]decimal.Decimal{
			"USDT": decimal.NewFromInt(1),
		},
	}

	builder := &ResultBuilder{
		marketProvider: mockProvider,
		baseCurrency:   "USDT",
		baseExchange:   ctypes.ExchangeBinance,
	}

	exSymbol := ctypes.NewExSymbol(ctypes.ExchangeBinance, ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture))

	// 空仓 -1 BTC，成本价 10000 USDT，当前价 12000 USDT
	posQty := decimal.NewFromInt(-1)
	avgPx := decimal.NewFromInt(10000)
	lastPx := decimal.NewFromInt(12000)

	unrealized := builder.calculateUnrealizedPnL(context.Background(), exSymbol, posQty, avgPx, lastPx)

	// 未实现盈亏应为 -1 * (12000 - 10000) = -2000 USDT（亏损）
	expected := decimal.NewFromInt(-2000)
	if !unrealized.Equal(expected) {
		t.Errorf("未实现盈亏应为 %s，got %s", expected, unrealized)
	}
}

// mockMarketProvider 简单的 mock 实现
type mockMarketProvider struct {
	prices map[string]decimal.Decimal
}

var _ marketdata.MarketProvider = (*mockMarketProvider)(nil)

func (m *mockMarketProvider) GetMarkets(ctx context.Context, exchange ctypes.Exchange) ([]*ctypes.Market, error) {
	return nil, nil
}

func (m *mockMarketProvider) GetMarket(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.Market, error) {
	return nil, nil
}

func (m *mockMarketProvider) GetPriceInBaseCurrency(ctx context.Context, asset string, quote string) (decimal.Decimal, error) {
	return decimal.Zero, nil
}

func (m *mockMarketProvider) GetLastPrice(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, error) {
	key := symbol.Quote
	if price, ok := m.prices[key]; ok {
		return price, nil
	}
	return decimal.Zero, nil
}

func (m *mockMarketProvider) GetTicker(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.Ticker, error) {
	return nil, nil
}

func (m *mockMarketProvider) GetMarkPrice(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, error) {
	return m.GetLastPrice(ctx, exchange, symbol)
}

func (m *mockMarketProvider) GetIndexPrice(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, error) {
	return m.GetLastPrice(ctx, exchange, symbol)
}

func (m *mockMarketProvider) GetBookPrice(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, decimal.Decimal, error) {
	return decimal.Zero, decimal.Zero, nil
}

func (m *mockMarketProvider) GetFundingRate(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.FundingRate, error) {
	return nil, nil
}

func (m *mockMarketProvider) GetHisFundingRates(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.FundingRate, error) {
	return nil, nil
}

func (m *mockMarketProvider) GetOpenInterest(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, error) {
	return decimal.Zero, nil
}

func (m *mockMarketProvider) GetKlines(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, interval ctypes.Interval, limit int) ([]*ctypes.Kline, error) {
	return nil, nil
}

func (m *mockMarketProvider) GetHisKlines(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, interval ctypes.Interval, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.Kline, error) {
	return nil, nil
}

func (m *mockMarketProvider) GetDepth(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, limit int) (*ctypes.OrderBook, error) {
	return nil, nil
}

func (m *mockMarketProvider) GetTrades(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, limit int) ([]*ctypes.Trade, error) {
	return nil, nil
}

func (m *mockMarketProvider) OnEvent(ctx context.Context, event stypes.Signal) error {
	return nil
}
