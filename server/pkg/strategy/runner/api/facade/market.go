package facade

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/proxy"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/marketdata"
)

// MarketFacade 市场数据外观模式，统一回测和实盘的市场数据访问
type MarketFacade struct {
	// 回测场景：优先使用 MarketProvider（从事件总线缓存）
	provider marketdata.MarketProvider

	// 是否为回测模式
	isBacktest bool
}

// NewMarketFacade 创建 MarketFacade
func NewMarketFacade(provider marketdata.MarketProvider, isBacktest bool) *MarketFacade {
	return &MarketFacade{
		provider:   provider,
		isBacktest: isBacktest,
	}
}

// GetTicker 获取ticker
func (f *MarketFacade) GetTicker(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, period time.Duration) (map[string]any, error) {
	// 优先使用 provider 缓存
	ticker, err := f.provider.GetTicker(ctx, exchange, symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get market data: %w", err)
	}
	if ticker == nil {
		return nil, errors.New("ticker is nil")
	}

	return map[string]any{
		"exchange":       ticker.Exchange.String(),
		"symbol":         ticker.Symbol.String(),
		"lastPrice":      ticker.LastPrice.String(),
		"open24h":        ticker.Open24.String(),
		"high24h":        ticker.High24.String(),
		"low24h":         ticker.Low24.String(),
		"avg24h":         ticker.Avg24.String(),
		"volume24h":      ticker.Volume24.String(),
		"quoteVolume24h": ticker.QuoteVolume24.String(),
		"ts":             ticker.Ts.UnixMilli(),
	}, nil
}

// GetDepth 获取深度
func (f *MarketFacade) GetDepth(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, depth int) (map[string]any, error) {
	if depth <= 0 {
		depth = 100
	}
	orderBook, err := f.provider.GetDepth(ctx, exchange, symbol, depth)
	if err != nil {
		return nil, fmt.Errorf("failed to get order book: %w", err)
	}
	if orderBook == nil {
		return nil, errors.New("order book is nil")
	}
	return orderBookToMap(orderBook), nil
}

func orderBookToMap(orderBook *ctypes.OrderBook) map[string]any {
	bids := make([]map[string]any, 0, len(orderBook.Bids))
	for _, bid := range orderBook.Bids {
		bids = append(bids, map[string]any{
			"price": bid.Price.String(),
			"size":  bid.Size.String(),
		})
	}
	asks := make([]map[string]any, 0, len(orderBook.Asks))
	for _, ask := range orderBook.Asks {
		asks = append(asks, map[string]any{
			"price": ask.Price.String(),
			"size":  ask.Size.String(),
		})
	}
	return map[string]any{
		"bids": bids,
		"asks": asks,
		"ts":   orderBook.Ts.UnixMilli(),
	}
}

// GetTrades 获取成交记录
func (f *MarketFacade) GetTrades(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, period time.Duration) ([]map[string]any, error) {
	limit := 100
	if f.provider != nil {
		trades, err := f.provider.GetTrades(ctx, exchange, symbol, limit)
		if err == nil {
			return tradesToMaps(trades, period), nil
		}
	}
	trades, err := proxy.GetTrades(ctx, exchange, symbol, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get trades: %w", err)
	}
	return tradesToMaps(trades, period), nil
}

func tradesToMaps(trades []*ctypes.Trade, period time.Duration) []map[string]any {
	if len(trades) == 0 {
		return []map[string]any{}
	}
	result := make([]map[string]any, 0)
	cutoff := time.Time{}
	if period > 0 {
		cutoff = time.Now().Add(-period)
	}
	for _, trade := range trades {
		if period > 0 && trade.Ts.Before(cutoff) {
			continue
		}
		result = append(result, map[string]any{
			"tradeId":  trade.TradeID,
			"exchange": trade.Exchange.String(),
			"symbol":   trade.Symbol.String(),
			"price":    trade.Price.String(),
			"size":     trade.Size.String(),
			"isBuy":    trade.IsBuy,
			"ts":       trade.Ts.UnixMilli(),
		})
	}
	return result
}

// GetKlines 获取K线
func (f *MarketFacade) GetKlines(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, interval ctypes.Interval, limit int, start, end *time.Time) ([]map[string]any, error) {
	var klines []*ctypes.Kline
	var err error

	klines, err = f.provider.GetKlines(ctx, exchange, symbol, interval, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get klines: %w", err)
	}
	if len(klines) == 0 {
		return []map[string]any{}, nil
	}

	result := make([]map[string]any, 0, len(klines))
	for _, k := range klines {
		result = append(result, map[string]any{
			"open":     k.Open.String(),
			"high":     k.High.String(),
			"low":      k.Low.String(),
			"close":    k.Close.String(),
			"volume":   k.Volume.String(),
			"openTs":   k.OpenTs.UnixMilli(),
			"closeTs":  k.CloseTs.UnixMilli(),
			"isClosed": k.IsClosed,
		})
	}

	if len(result) > 0 {
		log.Info().Str("start", time.UnixMilli(result[0]["openTs"].(int64)).Format(time.RFC3339)).
			Str("end", time.UnixMilli(result[len(result)-1]["closeTs"].(int64)).Format(time.RFC3339)).
			Str("symbol", symbol.String()).
			Str("exchange", exchange.String()).
			Str("interval", interval.String()).
			Int("limit", limit).
			Msg("klines")
	}

	return result, nil
}

// GetMarkets 获取市场列表
//
// marketType 支持：
// - "all" / ""：不过滤
// - "spot"：仅现货
// - "future"：仅合约
func (f *MarketFacade) GetMarkets(ctx context.Context, exchange ctypes.Exchange, marketType string) ([]map[string]any, error) {
	markets, err := proxy.GetMarkets(ctx, exchange)
	if err != nil {
		return nil, fmt.Errorf("failed to get markets: %w", err)
	}
	if len(markets) == 0 {
		return []map[string]any{}, nil
	}

	mt := strings.ToLower(strings.TrimSpace(marketType))

	out := make([]map[string]any, 0, len(markets))
	for _, m := range markets {
		if m == nil || !m.Symbol.IsValid() {
			continue
		}
		switch mt {
		case "", "all":
			// no-op
		case "spot":
			if m.Symbol.Type != ctypes.MarketTypeSpot {
				continue
			}
		case "future":
			if m.Symbol.Type != ctypes.MarketTypeFuture {
				continue
			}
		default:
			// 不认识的类型：按不过滤处理，避免把数据过滤没了导致策略误判
		}

		supportOrderTypes := make([]map[string]any, 0, len(m.OrderTypeRules))
		for _, ot := range m.OrderTypeRules {
			supportOrderTypes = append(supportOrderTypes, map[string]any{
				"orderType": ot.OrderType.String(),
				"rules": map[string]any{
					"maxOrderNum": ot.Rules.MaxOrderNum,
					"minPrice":    ot.Rules.MinPrice.String(),
					"maxPrice":    ot.Rules.MaxPrice.String(),
					"tickSize":    ot.Rules.TickSize.String(),
					"minQuantity": ot.Rules.MinQuantity.String(),
					"maxQuantity": ot.Rules.MaxQuantity.String(),
					"lotSize":     ot.Rules.LotSize.String(),
					"minNotional": ot.Rules.MinNotional.String(),
					"maxNotional": ot.Rules.MaxNotional.String(),
				},
			})
		}

		out = append(out, map[string]any{
			"exchange":            m.Exchange.String(),
			"symbol":              m.Symbol.String(),
			"status":              string(m.Status),
			"baseAssetPrecision":  m.BaseAssetPrecision,
			"quoteAssetPrecision": m.QuoteAssetPrecision,
			"pricePrecision":      m.PricePrecision,
			"rules": map[string]any{
				"maxOrderNum": m.Rules.MaxOrderNum,
				"minPrice":    m.Rules.MinPrice.String(),
				"maxPrice":    m.Rules.MaxPrice.String(),
				"tickSize":    m.Rules.TickSize.String(),
				"minQuantity": m.Rules.MinQuantity.String(),
				"maxQuantity": m.Rules.MaxQuantity.String(),
				"lotSize":     m.Rules.LotSize.String(),
				"minNotional": m.Rules.MinNotional.String(),
				"maxNotional": m.Rules.MaxNotional.String(),
			},
			"supportOrderTypes": supportOrderTypes,
		})
	}
	return out, nil
}

// GetTickers 获取 tickers（默认按 ExchangeHandle.symbols 过滤）
//
// 返回结构：
//
//	{
//	  exchange: "binance",
//	  tickers: { "BTC/USDT:SPOT": {...}, ... },
//	  ts: 1700000000000
//	}
func (f *MarketFacade) GetTickers(ctx context.Context, exchange ctypes.Exchange, symbols []ctypes.Symbol, period time.Duration) (map[string]any, error) {
	tickers := make(map[string]any, 0)

	for _, sym := range symbols {
		if !sym.IsValid() {
			continue
		}
		ticker, err := f.GetTicker(ctx, exchange, sym, period)
		if err != nil {
			return nil, err
		}
		if ticker == nil {
			continue
		}
		tickers[sym.String()] = ticker
	}

	return map[string]any{
		"exchange": exchange.String(),
		"tickers":  tickers,
		"ts":       time.Now().UnixMilli(),
	}, nil
}
