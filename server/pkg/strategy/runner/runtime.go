package runner

import (
	"fmt"

	"github.com/bytedance/sonic"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/misc"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/runner/api"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
	"rogchap.com/v8go"
)

// Runtime JS运行时API注入器
type Runtime struct {
	consoleAPI  *api.ConsoleAPI
	timeAPI     *api.TimeAPI
	symbolAPI   *api.SymbolAPI
	exchangeAPI *api.ExchangeAPI
	params      map[string]any

	// Storage 内存存储
	storage *Storage

	// 新增运行时上下文
	symbols []ctypes.ExSymbol
	runMode stypes.RunMode
}

// NewRuntime 创建运行时
func NewRuntime(
	consoleAPI *api.ConsoleAPI,
	timeAPI *api.TimeAPI,
) *Runtime {
	return &Runtime{
		consoleAPI: consoleAPI,
		timeAPI:    timeAPI,
	}
}

// WithParams 设置策略参数
func (r *Runtime) WithParams(params map[string]any) *Runtime {
	r.params = params
	return r
}

// WithSymbols 设置可交易标的列表
func (r *Runtime) WithSymbols(symbols []ctypes.ExSymbol) *Runtime {
	r.symbols = symbols
	return r
}

// WithMode 设置运行模式
func (r *Runtime) WithMode(mode stypes.RunMode) *Runtime {
	r.runMode = mode
	return r
}

// WithSymbolAPI 设置 SymbolAPI
func (r *Runtime) WithSymbolAPI(symbolAPI *api.SymbolAPI) *Runtime {
	r.symbolAPI = symbolAPI
	return r
}

// WithStorage 设置 Storage
func (r *Runtime) WithStorage(storage *Storage) *Runtime {
	r.storage = storage
	return r
}

// GetStorage 获取 Storage 实例
func (r *Runtime) GetStorage() *Storage {
	return r.storage
}

// WithExchangeAPI 设置 ExchangeAPI
func (r *Runtime) WithExchangeAPI(exchangeAPI *api.ExchangeAPI) *Runtime {
	r.exchangeAPI = exchangeAPI
	return r
}

// protectGlobalProperty 使用 Object.defineProperty 保护全局属性，防止被覆盖
func protectGlobalProperty(ctx *v8go.Context, name string) error {
	// 使用 Object.defineProperty 重新定义属性为不可写、不可配置
	// 由于属性已经通过 global.Set() 设置，默认是可配置的，所以可以重新定义
	code := fmt.Sprintf(`
		(function() {
			try {
				const val = this[%q];
				if (val === undefined) {
					return; // 属性不存在，跳过保护
				}
				Object.defineProperty(this, %q, {
					value: val,
					writable: false,
					configurable: false,
					enumerable: true
				});
			} catch (e) {
				// 如果属性已配置为不可配置，忽略错误（说明已经被保护了）
			}
		})();
	`, name, name)

	_, err := ctx.RunScript(code, fmt.Sprintf("protect_%s.js", name))
	if err != nil {
		// 保护失败不影响策略执行，只记录错误
		return fmt.Errorf("failed to protect global property %s: %w", name, err)
	}
	return nil
}

// Inject 注入运行时API到V8上下文
func (r *Runtime) Inject(ctx *v8go.Context) error {
	// 创建全局对象
	global := ctx.Global()

	// 注入params
	if r.params != nil {
		obj := v8go.NewObjectTemplate(ctx.Isolate())
		valObj, err := obj.NewInstance(ctx)
		if err != nil {
			return fmt.Errorf("failed to create params object: %w", err)
		}
		for k, v := range r.params {
			val, err := misc.AnyToV8Value(ctx, v)
			if err != nil {
				return fmt.Errorf("convert param %s: %w", k, err)
			}
			valObj.Set(k, val)
		}
		global.Set("params", valObj.Value)
		if err := protectGlobalProperty(ctx, "params"); err != nil {
			// 保护失败不影响策略执行，只记录错误
			// 可以在这里添加日志记录
		}
	}

	// 注入 console API
	if r.consoleAPI != nil {
		consoleObj := v8go.NewObjectTemplate(ctx.Isolate())
		consoleObj.Set("debug", v8go.NewFunctionTemplate(ctx.Isolate(), r.consoleAPI.Debug))
		consoleObj.Set("log", v8go.NewFunctionTemplate(ctx.Isolate(), r.consoleAPI.Log))
		consoleObj.Set("warn", v8go.NewFunctionTemplate(ctx.Isolate(), r.consoleAPI.Warn))
		consoleObj.Set("error", v8go.NewFunctionTemplate(ctx.Isolate(), r.consoleAPI.Error))
		consoleVal, err := consoleObj.NewInstance(ctx)
		if err != nil {
			return fmt.Errorf("failed to create console object: %w", err)
		}
		global.Set("console", consoleVal.Value)
		if err := protectGlobalProperty(ctx, "console"); err != nil {
			// 保护失败不影响策略执行，只记录错误
		}
	}

	// 注入 time API
	if r.timeAPI != nil {
		timeObj := v8go.NewObjectTemplate(ctx.Isolate())
		timeObj.Set("now", v8go.NewFunctionTemplate(ctx.Isolate(), r.timeAPI.Now))
		timeObj.Set("nowISO", v8go.NewFunctionTemplate(ctx.Isolate(), r.timeAPI.NowISO))
		timeVal, err := timeObj.NewInstance(ctx)
		if err != nil {
			return fmt.Errorf("failed to create time object: %w", err)
		}
		global.Set("time", timeVal.Value)
		if err := protectGlobalProperty(ctx, "time"); err != nil {
			// 保护失败不影响策略执行，只记录错误
		}
	}

	// 注入 symbols 数组（SymbolHandle 实例）
	if len(r.symbols) > 0 && r.symbolAPI != nil {
		if err := r.injectSymbols(ctx); err != nil {
			return fmt.Errorf("failed to inject symbols: %w", err)
		}
	}

	// 注入 _WithSymbol 底层工厂函数
	if r.symbolAPI != nil {
		if err := r.injectWithSymbol(ctx); err != nil {
			return fmt.Errorf("failed to inject _WithSymbol: %w", err)
		}
		// 注入 JS 层 WithSymbol wrapper（带缓存）
		if err := r.injectWithSymbolWrapper(ctx); err != nil {
			return fmt.Errorf("failed to inject WithSymbol wrapper: %w", err)
		}
	}

	// 注入 WithExchange 工厂函数
	if r.exchangeAPI != nil {
		if err := r.injectWithExchange(ctx); err != nil {
			return fmt.Errorf("failed to inject WithExchange: %w", err)
		}
	}

	// 注入 require()：仅允许加载系统内置三方库（如 decimal.js）
	if err := r.injectRequire(ctx); err != nil {
		return fmt.Errorf("failed to inject require: %w", err)
	}

	// 注入 storage 全局对象
	if r.storage != nil {
		if err := r.injectStorage(ctx); err != nil {
			return fmt.Errorf("failed to inject storage: %w", err)
		}
	}

	return nil
}

// injectSymbols 注入 symbols 数组
func (r *Runtime) injectSymbols(ctx *v8go.Context) error {
	global := ctx.Global()
	iso := ctx.Isolate()

	// 创建 symbols 数组
	arrTpl := v8go.NewObjectTemplate(iso)
	arr, err := arrTpl.NewInstance(ctx)
	if err != nil {
		return fmt.Errorf("failed to create symbols array: %w", err)
	}

	for i, exSymbol := range r.symbols {
		// 为每个 symbol 创建一个 SymbolHandle 对象
		symbolHandle, err := r.createSymbolHandle(ctx, exSymbol.Exchange, exSymbol.Symbol)
		if err != nil {
			return fmt.Errorf("failed to create symbol handle for %s: %w", exSymbol.String(), err)
		}
		if err := arr.SetIdx(uint32(i), symbolHandle); err != nil {
			return fmt.Errorf("failed to set symbol handle at index %d: %w", i, err)
		}
	}

	// 设置数组的 length 属性
	lengthVal, _ := v8go.NewValue(iso, int32(len(r.symbols)))
	arr.Set("length", lengthVal)

	global.Set("symbols", arr.Value)
	protectGlobalProperty(ctx, "symbols")

	return nil
}

// injectWithSymbol 注入 _WithSymbol 底层工厂函数
func (r *Runtime) injectWithSymbol(ctx *v8go.Context) error {
	global := ctx.Global()
	iso := ctx.Isolate()

	tpl := v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) *v8go.Value {
		args := info.Args()
		if len(args) < 2 {
			msg, _ := v8go.NewValue(iso, "_WithSymbol requires exchange and symbol arguments")
			iso.ThrowException(msg)
			return v8go.Undefined(iso)
		}

		exchangeStr := args[0].String()
		symbolStr := args[1].String()

		exchange, err := ctypes.ParseExchange(exchangeStr)
		if err != nil {
			msg, _ := v8go.NewValue(iso, fmt.Sprintf("invalid exchange: %s", exchangeStr))
			iso.ThrowException(msg)
			return v8go.Undefined(iso)
		}

		symbol, err := ctypes.ParseSymbol(symbolStr)
		if err != nil {
			msg, _ := v8go.NewValue(iso, fmt.Sprintf("invalid symbol: %s", symbolStr))
			iso.ThrowException(msg)
			return v8go.Undefined(iso)
		}

		symbolHandle, err := r.createSymbolHandle(ctx, exchange, symbol)
		if err != nil {
			msg, _ := v8go.NewValue(iso, fmt.Sprintf("failed to create symbol handle: %v", err))
			iso.ThrowException(msg)
			return v8go.Undefined(iso)
		}

		return symbolHandle
	})

	fn := tpl.GetFunction(ctx)
	global.Set("_WithSymbol", fn)
	protectGlobalProperty(ctx, "_WithSymbol")

	return nil
}

// injectWithSymbolWrapper 注入 JS 层 WithSymbol 缓存包装
func (r *Runtime) injectWithSymbolWrapper(ctx *v8go.Context) error {
	// JS 代码：定义 WithSymbol 为缓存包装，内部调用 _WithSymbol
	jsCode := `
(function() {
	var __symbolCache = Object.create(null);

	function WithSymbol(exchange, symbol) {
		var key = String(exchange) + "|" + String(symbol);
		if (__symbolCache[key]) {
			return __symbolCache[key];
		}
		var handle = _WithSymbol(exchange, symbol);
		__symbolCache[key] = handle;
		return handle;
	}

	// 定义为全局不可覆盖
	Object.defineProperty(this, "WithSymbol", {
		value: WithSymbol,
		writable: false,
		configurable: false,
		enumerable: true
	});
})();
`
	_, err := ctx.RunScript(jsCode, "withsymbol_wrapper.js")
	if err != nil {
		return fmt.Errorf("failed to run WithSymbol wrapper script: %w", err)
	}

	return nil
}

// injectWithExchange 注入 WithExchange 工厂函数
func (r *Runtime) injectWithExchange(ctx *v8go.Context) error {
	global := ctx.Global()
	iso := ctx.Isolate()

	tpl := v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) *v8go.Value {
		args := info.Args()
		if len(args) < 1 {
			msg, _ := v8go.NewValue(iso, "WithExchange requires exchange argument")
			iso.ThrowException(msg)
			return v8go.Undefined(iso)
		}

		exchangeStr := args[0].String()
		exchange, err := ctypes.ParseExchange(exchangeStr)
		if err != nil {
			msg, _ := v8go.NewValue(iso, fmt.Sprintf("invalid exchange: %s", exchangeStr))
			iso.ThrowException(msg)
			return v8go.Undefined(iso)
		}

		exchangeHandle, err := r.createExchangeHandle(ctx, exchange)
		if err != nil {
			msg, _ := v8go.NewValue(iso, fmt.Sprintf("failed to create exchange handle: %v", err))
			iso.ThrowException(msg)
			return v8go.Undefined(iso)
		}

		return exchangeHandle
	})

	fn := tpl.GetFunction(ctx)
	global.Set("WithExchange", fn)
	protectGlobalProperty(ctx, "WithExchange")

	return nil
}

// createSymbolHandle 创建 SymbolHandle 对象
func (r *Runtime) createSymbolHandle(ctx *v8go.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (*v8go.Value, error) {
	iso := ctx.Isolate()

	// 创建对象并设置属性
	objTpl := v8go.NewObjectTemplate(iso)
	obj, err := objTpl.NewInstance(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create symbol handle object: %w", err)
	}

	// 设置只读属性 exchange 和 symbol
	exchangeVal, _ := v8go.NewValue(iso, exchange.String())
	if err := obj.Set("exchange", exchangeVal); err != nil {
		return nil, fmt.Errorf("failed to set exchange: %w", err)
	}

	symbolVal, _ := v8go.NewValue(iso, symbol.String())
	if err := obj.Set("symbol", symbolVal); err != nil {
		return nil, fmt.Errorf("failed to set symbol: %w", err)
	}
	if err := obj.Set("base", symbol.Base); err != nil {
		return nil, fmt.Errorf("failed to set base: %w", err)
	}
	if err := obj.Set("quote", symbol.Quote); err != nil {
		return nil, fmt.Errorf("failed to set quote: %w", err)
	}
	// 注意：symbol.Type 是自定义类型（底层 string），这里显式转成 string，避免 v8go 反射转换失败导致 JS 侧拿不到。
	if err := obj.Set("type", string(symbol.Type)); err != nil {
		return nil, fmt.Errorf("failed to set type: %w", err)
	}

	// 绑定所有方法
	if r.symbolAPI != nil {
		// 创建新的 SymbolAPI 实例并设置 exchange 和 symbol
		symbolAPI := r.symbolAPI.WithExchangeSymbol(exchange.String(), symbol.String())

		obj.Set("Set", v8go.NewFunctionTemplate(iso, symbolAPI.Set).GetFunction(ctx))
		obj.Set("Get", v8go.NewFunctionTemplate(iso, symbolAPI.Get).GetFunction(ctx))
		obj.Set("Delete", v8go.NewFunctionTemplate(iso, symbolAPI.Delete).GetFunction(ctx))
		obj.Set("GetTicker", v8go.NewFunctionTemplate(iso, r.symbolAPI.GetTicker).GetFunction(ctx))
		obj.Set("GetDepth", v8go.NewFunctionTemplate(iso, r.symbolAPI.GetDepth).GetFunction(ctx))
		obj.Set("GetTrades", v8go.NewFunctionTemplate(iso, r.symbolAPI.GetTrades).GetFunction(ctx))
		obj.Set("GetKlines", v8go.NewFunctionTemplate(iso, r.symbolAPI.GetKlines).GetFunction(ctx))
		obj.Set("Buy", v8go.NewFunctionTemplate(iso, r.symbolAPI.Buy).GetFunction(ctx))
		obj.Set("Sell", v8go.NewFunctionTemplate(iso, r.symbolAPI.Sell).GetFunction(ctx))
		obj.Set("CancelOrder", v8go.NewFunctionTemplate(iso, r.symbolAPI.CancelOrder).GetFunction(ctx))
		obj.Set("GetOrders", v8go.NewFunctionTemplate(iso, r.symbolAPI.GetOrders).GetFunction(ctx))
		obj.Set("GetOrder", v8go.NewFunctionTemplate(iso, r.symbolAPI.GetOrder).GetFunction(ctx))
		obj.Set("GetFills", v8go.NewFunctionTemplate(iso, r.symbolAPI.GetFills).GetFunction(ctx))
		obj.Set("GetPositions", v8go.NewFunctionTemplate(iso, r.symbolAPI.GetPositions).GetFunction(ctx))
		obj.Set("GetLeverage", v8go.NewFunctionTemplate(iso, r.symbolAPI.GetLeverage).GetFunction(ctx))
		obj.Set("SetLeverage", v8go.NewFunctionTemplate(iso, r.symbolAPI.SetLeverage).GetFunction(ctx))
		obj.Set("GetFundings", v8go.NewFunctionTemplate(iso, r.symbolAPI.GetFundings).GetFunction(ctx))
		obj.Set("GetAccount", v8go.NewFunctionTemplate(iso, r.symbolAPI.GetAccount).GetFunction(ctx))
		obj.Set("GetAsset", v8go.NewFunctionTemplate(iso, r.symbolAPI.GetAsset).GetFunction(ctx))
	}

	return obj.Value, nil
}

// createExchangeHandle 创建 ExchangeHandle 对象
func (r *Runtime) createExchangeHandle(ctx *v8go.Context, exchange ctypes.Exchange) (*v8go.Value, error) {
	iso := ctx.Isolate()

	// 创建对象并设置属性
	objTpl := v8go.NewObjectTemplate(iso)
	obj, err := objTpl.NewInstance(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create exchange handle object: %w", err)
	}

	// 设置只读属性 exchange
	exchangeVal, _ := v8go.NewValue(iso, exchange.String())
	obj.Set("exchange", exchangeVal)

	// 设置 symbols 属性（过滤出属于该交易所的 symbols）
	var filteredSymbols []string
	for _, exSymbol := range r.symbols {
		if exSymbol.Exchange == exchange {
			filteredSymbols = append(filteredSymbols, exSymbol.Symbol.String())
		}
	}
	symbolsVal, _ := misc.AnyToV8Value(ctx, filteredSymbols)
	obj.Set("symbols", symbolsVal)

	// 绑定所有方法
	if r.exchangeAPI != nil {
		obj.Set("GetMarkets", v8go.NewFunctionTemplate(iso, r.exchangeAPI.GetMarkets).GetFunction(ctx))
		obj.Set("GetTickers", v8go.NewFunctionTemplate(iso, r.exchangeAPI.GetTickers).GetFunction(ctx))
		obj.Set("GetExchange", v8go.NewFunctionTemplate(iso, r.exchangeAPI.GetExchange).GetFunction(ctx))
	}

	return obj.Value, nil
}

// injectStorage 注入 storage 全局对象
func (r *Runtime) injectStorage(ctx *v8go.Context) error {
	global := ctx.Global()
	iso := ctx.Isolate()

	storageObj := v8go.NewObjectTemplate(iso)

	// storage.get(key)
	storageObj.Set("get", v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) *v8go.Value {
		args := info.Args()
		if len(args) < 1 {
			msg, _ := v8go.NewValue(iso, "storage.get(key) requires 1 argument")
			iso.ThrowException(msg)
			return v8go.Undefined(iso)
		}

		key := "global:" + args[0].String()
		value, _, ok := r.storage.Get(key)
		if !ok {
			return v8go.Undefined(iso)
		}

		var result any
		if err := sonic.Unmarshal(value, &result); err != nil {
			return v8go.Undefined(iso)
		}

		v, _ := misc.AnyToV8Value(ctx, result)
		return v
	}))

	// storage.set(key, value, ttl?)
	storageObj.Set("set", v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) *v8go.Value {
		args := info.Args()
		if len(args) < 2 {
			msg, _ := v8go.NewValue(iso, "storage.set(key, value) requires 2 arguments")
			iso.ThrowException(msg)
			return v8go.Undefined(iso)
		}

		key := "global:" + args[0].String()
		value := args[1]

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

		r.storage.Set(key, valueBytes, ttlMs)

		ret, _ := v8go.NewValue(iso, true)
		return ret
	}))

	// storage.delete(key)
	storageObj.Set("delete", v8go.NewFunctionTemplate(iso, func(info *v8go.FunctionCallbackInfo) *v8go.Value {
		args := info.Args()
		if len(args) < 1 {
			msg, _ := v8go.NewValue(iso, "storage.delete(key) requires 1 argument")
			iso.ThrowException(msg)
			return v8go.Undefined(iso)
		}

		key := "global:" + args[0].String()
		r.storage.Delete(key)

		ret, _ := v8go.NewValue(iso, true)
		return ret
	}))

	storageVal, err := storageObj.NewInstance(ctx)
	if err != nil {
		return fmt.Errorf("failed to create storage object: %w", err)
	}

	global.Set("storage", storageVal.Value)

	return nil
}
