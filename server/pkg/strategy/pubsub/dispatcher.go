package pubsub

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type Handler interface {
	OnEvent(ctx context.Context, signal stypes.Signal) error
}

// BackpressureConfig 分发背压相关配置
type BackpressureConfig struct {
	// WeakBackpressureEnabled 是否对弱一致信号启用背压丢弃/降采样逻辑
	WeakBackpressureEnabled bool
	// PerStreamBackpressureThreshold 单个 streamKey 连续发送失败（channel full）次数阈值
	// 到达阈值后，对该 streamKey 的弱一致信号优先丢弃，避免进一步加重阻塞
	PerStreamBackpressureThreshold int
	// WeakSignalMinInterval 同一 streamKey 的弱一致信号最小通过时间间隔（0 表示不做时间降采样）
	WeakSignalMinInterval time.Duration
}

func defaultBackpressureConfig() BackpressureConfig {
	return BackpressureConfig{
		WeakBackpressureEnabled:        true,
		PerStreamBackpressureThreshold: 100,
		WeakSignalMinInterval:          0,
	}
}

// Dispatcher 信号分发器
type Dispatcher struct {
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	subscriber     Subscriber
	recorder       SignalRecorder
	handlers       []Handler
	streamHandlers map[string]map[int32]chan stypes.Signal // streamKey -> botID -> channel
	signalCh       chan stypes.Signal
	started        bool
	closeOnce      sync.Once
	closed         bool

	// 背压相关状态，仅由 dispatch goroutine 访问，无需额外锁
	bpCfg              BackpressureConfig
	perStreamFailCount map[string]int       // streamKey -> 连续发送失败次数
	weakLastPassTs     map[string]time.Time // streamKey -> 最近一次成功投递弱一致信号的时间
}

type SignalRecorder interface {
	Record(botID int32, signal stypes.Signal)
	Close() error
}

// NewDispatcher 创建信号分发器（使用默认背压配置）
func NewDispatcher(subscriber Subscriber) *Dispatcher {
	return NewDispatcherWithConfig(subscriber, defaultBackpressureConfig())
}

// NewDispatcherWithConfig 创建带自定义背压配置的分发器
func NewDispatcherWithConfig(subscriber Subscriber, cfg BackpressureConfig) *Dispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &Dispatcher{
		ctx:                ctx,
		cancel:             cancel,
		streamHandlers:     make(map[string]map[int32]chan stypes.Signal),
		subscriber:         subscriber,
		bpCfg:              cfg,
		perStreamFailCount: make(map[string]int),
		weakLastPassTs:     make(map[string]time.Time),
	}
}

func (d *Dispatcher) SetSignalRecorder(recorder SignalRecorder) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.recorder = recorder
}

func (d *Dispatcher) AddHandler(handler Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers = append(d.handlers, handler)
}

// RegisterStreamHandler 注册信号处理器（按 streamKey）
func (d *Dispatcher) RegisterStreamHandler(streamKey string, botID int32, signalCh chan stypes.Signal) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.streamHandlers[streamKey] == nil {
		d.streamHandlers[streamKey] = make(map[int32]chan stypes.Signal)
	}
	// 一个 bot 只维护一个 channel，后注册的覆盖之前的
	d.streamHandlers[streamKey][botID] = signalCh
}

// UnregisterStreamHandler 取消注册信号处理器（按 streamKey）
func (d *Dispatcher) UnregisterStreamHandler(streamKey string, botID int32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.streamHandlers[streamKey] == nil {
		return
	}
	delete(d.streamHandlers[streamKey], botID)
	if len(d.streamHandlers[streamKey]) == 0 {
		delete(d.streamHandlers, streamKey)
	}
}

func (d *Dispatcher) GetSignalChannel() chan stypes.Signal {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.signalCh
}

// Start 启动分发器
func (d *Dispatcher) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.started {
		return nil
	}

	d.signalCh = make(chan stypes.Signal, 1024)
	if d.subscriber != nil {
		err := d.subscriber.Subscribe(d.signalCh)
		if err != nil {
			return err
		}
	}

	d.started = true

	// 启动分发goroutine
	go d.dispatch()

	return nil
}

// dispatch 分发信号到所有注册的处理器
func (d *Dispatcher) dispatch() {
	for {
		select {
		case <-d.ctx.Done():
			return
		case signal, ok := <-d.signalCh:
			if !ok {
				return
			}
			if signal == nil {
				continue
			}

			// 全局 handler 直接消费，不参与背压控制（由各自实现负责）
			for _, handler := range d.handlers {
				_ = handler.OnEvent(d.ctx, signal)
			}

			streamKey, ok := streamKeyFromSignal(signal)
			if !ok {
				log.Warn().Interface("signal", signal).Msg("failed to get stream key from signal")
				continue
			}
			consistency := stypes.GetSignalConsistencyBySignal(signal)

			// 对弱一致信号在进入具体 bot 通道前先做一次背压判定/降采样
			if d.shouldDropBeforeDispatch(streamKey, consistency) {
				log.Debug().
					Str("streamKey", streamKey).
					Str("signalType", string(signal.GetType())).
					Int("consistency", int(consistency)).
					Msg("dropping weak-consistency signal due to backpressure")
				continue
			}

			d.mu.RLock()
			handlerMap := d.streamHandlers[streamKey]
			d.mu.RUnlock()

			now := time.Now()

			for botID, handlerCh := range handlerMap {
				if d.recorder != nil {
					// recorder 仅在实际投递前调用，避免对已被背压丢弃的信号做多余记录
					d.recorder.Record(botID, signal)
				}
				select {
				case handlerCh <- signal:
					d.onDispatchSuccess(streamKey, consistency, now)
				default:
					d.onDispatchFailure(streamKey, consistency)
					log.Warn().
						Str("streamKey", streamKey).
						Str("signalType", string(signal.GetType())).
						Int("consistency", int(consistency)).
						Msg("handler channel full, dropping signal")
				}
			}

		}
	}
}

// shouldDropBeforeDispatch 判断在进入具体 handler 通道前是否需要直接丢弃弱一致信号
func (d *Dispatcher) shouldDropBeforeDispatch(streamKey string, consistency stypes.SignalConsistency) bool {
	cfg := d.bpCfg
	if !cfg.WeakBackpressureEnabled {
		return false
	}
	if consistency != stypes.ConsistencyWeak {
		return false
	}

	// 基于连续失败次数的背压阈值
	if cfg.PerStreamBackpressureThreshold > 0 {
		if d.perStreamFailCount[streamKey] >= cfg.PerStreamBackpressureThreshold {
			return true
		}
	}

	// 基于时间的简单降采样：同一 streamKey 的弱一致信号在最小间隔内只允许通过一次
	if cfg.WeakSignalMinInterval > 0 {
		if last, ok := d.weakLastPassTs[streamKey]; ok {
			if time.Since(last) < cfg.WeakSignalMinInterval {
				return true
			}
		}
	}

	return false
}

// onDispatchSuccess 记录成功投递后的背压状态
func (d *Dispatcher) onDispatchSuccess(streamKey string, consistency stypes.SignalConsistency, now time.Time) {
	if consistency == stypes.ConsistencyWeak {
		d.weakLastPassTs[streamKey] = now
	}
	// 无论强/弱一致，成功投递后都可以重置该 stream 的失败计数
	delete(d.perStreamFailCount, streamKey)
}

// onDispatchFailure 记录投递失败（channel full）后的背压状态
func (d *Dispatcher) onDispatchFailure(streamKey string, consistency stypes.SignalConsistency) {
	// 失败计数主要用于弱一致信号的背压判定，但统计对所有类型开放，便于后续扩展
	d.perStreamFailCount[streamKey] = d.perStreamFailCount[streamKey] + 1
	if consistency != stypes.ConsistencyWeak {
		return
	}
}

// Close 关闭分发器
func (d *Dispatcher) Close() error {
	var err error
	d.closeOnce.Do(func() {
		d.mu.Lock()
		defer d.mu.Unlock()

		d.closed = true

		d.cancel()
		if d.subscriber != nil {
			if closeErr := d.subscriber.Close(); closeErr != nil {
				log.Warn().Err(closeErr).Msg("failed to close subscriber")
				if err == nil {
					err = closeErr
				}
			}
		}
		if d.recorder != nil {
			if closeErr := d.recorder.Close(); closeErr != nil {
				log.Warn().Err(closeErr).Msg("failed to close signal recorder")
				if err == nil {
					err = closeErr
				}
			}
		}

		// 关闭所有handler channels（只关闭一次）
		closedChannels := make(map[chan stypes.Signal]bool)
		for _, handlerMap := range d.streamHandlers {
			for _, ch := range handlerMap {
				if ch != nil && !closedChannels[ch] {
					closedChannels[ch] = true
					select {
					case <-ch:
					default:
						close(ch)
					}
				}
			}
		}
	})

	return err
}

func streamKeyFromSignal(sig stypes.Signal) (string, bool) {
	if sig == nil {
		return "", false
	}
	ex := sig.GetExchange()
	if ex == nil {
		return "", false
	}
	streamType, ok := streamTypeFromSignalType(sig.GetType())
	if !ok {
		log.Warn().Str("signalType", string(sig.GetType())).Msg("invalid signal type")
		return "", false
	}
	selector := ctypes.StreamSelector{
		Stream: streamType,
	}
	if sym := sig.GetSymbol(); sym != nil {
		selector.Symbol = sym
	}
	if acc := sig.GetAccountID(); acc != nil {
		selector.Account = acc
	}
	if streamType == ctypes.StreamTypeKline {
		switch v := sig.(type) {
		case *stypes.KlineSignal:
			interval := v.Interval
			selector.Interval = &interval
		default:
			return "", false
		}
	}
	if err := selector.Validate(); err != nil {
		log.Warn().Err(err).Msg("invalid signal selector")
		return "", false
	}
	return ctypes.StreamKey(*ex, selector), true
}

func streamTypeFromSignalType(sigType types.SignalType) (ctypes.StreamType, bool) {
	switch sigType {
	case types.SignalTypeKline:
		return ctypes.StreamTypeKline, true
	case types.SignalTypeTicker:
		return ctypes.StreamTypeTicker, true
	case types.SignalTypeTrade:
		return ctypes.StreamTypeTrade, true
	case types.SignalTypeDepth:
		return ctypes.StreamTypeDepth, true
	case types.SignalTypeMarkPrice:
		return ctypes.StreamTypeMarkPrice, true
	case types.SignalTypeSocial:
		return ctypes.StreamTypeSocial, true
	case types.SignalTypeOrder,
		types.SignalTypePosition,
		types.SignalTypeBalance,
		types.SignalTypeFill,
		types.SignalTypeLeverage,
		types.SignalTypeRisk:
		return ctypes.StreamTypeAccount, true
	default:
		return "", false
	}
}
