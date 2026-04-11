package pubsub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
	"github.com/wangliang139/mow/database/cache"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

var ErrCircuitOpen = errors.New("publisher circuit open")

const (
	circuitThreshold = 10
	circuitDuration  = 10 * time.Second
)

// RedisStreamConfig configures the Redis Stream publisher.
type RedisStreamConfig struct {
	Topic string
}

// RedisStreamPublisher publishes market envelopes to Redis Streams (XADD).
// It implements distributor.Publisher and uses the same circuit breaker constants as Kafka.
type RedisStreamPublisher struct {
	cfg RedisStreamConfig

	client redis.UniversalClient

	mu               sync.Mutex
	consecutiveFails int
	circuitOpenUntil time.Time
}

// NewRedisStreamPublisher creates a publisher that writes to Redis Streams.
// Stream key per message is StreamKeyPrefix + topic.
func NewRedisStreamPublisher(cfg RedisStreamConfig) (*RedisStreamPublisher, error) {
	client := cache.NewRedisClient("REDIS_STREAM")
	return &RedisStreamPublisher{cfg: cfg, client: client}, nil
}

func (p *RedisStreamPublisher) Name() string {
	return "redis"
}

func (p *RedisStreamPublisher) Publish(ctx context.Context, topic string, envelope *ctypes.Envelope) error {
	p.mu.Lock()
	if !p.circuitOpenUntil.IsZero() {
		if time.Now().Before(p.circuitOpenUntil) {
			p.mu.Unlock()
			return ErrCircuitOpen
		}
		p.circuitOpenUntil = time.Time{}
	}
	p.mu.Unlock()

	payload, err := sonic.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	stream := p.cfg.Topic
	if envelope.Stream == ctypes.StreamTypeAccountRaw {
		stream = topic
	}
	err = p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		MaxLen: 100000,
		Approx: true,
		ID:     "*",
		Values: map[string]any{
			"payload": payload,
			"topic":   topic,
		},
	}).Err()

	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		p.consecutiveFails++
		if p.consecutiveFails >= circuitThreshold {
			p.circuitOpenUntil = time.Now().Add(circuitDuration)
		}
		return err
	}
	p.consecutiveFails = 0
	return nil
}

// Close releases resources. No-op when client is injected; do not close the shared Redis client.
func (p *RedisStreamPublisher) Close() error {
	return nil
}
