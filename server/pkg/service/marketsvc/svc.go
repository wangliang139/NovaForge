package marketsvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/entity"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/errors"
)

type Service struct {
	db *repos.Entity
}

func New(db *repos.Entity) (*Service, error) {
	return &Service{
		db: db,
	}, nil
}

func (s *Service) GetExchanges(ctx context.Context, _ *ctypes.GetExchangesRequest) (*ctypes.GetExchangesResponse, error) {
	return &ctypes.GetExchangesResponse{
		Exchanges: []ctypes.Exchange{ctypes.ExchangeBinance, ctypes.ExchangeOkx},
	}, nil
}

func (s *Service) GetMarkets(ctx context.Context, req *ctypes.GetMarketsRequest) (*ctypes.GetMarketsResponse, error) {
	if !req.Exchange.IsValid() {
		return nil, errors.New(errors.InvalidArgument, "exchange is required")
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}
	var mts []ctypes.MarketType
	if len(req.MarketTypes) > 0 {
		mts = make([]ctypes.MarketType, 0, len(req.MarketTypes))
		for _, mt := range req.MarketTypes {
			if !mt.Valid() {
				return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid market_type: %s", mt))
			}
			mts = append(mts, mt)
		}
	} else {
		mts = ctypes.AllMarketTypes()
	}
	markets, err := conn.GetMarkets(ctx, mts)
	if err != nil {
		return nil, err
	}
	// 过滤正常交易的市场
	result := make([]*ctypes.Market, 0, len(markets))
	for _, m := range markets {
		if m.Status == ctypes.MarketStatusTrading {
			result = append(result, m)
		}
	}
	return &ctypes.GetMarketsResponse{Markets: result}, nil
}

func (s *Service) GetMarket(ctx context.Context, req *ctypes.GetMarketRequest) (*ctypes.GetMarketResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}
	smb, _ := ctypes.ParseSymbol(req.Symbol)
	markets, err := conn.GetMarkets(ctx, ctypes.AllMarketTypes())
	if err != nil {
		return nil, err
	}
	for _, m := range markets {
		// 过滤正常交易的市场
		if m.Status != ctypes.MarketStatusTrading {
			continue
		}
		if m.Symbol.Equal(smb) {
			return &ctypes.GetMarketResponse{Market: m}, nil
		}
	}
	return &ctypes.GetMarketResponse{Market: nil}, nil
}

func (s *Service) GetTicker(ctx context.Context, req *ctypes.GetTickerRequest) (*ctypes.GetTickerResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}
	smb, _ := ctypes.ParseSymbol(req.Symbol)
	ticker, err := conn.Ticker(ctx, smb)
	if err != nil {
		return nil, err
	}
	return &ctypes.GetTickerResponse{Ticker: ticker}, nil
}

func (s *Service) GetTrades(ctx context.Context, req *ctypes.GetTradesRequest) (*ctypes.GetTradesResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}
	smb, _ := ctypes.ParseSymbol(req.Symbol)
	limit := 100
	if req.Limit != nil && *req.Limit > 0 {
		limit = *req.Limit
	}
	trades, err := conn.Trades(ctx, smb, limit)
	if err != nil {
		return nil, err
	}
	return &ctypes.GetTradesResponse{Trades: trades}, nil
}

func (s *Service) GetOrderBook(ctx context.Context, req *ctypes.GetOrderBookRequest) (*ctypes.GetOrderBookResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}
	smb, _ := ctypes.ParseSymbol(req.Symbol)
	depth := 20
	if req.Depth != nil && *req.Depth > 0 {
		depth = *req.Depth
	}
	book, err := conn.Depth(ctx, smb, depth)
	if err != nil {
		return nil, err
	}
	return &ctypes.GetOrderBookResponse{Snapshot: book}, nil
}

func (s *Service) GetKlines(ctx context.Context, req *ctypes.GetKlinesRequest) (*ctypes.GetKlinesResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Interval) == "" {
		return nil, errors.New(errors.InvalidArgument, "interval is required")
	}
	itv := ctypes.Interval(req.Interval)
	if !itv.Valid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid interval: %s", req.Interval))
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}
	smb, _ := ctypes.ParseSymbol(req.Symbol)
	limit := 500
	if req.Limit != nil && *req.Limit > 0 {
		limit = *req.Limit
	}
	klines, err := conn.Klines(ctx, smb, itv, limit)
	if err != nil {
		return nil, err
	}
	return &ctypes.GetKlinesResponse{Klines: klines}, nil
}

func (s *Service) GetHisKlines(ctx context.Context, req *ctypes.GetHisKlinesRequest) (*ctypes.GetHisKlinesResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Interval) == "" {
		return nil, errors.New(errors.InvalidArgument, "interval is required")
	}
	itv := ctypes.Interval(req.Interval)
	if !itv.Valid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid interval: %s", req.Interval))
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}
	var startTs, endTs *time.Time
	if req.StartTs != nil {
		t := time.UnixMilli(*req.StartTs)
		startTs = &t
	}
	if req.EndTs != nil {
		t := time.UnixMilli(*req.EndTs)
		endTs = &t
	}
	if startTs != nil && endTs != nil && endTs.Before(*startTs) {
		return nil, errors.New(errors.InvalidArgument, "end_ts must be >= start_ts")
	}
	var limit *int
	if req.Limit != nil {
		val := *req.Limit
		limit = &val
	}
	smb, _ := ctypes.ParseSymbol(req.Symbol)
	klines, err := conn.HisKlines(ctx, smb, itv, startTs, endTs, limit)
	if err != nil {
		return nil, err
	}
	return &ctypes.GetHisKlinesResponse{Klines: klines}, nil
}

func (s *Service) GetPrice(ctx context.Context, req *ctypes.GetPriceRequest) (*ctypes.GetPriceResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}

	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}

	smb, err := ctypes.ParseSymbol(req.Symbol)
	if err != nil {
		return nil, err
	}
	price, err := conn.Price(ctx, smb)
	if err != nil {
		return nil, err
	}
	return &ctypes.GetPriceResponse{Price: price}, nil
}

func (s *Service) GetHisPrice(ctx context.Context, req *ctypes.GetHisPriceRequest) (*ctypes.GetHisPriceResponse, error) {
	_ = req
	return nil, errors.New(errors.Unimplemented, "not implemented")
}

func (s *Service) GetBookPrice(ctx context.Context, req *ctypes.GetBookPriceRequest) (*ctypes.GetBookPriceResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}

	smb, err := ctypes.ParseSymbol(req.Symbol)
	if err != nil {
		return nil, err
	}
	bookPrice, err := conn.BookPrice(ctx, smb)
	if err != nil {
		return nil, err
	}
	return &ctypes.GetBookPriceResponse{BookPrice: bookPrice}, nil
}

func (s *Service) GetMarkPrice(ctx context.Context, req *ctypes.GetMarkPriceRequest) (*ctypes.GetMarkPriceResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}

	smb, err := ctypes.ParseSymbol(req.Symbol)
	if err != nil {
		return nil, err
	}
	markPrice, err := conn.MarkPrice(ctx, smb)
	if err != nil {
		return nil, err
	}
	return &ctypes.GetMarkPriceResponse{MarkPrice: markPrice}, nil
}

func (s *Service) GetFundingRate(ctx context.Context, req *ctypes.GetFundingRateRequest) (*ctypes.GetFundingRateResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}
	smb, _ := ctypes.ParseSymbol(req.Symbol)

	rate, err := conn.FundingRate(ctx, smb)
	if err != nil {
		return nil, err
	}
	return &ctypes.GetFundingRateResponse{FundingRate: rate}, nil
}

func (s *Service) GetHisFundingRates(ctx context.Context, req *ctypes.GetHisFundingRatesRequest) (*ctypes.GetHisFundingRatesResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}

	var startTs, endTs *time.Time
	if req.StartTs != nil {
		t := time.UnixMilli(*req.StartTs)
		startTs = &t
	}
	if req.EndTs != nil {
		t := time.UnixMilli(*req.EndTs)
		endTs = &t
	}
	if startTs != nil && endTs != nil && endTs.Before(*startTs) {
		return nil, errors.New(errors.InvalidArgument, "end_ts must be >= start_ts")
	}
	var limit *int
	if req.Limit != nil {
		val := *req.Limit
		limit = &val
	}

	smb, _ := ctypes.ParseSymbol(req.Symbol)
	rates, err := conn.HisFundingRates(ctx, smb, startTs, endTs, limit)
	if err != nil {
		return nil, err
	}

	out := make([]*ctypes.FundingRate, 0, len(rates))
	for _, rate := range rates {
		if rate == nil {
			continue
		}
		out = append(out, rate)
	}
	return &ctypes.GetHisFundingRatesResponse{FundingRates: out}, nil
}

func (s *Service) GetOpenInterest(ctx context.Context, req *ctypes.GetOpenInterestRequest) (*ctypes.GetOpenInterestResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}
	smb, _ := ctypes.ParseSymbol(req.Symbol)

	openInterest, err := conn.OpenInterest(ctx, smb)
	if err != nil {
		return nil, err
	}
	return &ctypes.GetOpenInterestResponse{
		OpenInterest: openInterest.String(),
	}, nil
}

func (s *Service) GetLeverageBracket(ctx context.Context, req *ctypes.GetLeverageBracketRequest) (*ctypes.GetLeverageBracketResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.MarkPrice) == "" {
		return nil, errors.New(errors.InvalidArgument, "mark_price is required")
	}
	mp, err := decimal.NewFromString(strings.TrimSpace(req.MarkPrice))
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid mark_price: %s", req.MarkPrice))
	}

	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}
	smb, _ := ctypes.ParseSymbol(req.Symbol)
	bracket, err := conn.GetLeverageBracket(ctx, smb, mp)
	if err != nil {
		return nil, err
	}
	return &ctypes.GetLeverageBracketResponse{LeverageBracket: bracket}, nil
}

func (s *Service) GetIndexPrice(ctx context.Context, req *ctypes.GetIndexPriceRequest) (*ctypes.GetIndexPriceResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}
	smb, _ := ctypes.ParseSymbol(req.Symbol)
	idx, err := conn.IndexPrice(ctx, smb)
	if err != nil {
		return nil, err
	}
	return &ctypes.GetIndexPriceResponse{IndexPrice: idx}, nil
}

func (s *Service) GetIndexComponent(ctx context.Context, req *ctypes.GetIndexComponentRequest) (*ctypes.GetIndexComponentResponse, error) {
	if err := validateBasic(req.Exchange, req.Symbol); err != nil {
		return nil, err
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountID)
	if err != nil {
		return nil, err
	}
	smb, _ := ctypes.ParseSymbol(req.Symbol)
	comp, err := conn.IndexComponent(ctx, smb)
	if err != nil {
		return nil, err
	}
	if comp == nil {
		return &ctypes.GetIndexComponentResponse{IndexComponent: nil}, nil
	}
	comp.Exchange = req.Exchange
	comp.Symbol = smb
	return &ctypes.GetIndexComponentResponse{IndexComponent: comp}, nil
}

func validateBasic(exchange ctypes.Exchange, symbol string) error {
	if !exchange.IsValid() {
		return errors.New(errors.InvalidArgument, fmt.Sprintf("invalid exchange: %s", exchange))
	}
	smb, err := ctypes.ParseSymbol(symbol)
	if err != nil {
		return errors.New(errors.InvalidArgument, err.Error())
	}
	if !smb.IsValid() {
		return errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %s", symbol))
	}
	return nil
}
