package strategy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/wangliang139/llt-trade/server/pkg/internal/chsdk"
	"github.com/wangliang139/llt-trade/server/pkg/repos"
	"github.com/wangliang139/llt-trade/server/pkg/strategy"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/account"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/executor/backtest"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/manager"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/marketdata"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/order"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/pubsub"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/registry"
	ss "github.com/wangliang139/llt-trade/server/pkg/strategy/signal"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/signalflow"
	"github.com/wangliang139/llt-trade/server/pkg/strategy/sources"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
	"github.com/wangliang139/llt-trade/server/pkg/types"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Config 策略模块配置
type Config struct {
	Enabled bool `split_words:"true" default:"true"`

	EnableAlarm bool `split_words:"true" default:"true"`

	EnableRedisSubscriber bool   `split_words:"true" default:"false"`
	RedisStreamAddr       string `split_words:"true" default:"127.0.0.1:6379"`
	RedisStreamPassword   string `split_words:"true"`
	RedisStreamDB         int    `split_words:"true" default:"0"`
	RedisStreamPoolSize   int    `split_words:"true" default:"20"`
	RedisStreamTopic      string `split_words:"true" default:"md.all.msg"`

	BaseCurrency string          `split_words:"true" default:"USDT"`
	BaseExchange ctypes.Exchange `split_words:"true" default:"binance"`

	// ConsoleLogMaxCache 控制策略 console.log 的最大缓存条数（0 表示不缓存）。
	ConsoleLogMaxCache int `split_words:"true" default:"1000"`

	// SignalFlowEnabled controls whether bot signal flow should be recorded to ClickHouse.
	SignalFlowEnabled bool `split_words:"true" default:"true"`
}

// Entity 策略实体
type Entity struct {
	cfg Config

	db *repos.Entity

	strategyManager   manager.StrategyManager
	botManager        manager.BotManager
	datasourceManager manager.DatasourceManager

	orderEngine    strategy.OrderEngine
	accountEngine  strategy.AccountEngine
	marketProvider *marketdata.GlobalMarketProvider

	dispatcher       *pubsub.Dispatcher
	executorRegistry *registry.ExecutorRegistry
}

func New(db *repos.Entity) (*Entity, error) {
	cfg := Config{}
	if err := envconfig.Process("STRATEGY", &cfg); err != nil {
		return nil, fmt.Errorf("failed to process config: %w", err)
	}

	strategyManager := manager.NewStrategyManager(db)
	datasourceManager := manager.NewDatasourceManager(db)
	if !cfg.Enabled {
		botManager := manager.NewBotManager(db, strategyManager, nil)
		return &Entity{
			cfg:               cfg,
			db:                db,
			strategyManager:   strategyManager,
			botManager:        botManager,
			datasourceManager: datasourceManager,
		}, nil
	}

	marketProvider := marketdata.NewGlobalMarketProvider(cfg.BaseExchange, cfg.BaseCurrency)

	var err error
	var subscriber pubsub.Subscriber
	if cfg.EnableRedisSubscriber {
		if len(cfg.RedisStreamAddr) == 0 {
			return nil, fmt.Errorf("redis stream addr is required")
		}
		subscriber, err = pubsub.NewRedisStreamSubscriber(pubsub.RedisStreamConfig{
			Addr:     cfg.RedisStreamAddr,
			Password: cfg.RedisStreamPassword,
			DB:       cfg.RedisStreamDB,
			PoolSize: cfg.RedisStreamPoolSize,
			Topic:    cfg.RedisStreamTopic,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create redis stream subscriber: %w", err)
		}
	}

	dispatcher := pubsub.NewDispatcher(subscriber)
	dispatcher.AddHandler(marketProvider)
	if cfg.SignalFlowEnabled {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		chClient, err := chsdk.Connect(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("signal flow recorder not available (ClickHouse not configured)")
		} else {
			rec, err := signalflow.NewRecorder(chClient)
			if err != nil {
				log.Warn().Err(err).Msg("failed to create signal flow recorder")
			} else {
				dispatcher.SetSignalRecorder(rec)
				log.Info().Msg("signal flow recorder initialized")
			}
		}
	}

	accountEngine := account.NewLiveAccountEngine()

	orderEngine, err := order.NewOrderEngine(marketProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create order engine: %w", err)
	}

	// 创建执行器注册表
	executorRegistry, err := registry.NewExecutorRegistry(registry.Config{
		BaseCurrency:          cfg.BaseCurrency,
		BaseExchange:          cfg.BaseExchange.String(),
		MonitorInterval:       30 * time.Second,
		SignalAlarmTimeout:    5 * time.Minute,
		SignalShutdownTimeout: 30 * time.Minute,
		EnableAlarm:           cfg.EnableAlarm,
	}, db, dispatcher, accountEngine, orderEngine, marketProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create executor registry: %w", err)
	}

	botManager := manager.NewBotManager(db, strategyManager, executorRegistry)

	return &Entity{
		cfg:               cfg,
		db:                db,
		strategyManager:   strategyManager,
		botManager:        botManager,
		datasourceManager: datasourceManager,
		orderEngine:       orderEngine,
		accountEngine:     accountEngine,
		marketProvider:    marketProvider,
		dispatcher:        dispatcher,
		executorRegistry:  executorRegistry,
	}, nil
}

// Start 启动策略实体
func (e *Entity) Start() error {
	if !e.cfg.Enabled {
		log.Warn().Msg("strategy module is disabled")
		return nil
	}

	if err := e.marketProvider.Start(); err != nil {
		return fmt.Errorf("failed to start market provider: %w", err)
	}

	// 启动信号分发器（动态订阅由 registry 管理）
	if err := e.dispatcher.Start(); err != nil {
		return fmt.Errorf("failed to start dispatcher: %w", err)
	}

	// 启动执行器后台健康监控
	if e.executorRegistry != nil {
		e.executorRegistry.StartMonitor(context.Background())
	}

	// 恢复运行中的 bots
	if err := e.resumeRunningBots(); err != nil {
		log.Warn().Err(err).Msg("failed to resume running bots")
	}

	log.Info().Msg("strategy entity started")
	return nil
}

// resumeRunningBots 恢复运行中的 bots
func (e *Entity) resumeRunningBots() error {
	ctx := context.Background()

	// 查询状态为 running 的 bots
	runningStatus := stypes.BotStatusRunning
	filter := &stypes.BotFilter{
		Status: &runningStatus,
	}

	bots, err := e.botManager.ListBots(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to list running bots: %w", err)
	}

	if len(bots) == 0 {
		log.Info().Msg("no running bots to resume")
		return nil
	}

	log.Info().Int("count", len(bots)).Msg("resuming running bots")

	// 逐个启动
	for _, bot := range bots {
		if bot == nil {
			continue
		}

		// 获取策略（按 bot 记录的版本恢复）
		strategy, err := e.strategyManager.GetStrategyByVersion(ctx, bot.StrategyID, bot.StrategyVer)
		if err != nil {
			log.Warn().Err(err).
				Int32("bot_id", bot.ID).
				Str("strategy_id", bot.StrategyID).
				Msg("failed to get strategy for bot, marking as error")
			// 标记为 error
			err2 := e.botManager.UpdateBotStatus(ctx, bot.ID, stypes.BotStatusError, lo.ToPtr(err.Error()))
			if err2 != nil {
				log.Warn().Err(err2).
					Int32("bot_id", bot.ID).
					Msg("failed to update bot status")
			}
			continue
		}

		// 启动执行器
		if err := e.executorRegistry.Start(ctx, bot, strategy); err != nil {
			if errors.Is(err, stypes.ErrBotAlreadyRunning) || errors.Is(err, stypes.ErrBotAlreadyStarting) {
				continue
			}
			log.Warn().Err(err).
				Int32("bot_id", bot.ID).
				Msg("failed to resume bot, marking as error")
			// 标记为 error
			err2 := e.botManager.UpdateBotStatus(ctx, bot.ID, stypes.BotStatusError, lo.ToPtr(err.Error()))
			if err2 != nil {
				log.Warn().Err(err2).
					Int32("bot_id", bot.ID).
					Msg("failed to update bot status")
			}
			continue
		}

		log.Info().Int32("bot_id", bot.ID).Msg("bot resumed")
	}

	return nil
}

// Stop 停止策略实体
func (e *Entity) Stop(ctx context.Context) error {
	if !e.cfg.Enabled {
		return nil
	}

	// 停止所有运行中的执行器
	if e.executorRegistry != nil {
		if err := e.executorRegistry.Close(ctx); err != nil {
			log.Warn().Err(err).Msg("failed to close executor registry")
		}
	}

	// 关闭分发器
	if err := e.dispatcher.Close(); err != nil {
		log.Warn().Err(err).Msg("failed to close dispatcher")
	}

	// 关闭市场数据提供器
	e.marketProvider.Close()

	log.Info().Msg("strategy entity stopped")
	return nil
}

// CreateStrategy 创建策略
func (e *Entity) CreateStrategy(ctx context.Context, req *stypes.CreateStrategyRequest) (*stypes.Strategy, error) {
	return e.strategyManager.CreateStrategy(ctx, req)
}

// GetStrategy 获取策略
func (e *Entity) GetStrategy(ctx context.Context, id string, version *string) (*stypes.Strategy, error) {
	if version != nil {
		return e.strategyManager.GetStrategyByVersion(ctx, id, *version)
	}
	return e.strategyManager.GetStrategy(ctx, id)
}

// ListStrategies 列出策略
func (e *Entity) ListStrategies(ctx context.Context, filter *stypes.StrategyFilter) ([]*stypes.Strategy, error) {
	return e.strategyManager.ListStrategies(ctx, filter)
}

// CountStrategies 统计策略总数
func (e *Entity) CountStrategies(ctx context.Context) (int64, error) {
	return e.strategyManager.CountStrategies(ctx)
}

// UpdateStrategy 更新策略
func (e *Entity) UpdateStrategy(ctx context.Context, req *stypes.UpdateStrategyRequest) (*stypes.Strategy, error) {
	return e.strategyManager.UpdateStrategy(ctx, req)
}

// DeleteStrategy 删除策略
func (e *Entity) DeleteStrategy(ctx context.Context, id string) error {
	return e.strategyManager.DeleteStrategy(ctx, id)
}

// ActiveStrategy 激活策略
func (e *Entity) ActiveStrategy(ctx context.Context, id string) error {
	return e.strategyManager.ActiveStrategy(ctx, id)
}

// InactiveStrategy 禁用策略
func (e *Entity) InactiveStrategy(ctx context.Context, id string) error {
	return e.strategyManager.InactiveStrategy(ctx, id)
}

// CreateBot 创建Bot
func (e *Entity) CreateBot(ctx context.Context, req *stypes.CreateBotInput) (*stypes.Bot, error) {
	return e.botManager.CreateBot(ctx, req)
}

// UpdateBot 更新Bot
func (e *Entity) UpdateBot(ctx context.Context, req *stypes.UpdateBotInput) (*stypes.Bot, error) {
	return e.botManager.UpdateBot(ctx, req)
}

// StartBot 启动Bot
func (e *Entity) StartBot(ctx context.Context, botID int32) error {
	return e.botManager.StartBot(ctx, botID)
}

// StopBot 停止Bot
func (e *Entity) StopBot(ctx context.Context, botID int32) error {
	return e.botManager.StopBot(ctx, botID)
}

// UpgradeBot 升级 Bot：停止 -> 更新策略版本为最新 -> 启动
func (e *Entity) UpgradeBot(ctx context.Context, botID int32) (*stypes.Bot, bool, string, error) {
	return e.botManager.UpgradeBot(ctx, botID)
}

// GetBot 获取Bot
func (e *Entity) GetBot(ctx context.Context, botID int32) (*stypes.Bot, error) {
	return e.botManager.GetBot(ctx, botID)
}

// ListBots 列出Bot
func (e *Entity) ListBots(ctx context.Context, filter *stypes.BotFilter) ([]*stypes.Bot, error) {
	return e.botManager.ListBots(ctx, filter)
}

// CountBots 统计 Bot 数量（支持按 status 过滤）
func (e *Entity) CountBots(ctx context.Context, filter *stypes.BotFilter) (int64, error) {
	return e.botManager.CountBots(ctx, filter)
}

// GetRunningBotExecutor returns the executor of a running bot.
// ok=false means the bot is not running.
func (e *Entity) GetRunningBotExecutor(botID int32) (strategy.Executor, bool) {
	if botID <= 0 {
		return nil, false
	}
	if e.executorRegistry == nil {
		return nil, false
	}
	return e.executorRegistry.Get(botID)
}

// DeleteBot 删除Bot
func (e *Entity) DeleteBot(ctx context.Context, botID int32) error {
	return e.botManager.DeleteBot(ctx, botID)
}

// RunBacktest 回测入口
func (e *Entity) RunBacktest(ctx context.Context, input *stypes.RunBacktestInput) (*stypes.BacktestResult, error) {
	var strategy *stypes.Strategy
	var err error

	// 如果输入中已提供策略对象，直接使用；否则从数据库查询
	if input.Strategy == nil {
		// 从数据库查询策略
		strategyId := input.Context.StrategyID
		strategy, err = e.strategyManager.GetStrategy(ctx, strategyId)
		if err != nil {
			return nil, fmt.Errorf("failed to get strategy: %w", err)
		}
		if strategy == nil {
			return nil, fmt.Errorf("strategy not found")
		}
		if strategy.Version != input.Context.StrategyVer {
			return nil, status.Error(codes.InvalidArgument, "strategy version mismatch")
		}
	} else {
		strategy = input.Strategy
		if strategy.ID != input.Context.StrategyID {
			return nil, status.Error(codes.InvalidArgument, "strategy id mismatch")
		}
		if strategy.Version != input.Context.StrategyVer {
			return nil, status.Error(codes.InvalidArgument, "strategy version mismatch")
		}
	}

	// 校验 symbols
	exSymbolSet := make(map[ctypes.ExSymbolKey]*stypes.BacktestSymbol)
	for _, symbol := range input.Symbols {
		exSymbol := ctypes.NewExSymbol(symbol.Exchange, symbol.Symbol)
		if _, ok := exSymbolSet[exSymbol.Key()]; ok {
			return nil, status.Errorf(codes.InvalidArgument, "duplicated symbol: %s", exSymbol.String())
		}
		exSymbolSet[exSymbol.Key()] = symbol
	}

	// 检查input.Params和策略所需参数，补齐默认值，校验必填参数
	// 删除strategy.Params中不存在的字段，防止用户传入非法参数
	params, err := mergeBacktestParams(strategy, input.Params)
	if err != nil {
		return nil, err
	}

	// 构建信号源
	sources, err := e.buildSignalSources(ctx, strategy, input.Signals, input.Symbols, input.StartTime, input.EndTime)
	if err != nil {
		return nil, err
	}

	log.Info().Interface("sources", sources).Msg("strategy backtest sources")

	config := stypes.BacktestConfig{
		StartTime:    input.StartTime,
		EndTime:      input.EndTime,
		Symbols:      input.Symbols,
		Sources:      sources,
		Params:       params,
		BaseCurrency: e.cfg.BaseCurrency, // 默认使用 USDT
		BaseExchange: e.cfg.BaseExchange, // 默认使用 Binance
	}

	executor, err := backtest.NewBacktestExecutor(strategy, input.Context, config,
		backtest.WithConsoleLogMaxCache(e.cfg.ConsoleLogMaxCache),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create backtest executor: %w", err)
	}
	done, err := executor.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start backtest: %w", err)
	}

	select {
	case <-done:
		err := executor.Stop(ctx)
		if err != nil {
			return nil, err
		}
		return executor.GetResult()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (e *Entity) GetDatasource(ctx context.Context, id int32) (*types.DataSource, error) {
	ds, err := e.datasourceManager.GetDatasource(ctx, id)
	if err != nil {
		return nil, err
	}
	if ds == nil {
		return nil, status.Error(codes.NotFound, "datasource not found")
	}
	return ds, nil
}

func (e *Entity) CreateDatasource(ctx context.Context, req *types.CreateDatasourceInput) (*types.DataSource, int64, error) {
	ds, inserted, err := e.datasourceManager.CreateDatasource(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	return ds, inserted, nil
}

func (e *Entity) ListDatasources(ctx context.Context, filter *types.DatasourceFilter) ([]*types.DataSource, int64, error) {
	return e.datasourceManager.ListDatasources(ctx, filter)
}

func (e *Entity) DeleteDatasource(ctx context.Context, id int32) error {
	return e.datasourceManager.DeleteDatasource(ctx, id)
}

func mergeBacktestParams(strategy *stypes.Strategy, params map[string]any) (map[string]any, error) {
	if params == nil {
		params = make(map[string]any)
	}
	missingParams := []string{}
	allowedParams := make(map[string]struct{})
	for _, p := range strategy.Params {
		allowedParams[p.Name] = struct{}{}
		_, exists := params[p.Name]
		if !exists {
			if p.Default != nil {
				params[p.Name] = p.Default
			} else if p.Required {
				missingParams = append(missingParams, p.Name)
			}
		}
	}
	if len(missingParams) > 0 {
		return nil, fmt.Errorf("missing required params: %v", missingParams)
	}
	// 删除 input.Params 中不存在于 strategy.Params 的字段
	for k := range params {
		if _, ok := allowedParams[k]; !ok {
			delete(params, k)
		}
	}
	return params, nil
}

func (e *Entity) buildSignalSources(ctx context.Context, strategy *stypes.Strategy, signals []*stypes.SignalBinding, symbols []*stypes.BacktestSymbol, startTs, endTs time.Time) ([]stypes.Source, error) {
	// 统计symbols中的交易所数量
	exSymbolSet := make(map[ctypes.ExSymbolKey]stypes.BacktestSymbol)
	exchangeSet := make(map[ctypes.Exchange]struct{})
	for _, symbol := range symbols {
		if symbol == nil {
			continue
		}
		exSymbol := ctypes.NewExSymbol(symbol.Exchange, symbol.Symbol)
		exchangeSet[exSymbol.Exchange] = struct{}{}
		exSymbolSet[exSymbol.Key()] = *symbol
	}

	strategySignalByID := make(map[string]stypes.SignalDefinition, len(strategy.Signals))
	for _, sig := range strategy.Signals {
		strategySignalByID[sig.ID] = sig
	}

	signalBindingsByID := make(map[string][]stypes.SignalBinding)
	for _, s := range signals {
		if s == nil {
			continue
		}
		_, ok := strategySignalByID[s.SignalID]
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "signal_id not found in strategy definition: %s", s.SignalID)
		}
		signalBindingsByID[s.SignalID] = append(signalBindingsByID[s.SignalID], *s)
	}

	// 校验signal数量
	flatSignals := make([]flatSignalDefinition, 0, len(signals))
	for _, sig := range strategy.Signals {
		bindings, ok := signalBindingsByID[sig.ID]
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "signal_id not found in input signals: %s", sig.ID)
		}

		// 校验信号绑定数量
		expectedSignalCount := 0
		switch sig.Scope {
		case types.SignalScopeSymbol:
			expectedSignalCount = len(symbols)
		case types.SignalScopeExchange:
			expectedSignalCount = len(exchangeSet)
		case types.SignalScopeTarget:
			expectedSignalCount = 1
		case types.SignalScopeStrategy:
			expectedSignalCount = 1
		}
		if len(bindings) != expectedSignalCount {
			return nil, status.Errorf(codes.InvalidArgument, "signal %s should have %d bindings, got %d", sig.ID, expectedSignalCount, len(bindings))
		}

		// 回测/调用方常只传 signalId + datasourceId；symbol scope 下绑定与 symbols 按序一一对应，此处补全缺省的 exchange/symbol。
		if sig.Scope == types.SignalScopeSymbol {
			for i := range bindings {
				if bindings[i].Exchange != nil && bindings[i].Symbol != nil {
					continue
				}
				if i >= len(symbols) || symbols[i] == nil {
					return nil, status.Errorf(codes.InvalidArgument, "signal %s (symbol scope) binding[%d]: exchange/symbol missing and no backtest symbol at that index", sig.ID, i)
				}
				ex := symbols[i].Exchange
				sy := symbols[i].Symbol
				bindings[i].Exchange = &ex
				bindings[i].Symbol = &sy
			}
		}

		for i, binding := range bindings {
			// 校验信号绑定中的 exchange 和 symbol 是否与所需值一致，防御无效绑定，避免后续数据源异常
			// 如果 scope == target, 则需要校验 exchange/symbol 是否匹配
			// 如果 scope == symbol, 则需要校验 exchange/symbol 是否与 symbols 一一对应
			// 如果 scope == exchange, 则需要校验 exchange 是否与 exchangeSet 一一对应
			// 如果 scope == strategy, 则不需要校验
			desiredSignalDefinition := sig.Clone()
			switch sig.Scope {
			case types.SignalScopeTarget:
				// 只应该有一个绑定，exchange/symbol 必须匹配定义
				if binding.Exchange == nil || binding.Symbol == nil {
					return nil, status.Errorf(codes.InvalidArgument, "signal %s (target scope) binding[%d] exchange or symbol is nil", sig.ID, i)
				}
				if !binding.Exchange.IsValid() || sig.Exchange == nil || !binding.Exchange.Equal(*sig.Exchange) {
					return nil, status.Errorf(codes.InvalidArgument, "signal %s (target scope) binding[%d] exchange mismatch: got %s, expected %s", sig.ID, i, binding.Exchange, sig.Exchange)
				}
				if !binding.Symbol.IsValid() || sig.Symbol == nil || !binding.Symbol.Equal(*sig.Symbol) {
					return nil, status.Errorf(codes.InvalidArgument, "signal %s (target scope) binding[%d] symbol mismatch: got %s, expected %s", sig.ID, i, binding.Symbol, sig.Symbol)
				}
			case types.SignalScopeSymbol:
				// bindings 必须和 symbols 一一对应，检查 exchange/symbol 匹配
				if binding.Exchange == nil || binding.Symbol == nil {
					return nil, status.Errorf(codes.InvalidArgument, "signal %s (symbol scope) binding[%d] exchange or symbol is nil", sig.ID, i)
				}
				exSymbol := ctypes.NewExSymbol(*binding.Exchange, *binding.Symbol)
				if _, ok := exSymbolSet[exSymbol.Key()]; !ok {
					return nil, status.Errorf(codes.InvalidArgument, "signal %s (symbol scope) binding[%d] exsymbol not found: %s", sig.ID, i, exSymbol.String())
				}
				desiredSignalDefinition.Exchange = binding.Exchange
				desiredSignalDefinition.Symbol = binding.Symbol
			case types.SignalScopeExchange:
				if _, ok := exchangeSet[*binding.Exchange]; !ok {
					return nil, status.Errorf(codes.InvalidArgument, "signal %s (exchange scope) binding[%d] exchange not found: %s", sig.ID, i, binding.Exchange.String())
				}
				desiredSignalDefinition.Exchange = binding.Exchange
				desiredSignalDefinition.Symbol = binding.Symbol
			case types.SignalScopeStrategy:
				desiredSignalDefinition.Exchange = binding.Exchange
				desiredSignalDefinition.Symbol = binding.Symbol
			}

			var err error
			var spec stypes.SignalSpec
			switch desiredSignalDefinition.Type {
			case types.SignalTypeKline:
				spec, err = ss.CreateKlineSignalSpec(*desiredSignalDefinition, startTs, endTs)
			case types.SignalTypeTimer:
				spec, err = ss.CreateTimerSignalSpec(*desiredSignalDefinition, startTs, endTs)
			default:
				spec = ss.NewCommonSignalSpec(*desiredSignalDefinition, startTs, endTs)
			}
			if err != nil {
				return nil, err
			}

			// 校验数据源
			var ds *types.DataSource
			if binding.DatasourceID > 0 {
				ds, err = e.GetDatasource(ctx, binding.DatasourceID)
				if err != nil {
					return nil, err
				}
				if ds == nil {
					return nil, status.Errorf(codes.InvalidArgument, "datasource not found: %d", binding.DatasourceID)
				}
				if err := e.validateDatasource(spec, ds); err != nil {
					return nil, err
				}
			}

			// 根据 strategy signal 补全 input signal 的参数
			flatSignal := flatSignalDefinition{
				Datasource: ds,
				Spec:       spec,
			}
			flatSignals = append(flatSignals, flatSignal)
		}
	}

	// 根据 spec.GetID() 去重 flatSignals
	// 目前先不考虑 datasource 的差异，直接报错，后续再优化
	for i, sig := range flatSignals {
		for j := i + 1; j < len(flatSignals); j++ {
			if sig.Spec.GetID() == flatSignals[j].Spec.GetID() {
				if sig.Datasource != nil && flatSignals[j].Datasource != nil && sig.Datasource.ID != flatSignals[j].Datasource.ID {
					return nil, status.Errorf(codes.InvalidArgument, "signal %s has multiple datasources: %d and %d", sig.Spec.GetID(), sig.Datasource.ID, flatSignals[j].Datasource.ID)
				}
				flatSignals = append(flatSignals[:j], flatSignals[j+1:]...)
				j--
			}
		}
	}

	// 添加附加数据源
	flatSignals, err := addAdditionalPriceKlineSources(e.cfg.BaseExchange, e.cfg.BaseCurrency, symbols, flatSignals, startTs, endTs)
	if err != nil {
		return nil, err
	}

	// 将 flatSignals 转为 source
	sources := make([]stypes.Source, 0, len(flatSignals))
	for _, sig := range flatSignals {
		source, err := e.createSource(ctx, sig.Spec, sig.Datasource, sig.IsDerived)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}

	return sources, nil
}

// createSourceFromFlatSignal 根据 flatSignalDefinition 创建 source
// - 如果 datasource 不为空，创建 db source
// - 如果 datasource 为空，根据 spec 类型通过 fetcher 创建 source
func (e *Entity) createSource(_ context.Context, signal stypes.SignalSpec, datasource *types.DataSource, isDerived bool) (stypes.Source, error) {
	// 如果 datasource 不为空，创建 db source
	if datasource != nil {
		return sources.NewDbSource(e.db, signal, datasource, isDerived), nil
	}

	// 如果 datasource 为空，根据 spec 类型通过 fetcher 创建 source
	switch signal.GetType() {
	case types.SignalTypeKline:
		// 从 spec 中提取 exchange、symbol、interval
		if signal.GetExchange() == nil || signal.GetSymbol() == nil {
			return nil, status.Errorf(codes.InvalidArgument, "kline signal %s exchange or symbol is nil", signal.GetID())
		}
		klineSpec, ok := signal.(*ss.KlineSignalSpec)
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "signal %s is not a KlineSignalSpec", signal.GetID())
		}
		return sources.NewKlineSource(*klineSpec, isDerived), nil
	case types.SignalTypeTimer:
		// 从 spec 中提取 interval、topic
		timerSpec, ok := signal.(*ss.TimerSignalSpec)
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "signal %s is not a TimerSignalSpec", signal.GetID())
		}
		return sources.NewTimerSource(*timerSpec, isDerived), nil
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported signal type for fetcher source: %s", signal.GetType())
	}
}

// validateDatasource 验证数据源是否匹配信号定义
func (e *Entity) validateDatasource(sig stypes.SignalSpec, ds *types.DataSource) error {
	sigID := sig.GetID()
	if ds.Exchange != nil && sig.GetExchange() != nil && !ds.Exchange.Equal(*sig.GetExchange()) {
		return status.Errorf(codes.InvalidArgument, "kline signal %s datasource exchange mismatch, got %s, expected %s", sigID, *ds.Exchange, sig.GetExchange())
	}
	if ds.Symbol != nil && sig.GetSymbol() != nil && !ds.Symbol.Equal(*sig.GetSymbol()) {
		return status.Errorf(codes.InvalidArgument, "kline signal %s datasource symbol mismatch, got %s, expected %s", sigID, *ds.Symbol, sig.GetSymbol())
	}
	if !sig.MatchProps(ds.Props) {
		return status.Errorf(codes.InvalidArgument, "kline signal %s datasource props mismatch, got %v, expected %v", sigID, ds.Props, sig.GetProps())
	}
	// 校验数据源的 startTs 和 endTs 是否在信号定义的 startTs 和 endTs 之间
	if ds.StartTs.After(sig.GetStartTs()) || ds.EndTs.Before(sig.GetEndTs()) {
		return status.Errorf(codes.InvalidArgument, "kline signal %s datasource startTs or endTs mismatch", sigID)
	}
	return nil
}

func calculateDefaultInterval(duration time.Duration, barLimit int) (ctypes.Interval, error) {
	for _, interval := range ctypes.Intervals() {
		d, err := interval.Duration()
		if err != nil {
			return "", err
		}
		if duration < d*time.Duration(barLimit) {
			return interval, nil
		}
	}
	return ctypes.Interval1M, nil
}

func addAdditionalPriceKlineSources(defaultExchange ctypes.Exchange, baseCurrency string, symbols []*stypes.BacktestSymbol, flatSignals []flatSignalDefinition, startTs time.Time, endTs time.Time) ([]flatSignalDefinition, error) {
	// 现货交易对集合
	exSymbolSet := make(map[ctypes.ExSymbolKey]ctypes.ExSymbol)
	for _, symbol := range symbols {
		if symbol == nil {
			continue
		}
		if symbol.Symbol.Quote == baseCurrency {
			exSymbol := ctypes.NewExSymbol(defaultExchange, ctypes.NewSymbol(symbol.Symbol.Base, symbol.Symbol.Quote, ctypes.MarketTypeSpot))
			exSymbolSet[exSymbol.Key()] = exSymbol
		} else {
			baseExSymbol := ctypes.NewExSymbol(defaultExchange, ctypes.NewSymbol(symbol.Symbol.Base, baseCurrency, ctypes.MarketTypeSpot))
			quoteExSymbol := ctypes.NewExSymbol(defaultExchange, ctypes.NewSymbol(symbol.Symbol.Quote, baseCurrency, ctypes.MarketTypeSpot))
			exSymbolSet[baseExSymbol.Key()] = baseExSymbol
			exSymbolSet[quoteExSymbol.Key()] = quoteExSymbol
		}
	}

	// 根据 时间跨度 计算默认 interval
	defaultInterval, err := calculateDefaultInterval(endTs.Sub(startTs), 1000)
	if err != nil {
		return nil, err
	}

	// 遍历已有的 flatSignals, 以 symbol 为维度，收集所有 symbol 的最小 interval，用于后续创建对应的数据源
	symbolMinInterval := make(map[ctypes.ExSymbolKey]ctypes.Interval)
	for _, sig := range flatSignals {
		if !sig.Spec.GetType().IsMarketSignal() {
			continue
		}
		if sig.Spec.GetExchange() == nil || sig.Spec.GetSymbol() == nil {
			return nil, status.Errorf(codes.InvalidArgument, "signal %s exchange or symbol is nil", sig.Spec.GetID())
		}

		var interval *ctypes.Interval
		switch sig.Spec.GetType() {
		case types.SignalTypeKline:
			klineSpec := sig.Spec.(*ss.KlineSignalSpec)
			interval = &klineSpec.Interval
		}
		if interval == nil {
			continue
		}

		// 构建需要添加的交易对
		symbol := *sig.Spec.GetSymbol()
		exSymbols := make([]ctypes.ExSymbol, 0)
		if symbol.Quote == baseCurrency {
			exSymbol := ctypes.NewExSymbol(defaultExchange, ctypes.NewSymbol(symbol.Base, symbol.Quote, ctypes.MarketTypeSpot))
			exSymbols = append(exSymbols, exSymbol)
		} else {
			baseExSymbol := ctypes.NewExSymbol(defaultExchange, ctypes.NewSymbol(symbol.Base, baseCurrency, ctypes.MarketTypeSpot))
			quoteExSymbol := ctypes.NewExSymbol(defaultExchange, ctypes.NewSymbol(symbol.Quote, baseCurrency, ctypes.MarketTypeSpot))
			exSymbols = append(exSymbols, baseExSymbol, quoteExSymbol)
		}

		for _, exSymbol := range exSymbols {
			if _, ok := symbolMinInterval[exSymbol.Key()]; !ok {
				symbolMinInterval[exSymbol.Key()] = *interval
			} else {
				d1, err := interval.Duration()
				if err != nil {
					return nil, err
				}
				d2, err := symbolMinInterval[exSymbol.Key()].Duration()
				if err != nil {
					return nil, err
				}
				if d1 < d2 {
					symbolMinInterval[exSymbol.Key()] = *interval
				}
			}
		}
	}

	for exSymbolKey, exSymbol := range exSymbolSet {
		var interval ctypes.Interval
		if _, ok := symbolMinInterval[exSymbolKey]; ok {
			interval = symbolMinInterval[exSymbolKey]
		} else {
			interval = defaultInterval
		}
		spec, err := ss.CreateKlineSignalSpec(stypes.SignalDefinition{
			ID:       "-1",
			Type:     types.SignalTypeKline,
			Scope:    types.SignalScopeTarget,
			Exchange: &exSymbol.Exchange,
			Symbol:   &exSymbol.Symbol,
			Props: map[string]any{
				"interval": interval.String(),
			},
		}, startTs, endTs)
		if err != nil {
			return nil, err
		}
		exists := false
		for _, sig := range flatSignals {
			if sig.Spec.GetID() == spec.GetID() {
				exists = true
				break
			}
		}
		if !exists {
			flatSignals = append(flatSignals, flatSignalDefinition{Spec: spec, IsDerived: true})
		}
	}
	return flatSignals, nil
}

type flatSignalDefinition struct {
	Spec       stypes.SignalSpec
	Datasource *types.DataSource
	IsDerived  bool
}
