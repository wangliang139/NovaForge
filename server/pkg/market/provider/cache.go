package provider

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

type Callback func(ctx context.Context, params ...any) (any, error)

type Cache struct {
	mu      sync.RWMutex
	entries map[string]entry
	cache   *cache.Cache
	group   singleflight.Group
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

// 直接获取缓存，并返回过期时间
func (c *Cache) Obtain(key string) (any, time.Time, bool) {
	return c.cache.GetWithExpiration(key)
}

func (c *Cache) SwapByTtl(key string, value any, ttl time.Duration) (any, bool) {
	if ttl <= 0 {
		v, _ := c.cache.Get(key)
		return v, false
	}
	c.mu.Lock()
	old, expiration, ok := c.cache.GetWithExpiration(key)
	if !ok {
		c.cache.Set(key, value, ttl)
		c.mu.Unlock()
		return value, true
	}
	if expiration.Before(time.Now().Add(ttl)) {
		c.cache.Set(key, value, ttl)
		c.mu.Unlock()
		return value, true
	}
	c.mu.Unlock()
	return old, false
}

func (c *Cache) SwapByFn(key string, fn func(old any, ttl time.Time, ok bool) (any, time.Time, bool)) (any, bool) {
	c.mu.Lock()
	old, ttl, ok := c.cache.GetWithExpiration(key)
	v, t, o := fn(old, ttl, ok)
	if o {
		c.cache.Set(key, v, time.Until(t))
	}
	c.mu.Unlock()
	return v, o
}

func (c *Cache) Get(ctx context.Context, key string, ttl time.Duration, callback Callback, params ...any) (any, error) {
	return c.getOrLoad(ctx, key, ttl, callback, params...)
}

func (c *Cache) getOrLoad(ctx context.Context, key string, ttl time.Duration, callback Callback, params ...any) (any, error) {
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
			return nil, err
		}

		n, _ := c.SwapByTtl(key, data, ttl)
		return n, nil
	})

	return v, err
}
