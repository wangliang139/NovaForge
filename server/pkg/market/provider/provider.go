package provider

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/market/connector"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

const (
	CacheKeyPrefix = "mdp"

	KlineCacheSize  = 300
	TradesCacheSize = 500
	DepthCacheSize  = 100

	CacheTTLMarkets      = 10 * time.Minute
	CacheTTLMarket       = 3 * time.Minute
	CacheTTLLastPrice    = 5 * time.Second
	CacheTTLBookPrice    = 1 * time.Second
	CacheTTLMarkPrice    = 5 * time.Second
	CacheTTLIndexPrice   = 5 * time.Second
	CacheTTLFundingRate  = 1 * time.Minute
	CacheTTLOpenInterest = 5 * time.Minute
	CacheTTLPrice        = 5 * time.Second
	CacheTTLKlines       = 5 * time.Second
	CacheTTLTrades       = 5 * time.Second
	CacheTTLDepth        = 5 * time.Second
	CacheTTLTicker       = 30 * time.Second

	CacheKeyMarkets      = "markets:%s"
	CacheKeyMarket       = "market:%s:%s"
	CacheKeyPrice        = "price:%s:%s"
	CacheKeyBookPrice    = "bookprice:%s:%s"
	CacheKeyMarkPrice    = "markprice:%s:%s"
	CacheKeyIndexPrice   = "indexprice:%s:%s"
	CacheKeyFundingRate  = "funding_rate:%s:%s"
	CacheKeyOpenInterest = "open_interest:%s:%s"
	CacheKeyTicker       = "ticker:%s:%s"
	CacheKeyTrades       = "trades:%s:%s"
	CacheKeyDepth        = "depth:%s:%s"
	CacheKeyKlines       = "klines:%s:%s:%s"
	CacheKeyPriceAt      = "priceat:%s:%s:%s:%d"
)

func newKey(key string, params ...any) string {
	return fmt.Sprintf("%s:%s", CacheKeyPrefix, fmt.Sprintf(key, params...))
}

// MarketProvider 基于事件的市场数据提供器（用于回测场景）
type MarketProvider struct {
	cache *Cache

	mu      sync.Mutex
	stopped atomic.Bool
	ch      chan *ctypes.Message

	baseExchange ctypes.Exchange
	baseCurrency string
}

// NewMarketProvider 创建基于事件的市场数据提供器
func NewMarketProvider(baseExchange ctypes.Exchange, baseCurrency string) *MarketProvider {
	provider := &MarketProvider{
		cache:        NewCache(time.Minute, time.Minute),
		baseExchange: baseExchange,
		baseCurrency: baseCurrency,
		mu:           sync.Mutex{},
		stopped:      atomic.Bool{},
		ch:           make(chan *ctypes.Message, 1000),
	}
	return provider
}

func (p *MarketProvider) Start() error {
	go func() {
		for ev := range p.ch {
			switch ev.GetStreamType() {
			case ctypes.StreamTypeKline:
				p.OnKlineEvent(ev)
			case ctypes.StreamTypeTrade:
				// p.OnTradeEvent(ev)
				continue
			case ctypes.StreamTypeDepth:
				// p.OnDepthEvent(ev)
				continue
			case ctypes.StreamTypeTicker:
				p.OnTickerEvent(ev)
			case ctypes.StreamTypeMarkPrice:
				p.OnMarkPriceEvent(ev)
			}
		}
	}()
	return nil
}

func (p *MarketProvider) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped.Load() {
		return
	}
	p.stopped.Store(true)
	close(p.ch)
}

func (p *MarketProvider) connect(exchange ctypes.Exchange) mdtypes.Connector {
	conn, err := connector.GetConnector(exchange, nil)
	if err != nil {
		log.Error().Err(err).Msg("get connector failed")
		return nil
	}
	return conn
}

// OnEvent 处理市场事件，更新缓存
func (p *MarketProvider) OnEvent(ctx context.Context, ev *ctypes.Message) error {
	if ev == nil || !ev.GetStreamType().IsMarketSignal() || ev.GetExchange() == nil || ev.GetSymbol() == nil {
		return nil
	}

	p.mu.Lock()
	if p.stopped.Load() {
		p.mu.Unlock()
		return nil
	}
	select {
	case p.ch <- ev:
	default:
		log.Warn().Str("type", ev.GetStreamType().String()).Msg("market data provider channel full, dropping event")
	}
	p.mu.Unlock()
	return nil
}

func (p *MarketProvider) OnKlineEvent(ev *ctypes.Message) error {
	if ev == nil || ev.Kline == nil {
		return nil
	}

	exchange := ev.Exchange
	symbol := lo.FromPtr(ev.GetSymbol())

	if ev.Ts.Before(time.Now().Add(-3 * time.Second)) {
		return nil
	}

	kline := ev.Kline

	// 保存最新价格
	lastPrice := &ctypes.Price{
		Exchange: exchange,
		Symbol:   symbol,
		Price:    kline.Close,
		Ts:       ev.Ts,
	}
	key := newKey(CacheKeyPrice, exchange.String(), symbol.String())
	ttl := time.Until(ev.Ts.Add(CacheTTLPrice))
	_, _ = p.cache.SwapByTtl(key, lastPrice, ttl)

	// 保存 K 线
	interval, err := kline.Interval.Duration()
	if err != nil {
		return nil
	}
	key = newKey(CacheKeyKlines, exchange.String(), symbol.String(), kline.Interval.String())
	ddl := ev.Ts.Add(CacheTTLKlines)
	p.cache.SwapByFn(key, func(old any, exp time.Time, ok bool) (any, time.Time, bool) {
		if !ok {
			return nil, exp, false
		}
		if !exp.Before(time.Now().Add(ttl)) {
			return old, exp, false
		}
		openTs := kline.OpenTs
		// 如果有值，则根据时间戳进行替换最新 bar
		if klines, ok := old.([]*ctypes.Kline); ok && len(klines) > 0 {
			last := klines[len(klines)-1]
			if last.OpenTs.After(kline.OpenTs) {
				return klines, exp, false
			}
			// 当前 K 线
			if last.OpenTs.Equal(openTs) {
				klines[len(klines)-1] = kline
				return klines, ddl, true
			}
			// 下一条 K 线
			if last.OpenTs.Add(interval).Equal(openTs) {
				// 关闭当前K线
				last.IsClosed = true
				last.Close = kline.Open
				last.CloseTs = kline.OpenTs
				// 创建新 K 线
				klines = append(klines, kline)
				// 移除最后一条K线
				klines = klines[1:]
				return klines, ddl, true
			}
			return klines, exp, false
		}
		return old, exp, false
	})

	return nil
}

func (p *MarketProvider) OnTradeEvent(ev *ctypes.Message) error {
	if ev == nil || ev.Trade == nil {
		return nil
	}

	exchange := ev.Exchange
	symbol := lo.FromPtr(ev.GetSymbol())

	if ev.Ts.Before(time.Now().Add(-5 * time.Second)) {
		return nil
	}

	trade := ev.Trade

	// 保存 Trade
	key := newKey(CacheKeyTrades, exchange.String(), symbol.String())
	ttl := trade.Ts.Add(CacheTTLTrades)
	p.cache.SwapByFn(key, func(old any, exp time.Time, ok bool) (any, time.Time, bool) {
		if !ok {
			return nil, exp, false
		}
		// 如果有值，则根据时间戳进行替换最新 bar
		if trades, ok := old.([]*ctypes.Trade); ok && len(trades) > 0 {
			sort.SliceStable(trades, func(i, j int) bool {
				return trades[i].Ts.After(trades[j].Ts)
			})
			exist := false
			for i := range trades {
				if trades[i].TradeID == trade.TradeID {
					exist = true
					break
				}
			}
			if exist {
				return trades, exp, false
			}
			// 移除最旧的成交
			trades = trades[:len(trades)-1]
			trades = append(trades, trade)
			if ttl.After(exp) {
				return trades, ttl, true
			}
			return trades, exp, true
		}
		return old, exp, false
	})
	return nil
}

// mergeDepthLevels 合并深度档位：相同价格覆盖、数量为0删除、不存在则添加；
// 排序后：买单从高到低取前 DepthCacheSize 条，卖单从高到低取最后 DepthCacheSize 条（即最靠近盘口的卖价）
func mergeDepthLevels(base, updates []ctypes.OrderBookLevel, isBid bool) []ctypes.OrderBookLevel {
	m := make(map[string]ctypes.OrderBookLevel)
	for _, l := range base {
		m[l.Price.String()] = l
	}
	for _, l := range updates {
		if l.Size.IsZero() {
			delete(m, l.Price.String())
		} else {
			m[l.Price.String()] = l
		}
	}
	result := make([]ctypes.OrderBookLevel, 0, len(m))
	for _, l := range m {
		result = append(result, l)
	}
	// 按价格排序：买单和卖单都从高到低
	sort.Slice(result, func(i, j int) bool {
		return result[i].Price.GreaterThan(result[j].Price)
	})
	// 买单：取前 DepthCacheSize 条（最高价）
	// 卖单：取最后 DepthCacheSize 条（最低价，即最靠近盘口）
	if len(result) <= DepthCacheSize {
		return result
	}
	if isBid {
		return result[:DepthCacheSize]
	}
	return result[len(result)-DepthCacheSize:]
}

func (p *MarketProvider) OnDepthEvent(ev *ctypes.Message) error {
	if ev == nil || ev.Depth == nil {
		return nil
	}

	exchange := ev.Exchange
	symbol := lo.FromPtr(ev.GetSymbol())

	if ev.Ts.Before(time.Now().Add(-5 * time.Second)) {
		return nil
	}

	depth := ev.Depth

	key := newKey(CacheKeyDepth, exchange.String(), symbol.String())
	ttl := depth.Ts.Add(CacheTTLDepth)
	p.cache.SwapByFn(key, func(old any, exp time.Time, ok bool) (any, time.Time, bool) {
		var book *ctypes.OrderBook
		needRefresh := !ok || old == nil
		if ok && old != nil {
			book = old.(*ctypes.OrderBook)
			if book == nil || book.SeqId != depth.PrevSeqId {
				needRefresh = true
			}
		}

		if needRefresh {
			snapshot, err := p.GetDepth(context.Background(), exchange, symbol, DepthCacheSize)
			if err != nil {
				log.Error().Err(err).Msg("get order book failed")
				return old, exp, false
			}
			book = snapshot
		}

		baseBids := book.Bids
		if baseBids == nil {
			baseBids = []ctypes.OrderBookLevel{}
		}
		baseAsks := book.Asks
		if baseAsks == nil {
			baseAsks = []ctypes.OrderBookLevel{}
		}
		updBids := depth.Bids
		if updBids == nil {
			updBids = []ctypes.OrderBookLevel{}
		}
		updAsks := depth.Asks
		if updAsks == nil {
			updAsks = []ctypes.OrderBookLevel{}
		}

		merged := &ctypes.OrderBook{
			Exchange:  exchange,
			Symbol:    symbol,
			Bids:      mergeDepthLevels(baseBids, updBids, true),
			Asks:      mergeDepthLevels(baseAsks, updAsks, false),
			Ts:        depth.Ts,
			SeqId:     depth.SeqId,
			PrevSeqId: depth.PrevSeqId,
		}
		return merged, ttl, true
	})

	return nil
}

func (p *MarketProvider) OnTickerEvent(ev *ctypes.Message) error {
	if ev == nil || ev.Ticker == nil {
		return nil
	}

	if ev.Ts.Before(time.Now().Add(-5 * time.Second)) {
		return nil
	}

	ticker := ev.Ticker

	// 保存最新价格
	lastPrice := &ctypes.Price{
		Exchange: ticker.Exchange,
		Symbol:   ticker.Symbol,
		Price:    ticker.LastPrice,
		Ts:       ticker.Ts,
	}
	key := newKey(CacheKeyPrice, ticker.Exchange.String(), ticker.Symbol.String())
	ttl := time.Until(ticker.Ts.Add(CacheTTLPrice))
	_, _ = p.cache.SwapByTtl(key, lastPrice, ttl)

	// 保存ticker
	key = newKey(CacheKeyTicker, ticker.Exchange.String(), ticker.Symbol.String())
	ttl = time.Until(ticker.Ts.Add(CacheTTLTicker))
	_, _ = p.cache.SwapByTtl(key, ticker, ttl)
	return nil
}

func (p *MarketProvider) OnMarkPriceEvent(ev *ctypes.Message) error {
	if ev == nil || ev.MarkPrice == nil {
		return nil
	}

	if ev.Ts.Before(time.Now().Add(-3 * time.Second)) {
		return nil
	}

	markPrice := ev.MarkPrice

	// 保存标记价格
	key := newKey(CacheKeyMarkPrice, markPrice.Exchange.String(), markPrice.Symbol.String())
	ttl := time.Until(markPrice.Ts.Add(CacheTTLMarkPrice))
	_, _ = p.cache.SwapByTtl(key, markPrice, ttl)
	return nil
}

func (p *MarketProvider) GetMarkets(ctx context.Context, ex ctypes.Exchange) ([]*ctypes.Market, error) {
	key := newKey(CacheKeyMarkets, ex.String())
	markets, err := p.cache.Get(ctx, key, CacheTTLMarkets, func(ctx context.Context, params ...any) (any, error) {
		conn := p.connect(ex)
		if conn == nil {
			return nil, errors.New("connector not found")
		}
		return conn.GetMarkets(ctx, nil)
	})
	if err != nil {
		return nil, err
	}
	return markets.([]*ctypes.Market), nil
}

// GetMarket 获取市场
func (p *MarketProvider) GetMarket(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.Market, error) {
	key := newKey(CacheKeyMarket, ex.String(), symbol.String())
	market, err := p.cache.Get(ctx, key, CacheTTLMarket, func(ctx context.Context, params ...any) (any, error) {
		conn := p.connect(ex)
		if conn == nil {
			return nil, errors.New("connector not found")
		}
		return conn.GetMarket(ctx, symbol)
	})
	if err != nil {
		return nil, err
	}
	return market.(*ctypes.Market), nil
}

func (p *MarketProvider) GetPrices(ctx context.Context, ex ctypes.Exchange) ([]*ctypes.Price, error) {
	conn := p.connect(ex)
	if conn == nil {
		return nil, errors.New("connector not found")
	}
	prices, err := conn.Prices(ctx, nil)
	if err != nil {
		return nil, err
	}
	return prices, nil
}

// GetLastPrice 获取最新价格
func (p *MarketProvider) GetLastPrice(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, error) {
	if symbol.Base == symbol.Quote {
		return decimal.NewFromInt(1), nil
	}
	key := newKey(CacheKeyPrice, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLLastPrice, func(ctx context.Context, params ...any) (any, error) {
		conn := p.connect(ex)
		if conn == nil {
			return nil, errors.New("connector not found")
		}
		return conn.Price(ctx, symbol)
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
		conn := p.connect(ex)
		if conn == nil {
			return nil, errors.New("connector not found")
		}
		return conn.Price(ctx, reverseExSymbol)
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

// GetBookPrice 获取盘口价格
func (p *MarketProvider) GetBookPrice(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, decimal.Decimal, error) {
	key := newKey(CacheKeyBookPrice, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLBookPrice, func(ctx context.Context, params ...any) (any, error) {
		conn := p.connect(ex)
		if conn == nil {
			return nil, errors.New("connector not found")
		}
		return conn.BookPrice(ctx, symbol)
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

func (p *MarketProvider) GetMarkPrices(ctx context.Context, ex ctypes.Exchange) ([]*ctypes.MarkPrice, error) {
	conn := p.connect(ex)
	if conn == nil {
		return nil, errors.New("connector not found")
	}
	return conn.MarkPrices(ctx)
}

// GetMarkPrice 获取标记价格
func (p *MarketProvider) GetMarkPrice(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, error) {
	key := newKey(CacheKeyMarkPrice, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLMarkPrice, func(ctx context.Context, params ...any) (any, error) {
		conn := p.connect(ex)
		if conn == nil {
			return nil, errors.New("connector not found")
		}
		return conn.MarkPrice(ctx, symbol)
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

// GetIndexPrice 获取指数价格
func (p *MarketProvider) GetIndexPrice(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, error) {
	key := newKey(CacheKeyIndexPrice, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLIndexPrice, func(ctx context.Context, params ...any) (any, error) {
		conn := p.connect(ex)
		if conn == nil {
			return nil, errors.New("connector not found")
		}
		return conn.IndexPrice(ctx, symbol)
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

// GetFundingRate 获取资金费率
func (p *MarketProvider) GetFundingRate(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.FundingRate, error) {
	key := newKey(CacheKeyFundingRate, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLFundingRate, func(ctx context.Context, params ...any) (any, error) {
		conn := p.connect(ex)
		if conn == nil {
			return nil, errors.New("connector not found")
		}
		return conn.FundingRate(ctx, symbol)
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

func (p *MarketProvider) GetHisFundingRates(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.FundingRate, error) {
	conn := p.connect(ex)
	if conn == nil {
		return nil, errors.New("connector not found")
	}
	fundingRates, err := conn.HisFundingRates(ctx, symbol, startTs, endTs, limit)
	if err != nil {
		return nil, err
	}
	return fundingRates, nil
}

func (p *MarketProvider) GetOpenInterest(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (decimal.Decimal, error) {
	key := newKey(CacheKeyOpenInterest, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLOpenInterest, func(ctx context.Context, params ...any) (any, error) {
		conn := p.connect(ex)
		if conn == nil {
			return nil, errors.New("connector not found")
		}
		return conn.OpenInterest(ctx, symbol)
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

// GetTicker 获取ticker
func (p *MarketProvider) GetTicker(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol) (*ctypes.Ticker, error) {
	key := newKey(CacheKeyTicker, ex.String(), symbol.String())
	ticker, err := p.cache.Get(ctx, key, CacheTTLTicker, func(ctx context.Context, params ...any) (any, error) {
		conn := p.connect(ex)
		if conn == nil {
			return nil, errors.New("connector not found")
		}
		return conn.Ticker(ctx, symbol)
	})
	if err != nil {
		return nil, err
	}
	return ticker.(*ctypes.Ticker), nil
}

// GetKlines 获取 K 线：先读缓存，不足时 RPC 并回写缓存
func (p *MarketProvider) GetKlines(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, interval ctypes.Interval, limit int) ([]*ctypes.Kline, error) {
	if limit <= 0 || limit > KlineCacheSize {
		limit = KlineCacheSize
	}

	key := newKey(CacheKeyKlines, ex.String(), symbol.String(), interval.String())
	result, err := p.cache.Get(ctx, key, CacheTTLKlines, func(ctx context.Context, params ...any) (any, error) {
		conn := p.connect(ex)
		if conn == nil {
			return nil, errors.New("connector not found")
		}
		return conn.Klines(ctx, symbol, interval, KlineCacheSize)
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

func (p *MarketProvider) GetHisKlines(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, interval ctypes.Interval, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.Kline, error) {
	conn := p.connect(ex)
	if conn == nil {
		return nil, errors.New("connector not found")
	}
	bars, err := conn.HisKlines(ctx, symbol, interval, startTs, endTs, limit)
	if err != nil {
		return nil, err
	}
	return bars, nil
}

func (p *MarketProvider) GetPriceAt(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, ts time.Time, interval ctypes.Interval) (decimal.Decimal, error) {
	intervalDur, err := interval.Duration()
	if err != nil {
		return decimal.Zero, err
	}
	barStart := alignToIntervalStart(ts, intervalDur)
	key := newKey(CacheKeyPriceAt, ex.String(), symbol.String(), interval.String(), barStart.UnixMilli())
	result, err := p.cache.Get(ctx, key, intervalDur, func(ctx context.Context, params ...any) (any, error) {
		limit := 1
		endTs := barStart.Add(intervalDur)
		bars, err := p.GetHisKlines(ctx, ex, symbol, interval, &barStart, &endTs, &limit)
		if err != nil {
			return nil, err
		}
		price, ok := priceAtFromKlines(bars, ts, intervalDur)
		if !ok {
			return nil, errors.New("price not found at time")
		}
		return &price, nil
	})
	if err != nil {
		return decimal.Zero, err
	}
	if result == nil {
		return decimal.Zero, errors.New("price not found at time")
	}
	return *(result.(*decimal.Decimal)), nil
}

func alignToIntervalStart(ts time.Time, intervalDur time.Duration) time.Time {
	if intervalDur <= 0 {
		return ts
	}
	return ts.Truncate(intervalDur)
}

func priceAtFromKlines(bars []*ctypes.Kline, ts time.Time, intervalDur time.Duration) (decimal.Decimal, bool) {
	barStart := alignToIntervalStart(ts, intervalDur)
	for _, bar := range bars {
		if bar == nil {
			continue
		}
		if bar.OpenTs.Equal(barStart) {
			return bar.Open, true
		}
		if !bar.OpenTs.After(ts) && bar.OpenTs.Add(intervalDur).After(ts) {
			return bar.Open, true
		}
	}
	return decimal.Zero, false
}

// GetDepth 获取深度：先读缓存，未命中时 RPC 并回写缓存
func (p *MarketProvider) GetDepth(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, limit int) (*ctypes.OrderBook, error) {
	if limit <= 0 || limit > DepthCacheSize {
		limit = DepthCacheSize
	}

	key := newKey(CacheKeyDepth, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLDepth, func(ctx context.Context, params ...any) (any, error) {
		conn := p.connect(ex)
		if conn == nil {
			return nil, errors.New("connector not found")
		}
		return conn.Depth(ctx, symbol, DepthCacheSize)
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
		if limit > len(orderBook.Bids) {
			limit = len(orderBook.Bids)
		}
		if limit > len(orderBook.Asks) {
			limit = len(orderBook.Asks)
		}
		out.Ts = orderBook.Ts
		copy(out.Bids, orderBook.Bids[len(orderBook.Bids)-limit:])
		copy(out.Asks, orderBook.Asks[len(orderBook.Asks)-limit:])
		return out, nil
	}
	return nil, errors.New("depth not found")
}

// GetTrades 获取成交：先读缓存，未命中时 RPC 并回写缓存
func (p *MarketProvider) GetTrades(ctx context.Context, ex ctypes.Exchange, symbol ctypes.Symbol, limit int) ([]*ctypes.Trade, error) {
	if limit <= 0 || limit > TradesCacheSize {
		limit = TradesCacheSize
	}
	key := newKey(CacheKeyTrades, ex.String(), symbol.String())
	result, err := p.cache.Get(ctx, key, CacheTTLTrades, func(ctx context.Context, params ...any) (any, error) {
		conn := p.connect(ex)
		if conn == nil {
			return nil, errors.New("connector not found")
		}
		return conn.Trades(ctx, symbol, TradesCacheSize)
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

// GetPriceInBaseCurrency 获取资产在 BaseCurrency 中的价格
func (p *MarketProvider) GetPriceInBaseCurrency(ctx context.Context, asset string, quote string) (decimal.Decimal, error) {
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
