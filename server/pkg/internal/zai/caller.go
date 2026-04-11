package zai

import (
	"context"
	"errors"

	"github.com/bytedance/sonic"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/pagination"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/samber/lo"
	"github.com/wangliang139/mow/logger"
)

type Caller struct {
	engine *Engine

	apiType LlmApiType

	wantedPlatform *PlatformType
	wantedModels   []string

	platform PlatformType
	model    *Model

	client *openai.Client
}

func (c *Caller) WithPlatform(platform PlatformType) *Caller {
	c.wantedPlatform = lo.ToPtr(platform)
	return c
}

func (c *Caller) WithModels(models ...string) *Caller {
	c.wantedModels = models
	return c
}

func (c *Caller) route(ctx context.Context, preferModel *string) error {
	platforms, err := c.engine.platformsSnapshot(ctx)
	if err != nil {
		return err
	}

	var matchedModels []*Model

	var wantedModels []string
	if preferModel != nil && len(*preferModel) > 0 {
		wantedModels = append(wantedModels, *preferModel)
	}
	wantedModels = append(wantedModels, c.wantedModels...)
	wantedModels = lo.Uniq(wantedModels)

	for _, platform := range platforms {
		if c.wantedPlatform != nil && platform.Type != *c.wantedPlatform {
			continue
		}

		if !lo.Contains(platform.Capability, c.apiType) && !lo.Contains(platform.Capability, LlmApiTypeAll) && c.apiType != LlmApiTypeAll {
			continue
		}

		if c.apiType == LlmApiTypeEmbedding {
			matchedModels = append(matchedModels, &Model{
				Id:       *preferModel,
				Platform: platform.Type,
			})
			continue
		}
		for _, model := range platform.Models {
			if len(wantedModels) > 0 && !lo.Contains(wantedModels, model.Id) {
				continue
			}

			matchedModels = append(matchedModels, model)
		}
	}

	if len(matchedModels) == 0 {
		return errors.New("no usable model found for api type: " + string(c.apiType))
	}

	destModel := matchedModels[0]
	if len(matchedModels) > 1 {
		// TODO: 选择最佳模型
	}

	c.platform = destModel.Platform
	c.model = destModel
	c.client = platforms[destModel.Platform].Client

	logger.Ctx(ctx).Debug().Str("model", destModel.Id).Str("platform", string(destModel.Platform)).Msg("api route")

	return nil
}

func (c *Caller) CreateEmbeddings(ctx context.Context, body openai.EmbeddingNewParams, opts ...option.RequestOption) (*openai.CreateEmbeddingResponse, error) {
	c.apiType = LlmApiTypeEmbedding
	err := c.route(ctx, &body.Model)
	if err != nil {
		return nil, err
	}

	body.Model = c.model.Id

	return c.client.Embeddings.New(ctx, body, opts...)
}

func (c *Caller) CreateChatCompletion(ctx context.Context, body openai.ChatCompletionNewParams, opts ...option.RequestOption) (*openai.ChatCompletion, error) {
	c.apiType = LlmApiTypeChat
	err := c.route(ctx, &body.Model)
	if err != nil {
		return nil, err
	}

	body.Model = c.model.Id
	resp, err := c.client.Chat.Completions.New(ctx, body, opts...)
	if err != nil {
		return nil, err
	}

	if c.platform == PlatformTypeOpenRouter {
		var orErrorResponse OrErrorResponse
		if err := sonic.UnmarshalString(resp.RawJSON(), &orErrorResponse); err == nil && orErrorResponse.Error.Code >= 400 {
			return nil, errors.New(orErrorResponse.Error.Message)
		}
	}

	return resp, nil
}

func (c *Caller) CreateChatCompletionStream(ctx context.Context, body openai.ChatCompletionNewParams, opts ...option.RequestOption) (*ssestream.Stream[openai.ChatCompletionChunk], error) {
	c.apiType = LlmApiTypeChat
	err := c.route(ctx, &body.Model)
	if err != nil {
		return nil, err
	}

	body.Model = c.model.Id
	return c.client.Chat.Completions.NewStreaming(ctx, body, opts...), nil
}

func (c *Caller) CreateCompletion(ctx context.Context, body openai.CompletionNewParams, opts ...option.RequestOption) (*openai.Completion, error) {
	c.apiType = LlmApiTypeChat
	err := c.route(ctx, lo.ToPtr(string(body.Model)))
	if err != nil {
		return nil, err
	}

	body.Model = openai.CompletionNewParamsModel(c.model.Id)
	return c.client.Completions.New(ctx, body, opts...)
}

func (c *Caller) CreateCompletionStream(ctx context.Context, body openai.CompletionNewParams, opts ...option.RequestOption) (*ssestream.Stream[openai.Completion], error) {
	c.apiType = LlmApiTypeChat
	err := c.route(ctx, lo.ToPtr(string(body.Model)))
	if err != nil {
		return nil, err
	}

	body.Model = openai.CompletionNewParamsModel(c.model.Id)
	return c.client.Completions.NewStreaming(ctx, body, opts...), nil
}

func (c *Caller) Models(ctx context.Context, opts ...option.RequestOption) (*pagination.Page[openai.Model], error) {
	c.apiType = LlmApiTypeModels
	err := c.route(ctx, nil)
	if err != nil {
		return nil, err
	}
	return c.client.Models.List(ctx, opts...)
}

func (c *Caller) Moderations(ctx context.Context, body openai.ModerationNewParams, opts ...option.RequestOption) (*openai.ModerationNewResponse, error) {
	c.apiType = LlmApiTypeModeration
	err := c.route(ctx, lo.ToPtr(string(body.Model)))
	if err != nil {
		return nil, err
	}

	body.Model = c.model.Id
	return c.client.Moderations.New(ctx, body, opts...)
}
