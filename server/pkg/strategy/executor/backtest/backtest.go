package backtest

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/precision"
	"github.com/wangliang139/NovaForge/server/pkg/strategy"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/exchange"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/executor/backtest/account"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/executor/backtest/builder"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/executor/backtest/collectors"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/executor/backtest/order"
	mb "github.com/wangliang139/NovaForge/server/pkg/strategy/infra/bus"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/clock"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/logging"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/logging/store"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/timeline"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/marketdata"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/portfolio"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/risk"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/runner"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/runner/api"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/runner/api/facade"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/symbolaccount"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
)

type BacktestExecutorOption func(*backtestExecutorOptions)

type backtestExecutorOptions struct {
	consoleLogMaxCache int
}

func WithConsoleLogMaxCache(maxCache int) BacktestExecutorOption {
	return func(o *backtestExecutorOptions) {
		o.consoleLogMaxCache = maxCache
	}
}

// BacktestExecutor 回测执行器
type BacktestExecutor struct {
	ctx    context.Context
	cancel context.CancelFunc

	status stypes.ExecutorStatus

	done   chan struct{}
	errMu  sync.RWMutex
	runErr error

	btCtx  stypes.BacktestContext
	config stypes.BacktestConfig

	jsRunner *runner.V8Engine

	clock      *clock.BacktestClock        // 回测时钟
	bus        *mb.TimelineEventBus        // 内部事件总线
	tScheduler *timeline.TimelineScheduler // 回测事件编排器

	marketProvider   *marketdata.GlobalMarketProvider // 市场数据提供器（全局视角）
	exchangeGateway  *exchange.BacktestGateway        // 交易所网关（交易所视角）
	orderManager     strategy.OrderEngine             // 订单引擎（账户视角）
	accountManager   strategy.AccountEngine           // 账户管理器（账户视角）
	portfolio        *portfolio.Portfolio             // 投资组合（策略视角）
	symbolAccountMgr *symbolaccount.Manager           // 策略级交易对账户管理器（用于资金隔离）

	// Collectors 和 ResultBuilder
	collectors    *collectors.Collectors
	resultBuilder *builder.ResultBuilder

	runAt time.Time
	endAt time.Time
}

func accountIDProvider(exchange ctypes.Exchange, symbol ctypes.Symbol) *string {
	return lo.ToPtr(exchange.String())
}

// NewBacktestExecutor 创建回测执行器
func NewBacktestExecutor(
	strategy *stypes.Strategy,
	btCtx stypes.BacktestContext,
	config stypes.BacktestConfig,
	opts ...BacktestExecutorOption,
) (*BacktestExecutor, error) {
	if strategy == nil {
		return nil, fmt.Errorf("strategy is required")
	}

	cfg := backtestExecutorOptions{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&cfg)
	}

	if config.StartTime.IsZero() || config.EndTime.IsZero() || config.StartTime.After(config.EndTime) {
		return nil, fmt.Errorf("backtest start time/end time is invalid")
	}

	// 设置默认 BaseCurrency
	if config.BaseCurrency == "" {
		config.BaseCurrency = "USDT"
	}
	if config.BaseExchange == "" {
		config.BaseExchange = ctypes.ExchangeBinance
	}

	clock := clock.NewBacktestClock(config.StartTime)

	// 创建 InternalQueue
	internalQ := timeline.NewInternalQueue()

	// market datasource -> timeline scheduler（external merger + internal queue）
	sorterCfg := timeline.DefaultSorterConfig()
	scheduler := timeline.NewTimelineScheduler(timeline.SchedulerConfig{
		External: timeline.NewExternalMerger(config.Sources, timeline.ExternalMergerConfig{
			Sort:   sorterCfg,
			Policy: timeline.ErrorPolicyFailFast,
		}),
		Internal: internalQ,
		Sorter:   sorterCfg,
	})

	// 创建 TimelineEventBus
	eventBus := mb.NewTimeline(scheduler)
	if err := eventBus.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start timeline event bus: %w", err)
	}

	// 创建市场数据提供器
	marketProvider := marketdata.NewGlobalMarketProvider(config.BaseExchange, config.BaseCurrency)

	// 创建 Portfolio
	portfolio := portfolio.NewPortfolio(eventBus, marketProvider)

	// 创建策略级交易对账户管理器（用于资金隔离）
	symbolAccountMgr := symbolaccount.NewManager()
	symbolAccountMgr.Subscribe(eventBus)

	// 创建账户管理器（支持多账户管理）
	accountManager, err := account.NewAccountManager(eventBus, clock)
	if err != nil {
		return nil, fmt.Errorf("failed to create account manager: %w", err)
	}

	// 为每个交易所创建 Account
	for _, sym := range config.Symbols {
		accountID := sym.Exchange.String()
		acctConfig := account.AccountConfig{
			Exchange: sym.Exchange,
		}
		accountManager.CreateAccount(accountID, acctConfig)
	}

	// 构建允许的交易对 map（用于快速查找）
	allowedSymbols := make([]ctypes.ExSymbolKey, 0, len(config.Symbols))
	for _, sym := range config.Symbols {
		exSymbol := ctypes.NewExSymbol(sym.Exchange, sym.Symbol)
		allowedSymbols = append(allowedSymbols, exSymbol.Key())
	}

	// 创建风险控制器（使用默认配置，可以从 config.Params 中读取配置）
	// TODO: 从 config.Params 中读取风险控制配置
	riskController := risk.NewRiskController(risk.DefaultConfig(), portfolio, marketProvider)

	// 创建交易所网关
	exGateway, err := exchange.NewExchangeGateway(eventBus, clock, accountManager, marketProvider, config.BaseCurrency, config.BaseExchange)
	if err != nil {
		return nil, fmt.Errorf("failed to create exchange gateway: %w", err)
	}

	// 创建订单管理器（需要持有 matchingEngine 以同步下单/撤单）
	orderManager, err := order.NewOrderEngineManager(
		order.Config{
			AllowedSymbols:            allowedSymbols,
			MarketOrderFreezeFactor:   precision.BacktestDefaultMarketOrderFreezeFactor,
			TakerCommissionRate:       decimal.RequireFromString("0.001"),
			MakerCommissionRate:       decimal.RequireFromString("0.001"),
			FutureTakerCommissionRate: decimal.RequireFromString("0.0005"),
			FutureMakerCommissionRate: decimal.RequireFromString("0.0005"),
		},
		clock,
		eventBus,
		accountManager,
		marketProvider,
		exGateway,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create order manager: %w", err)
	}

	// 日志记录器
	baseLogger := logging.NewZeroLogger(logging.WithModule("backtest"))
	logStorage := store.NewBufferStorage(cfg.consoleLogMaxCache, clock.Now)
	bufferLogger := logging.NewSinkLogger(logStorage, clock.Now)
	logRecorder := logging.NewCombinedLogger(bufferLogger, baseLogger)

	// 创建 Collectors
	collectors := collectors.NewCollectors(logStorage, eventBus, orderManager)

	// 创建 ResultBuilder
	resultBuilder := builder.NewResultBuilder(
		collectors,
		orderManager,
		accountManager,
		marketProvider,
		config.BaseCurrency,
		config.BaseExchange,
		accountIDProvider,
	)
	consoleAPI := api.NewConsoleAPI(logRecorder)
	timeAPI := api.NewTimeAPI(clock)

	// 创建 MarketFacade 用于统一市场数据访问
	marketFacade := facade.NewMarketFacade(marketProvider, true) // isBacktest = true

	// 创建 TradeFacade 用于统一交易能力（回测中暂时不完全实现所有功能）
	tradeFacade := facade.NewTradeFacade(facade.TradeFacadeConfig{
		TradeCollector: collectors.Trade,
		Portfolio:      portfolio,
		PlaceOrderFn: func(ctx context.Context, req *stypes.PlaceOrderCommand) (*stypes.PlaceOrderResult, error) {
			if req != nil {
				accountID := accountIDProvider(req.Exchange, req.Symbol)
				req.AccountID = lo.FromPtr(accountID)
			}
			return orderManager.PlaceOrder(ctx, req, riskController.Check)
		},
		CancelOrderFn: func(ctx context.Context, req *stypes.CancelOrderCommand) error {
			if req != nil {
				accountID := accountIDProvider(req.Exchange, req.Symbol)
				req.AccountID = lo.FromPtr(accountID)
			}
			return orderManager.CancelOrder(ctx, req)
		},
		GetOrdersFn: func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) ([]*ctypes.Order, error) {
			accountID := accountIDProvider(exchange, symbol)
			return orderManager.GetOrders(ctx, lo.FromPtr(accountID), symbol)
		},
		GetOrderFn: func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error) {
			accountID := accountIDProvider(exchange, symbol)
			return orderManager.GetOrder(ctx, lo.FromPtr(accountID), symbol, ctypes.OrderId(orderId))
		},
		GetPositionsFn: func(ctx context.Context, exchange ctypes.Exchange, symbol *ctypes.Symbol, side *ctypes.PositionSide) ([]*ctypes.Position, error) {
			return portfolio.GetPositions(exchange, symbol)
		},
		GetLeverageFn: func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (int, error) {
			accountID := accountIDProvider(exchange, symbol)
			if accountID == nil {
				return 0, fmt.Errorf("account not found for %s %s", exchange, symbol)
			}
			return accountManager.GetLeverage(ctx, lo.FromPtr(accountID), symbol)
		},
		SetLeverageFn: func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, leverage int) error {
			accountID := accountIDProvider(exchange, symbol)
			if accountID == nil {
				return fmt.Errorf("account not found for %s %s", exchange, symbol)
			}
			return accountManager.SetLeverage(ctx, lo.FromPtr(accountID), symbol, leverage)
		},
		AccountIDProvider: accountIDProvider,
		IsBacktest:        true,
	})

	storage := runner.NewStorage()

	// 创建 SymbolAPI（提供完整的 symbol 方法）
	symbolAPI := api.NewSymbolAPI(
		marketFacade.GetTicker,
		marketFacade.GetDepth,
		marketFacade.GetTrades,
		marketFacade.GetKlines,
		tradeFacade.Buy,
		tradeFacade.Sell,
		tradeFacade.CancelOrder,
		tradeFacade.GetOrders,
		tradeFacade.GetOrder,
		tradeFacade.GetFills,
		tradeFacade.GetPositions,
		tradeFacade.GetLeverage,
		tradeFacade.SetLeverage,
		tradeFacade.GetFundings,
		tradeFacade.GetAccount,
		tradeFacade.GetAsset,
		storage.Set,
		storage.Get,
		storage.Delete,
	)

	// 创建 ExchangeAPI
	exchangeAPI := api.NewExchangeAPI(
		marketFacade.GetMarkets,
		marketFacade.GetTickers,
	)

	// 提取 symbols 列表
	symbols := make([]ctypes.ExSymbol, 0, len(config.Symbols))
	for _, sym := range config.Symbols {
		symbols = append(symbols, ctypes.NewExSymbol(sym.Exchange, sym.Symbol))
	}

	runtime := runner.NewRuntime(consoleAPI, timeAPI).
		WithParams(config.Params).
		WithSymbolAPI(symbolAPI).
		WithExchangeAPI(exchangeAPI).
		WithSymbols(symbols).
		WithMode(stypes.RunModeBacktest).
		WithStorage(storage)

	sandbox := runner.DefaultSandbox()
	jsEngine, err := runner.NewV8Engine(strategy.Code, sandbox, runtime, logRecorder)
	if err != nil {
		return nil, fmt.Errorf("failed to create JS engine: %w", err)
	}

	return &BacktestExecutor{
		status:           stypes.ExecutorStatusInit,
		done:             make(chan struct{}),
		config:           config,
		btCtx:            btCtx,
		clock:            clock,
		bus:              eventBus,
		tScheduler:       scheduler,
		jsRunner:         jsEngine,
		exchangeGateway:  exGateway,
		orderManager:     orderManager,
		accountManager:   accountManager,
		portfolio:        portfolio,
		symbolAccountMgr: symbolAccountMgr,
		marketProvider:   marketProvider,
		collectors:       collectors,
		resultBuilder:    resultBuilder,
	}, nil
}

// Start 启动回测
func (e *BacktestExecutor) Start(ctx context.Context) (<-chan struct{}, error) {
	if e.status != stypes.ExecutorStatusInit {
		return nil, fmt.Errorf("executor status is not init")
	}

	e.runAt = time.Now()
	e.status = stypes.ExecutorStatusRunning
	e.ctx, e.cancel = context.WithCancel(ctx)

	// 启动交易所 gateway 桥接：订阅 market、驱动撮合并发布订单/成交/快照事件
	if e.exchangeGateway != nil {
		err := e.exchangeGateway.Start(e.ctx)
		if err != nil {
			return nil, err
		}
	}

	if e.marketProvider != nil {
		err := e.marketProvider.Start()
		if err != nil {
			return nil, err
		}
	}

	// 注入初始状态（保证策略 OnInit 可读取余额/仓位）
	if err := e.injectInitialState(); err != nil {
		e.setRunErr(err)
		close(e.done)
		return nil, err
	}

	// 调用初始化函数
	if err := e.jsRunner.OnInit(e.ctx); err != nil {
		log.Ctx(e.ctx).Err(err).Str("backtest_id", e.btCtx.ID).Msg("failed to initialize strategy")
		e.setRunErr(err)
		close(e.done)
		return nil, err
	}

	// 执行回测
	go e.runBacktest()

	log.Ctx(e.ctx).Info().Str("backtest_id", e.btCtx.ID).Msg("backtest executor started")
	return e.done, nil
}

// runBacktest 执行回测
func (e *BacktestExecutor) runBacktest() {
	defer close(e.done)
	defer func() {
		e.endAt = time.Now()
		if r := recover(); r != nil {
			err := fmt.Errorf("panic in backtest: %v", r)
			log.Ctx(e.ctx).Err(err).
				Str("panic.stack", string(debug.Stack())).
				Str("backtest_id", e.btCtx.ID).
				Msg("backtest panic")
			e.setRunErr(err)
		}
	}()

	var (
		st   time.Time
		p1Ts time.Time
		p2Ts time.Time
		p3Ts time.Time
		p4Ts time.Time

		p1Duration time.Duration
		p2Duration time.Duration
		p3Duration time.Duration
		p4Duration time.Duration
	)

	// 按时间顺序处理信号（point frame：internal before -> external -> internal after）
	for {
		select {
		case <-e.ctx.Done():
			e.status = stypes.ExecutorStatusCanceled
			return
		default:
		}

		st = time.Now()
		pf, ok, err := e.tScheduler.NextFrame(e.ctx)
		if err != nil {
			log.Ctx(e.ctx).Err(err).Str("backtest_id", e.btCtx.ID).Msg("timeline scheduler next frame failed")
			e.setRunErr(err)
			return
		}
		p1Ts = time.Now()
		if !ok {
			// signal 耗尽：回测结束
			break
		}

		// 防止“纯 internal 链条”把时间推进到回测窗口之外导致无限运行：
		// 当 external 数据源耗尽后，策略仍可能基于 internal event 继续下单产生新 internal event。
		// 这里以 BacktestConfig.EndTime 作为硬边界，超过即结束回测。
		if !e.config.EndTime.IsZero() && pf.Ts.After(e.config.EndTime) {
			break
		}

		// 更新当前时间
		if err := e.clock.Set(pf.Ts); err != nil {
			log.Ctx(e.ctx).Err(err).
				Str("backtest_id", e.btCtx.ID).
				Time("timestamp", pf.Ts).
				Msg("failed to set clock time")
			e.setRunErr(err)
			return
		}

		// 两阶段处理机制：先更新所有市场数据缓存，再处理业务逻辑
		// 第一阶段：更新所有市场数据缓存(回测场景需要手动更新市场数据，生产环境由数据源自动更新)
		for _, ev := range pf.Messages {
			if ev == nil {
				continue
			}
			// 所有 market signal 都先更新市场数据缓存
			if ev.Type().IsMarketSignal() {
				e.marketProvider.OnEvent(e.ctx, ev.Signal)
			}
		}

		// 第二阶段：将市场事件转发给交易所网关，撮合引擎消费市场事件并撮合
		for _, ev := range pf.Messages {
			if ev == nil {
				continue
			}
			if err := e.exchangeGateway.OnMarketSignal(e.ctx, ev.Signal); err != nil {
				log.Ctx(e.ctx).Err(err).Str("backtest_id", e.btCtx.ID).Msg("failed to send market signal to exchange gateway")
			}
		}

		p2Ts = time.Now()

		// 第三阶段：通过 TimelineEventBus.Send 分发到订阅者（统一调度）
		// V8Engine 仍需要 Signal（JavaScript 接口兼容性）
		for _, msg := range pf.Messages {
			if msg == nil {
				continue
			}
			if err := e.bus.Send(e.ctx, msg.Signal); err != nil {
				log.Ctx(e.ctx).Err(err).
					Str("backtest_id", e.btCtx.ID).
					Time("timestamp", pf.Ts).
					Msg("failed to send frame events")
				e.setRunErr(err)
				return
			}
			if msg.IsDerived {
				continue
			}
			if err := e.jsRunner.OnSignal(e.ctx, msg.Signal); err != nil {
				log.Ctx(e.ctx).Err(err).
					Str("backtest_id", e.btCtx.ID).
					Time("timestamp", msg.Ts).
					Str("signal_type", string(msg.Type())).
					Msg("failed to process merged frame signal")
			}
		}

		p3Ts = time.Now()

		// 触发权益点计算
		equityPoint, err := e.CalculateEquityPoint(e.ctx)
		if err != nil {
			log.Ctx(e.ctx).Err(err).Str("backtest_id", e.btCtx.ID).Msg("failed to calculate equity point")
			e.setRunErr(err)
			return
		}
		e.collectors.Equity.OnEquityPoint(*equityPoint)

		p4Ts = time.Now()

		p1Duration = p1Duration + p1Ts.Sub(st)
		p2Duration = p2Duration + p2Ts.Sub(p1Ts)
		p3Duration = p3Duration + p3Ts.Sub(p2Ts)
		p4Duration = p4Duration + p4Ts.Sub(p3Ts)
	}

	log.Ctx(e.ctx).Info().Str("backtest_id", e.btCtx.ID).
		Int64("p1_duration", p1Duration.Milliseconds()).
		Int64("p2_duration", p2Duration.Milliseconds()).
		Int64("p3_duration", p3Duration.Milliseconds()).
		Int64("p4_duration", p4Duration.Milliseconds()).
		Msg("backtest duration")

	// 回测完成
	e.status = stypes.ExecutorStatusFinished
	log.Ctx(e.ctx).Info().Str("backtest_id", e.btCtx.ID).Msg("backtest completed")
}

func (e *BacktestExecutor) injectInitialState() error {
	// 让撮合引擎/策略状态以 startTime 为一致的初始时间
	if err := e.clock.Set(e.config.StartTime); err != nil {
		return fmt.Errorf("failed to set initial clock time: %w", err)
	}

	// 初始化价格提供器
	for _, source := range e.config.Sources {
		if source == nil {
			continue
		}
		ev, ok, err := source.Peek(e.ctx)
		if err != nil {
			return fmt.Errorf("failed to peek source: %w", err)
		}
		if !ok {
			continue
		}
		if ev == nil {
			continue
		}
		_ = e.marketProvider.OnEvent(e.ctx, ev.Signal)
	}

	for _, symCfg := range e.config.Symbols {
		ex := symCfg.Exchange
		sym := symCfg.Symbol
		baseQty, err := decimal.NewFromString(symCfg.BaseAssetQty)
		if err != nil {
			return fmt.Errorf("invalid base asset qty %q for %s: %w", symCfg.BaseAssetQty, sym.String(), err)
		}
		quoteQty, err := decimal.NewFromString(symCfg.QuoteAssetQty)
		if err != nil {
			return fmt.Errorf("invalid quote asset qty %q for %s: %w", symCfg.QuoteAssetQty, sym.String(), err)
		}

		baseBal := baseQty
		if sym.Type == ctypes.MarketTypeFuture {
			baseBal = decimal.Zero
		}

		// 现阶段：用配置值作为初始快照写入 StrategyState（事件形式），以便 JS API 读取。
		// 对 FUTURE：先把 QuoteAssetQty 视作 collateral，BaseAssetQty 视作初始仓位 qty（可为负）；
		// AvgPrice 等更完整字段会在永续合约 todo 中补齐。

		// 发布初始余额事件
		if baseBal.GreaterThan(decimal.Zero) {
			baseBalanceSignal := &stypes.BalanceSignal{
				BaseSignal: stypes.BaseSignal{
					Exchange:  &ex,
					Symbol:    &sym,
					AccountID: accountIDProvider(ex, sym),
					Ts:        e.config.StartTime,
				},
				WalletType: ctypes.WalletTypeTrade,
				Asset:      sym.Base,
				Free:       baseBal,
				Frozen:     decimal.Zero,
			}
			if err := e.bus.Publish(e.ctx, baseBalanceSignal); err != nil {
				return fmt.Errorf("failed to publish base balance event: %w", err)
			}

			// 现货初始持仓需要记录成本价（使用初始价格作为成本价）
			if sym.Type == ctypes.MarketTypeSpot {
				exSymbol := ctypes.NewExSymbol(ex, sym)
				initialPrice, err := e.marketProvider.GetLastPrice(e.ctx, ex, sym)
				if err == nil && !initialPrice.IsZero() {
					// 通过发布 FillSignal 来初始化成本跟踪（模拟初始买入）
					accountID := accountIDProvider(ex, sym)
					if accountID != nil {
						initFillSignal := &stypes.FillSignal{
							BaseSignal: stypes.BaseSignal{
								Exchange:  &ex,
								Symbol:    &sym,
								AccountID: accountID,
								Ts:        e.config.StartTime,
							},
							OrderID: ctypes.OrderId("INIT_" + exSymbol.String()),
							Side:    ctypes.PositionSideLong,
							IsBuy:   true,
							Qty:     baseBal,
							Price:   initialPrice,
							Fee:     decimal.Zero,
							Asset:   sym.Base,
						}
						if err := e.bus.Publish(e.ctx, initFillSignal); err != nil {
							return fmt.Errorf("failed to publish initial fill signal: %w", err)
						}
					}
				}
			}
		}
		if quoteQty.GreaterThan(decimal.Zero) {
			quoteBalanceSignal := &stypes.BalanceSignal{
				BaseSignal: stypes.BaseSignal{
					Exchange:  &ex,
					Symbol:    &sym,
					AccountID: accountIDProvider(ex, sym),
					Ts:        e.config.StartTime,
				},
				WalletType: ctypes.WalletTypeTrade,
				Asset:      sym.Quote,
				Free:       quoteQty,
				Frozen:     decimal.Zero,
			}
			if err := e.bus.Publish(e.ctx, quoteBalanceSignal); err != nil {
				return fmt.Errorf("failed to publish quote balance event: %w", err)
			}
		}

	}

	return nil
}

// Done 返回回测完成信号
func (e *BacktestExecutor) Done() <-chan struct{} {
	return e.done
}

// GetSymbolAccountManager 获取策略级交易对账户管理器（用于测试）
func (e *BacktestExecutor) GetSymbolAccountManager() *symbolaccount.Manager {
	return e.symbolAccountMgr
}

// Stop 停止回测
func (e *BacktestExecutor) Stop(ctx context.Context) error {
	if e.status != stypes.ExecutorStatusRunning {
		return nil
	}

	e.status = stypes.ExecutorStatusCanceled

	if e.cancel != nil {
		e.cancel()
	}
	if e.exchangeGateway != nil {
		e.exchangeGateway.Stop()
	}

	if e.marketProvider != nil {
		e.marketProvider.Close()
	}

	err := e.jsRunner.Close()
	if err != nil {
		logger.Ctx(ctx).Err(err).Str("backtest_id", e.btCtx.ID).Msg("failed to close JS engine")
		return err
	}

	logger.Ctx(ctx).Info().Str("backtest_id", e.btCtx.ID).Msg("backtest executor stopped")
	return nil
}

func (e *BacktestExecutor) GetResult() (*stypes.BacktestResult, error) {
	if e.status != stypes.ExecutorStatusFinished &&
		e.status != stypes.ExecutorStatusError &&
		e.status != stypes.ExecutorStatusCanceled {
		return nil, fmt.Errorf("executor status is not finished")
	}

	if e.status == stypes.ExecutorStatusError {
		return nil, e.getRunErr()
	}

	// 使用 ResultBuilder 构建结果
	return e.resultBuilder.BuildResult(
		context.Background(),
		e.btCtx,
		e.config,
		e.config.StartTime,
		e.config.EndTime,
		e.endAt.Sub(e.runAt).Milliseconds(),
	)
}

func (e *BacktestExecutor) setRunErr(err error) {
	e.errMu.Lock()
	defer e.errMu.Unlock()
	e.runErr = err
	e.endAt = time.Now()
	e.status = stypes.ExecutorStatusError
}

func (e *BacktestExecutor) getRunErr() error {
	e.errMu.RLock()
	defer e.errMu.RUnlock()
	return e.runErr
}

// CalculateEquityPoint 计算权益点
func (e *BacktestExecutor) CalculateEquityPoint(ctx context.Context) (*stypes.EquityPoint, error) {
	equityPoint := &stypes.EquityPoint{
		Ts:            e.clock.Now(),
		TotalNetValue: decimal.Zero,
		Symbols:       make([]stypes.SymbolEquityPoint, 0, len(e.config.Symbols)),
	}

	// 遍历所有配置的标的，计算每个标的的权益
	for _, symCfg := range e.config.Symbols {
		exSymbol := ctypes.NewExSymbol(symCfg.Exchange, symCfg.Symbol)

		// 从 symbolAccountMgr 获取该交易对的余额（避免跨交易对重复计价同一资产）
		var baseQty, quoteQty decimal.Decimal
		baseFree, baseFrozen := e.symbolAccountMgr.GetBalance(exSymbol, exSymbol.GetBase())
		baseQty = baseFree.Add(baseFrozen)
		quoteFree, quoteFrozen := e.symbolAccountMgr.GetBalance(exSymbol, exSymbol.GetQuote())
		quoteQty = quoteFree.Add(quoteFrozen)

		var symbolEquity stypes.SymbolEquityPoint
		var baseNetValue, quoteNetValue decimal.Decimal

		if exSymbol.GetType() == ctypes.MarketTypeFuture {
			// FUTURE: 先计算该 symbol 自身货币下的账户权益
			// 获取标记价格
			mark, err := e.marketProvider.GetMarkPrice(ctx, exSymbol.Exchange, exSymbol.Symbol)
			if err != nil {
				// 如果获取失败，尝试使用 lastPrice
				mark, err = e.marketProvider.GetLastPrice(ctx, exSymbol.Exchange, exSymbol.Symbol)
				if err != nil {
					// 如果获取失败，使用 0（避免阻塞）
					mark = decimal.Zero
				}
			}

			// 从 portfolio 获取持仓信息
			positions, err := e.portfolio.GetPositions(symCfg.Exchange, &symCfg.Symbol)
			if err != nil {
				return nil, fmt.Errorf("failed to get positions for %s: %w", exSymbol.String(), err)
			}

			var posQty, avgPx decimal.Decimal
			if len(positions) > 0 && positions[0] != nil {
				pos := positions[0]
				// 根据持仓方向确定数量（正数表示多头，负数表示空头）
				if pos.Side == ctypes.PositionSideLong {
					posQty = pos.Amount
				} else {
					posQty = pos.Amount.Neg()
				}
				avgPx = pos.EntryPrice
			}

			// 计算未实现盈亏
			unrealized := decimal.Zero
			if !posQty.IsZero() && !avgPx.IsZero() && !mark.IsZero() {
				unrealized = mark.Sub(avgPx).Mul(posQty)
			}

			// 权益 = 保证金余额 + 未实现盈亏
			equityInCollateral := quoteQty.Add(unrealized)

			// 将 collateral 换算到 BaseCurrency
			quotePrice, err := e.marketProvider.GetPriceInBaseCurrency(ctx, exSymbol.GetQuote(), e.config.BaseCurrency)
			if err != nil {
				return nil, fmt.Errorf("failed to get quote price for %s: %w", exSymbol.GetQuote(), err)
			}

			baseNetValue = decimal.Zero
			quoteNetValue = equityInCollateral.Mul(quotePrice)

			symbolEquity = stypes.SymbolEquityPoint{
				ExSymbol:      exSymbol,
				BaseNetValue:  baseNetValue,
				QuoteNetValue: quoteNetValue,
				BaseQty:       posQty,
				QuoteQty:      equityInCollateral,
				PosQty:        posQty,
				AvgPx:         avgPx,
			}
		} else {
			// SPOT: baseQty * basePriceInBase + quoteQty * quotePriceInBase
			// 优化：如果数量为零，跳过价格查询
			var basePrice, quotePrice decimal.Decimal

			if !baseQty.IsZero() {
				var err error
				basePrice, err = e.marketProvider.GetPriceInBaseCurrency(ctx, exSymbol.GetBase(), e.config.BaseCurrency)
				if err != nil {
					return nil, fmt.Errorf("failed to get base price for %s: %w", exSymbol.GetBase(), err)
				}
			} else {
				basePrice = decimal.Zero
			}

			if !quoteQty.IsZero() {
				var err error
				quotePrice, err = e.marketProvider.GetPriceInBaseCurrency(ctx, exSymbol.GetQuote(), e.config.BaseCurrency)
				if err != nil {
					return nil, fmt.Errorf("failed to get quote price for %s: %w", exSymbol.GetQuote(), err)
				}
			} else {
				quotePrice = decimal.Zero
			}

			baseNetValue = baseQty.Mul(basePrice)
			quoteNetValue = quoteQty.Mul(quotePrice)

			symbolEquity = stypes.SymbolEquityPoint{
				ExSymbol:      exSymbol,
				BaseNetValue:  baseNetValue,
				QuoteNetValue: quoteNetValue,
				BaseQty:       baseQty,
				QuoteQty:      quoteQty,
				PosQty:        decimal.Zero,
				AvgPx:         decimal.Zero,
			}
		}

		// 累加总权益（使用 symbolAccount 后，每个资产只在其对应的 symbol 下计入一次，不会跨 symbol 重复）
		equityPoint.TotalNetValue = equityPoint.TotalNetValue.Add(baseNetValue).Add(quoteNetValue)
		equityPoint.Symbols = append(equityPoint.Symbols, symbolEquity)
	}

	return equityPoint, nil
}
