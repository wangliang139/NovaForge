package binance

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wangliang139/llt-trade/server/pkg/market/connector/cache"
)

const (
	CacheKeySpotExchangeInfoV3       = "api:spot:exchange:info:v3"
	CacheKeyFutureExchangeInfo       = "fapi:future:exchange:info"
	CacheKeySpotTickerPrice          = "api:spot:ticker:price:%s"
	CacheKeySpotBookTicker           = "api:spot:tickers:book:%s"
	CacheKeySpotSymbolInfoV3         = "api:spot:symbol:info:v3:%s"
	CacheKeySpotSymbolTicker         = "api:spot:symbol:ticker:%s"
	CacheKeySpotPrices               = "api:spot:prices"
	CacheKeyFuturePrices             = "fapi:future:prices"
	CacheKeyFutureMarkPrices         = "fapi:future:mark:prices"
	CacheKeyFutureTickerPrice        = "fapi:future:ticker:price:%s"
	CacheKeyFutureBookTicker         = "fapi:future:tickers:book:%s"
	CacheKeyFuturePriceChangeStats   = "fapi:future:price:change:stats:%s"
	CacheKeyFutureSymbolPremiumIndex = "fapi:future:premium:index:%s"
	CacheKeySpotBookTickers          = "api:spot:tickers:book"
	CacheKeyFutureBookTickers        = "api:future:tickers:book"
	CacheKeyIndexComponent           = "api:index:component:%s"

	CacheKeySapiAccount           = "sapi:account"
	CacheKeyApiKeyPermission      = "sapi:apikey:permission"
	CacheKeyFundingAccount        = "sapi:funding:account"
	CacheKeyWalletBalance         = "sapi:wallet:balance"
	CacheKeySpotAccountV3         = "api:spot:account:v3"
	CacheKeyFutureAccountV3       = "fapi:future:account:v3"
	CacheKeyFutureAccountConfig   = "fapi:future:account:config"
	CacheKeyFutureBalance         = "fapi:future:balance"
	CacheKeyFutureSymbolConfig    = "fapi:future:symbol:config"
	CacheKeyFuturePositionRisk    = "fapi:future:position:risk"
	CacheKeyFutureLeverageBracket = "fapi:future:leverage:bracket"
	CacheKeyFutureFundingInfo     = "fapi:future:funding:info"
	CacheKeyFuturePositionMode    = "fapi:future:position:mode"
	CacheKeyPortfolioAccountPro   = "sapi:portfolio:account:pro"
	CacheKeyPortfolioBalance      = "sapi:portfolio:balance"
	CacheKeyPortfolioUmBalance    = "sapi:portfolio:um:balance"
	CacheKeyUMAccountConfig       = "papi:portfolio:um:account:config"
	CacheKeyUMSymbolConfig        = "papi:portfolio:um:symbol:config"
	CacheKeyUMLeverageBracket     = "papi:portfolio:um:leverage:bracket"
	CacheKeyUMPositionRisk        = "papi:portfolio:um:position:risk"
	CacheKeyUMPositionMode        = "papi:portfolio:um:position:mode"
)

var PublicCacheKeys = map[string]bool{
	CacheKeySpotExchangeInfoV3:       true,
	CacheKeyFutureExchangeInfo:       true,
	CacheKeySpotTickerPrice:          true,
	CacheKeySpotBookTicker:           true,
	CacheKeySpotSymbolInfoV3:         true,
	CacheKeySpotSymbolTicker:         true,
	CacheKeySpotPrices:               true,
	CacheKeyFuturePrices:             true,
	CacheKeyFutureMarkPrices:         true,
	CacheKeyFutureTickerPrice:        true,
	CacheKeyFutureBookTicker:         true,
	CacheKeyFuturePriceChangeStats:   true,
	CacheKeyFutureSymbolPremiumIndex: true,
	CacheKeySpotBookTickers:          true,
	CacheKeyFutureBookTickers:        true,
	CacheKeyIndexComponent:           true,
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

// SetupCache 仍然针对当前账户实例注册完整的缓存，包括 public + private。
// public 相关的 key 会被公共缓存重复注册一遍，用于多账户共享。
func (c *Connector) SetupCache() {
	c.cache.Register(CacheKeySapiAccount, 10*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.spotClient.NewGetSapiAccountInfoService().Do(ctx)
	})
	c.cache.Register(CacheKeyApiKeyPermission, 10*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.spotClient.NewGetAPIKeyPermission().Do(ctx)
	})
	c.cache.Register(CacheKeySpotAccountV3, 1*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.spotClient.NewGetAccountService().OmitZeroBalances(true).Do(ctx)
	})
	c.cache.Register(CacheKeyFundingAccount, 1*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.spotClient.NewGetFundingAssetService().Do(ctx)
	})
	c.cache.Register(CacheKeyPortfolioAccountPro, 1*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.portfolioProClient.NewGetAccountService().Do(ctx)
	})
	c.cache.Register(CacheKeyFutureFundingInfo, 1*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.futureClient.NewFundingRateInfoService().Do(ctx)
	})
	c.cache.Register(CacheKeyUMAccountConfig, 10*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.portfolioClient.NewGetUMAccountConfigService().Do(ctx)
	})
	c.cache.Register(CacheKeyFutureAccountV3, 1*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.futureClient.NewGetAccountV3Service().Do(ctx)
	})
	c.cache.Register(CacheKeyFutureAccountConfig, 10*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.futureClient.NewGetAccountConfigService().Do(ctx)
	})
	c.cache.Register(CacheKeyWalletBalance, 1*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.spotClient.NewWalletBalanceService().QuoteAsset("USDT").Do(ctx)
	})
	c.cache.Register(CacheKeyPortfolioBalance, 1*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.portfolioProClient.NewGetAccountBalanceService().Do(ctx)
	})
	c.cache.Register(CacheKeyPortfolioUmBalance, 1*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.portfolioClient.NewGetUMAccountDetailV2Service().Do(ctx)
	})
	c.cache.Register(CacheKeyFutureBalance, 1*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.futureClient.NewGetBalanceService().Do(ctx)
	})
	c.cache.Register(CacheKeyFutureSymbolConfig, 5*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.futureClient.NewGetSymbolConfigService().Do(ctx)
	})
	c.cache.Register(CacheKeyUMSymbolConfig, 5*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.portfolioClient.NewGetUMSymbolConfigService().Do(ctx)
	})
	c.cache.Register(CacheKeyUMLeverageBracket, 30*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.portfolioClient.NewGetUMLeverageBracketService().Do(ctx)
	})
	c.cache.Register(CacheKeyFutureLeverageBracket, 30*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.futureClient.NewGetLeverageBracketService().Do(ctx)
	})
	c.cache.Register(CacheKeyUMPositionRisk, 1*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.portfolioClient.NewGetUMPositionRiskService().Do(ctx)
	})
	c.cache.Register(CacheKeyFuturePositionRisk, 1*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.futureClient.NewGetPositionRiskV3Service().Do(ctx)
	})
	c.cache.Register(CacheKeyFuturePositionMode, 5*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.futureClient.NewGetPositionModeService().Do(ctx)
	})
	c.cache.Register(CacheKeyUMPositionMode, 5*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.portfolioClient.NewGetUMPositionModeService().Do(ctx)
	})
}

// setupPublicCache 只在公共缓存实例上注册「公共市场数据」相关的 key，
// 回调仍然复用当前 Connector 的 client，但不依赖账户状态。
func setupPublicCache(c *Connector, target *cache.Cache) {
	target.Register(CacheKeySpotExchangeInfoV3, 10*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.spotClient.NewExchangeInfoService().Do(ctx)
	})
	target.Register(CacheKeyFutureExchangeInfo, 10*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.futureClient.NewExchangeInfoService().Do(ctx)
	})
	target.Register(CacheKeySpotExchangeInfoV3, 10*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.spotClient.NewExchangeInfoService().Do(ctx)
	})
	target.Register(CacheKeyFutureExchangeInfo, 10*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		return c.futureClient.NewExchangeInfoService().Do(ctx)
	})
	target.Register(CacheKeySpotSymbolInfoV3, 10*time.Minute, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no symbol provided")
		}
		symbol, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid symbol: %T", params[0])
		}
		return c.spotClient.NewExchangeInfoService().Symbol(symbol).Do(ctx)
	})
	target.Register(CacheKeySpotPrices, 3*time.Second, func(ctx context.Context, params ...any) (any, error) {
		return c.spotClient.NewListPricesService().Do(ctx)
	})
	target.Register(CacheKeyFuturePrices, 3*time.Second, func(ctx context.Context, params ...any) (any, error) {
		return c.futureClient.NewListPricesService().Do(ctx)
	})
	target.Register(CacheKeySpotSymbolTicker, 5*time.Second, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no symbol provided")
		}
		symbol, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid symbol: %T", params[0])
		}
		return c.spotClient.NewListSymbolTickerService().Symbols([]string{symbol}).WindowSize("1d").Do(ctx)
	})
	target.Register(CacheKeySpotBookTicker, 1*time.Second, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no symbol provided")
		}
		symbol, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid symbol: %T", params[0])
		}
		return c.spotClient.NewListBookTickersService().Symbol(symbol).Do(ctx)
	})
	target.Register(CacheKeyFutureBookTicker, 1*time.Second, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no symbol provided")
		}
		symbol, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid symbol: %T", params[0])
		}
		return c.futureClient.NewListBookTickersService().Symbol(symbol).Do(ctx)
	})
	target.Register(CacheKeyFuturePriceChangeStats, 5*time.Second, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no symbol provided")
		}
		symbol, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid symbol: %T", params[0])
		}
		return c.futureClient.NewListPriceChangeStatsService().Symbol(symbol).Do(ctx)
	})
	target.Register(CacheKeySpotBookTickers, 3*time.Second, func(ctx context.Context, params ...any) (any, error) {
		return c.spotClient.NewListBookTickersService().Do(ctx)
	})
	target.Register(CacheKeyFutureBookTickers, 3*time.Second, func(ctx context.Context, params ...any) (any, error) {
		return c.futureClient.NewListBookTickersService().Do(ctx)
	})
	target.Register(CacheKeySpotTickerPrice, 2*time.Second, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no symbol provided")
		}
		symbol, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid symbol: %T", params[0])
		}
		return c.spotClient.NewListPricesService().Symbol(symbol).Do(ctx)
	})
	target.Register(CacheKeyFutureTickerPrice, 2*time.Second, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no symbol provided")
		}
		symbol, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid symbol: %T", params[0])
		}
		return c.futureClient.NewListPricesService().Symbol(symbol).Do(ctx)
	})
	target.Register(CacheKeyFutureMarkPrices, 5*time.Second, func(ctx context.Context, params ...any) (any, error) {
		return c.futureClient.NewPremiumIndexService().Do(ctx)
	})
	target.Register(CacheKeyFutureSymbolPremiumIndex, 10*time.Second, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no symbol provided")
		}
		symbol, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid symbol: %T", params[0])
		}
		return c.futureClient.NewPremiumIndexService().Symbol(symbol).Do(ctx)
	})
	target.Register(CacheKeyIndexComponent, 10*time.Second, func(ctx context.Context, params ...any) (any, error) {
		if len(params) == 0 {
			return nil, fmt.Errorf("no symbol provided")
		}
		symbol, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("invalid symbol: %T", params[0])
		}
		return c.futureClient.NewConstituentsService().Symbol(symbol).Do(ctx)
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
