package simulate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type fakePublicConnector struct {
	market   *ctypes.Market
	price    *ctypes.Price
	ticker   *ctypes.Ticker
	depth    *ctypes.OrderBook
	supports map[ctypes.StreamType]bool
}

func (f *fakePublicConnector) Subscribe(ctx context.Context, selector ctypes.StreamSelector) (*mdtypes.StreamHandle, error) {
	out := make(chan *ctypes.Message)
	errCh := make(chan error)
	stopC := make(chan struct{})
	doneC := make(chan struct{})
	close(doneC)
	return mdtypes.BuildHandle(ctx, out, errCh, stopC, doneC), nil
}
func (f *fakePublicConnector) Supports(selector ctypes.StreamSelector) bool {
	if f.supports == nil {
		return false
	}
	return f.supports[selector.Stream]
}
func (f *fakePublicConnector) Exchange() ctypes.Exchange { return ctypes.ExchangeBinance }
func (f *fakePublicConnector) GetMarkets(ctx context.Context, tps []ctypes.MarketType) ([]*ctypes.Market, error) {
	if f.market == nil {
		return nil, nil
	}
	return []*ctypes.Market{f.market}, nil
}
func (f *fakePublicConnector) GetMarket(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Market, error) {
	return f.market, nil
}
func (f *fakePublicConnector) Prices(ctx context.Context, marketType *ctypes.MarketType) ([]*ctypes.Price, error) {
	if f.price == nil {
		return nil, nil
	}
	return []*ctypes.Price{f.price}, nil
}
func (f *fakePublicConnector) Price(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Price, error) {
	return f.price, nil
}
func (f *fakePublicConnector) BookPrices(ctx context.Context, marketType *ctypes.MarketType) ([]*ctypes.BookPrice, error) {
	return nil, nil
}
func (f *fakePublicConnector) BookPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.BookPrice, error) {
	return nil, nil
}
func (f *fakePublicConnector) MarkPrices(ctx context.Context) ([]*ctypes.MarkPrice, error) {
	return nil, nil
}
func (f *fakePublicConnector) MarkPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.MarkPrice, error) {
	if !symbol.IsValid() {
		return nil, nil
	}
	return &ctypes.MarkPrice{
		Exchange:  ctypes.ExchangeBinance,
		Symbol:    symbol,
		MarkPrice: decimal.RequireFromString("25000"),
		Ts:        time.Now(),
	}, nil
}
func (f *fakePublicConnector) IndexPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.IndexPrice, error) {
	if !symbol.IsValid() {
		return nil, nil
	}
	return &ctypes.IndexPrice{
		Exchange:   ctypes.ExchangeBinance,
		Symbol:     symbol,
		IndexPrice: decimal.RequireFromString("24990"),
		Ts:         time.Now(),
	}, nil
}
func (f *fakePublicConnector) IndexComponent(ctx context.Context, symbol ctypes.Symbol) (*ctypes.IndexComponent, error) {
	return nil, errors.New("not implemented")
}
func (f *fakePublicConnector) Ticker(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Ticker, error) {
	return f.ticker, nil
}
func (f *fakePublicConnector) Trades(ctx context.Context, symbol ctypes.Symbol, limit int) ([]*ctypes.Trade, error) {
	return nil, nil
}
func (f *fakePublicConnector) Depth(ctx context.Context, symbol ctypes.Symbol, limit int) (*ctypes.OrderBook, error) {
	return f.depth, nil
}
func (f *fakePublicConnector) Klines(ctx context.Context, symbol ctypes.Symbol, interval ctypes.Interval, limit int) ([]*ctypes.Kline, error) {
	return nil, nil
}
func (f *fakePublicConnector) HisKlines(ctx context.Context, symbol ctypes.Symbol, interval ctypes.Interval, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.Kline, error) {
	return nil, nil
}
func (f *fakePublicConnector) FundingRate(ctx context.Context, symbol ctypes.Symbol) (*ctypes.FundingRate, error) {
	if !symbol.IsValid() {
		return nil, nil
	}
	return &ctypes.FundingRate{
		Exchange:    ctypes.ExchangeBinance,
		Symbol:      symbol,
		FundingRate: decimal.RequireFromString("0.0001"),
		Ts:          time.Now(),
	}, nil
}
func (f *fakePublicConnector) HisFundingRates(ctx context.Context, symbol ctypes.Symbol, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.FundingRate, error) {
	return nil, nil
}
func (f *fakePublicConnector) OpenInterest(ctx context.Context, symbol ctypes.Symbol) (decimal.Decimal, error) {
	return decimal.NewFromInt(12345), nil
}
func (f *fakePublicConnector) IsPrivate() bool { return false }
func (f *fakePublicConnector) Account(ctx context.Context) (*ctypes.AccountBo, error) {
	return nil, nil
}
func (f *fakePublicConnector) Balance(ctx context.Context) (*ctypes.Balance, error) {
	return nil, nil
}
func (f *fakePublicConnector) Positions(ctx context.Context, mt *ctypes.MarketType) ([]*ctypes.Position, error) {
	return nil, nil
}
func (f *fakePublicConnector) SymbolConfig(ctx context.Context, symbol ctypes.Symbol) (*ctypes.SymbolConfig, error) {
	return nil, nil
}
func (f *fakePublicConnector) GetOrders(ctx context.Context, symbol *ctypes.Symbol) ([]*ctypes.Order, error) {
	return nil, nil
}
func (f *fakePublicConnector) GetOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error) {
	return nil, nil
}
func (f *fakePublicConnector) CalcOrderFee(ctx context.Context, order ctypes.Order) (*decimal.Decimal, *string, error) {
	return nil, nil, nil
}
func (f *fakePublicConnector) PlaceOrder(ctx context.Context, input mdtypes.PlaceOrderInput) (*mdtypes.PlaceOrderResult, error) {
	return nil, nil
}
func (f *fakePublicConnector) CancelOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) error {
	return nil
}
func (f *fakePublicConnector) SetLeverage(ctx context.Context, symbol ctypes.Symbol, leverage int) (int, error) {
	return leverage, nil
}
func (f *fakePublicConnector) GetLeverageBracket(ctx context.Context, symbol ctypes.Symbol, markPrice decimal.Decimal) (*ctypes.LeverageBracket, error) {
	return nil, nil
}

func newTestConnector(accountID string, market *ctypes.Market, depth *ctypes.OrderBook) *Connector {
	ex := NewSimExchange()
	pub := &fakePublicConnector{
		market: market,
		depth:  depth,
		supports: map[ctypes.StreamType]bool{
			ctypes.StreamTypeTicker:    true,
			ctypes.StreamTypeDepth:     true,
			ctypes.StreamTypeMarkPrice: true,
		},
	}
	c := &Connector{
		exchange:    ctypes.ExchangeBinance,
		accountID:   accountID,
		accountSubs: make(map[chan *ctypes.Message]struct{}),
		state: &exchangeState{
			ex:            ex,
			adapter:       NewConnectorAdapter(ex),
			ticker:        NewTickerStore(),
			publicConn:    pub,
			depths:        make(map[Symbol]*MarketDepth),
			leverages:     make(map[leverageKey]int),
			bootstraps:    make(map[string]bool),
		},
	}
	c.state.registerConn(c)
	return c
}

func TestConnector_PlaceGetCancelOrder(t *testing.T) {
	symbol, err := ctypes.ParseSymbol("BTC/USDT:SPOT")
	if err != nil {
		t.Fatalf("parse symbol: %v", err)
	}
	market := &ctypes.Market{Exchange: ctypes.ExchangeBinance, Symbol: symbol}
	depth := &ctypes.OrderBook{
		Symbol: symbol,
		Bids:   []ctypes.OrderBookLevel{{Price: decimal.NewFromInt(99), Size: decimal.NewFromInt(1)}},
		Asks:   []ctypes.OrderBookLevel{{Price: decimal.NewFromInt(101), Size: decimal.NewFromInt(1)}},
		SeqId:  1,
		Ts:     time.Now(),
	}
	c := newTestConnector("u1", market, depth)
	_ = c.state.ex.InitBalances("u1", seedUSDT(ctypes.WalletTypeSpot, decimal.NewFromInt(10000)))
	if _, err := c.GetMarket(context.Background(), symbol); err != nil {
		t.Fatalf("seed market failed: %v", err)
	}
	if _, err := c.Depth(context.Background(), symbol, 20); err != nil {
		t.Fatalf("seed depth failed: %v", err)
	}

	price := decimal.NewFromInt(100)
	qty := decimal.RequireFromString("0.1")
	clientOrderID := ctypes.OrderId("cid-1")
	placeRes, err := c.PlaceOrder(context.Background(), mdtypes.PlaceOrderInput{
		Symbol:        symbol,
		IsBuy:         true,
		OrderType:     ctypes.OrderTypeLimit,
		Price:         lo.ToPtr(price),
		Quantity:      lo.ToPtr(qty),
		ClientOrderID: &clientOrderID,
	})
	if err != nil {
		t.Fatalf("place order failed: %v", err)
	}

	orders, err := c.GetOrders(context.Background(), &symbol)
	if err != nil {
		t.Fatalf("get orders failed: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("expected 1 open order, got %d", len(orders))
	}

	got, err := c.GetOrder(context.Background(), symbol, placeRes.OrderID.String())
	if err != nil {
		t.Fatalf("get order by id failed: %v", err)
	}
	if got == nil || got.ClientOrderID != clientOrderID {
		t.Fatalf("unexpected order lookup result: %+v", got)
	}

	if err := c.CancelOrder(context.Background(), symbol, placeRes.OrderID.String()); err != nil {
		t.Fatalf("cancel order failed: %v", err)
	}
	ordersAfter, err := c.GetOrders(context.Background(), &symbol)
	if err != nil {
		t.Fatalf("get orders after cancel failed: %v", err)
	}
	if len(ordersAfter) != 0 {
		t.Fatalf("expected 0 open orders after cancel, got %d", len(ordersAfter))
	}
}

func TestConnector_PlaceOrderAutoInitsDepth(t *testing.T) {
	symbol, err := ctypes.ParseSymbol("BTC/USDT:FUTURE")
	if err != nil {
		t.Fatalf("parse symbol: %v", err)
	}
	market := &ctypes.Market{
		Exchange: ctypes.ExchangeBinance,
		Symbol:   symbol,
		Status:   ctypes.MarketStatusTrading,
		Rules: ctypes.MarketRules{
			TickSize:    decimal.RequireFromString("0.1"),
			LotSize:     decimal.RequireFromString("0.001"),
			MinQuantity: decimal.RequireFromString("0.001"),
		},
	}
	depth := &ctypes.OrderBook{
		Symbol: symbol,
		SeqId:  1,
		Ts:     time.Now(),
		Bids:   []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("24999.9"), Size: decimal.NewFromInt(5)}},
		Asks:   []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("25000.0"), Size: decimal.NewFromInt(5)}},
	}
	c := newTestConnector("u1-auto-depth", market, depth)
	_ = c.state.ex.InitBalances("u1-auto-depth", seedUSDT(ctypes.WalletTypeFuture, decimal.NewFromInt(100000)))
	if _, err := c.GetMarket(context.Background(), symbol); err != nil {
		t.Fatalf("seed market failed: %v", err)
	}

	// Do NOT call c.Depth here; PlaceOrder should initialize depth by itself.
	_, err = c.PlaceOrder(context.Background(), mdtypes.PlaceOrderInput{
		Symbol:    symbol,
		IsBuy:     true,
		OrderType: ctypes.OrderTypeLimit,
		Price:     lo.ToPtr(decimal.RequireFromString("25000.0")),
		Quantity:  lo.ToPtr(decimal.RequireFromString("0.01")),
	})
	if err != nil {
		t.Fatalf("place order should auto init depth, got: %v", err)
	}
}

func TestConnector_PlaceOrderWithSeededBalance(t *testing.T) {
	symbol, err := ctypes.ParseSymbol("ETH/USDT:FUTURE")
	if err != nil {
		t.Fatalf("parse symbol: %v", err)
	}
	market := &ctypes.Market{
		Exchange: ctypes.ExchangeBinance,
		Symbol:   symbol,
		Status:   ctypes.MarketStatusTrading,
		Rules: ctypes.MarketRules{
			TickSize:    decimal.RequireFromString("0.1"),
			LotSize:     decimal.RequireFromString("0.001"),
			MinQuantity: decimal.RequireFromString("0.001"),
		},
	}
	depth := &ctypes.OrderBook{
		Symbol: symbol,
		SeqId:  1,
		Ts:     time.Now(),
		Bids:   []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("2999.9"), Size: decimal.NewFromInt(10)}},
		Asks:   []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("3000.0"), Size: decimal.NewFromInt(10)}},
	}
	c := newTestConnector("u-balance-bootstrap", market, depth)
	if _, err := c.GetMarket(context.Background(), symbol); err != nil {
		t.Fatalf("seed market failed: %v", err)
	}
	if _, err := c.Depth(context.Background(), symbol, 20); err != nil {
		t.Fatalf("seed depth failed: %v", err)
	}

	_ = c.SeedAccountBalances(seedUSDT(ctypes.WalletTypeFuture, decimal.NewFromInt(100000)))
	_, err = c.PlaceOrder(context.Background(), mdtypes.PlaceOrderInput{
		Symbol:    symbol,
		IsBuy:     true,
		OrderType: ctypes.OrderTypeLimit,
		Price:     lo.ToPtr(decimal.RequireFromString("3000.0")),
		Quantity:  lo.ToPtr(decimal.RequireFromString("0.01")),
	})
	if err != nil {
		t.Fatalf("place order should succeed with seeded balance, got: %v", err)
	}
}

func TestConnector_PlaceOrderWithoutSeedBalanceFails(t *testing.T) {
	symbol, err := ctypes.ParseSymbol("ETH/USDT:FUTURE")
	if err != nil {
		t.Fatalf("parse symbol: %v", err)
	}
	market := &ctypes.Market{
		Exchange: ctypes.ExchangeBinance,
		Symbol:   symbol,
		Status:   ctypes.MarketStatusTrading,
		Rules: ctypes.MarketRules{
			TickSize:    decimal.RequireFromString("0.1"),
			LotSize:     decimal.RequireFromString("0.001"),
			MinQuantity: decimal.RequireFromString("0.001"),
		},
	}
	depth := &ctypes.OrderBook{
		Symbol: symbol,
		SeqId:  1,
		Ts:     time.Now(),
		Bids:   []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("2999.9"), Size: decimal.NewFromInt(10)}},
		Asks:   []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("3000.0"), Size: decimal.NewFromInt(10)}},
	}
	c := newTestConnector("u-no-seed", market, depth)
	if _, err := c.GetMarket(context.Background(), symbol); err != nil {
		t.Fatalf("seed market failed: %v", err)
	}
	if _, err := c.Depth(context.Background(), symbol, 20); err != nil {
		t.Fatalf("seed depth failed: %v", err)
	}
	_, err = c.PlaceOrder(context.Background(), mdtypes.PlaceOrderInput{
		Symbol:    symbol,
		IsBuy:     true,
		OrderType: ctypes.OrderTypeLimit,
		Price:     lo.ToPtr(decimal.RequireFromString("3000.0")),
		Quantity:  lo.ToPtr(decimal.RequireFromString("0.01")),
	})
	if err == nil || !strings.Contains(err.Error(), "insufficient balance") {
		t.Fatalf("expected insufficient balance error, got: %v", err)
	}
}

func TestConnector_DepthAndTickerIngestion(t *testing.T) {
	symbol, err := ctypes.ParseSymbol("ETH/USDT:SPOT")
	if err != nil {
		t.Fatalf("parse symbol: %v", err)
	}
	market := &ctypes.Market{
		Exchange: ctypes.ExchangeBinance,
		Symbol:   symbol,
		Rules: ctypes.MarketRules{
			TickSize:    decimal.RequireFromString("0.1"),
			LotSize:     decimal.RequireFromString("0.01"),
			MinQuantity: decimal.RequireFromString("0.01"),
		},
	}
	depth := &ctypes.OrderBook{
		Symbol: symbol,
		Bids:   []ctypes.OrderBookLevel{{Price: decimal.NewFromInt(1999), Size: decimal.NewFromInt(1)}},
		Asks:   []ctypes.OrderBookLevel{{Price: decimal.NewFromInt(2001), Size: decimal.NewFromInt(1)}},
		SeqId:  1,
		Ts:     time.Now(),
	}
	c := newTestConnector("u2", market, depth)

	c.WarmSymbols(context.Background(), []ctypes.Symbol{symbol})
	if _, ok := c.state.depths[toSimSymbol(symbol)]; !ok {
		t.Fatalf("depth should be ingested into simulate state")
	}

	msgTs := time.Now()
	c.ingestMessage(&ctypes.Message{
		Ticker: &ctypes.Ticker{
			Exchange:  ctypes.ExchangeBinance,
			Symbol:    symbol,
			LastPrice: decimal.RequireFromString("2000.5"),
			Ts:        msgTs,
		},
	})
	tk, err := c.Ticker(context.Background(), symbol)
	if err != nil {
		t.Fatalf("ticker failed: %v", err)
	}
	if tk == nil || !tk.LastPrice.Equal(decimal.RequireFromString("2000.5")) {
		t.Fatalf("unexpected ticker: %+v", tk)
	}
	if !tk.Ts.Equal(msgTs) {
		t.Fatalf("unexpected ticker ts: %v", tk.Ts)
	}
}

func TestConnector_Supports(t *testing.T) {
	c := newTestConnector("u3", nil, nil)
	if !c.Supports(ctypes.StreamSelector{Stream: ctypes.StreamTypeTicker}) {
		t.Fatalf("ticker stream should be supported")
	}
	if !c.Supports(ctypes.StreamSelector{Stream: ctypes.StreamTypeDepth}) {
		t.Fatalf("depth stream should be supported")
	}
	if c.Supports(ctypes.StreamSelector{Stream: ctypes.StreamTypeTrade}) {
		t.Fatalf("trade stream should not be supported")
	}
	if !c.Supports(ctypes.StreamSelector{Stream: ctypes.StreamTypeAccountRaw}) {
		t.Fatalf("account raw stream should be supported")
	}
}

func TestToTypesOrderStatus(t *testing.T) {
	cases := []struct {
		in   OrderStatus
		want ctypes.OrderStatus
	}{
		{OrderStatusNew, ctypes.OrderStatusNew},
		{OrderStatusPartiallyFilled, ctypes.OrderStatusPartialDone},
		{OrderStatusFilled, ctypes.OrderStatusDone},
		{OrderStatusCanceled, ctypes.OrderStatusCanceled},
		{OrderStatusRejected, ctypes.OrderStatusRejected},
		{OrderStatus(999), ctypes.OrderStatusPending},
	}
	for _, tc := range cases {
		got := toTypesOrderStatus(tc.in)
		if got != tc.want {
			t.Fatalf("status map mismatch: in=%v got=%v want=%v", tc.in, got, tc.want)
		}
	}
}

func TestConnector_SymbolConfigMapsFeesAndPrecision(t *testing.T) {
	symbol, err := ctypes.ParseSymbol("SOL/USDT:SPOT")
	if err != nil {
		t.Fatalf("parse symbol: %v", err)
	}
	c := newTestConnector("u4", &ctypes.Market{
		Exchange: ctypes.ExchangeBinance,
		Symbol:   symbol,
		Rules: ctypes.MarketRules{
			TickSize: decimal.RequireFromString("0.001"),
			LotSize:  decimal.RequireFromString("0.0001"),
		},
	}, nil)

	cfg, err := c.SymbolConfig(context.Background(), symbol)
	if err != nil {
		t.Fatalf("symbol config failed: %v", err)
	}
	if cfg == nil {
		t.Fatalf("symbol config should not be nil")
	}
	if !cfg.MakerCommission.Equal(decimal.RequireFromString("0.001")) || !cfg.TakerCommission.Equal(decimal.RequireFromString("0.001")) {
		t.Fatalf("unexpected fees: maker=%s taker=%s", cfg.MakerCommission, cfg.TakerCommission)
	}
	if cfg.Market.PricePrecision != 3 {
		t.Fatalf("unexpected price precision: %d", cfg.Market.PricePrecision)
	}
	if cfg.Market.BaseAssetPrecision != 4 {
		t.Fatalf("unexpected base precision: %d", cfg.Market.BaseAssetPrecision)
	}
	if cfg.Market.QuoteAssetPrecision != 3 {
		t.Fatalf("unexpected quote precision: %d", cfg.Market.QuoteAssetPrecision)
	}
}

func TestConnector_PlaceOrderPreValidation(t *testing.T) {
	symbol, err := ctypes.ParseSymbol("BTC/USDT:SPOT")
	if err != nil {
		t.Fatalf("parse symbol: %v", err)
	}
	market := &ctypes.Market{
		Exchange: ctypes.ExchangeBinance,
		Symbol:   symbol,
		Status:   ctypes.MarketStatusTrading,
		Rules: ctypes.MarketRules{
			TickSize:    decimal.RequireFromString("0.1"),
			LotSize:     decimal.RequireFromString("0.01"),
			MinQuantity: decimal.RequireFromString("0.01"),
			MinNotional: decimal.RequireFromString("10"),
		},
	}
	depth := &ctypes.OrderBook{
		Symbol: symbol,
		Bids:   []ctypes.OrderBookLevel{{Price: decimal.NewFromInt(99), Size: decimal.NewFromInt(1)}},
		Asks:   []ctypes.OrderBookLevel{{Price: decimal.NewFromInt(101), Size: decimal.NewFromInt(1)}},
		Ts:     time.Now(),
	}
	c := newTestConnector("u5", market, depth)
	_ = c.state.ex.InitBalances("u5", seedUSDT(ctypes.WalletTypeSpot, decimal.NewFromInt(10000)))
	_, _ = c.GetMarket(context.Background(), symbol)
	_, _ = c.Depth(context.Background(), symbol, 20)

	_, err = c.PlaceOrder(context.Background(), mdtypes.PlaceOrderInput{
		Symbol:    symbol,
		IsBuy:     true,
		OrderType: ctypes.OrderTypeLimit,
		Price:     lo.ToPtr(decimal.RequireFromString("100.03")),
		Quantity:  lo.ToPtr(decimal.RequireFromString("0.1")),
	})
	if err == nil || !strings.Contains(err.Error(), "tick size") {
		t.Fatalf("expect tick size validation error, got: %v", err)
	}

	_, err = c.PlaceOrder(context.Background(), mdtypes.PlaceOrderInput{
		Symbol:    symbol,
		IsBuy:     true,
		OrderType: ctypes.OrderTypeLimit,
		Price:     lo.ToPtr(decimal.RequireFromString("100.0")),
		Quantity:  lo.ToPtr(decimal.RequireFromString("0.001")),
	})
	if err == nil || !strings.Contains(err.Error(), "quantity must be >=") {
		t.Fatalf("expect min quantity validation error, got: %v", err)
	}

	_, err = c.PlaceOrder(context.Background(), mdtypes.PlaceOrderInput{
		Symbol:    symbol,
		IsBuy:     true,
		OrderType: ctypes.OrderTypeLimit,
		Price:     lo.ToPtr(decimal.RequireFromString("100.0")),
		Quantity:  lo.ToPtr(decimal.RequireFromString("0.05")),
	})
	if err == nil || !strings.Contains(err.Error(), "notional must be >=") {
		t.Fatalf("expect min notional validation error, got: %v", err)
	}
}

func TestConnector_MarketOrderValidatesNotionalByReferencePrice(t *testing.T) {
	symbol, err := ctypes.ParseSymbol("ETH/USDT:SPOT")
	if err != nil {
		t.Fatalf("parse symbol: %v", err)
	}
	market := &ctypes.Market{
		Exchange: ctypes.ExchangeBinance,
		Symbol:   symbol,
		Status:   ctypes.MarketStatusTrading,
		Rules: ctypes.MarketRules{
			LotSize:     decimal.RequireFromString("0.01"),
			MinQuantity: decimal.RequireFromString("0.01"),
			MinNotional: decimal.RequireFromString("20"),
		},
	}
	depth := &ctypes.OrderBook{
		Symbol: symbol,
		Bids:   []ctypes.OrderBookLevel{{Price: decimal.NewFromInt(1999), Size: decimal.NewFromInt(1)}},
		Asks:   []ctypes.OrderBookLevel{{Price: decimal.NewFromInt(2001), Size: decimal.NewFromInt(1)}},
		Ts:     time.Now(),
	}
	c := newTestConnector("u6", market, depth)
	pub := c.state.publicConn.(*fakePublicConnector)
	pub.price = &ctypes.Price{Exchange: ctypes.ExchangeBinance, Symbol: symbol, Price: decimal.RequireFromString("1000")}
	_ = c.state.ex.InitBalances("u6", seedUSDT(ctypes.WalletTypeSpot, decimal.NewFromInt(10000)))
	_, _ = c.GetMarket(context.Background(), symbol)
	_, _ = c.Depth(context.Background(), symbol, 20)

	_, err = c.PlaceOrder(context.Background(), mdtypes.PlaceOrderInput{
		Symbol:    symbol,
		IsBuy:     true,
		OrderType: ctypes.OrderTypeMarket,
		Quantity:  lo.ToPtr(decimal.RequireFromString("0.01")),
	})
	if err == nil || !strings.Contains(err.Error(), "notional must be >=") {
		t.Fatalf("expect min notional error for market order, got: %v", err)
	}
}

func TestConnector_StrongConstraint_UnimplementedAPIs(t *testing.T) {
	symbol, err := ctypes.ParseSymbol("BTC/USDT:SPOT")
	if err != nil {
		t.Fatalf("parse symbol: %v", err)
	}
	c := newTestConnector("u7", nil, nil)

	_, err = c.MarkPrices(context.Background())
	if err != nil {
		t.Fatalf("mark prices should be available, got: %v", err)
	}
	_, err = c.MarkPrice(context.Background(), symbol)
	if err != nil {
		t.Fatalf("mark price should be available, got: %v", err)
	}
	_, err = c.IndexPrice(context.Background(), symbol)
	if err != nil {
		t.Fatalf("index price should be available, got: %v", err)
	}
	_, err = c.IndexComponent(context.Background(), symbol)
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("expect not implemented error for IndexComponent, got: %v", err)
	}
	_, err = c.FundingRate(context.Background(), symbol)
	if err != nil {
		t.Fatalf("funding rate should be available, got: %v", err)
	}
}

func TestConnector_EventTimeAndDepthConsistency(t *testing.T) {
	symbol, err := ctypes.ParseSymbol("ETH/USDT:SPOT")
	if err != nil {
		t.Fatalf("parse symbol: %v", err)
	}
	c := newTestConnector("u8", nil, nil)

	ts1 := time.Now().Add(-3 * time.Second)
	ts2 := time.Now().Add(-2 * time.Second)
	ts3 := time.Now()
	c.ingestMessage(&ctypes.Message{
		Ticker: &ctypes.Ticker{
			Exchange:  ctypes.ExchangeBinance,
			Symbol:    symbol,
			LastPrice: decimal.RequireFromString("1999.5"),
			Ts:        ts1,
		},
	})
	c.ingestMessage(&ctypes.Message{
		Ticker: &ctypes.Ticker{
			Exchange:  ctypes.ExchangeBinance,
			Symbol:    symbol,
			LastPrice: decimal.RequireFromString("2001.2"),
			Ts:        ts2,
		},
	})
	c.ingestMessage(&ctypes.Message{
		Ticker: &ctypes.Ticker{
			Exchange:  ctypes.ExchangeBinance,
			Symbol:    symbol,
			LastPrice: decimal.RequireFromString("2000.8"),
			Ts:        ts1, // older event should not roll back ts
		},
	})

	tk, err := c.Ticker(context.Background(), symbol)
	if err != nil {
		t.Fatalf("ticker failed: %v", err)
	}
	if tk == nil || !tk.LastPrice.Equal(decimal.RequireFromString("2001.2")) {
		t.Fatalf("unexpected ticker after events: %+v", tk)
	}
	if !tk.Ts.Equal(ts2) {
		t.Fatalf("ticker ts mismatch: got=%v want=%v", tk.Ts, ts2)
	}

	c.ingestMessage(&ctypes.Message{
		Depth: &ctypes.OrderBook{
			Symbol: symbol,
			SeqId:  1,
			Ts:     ts1,
			Bids:   []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("1999"), Size: decimal.NewFromInt(2)}},
			Asks:   []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("2002"), Size: decimal.NewFromInt(3)}},
		},
	})
	c.ingestMessage(&ctypes.Message{
		Depth: &ctypes.OrderBook{
			Symbol:    symbol,
			SeqId:     2,
			PrevSeqId: 1,
			Ts:        ts2,
			Bids:      []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("2000"), Size: decimal.NewFromInt(1)}},
			Asks:      []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("2001"), Size: decimal.NewFromInt(1)}},
		},
	})
	c.ingestMessage(&ctypes.Message{
		Depth: &ctypes.OrderBook{
			Symbol:    symbol,
			SeqId:     3,
			PrevSeqId: 2,
			Ts:        ts3,
			Bids:      []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("2000"), Size: decimal.NewFromInt(2)}},
			Asks:      []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("2001"), Size: decimal.NewFromInt(2)}},
		},
	})

	d := c.state.depths[toSimSymbol(symbol)]
	if d == nil {
		t.Fatalf("depth not ingested")
	}
	if d.LastSeqID() != 3 {
		t.Fatalf("depth seq mismatch: got=%d want=3", d.LastSeqID())
	}
	bid, _, ok := d.BestBid()
	if !ok || !bid.Equal(decimal.RequireFromString("2000")) {
		t.Fatalf("unexpected best bid after depth events: ok=%v bid=%s", ok, bid)
	}
}

func TestConnector_PerpReadAndPositionLeverageFlow(t *testing.T) {
	symbol, err := ctypes.ParseSymbol("BTC/USDT:FUTURE")
	if err != nil {
		t.Fatalf("parse symbol: %v", err)
	}
	market := &ctypes.Market{
		Exchange: ctypes.ExchangeBinance,
		Symbol:   symbol,
		Status:   ctypes.MarketStatusTrading,
		Rules: ctypes.MarketRules{
			TickSize:    decimal.RequireFromString("0.1"),
			LotSize:     decimal.RequireFromString("0.001"),
			MinQuantity: decimal.RequireFromString("0.001"),
		},
	}
	depth := &ctypes.OrderBook{
		Symbol: symbol,
		SeqId:  1,
		Bids:   []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("24999.9"), Size: decimal.NewFromInt(10)}},
		Asks:   []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("25000.0"), Size: decimal.NewFromInt(10)}},
		Ts:     time.Now(),
	}
	c := newTestConnector("u9", market, depth)
	_ = c.state.ex.InitBalances("u9", seedUSDT(ctypes.WalletTypeFuture, decimal.NewFromInt(100000)))
	_, _ = c.GetMarket(context.Background(), symbol)
	_, _ = c.Depth(context.Background(), symbol, 20)
	c.ingestMessage(&ctypes.Message{
		Ticker: &ctypes.Ticker{
			Exchange:  ctypes.ExchangeBinance,
			Symbol:    symbol,
			LastPrice: decimal.RequireFromString("25010.0"),
			Ts:        time.Now(),
		},
	})

	if lev, err := c.SetLeverage(context.Background(), symbol, 10); err != nil || lev != 10 {
		t.Fatalf("set leverage failed: lev=%d err=%v", lev, err)
	}
	_, err = c.PlaceOrder(context.Background(), mdtypes.PlaceOrderInput{
		Symbol:    symbol,
		IsBuy:     true,
		OrderType: ctypes.OrderTypeLimit,
		Price:     lo.ToPtr(decimal.RequireFromString("25000.0")),
		Quantity:  lo.ToPtr(decimal.RequireFromString("0.01")),
	})
	if err != nil {
		t.Fatalf("place perp order failed: %v", err)
	}

	positions, err := c.Positions(context.Background(), lo.ToPtr(ctypes.MarketTypeFuture))
	if err != nil {
		t.Fatalf("positions failed: %v", err)
	}
	if len(positions) == 0 {
		t.Fatalf("expected perp position after filled order")
	}
	if positions[0].Leverage != 10 {
		t.Fatalf("unexpected leverage on position: %d", positions[0].Leverage)
	}

	mp, err := c.MarkPrice(context.Background(), symbol)
	if err != nil || mp == nil || !mp.MarkPrice.GreaterThan(decimal.Zero) {
		t.Fatalf("mark price invalid: mp=%+v err=%v", mp, err)
	}
	ip, err := c.IndexPrice(context.Background(), symbol)
	if err != nil || ip == nil || !ip.IndexPrice.GreaterThan(decimal.Zero) {
		t.Fatalf("index price invalid: ip=%+v err=%v", ip, err)
	}
	fr, err := c.FundingRate(context.Background(), symbol)
	if err != nil || fr == nil {
		t.Fatalf("funding rate invalid: fr=%+v err=%v", fr, err)
	}
	oi, err := c.OpenInterest(context.Background(), symbol)
	if err != nil || !oi.GreaterThan(decimal.Zero) {
		t.Fatalf("open interest invalid: oi=%s err=%v", oi, err)
	}
	bracket, err := c.GetLeverageBracket(context.Background(), symbol, mp.MarkPrice)
	if err != nil || bracket == nil || len(bracket.Brackets) == 0 {
		t.Fatalf("leverage bracket invalid: bracket=%+v err=%v", bracket, err)
	}
}

func TestConnector_AccountRawEmitsOrderAndSnapshots(t *testing.T) {
	symbol, err := ctypes.ParseSymbol("BTC/USDT:FUTURE")
	if err != nil {
		t.Fatalf("parse symbol: %v", err)
	}
	market := &ctypes.Market{
		Exchange: ctypes.ExchangeBinance,
		Symbol:   symbol,
		Status:   ctypes.MarketStatusTrading,
		Rules: ctypes.MarketRules{
			TickSize:    decimal.RequireFromString("0.1"),
			LotSize:     decimal.RequireFromString("0.001"),
			MinQuantity: decimal.RequireFromString("0.001"),
		},
	}
	depth := &ctypes.OrderBook{
		Symbol: symbol,
		SeqId:  1,
		Bids:   []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("24999.9"), Size: decimal.NewFromInt(10)}},
		Asks:   []ctypes.OrderBookLevel{{Price: decimal.RequireFromString("25000.0"), Size: decimal.NewFromInt(10)}},
		Ts:     time.Now(),
	}
	c := newTestConnector("u-event", market, depth)
	_ = c.SeedAccountBalances(seedUSDT(ctypes.WalletTypeFuture, decimal.NewFromInt(100000)))
	_, _ = c.GetMarket(context.Background(), symbol)
	_, _ = c.Depth(context.Background(), symbol, 20)

	h, err := c.Subscribe(context.Background(), ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccountRaw,
		Account: lo.ToPtr("u-event"),
	})
	if err != nil {
		t.Fatalf("subscribe account raw failed: %v", err)
	}
	defer h.Stop()

	_, err = c.PlaceOrder(context.Background(), mdtypes.PlaceOrderInput{
		Symbol:    symbol,
		IsBuy:     true,
		OrderType: ctypes.OrderTypeLimit,
		Price:     lo.ToPtr(decimal.RequireFromString("25000.0")),
		Quantity:  lo.ToPtr(decimal.RequireFromString("0.01")),
	})
	if err != nil {
		t.Fatalf("place order failed: %v", err)
	}

	gotOrder := false
	gotBal := false
	gotPos := false
	deadline := time.After(2 * time.Second)
	for !(gotOrder && gotBal && gotPos) {
		select {
		case msg := <-h.C:
			if msg == nil {
				continue
			}
			if msg.Order != nil {
				gotOrder = true
			}
			if msg.BalanceUpdate != nil && len(msg.BalanceUpdate.Assets) > 0 {
				gotBal = true
			}
			if msg.PositionsUpdate != nil && len(msg.PositionsUpdate.Positions) > 0 {
				gotPos = true
			}
		case <-deadline:
			t.Fatalf("expect order+balance+position events, got order=%v balance=%v position=%v", gotOrder, gotBal, gotPos)
		}
	}
}
