package api

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/misc"
	"rogchap.com/v8go"
)

// ExchangeAPI Exchange对象API
type ExchangeAPI struct {
	getMarketsFn func(ctx context.Context, exchange ctypes.Exchange, marketType string) ([]map[string]any, error)
	getTickersFn func(ctx context.Context, exchange ctypes.Exchange, symbols []ctypes.Symbol, period time.Duration) (map[string]any, error)
}

// NewExchangeAPI 创建 ExchangeAPI
func NewExchangeAPI(
	getMarketsFn func(ctx context.Context, exchange ctypes.Exchange, marketType string) ([]map[string]any, error),
	getTickersFn func(ctx context.Context, exchange ctypes.Exchange, symbols []ctypes.Symbol, period time.Duration) (map[string]any, error),
) *ExchangeAPI {
	return &ExchangeAPI{
		getMarketsFn: getMarketsFn,
		getTickersFn: getTickersFn,
	}
}

// extractExchange 从 this 对象中提取 exchange
func extractExchange(info *v8go.FunctionCallbackInfo) (ctypes.Exchange, error) {
	thisObj := info.This()

	exchangeVal, err := thisObj.Get("exchange")
	if err != nil || exchangeVal == nil {
		return "", fmt.Errorf("exchange property not found")
	}

	exchangeStr := exchangeVal.String()

	exchange, err := ctypes.ParseExchange(exchangeStr)
	if err != nil {
		return "", fmt.Errorf("invalid exchange: %s", exchangeStr)
	}

	return exchange, nil
}

// GetMarkets JS函数：获取市场列表
func (e *ExchangeAPI) GetMarkets(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, err := extractExchange(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	marketType := "all"
	if len(args) > 0 && args[0].IsString() {
		marketType = args[0].String()
	}

	if e.getMarketsFn == nil {
		return throwError(ctx, "GetMarkets not implemented")
	}

	markets, err := e.getMarketsFn(context.Background(), exchange, marketType)
	if err != nil {
		log.Error().Err(err).Msg("failed to get markets")
		return throwError(ctx, fmt.Sprintf("failed to get markets: %v", err))
	}

	val, err := misc.AnyToV8Value(ctx, markets)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert markets: %v", err))
	}

	return val
}

// GetTickers JS函数：获取全市场tickers
func (e *ExchangeAPI) GetTickers(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, err := extractExchange(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	var period time.Duration
	if len(args) > 0 {
		period, err = parsePeriod(args[0])
		if err != nil {
			return throwError(ctx, err.Error())
		}
	}

	if e.getTickersFn == nil {
		return throwError(ctx, "GetTickers not implemented")
	}

	// 获取 symbols 列表（从 this.symbols 或全局 symbols）
	var symbols []ctypes.Symbol
	symbolsVal, err := info.This().Get("symbols")
	if err == nil && symbolsVal != nil && symbolsVal.IsArray() {
		// 从 ExchangeHandle 的 symbols 属性获取
		arr := symbolsVal.Object()
		length, _ := arr.Get("length")
		for i := 0; i < int(length.Int32()); i++ {
			item, _ := arr.GetIdx(uint32(i))
			symbolStr := item.String()
			sym, err := ctypes.ParseSymbol(symbolStr)
			if err == nil {
				symbols = append(symbols, sym)
			}
		}
	}

	tickers, err := e.getTickersFn(context.Background(), exchange, symbols, period)
	if err != nil {
		log.Error().Err(err).Msg("failed to get tickers")
		return throwError(ctx, fmt.Sprintf("failed to get tickers: %v", err))
	}

	val, err := misc.AnyToV8Value(ctx, tickers)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert tickers: %v", err))
	}

	return val
}

// GetExchange JS函数：获取交易所
func (e *ExchangeAPI) GetExchange(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()

	exchange, err := extractExchange(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	val, _ := v8go.NewValue(ctx.Isolate(), exchange.String())
	return val
}
