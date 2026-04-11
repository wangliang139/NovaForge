package api

import (
	"context"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/rs/zerolog/log"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/misc"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"rogchap.com/v8go"
)

// SymbolAPI Symbol对象API
type SymbolAPI struct {
	// 行情数据获取函数
	getTickerFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, period time.Duration) (map[string]any, error)
	getDepthFn  func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, depth int) (map[string]any, error)
	getTradesFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, period time.Duration) ([]map[string]any, error)
	getKlinesFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, interval ctypes.Interval, limit int, start, end *time.Time) ([]map[string]any, error)

	// 交易函数
	buyFn    func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, opts map[string]any) (map[string]any, error)
	sellFn   func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, opts map[string]any) (map[string]any, error)
	cancelFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, orderId string) error

	// 账户/订单查询函数
	getOrdersFn    func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) ([]map[string]any, error)
	getOrderFn     func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, orderId string) (map[string]any, error)
	getFillsFn     func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, period time.Duration) ([]map[string]any, error)
	getPositionsFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, side *ctypes.PositionSide) ([]map[string]any, error)
	getLeverageFn  func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (int, error)
	setLeverageFn  func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, leverage int) error
	getFundingsFn  func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, period time.Duration) ([]map[string]any, error)
	getAccountFn   func(ctx context.Context, exchange ctypes.Exchange) (map[string]any, error)
	getAssetFn     func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, asset string) (*ctypes.AssetBo, error)

	// variables
	setFn    func(key string, value []byte, ttlMs ...int64)
	getFn    func(key string) (value []byte, expiresAt int64, ok bool)
	deleteFn func(key string) bool

	// exchange 和 symbol，用于构建 key 前缀
	exchange string
	symbol   string
}

// NewSymbolAPI 创建 SymbolAPI
func NewSymbolAPI(
	getTickerFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, period time.Duration) (map[string]any, error),
	getDepthFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, depth int) (map[string]any, error),
	getTradesFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, period time.Duration) ([]map[string]any, error),
	getKlinesFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, interval ctypes.Interval, limit int, start, end *time.Time) ([]map[string]any, error),
	buyFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, opts map[string]any) (map[string]any, error),
	sellFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, opts map[string]any) (map[string]any, error),
	cancelFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, orderId string) error,
	getOrdersFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) ([]map[string]any, error),
	getOrderFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, orderId string) (map[string]any, error),
	getFillsFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, period time.Duration) ([]map[string]any, error),
	getPositionsFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, side *ctypes.PositionSide) ([]map[string]any, error),
	getLeverageFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (int, error),
	setLeverageFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, leverage int) error,
	getFundingsFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, period time.Duration) ([]map[string]any, error),
	getAccountFn func(ctx context.Context, exchange ctypes.Exchange) (map[string]any, error),
	getAssetFn func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, asset string) (*ctypes.AssetBo, error),
	setFn func(key string, value []byte, ttlMs ...int64),
	getFn func(key string) (value []byte, expiresAt int64, ok bool),
	deleteFn func(key string) bool,
) *SymbolAPI {
	return &SymbolAPI{
		getTickerFn:    getTickerFn,
		getDepthFn:     getDepthFn,
		getTradesFn:    getTradesFn,
		getKlinesFn:    getKlinesFn,
		buyFn:          buyFn,
		sellFn:         sellFn,
		cancelFn:       cancelFn,
		getOrdersFn:    getOrdersFn,
		getOrderFn:     getOrderFn,
		getFillsFn:     getFillsFn,
		getPositionsFn: getPositionsFn,
		getLeverageFn:  getLeverageFn,
		setLeverageFn:  setLeverageFn,
		getFundingsFn:  getFundingsFn,
		getAccountFn:   getAccountFn,
		getAssetFn:     getAssetFn,
		setFn:          setFn,
		getFn:          getFn,
		deleteFn:       deleteFn,
	}
}

// WithExchangeSymbol 设置 exchange 和 symbol
func (s *SymbolAPI) WithExchangeSymbol(exchange, symbol string) *SymbolAPI {
	s.exchange = exchange
	s.symbol = symbol
	return s
}

// buildSymbolKey 构建带前缀的 key
func buildSymbolKey(exchange ctypes.Exchange, symbol ctypes.Symbol, key string) string {
	return fmt.Sprintf("symbol:%s:%s:%s", exchange.String(), symbol.String(), key)
}

// parsePeriod 解析 period 参数（支持字符串 "5m" 或数字毫秒）
func parsePeriod(value *v8go.Value) (time.Duration, error) {
	if value == nil {
		return 0, fmt.Errorf("period is required")
	}

	if value.IsString() {
		// 字符串格式："5m", "1h" 等
		str := value.String()
		d, err := time.ParseDuration(str)
		if err != nil {
			return 0, fmt.Errorf("invalid period string: %s", str)
		}
		return d, nil
	} else if value.IsNumber() {
		// 数字格式：毫秒
		ms := value.Int32()
		return time.Duration(ms) * time.Millisecond, nil
	}

	return 0, fmt.Errorf("period must be string or number")
}

// extractExchangeSymbol 从 this 对象中提取 exchange 和 symbol
func extractExchangeSymbol(info *v8go.FunctionCallbackInfo) (ctypes.Exchange, ctypes.Symbol, error) {
	thisObj := info.This()

	exchangeVal, err := thisObj.Get("exchange")
	if err != nil || exchangeVal == nil {
		return "", ctypes.Symbol{}, fmt.Errorf("exchange property not found")
	}

	symbolVal, err := thisObj.Get("symbol")
	if err != nil || symbolVal == nil {
		return "", ctypes.Symbol{}, fmt.Errorf("symbol property not found")
	}

	exchangeStr := exchangeVal.String()
	symbolStr := symbolVal.String()

	exchange, err := ctypes.ParseExchange(exchangeStr)
	if err != nil {
		return "", ctypes.Symbol{}, fmt.Errorf("invalid exchange: %s", exchangeStr)
	}

	symbol, err := ctypes.ParseSymbol(symbolStr)
	if err != nil {
		return "", ctypes.Symbol{}, fmt.Errorf("invalid symbol: %s", symbolStr)
	}

	return exchange, symbol, nil
}

// throwError 在 V8 中抛出异常
func throwError(ctx *v8go.Context, message string) *v8go.Value {
	iso := ctx.Isolate()
	val, _ := v8go.NewValue(iso, message)
	iso.ThrowException(val)
	return v8go.Undefined(iso)
}

// Set JS函数：缓存自定义数据（绑定到该 SymbolHandle 实例上）
// 用法：sym.Set("foo", 123)
func (s *SymbolAPI) Set(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	iso := ctx.Isolate()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	if len(args) < 2 {
		return throwError(ctx, "symbol.Set(key, value) requires 2 arguments")
	}

	key := args[0].String()
	if len(key) == 0 {
		return throwError(ctx, "symbol.Set(key, value) key cannot be empty")
	}

	fullKey := buildSymbolKey(exchange, symbol, key)
	value := args[1]

	if len(args) < 2 {
		msg, _ := v8go.NewValue(iso, "storage.set(key, value) requires 2 arguments")
		iso.ThrowException(msg)
		return v8go.Undefined(iso)
	}

	valueBytes, err := sonic.Marshal(value)
	if err != nil {
		msg, _ := v8go.NewValue(iso, "failed to serialize value")
		iso.ThrowException(msg)
		return v8go.Undefined(iso)
	}

	var ttlMs int64
	if len(args) >= 3 && args[2].IsNumber() {
		ttlMs = int64(args[2].Int32())
	}

	s.setFn(fullKey, valueBytes, ttlMs)

	ret, _ := v8go.NewValue(iso, true)
	return ret
}

// Get JS函数：读取自定义缓存数据（绑定到该 SymbolHandle 实例上）
// 用法：const v = sym.Get("foo")
func (s *SymbolAPI) Get(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	iso := ctx.Isolate()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	if len(args) < 1 {
		return throwError(ctx, "Get(key) requires 1 argument")
	}

	key := args[0].String()
	if len(key) == 0 {
		return throwError(ctx, "Get(key) key cannot be empty")
	}

	fullKey := buildSymbolKey(exchange, symbol, key)

	value, _, ok := s.getFn(fullKey)
	if !ok {
		return v8go.Undefined(iso)
	}

	var result any
	if err := sonic.Unmarshal(value, &result); err != nil {
		return v8go.Undefined(iso)
	}

	v, _ := misc.AnyToV8Value(ctx, result)

	return v
}

func (s *SymbolAPI) Delete(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	if len(args) < 1 {
		return throwError(ctx, "delete(key) requires 1 argument")
	}

	key := args[0].String()
	fullKey := buildSymbolKey(exchange, symbol, key)

	s.deleteFn(fullKey)

	ret, _ := v8go.NewValue(ctx.Isolate(), true)
	return ret
}

// GetTicker JS函数：获取ticker
func (s *SymbolAPI) GetTicker(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
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

	if s.getTickerFn == nil {
		return throwError(ctx, "GetTicker not implemented")
	}

	ticker, err := s.getTickerFn(context.Background(), exchange, symbol, period)
	if err != nil {
		log.Error().Err(err).Msg("failed to get ticker")
		return throwError(ctx, fmt.Sprintf("failed to get ticker: %v", err))
	}

	if ticker == nil {
		return v8go.Null(ctx.Isolate())
	}

	val, err := misc.AnyToV8Value(ctx, ticker)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert ticker: %v", err))
	}

	return val
}

// GetDepth JS函数：获取订单簿
func (s *SymbolAPI) GetDepth(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	depth := 20 // 默认深度
	if len(args) > 0 && args[0].IsNumber() {
		depth = int(args[0].Int32())
	}

	if s.getDepthFn == nil {
		return throwError(ctx, "GetDepth not implemented")
	}

	orderBook, err := s.getDepthFn(context.Background(), exchange, symbol, depth)
	if err != nil {
		log.Error().Err(err).Msg("failed to get depth")
		return throwError(ctx, fmt.Sprintf("failed to get depth: %v", err))
	}

	if orderBook == nil {
		return v8go.Null(ctx.Isolate())
	}

	val, err := misc.AnyToV8Value(ctx, orderBook)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert depth: %v", err))
	}

	return val
}

// GetTrades JS函数：获取成交记录
func (s *SymbolAPI) GetTrades(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
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

	if s.getTradesFn == nil {
		return throwError(ctx, "GetTrades not implemented")
	}

	trades, err := s.getTradesFn(context.Background(), exchange, symbol, period)
	if err != nil {
		log.Error().Err(err).Msg("failed to get trades")
		return throwError(ctx, fmt.Sprintf("failed to get trades: %v", err))
	}

	val, err := misc.AnyToV8Value(ctx, trades)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert trades: %v", err))
	}

	return val
}

// GetKlines JS函数：获取K线
func (s *SymbolAPI) GetKlines(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	if len(args) < 1 {
		return throwError(ctx, "interval is required")
	}

	intervalStr := args[0].String()
	interval := ctypes.Interval(intervalStr)
	if !interval.Valid() {
		return throwError(ctx, fmt.Sprintf("invalid interval: %s", intervalStr))
	}

	limit := 100
	if len(args) > 1 && args[1].IsNumber() {
		limit = int(args[1].Int32())
	}

	var start, end *time.Time
	if len(args) > 2 && args[2].IsNumber() {
		t := time.UnixMilli(args[2].Integer())
		start = &t
	}
	if len(args) > 3 && args[3].IsNumber() {
		t := time.UnixMilli(args[3].Integer())
		end = &t
	}

	if s.getKlinesFn == nil {
		return throwError(ctx, "GetKlines not implemented")
	}

	klines, err := s.getKlinesFn(context.Background(), exchange, symbol, interval, limit, start, end)
	if err != nil {
		log.Error().Err(err).Msg("failed to get klines")
		return throwError(ctx, fmt.Sprintf("failed to get klines: %v", err))
	}

	val, err := misc.AnyToV8Value(ctx, klines)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert klines: %v", err))
	}

	return val
}

// Buy JS函数：买入
func (s *SymbolAPI) Buy(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	if len(args) < 1 || !args[0].IsObject() {
		return throwError(ctx, "opts object is required")
	}

	opts, err := misc.V8ValueToMap(ctx, args[0])
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to parse opts: %v", err))
	}

	if s.buyFn == nil {
		return throwError(ctx, "Buy not implemented")
	}

	result, err := s.buyFn(context.Background(), exchange, symbol, opts)
	if err != nil {
		log.Error().Err(err).Msg("failed to buy")
		return throwError(ctx, fmt.Sprintf("failed to buy: %v", err))
	}

	val, err := misc.AnyToV8Value(ctx, result)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert result: %v", err))
	}

	return val
}

// Sell JS函数：卖出
func (s *SymbolAPI) Sell(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	if len(args) < 1 || !args[0].IsObject() {
		return throwError(ctx, "opts object is required")
	}

	opts, err := misc.V8ValueToMap(ctx, args[0])
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to parse opts: %v", err))
	}

	if s.sellFn == nil {
		return throwError(ctx, "Sell not implemented")
	}

	result, err := s.sellFn(context.Background(), exchange, symbol, opts)
	if err != nil {
		log.Error().Err(err).Msg("failed to sell")
		return throwError(ctx, fmt.Sprintf("failed to sell: %v", err))
	}

	val, err := misc.AnyToV8Value(ctx, result)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert result: %v", err))
	}

	return val
}

// CancelOrder JS函数：撤单
func (s *SymbolAPI) CancelOrder(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	if len(args) < 1 {
		return throwError(ctx, "orderId is required")
	}

	orderId := args[0].String()

	if s.cancelFn == nil {
		return throwError(ctx, "CancelOrder not implemented")
	}

	err = s.cancelFn(context.Background(), exchange, symbol, orderId)
	if err != nil {
		log.Error().Err(err).Msg("failed to cancel order")
		return throwError(ctx, fmt.Sprintf("failed to cancel order: %v", err))
	}

	val, _ := v8go.NewValue(ctx.Isolate(), true)
	return val
}

// GetOrders JS函数：获取订单列表
func (s *SymbolAPI) GetOrders(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()

	exchange, symbol, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	if s.getOrdersFn == nil {
		return throwError(ctx, "GetOrders not implemented")
	}

	orders, err := s.getOrdersFn(context.Background(), exchange, symbol)
	if err != nil {
		log.Error().Err(err).Msg("failed to get orders")
		return throwError(ctx, fmt.Sprintf("failed to get orders: %v", err))
	}

	val, err := misc.AnyToV8Value(ctx, orders)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert orders: %v", err))
	}

	return val
}

// GetOrder JS函数：获取单个订单
func (s *SymbolAPI) GetOrder(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	if len(args) < 1 {
		return throwError(ctx, "orderId is required")
	}

	orderId := args[0].String()

	if s.getOrderFn == nil {
		return throwError(ctx, "GetOrder not implemented")
	}

	order, err := s.getOrderFn(context.Background(), exchange, symbol, orderId)
	if err != nil {
		log.Error().Err(err).Msg("failed to get order")
		return throwError(ctx, fmt.Sprintf("failed to get order: %v", err))
	}

	if order == nil {
		return v8go.Null(ctx.Isolate())
	}

	val, err := misc.AnyToV8Value(ctx, order)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert order: %v", err))
	}

	return val
}

// GetFills JS函数：获取成交记录
func (s *SymbolAPI) GetFills(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
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

	if s.getFillsFn == nil {
		return throwError(ctx, "GetFills not implemented")
	}

	fills, err := s.getFillsFn(context.Background(), exchange, symbol, period)
	if err != nil {
		log.Error().Err(err).Msg("failed to get fills")
		return throwError(ctx, fmt.Sprintf("failed to get fills: %v", err))
	}

	val, err := misc.AnyToV8Value(ctx, fills)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert fills: %v", err))
	}

	return val
}

// GetPositions JS函数：获取仓位
func (s *SymbolAPI) GetPositions(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	var side *ctypes.PositionSide
	if len(args) > 0 && args[0].IsString() {
		sideStr := args[0].String()
		s := ctypes.ParsePositionSide(sideStr)
		side = &s
	}

	if s.getPositionsFn == nil {
		return throwError(ctx, "GetPositions not implemented")
	}

	positions, err := s.getPositionsFn(context.Background(), exchange, symbol, side)
	if err != nil {
		log.Error().Err(err).Msg("failed to get positions")
		return throwError(ctx, fmt.Sprintf("failed to get positions: %v", err))
	}

	val, err := misc.AnyToV8Value(ctx, positions)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert positions: %v", err))
	}

	return val
}

// GetLeverage JS函数：获取杠杆
func (s *SymbolAPI) GetLeverage(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()

	exchange, symbol, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	if s.getLeverageFn == nil {
		return throwError(ctx, "GetLeverage not implemented")
	}

	leverage, err := s.getLeverageFn(context.Background(), exchange, symbol)
	if err != nil {
		log.Error().Err(err).Msg("failed to get leverage")
		return throwError(ctx, fmt.Sprintf("failed to get leverage: %v", err))
	}

	val, _ := v8go.NewValue(ctx.Isolate(), int32(leverage))
	return val
}

// SetLeverage JS函数：设置杠杆
func (s *SymbolAPI) SetLeverage(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	leverage := int(args[0].Int32())

	if s.setLeverageFn == nil {
		return throwError(ctx, "SetLeverage not implemented")
	}

	err = s.setLeverageFn(context.Background(), exchange, symbol, leverage)
	if err != nil {
		log.Error().Err(err).Msg("failed to set leverage")
		return throwError(ctx, fmt.Sprintf("failed to set leverage: %v", err))
	}

	val, _ := v8go.NewValue(ctx.Isolate(), true)
	return val
}

// GetFundings JS函数：获取资金费率
func (s *SymbolAPI) GetFundings(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
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

	if s.getFundingsFn == nil {
		return throwError(ctx, "GetFundings not implemented")
	}

	fundings, err := s.getFundingsFn(context.Background(), exchange, symbol, period)
	if err != nil {
		log.Error().Err(err).Msg("failed to get fundings")
		return throwError(ctx, fmt.Sprintf("failed to get fundings: %v", err))
	}

	val, err := misc.AnyToV8Value(ctx, fundings)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert fundings: %v", err))
	}

	return val
}

// GetAccount JS函数：获取账户信息
func (s *SymbolAPI) GetAccount(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()

	exchange, _, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	if s.getAccountFn == nil {
		return throwError(ctx, "GetAccount not implemented")
	}

	account, err := s.getAccountFn(context.Background(), exchange)
	if err != nil {
		log.Error().Err(err).Msg("failed to get account")
		return throwError(ctx, fmt.Sprintf("failed to get account: %v", err))
	}

	if account == nil {
		return v8go.Null(ctx.Isolate())
	}

	val, err := misc.AnyToV8Value(ctx, account)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert account: %v", err))
	}

	return val
}

// GetAsset JS函数：获取资产信息
func (s *SymbolAPI) GetAsset(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	args := info.Args()

	exchange, symbol, err := extractExchangeSymbol(info)
	if err != nil {
		return throwError(ctx, err.Error())
	}

	var asset string
	if len(args) > 0 && args[0].IsString() {
		asset = args[0].String()
	}

	if s.getAssetFn == nil {
		return throwError(ctx, "GetAsset not implemented")
	}

	resp, err := s.getAssetFn(context.Background(), exchange, symbol, asset)
	if err != nil {
		log.Error().Err(err).Msg("failed to get asset")
		return throwError(ctx, fmt.Sprintf("failed to get asset: %v", err))
	}

	val, err := misc.AnyToV8Value(ctx, resp)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert asset: %v", err))
	}

	return val
}
