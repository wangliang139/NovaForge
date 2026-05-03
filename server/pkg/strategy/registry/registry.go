package registry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	converter "github.com/wangliang139/NovaForge/server/pkg/converter"
	"github.com/wangliang139/NovaForge/server/pkg/internal/chsdk"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	"github.com/wangliang139/NovaForge/server/pkg/strategy"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/executor/live"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/marketdata"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/proxy"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/pubsub"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/signal"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
)

// ExecutorRegistry 执行器注册表
type ExecutorRegistry struct {
	mu sync.RWMutex

	cfg        Config
	db         *repos.Entity
	chClient   *chsdk.Client
	dispatcher *pubsub.Dispatcher

	// 监控退出控制
	monitorCancel context.CancelFunc

	marketProvider marketdata.MarketProvider
	accountEngine  strategy.AccountEngine
	orderEngine    strategy.OrderEngine

	// 运行中的执行器
	executors map[int32]strategy.Executor // botID -> executor
	starting  map[int32]struct{}          // botID -> starting marker

	// 订阅管理
	streamSubscriptions map[string]*streamSubscription // streamKey -> subscription
	streamBots          map[string]map[int32]struct{}  // streamKey -> botIDs
	botStreams          map[int32]map[string]struct{}  // botID -> streamKeys
	// timer 订阅：每个 bot 一个 cancel，用于停止该 bot 下所有 timer 协程
	botTimerCancel map[int32]context.CancelFunc
}

// Config 注册表配置
type Config struct {
	BaseCurrency string
	BaseExchange string
	// MonitorInterval 控制健康检查轮询周期，为 0 时使用默认值 30s。
	MonitorInterval time.Duration
	// SignalAlarmTimeout 控制“长时间未收到信号”的判定阈值，为 0 时使用默认值 5 分钟。
	SignalAlarmTimeout time.Duration
	// SignalShutdownTimeout 控制“长时间未收到信号”的判定阈值，为 0 时使用默认值 30 分钟。
	SignalShutdownTimeout time.Duration
	EnableAlarm           bool
}

// NewExecutorRegistry 创建执行器注册表
func NewExecutorRegistry(
	cfg Config,
	db *repos.Entity,
	dispatcher *pubsub.Dispatcher,
	accountEngine strategy.AccountEngine,
	orderEngine strategy.OrderEngine,
	marketProvider marketdata.MarketProvider,
) (*ExecutorRegistry, error) {
	if accountEngine == nil {
		return nil, fmt.Errorf("account engine is nil")
	}
	if orderEngine == nil {
		return nil, fmt.Errorf("order engine is nil")
	}
	if marketProvider == nil {
		return nil, fmt.Errorf("market provider is nil")
	}
	if cfg.BaseCurrency == "" {
		cfg.BaseCurrency = "USDT"
	}
	if cfg.BaseExchange == "" {
		cfg.BaseExchange = "binance"
	}
	if cfg.MonitorInterval <= 0 {
		cfg.MonitorInterval = 30 * time.Second
	}
	if cfg.SignalAlarmTimeout <= 0 {
		cfg.SignalAlarmTimeout = 5 * time.Minute
	}
	if cfg.SignalShutdownTimeout <= 0 {
		cfg.SignalShutdownTimeout = 30 * time.Minute
	}

	chClient, err := chsdk.Connect(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to create clickhouse client: %w", err)
	}

	return &ExecutorRegistry{
		cfg:                 cfg,
		db:                  db,
		chClient:            chClient,
		dispatcher:          dispatcher,
		executors:           make(map[int32]strategy.Executor),
		accountEngine:       accountEngine,
		orderEngine:         orderEngine,
		marketProvider:      marketProvider,
		starting:            make(map[int32]struct{}),
		streamSubscriptions: make(map[string]*streamSubscription),
		streamBots:          make(map[string]map[int32]struct{}),
		botStreams:          make(map[int32]map[string]struct{}),
		botTimerCancel:      make(map[int32]context.CancelFunc),
	}, nil
}

// StartMonitor 启动后台健康检查协程
func (r *ExecutorRegistry) StartMonitor(ctx context.Context) {
	r.mu.Lock()
	// 已经启动过则直接返回
	if r.monitorCancel != nil {
		r.mu.Unlock()
		return
	}
	monitorCtx, cancel := context.WithCancel(ctx)
	r.monitorCancel = cancel
	interval := r.cfg.MonitorInterval
	r.mu.Unlock()

	if interval <= 0 {
		interval = 30 * time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-monitorCtx.Done():
				return
			case <-ticker.C:
				r.checkExecutorsHealth(monitorCtx)
			}
		}
	}()
}

// Start 启动 Bot（创建并启动执行器）
func (r *ExecutorRegistry) Start(ctx context.Context, bot *stypes.Bot, stg *stypes.Strategy) error {
	if bot == nil {
		return fmt.Errorf("bot is nil")
	}
	if stg == nil {
		return fmt.Errorf("strategy is nil")
	}

	r.mu.Lock()
	if _, exists := r.executors[bot.ID]; exists {
		r.mu.Unlock()
		return stypes.ErrBotAlreadyRunning
	}
	if _, exists := r.starting[bot.ID]; exists {
		r.mu.Unlock()
		return stypes.ErrBotAlreadyStarting
	}
	r.starting[bot.ID] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.starting, bot.ID)
		r.mu.Unlock()
	}()

	// 根据 bot.Mode 创建对应的执行器
	executor, err := r.newExecutor(bot, stg)
	if err != nil {
		return err
	}

	// 启动执行器
	if err = executor.Start(ctx); err != nil {
		_ = executor.Stop(ctx)
		return fmt.Errorf("failed to start executor: %w", err)
	}

	// 注册订阅与信号路由
	if err := r.ensureBotSubscriptions(ctx, bot, stg, executor.GetSignalChannel()); err != nil {
		_ = executor.Stop(ctx)
		return fmt.Errorf("failed to ensure bot subscriptions: %w", err)
	}

	r.mu.Lock()
	if _, exists := r.executors[bot.ID]; exists {
		r.mu.Unlock()
		r.removeBotSubscriptions(ctx, bot.ID)
		if stopErr := executor.Stop(ctx); stopErr != nil {
			logger.Ctx(ctx).Warn().Err(stopErr).Int32("bot_id", bot.ID).Msg("failed to stop executor after duplicate start")
		}
		return errors.New("bot is already running")
	}
	r.executors[bot.ID] = executor
	r.mu.Unlock()

	logger.Ctx(ctx).Info().
		Int32("bot_id", bot.ID).
		Str("mode", string(bot.Mode)).
		Msg("executor started")

	return nil
}

// newExecutor 根据 bot.Mode 创建对应类型的执行器（Paper/Live）
func (r *ExecutorRegistry) newExecutor(bot *stypes.Bot, stg *stypes.Strategy) (strategy.Executor, error) {
	baseExchange, err := ctypes.ParseExchange(r.cfg.BaseExchange)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base exchange: %w", err)
	}
	switch bot.Mode {
	case stypes.BotModePaper, stypes.BotModeLive:
		cfg := live.LiveExecutorConfig{
			DB:           r.db,
			ChClient:     r.chClient,
			Bot:          bot,
			Strategy:     stg,
			BaseCurrency: r.cfg.BaseCurrency,
			BaseExchange: baseExchange,
		}
		exec, err := live.NewLiveExecutor(cfg, r.marketProvider, r.accountEngine, r.orderEngine)
		if err != nil {
			return nil, fmt.Errorf("failed to create live executor: %w", err)
		}
		return exec, nil
	default:
		return nil, fmt.Errorf("unsupported bot mode: %s", bot.Mode)
	}
}

// Stop 停止 Bot（停止并移除执行器）
func (r *ExecutorRegistry) Stop(ctx context.Context, botID int32) error {
	r.mu.Lock()
	executor, exists := r.executors[botID]
	if !exists {
		r.mu.Unlock()
		return fmt.Errorf("bot %d is not running", botID)
	}
	delete(r.executors, botID)
	r.mu.Unlock()

	// 取消订阅与路由
	r.removeBotSubscriptions(ctx, botID)

	// 停止执行器
	if err := executor.Stop(ctx); err != nil {
		logger.Ctx(ctx).Warn().Err(err).Int32("bot_id", botID).Msg("failed to stop executor")
	}

	logger.Ctx(ctx).Info().Int32("bot_id", botID).Msg("executor stopped")

	return nil
}

// Get 获取执行器
func (r *ExecutorRegistry) Get(botID int32) (strategy.Executor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	executor, exists := r.executors[botID]
	return executor, exists
}

// List 列出所有运行中的执行器
func (r *ExecutorRegistry) List() []strategy.Executor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	executors := make([]strategy.Executor, 0, len(r.executors))
	for _, executor := range r.executors {
		executors = append(executors, executor)
	}
	return executors
}

// StopAll 停止所有执行器
func (r *ExecutorRegistry) StopAll(ctx context.Context) {
	r.mu.Lock()
	botIDs := make([]int32, 0, len(r.executors))
	for botID := range r.executors {
		botIDs = append(botIDs, botID)
	}
	r.mu.Unlock()

	for _, botID := range botIDs {
		if err := r.Stop(ctx, botID); err != nil {
			logger.Ctx(ctx).Warn().Err(err).Int32("bot_id", botID).Msg("failed to stop executor")
		}
	}
}

func (r *ExecutorRegistry) Close(ctx context.Context) error {
	// 先停止健康检查循环
	r.mu.Lock()
	if r.monitorCancel != nil {
		r.monitorCancel()
		r.monitorCancel = nil
	}
	r.mu.Unlock()

	r.StopAll(ctx)
	r.ReleaseAllSubscriptions(ctx)
	if err := r.chClient.Close(); err != nil {
		logger.Ctx(ctx).Warn().Err(err).Msg("failed to close clickhouse client")
	}
	logger.Ctx(ctx).Info().Msg("executor registry closed")
	return nil
}

type streamSubscription struct {
	exchange     ctypes.Exchange
	selector     ctypes.StreamSelector
	streamKey    string
	streamCancel context.CancelFunc
	botSignalChs map[int32]chan stypes.Signal
}

type streamSpec struct {
	exchange ctypes.Exchange
	selector ctypes.StreamSelector
}

// timerSpec 描述策略中一个 timer 信号的订阅参数（interval + topic），用于实盘/模拟盘按间隔向 signalCh 发送 TimerEvent。
type timerSpec struct {
	Interval time.Duration
	Topic    string
	SignalID string
}

func (r *ExecutorRegistry) ensureBotSubscriptions(ctx context.Context, bot *stypes.Bot, strategy *stypes.Strategy, signalCh chan stypes.Signal) error {
	specs, err := r.buildStreamSelectors(ctx, bot, strategy)
	if err != nil {
		return err
	}

	// 运行态需要账户事件驱动订单/资产/仓位状态；live 与 paper 都要补订账户级事件。
	if (bot.Mode == stypes.BotModeLive || bot.Mode == stypes.BotModePaper) && bot.AccountID != "" {
		accountSelector := ctypes.StreamSelector{
			Stream:  ctypes.StreamTypeAccount,
			Account: &bot.AccountID,
		}
		if err := accountSelector.Validate(); err == nil {
			accountSpec := streamSpec{
				exchange: bot.Exchange,
				selector: accountSelector,
			}
			specs = append(specs, accountSpec)
		}
	}

	added := make([]string, 0, len(specs))

	defer func() {
		if err != nil {
			for i := len(added) - 1; i >= 0; i-- {
				r.removeBotStream(ctx, bot.ID, added[i])
			}
		}
	}()
	for _, spec := range specs {
		key := ctypes.StreamKey(spec.exchange, spec.selector)
		if err := r.ensureStreamSubscription(ctx, bot.ID, spec, signalCh); err != nil {
			return err
		}
		added = append(added, key)
	}
	if err := r.ensureTimerSubscriptions(ctx, bot, strategy, signalCh); err != nil {
		return err
	}
	return nil
}

// buildSignalDefinitionMap 从策略中构建 SignalID -> SignalDefinition 映射，供 buildStreamSelectors/buildTimerSpecs 复用。
func buildSignalDefinitionMap(strategy *stypes.Strategy) map[string]stypes.SignalDefinition {
	definitions := make(map[string]stypes.SignalDefinition, len(strategy.Signals))
	for _, sig := range strategy.Signals {
		definitions[sig.ID] = sig
	}
	return definitions
}

func (r *ExecutorRegistry) buildStreamSelectors(ctx context.Context, bot *stypes.Bot, strategy *stypes.Strategy) ([]streamSpec, error) {
	if bot == nil || strategy == nil {
		return nil, fmt.Errorf("bot or strategy is nil")
	}
	definitions := buildSignalDefinitionMap(strategy)
	result := make(map[string]streamSpec)
	for _, binding := range bot.Config.Signals {
		def, ok := definitions[binding.SignalID]
		if !ok {
			return nil, fmt.Errorf("signal definition not found: %s", binding.SignalID)
		}

		streamType, ok := converter.SignalType2StreamType(def.Type)
		if !ok {
			logger.Ctx(ctx).Warn().Str("signal_id", def.ID).Str("type", def.Type.String()).Msg("skip unsupported signal type for subscription")
			continue
		}

		exchange := bot.Exchange
		if binding.Exchange != nil && binding.Exchange.IsValid() {
			exchange = *binding.Exchange
		}
		if !exchange.IsValid() {
			continue
		}
		symbol := binding.Symbol

		selector := ctypes.StreamSelector{Stream: streamType}
		if streamType.IsMarketSignal() {
			if symbol == nil || !symbol.IsValid() {
				continue
			}
			selector.Symbol = symbol
		}
		if streamType == ctypes.StreamTypeKline {
			intervalVal, ok := def.Props["interval"]
			if !ok {
				return nil, fmt.Errorf("signal %s props.interval is required", def.ID)
			}
			intervalStr, err := converter.AnyToString(intervalVal)
			if err != nil {
				return nil, fmt.Errorf("signal %s props.interval is invalid: %w", def.ID, err)
			}
			interval := ctypes.Interval(intervalStr)
			if !interval.Valid() {
				return nil, fmt.Errorf("signal %s props.interval is invalid: %s", def.ID, intervalStr)
			}
			selector.Interval = &interval
		}
		if streamType == ctypes.StreamTypeAccount {
			selector.Account = &bot.AccountID
		}
		if err := selector.Validate(); err != nil {
			return nil, fmt.Errorf("signal %s selector invalid: %w", def.ID, err)
		}

		key := ctypes.StreamKey(exchange, selector)
		result[key] = streamSpec{exchange: exchange, selector: selector}

		// 针对合约 symbol，自动订阅 mark_price 事件（用于风控/仓位等逻辑）
		if symbol != nil && symbol.IsValid() && symbol.Type == ctypes.MarketTypeFuture {
			mpSelector := ctypes.StreamSelector{
				Stream: ctypes.StreamTypeMarkPrice,
				Symbol: symbol,
			}
			if err := mpSelector.Validate(); err != nil {
				return nil, fmt.Errorf("signal %s mark_price selector invalid: %w", def.ID, err)
			}
			mpKey := ctypes.StreamKey(exchange, mpSelector)
			result[mpKey] = streamSpec{exchange: exchange, selector: mpSelector}
		}
	}

	out := make([]streamSpec, 0, len(result))
	for _, spec := range result {
		out = append(out, spec)
	}
	return out, nil
}

// buildTimerSpecs 从策略与 bot 绑定中解析出 timer 信号规格（仅包含 type=timer 且 bot 已绑定的信号），用于实盘/模拟盘定时向 signalCh 发送 TimerEvent。
func (r *ExecutorRegistry) buildTimerSpecs(_ context.Context, bot *stypes.Bot, strategy *stypes.Strategy) ([]timerSpec, error) {
	if bot == nil || strategy == nil {
		return nil, fmt.Errorf("bot or strategy is nil")
	}
	definitions := buildSignalDefinitionMap(strategy)
	seen := make(map[string]struct{})
	out := make([]timerSpec, 0)
	for _, binding := range bot.Config.Signals {
		def, ok := definitions[binding.SignalID]
		if !ok {
			return nil, fmt.Errorf("signal definition not found: %s", binding.SignalID)
		}
		if def.Type != stypes.SignalTypeTimer {
			continue
		}
		if _, ok := def.Props["interval"]; !ok {
			return nil, fmt.Errorf("signal %s props.interval is required", def.ID)
		}
		intervalMs, err := converter.AnyToInt(def.Props["interval"])
		if err != nil {
			return nil, fmt.Errorf("signal %s props.interval is invalid: %w", def.ID, err)
		}
		if _, ok := def.Props["topic"]; !ok {
			return nil, fmt.Errorf("signal %s props.topic is required", def.ID)
		}
		topic, err := converter.AnyToString(def.Props["topic"])
		if err != nil {
			return nil, fmt.Errorf("signal %s props.topic is invalid: %w", def.ID, err)
		}
		interval := time.Duration(intervalMs) * time.Millisecond
		if interval <= 0 {
			return nil, fmt.Errorf("signal %s props.interval must be positive", def.ID)
		}
		key := fmt.Sprintf("%s:%s:%s", def.ID, topic, interval)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, timerSpec{Interval: interval, Topic: topic, SignalID: def.ID})
	}
	return out, nil
}

func (r *ExecutorRegistry) ensureTimerSubscriptions(ctx context.Context, bot *stypes.Bot, strategy *stypes.Strategy, signalCh chan stypes.Signal) error {
	specs, err := r.buildTimerSpecs(ctx, bot, strategy)
	if err != nil {
		return err
	}
	if len(specs) == 0 {
		return nil
	}
	timerCtx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	if prev, exists := r.botTimerCancel[bot.ID]; exists {
		prev()
	}
	r.botTimerCancel[bot.ID] = cancel
	r.mu.Unlock()

	for _, spec := range specs {
		go func(spec timerSpec) {
			ticker := time.NewTicker(spec.Interval)
			defer ticker.Stop()
			for {
				select {
				case <-timerCtx.Done():
					return
				case t := <-ticker.C:
					topic := spec.Topic
					ev := &stypes.TimerSignal{
						BaseSignal: stypes.BaseSignal{
							ID:    fmt.Sprintf("timer-%s-%d", spec.SignalID, t.UnixNano()),
							Ts:    t,
							Topic: &topic,
						},
						Time: t,
					}
					select {
					case signalCh <- ev:
					default:
						logger.Ctx(ctx).Warn().Str("topic", spec.Topic).Msg("timer signal channel full, dropping")
					}
				}
			}
		}(spec)
	}
	return nil
}

func (r *ExecutorRegistry) ensureStreamSubscription(ctx context.Context, botID int32, spec streamSpec, signalCh chan stypes.Signal) error {
	key := ctypes.StreamKey(spec.exchange, spec.selector)
	needSubscribe := false

	r.mu.Lock()
	if r.streamBots[key] == nil {
		r.streamBots[key] = make(map[int32]struct{})
	}
	if _, exists := r.streamBots[key][botID]; exists {
		r.mu.Unlock()
		return nil
	}
	r.streamBots[key][botID] = struct{}{}
	if r.botStreams[botID] == nil {
		r.botStreams[botID] = make(map[string]struct{})
	}
	r.botStreams[botID][key] = struct{}{}
	if _, exists := r.streamSubscriptions[key]; !exists {
		r.streamSubscriptions[key] = &streamSubscription{
			exchange:     spec.exchange,
			selector:     spec.selector,
			botSignalChs: make(map[int32]chan stypes.Signal),
		}
		needSubscribe = true
	}
	if sub, ok := r.streamSubscriptions[key]; ok {
		sub.botSignalChs[botID] = signalCh
	}
	r.mu.Unlock()

	if r.dispatcher != nil {
		r.dispatcher.RegisterStreamHandler(key, botID, signalCh)
	}

	if !needSubscribe {
		return nil
	}

	_, err := r.ensureDataSubscription(ctx, key, spec.exchange, spec.selector)
	if err != nil {
		r.removeBotStream(ctx, botID, key)
		return err
	}

	return nil
}

func (r *ExecutorRegistry) removeBotSubscriptions(ctx context.Context, botID int32) {
	r.mu.RLock()
	streams := r.botStreams[botID]
	keys := make([]string, 0, len(streams))
	for key := range streams {
		keys = append(keys, key)
	}
	r.mu.RUnlock()

	for _, key := range keys {
		r.removeBotStream(ctx, botID, key)
	}

	r.mu.Lock()
	if cancel, exists := r.botTimerCancel[botID]; exists {
		cancel()
		delete(r.botTimerCancel, botID)
	}
	r.mu.Unlock()
}

func (r *ExecutorRegistry) removeBotStream(_ context.Context, botID int32, streamKey string) {
	var streamCancel context.CancelFunc

	r.mu.Lock()
	if bots, ok := r.streamBots[streamKey]; ok {
		delete(bots, botID)
		if len(bots) == 0 {
			delete(r.streamBots, streamKey)
			if sub, exists := r.streamSubscriptions[streamKey]; exists {
				if sub.streamCancel != nil {
					streamCancel = sub.streamCancel
				}
			}
			delete(r.streamSubscriptions, streamKey)
		} else {
			if sub, exists := r.streamSubscriptions[streamKey]; exists && sub.botSignalChs != nil {
				delete(sub.botSignalChs, botID)
			}
		}
	}
	if streams, ok := r.botStreams[botID]; ok {
		delete(streams, streamKey)
		if len(streams) == 0 {
			delete(r.botStreams, botID)
		}
	}
	r.mu.Unlock()

	if r.dispatcher != nil {
		r.dispatcher.UnregisterStreamHandler(streamKey, botID)
	}
	if streamCancel != nil {
		streamCancel()
	}
}

func (r *ExecutorRegistry) ReleaseAllSubscriptions(ctx context.Context) {
	r.mu.Lock()
	subscriptions := make([]*streamSubscription, 0, len(r.streamSubscriptions))
	for _, sub := range r.streamSubscriptions {
		subscriptions = append(subscriptions, sub)
	}
	timerCancels := make([]context.CancelFunc, 0, len(r.botTimerCancel))
	for _, cancel := range r.botTimerCancel {
		timerCancels = append(timerCancels, cancel)
	}
	r.streamSubscriptions = make(map[string]*streamSubscription)
	r.streamBots = make(map[string]map[int32]struct{})
	r.botStreams = make(map[int32]map[string]struct{})
	r.botTimerCancel = make(map[int32]context.CancelFunc)
	r.mu.Unlock()

	for _, cancel := range timerCancels {
		cancel()
	}
	for _, sub := range subscriptions {
		if sub == nil {
			continue
		}
		if sub.streamCancel != nil {
			sub.streamCancel()
		}
	}
}

func (r *ExecutorRegistry) ensureDataSubscription(_ context.Context, streamKey string, exchange ctypes.Exchange, selector ctypes.StreamSelector) (string, error) {
	topic := ctypes.TopicName(exchange, selector)
	streamCtx, streamCancel := context.WithCancel(context.Background())

	r.mu.Lock()
	if sub, ok := r.streamSubscriptions[streamKey]; ok {
		sub.streamCancel = streamCancel
	} else {
		streamCancel()
	}
	r.mu.Unlock()

	go r.runStreamSubscription(streamCtx, exchange, selector, streamKey)

	return topic, nil
}

func (r *ExecutorRegistry) runStreamSubscription(ctx context.Context, exchange ctypes.Exchange, selector ctypes.StreamSelector, streamKey string) {
	retryDelay := time.Second
	maxRetryDelay := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		ch, err := r.createStreamSubscription(ctx, exchange, selector)
		if err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("streamKey", streamKey).Msg("create stream subscription failed, will retry")
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryDelay):
			}
			retryDelay = min(retryDelay*2, maxRetryDelay)
			continue
		}

		retryDelay = time.Second

		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-ch:
				if !ok {
					return
				}
				env := resp.Envelope
				if env == nil {
					logger.Ctx(ctx).Error().Msg("empty envelope")
					continue
				}
				sigs, err := signal.Envelope2Signals(*env)
				if err != nil {
					logger.Ctx(ctx).Error().Err(err).Str("streamKey", streamKey).Msg("failed to convert envelope to signals")
					continue
				}
				for _, sig := range sigs {
					if sig == nil {
						continue
					}
					select {
					case r.dispatcher.GetSignalChannel() <- sig:
					default:
						logger.Ctx(ctx).Warn().Str("streamKey", streamKey).Msg("signal channel full, dropping")
					}
				}
			}
		}
	}
}

func (r *ExecutorRegistry) createStreamSubscription(ctx context.Context, exchange ctypes.Exchange, selector ctypes.StreamSelector) (<-chan *ctypes.SubscribeStreamResponse, error) {
	req := &ctypes.SubscribeStreamRequest{
		StreamType: selector.Stream,
		Exchange:   &exchange,
	}
	if selector.Symbol != nil {
		req.Symbol = selector.Symbol.String()
	}
	if selector.Interval != nil {
		req.Interval = selector.Interval
	}
	if selector.Account != nil {
		req.AccountId = selector.Account
	}

	stream, err := proxy.SubscribeStream(ctx, req)
	if err != nil {
		return nil, err
	}

	logger.Ctx(ctx).Info().
		Str("exchange", exchange.String()).
		Str("stream", selector.Stream.String()).
		Msg("stream subscription connected")

	return stream, nil
}
