package pubsub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/signal"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

const TopicPattern = `^md-(binance|okx)-.*$`

// Subscriber 信号订阅器接口
type Subscriber interface {
	Subscribe(chan<- stypes.Signal) error
	Close() error
}

// RedisStreamSubscriberConfig configures the Redis Stream subscriber.
// Stream keys are StreamKeyPrefix + topic.
// If StreamKeys is empty, stream keys are discovered via SCAN with StreamKeyPrefix+"*" and filtered by TopicPattern.
type RedisStreamConfig struct {
	Addr     string
	Password string
	DB       int
	PoolSize int

	Topic string
}

// RedisStreamSubscriber subscribes to Redis Streams (XREAD) and emits signals from envelope payloads.
type RedisStreamSubscriber struct {
	once   sync.Once
	cfg    RedisStreamConfig
	client *redis.Client
	ctx    context.Context
	cancel context.CancelFunc
}

var _ Subscriber = (*RedisStreamSubscriber)(nil)

// NewRedisStreamSubscriber creates a Redis Stream subscriber.
func NewRedisStreamSubscriber(cfg RedisStreamConfig) (*RedisStreamSubscriber, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("addr is required")
	}
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})
	ctx, cancel := context.WithCancel(context.Background())
	return &RedisStreamSubscriber{
		cfg:    cfg,
		client: client,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Subscribe starts consuming Redis Streams and returns the signal channel.
func (s *RedisStreamSubscriber) Subscribe(signalCh chan<- stypes.Signal) error {
	go func() {
		block := 5 * time.Second
		for {
			if s.ctx.Err() != nil {
				return
			}
			result, err := s.client.XRead(s.ctx, &redis.XReadArgs{
				Streams: []string{s.cfg.Topic, "$"},
				Block:   block,
				Count:   100,
			}).Result()
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				if errors.Is(err, redis.Nil) {
					continue
				}
				log.Err(err).Msg("redis XREAD error")
				continue
			}
			for _, xstream := range result {
				for _, msg := range xstream.Messages {
					v, ok := msg.Values["payload"]
					if !ok {
						continue
					}
					var payload []byte
					switch p := v.(type) {
					case string:
						payload = []byte(p)
					case []byte:
						payload = p
					default:
						log.Warn().Interface("payload_type", v).Msg("redis stream payload type unsupported")
						continue
					}
					signals, err := signal.Parse(payload)
					if err != nil {
						log.Err(err).Msg("failed to parse redis stream message")
						continue
					}
					for _, signal := range signals {
						if signal == nil {
							continue
						}
						select {
						case signalCh <- signal:
						default:
							log.Warn().Msg("signal channel full, dropping message")
						}
					}
				}
			}
		}
	}()
	return nil
}

// Close stops the consumer and closes the signal channel.
func (s *RedisStreamSubscriber) Close() error {
	s.cancel()
	return nil
}
