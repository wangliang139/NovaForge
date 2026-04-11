package live

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/internal/chsdk"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	"github.com/wangliang139/NovaForge/server/pkg/repos/bot"
	"github.com/wangliang139/NovaForge/server/pkg/strategy"
	mb "github.com/wangliang139/NovaForge/server/pkg/strategy/infra/bus"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/clock"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/logging"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/logging/store"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/marketdata"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/portfolio"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/risk"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/runner"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/runner/api"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/runner/api/facade"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	"github.com/wangliang139/mow/logger"
)

const (
	maxSignalHistoryCount = 1000
	signalHistoryWindow   = time.Hour
)

// LiveExecutorConfig 模拟盘执行器配置
type LiveExecutorConfig struct {
	DB           *repos.Entity
	ChClient     *chsdk.Client
	Bot          *stypes.Bot
	Strategy     *stypes.Strategy
	BaseCurrency string
	BaseExchange ctypes.Exchange
}

// LiveExecutor 模拟盘执行器
type LiveExecutor struct {
	ctx    context.Context
	cancel context.CancelFunc

	status stypes.ExecutorStatus

	done   chan struct{}
	errMu  sync.RWMutex
	runErr error

	logger logging.Logger

	bot      *stypes.Bot
	strategy *stypes.Strategy
	config   LiveExecutorConfig

	chClient *chsdk.Client
	exchange ctypes.Exchange

	jsRunner *runner.V8Engine
	storage  *runner.Storage

	clock          clock.Clock               // 实时时钟
	bus            mb.Bus                    // 内部事件总线
	signalCh       chan stypes.Signal        // 外部信号通道
	marketProvider marketdata.MarketProvider // 市场数据提供器
	orderEngine    strategy.OrderEngine      // 订单引擎
	accountEngine  strategy.AccountEngine    // 账户管理器
	portfolio      *portfolio.Portfolio      // 投资组合

	startAt time.Time

	// signal 统计
	lastSignalTs       int64 // unix milliseconds
	signalHistoryMu    sync.RWMutex
	signalHistory      []stypes.SignalStats
	signalHistoryCount int64 // number of signals appended to history
}

var _ strategy.Executor = &LiveExecutor{}

// NewLiveExecutor 创建模拟盘执行器
func NewLiveExecutor(
	config LiveExecutorConfig,
	marketProvider marketdata.MarketProvider,
	accountEngine strategy.AccountEngine,
	orderEngine strategy.OrderEngine,
) (*LiveExecutor, error) {
	if config.Bot == nil {
		return nil, fmt.Errorf("bot is required")
	}
	if config.Strategy == nil {
		return nil, fmt.Errorf("strategy is required")
	}
	if accountEngine == nil {
		return nil, fmt.Errorf("account engine is required")
	}
	if orderEngine == nil {
		return nil, fmt.Errorf("order engine is required")
	}

	// 设置默认值
	if config.BaseCurrency == "" {
		config.BaseCurrency = "USDT"
	}

	// 创建实时时钟
	clk := clock.DefaultRealClock

	// 创建事件总线
	eventBus := mb.NewAsync()
	if err := eventBus.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start event bus: %w", err)
	}

	// 创建 Portfolio
	ptf := portfolio.NewPortfolio(eventBus, marketProvider)

	symbols := []ctypes.ExSymbol{}
	symbolKeys := []ctypes.ExSymbolKey{}

	if config.Bot.AccountID == "" {
		return nil, fmt.Errorf("bot account id is required")
	}
	for _, symbol := range config.Bot.Symbols {
		exSymbol := ctypes.NewExSymbol(config.Bot.Exchange, symbol)
		symbolKeys = append(symbolKeys, exSymbol.Key())
		symbols = append(symbols, exSymbol)
	}

	// accountIDProvider: live 模式下固定返回 bot.AccountID
	accountIDProvider := func(exchange ctypes.Exchange, symbol ctypes.Symbol) *string {
		return lo.ToPtr(config.Bot.AccountID)
	}

	riskController := risk.NewRiskController(risk.DefaultConfig(), ptf, marketProvider)

	// 日志记录器
	baseLogger := logging.NewZeroLogger(logging.WithModule("live"))
	logStorage, err := store.NewClickhouseStorage(config.Bot.ID, 1*time.Second, config.ChClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create clickhouse storage: %w", err)
	}
	chLogger := logging.NewSinkLogger(logStorage, clk.Now)
	logRecorder := logging.NewCombinedLogger(chLogger, baseLogger)

	// 加载 Storage（从 DB）
	storage := runner.NewStorage()
	botData, err := config.DB.BotRepo.GetBot(context.Background(), config.Bot.ID)
	if err == nil && botData != nil && len(botData.Config) > 0 {
		var configMap map[string]any
		if sonic.Unmarshal(botData.Config, &configMap) == nil {
			if storageData, ok := configMap["storage"].(map[string]any); ok {
				storageMap := make(map[string]runner.StorageItem)
				for k, v := range storageData {
					if vm, ok := v.(map[string]any); ok {
						item := runner.StorageItem{}
						if vStr, ok := vm["v"].(string); ok {
							item.Value = vStr
						}
						if ve, ok := vm["e"].(float64); ok {
							item.ExpiresAt = int64(ve)
						}
						storageMap[k] = item
					}
				}
				storage.Load(storageMap)
			}
		}
	}

	// 创建 API
	consoleAPI := api.NewConsoleAPI(logRecorder)
	timeAPI := api.NewTimeAPI(clk)

	// 创建 MarketFacade
	marketFacade := facade.NewMarketFacade(marketProvider, false) // isBacktest = false

	// 创建 TradeFacade
	tradeFacade := facade.NewTradeFacade(facade.TradeFacadeConfig{
		Portfolio: ptf,
		PlaceOrderFn: func(ctx context.Context, req *stypes.PlaceOrderCommand) (*stypes.PlaceOrderResult, error) {
			if req != nil {
				req.AccountID = lo.FromPtr(accountIDProvider(req.Exchange, req.Symbol))
				req.BotID = int64(config.Bot.ID)
			}
			return orderEngine.PlaceOrder(ctx, req, riskController.Check)
		},
		CancelOrderFn: func(ctx context.Context, req *stypes.CancelOrderCommand) error {
			if req != nil {
				req.AccountID = lo.FromPtr(accountIDProvider(req.Exchange, req.Symbol))
			}
			return orderEngine.CancelOrder(ctx, req)
		},
		GetOrdersFn: func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) ([]*ctypes.Order, error) {
			accountID := lo.FromPtr(accountIDProvider(exchange, symbol))
			return orderEngine.GetOrders(ctx, accountID, symbol)
		},
		GetOrderFn: func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error) {
			accountID := lo.FromPtr(accountIDProvider(exchange, symbol))
			return orderEngine.GetOrder(ctx, accountID, symbol, ctypes.OrderId(orderId))
		},
		GetPositionsFn: func(ctx context.Context, exchange ctypes.Exchange, symbol *ctypes.Symbol, side *ctypes.PositionSide) ([]*ctypes.Position, error) {
			return ptf.GetPositions(exchange, symbol)
		},
		GetLeverageFn: func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol) (int, error) {
			return ptf.GetLeverage(ctx, exchange, symbol)
		},
		SetLeverageFn: func(ctx context.Context, exchange ctypes.Exchange, symbol ctypes.Symbol, leverage int) error {
			return ptf.SetLeverage(ctx, exchange, symbol, leverage)
		},
		AccountIDProvider: accountIDProvider,
		IsBacktest:        false,
	})

	// 创建 SymbolAPI
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

	// 创建 Runtime
	runtime := runner.NewRuntime(consoleAPI, timeAPI).
		WithParams(config.Bot.Config.Params).
		WithSymbolAPI(symbolAPI).
		WithExchangeAPI(exchangeAPI).
		WithSymbols(symbols).
		WithMode(stypes.RunModeLive).
		WithStorage(storage)

	// 创建 JS 引擎
	sandbox := runner.DefaultSandbox()
	jsEngine, err := runner.NewV8Engine(config.Strategy.Code, sandbox, runtime, logRecorder)
	if err != nil {
		return nil, fmt.Errorf("failed to create JS engine: %w", err)
	}

	// 创建外部信号通道
	signalCh := make(chan stypes.Signal, 100)

	executor := &LiveExecutor{
		status:         stypes.ExecutorStatusInit,
		done:           make(chan struct{}),
		bot:            config.Bot,
		strategy:       config.Strategy,
		config:         config,
		exchange:       config.Bot.Exchange,
		clock:          clk,
		bus:            eventBus,
		signalCh:       signalCh,
		jsRunner:       jsEngine,
		storage:        storage,
		orderEngine:    orderEngine,
		accountEngine:  accountEngine,
		portfolio:      ptf,
		marketProvider: marketProvider,
		logger:         logRecorder,
	}

	return executor, nil
}

// Start 启动模拟盘执行器
func (e *LiveExecutor) Start(ctx context.Context) error {
	if e.status != stypes.ExecutorStatusInit {
		return fmt.Errorf("executor status is not init")
	}

	e.startAt = time.Now()
	e.ctx, e.cancel = context.WithCancel(context.Background())
	cleanup := func() {
		if e.cancel != nil {
			e.cancel()
		}
		if e.bus != nil {
			_ = e.bus.Stop(context.Background())
		}
		if e.jsRunner != nil {
			_ = e.jsRunner.Close()
		}
	}

	// 注入初始状态（从快照恢复或配置中初始化）
	if err := e.injectInitialState(); err != nil {
		e.setRunErr(err)
		cleanup()
		return err
	}

	// 从快照恢复策略变量
	if err := e.restoreFromSnapshot(nil); err != nil {
		log.Ctx(e.ctx).Warn().Err(err).Msg("failed to restore from snapshot, starting fresh")
		e.setRunErr(err)
		cleanup()
		return err
	}

	// 调用策略初始化函数
	if err := e.jsRunner.OnInit(e.ctx); err != nil {
		log.Ctx(e.ctx).Err(err).Int32("bot_id", e.bot.ID).Msg("failed to initialize strategy")
		e.setRunErr(err)
		cleanup()
		return err
	}

	e.status = stypes.ExecutorStatusRunning
	e.lastSignalTs = time.Now().UnixMilli()

	// 启动 Storage 定时持久化任务
	go e.persistStorageLoop()

	// 启动信号处理循环
	go e.runLoop()

	log.Ctx(ctx).Info().
		Int32("bot_id", e.bot.ID).
		Str("strategy_id", e.strategy.ID).
		Msg("live executor started")

	return nil
}

// persistStorageLoop 定时持久化 Storage 到数据库
func (e *LiveExecutor) persistStorageLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			// 最后一次持久化
			e.persistStorage()
			return
		case <-ticker.C:
			e.persistStorage()
		}
	}
}

// persistStorage 将 Storage 持久化到数据库
func (e *LiveExecutor) persistStorage() {
	if e.storage == nil {
		return
	}

	items := e.storage.All()
	if len(items) == 0 {
		return
	}

	// 序列化为 JSON
	storageData := make(map[string]map[string]any)
	for k, v := range items {
		storageData[k] = map[string]any{
			"v": v.Value,
			"e": v.ExpiresAt,
		}
	}

	// 从 config 中读取现有配置，合并 storage
	botData, err := e.config.DB.BotRepo.GetBot(e.ctx, e.bot.ID)
	if err != nil {
		log.Error().Err(err).Int32("bot_id", e.bot.ID).Msg("failed to get bot for storage persist")
		return
	}

	var configMap map[string]any
	if len(botData.Config) > 0 {
		if err := sonic.Unmarshal(botData.Config, &configMap); err != nil {
			log.Error().Err(err).Int32("bot_id", e.bot.ID).Msg("failed to parse config for storage persist")
			return
		}
	} else {
		configMap = make(map[string]any)
	}

	configMap["storage"] = storageData
	newConfig, err := sonic.Marshal(configMap)
	if err != nil {
		log.Error().Err(err).Int32("bot_id", e.bot.ID).Msg("failed to marshal config for storage persist")
		return
	}

	// 更新数据库
	_, err = e.config.DB.BotRepo.UpdateBotStorage(e.ctx, bot.UpdateBotStorageParams{
		ID:      e.bot.ID,
		Storage: newConfig,
	})
	if err != nil {
		log.Error().Err(err).Int32("bot_id", e.bot.ID).Msg("failed to update bot storage")
	}
}

// Stop 停止模拟盘执行器
func (e *LiveExecutor) Stop(ctx context.Context) error {
	if e.cancel != nil {
		e.cancel()
	}

	// 等待运行循环结束
	select {
	case <-e.done:
	case <-time.After(5 * time.Second):
		log.Warn().Int32("bot_id", e.bot.ID).Msg("live executor stop timeout")
	}

	// 停止 bus
	if e.bus != nil {
		_ = e.bus.Stop(ctx)
	}

	// 关闭 JS 引擎
	if e.jsRunner != nil {
		_ = e.jsRunner.Close()
	}

	e.status = stypes.ExecutorStatusFinished
	logger.Ctx(ctx).Info().Int32("bot_id", e.bot.ID).Msg("live executor stopped")
	return nil
}

// GetSignalChannel 返回信号通道（供 Dispatcher 注册）
func (e *LiveExecutor) GetSignalChannel() chan stypes.Signal {
	return e.signalCh
}

// runLoop 运行循环：消费外部信号
func (e *LiveExecutor) runLoop() {
	defer close(e.done)
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("panic in live executor: %v", r)
			log.Ctx(e.ctx).Err(err).
				Str("panic.stack", string(debug.Stack())).
				Int32("bot_id", e.bot.ID).
				Msg("live executor panic")
			e.setRunErr(err)
		}
	}()

	for {
		select {
		case <-e.ctx.Done():
			return
		case sig, ok := <-e.signalCh:
			if !ok {
				// 信号通道关闭
				return
			}
			if sig == nil {
				continue
			}

			if err := e.handleSignal(sig); err != nil {
				log.Ctx(e.ctx).Err(err).
					Int32("bot_id", e.bot.ID).
					Str("signal_type", string(sig.GetType())).
					Msg("failed to handle signal")
			}
		}
	}
}

// handleSignal 处理单个信号
func (e *LiveExecutor) handleSignal(sig stypes.Signal) error {
	startTime := time.Now()

	// 更新最新 signal 时间戳
	latencyMs := time.Since(sig.GetTimestamp()).Milliseconds()
	e.lastSignalTs = startTime.UnixMilli()
	if time.Since(sig.GetTimestamp()) > 10*time.Second {
		log.Ctx(e.ctx).Warn().Interface("signal", sig).Str("signal_type", string(sig.GetType())).Str("signal_timestamp", sig.GetTimestamp().Format(time.RFC3339)).Msg("signal timestamp is too old")
	}

	// 1. 发布到内部事件总线（触发订单/账户/portfolio 等订阅者）
	if err := e.bus.Publish(e.ctx, sig); err != nil {
		log.Ctx(e.ctx).Err(err).Msg("failed to publish signal to bus")
	}

	// 2. 调用策略的信号处理函数
	if err := e.jsRunner.OnSignal(e.ctx, sig); err != nil {
		e.logger.Errorf("strategy OnSignal error: %s", err.Error())
	}

	// 统计
	durationMs := time.Since(startTime).Milliseconds()
	e.appendSignalHistory(stypes.SignalStats{
		Ts:         sig.GetTimestamp(),
		DurationMs: durationMs,
		LatencyMs:  latencyMs,
	})

	return nil
}

func (e *LiveExecutor) appendSignalHistory(sig stypes.SignalStats) {
	e.signalHistoryMu.Lock()
	defer e.signalHistoryMu.Unlock()

	// 追加当前信号
	e.signalHistory = append(e.signalHistory, sig)

	// 每累计一定数量再触发一次裁剪，避免每次追加都遍历
	e.signalHistoryCount++
	if e.signalHistoryCount%100 != 0 {
		return
	}

	// 限制总数不超过 maxSignalHistoryCount
	if len(e.signalHistory) > maxSignalHistoryCount {
		excess := len(e.signalHistory) - maxSignalHistoryCount
		copy(e.signalHistory, e.signalHistory[excess:])
		e.signalHistory = e.signalHistory[:maxSignalHistoryCount]
	}

	// 按时间窗口裁剪旧的记录
	if len(e.signalHistory) > 0 {
		now := time.Now()
		cutoff := now.Add(-signalHistoryWindow)
		idx := 0
		for _, s := range e.signalHistory {
			if s.Ts.After(cutoff) {
				break
			}
			idx++
		}
		if idx > 0 {
			dstLen := len(e.signalHistory) - idx
			copy(e.signalHistory, e.signalHistory[idx:])
			e.signalHistory = e.signalHistory[:dstLen]
		}
	}
}

func (e *LiveExecutor) GetState() *stypes.ExecutorState {
	return &stypes.ExecutorState{
		Status:              e.status,
		RunErr:              e.runErr,
		Portfolio:           e.portfolio.Snapshot(),
		JsRunnerStatus:      e.GetJsRunnerStatus(),
		LastSignalTs:        e.lastSignalTs,
		SignalAvgDurationMs: e.GetSignalAvgDurationMs(),
		SignalAvgLatencyMs:  e.GetSignalAvgLatencyMs(),
	}
}

// setRunErr 设置运行错误
func (e *LiveExecutor) setRunErr(err error) {
	e.errMu.Lock()
	defer e.errMu.Unlock()
	if e.runErr == nil {
		e.runErr = err
	}
}

// GetRunErr 获取运行错误
func (e *LiveExecutor) GetRunErr() error {
	e.errMu.RLock()
	defer e.errMu.RUnlock()
	return e.runErr
}

// GetBotID 获取 Bot ID
func (e *LiveExecutor) GetBotID() int32 {
	return e.bot.ID
}

// GetStatus 获取执行器状态
func (e *LiveExecutor) GetStatus() stypes.ExecutorStatus {
	return e.status
}

// GetJsRunnerStatus 返回 JS runner 状态
func (e *LiveExecutor) GetJsRunnerStatus() string {
	if e.jsRunner == nil {
		return "nil"
	}
	if e.jsRunner.IsClosed() {
		return "closed"
	}
	return "running"
}

// GetLastSignalTs 返回最新 signal 的时间戳（unix milliseconds）
func (e *LiveExecutor) GetLastSignalTs() int64 {
	return e.lastSignalTs
}

// GetSignalAvgDurationMs 返回 signal 平均处理耗时（毫秒）
func (e *LiveExecutor) GetSignalAvgDurationMs() int64 {
	e.signalHistoryMu.RLock()
	defer e.signalHistoryMu.RUnlock()
	if len(e.signalHistory) == 0 {
		return 0
	}
	totalDurMs := int64(0)
	for _, s := range e.signalHistory {
		totalDurMs += s.DurationMs
	}
	return totalDurMs / int64(len(e.signalHistory))
}

// GetSignalAvgLatencyMs 返回 signal 平均延迟（从创建到开始处理的毫秒数）
func (e *LiveExecutor) GetSignalAvgLatencyMs() int64 {
	e.signalHistoryMu.RLock()
	defer e.signalHistoryMu.RUnlock()
	if len(e.signalHistory) == 0 {
		return 0
	}
	totalLatencyMs := int64(0)
	for _, s := range e.signalHistory {
		totalLatencyMs += s.LatencyMs
	}
	return totalLatencyMs / int64(len(e.signalHistory))
}

// injectInitialState 注入初始状态（余额/仓位）
// 优先从快照恢复，如果快照不存在则从配置初始化
func (e *LiveExecutor) injectInitialState() error {
	if e.portfolio == nil {
		return fmt.Errorf("portfolio is not initialized")
	}
	if e.accountEngine == nil {
		return fmt.Errorf("account engine is not initialized")
	}
	if e.bot == nil {
		return fmt.Errorf("bot is not set")
	}
	if e.bot.AccountID == "" {
		return fmt.Errorf("bot account id is required")
	}

	// 将 Bot 配置中的 symbols 传递给 Portfolio，便于后续按标的维度查询
	symbols := make([]ctypes.Symbol, 0, len(e.bot.Symbols))
	for _, s := range e.bot.Symbols {
		symbols = append(symbols, s)
	}

	ctx := e.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	log.Ctx(ctx).Info().
		Int32("bot_id", e.bot.ID).
		Str("account_id", e.bot.AccountID).
		Msg("initializing portfolio from account engine")

	return e.portfolio.Init(ctx, e.accountEngine, e.bot.AccountID, e.exchange, symbols)
}

// restoreFromSnapshot 从快照恢复状态
func (e *LiveExecutor) restoreFromSnapshot(snapshot *stypes.BotSnapshot) error {
	if snapshot == nil {
		return nil
	}

	// accountID := e.bot.AccountID
	// if accountID == "" {
	// 	accountID = e.bot.ID
	// }

	// ex := e.primaryExchange
	// sym := e.primarySymbol
	// aid := accountID
	// now := e.clock.Now()

	// log.Ctx(e.ctx).Info().
	// 	Int32("bot_id", e.bot.ID).
	// 	Int("balances", len(snapshot.Balances)).
	// 	Int("positions", len(snapshot.Positions)).
	// 	Int("orders", len(snapshot.Orders)).
	// 	Msg("restoring state from snapshot")

	// // 1. 恢复余额（使用 total = free + frozen 作为 free，frozen=0，让订单重放重建冻结）
	// for _, balance := range snapshot.Balances {
	// 	freeAmount, err := decimal.NewFromString(balance.Free)
	// 	if err != nil {
	// 		log.Ctx(e.ctx).Warn().Err(err).Str("asset", balance.Asset).Msg("failed to parse free amount")
	// 		continue
	// 	}
	// 	frozenAmount, err := decimal.NewFromString(balance.Frozen)
	// 	if err != nil {
	// 		frozenAmount = decimal.Zero
	// 	}

	// 	// 总余额 = free + frozen，全部恢复为 free（frozen 由订单重放重建）
	// 	totalAmount := freeAmount.Add(frozenAmount)

	// 	balanceSig := &stypes.BalanceSignal{
	// 		BaseSignal: stypes.BaseSignal{
	// 			Exchange:  &ex,
	// 			Symbol:    &sym,
	// 			AccountID: &aid,
	// 			Ts:        now,
	// 		},
	// 		Asset:  balance.Asset,
	// 		Free:   totalAmount,
	// 		Frozen: decimal.Zero,
	// 	}
	// 	_ = e.bus.Publish(e.ctx, balanceSig)
	// }

	// // 2. 恢复持仓（期货）
	// exSymbol := ctypes.NewExSymbol(e.primaryExchange, e.primarySymbol)
	// if exSymbol.GetType() == ctypes.MarketTypeFuture {
	// 	for _, position := range snapshot.Positions {
	// 		qty, err := decimal.NewFromString(position.Qty)
	// 		if err != nil || qty.IsZero() {
	// 			continue
	// 		}
	// 		entryPrice, err := decimal.NewFromString(position.EntryPrice)
	// 		if err != nil {
	// 			entryPrice = decimal.Zero
	// 		}

	// 		posSig := &stypes.PositionSignal{
	// 			BaseSignal: stypes.BaseSignal{
	// 				Exchange:  &ex,
	// 				Symbol:    &sym,
	// 				AccountID: &aid,
	// 				Ts:        now,
	// 			},
	// 			Qty:   qty,
	// 			Price: entryPrice,
	// 		}
	// 		_ = e.bus.Publish(e.ctx, posSig)
	// 	}
	// }

	// // 3. 取消所有在途订单
	// for _, orderState := range snapshot.Orders {
	// 	if err := e.cancelSnapshotOrder(orderState); err != nil {
	// 		log.Ctx(e.ctx).Warn().Err(err).
	// 			Int32("bot_id", e.bot.ID).
	// 			Str("client_order_id", orderState.ClientOrderID).
	// 			Msg("failed to cancel snapshot order")
	// 	}
	// }

	return nil
}
