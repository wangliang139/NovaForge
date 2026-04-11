package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/singleflight"
)

const TracerName = "github.com/wangliang139/llt-trade/server/pkg/market/connector/cache"

var tracer = otel.Tracer(TracerName)

type Callback func(ctx context.Context, params ...any) (any, error)

type Cache struct {
	mu       sync.RWMutex
	entries  map[string]entry
	cache    *cache.Cache
	group    singleflight.Group
	errorTTL time.Duration // 可选：错误缓存 TTL
}

func (c *Cache) EnableErrorCache(ttl time.Duration) {
	c.errorTTL = ttl
}

type entry struct {
	ttl      time.Duration
	keyFn    func(params ...any) string
	callback Callback
}

func NewCache(defaultTTL, cleanup time.Duration) *Cache {
	return &Cache{
		entries: make(map[string]entry),
		cache:   cache.New(defaultTTL, cleanup),
	}
}

// 默认 keyFn 支持 params
func defaultKeyFn(pattern string) func(params ...any) string {
	return func(params ...any) string {
		if len(params) == 0 {
			return pattern
		}
		return fmt.Sprintf(pattern, params...)
	}
}

func (c *Cache) Register(pattern string, ttl time.Duration, callback Callback) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.entries[pattern]; ok {
		return fmt.Errorf("entry already registered: %s", pattern)
	}

	c.entries[pattern] = entry{
		ttl:      ttl,
		keyFn:    defaultKeyFn(pattern),
		callback: callback,
	}

	return nil
}

func (c *Cache) SimpleFetch(ctx context.Context, key string, params ...any) (any, error) {
	return c.Fetch(ctx, key, params...)
}

func (c *Cache) Fetch(ctx context.Context, pattern string, params ...any) (any, error) {
	c.mu.RLock()
	ent, ok := c.entries[pattern]
	c.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("pattern not registered: %s", pattern)
	}

	key := ent.keyFn(params...)
	return c.getOrLoad(ctx, key, ent.ttl, ent.callback, params...)
}

func (c *Cache) Get(ctx context.Context, key string, ttl time.Duration, callback Callback, params ...any) (any, error) {
	return c.getOrLoad(ctx, key, ttl, callback, params...)
}

func (c *Cache) getOrLoad(ctx context.Context, key string, ttl time.Duration, callback Callback, params ...any) (any, error) {
	ctx, span := tracer.Start(ctx, "Cache.Get")
	span.SetAttributes(attribute.String("key", key))
	defer span.End()

	// first check
	if v, ok := c.cache.Get(key); ok {
		return v, nil
	}

	// singleflight: multiple goroutine fetch same key -> only one callback executes
	v, err, _ := c.group.Do(key, func() (any, error) {
		// double-check inside
		if v, ok := c.cache.Get(key); ok {
			return v, nil
		}

		data, err := callback(ctx, params...)
		if err != nil {
			// 可选：错误缓存
			if c.errorTTL > 0 {
				c.cache.Set(key+":err", err, c.errorTTL)
			}
			return nil, err
		}

		c.cache.Set(key, data, ttl)
		return data, nil
	})

	return v, err
}
