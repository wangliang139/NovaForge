package zai

import (
	"context"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type PlatformType string

const (
	PlatformTypeOpenRouter PlatformType = "openrouter"
)

func (p PlatformType) Valid() bool {
	switch p {
	case PlatformTypeOpenRouter:
		return true
	}
	return false
}

type Platform struct {
	Type PlatformType

	ApiKey  string
	BaseURL string

	Capability []LlmApiType

	Client *openai.Client

	Models map[string]*Model
}

type Model struct {
	Id       string
	Platform PlatformType
}

func NewPlatform(tp PlatformType, apiKey string, baseURL string, capability []LlmApiType) (*Platform, error) {
	client := NewClient(apiKey, baseURL)

	platform := &Platform{
		Type:       tp,
		ApiKey:     apiKey,
		BaseURL:    baseURL,
		Client:     client,
		Capability: capability,
		Models:     make(map[string]*Model),
	}

	// 查询平台支持模型列表
	models, err := client.Models.List(context.Background())
	if err != nil {
		return nil, err
	}

	for _, model := range models.Data {
		platform.Models[model.ID] = &Model{
			Id:       model.ID,
			Platform: tp,
		}
	}

	return platform, nil
}

func NewClient(apiKey string, baseURL string) *openai.Client {
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(&http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		}),
	)
	return &client
}
