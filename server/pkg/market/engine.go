package market

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/wangliang139/NovaForge/server/pkg/internal/locker"
	"github.com/wangliang139/NovaForge/server/pkg/market/connector"
	"github.com/wangliang139/NovaForge/server/pkg/market/metrics"
	"github.com/wangliang139/NovaForge/server/pkg/market/provider"
	"github.com/wangliang139/NovaForge/server/pkg/market/pubsub"
	"github.com/wangliang139/NovaForge/server/pkg/market/subscription"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	accountrepo "github.com/wangliang139/NovaForge/server/pkg/repos/account"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
)

type Option func(*Engine) error

func WithPublisher(pub pubsub.Publisher) Option {
	return func(e *Engine) error {
		if pub == nil {
			return fmt.Errorf("nil publisher")
		}
		e.distributor.Register(pub)
		return nil
	}
}

func WithRecorder(rec pubsub.Recorder) Option {
	return func(e *Engine) error {
		if rec == nil {
			return fmt.Errorf("nil recorder")
		}
		e.distributor.SetRecorder(rec)
		return nil
	}
}

func WithConnectorMetrics(m *metrics.ConnectorMetrics) Option {
	return func(e *Engine) error {
		if m == nil {
			return fmt.Errorf("nil connector metrics")
		}
		e.connectorMetrics = m
		return nil
	}
}

type EngineStatus int

const (
	EngineStatusInit    EngineStatus = 0
	EngineStatusRunning EngineStatus = 1
	EngineStatusStopped EngineStatus = 2
)

type Engine struct {
	mu     sync.RWMutex
	status EngineStatus

	locks *locker.ShardedLock

	cfg          Config
	distributor  *pubsub.Distributor
	subscription *subscription.Manager
	db           *repos.Entity

	provider *provider.MarketProvider

	streamsMu sync.RWMutex
	streams   map[string]*streamRuntime

	connectorMetrics *metrics.ConnectorMetrics

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

var errStreamStopped = errors.New("stream stopped")

func NewEngine(cfg Config, db *repos.Entity, opts ...Option) (*Engine, error) {
	cfg.applyDefaults()
	provider := provider.NewMarketProvider(ctypes.ExchangeBinance, "USDT")
	distributor := pubsub.NewDistributor(cfg.AccountRawMsgTopic)
	distributor.SetMarketProvider(provider)
	e := &Engine{
		status:       EngineStatusInit,
		cfg:          cfg,
		db:           db,
		distributor:  distributor,
		subscription: subscription.NewManager(),
		provider:     provider,
		streams:      make(map[string]*streamRuntime),
		locks:        locker.NewShardedLock(),
	}
	for _, opt := range opts {
		if err := opt(e); err != nil {
			return nil, err
		}
	}
	return e, nil
}

func (e *Engine) Start(ctx context.Context) {
	if e.status != EngineStatusInit {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.status != EngineStatusInit {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	e.ctx, e.cancel = context.WithCancel(ctx)
	e.provider.Start()
	e.status = EngineStatusRunning
}

func (e *Engine) Shutdown(ctx context.Context) error {
	if e.status == EngineStatusStopped {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.status == EngineStatusStopped {
		return nil
	}
	if e.cancel != nil {
		e.cancel()
	}
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()
	timeout := e.cfg.ShutdownTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	select {
	case <-ctx.Done():
	case <-done:
	case <-time.After(timeout):
	}
	err := e.distributor.Close()
	if err != nil {
		return err
	}
	e.provider.Close()
	e.status = EngineStatusStopped
	return nil
}

func (e *Engine) GetMarketProvider() *provider.MarketProvider {
	return e.provider
}

func (e *Engine) EnsureSubscription(ctx context.Context, exchange ctypes.Exchange, selector StreamSelector) (*Subscription, error) {
	key := StreamKey(exchange, selector)
	e.locks.Lock(key)
	defer e.locks.Unlock(key)
	subscription, err := e.subscription.Add(exchange, selector)
	if err != nil {
		return nil, err
	}
	if subscription.Count == 1 {
		if err := e.startStream(subscription); err != nil {
			logger.Ctx(ctx).Err(err).Fields(map[string]any{
				"exchange": subscription.Exchange,
				"stream":   subscription.Selector.Key(),
			}).Msg("failed to start stream")
			e.subscription.Remove(subscription.ID)
			return nil, err
		}
	}
	return subscription, nil
}

// GetSubscription 按 id 获取订阅信息，用于释放前从缓存中移除对应项。
func (e *Engine) GetSubscription(id string) *Subscription {
	return e.subscription.Get(id)
}

func (e *Engine) ReleaseSubscription(id string) (bool, error) {
	subscription := e.subscription.Get(id)
	if subscription == nil {
		return true, nil
	}
	key := StreamKey(subscription.Exchange, subscription.Selector)
	e.locks.Lock(key)
	defer e.locks.Unlock(key)
	subscription, err := e.subscription.Remove(id)
	if err != nil {
		return false, err
	}
	if subscription == nil {
		return true, nil
	}
	if subscription.Refs == 0 {
		e.stopStream(subscription)
	}
	return true, nil
}

// ReleaseSubscriptionBySelector 按 exchange 与 selector 释放订阅（用于按账户取消 account stream 等场景）。
func (e *Engine) ReleaseSubscriptionBySelector(exchange ctypes.Exchange, selector ctypes.StreamSelector) (bool, error) {
	id := subscription.SubscriptionID(exchange, selector)
	return e.ReleaseSubscription(id)
}

func (e *Engine) connector(subscription *Subscription) (Connector, error) {
	if !subscription.Selector.Stream.IsAccountRequired() {
		return connector.GetConnector(subscription.Exchange, nil)
	}
	if subscription.Selector.Account == nil {
		return nil, fmt.Errorf("account is required for stream: %s", subscription.Selector.Stream)
	}
	account, err := e.db.AccountRepo.GetById(e.ctx, *subscription.Selector.Account)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	if account == nil {
		return nil, fmt.Errorf("account not found: %s", *subscription.Selector.Account)
	}
	if account.AccountType != accountrepo.AccountTypeReal {
		return nil, fmt.Errorf("only real account is supported")
	}
	apiAccount := NewSecretApiAccount(account.ID, subscription.Exchange, account.ApiKey, account.ApiSecret, account.Passphrase, string(account.Algorithm))
	return connector.GetConnector(subscription.Exchange, apiAccount)
}

func (e *Engine) startStream(subscription *Subscription) error {
	conn, err := e.connector(subscription)
	if err != nil {
		return fmt.Errorf("create connector: %w", err)
	}
	handle, err := conn.Subscribe(e.ctx, subscription.Selector)
	if err != nil {
		return fmt.Errorf("subscribe stream: %w", err)
	}
	key := StreamKey(subscription.Exchange, subscription.Selector)
	runtime := &streamRuntime{
		exchange:  subscription.Exchange,
		selector:  subscription.Selector,
		connector: conn,
		handle:    handle,
		stopCh:    make(chan struct{}),
	}
	runtime.status.Store(int32(StreamStatusInit))
	e.streamsMu.Lock()
	e.streams[key] = runtime
	e.streamsMu.Unlock()

	e.wg.Add(1)
	go e.consume(runtime)
	return nil
}

func (e *Engine) stopStream(subscription *Subscription) {
	key := StreamKey(subscription.Exchange, subscription.Selector)
	e.streamsMu.Lock()
	runtime, ok := e.streams[key]
	if ok {
		delete(e.streams, key)
	}
	e.streamsMu.Unlock()
	if ok && runtime != nil {
		runtime.stop()
	}
}

func (e *Engine) consume(rt *streamRuntime) {
	defer e.wg.Done()
	if rt.handle == nil || rt.status.Load() == int32(StreamStatusStopped) {
		return
	}

	rtStopCh := rt.stopCh
	streamMsgCh := rt.handle.C
	streamErrCh := rt.handle.ErrCh

	reconnect := func(reason string) bool {
		handle, err := e.reconnectStream(rt)
		if err != nil {
			switch {
			case errors.Is(err, errStreamStopped):
				log.Ctx(e.ctx).Info().
					Str("exchange", rt.exchange.String()).
					Str("selector", rt.selector.Key()).
					Str("reason", reason).
					Msg("stream stopped, exit consume")
			case errors.Is(err, context.Canceled):
			default:
				log.Ctx(e.ctx).Err(err).
					Str("exchange", rt.exchange.String()).
					Str("selector", rt.selector.Key()).
					Str("reason", reason).
					Msg("reconnect aborted")
			}
			return false
		}
		streamMsgCh = handle.C
		streamErrCh = handle.ErrCh
		return true
	}

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-rtStopCh:
			return
		case err, ok := <-streamErrCh:
			if !ok {
				log.Ctx(e.ctx).
					Err(err).
					Str("selector", rt.selector.Key()).
					Str("exchange", rt.exchange.String()).
					Msg("connector error channel closed")
				if !reconnect("error channel closed") {
					log.Ctx(e.ctx).Info().
						Str("exchange", rt.exchange.String()).
						Str("selector", rt.selector.Key()).
						Msg("failed to reconnect, exit consume")
					return
				}
				continue
			}
			if err == nil {
				continue
			}
			log.Ctx(e.ctx).Err(err).
				Str("selector", rt.selector.Key()).
				Str("exchange", rt.exchange.String()).
				Msg("connector error")
			if !reconnect("connector error") {
				log.Ctx(e.ctx).Info().
					Str("exchange", rt.exchange.String()).
					Str("selector", rt.selector.Key()).
					Msg("failed to reconnect, exit consume")
				return
			}
			continue
		case msg, ok := <-streamMsgCh:
			if !ok {
				if !reconnect("stream closed") {
					log.Ctx(e.ctx).Info().
						Str("exchange", rt.exchange.String()).
						Str("selector", rt.selector.Key()).
						Msg("failed to reconnect, exit consume")
					return
				}
				continue
			}
			if msg == nil {
				continue
			}
			msg.Exchange = rt.exchange
			msg.Selector = rt.selector
			if msg.Ts.IsZero() {
				receiveAt := time.Now()
				msg.Ts = receiveAt
				if e.connectorMetrics != nil {
					e.connectorMetrics.RecordEvent(rt.exchange.String(), rt.selector.Stream.String(), msg.Ts, receiveAt)
				}
			} else if e.connectorMetrics != nil {
				receiveAt := time.Now()
				e.connectorMetrics.RecordEvent(rt.exchange.String(), rt.selector.Stream.String(), msg.Ts, receiveAt)
			}
			if err := e.distributor.Publish(e.ctx, msg); err != nil {
				log.Ctx(e.ctx).Err(err).Interface("message", msg).Msg("publish market data failed")
			}
		}
	}
}

func (e *Engine) Publish(ctx context.Context, msg *Message) error {
	return e.distributor.Publish(ctx, msg)
}

func (e *Engine) SubscribeTopic(topic string, buffer int) (<-chan *ctypes.Envelope, func()) {
	return e.distributor.Subscribe(topic, buffer)
}

func (e *Engine) reconnectStream(rt *streamRuntime) (*StreamHandle, error) {
	log.Warn().Str("exchange", rt.exchange.String()).
		Any("account_id", lo.FromPtr(rt.selector.Account)).
		Any("symbol", rt.selector.Symbol).
		Any("stream", rt.selector.Stream).
		Msg("reconnect stream")
	if rt == nil {
		return nil, fmt.Errorf("nil runtime")
	}
	if rt.status.Load() == int32(StreamStatusStopped) {
		return nil, errStreamStopped
	}
	if rt.connector == nil {
		return nil, fmt.Errorf("nil connector")
	}
	if rt.handle != nil && rt.handle.Stop != nil {
		rt.handle.Stop()
	}

	rt.status.Store(int32(StreamStatusReconnecting))

	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for attempt := 1; ; attempt++ {
		select {
		case <-e.ctx.Done():
			return nil, e.ctx.Err()
		case <-rt.stopCh:
			return nil, errStreamStopped
		default:
		}

		handle, err := rt.connector.Subscribe(e.ctx, rt.selector)
		if err == nil {
			rt.handle = handle
			rt.status.Store(int32(StreamStatusReady))
			if e.connectorMetrics != nil {
				e.connectorMetrics.RecordReconnect(rt.exchange.String(), rt.selector.Stream.String())
			}
			log.Ctx(e.ctx).
				Info().
				Str("exchange", rt.exchange.String()).
				Str("selector", rt.selector.Key()).
				Int("attempt", attempt).
				Msg("stream reconnected")
			return handle, nil
		}

		log.Ctx(e.ctx).
			Warn().
			Err(err).
			Str("exchange", rt.exchange.String()).
			Str("selector", rt.selector.Key()).
			Int("attempt", attempt).
			Dur("backoff", backoff).
			Msg("reconnect failed, retrying")

		select {
		case <-e.ctx.Done():
			return nil, e.ctx.Err()
		case <-rt.stopCh:
			return nil, errStreamStopped
		case <-time.After(backoff):
		}

		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// GetConnectorStreamStats 返回 Connector 流统计（内存滑动窗口）
func (e *Engine) GetConnectorStreamStats(windowHours int) []metrics.StreamStats {
	if e.connectorMetrics == nil {
		return nil
	}
	return e.connectorMetrics.Snapshot(windowHours)
}

func (e *Engine) Snapshot(exchange *ctypes.Exchange, filterSymbol *string, accountID *string) ([]Subscription, error) {
	var symbol *ctypes.Symbol
	if filterSymbol != nil {
		sb, err := ctypes.ParseSymbol(*filterSymbol)
		if err != nil {
			return nil, err
		}
		if !sb.IsValid() {
			return nil, fmt.Errorf("invalid symbol: %s", *filterSymbol)
		}
		symbol = &sb
	}
	return e.subscription.Snapshot(exchange, symbol, accountID), nil
}
