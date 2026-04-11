package okx

import (
	"context"
	"fmt"
	"sync"
	"time"

	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/market/connector/cache"
)

const (
	CacheKeyMarkets          = "api:markets:%s"
	CacheKeyTickers          = "api:tickers:%s"
	CacheKeyMarkPrices       = "api:mark:prices"
	CacheKeyMarkPrice        = "api:mark:price:%s"
	CacheKeyIndexPrice       = "api:index:price:%s"
	CacheKeyTicker           = "api:ticker:%s"
	CacheKeySymbolInfo       = "api:symbol:info:%s:%s"
	CacheKeyLeverageBrackets = "api:leverage:brackets:%s"
	CacheKeyOpenInterest     = "api:open:interest:%s:%s"
	CacheKeyFundingRate      = "api:funding:rate:%s"
	CacheKeyIndexComponents  = "api:index:components:%s"

	CacheKeyApiAccountConfig    = "api:account:config"
	CacheKeyFundingValues       = "api:funding:values"
	CacheKeyFundingBalances     = "api:funding:balances"
	CacheKeyTradingBalances     = "api:trading:balances"
	CacheKeyAccountTradeFee     = "api:account:trade:fee:%s"
	CacheKeyAccountLeverageInfo = "api:account:leverage:info:%s"
)

var PublicCacheKeys = map[string]bool{
	CacheKeyMarkets:          true,
	CacheKeyTickers:          true,
	CacheKeyMarkPrices:       true,
	CacheKeyMarkPrice:        true,
	CacheKeyIndexPrice:       true,
	CacheKeyTicker:           true,
	CacheKeySymbolInfo:       true,
	CacheKeyLeverageBrackets: true,
	CacheKeyOpenInterest:     true,
	CacheKeyFundingRate:      true,
	CacheKeyIndexComponents:  true,
}

// publicCacheStore 在 binance 维度复用公共市场数据缓存，
// 按 exchange + proxyURL 进行区分，避免多账户重复拉取相同 public 数据。
var (
	publicCacheMu    sync.Mutex
	publicCacheStore = map[string]*cache.Cache{}
)

func getPublicCache(c *Connector) *cache.Cache {
	key := fmt.Sprintf("%s", c.exchange.String())

	publicCacheMu.Lock()
	defer publicCacheMu.Unlock()

	if cc, ok := publicCacheStore[key]; ok {
		return cc
	}

	cc := cache.NewCache(5*time.Minute, 10*time.Minute)
	setupPublicCache(c, cc)
	publicCacheStore[key] = cc
	return cc
}

func (c *Connector) SetupCache() {
	c.cache.Register(CacheKeyApiAccountConfig, 5*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.restClient.NewAccountConfigService().Do(ctx)
	})
	c.cache.Register(CacheKeyFundingValues, time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.restClient.NewFundingAssetValuationService().Ccy("USDT").Do(ctx)
	})
	c.cache.Register(CacheKeyFundingBalances, time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.restClient.NewFundingAssetBalancesService().Do(ctx)
	})
	c.cache.Register(CacheKeyTradingBalances, time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.restClient.NewAccountBalanceService().Do(ctx)
	})
	c.cache.Register(CacheKeyAccountTradeFee, 30*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no symbol provided")
		}
		symbol, ok := params[0].(ctypes.Symbol)
		if !ok {
			return nil, fmt.Errorf("invalid symbol: %T", params[0])
		}
		instType := FormatMarketType(symbol.Type)
		instId := Symbol2InstId(symbol)
		instFamily := Symbol2InstFamily(symbol)
		svc := c.restClient.NewAccountTradeFeeService().InstType(instType)
		if symbol.Type == ctypes.MarketTypeSpot {
			svc = svc.InstId(instId)
		} else {
			svc = svc.InstFamily(instFamily)
		}
		return svc.Do(ctx)
	})
	c.cache.Register(CacheKeyAccountLeverageInfo, 1*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no inst id provided")
		}
		instId, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid inst id: %T", params[0])
		}
		return c.restClient.NewGetLeverageInfoService().InstId(instId).MgnMode("cross").Do(ctx)
	})
}

// setupPublicCache 只在公共缓存实例上注册「公共市场数据」相关的 key，
// 回调仍然复用当前 Connector 的 client，但不依赖账户状态。
func setupPublicCache(c *Connector, target *cache.Cache) {
	target.Register(CacheKeyMarkets, 30*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no market type provided")
		}
		instType, ok := params[0].(string)
		if !ok || instType == "" {
			return nil, fmt.Errorf("invalid market type: %T", params[0])
		}
		return c.restClient.NewSymbolInfoService().InstType(instType).Do(ctx)
	})
	target.Register(CacheKeyLeverageBrackets, 30*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no inst family provided")
		}
		instFamily, ok := params[0].(string)
		if !ok || instFamily == "" {
			return nil, fmt.Errorf("invalid inst family: %T", params[0])
		}
		return c.restClient.NewPositionTiersService().InstType("SWAP").TdMode("cross").InstFamily(instFamily).Do(ctx)
	})
	target.Register(CacheKeySymbolInfo, 10*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		if len(params) < 2 {
			return nil, fmt.Errorf("no inst id or inst type provided")
		}
		instId, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid inst id: %T", params[0])
		}
		instType, ok := params[1].(string)
		if !ok {
			return nil, fmt.Errorf("invalid inst type: %T", params[1])
		}
		return c.restClient.NewSymbolInfoService().InstType(instType).InstId(instId).Do(ctx)
	})
	target.Register(CacheKeyFundingRate, 1*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no symbol provided")
		}
		instId, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid inst id: %T", params[0])
		}
		return c.restClient.NewFundingRateService().InstId(instId).Do(ctx)
	})
	target.Register(CacheKeyTickers, 5*time.Second, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no market type provided")
		}
		instType, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid market type: %T", params[0])
		}
		return c.restClient.NewMarketTickersService().InstType(instType).Do(ctx)
	})
	target.Register(CacheKeyMarkPrices, 5*time.Second, func(ctx context.Context, params ...any) (any, error) {
		return c.restClient.NewMarkPriceService().InstType("SWAP").Do(ctx)
	})
	target.Register(CacheKeyTicker, 3*time.Second, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no inst id provided")
		}
		instId, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid inst id: %T", params[0])
		}
		return c.restClient.NewSymbolQuotationService().InstId(instId).Do(ctx)
	})
	target.Register(CacheKeyMarkPrice, 3*time.Second, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no inst id provided")
		}
		instId, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid inst id: %T", params[0])
		}
		return c.restClient.NewMarkPriceService().InstType("SWAP").InstId(instId).Do(ctx)
	})
	target.Register(CacheKeyIndexPrice, 5*time.Second, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no symbol provided")
		}
		instId, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid inst id: %T", params[0])
		}
		return c.restClient.NewIndexTickersService().InstId(instId).Do(ctx)
	})
	target.Register(CacheKeyOpenInterest, 5*time.Second, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no symbol provided")
		}
		instType, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid inst type: %T", params[0])
		}
		instId, ok := params[1].(string)
		if !ok {
			return nil, fmt.Errorf("invalid inst id: %T", params[1])
		}
		return c.restClient.NewOpenInterestService().InstType(instType).InstId(instId).Do(ctx)
	})

	target.Register(CacheKeyIndexComponents, 5*time.Second, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no symbol provided")
		}
		instId, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid inst id: %T", params[0])
		}
		return c.restClient.NewIndexComponentsService().Index(instId).Do(ctx)
	})
}

func fetchWithType[T any](c *Connector, ctx context.Context, key string, params ...any) (T, error) {
	var zero T
	cache := c.cache
	if isPublic, ok := PublicCacheKeys[key]; ok && isPublic {
		cache = getPublicCache(c)
	}
	data, err := cache.SimpleFetch(ctx, key, params...)
	if err != nil {
		return zero, err
	}
	return data.(T), nil
}
