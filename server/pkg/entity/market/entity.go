package market

import (
	"context"

	"github.com/kelseyhightower/envconfig"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/wangliang139/NovaForge/server/pkg/internal/chsdk"
	"github.com/wangliang139/NovaForge/server/pkg/market"
	"github.com/wangliang139/NovaForge/server/pkg/market/eventflow"
	"github.com/wangliang139/NovaForge/server/pkg/market/metrics"
	"github.com/wangliang139/NovaForge/server/pkg/market/pubsub"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
)

type Config struct {
	Enabled bool `split_words:"true" envconfig:"ENABLE_MARKET_ENGINE" default:"true"`

	EnableRedisPublisher bool   `split_words:"true" default:"true"`
	RedisStreamTopic     string `split_words:"true" envconfig:"REDIS_STREAM_TOPIC" default:"md.all.msg"`

	AccountRawMsgTopic string `split_words:"true" default:"account.raw.msg"`
	TradeBufferSize    int    `split_words:"true" default:"1000"`
	KlineBufferSize    int    `split_words:"true" default:"500"`
	OrderbookDepth     int    `split_words:"true" default:"200"`

	BinanceUseTest  bool   `split_words:"true" default:"false"`
	BinanceProxyURL string `split_words:"true"`

	// ClickHouse for event flow recording
	EnableEventFlow bool `split_words:"true" default:"true"`
}

type Entity struct {
	cfg    Config
	db     *repos.Entity
	engine *market.Engine
	cache  redis.UniversalClient
}

func New(cache redis.UniversalClient, db *repos.Entity) (*Entity, error) {
	cfg := Config{}
	envconfig.MustProcess("MARKET_ENTITY", &cfg)

	mdCfg := market.Config{
		AccountRawMsgTopic: cfg.AccountRawMsgTopic,
		TradeBufferSize:    cfg.TradeBufferSize,
		KlineBufferSize:    cfg.KlineBufferSize,
		MaxOrderBookSize:   cfg.OrderbookDepth,
	}

	var opts []market.Option
	if cfg.Enabled {
		if cfg.EnableRedisPublisher {
			pub, err := pubsub.NewRedisStreamPublisher(pubsub.RedisStreamConfig{
				Topic: cfg.RedisStreamTopic,
			})
			if err != nil {
				log.Warn().Err(err).Msg("failed create Redis Stream publisher")
			} else {
				opts = append(opts, market.WithPublisher(pub))
			}
		}
		if cfg.EnableEventFlow {
			chClient, err := chsdk.Connect(context.Background())
			if err != nil {
				log.Warn().Err(err).Msg("failed connect ClickHouse for event flow recording")
			} else {
				recorder, err := eventflow.NewRecorder(chClient)
				if err != nil {
					log.Warn().Err(err).Msg("failed create event flow recorder")
				} else {
					opts = append(opts, market.WithRecorder(recorder))
					log.Info().Msg("event flow recorder enabled")
				}
			}
		}
		opts = append(opts, market.WithConnectorMetrics(metrics.NewConnectorMetrics()))
	}

	engine, err := market.NewEngine(mdCfg, db, opts...)
	if err != nil {
		return nil, err
	}

	return &Entity{
		cfg:    cfg,
		db:     db,
		engine: engine,
		cache:  cache,
	}, nil
}

func (e *Entity) Engine() *market.Engine {
	return e.engine
}

// GetConnectorStreamStats 返回 Connector 流统计（内存滑动窗口，默认 1 小时）
func (e *Entity) GetConnectorStreamStats(windowHours int) []metrics.StreamStats {
	if !e.cfg.Enabled || e.engine == nil {
		return nil
	}
	return e.engine.GetConnectorStreamStats(windowHours)
}

func (e *Entity) Start(ctx context.Context) error {
	if !e.cfg.Enabled {
		log.Warn().Msg("market data engine is disabled")
		return nil
	}
	e.engine.Start(ctx)
	return nil
}

func (e *Entity) Stop(ctx context.Context) error {
	if !e.cfg.Enabled {
		return nil
	}
	return e.engine.Shutdown(ctx)
}

func (e *Entity) EnsureSubscription(ctx context.Context, exchange ctypes.Exchange, selector ctypes.StreamSelector) (*ctypes.Subscription, error) {
	if !e.cfg.Enabled {
		return nil, nil
	}
	sub, err := e.engine.EnsureSubscription(ctx, exchange, selector)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (e *Entity) ReleaseSubscription(ctx context.Context, id string) (bool, error) {
	if !e.cfg.Enabled {
		return false, nil
	}
	return e.engine.ReleaseSubscription(id)
}

func (e *Entity) ListSubscriptions(exchange *ctypes.Exchange, symbol *string, accountID *string) ([]ctypes.Subscription, error) {
	if !e.cfg.Enabled {
		return nil, nil
	}
	return e.engine.Snapshot(exchange, symbol, accountID)
}

// ReleaseSubscriptionsForAccount 取消指定账户下的所有订阅（含 account stream 及其他流），下线账户时调用。
func (e *Entity) ReleaseSubscriptionsForAccount(ctx context.Context, accountID string) {
	if !e.cfg.Enabled || accountID == "" {
		return
	}
	aid := accountID
	list, err := e.ListSubscriptions(nil, nil, &aid)
	if err != nil || len(list) == 0 {
		return
	}
	for _, sub := range list {
		_, _ = e.engine.ReleaseSubscriptionBySelector(sub.Exchange, sub.Selector)
		logger.Ctx(ctx).Info().
			Str("account_id", accountID).
			Str("exchange", sub.Exchange.String()).
			Str("stream", sub.Selector.Stream.String()).
			Msg("released subscription")
	}
}
