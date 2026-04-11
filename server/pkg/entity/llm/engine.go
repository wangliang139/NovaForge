package llm

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"runtime/debug"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"github.com/samber/lo"
	"github.com/wangliang139/NovaForge/server/pkg/repos/llm_completion"
	"github.com/wangliang139/NovaForge/server/pkg/repos/llm_prompt"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/logger"
)

type dumper interface {
	DumpRequest(bool) []byte
}

func (e *Entity) Completion(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	var sceneKey SceneKey

	sceneKeyParts := strings.Split(req.SceneKey, ":")
	if len(sceneKeyParts) == 2 {
		sceneKey.Key = sceneKeyParts[0]
		sceneKey.Variant = sceneKeyParts[1]
	} else {
		sceneKey.Key = req.SceneKey
	}
	scene, err := e.getScene(ctx, sceneKey.Key, req.Force)
	if err != nil {
		return nil, err
	}

	var prompt *types.LlmPrompt
	if req.ByPrompt != nil {
		prompt = req.ByPrompt
	} else {
		// 根据权重随机选择一个模型
		prompt = e.route(ctx, scene, sceneKey, req.ByPromptID)
		if prompt == nil {
			return nil, fmt.Errorf("no available model")
		}
	}

	return e.completion(ctx, sceneKey, scene, prompt, req)
}

func (e *Entity) getScene(ctx context.Context, sceneKey string, force bool) (*types.LlmScene, error) {
	scenePo, err := e.db.LlmSceneRepo.GetByKey(ctx, sceneKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get scene: %w", err)
	}
	if scenePo == nil {
		return nil, fmt.Errorf("scene not found: %s", sceneKey)
	}
	if !scenePo.Enabled && !force {
		return nil, fmt.Errorf("scene is disabled: %s", sceneKey)
	}

	scene, err := types.SceneFromDB(scenePo)
	if err != nil {
		return nil, fmt.Errorf("failed to parse scene: %w", err)
	}

	enabled := lo.ToPtr(true)
	if !force {
		enabled = nil
	}

	promptsPo, err := e.db.LlmPromptRepo.GetBySceneKey(ctx, llm_prompt.GetBySceneKeyParams{
		SceneKey: sceneKey,
		Enabled:  enabled,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get prompts: %w", err)
	}

	for _, promptPo := range promptsPo {
		prompt, err := types.PromptFromDB(&promptPo)
		if err != nil {
			logger.Ctx(ctx).Err(err).Msg("failed to parse prompt")
			continue
		}
		scene.Prompts = append(scene.Prompts, prompt)
	}

	return scene, nil
}

func (e *Entity) completion(ctx context.Context, sceneKey SceneKey, scene *types.LlmScene, prompt *types.LlmPrompt, input *CompletionRequest) (response *CompletionResponse, err error) {
	// 组装最终配置
	config := e.buildConfig(prompt)

	request, err := e.buildRequest(scene, prompt, &config, input)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	if request == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if len(request.Messages) == 0 {
		return nil, fmt.Errorf("no prompt messages")
	}

	timeout := prompt.Timeout
	if timeout <= 0 {
		timeout = e.cfg.MaxTimeout
	}

	options := []option.RequestOption{}
	if len(prompt.Providers) > 0 {
		options = append(options, option.WithJSONSet("provider", map[string]any{
			"order": prompt.Providers,
		}))
	}

	startTime := time.Now()

	metadata := &Metadata{
		SceneKey:  sceneKey.String(),
		Scene:     *scene,
		Prompt:    *prompt,
		Variables: input.Variables,
	}

	response = &CompletionResponse{
		Metadata: metadata,
	}

	ctxNoCancel := context.WithoutCancel(ctx)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
			logger.Ctx(ctxNoCancel).Err(err).Str("panic.stack", string(debug.Stack())).Msg("panic recovered")
		}
		completion, err2 := e.saveCompletion(ctxNoCancel, sceneKey, metadata, input, request, response, err)
		if err2 != nil {
			logger.Ctx(ctxNoCancel).Err(err2).Msg("failed to save completion")
		}
		if completion != nil {
			response.CompletionID = lo.ToPtr(completion.ID)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// 调用模型API
	var resp *openai.ChatCompletion
	resp, err = e.zai.Caller().WithPlatform(prompt.Platform).CreateChatCompletion(ctx, *request, options...)

	response.Duration = time.Since(startTime)
	response.Raw = resp
	if resp != nil {
		response.Usage = lo.ToPtr(resp.Usage)
		response.Metadata.Model = lo.ToPtr(resp.Model)
		if resp.JSON.ExtraFields != nil {
			if p, ok := resp.JSON.ExtraFields["provider"]; ok {
				provider := strings.TrimSuffix(p.Raw(), "\"")
				provider = strings.TrimPrefix(provider, "\"")
				response.Metadata.Provider = lo.ToPtr(provider)
			}
		}
	}

	if err != nil {
		err = fmt.Errorf("failed to call llm: %w", err)
		var d dumper
		if errors.As(err, &d) {
			logger.Ctx(ctx).Err(err).Str("request", string(d.DumpRequest(true))).Msg("failed to call llm")
		}
		return
	}

	if resp == nil {
		err = fmt.Errorf("response is nil")
		return
	}

	if len(resp.Choices) == 0 {
		err = fmt.Errorf("no choices in response")
		return
	}

	choice := resp.Choices[0]
	if choice.Message.Content == "" {
		err = fmt.Errorf("no content in choice")
		return
	}

	response.Result = choice.Message.Content

	return response, nil
}

func (e *Entity) buildRequest(scene *types.LlmScene, prompt *types.LlmPrompt, config *types.LlmConfig, req *CompletionRequest) (*openai.ChatCompletionNewParams, error) {
	request := openai.ChatCompletionNewParams{
		Model:               prompt.Model,
		Temperature:         openai.Float(lo.FromPtrOr(config.Temperature, 0)),
		TopP:                openai.Float(lo.FromPtrOr(config.TopP, 0)),
		MaxTokens:           openai.Int(lo.FromPtrOr(config.MaxTokens, 0)),
		MaxCompletionTokens: openai.Int(lo.FromPtrOr(config.MaxCompletionTokens, 0)),
	}

	switch scene.ResponseFormat.Type {
	case "text":
		request.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfText: lo.ToPtr(shared.NewResponseFormatTextParam()),
		}
	case "json_object":
		request.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: lo.ToPtr(shared.NewResponseFormatJSONObjectParam()),
		}
	case "json_schema":
		if scene.ResponseFormat.JSONSchema == nil {
			return nil, fmt.Errorf("json schema is required")
		}
		request.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: *scene.ResponseFormat.JSONSchema,
			},
		}
	}

	// 变量替换
	for _, message := range prompt.Messages {
		content, err := utils.Template.Render(message.Content, req.Variables)
		if err != nil {
			return nil, fmt.Errorf("failed to render prompt: %w", err)
		}

		var msg openai.ChatCompletionMessageParamUnion
		switch message.Role {
		case "system":
			msg = openai.ChatCompletionMessageParamUnion{
				OfSystem: &openai.ChatCompletionSystemMessageParam{
					Content: openai.ChatCompletionSystemMessageParamContentUnion{
						OfString: openai.String(content),
					},
				},
			}
		case "user":
			msg = openai.ChatCompletionMessageParamUnion{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{
						OfString: openai.String(content),
					},
				},
			}
		case "assistant":
			msg = openai.ChatCompletionMessageParamUnion{
				OfAssistant: &openai.ChatCompletionAssistantMessageParam{
					Content: openai.ChatCompletionAssistantMessageParamContentUnion{
						OfString: openai.String(content),
					},
				},
			}
		case "tool":
			msg = openai.ChatCompletionMessageParamUnion{
				OfTool: &openai.ChatCompletionToolMessageParam{
					Content: openai.ChatCompletionToolMessageParamContentUnion{
						OfString: openai.String(content),
					},
				},
			}
		default:
			return nil, fmt.Errorf("invalid message role: %s", message.Role)
		}
		request.Messages = append(request.Messages, msg)
	}
	return &request, nil
}

// route 根据权重随机选择模型
func (e *Entity) route(ctx context.Context, scene *types.LlmScene, sceneKey SceneKey, promptID *int64) *types.LlmPrompt {
	prompts := scene.Prompts

	if promptID != nil && *promptID > 0 {
		prompt, ok := lo.Find(prompts, func(p *types.LlmPrompt) bool {
			return p.Enabled && p.ID == *promptID
		})
		if ok {
			return prompt
		}
		return nil
	}

	if len(sceneKey.Variant) > 0 {
		variantPrompts := lo.Filter(prompts, func(p *types.LlmPrompt, _ int) bool {
			return p.Enabled && lo.Contains(p.Variants, sceneKey.Variant)
		})
		if len(variantPrompts) > 1 {
			logger.Ctx(ctx).Warn().Str("scene_key", scene.Key).Str("variant", sceneKey.Variant).Msg("multiple variant prompts found, default use the first one")
			return variantPrompts[0]
		} else if len(variantPrompts) == 1 {
			return variantPrompts[0]
		} else {
			logger.Ctx(ctx).Warn().Str("scene_key", scene.Key).Str("variant", sceneKey.Variant).Msg("no variant prompts found, route to weight prompts")
		}
	}

	prompts = lo.Filter(prompts, func(p *types.LlmPrompt, _ int) bool {
		return p.Enabled && p.Weight > 0
	})
	if len(prompts) == 0 {
		return nil
	}

	// 计算总权重
	totalWeight := int64(0)
	for _, p := range prompts {
		totalWeight += int64(p.Weight)
	}

	// 生成随机数
	r, _ := rand.Int(rand.Reader, big.NewInt(totalWeight))
	random := r.Int64()

	// 根据权重范围选择
	current := int64(0)
	for _, p := range prompts {
		current += int64(p.Weight)
		if random < current {
			return p
		}
	}

	return prompts[0]
}

// buildConfig 组装最终配置
func (e *Entity) buildConfig(prompt *types.LlmPrompt) types.LlmConfig {
	config := &types.LlmConfig{}
	if prompt.Config == nil {
		return *config
	}

	config = config.Merge(prompt.Config)
	return *config
}

func (e *Entity) saveCompletion(ctx context.Context, sceneKey SceneKey, metadata *Metadata, input *CompletionRequest, req *openai.ChatCompletionNewParams, resp *CompletionResponse, err error) (*llm_completion.LlmCompletion, error) {
	scene := metadata.Scene
	prompt := metadata.Prompt

	varBytes, err2 := sonic.Marshal(input.Variables)
	if err2 != nil {
		logger.Ctx(ctx).Err(err2).Msg("failed to marshal variables")
	}

	msgBytes, err2 := sonic.Marshal(req.Messages)
	if err2 != nil {
		logger.Ctx(ctx).Err(err2).Msg("failed to marshal messages")
	}

	tokensBytes := []byte("{}")
	if resp != nil && resp.Usage != nil {
		var err2 error
		tokensBytes, err2 = sonic.Marshal(resp.Usage)
		if err2 != nil {
			logger.Ctx(ctx).Err(err2).Msg("failed to marshal tokens")
		}
	}

	var errMsg string
	if err != nil {
		errMsg = err.Error()
	}
	if len(errMsg) > 1000 {
		errMsg = utils.Strings.TruncateUTF8(errMsg, 1000) + "..."
	}

	answer := ""
	if resp != nil {
		answer = resp.Result
	}

	params := llm_completion.CreateParams{
		SceneKey:  sceneKey.String(),
		SceneID:   scene.ID,
		PromptID:  prompt.ID,
		Platform:  string(prompt.Platform),
		Provider:  lo.FromPtrOr(metadata.Provider, ""),
		Model:     lo.FromPtrOr(metadata.Model, ""),
		Variables: varBytes,
		Messages:  msgBytes,
		Answer:    answer,
		Error:     errMsg,
		Duration:  int32(resp.Duration.Milliseconds()),
		Tokens:    tokensBytes,
		Status:    llm_completion.LlmCompletionStatusActive,
	}

	return e.db.LlmCompletionRepo.Create(ctx, params)
}
