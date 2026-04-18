package simulate

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/proxy"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type testEntityLayer struct {
	conn *Connector
}

func (e *testEntityLayer) PlaceOrder(ctx context.Context, req *ctypes.PlaceOrderRequest) (*ctypes.PlaceOrderResponse, error) {
	symbol, err := ctypes.ParseSymbol(req.Symbol)
	if err != nil {
		return nil, err
	}
	var (
		pricePtr *decimal.Decimal
		qtyPtr   *decimal.Decimal
	)
	if req.Price != nil {
		p := decimal.RequireFromString(*req.Price)
		pricePtr = &p
	}
	if req.Quantity != nil {
		q := decimal.RequireFromString(*req.Quantity)
		qtyPtr = &q
	}
	res, err := e.conn.PlaceOrder(ctx, mdtypes.PlaceOrderInput{
		Symbol:        symbol,
		Side:          req.Side,
		IsBuy:         req.IsBuy,
		OrderType:     req.OrderType,
		Price:         pricePtr,
		Quantity:      qtyPtr,
		ClientOrderID: lo.ToPtr(ctypes.OrderId(lo.FromPtr(req.ClientOrderID))),
	})
	if err != nil {
		return &ctypes.PlaceOrderResponse{Error: lo.ToPtr(err.Error())}, nil
	}
	return &ctypes.PlaceOrderResponse{
		OrderId:       res.OrderID.String(),
		ClientOrderId: res.ClientOrderID.String(),
		Status:        res.Status,
	}, nil
}

func (e *testEntityLayer) GetSymbolConfig(ctx context.Context, req *ctypes.GetSymbolConfigRequest) (*ctypes.GetSymbolConfigResponse, error) {
	symbol, err := ctypes.ParseSymbol(req.Symbol)
	if err != nil {
		return nil, err
	}
	cfg, err := e.conn.SymbolConfig(ctx, symbol)
	if err != nil {
		return nil, err
	}
	return &ctypes.GetSymbolConfigResponse{Config: cfg}, nil
}

func assignTestProxyStubs(entityLayer *testEntityLayer) {
	noopMarkets := func(context.Context, *ctypes.GetMarketsRequest) (*ctypes.GetMarketsResponse, error) {
		return &ctypes.GetMarketsResponse{}, nil
	}
	noopMarket := func(context.Context, *ctypes.GetMarketRequest) (*ctypes.GetMarketResponse, error) {
		return &ctypes.GetMarketResponse{}, nil
	}
	noopPrice := func(context.Context, *ctypes.GetPriceRequest) (*ctypes.GetPriceResponse, error) {
		return &ctypes.GetPriceResponse{}, nil
	}
	noopBookPrice := func(context.Context, *ctypes.GetBookPriceRequest) (*ctypes.GetBookPriceResponse, error) {
		return &ctypes.GetBookPriceResponse{}, nil
	}
	noopMark := func(context.Context, *ctypes.GetMarkPriceRequest) (*ctypes.GetMarkPriceResponse, error) {
		return &ctypes.GetMarkPriceResponse{}, nil
	}
	noopIndex := func(context.Context, *ctypes.GetIndexPriceRequest) (*ctypes.GetIndexPriceResponse, error) {
		return &ctypes.GetIndexPriceResponse{}, nil
	}
	noopTicker := func(context.Context, *ctypes.GetTickerRequest) (*ctypes.GetTickerResponse, error) {
		return &ctypes.GetTickerResponse{}, nil
	}
	noopTrades := func(context.Context, *ctypes.GetTradesRequest) (*ctypes.GetTradesResponse, error) {
		return &ctypes.GetTradesResponse{}, nil
	}
	noopDepth := func(context.Context, *ctypes.GetOrderBookRequest) (*ctypes.GetOrderBookResponse, error) {
		return &ctypes.GetOrderBookResponse{}, nil
	}
	noopKlines := func(context.Context, *ctypes.GetKlinesRequest) (*ctypes.GetKlinesResponse, error) {
		return &ctypes.GetKlinesResponse{}, nil
	}
	noopHisKlines := func(context.Context, *ctypes.GetHisKlinesRequest) (*ctypes.GetHisKlinesResponse, error) {
		return &ctypes.GetHisKlinesResponse{}, nil
	}
	noopFunding := func(context.Context, *ctypes.GetFundingRateRequest) (*ctypes.GetFundingRateResponse, error) {
		return &ctypes.GetFundingRateResponse{}, nil
	}
	noopHisFunding := func(context.Context, *ctypes.GetHisFundingRatesRequest) (*ctypes.GetHisFundingRatesResponse, error) {
		return &ctypes.GetHisFundingRatesResponse{}, nil
	}
	noopOI := func(context.Context, *ctypes.GetOpenInterestRequest) (*ctypes.GetOpenInterestResponse, error) {
		return &ctypes.GetOpenInterestResponse{}, nil
	}
	noopBalance := func(context.Context, *ctypes.GetBalanceRequest) (*ctypes.GetBalanceResponse, error) {
		return &ctypes.GetBalanceResponse{}, nil
	}
	noopPos := func(context.Context, *ctypes.GetPositionsRequest) (*ctypes.GetPositionsResponse, error) {
		return &ctypes.GetPositionsResponse{}, nil
	}
	noopOpenOrders := func(context.Context, *ctypes.GetOpenOrdersRequest) (*ctypes.GetOpenOrdersResponse, error) {
		return &ctypes.GetOpenOrdersResponse{}, nil
	}
	noopOrder := func(context.Context, *ctypes.GetOrderRequest) (*ctypes.GetOrderResponse, error) {
		return &ctypes.GetOrderResponse{}, nil
	}
	noopCancel := func(context.Context, *ctypes.CancelOrderRequest) (*ctypes.CancelOrderResponse, error) {
		return &ctypes.CancelOrderResponse{Success: true}, nil
	}
	noopGetLev := func(context.Context, *ctypes.GetLeverageRequest) (*ctypes.GetLeverageResponse, error) {
		return &ctypes.GetLeverageResponse{Leverage: 1}, nil
	}
	noopSetLev := func(context.Context, *ctypes.SetLeverageRequest) (*ctypes.SetLeverageResponse, error) {
		return &ctypes.SetLeverageResponse{Leverage: 1}, nil
	}
	noopFreeze := func(context.Context, *ctypes.FundsFreezeRequest) (*ctypes.FundsFreezeResponse, error) {
		return &ctypes.FundsFreezeResponse{Success: true}, nil
	}
	noopUnfreeze := func(context.Context, *ctypes.FundsUnfreezeRequest) (*ctypes.FundsUnfreezeResponse, error) {
		return &ctypes.FundsUnfreezeResponse{Success: true}, nil
	}
	noopSub := func(context.Context, *ctypes.SubscribeStreamRequest) (<-chan *ctypes.SubscribeStreamResponse, error) {
		ch := make(chan *ctypes.SubscribeStreamResponse)
		close(ch)
		return ch, nil
	}

	proxy.AssignStub(
		noopMarkets, noopMarket, noopPrice, noopBookPrice, noopMark, noopIndex, noopTicker, noopTrades, noopDepth,
		noopKlines, noopHisKlines, noopFunding, noopHisFunding, noopOI,
		entityLayer.GetSymbolConfig,
		noopBalance, noopPos, noopOpenOrders, noopOrder,
		entityLayer.PlaceOrder, noopCancel, noopGetLev, noopSetLev, noopFreeze, noopUnfreeze, noopSub,
	)
}

func TestPaperProxyServiceEntitySimulateChain(t *testing.T) {
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
		Bids:   []ctypes.OrderBookLevel{{Price: decimal.NewFromInt(99), Size: decimal.NewFromInt(2)}},
		Asks:   []ctypes.OrderBookLevel{{Price: decimal.NewFromInt(101), Size: decimal.NewFromInt(2)}},
		Ts:     time.Now(),
	}
	conn := newTestConnector("paper-acct", market, depth)
	_ = conn.state.ex.InitBalances("paper-acct", seedUSDT(ctypes.WalletTypeSpot, decimal.NewFromInt(10000)))
	_, _ = conn.GetMarket(context.Background(), symbol)
	_, _ = conn.Depth(context.Background(), symbol, 20)

	entityLayer := &testEntityLayer{conn: conn}
	assignTestProxyStubs(entityLayer)

	cfg, err := proxy.GetSymbolConfig(context.Background(), "paper-acct", symbol)
	if err != nil {
		t.Fatalf("proxy get symbol config failed: %v", err)
	}
	if cfg == nil || !cfg.Market.Rules.TickSize.Equal(decimal.RequireFromString("0.1")) {
		t.Fatalf("unexpected symbol config: %+v", cfg)
	}

	ex := ctypes.ExchangeBinance
	accountID := "paper-acct"
	intent := stypes.OrderPlaceIntent{
		BaseSignal: stypes.BaseSignal{
			Exchange:  &ex,
			Symbol:    &symbol,
			AccountID: &accountID,
		},
		ClientOrderID: ctypes.OrderId("paper-cid-1"),
		IsBuy:         true,
		Side:          ctypes.PositionSideLong,
		OrderType:     ctypes.OrderTypeLimit,
		Price:         lo.ToPtr(decimal.RequireFromString("100")),
		Quantity:      lo.ToPtr(decimal.RequireFromString("0.2")),
	}
	orderID, clientID, err := proxy.PlaceOrder(context.Background(), intent)
	if err != nil {
		t.Fatalf("proxy place order failed: %v", err)
	}
	if orderID == "" || clientID != "paper-cid-1" {
		t.Fatalf("unexpected place order result: orderID=%s clientID=%s", orderID, clientID)
	}
}
