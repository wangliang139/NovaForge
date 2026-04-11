package proxy

import (
	"context"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Stub struct {
	GetMarketsFn         func(context.Context, *ctypes.GetMarketsRequest) (*ctypes.GetMarketsResponse, error)
	GetMarketFn          func(context.Context, *ctypes.GetMarketRequest) (*ctypes.GetMarketResponse, error)
	GetPriceFn           func(context.Context, *ctypes.GetPriceRequest) (*ctypes.GetPriceResponse, error)
	GetBookPriceFn       func(context.Context, *ctypes.GetBookPriceRequest) (*ctypes.GetBookPriceResponse, error)
	GetMarkPriceFn       func(context.Context, *ctypes.GetMarkPriceRequest) (*ctypes.GetMarkPriceResponse, error)
	GetIndexPriceFn      func(context.Context, *ctypes.GetIndexPriceRequest) (*ctypes.GetIndexPriceResponse, error)
	GetTickerFn          func(context.Context, *ctypes.GetTickerRequest) (*ctypes.GetTickerResponse, error)
	GetTradesFn          func(context.Context, *ctypes.GetTradesRequest) (*ctypes.GetTradesResponse, error)
	GetDepthFn           func(context.Context, *ctypes.GetOrderBookRequest) (*ctypes.GetOrderBookResponse, error)
	GetKlinesFn          func(context.Context, *ctypes.GetKlinesRequest) (*ctypes.GetKlinesResponse, error)
	GetHisKlinesFn       func(context.Context, *ctypes.GetHisKlinesRequest) (*ctypes.GetHisKlinesResponse, error)
	GetFundingRateFn     func(context.Context, *ctypes.GetFundingRateRequest) (*ctypes.GetFundingRateResponse, error)
	GetHisFundingRatesFn func(context.Context, *ctypes.GetHisFundingRatesRequest) (*ctypes.GetHisFundingRatesResponse, error)
	GetOpenInterestFn    func(context.Context, *ctypes.GetOpenInterestRequest) (*ctypes.GetOpenInterestResponse, error)

	GetSymbolConfigFn func(context.Context, *ctypes.GetSymbolConfigRequest) (*ctypes.GetSymbolConfigResponse, error)
	GetBalanceFn      func(context.Context, *ctypes.GetBalanceRequest) (*ctypes.GetBalanceResponse, error)
	GetPositionsFn    func(context.Context, *ctypes.GetPositionsRequest) (*ctypes.GetPositionsResponse, error)
	GetOpenOrdersFn   func(context.Context, *ctypes.GetOpenOrdersRequest) (*ctypes.GetOpenOrdersResponse, error)
	GetOrderFn        func(context.Context, *ctypes.GetOrderRequest) (*ctypes.GetOrderResponse, error)
	PlaceOrderFn      func(context.Context, *ctypes.PlaceOrderRequest) (*ctypes.PlaceOrderResponse, error)
	CancelOrderFn     func(context.Context, *ctypes.CancelOrderRequest) (*ctypes.CancelOrderResponse, error)
	GetLeverageFn     func(context.Context, *ctypes.GetLeverageRequest) (*ctypes.GetLeverageResponse, error)
	SetLeverageFn     func(context.Context, *ctypes.SetLeverageRequest) (*ctypes.SetLeverageResponse, error)
	FundsFreezeFn     func(context.Context, *ctypes.FundsFreezeRequest) (*ctypes.FundsFreezeResponse, error)
	FundsUnfreezeFn   func(context.Context, *ctypes.FundsUnfreezeRequest) (*ctypes.FundsUnfreezeResponse, error)

	SubscribeStreamFn func(context.Context, *ctypes.SubscribeStreamRequest) (<-chan *ctypes.SubscribeStreamResponse, error)
}

var stub Stub

func AssignStub(
	GetMarketsFn func(context.Context, *ctypes.GetMarketsRequest) (*ctypes.GetMarketsResponse, error),
	GetMarketFn func(context.Context, *ctypes.GetMarketRequest) (*ctypes.GetMarketResponse, error),
	GetPriceFn func(context.Context, *ctypes.GetPriceRequest) (*ctypes.GetPriceResponse, error),
	GetBookPriceFn func(context.Context, *ctypes.GetBookPriceRequest) (*ctypes.GetBookPriceResponse, error),
	GetMarkPriceFn func(context.Context, *ctypes.GetMarkPriceRequest) (*ctypes.GetMarkPriceResponse, error),
	GetIndexPriceFn func(context.Context, *ctypes.GetIndexPriceRequest) (*ctypes.GetIndexPriceResponse, error),
	GetTickerFn func(context.Context, *ctypes.GetTickerRequest) (*ctypes.GetTickerResponse, error),
	GetTradesFn func(context.Context, *ctypes.GetTradesRequest) (*ctypes.GetTradesResponse, error),
	GetDepthFn func(context.Context, *ctypes.GetOrderBookRequest) (*ctypes.GetOrderBookResponse, error),
	GetKlinesFn func(context.Context, *ctypes.GetKlinesRequest) (*ctypes.GetKlinesResponse, error),
	GetHisKlinesFn func(context.Context, *ctypes.GetHisKlinesRequest) (*ctypes.GetHisKlinesResponse, error),
	GetFundingRateFn func(context.Context, *ctypes.GetFundingRateRequest) (*ctypes.GetFundingRateResponse, error),
	GetHisFundingRatesFn func(context.Context, *ctypes.GetHisFundingRatesRequest) (*ctypes.GetHisFundingRatesResponse, error),
	GetOpenInterestFn func(context.Context, *ctypes.GetOpenInterestRequest) (*ctypes.GetOpenInterestResponse, error),
	GetSymbolConfigFn func(context.Context, *ctypes.GetSymbolConfigRequest) (*ctypes.GetSymbolConfigResponse, error),
	GetBalanceFn func(context.Context, *ctypes.GetBalanceRequest) (*ctypes.GetBalanceResponse, error),
	GetPositionsFn func(context.Context, *ctypes.GetPositionsRequest) (*ctypes.GetPositionsResponse, error),
	GetOpenOrdersFn func(context.Context, *ctypes.GetOpenOrdersRequest) (*ctypes.GetOpenOrdersResponse, error),
	GetOrderFn func(context.Context, *ctypes.GetOrderRequest) (*ctypes.GetOrderResponse, error),
	PlaceOrderFn func(context.Context, *ctypes.PlaceOrderRequest) (*ctypes.PlaceOrderResponse, error),
	CancelOrderFn func(context.Context, *ctypes.CancelOrderRequest) (*ctypes.CancelOrderResponse, error),
	GetLeverageFn func(context.Context, *ctypes.GetLeverageRequest) (*ctypes.GetLeverageResponse, error),
	SetLeverageFn func(context.Context, *ctypes.SetLeverageRequest) (*ctypes.SetLeverageResponse, error),
	FundsFreezeFn func(context.Context, *ctypes.FundsFreezeRequest) (*ctypes.FundsFreezeResponse, error),
	FundsUnfreezeFn func(context.Context, *ctypes.FundsUnfreezeRequest) (*ctypes.FundsUnfreezeResponse, error),
	SubscribeStreamFn func(context.Context, *ctypes.SubscribeStreamRequest) (<-chan *ctypes.SubscribeStreamResponse, error),
) {
	stub = Stub{
		GetMarketsFn:         GetMarketsFn,
		GetMarketFn:          GetMarketFn,
		GetPriceFn:           GetPriceFn,
		GetBookPriceFn:       GetBookPriceFn,
		GetMarkPriceFn:       GetMarkPriceFn,
		GetIndexPriceFn:      GetIndexPriceFn,
		GetTickerFn:          GetTickerFn,
		GetTradesFn:          GetTradesFn,
		GetDepthFn:           GetDepthFn,
		GetKlinesFn:          GetKlinesFn,
		GetHisKlinesFn:       GetHisKlinesFn,
		GetFundingRateFn:     GetFundingRateFn,
		GetHisFundingRatesFn: GetHisFundingRatesFn,
		GetOpenInterestFn:    GetOpenInterestFn,
		GetSymbolConfigFn:    GetSymbolConfigFn,
		GetBalanceFn:         GetBalanceFn,
		GetPositionsFn:       GetPositionsFn,
		GetOpenOrdersFn:      GetOpenOrdersFn,
		GetOrderFn:           GetOrderFn,
		PlaceOrderFn:         PlaceOrderFn,
		CancelOrderFn:        CancelOrderFn,
		GetLeverageFn:        GetLeverageFn,
		SetLeverageFn:        SetLeverageFn,
		FundsFreezeFn:        FundsFreezeFn,
		FundsUnfreezeFn:      FundsUnfreezeFn,
		SubscribeStreamFn:    SubscribeStreamFn,
	}
}

func GetMarkets(ctx context.Context, exchange ctypes.Exchange) ([]*ctypes.Market, error) {
	if !exchange.IsValid() {
		return nil, status.Error(codes.InvalidArgument, "invalid exchange")
	}
	req := &ctypes.GetMarketsRequest{
		Exchange: exchange,
	}
	resp, err := stub.GetMarketsFn(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Markets, nil
}

func GetMarket(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.Market, error) {
	if !exchange.IsValid() {
		return nil, status.Error(codes.InvalidArgument, "invalid exchange")
	}
	if !symbol.IsValid() {
		return nil, status.Error(codes.InvalidArgument, "invalid symbol")
	}
	req := &ctypes.GetMarketRequest{
		Exchange: exchange,
		Symbol:   symbol.String(),
	}
	resp, err := stub.GetMarketFn(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Market == nil {
		return nil, nil
	}
	return resp.Market, nil
}

func GetSymbolConfig(ctx context.Context, accountID string, symbol ctypes.Symbol) (*ctypes.SymbolConfig, error) {
	resp, err := stub.GetSymbolConfigFn(ctx, &ctypes.GetSymbolConfigRequest{
		AccountID: accountID,
		Symbol:    symbol.String(),
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return resp.Config, nil
}

func GetPrice(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.Price, error) {
	resp, err := stub.GetPriceFn(ctx, &ctypes.GetPriceRequest{
		Exchange: exchange,
		Symbol:   symbol.String(),
	})
	if err != nil {
		return nil, err
	}
	return resp.Price, nil
}

func GetBookPrice(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.BookPrice, error) {
	req := &ctypes.GetBookPriceRequest{
		Exchange: exchange,
		Symbol:   symbol.String(),
	}
	resp, err := stub.GetBookPriceFn(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.BookPrice, nil
}

func GetMarkPrice(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.MarkPrice, error) {
	req := &ctypes.GetMarkPriceRequest{
		Exchange: exchange,
		Symbol:   symbol.String(),
	}
	resp, err := stub.GetMarkPriceFn(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.MarkPrice, nil
}

func GetIndexPrice(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.IndexPrice, error) {
	req := &ctypes.GetIndexPriceRequest{
		Exchange: exchange,
		Symbol:   symbol.String(),
	}

	resp, err := stub.GetIndexPriceFn(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.IndexPrice, nil
}

// GetTicker 获取ticker（实时请求交易所）
func GetTicker(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.Ticker, error) {
	req := &ctypes.GetTickerRequest{
		Exchange: exchange,
		Symbol:   symbol.String(),
	}

	resp, err := stub.GetTickerFn(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Ticker, nil
}

// GetTrades 获取成交（实时请求交易所）
func GetTrades(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, limit int) ([]*ctypes.Trade, error) {
	req := &ctypes.GetTradesRequest{
		Exchange: exchange,
		Symbol:   symbol.String(),
		Limit:    lo.ToPtr(limit),
	}
	resp, err := stub.GetTradesFn(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Trades) == 0 {
		return nil, nil
	}
	result := make([]*ctypes.Trade, 0, len(resp.Trades))
	for _, t := range resp.Trades {
		result = append(result, t)
	}
	return result, nil
}

// GetOrderBook 获取订单簿（实时请求交易所）
func GetOrderBook(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, depth int) (*ctypes.OrderBook, error) {
	req := &ctypes.GetOrderBookRequest{
		Exchange: exchange,
		Symbol:   symbol.String(),
		Depth:    lo.ToPtr(depth),
	}
	resp, err := stub.GetDepthFn(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Snapshot == nil {
		return nil, nil
	}
	return resp.Snapshot, nil
}

// GetKlines 获取K线（实时请求交易所）
func GetKlines(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, interval ctypes.Interval, limit int) ([]*ctypes.Kline, error) {
	req := &ctypes.GetKlinesRequest{
		Exchange: exchange,
		Symbol:   symbol.String(),
		Interval: interval.String(),
		Limit:    lo.ToPtr(limit),
	}
	resp, err := stub.GetKlinesFn(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Klines) == 0 {
		return nil, nil
	}
	result := make([]*ctypes.Kline, 0, len(resp.Klines))
	for _, k := range resp.Klines {
		result = append(result, k)
	}
	return result, nil
}

func GetHisKlines(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, interval ctypes.Interval, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.Kline, error) {
	req := &ctypes.GetHisKlinesRequest{
		Exchange: exchange,
		Symbol:   symbol.String(),
		Interval: interval.String(),
	}
	if startTs != nil {
		req.StartTs = lo.ToPtr(int64(startTs.UnixMilli()))
	}
	if endTs != nil {
		req.EndTs = lo.ToPtr(int64(endTs.UnixMilli()))
	}
	if limit != nil {
		req.Limit = lo.ToPtr(*limit)
	}
	resp, err := stub.GetHisKlinesFn(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Klines) == 0 {
		return nil, nil
	}
	result := make([]*ctypes.Kline, 0, len(resp.Klines))
	for _, k := range resp.Klines {
		result = append(result, k)
	}
	return result, nil
}

// GetFundingRate 获取资金费率
func GetFundingRate(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.FundingRate, error) {
	req := &ctypes.GetFundingRateRequest{
		Exchange: exchange,
		Symbol:   symbol.String(),
	}
	resp, err := stub.GetFundingRateFn(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.FundingRate == nil {
		return nil, nil
	}
	return resp.FundingRate, nil
}

// GetHisFundingRates 获取历史资金费率
func GetHisFundingRates(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.FundingRate, error) {
	req := &ctypes.GetHisFundingRatesRequest{
		Exchange: exchange,
		Symbol:   symbol.String(),
	}
	if startTs != nil {
		req.StartTs = lo.ToPtr(int64(startTs.UnixMilli()))
	}
	if endTs != nil {
		req.EndTs = lo.ToPtr(int64(endTs.UnixMilli()))
	}
	if limit != nil {
		req.Limit = lo.ToPtr(*limit)
	}
	resp, err := stub.GetHisFundingRatesFn(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.FundingRates) == 0 {
		return nil, nil
	}
	result := make([]*ctypes.FundingRate, 0, len(resp.FundingRates))
	for _, fr := range resp.FundingRates {
		result = append(result, fr)
	}
	return result, nil
}

// GetOpenInterest 获取未平仓合约数
func GetOpenInterest(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, error) {
	req := &ctypes.GetOpenInterestRequest{
		Exchange: exchange,
		Symbol:   symbol.String(),
	}
	resp, err := stub.GetOpenInterestFn(ctx, req)
	if err != nil {
		return decimal.Zero, err
	}
	if resp.OpenInterest == "" {
		return decimal.Zero, nil
	}
	return decimal.RequireFromString(resp.OpenInterest), nil
}

func GetBalance(ctx context.Context, accountId string) (*ctypes.Balance, error) {
	resp, err := stub.GetBalanceFn(ctx, &ctypes.GetBalanceRequest{
		AccountID:    accountId,
		WithNotional: lo.ToPtr(false),
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Balance == nil {
		return &ctypes.Balance{Assets: []*ctypes.AssetBo{}}, nil
	}
	return resp.Balance, nil
}

func GetPositions(ctx context.Context, accountId string) ([]*ctypes.Position, error) {
	resp, err := stub.GetPositionsFn(ctx, &ctypes.GetPositionsRequest{
		AccountID: accountId,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Positions) == 0 {
		return nil, nil
	}
	return resp.Positions, nil
}

func GetOrders(ctx context.Context, accountId string, symbol *ctypes.Symbol) ([]*ctypes.Order, error) {
	req := &ctypes.GetOpenOrdersRequest{AccountID: accountId}
	if symbol != nil {
		req.Symbol = symbol.String()
	}
	resp, err := stub.GetOpenOrdersFn(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Orders) == 0 {
		return nil, nil
	}
	return resp.Orders, nil
}

// GetOrder 获取单个订单
func GetOrder(ctx context.Context, accountId string, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error) {
	req := &ctypes.GetOrderRequest{
		AccountID:     accountId,
		Symbol:        symbol.String(),
		ClientOrderID: orderId,
	}
	resp, err := stub.GetOrderFn(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Order == nil {
		return nil, nil
	}
	return resp.Order, nil
}

func PlaceOrder(ctx context.Context, intent types.OrderPlaceIntent) (string, string, error) {
	if intent.GetExchange() == nil {
		return "", "", status.Error(codes.InvalidArgument, "exchange is required")
	}
	if intent.GetSymbol() == nil {
		return "", "", status.Error(codes.InvalidArgument, "symbol is required")
	}
	if intent.GetAccountID() == nil {
		return "", "", status.Error(codes.InvalidArgument, "account id is required")
	}

	req := &ctypes.PlaceOrderRequest{
		Exchange:      *intent.GetExchange(),
		AccountID:     *intent.GetAccountID(),
		BotID:         lo.ToPtr(intent.BotID),
		ClientOrderID: lo.ToPtr(intent.ClientOrderID.String()),
		Symbol:        intent.Symbol.String(),
		Side:          intent.Side,
		IsBuy:         intent.IsBuy,
		OrderType:     intent.OrderType,
		ReduceOnly:    intent.ReduceOnly,
	}
	if intent.Price != nil {
		req.Price = lo.ToPtr(intent.Price.String())
	}
	if intent.Quantity != nil {
		req.Quantity = lo.ToPtr(intent.Quantity.String())
	}
	if intent.QuoteQty != nil {
		req.QuoteQty = lo.ToPtr(intent.QuoteQty.String())
	}
	if intent.TimeInForce != nil {
		req.TimeInForce = lo.ToPtr(intent.TimeInForce.String())
	}

	resp, err := stub.PlaceOrderFn(ctx, req)
	if err != nil {
		return "", "", err
	}
	if resp.Error != nil && *resp.Error != "" {
		return resp.OrderId, resp.ClientOrderId, status.Error(codes.Internal, *resp.Error)
	}
	return resp.OrderId, resp.ClientOrderId, nil
}

func CancelOrder(ctx context.Context, accountId string, symbol ctypes.Symbol, clientOrderID string) error {
	if !symbol.IsValid() {
		return status.Error(codes.InvalidArgument, "invalid symbol")
	}
	if clientOrderID == "" {
		return status.Error(codes.InvalidArgument, "client_order_id is required")
	}

	req := &ctypes.CancelOrderRequest{
		AccountID:     accountId,
		Symbol:        symbol.String(),
		ClientOrderID: clientOrderID,
	}
	resp, err := stub.CancelOrderFn(ctx, req)
	if err != nil {
		return err
	}
	if !resp.Success && resp.Error != nil && *resp.Error != "" {
		return status.Error(codes.Internal, *resp.Error)
	}
	return nil
}

func GetLeverage(ctx context.Context, accountId string, symbol ctypes.Symbol) (int, error) {
	if !symbol.IsValid() {
		return 0, status.Error(codes.InvalidArgument, "invalid symbol")
	}

	resp, err := stub.GetLeverageFn(ctx, &ctypes.GetLeverageRequest{
		AccountID: accountId,
		Symbol:    symbol.String(),
	})
	if err != nil {
		return 0, err
	}
	return int(resp.Leverage), nil
}

func SetLeverage(ctx context.Context, accountId string, symbol ctypes.Symbol, leverage int) (int, error) {
	resp, err := stub.SetLeverageFn(ctx, &ctypes.SetLeverageRequest{
		AccountID: accountId,
		Symbol:    symbol.String(),
		Leverage:  int32(leverage),
	})
	if err != nil {
		return 0, err
	}
	return int(resp.Leverage), nil
}

func FreezeFunds(ctx context.Context, accountId string, symbol ctypes.Symbol, asset string, amount decimal.Decimal, order *ctypes.Order) error {
	_, err := stub.FundsFreezeFn(ctx, &ctypes.FundsFreezeRequest{
		AccountID:  accountId,
		Symbol:     symbol.String(),
		Asset:      asset,
		Amount:     amount.String(),
		FreezeType: ctypes.FundsFreezeTypeOrder,
		Order:      order,
	})
	return err
}

func UnfreezeFunds(ctx context.Context, accountId string, symbol ctypes.Symbol, asset string, amount decimal.Decimal, order *ctypes.Order) error {
	_, err := stub.FundsUnfreezeFn(ctx, &ctypes.FundsUnfreezeRequest{
		AccountID:  accountId,
		Symbol:     symbol.String(),
		Asset:      asset,
		Amount:     amount.String(),
		FreezeType: ctypes.FundsFreezeTypeOrder,
		Order:      order,
	})
	return err
}

func SubscribeStream(ctx context.Context, req *ctypes.SubscribeStreamRequest) (<-chan *ctypes.SubscribeStreamResponse, error) {
	return stub.SubscribeStreamFn(ctx, req)
}
