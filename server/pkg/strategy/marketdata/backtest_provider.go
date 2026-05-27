package marketdata

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/proxy"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

const backtestCacheTTL = 24 * time.Hour

// BacktestMarketProvider 回测专用：同步 OnEvent 更新缓存，不依赖异步 channel 与墙钟新鲜度判断。
type BacktestMarketProvider struct {
	cache        *Cache
	baseExchange ctypes.Exchange
	baseCurrency string

	mu      sync.Mutex
	stopped atomic.Bool
}

var _ MarketProvider = (*BacktestMarketProvider)(nil)

func NewBacktestMarketProvider(baseExchange ctypes.Exchange, baseCurrency string) *BacktestMarketProvider {
	return &BacktestMarketProvider{
		cache:        NewCache(time.Minute, time.Minute),
		baseExchange: baseExchange,
		baseCurrency: baseCurrency,
	}
}

func (p *BacktestMarketProvider) Start() error {
	return nil
}

func (p *BacktestMarketProvider) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped.Store(true)
}

func (p *BacktestMarketProvider) OnEvent(ctx context.Context, ev stypes.Signal) error {
	_ = ctx
	if !ev.GetType().IsMarketSignal() {
		return nil
	}
	if ev.GetExchange() == nil || ev.GetSymbol() == nil {
		return nil
	}
	p.mu.Lock()
	stopped := p.stopped.Load()
	p.mu.Unlock()
	if stopped {
		return nil
	}
	switch ev.GetType() {
	case stypes.SignalTypeKline:
		return p.onKline(ev.(*stypes.KlineSignal))
	case stypes.SignalTypeTrade:
		return p.onTrade(ev.(*stypes.TradeSignal))
	case stypes.SignalTypeDepth:
		return p.onDepth(ev.(*stypes.DepthSignal))
	case stypes.SignalTypeTicker:
		return p.onTicker(ev.(*stypes.TickerSignal))
	case stypes.SignalTypeMarkPrice:
		return p.onMarkPrice(ev.(*stypes.MarkPriceSignal))
	default:
		return nil
	}
}

func (p *BacktestMarketProvider) onKline(kline *stypes.KlineSignal) error {
	if kline == nil || kline.Exchange == nil || kline.Symbol == nil {
		return nil
	}
	if !kline.IsClosed {
		return nil
	}
	lastPrice := &ctypes.Price{
		Exchange: *kline.Exchange,
		Symbol:   *kline.Symbol,
		Price:    kline.Close,
		Ts:       kline.Ts,
	}
	key := newKey(CacheKeyPrice, kline.Exchange.String(), kline.Symbol.String())
	_, _ = p.cache.SwapByTtl(key, lastPrice, backtestCacheTTL)

	interval, err := kline.Interval.Duration()
	if err != nil {
		return nil
	}
	openTs := time.UnixMilli(kline.OpenTs)
	key = newKey(CacheKeyKlines, kline.Exchange.String(), kline.Symbol.String(), kline.Interval.String())
	ddl := kline.Ts.Add(backtestCacheTTL)
	p.cache.SwapByFn(key, func(old any, exp time.Time, ok bool) (any, time.Time, bool) {
		bar := &ctypes.Kline{
			Exchange: *kline.Exchange,
			Symbol:   *kline.Symbol,
			Interval: kline.Interval,
			Open:     kline.Open,
			High:     kline.High,
			Low:      kline.Low,
			Close:    kline.Close,
			Volume:   kline.Volume,
			IsClosed: kline.IsClosed,
			OpenTs:   openTs,
			CloseTs:  openTs.Add(interval),
		}
		if !ok || old == nil {
			return []*ctypes.Kline{bar}, ddl, true
		}
		klines, typed := old.([]*ctypes.Kline)
		if !typed || len(klines) == 0 {
			return []*ctypes.Kline{bar}, ddl, true
		}
		last := klines[len(klines)-1]
		if last.OpenTs.Equal(openTs) {
			bar.OpenTs = last.OpenTs
			bar.CloseTs = last.CloseTs
			klines[len(klines)-1] = bar
			return klines, ddl, true
		}
		if last.OpenTs.Add(interval).Equal(openTs) {
			last.IsClosed = true
			last.Close = kline.Open
			last.CloseTs = openTs
			klines = append(klines, bar)
			if len(klines) > KlineCacheSize {
				klines = klines[len(klines)-KlineCacheSize:]
			}
			return klines, ddl, true
		}
		klines = append(klines, bar)
		if len(klines) > KlineCacheSize {
			klines = klines[len(klines)-KlineCacheSize:]
		}
		return klines, ddl, true
	})
	return nil
}

func (p *BacktestMarketProvider) onTrade(trade *stypes.TradeSignal) error {
	if trade == nil || trade.Exchange == nil || trade.Symbol == nil {
		return nil
	}
	key := newKey(CacheKeyTrades, trade.Exchange.String(), trade.Symbol.String())
	ttl := trade.Ts.Add(backtestCacheTTL)
	p.cache.SwapByFn(key, func(old any, exp time.Time, ok bool) (any, time.Time, bool) {
		t := &ctypes.Trade{
			Exchange: *trade.Exchange,
			Symbol:   *trade.Symbol,
			TradeID:  trade.TradeID,
			Price:    trade.Price,
			Size:     trade.Size,
			IsBuy:    trade.IsBuy,
			Ts:       trade.Ts,
		}
		if !ok || old == nil {
			return []*ctypes.Trade{t}, ttl, true
		}
		trades, typed := old.([]*ctypes.Trade)
		if !typed || len(trades) == 0 {
			return []*ctypes.Trade{t}, ttl, true
		}
		sort.SliceStable(trades, func(i, j int) bool {
			return trades[i].Ts.After(trades[j].Ts)
		})
		for i := range trades {
			if trades[i].TradeID == trade.TradeID {
				return trades, ttl, true
			}
		}
		trades = append(trades, t)
		sort.SliceStable(trades, func(i, j int) bool {
			return trades[i].Ts.After(trades[j].Ts)
		})
		if len(trades) > TradesCacheSize {
			trades = trades[len(trades)-TradesCacheSize:]
		}
		return trades, ttl, true
	})
	return nil
}

func (p *BacktestMarketProvider) onDepth(depth *stypes.DepthSignal) error {
	if depth.OrderBook == nil {
		return nil
	}
	key := newKey(CacheKeyDepth, depth.Exchange.String(), depth.Symbol.String())
	ttl := depth.Ts.Add(backtestCacheTTL)
	p.cache.SwapByFn(key, func(old any, exp time.Time, ok bool) (any, time.Time, bool) {
		var book *ctypes.OrderBook
		needRefresh := !ok || old == nil
		if ok && old != nil {
			book = old.(*ctypes.OrderBook)
			if book == nil || book.SeqId != depth.OrderBook.PrevSeqId {
				needRefresh = true
			}
		}
		if needRefresh {
			book = &ctypes.OrderBook{
				Exchange:  *depth.Exchange,
				Symbol:    *depth.Symbol,
				Bids:      []ctypes.OrderBookLevel{},
				Asks:      []ctypes.OrderBookLevel{},
				Ts:        depth.Ts,
				SeqId:     depth.OrderBook.SeqId,
				PrevSeqId: depth.OrderBook.PrevSeqId,
			}
		}
		baseBids := book.Bids
		if baseBids == nil {
			baseBids = []ctypes.OrderBookLevel{}
		}
		baseAsks := book.Asks
		if baseAsks == nil {
			baseAsks = []ctypes.OrderBookLevel{}
		}
		updBids := depth.OrderBook.Bids
		if updBids == nil {
			updBids = []ctypes.OrderBookLevel{}
		}
		updAsks := depth.OrderBook.Asks
		if updAsks == nil {
			updAsks = []ctypes.OrderBookLevel{}
		}
		merged := &ctypes.OrderBook{
			Exchange:  *depth.Exchange,
			Symbol:    *depth.Symbol,
			Bids:      mergeDepthLevels(baseBids, updBids, true),
			Asks:      mergeDepthLevels(baseAsks, updAsks, false),
			Ts:        depth.Ts,
			SeqId:     depth.OrderBook.SeqId,
			PrevSeqId: depth.OrderBook.PrevSeqId,
		}
		return merged, ttl, true
	})
	return nil
}

func (p *BacktestMarketProvider) onTicker(signal *stypes.TickerSignal) error {
	lastPrice := &ctypes.Price{
		Exchange: *signal.Exchange,
		Symbol:   *signal.Symbol,
		Price:    signal.LastPrice,
		Ts:       signal.Ts,
	}
	key := newKey(CacheKeyPrice, signal.Exchange.String(), signal.Symbol.String())
	_, _ = p.cache.SwapByTtl(key, lastPrice, backtestCacheTTL)

	ticker := &ctypes.Ticker{
		Exchange:      *signal.Exchange,
		Symbol:        *signal.Symbol,
		LastPrice:     signal.LastPrice,
		Open24:        signal.Open24,
		High24:        signal.High24,
		Low24:         signal.Low24,
		Avg24:         signal.Avg24,
		Volume24:      signal.Volume24,
		QuoteVolume24: signal.QuoteVolume24,
		Ts:            signal.Ts,
	}
	key = newKey(CacheKeyTicker, ticker.Exchange.String(), ticker.Symbol.String())
	_, _ = p.cache.SwapByTtl(key, ticker, backtestCacheTTL)
	return nil
}

func (p *BacktestMarketProvider) onMarkPrice(signal *stypes.MarkPriceSignal) error {
	markPrice := &ctypes.MarkPrice{
		Exchange:  *signal.Exchange,
		Symbol:    *signal.Symbol,
		MarkPrice: signal.Price,
		Ts:        signal.Ts,
	}
	key := newKey(CacheKeyMarkPrice, markPrice.Exchange.String(), markPrice.Symbol.String())
	_, _ = p.cache.SwapByTtl(key, markPrice, backtestCacheTTL)
	return nil
}

// ====== 查询接口：在回测中优先从本地缓存读取，必要时仍可走 proxy，语义与 GlobalMarketProvider 保持一致 ======

func (p *BacktestMarketProvider) GetMarkets(ctx context.Context, ex ctypes.Exchange) ([]*ctypes.Market, error) {
	key := newKey(CacheKeyMarkets, ex.String())
	markets, err := p.cache.Get(ctx, key, CacheTTLMarkets, func(ctx context.Context, params ...any) (any, error) {
		return proxy.GetMarkets(ctx, ex)
	})
	if err != nil {
		return nil, err
	}
	return markets.([]*ctypes.Market), nil
}

func (p *BacktestMarketProvider) GetMarket(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.Market, error) {
	key := newKey(CacheKeyMarket, ex.String(), symbol.String())
	market, err := p.cache.Get(ctx, key, CacheTTLMarket, func(ctx context.Context, params ...any) (any, error) {
		return proxy.GetMarket(ctx, ex, symbol)
	})
	if err != nil {
		return nil, err
	}
	return market.(*ctypes.Market), nil
}

func (p *BacktestMarketProvider) GetLastPrice(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, error) {
	key := newKey(CacheKeyPrice, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLLastPrice, func(ctx context.Context, params ...any) (any, error) {
		return proxy.GetPrice(ctx, ex, symbol)
	})
	if err != nil {
		return decimal.Zero, err
	}
	if result != nil {
		price := result.(*ctypes.Price)
		return price.Price, nil
	}

	// 尝试反向价格
	reverseExSymbol := ctypes.NewSymbol(symbol.Quote, symbol.Base, symbol.Type)
	key = newKey(CacheKeyPrice, ex.String(), reverseExSymbol.String())
	reversePrice, err := p.cache.Get(ctx, key, CacheTTLLastPrice, func(ctx context.Context, params ...any) (any, error) {
		return proxy.GetPrice(ctx, ex, reverseExSymbol)
	})
	if err != nil {
		return decimal.Zero, err
	}
	if reversePrice != nil {
		price := reversePrice.(*ctypes.Price)
		return decimal.NewFromInt(1).Div(price.Price), nil
	}

	return decimal.Zero, errors.New("last price not found")
}

func (p *BacktestMarketProvider) GetBookPrice(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, decimal.Decimal, error) {
	key := newKey(CacheKeyBookPrice, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLBookPrice, func(ctx context.Context, params ...any) (any, error) {
		return proxy.GetBookPrice(ctx, ex, symbol)
	})
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	if result != nil {
		bookPrice := result.(*ctypes.BookPrice)
		return bookPrice.BidPrice, bookPrice.AskPrice, nil
	}
	return decimal.Zero, decimal.Zero, errors.New("book price not found")
}

func (p *BacktestMarketProvider) GetMarkPrice(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, error) {
	key := newKey(CacheKeyMarkPrice, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLMarkPrice, func(ctx context.Context, params ...any) (any, error) {
		return proxy.GetMarkPrice(ctx, ex, symbol)
	})
	if err != nil {
		return decimal.Zero, err
	}
	if result != nil {
		markPrice := result.(*ctypes.MarkPrice)
		return markPrice.MarkPrice, nil
	}
	return decimal.Zero, errors.New("mark price not found")
}

func (p *BacktestMarketProvider) GetIndexPrice(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, error) {
	key := newKey(CacheKeyIndexPrice, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLIndexPrice, func(ctx context.Context, params ...any) (any, error) {
		return proxy.GetIndexPrice(ctx, ex, symbol)
	})
	if err != nil {
		return decimal.Zero, err
	}
	if result != nil {
		indexPrice := result.(*ctypes.IndexPrice)
		return indexPrice.IndexPrice, nil
	}
	return decimal.Zero, errors.New("index price not found")
}

func (p *BacktestMarketProvider) GetFundingRate(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.FundingRate, error) {
	key := newKey(CacheKeyFundingRate, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLFundingRate, func(ctx context.Context, params ...any) (any, error) {
		return proxy.GetFundingRate(ctx, ex, symbol)
	})
	if err != nil {
		return nil, err
	}
	if result != nil {
		fundingRate := result.(*ctypes.FundingRate)
		return fundingRate, nil
	}
	return nil, errors.New("funding rate not found")
}

func (p *BacktestMarketProvider) GetHisFundingRates(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.FundingRate, error) {
	fundingRates, err := proxy.GetHisFundingRates(ctx, ex, symbol, startTs, endTs, limit)
	if err != nil {
		return nil, err
	}
	return fundingRates, nil
}

func (p *BacktestMarketProvider) GetOpenInterest(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, error) {
	key := newKey(CacheKeyOpenInterest, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLOpenInterest, func(ctx context.Context, params ...any) (any, error) {
		return proxy.GetOpenInterest(ctx, ex, symbol)
	})
	if err != nil {
		return decimal.Zero, err
	}
	if result != nil {
		openInterest := result.(*decimal.Decimal)
		return *openInterest, nil
	}
	return decimal.Zero, errors.New("open interest not found")
}

func (p *BacktestMarketProvider) GetTicker(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.Ticker, error) {
	key := newKey(CacheKeyTicker, ex.String(), symbol.String())
	ticker, err := p.cache.Get(ctx, key, CacheTTLTicker, func(ctx context.Context, params ...any) (any, error) {
		return proxy.GetTicker(ctx, ex, symbol)
	})
	if err != nil {
		return nil, err
	}
	return ticker.(*ctypes.Ticker), nil
}

func (p *BacktestMarketProvider) GetKlines(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, interval ctypes.Interval, limit int) ([]*ctypes.Kline, error) {
	if limit <= 0 || limit > KlineCacheSize {
		limit = KlineCacheSize
	}

	key := newKey(CacheKeyKlines, ex.String(), symbol.String(), interval.String())
	result, err := p.cache.Get(ctx, key, CacheTTLKlines, func(ctx context.Context, params ...any) (any, error) {
		return proxy.GetKlines(ctx, ex, symbol, interval, KlineCacheSize)
	})
	if err != nil {
		return nil, err
	}
	if result != nil {
		klines := result.([]*ctypes.Kline)
		if limit > len(klines) {
			limit = len(klines)
		}
		out := make([]*ctypes.Kline, limit)
		copy(out, klines[len(klines)-limit:])
		return out, nil
	}
	return nil, errors.New("klines not found")
}

func (p *BacktestMarketProvider) GetHisKlines(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, interval ctypes.Interval, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.Kline, error) {
	bars, err := proxy.GetHisKlines(ctx, ex, symbol, interval, startTs, endTs, limit)
	if err != nil {
		return nil, err
	}
	return bars, nil
}

func (p *BacktestMarketProvider) GetDepth(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, limit int) (*ctypes.OrderBook, error) {
	if limit <= 0 || limit > DepthCacheSize {
		limit = DepthCacheSize
	}

	key := newKey(CacheKeyDepth, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLDepth, func(ctx context.Context, params ...any) (any, error) {
		return proxy.GetOrderBook(ctx, ex, symbol, DepthCacheSize)
	})
	if err != nil {
		return nil, err
	}
	out := &ctypes.OrderBook{
		Symbol: symbol,
		Bids:   make([]ctypes.OrderBookLevel, 0, limit),
		Asks:   make([]ctypes.OrderBookLevel, 0, limit),
		Ts:     time.Now(),
	}
	if result != nil {
		orderBook := result.(*ctypes.OrderBook)
		bidN := limit
		if bidN > len(orderBook.Bids) {
			bidN = len(orderBook.Bids)
		}
		askN := limit
		if askN > len(orderBook.Asks) {
			askN = len(orderBook.Asks)
		}
		out.Bids = make([]ctypes.OrderBookLevel, bidN)
		out.Asks = make([]ctypes.OrderBookLevel, askN)
		out.Ts = orderBook.Ts
		if bidN > 0 {
			copy(out.Bids, orderBook.Bids[len(orderBook.Bids)-bidN:])
		}
		if askN > 0 {
			copy(out.Asks, orderBook.Asks[len(orderBook.Asks)-askN:])
		}
		return out, nil
	}
	return nil, errors.New("depth not found")
}

func (p *BacktestMarketProvider) GetTrades(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, limit int) ([]*ctypes.Trade, error) {
	if limit <= 0 || limit > TradesCacheSize {
		limit = TradesCacheSize
	}
	key := newKey(CacheKeyTrades, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLTrades, func(ctx context.Context, params ...any) (any, error) {
		return proxy.GetTrades(ctx, ex, symbol, TradesCacheSize)
	})
	if err != nil {
		return nil, err
	}
	if result != nil {
		trades := result.([]*ctypes.Trade)
		if limit > len(trades) {
			limit = len(trades)
		}
		out := make([]*ctypes.Trade, limit)
		copy(out, trades[len(trades)-limit:])
		return out, nil
	}
	return nil, errors.New("trades not found")
}

func (p *BacktestMarketProvider) GetPriceInBaseCurrency(ctx context.Context, asset string, quote string) (decimal.Decimal, error) {
	// 如果资产就是 BaseCurrency，返回 1
	if strings.EqualFold(asset, quote) {
		return decimal.NewFromInt(1), nil
	}

	// 尝试直接获取 asset/BaseCurrency 价格
	symbol := ctypes.NewSymbol(asset, quote, ctypes.MarketTypeSpot)
	price, err := p.GetLastPrice(ctx, p.baseExchange, symbol)
	if err == nil {
		return price, nil
	}

	// 如果直接交易对不存在，尝试通过 USDT 中转
	assetUsdtPrice, err1 := p.GetLastPrice(ctx, p.baseExchange, ctypes.NewSymbol(asset, "USDT", ctypes.MarketTypeSpot))
	if err1 != nil {
		return decimal.Zero, fmt.Errorf("failed to get price for %s: %w", asset, err)
	}

	// 如果 BaseCurrency 是 USDT，直接返回
	if strings.EqualFold(quote, "USDT") {
		return assetUsdtPrice, nil
	}

	usdtBasePrice, err2 := p.GetLastPrice(ctx, p.baseExchange, ctypes.NewSymbol("USDT", quote, ctypes.MarketTypeSpot))
	if err2 != nil {
		return decimal.Zero, fmt.Errorf("failed to get USDT/%s price: %w", quote, err2)
	}

	return assetUsdtPrice.Mul(usdtBasePrice), nil
}
