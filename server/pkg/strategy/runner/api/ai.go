package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/samber/lo"
	"github.com/wangliang139/NovaForge/server/pkg/internal/zai"
	"github.com/wangliang139/NovaForge/server/pkg/settings"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/misc"
	"rogchap.com/v8go"
)

const (
	defaultAICompletionTimeout = 15 * time.Second
	defaultAIMaxTimeout        = 30 * time.Second
)

type AIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AICompletionRequest struct {
	Model          string        `json:"model,omitempty"`
	Prompt         string        `json:"prompt,omitempty"`
	Messages       []AIMessage   `json:"messages,omitempty"`
	JSON           bool          `json:"json,omitempty"`
	ResponseFormat string        `json:"responseFormat,omitempty"`
	Timeout        time.Duration `json:"-"`
}

type AICompletionResult struct {
	Result   any            `json:"result"`
	Text     string         `json:"text,omitempty"`
	JSON     any            `json:"json,omitempty"`
	Model    string         `json:"model,omitempty"`
	Duration int64          `json:"duration"`
	Usage    map[string]any `json:"usage,omitempty"`
}

type AICompleter interface {
	Complete(ctx context.Context, req AICompletionRequest) (*AICompletionResult, error)
}

type AIAPI struct {
	completer      AICompleter
	defaultTimeout time.Duration
	maxTimeout     time.Duration
}

type AIAPIConfig struct {
	Completer      AICompleter
	DefaultTimeout time.Duration
	MaxTimeout     time.Duration
}

func NewAIAPI(cfg AIAPIConfig) *AIAPI {
	defaultTimeout := cfg.DefaultTimeout
	if defaultTimeout <= 0 {
		defaultTimeout = defaultAICompletionTimeout
	}
	maxTimeout := cfg.MaxTimeout
	if maxTimeout <= 0 {
		maxTimeout = defaultAIMaxTimeout
	}
	if defaultTimeout > maxTimeout {
		defaultTimeout = maxTimeout
	}
	return &AIAPI{
		completer:      cfg.Completer,
		defaultTimeout: defaultTimeout,
		maxTimeout:     maxTimeout,
	}
}

func (a *AIAPI) Complete(info *v8go.FunctionCallbackInfo) *v8go.Value {
	ctx := info.Context()
	if a == nil || a.completer == nil {
		return throwError(ctx, "ai.complete is not configured")
	}
	args := info.Args()
	if len(args) == 0 || !args[0].IsObject() {
		return throwError(ctx, "ai.complete(options) requires an options object")
	}

	req, err := a.parseRequest(ctx, args[0])
	if err != nil {
		return throwError(ctx, err.Error())
	}

	callCtx, cancel := context.WithTimeout(context.Background(), req.Timeout)
	defer cancel()
	result, err := a.completer.Complete(callCtx, req)
	if err != nil {
		return throwError(ctx, err.Error())
	}
	if result == nil {
		return throwError(ctx, "ai completion returned nil result")
	}

	val, err := misc.AnyToV8Value(ctx, result)
	if err != nil {
		return throwError(ctx, fmt.Sprintf("failed to convert ai completion result: %v", err))
	}
	return val
}

func (a *AIAPI) parseRequest(ctx *v8go.Context, val *v8go.Value) (AICompletionRequest, error) {
	opts, err := misc.V8ValueToMap(ctx, val)
	if err != nil {
		return AICompletionRequest{}, fmt.Errorf("invalid ai.complete options: %w", err)
	}

	req := AICompletionRequest{Timeout: a.defaultTimeout}
	if model, ok := stringOption(opts, "model"); ok {
		req.Model = model
	}
	if prompt, ok := stringOption(opts, "prompt"); ok {
		req.Prompt = prompt
	}
	if responseFormat, ok := stringOption(opts, "responseFormat"); ok {
		req.ResponseFormat = responseFormat
	}
	if jsonFlag, ok := opts["json"].(bool); ok {
		req.JSON = jsonFlag
	}
	if timeoutMs, ok := numericOption(opts, "timeoutMs"); ok {
		if timeoutMs <= 0 {
			return AICompletionRequest{}, fmt.Errorf("timeoutMs must be greater than 0")
		}
		req.Timeout = time.Duration(timeoutMs) * time.Millisecond
	}
	if req.Timeout > a.maxTimeout {
		return AICompletionRequest{}, fmt.Errorf("timeoutMs exceeds max timeout %dms", a.maxTimeout.Milliseconds())
	}

	if messagesRaw, ok := opts["messages"]; ok && messagesRaw != nil {
		messagesBytes, err := sonic.Marshal(messagesRaw)
		if err != nil {
			return AICompletionRequest{}, fmt.Errorf("invalid messages: %w", err)
		}
		if err := sonic.Unmarshal(messagesBytes, &req.Messages); err != nil {
			return AICompletionRequest{}, fmt.Errorf("invalid messages: %w", err)
		}
	}

	if strings.TrimSpace(req.Prompt) == "" && len(req.Messages) == 0 {
		return AICompletionRequest{}, fmt.Errorf("prompt or messages is required")
	}
	for i := range req.Messages {
		req.Messages[i].Role = strings.TrimSpace(req.Messages[i].Role)
		req.Messages[i].Content = strings.TrimSpace(req.Messages[i].Content)
		if req.Messages[i].Role == "" {
			return AICompletionRequest{}, fmt.Errorf("messages[%d].role is required", i)
		}
		if req.Messages[i].Content == "" {
			return AICompletionRequest{}, fmt.Errorf("messages[%d].content is required", i)
		}
	}

	return req, nil
}

func stringOption(opts map[string]any, key string) (string, bool) {
	val, ok := opts[key].(string)
	if !ok {
		return "", false
	}
	val = strings.TrimSpace(val)
	if val == "" {
		return "", false
	}
	return val, true
}

func numericOption(opts map[string]any, key string) (int64, bool) {
	switch v := opts[key].(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	default:
		return 0, false
	}
}

type ZaiCompleter struct {
	engine *zai.Engine
}

func NewZaiCompleter(engine *zai.Engine) *ZaiCompleter {
	return &ZaiCompleter{engine: engine}
}

func (c *ZaiCompleter) Complete(ctx context.Context, req AICompletionRequest) (*AICompletionResult, error) {
	if c == nil || c.engine == nil {
		return nil, fmt.Errorf("llm engine is not configured")
	}
	model := req.Model
	if strings.TrimSpace(model) == "" {
		if cfg, err := settings.GetLlmProviderConfig(ctx); err == nil {
			model = cfg.DefaultModel
		}
	}

	params := openai.ChatCompletionNewParams{
		Model: openai.ChatModel(model),
	}
	if req.JSON || req.ResponseFormat == "json_object" {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: lo.ToPtr(shared.NewResponseFormatJSONObjectParam()),
		}
	}
	for _, msg := range buildChatMessages(req) {
		params.Messages = append(params.Messages, msg)
	}

	start := time.Now()
	resp, err := c.engine.Caller().
		WithPlatform(zai.PlatformTypeOpenRouter).
		CreateChatCompletion(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to call llm: %w", err)
	}
	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("llm returned empty response")
	}

	result := resp.Choices[0].Message.Content
	out := &AICompletionResult{
		Result:   result,
		Text:     result,
		Model:    resp.Model,
		Duration: time.Since(start).Milliseconds(),
		Usage: map[string]any{
			"promptTokens":     resp.Usage.PromptTokens,
			"completionTokens": resp.Usage.CompletionTokens,
			"totalTokens":      resp.Usage.TotalTokens,
		},
	}
	if req.JSON || req.ResponseFormat == "json_object" {
		var parsed any
		if err := sonic.UnmarshalString(result, &parsed); err != nil {
			return nil, fmt.Errorf("llm returned invalid json: %w", err)
		}
		out.Result = parsed
		out.JSON = parsed
	}
	return out, nil
}

func buildChatMessages(req AICompletionRequest) []openai.ChatCompletionMessageParamUnion {
	messages := req.Messages
	if len(messages) == 0 {
		messages = []AIMessage{{Role: "user", Content: req.Prompt}}
	}
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			out = append(out, openai.ChatCompletionMessageParamUnion{
				OfSystem: &openai.ChatCompletionSystemMessageParam{
					Content: openai.ChatCompletionSystemMessageParamContentUnion{OfString: openai.String(msg.Content)},
				},
			})
		case "assistant":
			out = append(out, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &openai.ChatCompletionAssistantMessageParam{
					Content: openai.ChatCompletionAssistantMessageParamContentUnion{OfString: openai.String(msg.Content)},
				},
			})
		default:
			out = append(out, openai.ChatCompletionMessageParamUnion{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{OfString: openai.String(msg.Content)},
				},
			})
		}
	}
	return out
}
