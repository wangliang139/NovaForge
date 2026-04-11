package types

import (
	"time"

	"github.com/shopspring/decimal"
)

type Market struct {
	Exchange Exchange     `json:"exchange,omitempty"`
	Symbol   Symbol       `json:"symbol,omitempty"`
	Status   MarketStatus `json:"status,omitempty"`

	BaseAssetPrecision  int `json:"baseAssetPrecision,omitempty"`  // 基础资产精度
	QuoteAssetPrecision int `json:"quoteAssetPrecision,omitempty"` // 报价资产精度
	PricePrecision      int `json:"pricePrecision,omitempty"`      // 下单价格精度

	Rules MarketRules `json:"rules,omitempty"` // 市场过滤器

	OrderTypeRules []OrderTypeRule `json:"orderTypeRules,omitempty"` // 支持的下单类型
}

type MarketType string

const (
	MarketTypeSpot   MarketType = "SPOT"   // 现货
	MarketTypeFuture MarketType = "FUTURE" // 永续合约(U本位)
)

func (m MarketType) Valid() bool {
	switch m {
	case MarketTypeSpot, MarketTypeFuture:
		return true
	}
	return false
}

func AllMarketTypes() []MarketType {
	return []MarketType{MarketTypeSpot, MarketTypeFuture}
}

type OrderTypeRule struct {
	OrderType OrderType `json:"orderType,omitempty"`

	Rules MarketRules `json:"rules,omitempty"`
}

type MarketRules struct {
	MaxOrderNum int `json:"maxOrderNum,omitempty"` // 最大挂单数量

	MinPrice decimal.Decimal `json:"minPrice,omitempty"` // 最小价格
	MaxPrice decimal.Decimal `json:"maxPrice,omitempty"` // 最大价格
	TickSize decimal.Decimal `json:"tickSize,omitempty"` // 下单价格精度/步长 price % tickSize == 0

	MinQuantity decimal.Decimal `json:"minQuantity,omitempty"` // 最小数量
	MaxQuantity decimal.Decimal `json:"maxQuantity,omitempty"` // 最大数量
	LotSize     decimal.Decimal `json:"lotSize,omitempty"`     // 下单数量精度/步长 quantity % lotSize == 0

	MinNotional decimal.Decimal `json:"minNotional,omitempty"` // 最小订单价值 price * quantity >= minNotional
	MaxNotional decimal.Decimal `json:"maxNotional,omitempty"` // 最大订单价值 price * quantity <= maxNotional
}

type MarketStatus string

const (
	MarketStatusPending    MarketStatus = "PENDING"    // 待交易
	MarketStatusTrading    MarketStatus = "TRADING"    // 交易中
	MarketStatusClosing    MarketStatus = "CLOSING"    // 收盘
	MarketStatusDelivering MarketStatus = "DELIVERING" // 交割中（合约）
	MarketStatusSettling   MarketStatus = "SETTLING"   // 结算中（合约）
	MarketStatusSuspended  MarketStatus = "SUSPENDED"  // 暂停
	MarketStatusDelisted   MarketStatus = "DELISTED"   // 下架
)

func (m MarketStatus) Valid() bool {
	switch m {
	case MarketStatusPending, MarketStatusTrading, MarketStatusClosing, MarketStatusDelivering, MarketStatusSettling, MarketStatusSuspended, MarketStatusDelisted:
		return true
	}
	return false
}

type FundingRate struct {
	Exchange        Exchange        `json:"exchange,omitempty"`
	Symbol          Symbol          `json:"symbol,omitempty"`
	FundingRate     decimal.Decimal `json:"fundingRate,omitempty"`
	InterestRate    decimal.Decimal `json:"interestRate,omitempty"`
	NextFundingTime time.Time       `json:"nextFundingTime,omitempty"`
	Ts              time.Time       `json:"ts,omitempty"`
}

type FundingInfo struct {
	Symbol                   Symbol `json:"symbol,omitempty"`
	AdjustedFundingRateCap   string `json:"adjustedFundingRateCap,omitempty"`
	AdjustedFundingRateFloor string `json:"adjustedFundingRateFloor,omitempty"`
	FundingIntervalHours     *int   `json:"fundingIntervalHours,omitempty"`
}

// --- Market 服务请求/响应 DTO（避免依赖 protobuf 生成类型）---

type GetExchangesRequest struct{}

type GetExchangesResponse struct {
	Exchanges []Exchange `json:"exchanges,omitempty"`
}

type GetMarketsRequest struct {
	Exchange    Exchange     `json:"exchange,omitempty"`
	AccountID   string       `json:"accountId,omitempty"`
	MarketTypes []MarketType `json:"marketTypes,omitempty"`
}

type GetMarketsResponse struct {
	Markets []*Market `json:"markets,omitempty"`
}

type GetMarketRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
}

type GetMarketResponse struct {
	Market *Market `json:"market,omitempty"`
}

type GetTickerRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
}

type GetTickerResponse struct {
	Ticker *Ticker `json:"ticker,omitempty"`
}

type GetTradesRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
	Limit     *int     `json:"limit,omitempty"`
}

type GetTradesResponse struct {
	Trades []*Trade `json:"trades,omitempty"`
}

type GetOrderBookRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
	Depth     *int     `json:"depth,omitempty"`
}

type GetOrderBookResponse struct {
	Snapshot *OrderBook `json:"snapshot,omitempty"`
}

type GetKlinesRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
	Interval  string   `json:"interval,omitempty"`
	Limit     *int     `json:"limit,omitempty"`
}

type GetKlinesResponse struct {
	Klines []*Kline `json:"klines,omitempty"`
}

type GetHisKlinesRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
	Interval  string   `json:"interval,omitempty"`
	StartTs   *int64   `json:"startTs,omitempty"`
	EndTs     *int64   `json:"endTs,omitempty"`
	Limit     *int     `json:"limit,omitempty"`
}

type GetHisKlinesResponse struct {
	Klines []*Kline `json:"klines,omitempty"`
}

type GetPriceRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
}

type GetPriceResponse struct {
	Price *Price `json:"price,omitempty"`
}

type GetHisPriceRequest struct{}

type GetHisPriceResponse struct{}

type GetBookPriceRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
}

type GetBookPriceResponse struct {
	BookPrice *BookPrice `json:"bookPrice,omitempty"`
}

type GetMarkPriceRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
}

type GetMarkPriceResponse struct {
	MarkPrice *MarkPrice `json:"markPrice,omitempty"`
}

type GetFundingRateRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
}

type GetFundingRateResponse struct {
	FundingRate *FundingRate `json:"fundingRate,omitempty"`
}

type GetHisFundingRatesRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
	StartTs   *int64   `json:"startTs,omitempty"`
	EndTs     *int64   `json:"endTs,omitempty"`
	Limit     *int     `json:"limit,omitempty"`
}

type GetHisFundingRatesResponse struct {
	FundingRates []*FundingRate `json:"fundingRates,omitempty"`
}

type GetOpenInterestRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
}

type GetOpenInterestResponse struct {
	OpenInterest string `json:"openInterest,omitempty"`
}

type GetLeverageBracketRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
	MarkPrice string   `json:"markPrice,omitempty"`
}

type GetLeverageBracketResponse struct {
	LeverageBracket *LeverageBracket `json:"leverageBracket,omitempty"`
}

type GetIndexPriceRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
}

type GetIndexPriceResponse struct {
	IndexPrice *IndexPrice `json:"indexPrice,omitempty"`
}

type GetIndexComponentRequest struct {
	Exchange  Exchange `json:"exchange,omitempty"`
	AccountID string   `json:"accountId,omitempty"`
	Symbol    string   `json:"symbol,omitempty"`
}

type GetIndexComponentResponse struct {
	IndexComponent *IndexComponent `json:"indexComponent,omitempty"`
}
