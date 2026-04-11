package zai

import (
	"context"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/wangliang139/llt-trade/server/pkg/settings"
)

type Engine struct {
	mu        sync.RWMutex
	keySig    string
	platforms map[PlatformType]*Platform
}

func buildPlatformsFromKeys(openRouterKey string) map[PlatformType]*Platform {
	platforms := make(map[PlatformType]*Platform)

	ps := []Platform{}
	if openRouterKey != "" {
		ps = append(ps, Platform{
			Type:       PlatformTypeOpenRouter,
			ApiKey:     openRouterKey,
			BaseURL:    "https://openrouter.ai/api/v1",
			Capability: []LlmApiType{LlmApiTypeAll},
		})
	}

	if len(ps) == 0 {
		log.Warn().Msg("no llm api keys provided (kv + env)")
	}

	wg := sync.WaitGroup{}
	wg.Add(len(ps))
	for _, p := range ps {
		go func(p Platform) {
			defer wg.Done()
			client, err := NewPlatform(p.Type, p.ApiKey, p.BaseURL, p.Capability)
			if err != nil {
				log.Err(err).Msgf("failed to initialize api: %s", p.Type)
			} else {
				platforms[p.Type] = client
			}
		}(p)
	}
	wg.Wait()

	return platforms
}

// platformsSnapshot 按当前请求上下文解析 kv + 环境变量中的密钥，并在指纹变化时重建平台客户端。
func (e *Engine) platformsSnapshot(ctx context.Context) (map[PlatformType]*Platform, error) {
	orK, err := settings.GetLlmProviderConfig(ctx)
	if err != nil {
		return nil, err
	}
	sig := orK.OpenRouterAPIKey

	e.mu.RLock()
	if e.keySig == sig && e.platforms != nil {
		p := e.platforms
		e.mu.RUnlock()
		return p, nil
	}
	e.mu.RUnlock()

	created := buildPlatformsFromKeys(orK.OpenRouterAPIKey)

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.keySig == sig && e.platforms != nil {
		return e.platforms, nil
	}
	e.platforms = created
	e.keySig = sig
	return e.platforms, nil
}

func NewEngine() *Engine {
	return &Engine{
		platforms: nil,
	}
}

func (c *Engine) Caller() *Caller {
	return &Caller{
		engine:   c,
		platform: PlatformTypeOpenRouter,
	}
}
