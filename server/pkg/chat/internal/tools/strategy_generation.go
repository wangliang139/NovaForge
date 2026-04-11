package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/openai/openai-go/v3"
	"github.com/wangliang139/NovaForge/server/pkg/chat/domain"
	"github.com/wangliang139/NovaForge/server/pkg/internal/zai"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type generatedStrategyResult struct {
	Name        string                    `json:"name"`
	Code        string                    `json:"code"`
	Params      []stypes.StrategyParam    `json:"params"`
	Signals     []stypes.SignalDefinition `json:"signals"`
	Description string                    `json:"description"`
}

func GenerateStrategy(ctx context.Context, args map[string]any, env domain.Env) (any, error) {
	if env.DB == nil {
		return nil, domain.NewRuntimeError("dependency_missing", "kv repo is not configured")
	}
	if env.ZaiEngine == nil {
		return nil, domain.NewRuntimeError("dependency_missing", "zai engine is not configured")
	}

	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return nil, domain.NewRuntimeError("invalid_argument", "query must be non-empty string")
	}
	query = strings.TrimSpace(query)

	guide, err := env.DB.KvRepo.GetByKey(ctx, "doc.strategy.guide")
	if err != nil {
		return nil, err
	}
	if guide == nil || strings.TrimSpace(guide.Value) == "" {
		return map[string]any{
			"code":    "doc_not_found",
			"message": "kv key doc.strategy.guide not found or empty",
			"skill": map[string]any{
				"callName": "skill.generate_strategy",
				"name":     "生成策略",
			},
		}, nil
	}

	currentStrategy := map[string]any{}
	if raw, exists := args["current_strategy"]; exists && raw != nil {
		if cast, ok := raw.(map[string]any); ok {
			currentStrategy = cast
		}
	}

	userPayload := map[string]any{
		"guide":            guide.Value,
		"query":            query,
		"current_strategy": currentStrategy,
	}
	userPayloadRaw, err := sonic.Marshal(userPayload)
	if err != nil {
		return nil, domain.NewRuntimeError("internal_error", "failed to encode prompt payload")
	}

	systemPrompt := strings.TrimSpace(`你是资深量化策略工程师。
请根据策略开发手册和用户描述，生成一个完整可用的策略草案。
如果 current_strategy 不为空，代表用户希望在已有策略上做增量修改；你必须保留合理的原有结构，并仅按需求调整。

你必须只输出 JSON（禁止 markdown、禁止解释文字），严格匹配如下结构：
{
  "name": "string",
  "code": "string",
  "description": "string",
  "params": [
    {
      "name": "string",
      "description": "string",
      "type": "string|number|bool|object|[]string|[]number|[]bool|[]object",
      "required": true,
      "default": any
    }
  ],
  "signals": [
    {
      "id": "string",
      "type": "kline|trade|depth|ticker|mark_price|social|timer|order|position|balance|fill|leverage|risk|system|test",
      "scope": "symbol|target|exchange|strategy",
      "exchange": "string or null",
      "symbol": "string or null",
      "props": {}
    }
  ]
}

约束：
1) name/code/description 必须非空。
2) params/signals 必须是数组（可为空数组）。
3) code 必须是纯 JavaScript 代码文本，不要使用代码围栏。
4) 输出必须是可被 JSON 解析的单个对象。`)

	resp, err := env.ZaiEngine.Caller().
		WithPlatform(zai.PlatformTypeOpenRouter).
		CreateChatCompletion(ctx, openai.ChatCompletionNewParams{
			Model: openai.ChatModel(env.Model),
			Messages: []openai.ChatCompletionMessageParamUnion{
				{
					OfSystem: &openai.ChatCompletionSystemMessageParam{
						Content: openai.ChatCompletionSystemMessageParamContentUnion{
							OfString: openai.String(systemPrompt),
						},
					},
				},
				{
					OfUser: &openai.ChatCompletionUserMessageParam{
						Content: openai.ChatCompletionUserMessageParamContentUnion{
							OfString: openai.String(string(userPayloadRaw)),
						},
					},
				},
			},
			ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONObject: &openai.ResponseFormatJSONObjectParam{
					Type: "json_object",
				},
			},
		})
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, domain.NewRuntimeError("empty_response", "llm returned empty choices")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, domain.NewRuntimeError("empty_response", "llm returned empty strategy payload")
	}

	var result generatedStrategyResult
	if err := sonic.Unmarshal([]byte(content), &result); err != nil {
		return nil, domain.NewRuntimeError("invalid_llm_output", "llm result is not valid strategy json")
	}
	if err := validateGeneratedStrategy(&result); err != nil {
		return nil, domain.NewRuntimeError("invalid_llm_output", err.Error())
	}

	return map[string]any{
		"skill": map[string]any{
			"callName": "skill.generate_strategy",
			"name":     "生成策略",
		},
		"strategy": result,
	}, nil
}

func validateGeneratedStrategy(out *generatedStrategyResult) error {
	if out == nil {
		return fmt.Errorf("strategy payload is empty")
	}
	out.Name = strings.TrimSpace(out.Name)
	out.Description = strings.TrimSpace(out.Description)
	out.Code = strings.TrimSpace(out.Code)

	if out.Name == "" {
		return fmt.Errorf("strategy.name is required")
	}
	if out.Code == "" {
		return fmt.Errorf("strategy.code is required")
	}
	if out.Description == "" {
		return fmt.Errorf("strategy.description is required")
	}

	if out.Params == nil {
		out.Params = []stypes.StrategyParam{}
	}
	for i := range out.Params {
		p := &out.Params[i]
		p.Name = strings.TrimSpace(p.Name)
		if p.Name == "" {
			return fmt.Errorf("strategy.params[%d].name is required", i)
		}
		if !p.Type.Valid() {
			return fmt.Errorf("strategy.params[%d].type is invalid", i)
		}
	}

	if out.Signals == nil {
		out.Signals = []stypes.SignalDefinition{}
	}
	for i := range out.Signals {
		s := &out.Signals[i]
		s.ID = strings.TrimSpace(s.ID)
		if s.ID == "" {
			return fmt.Errorf("strategy.signals[%d].id is required", i)
		}
		if !s.Type.Valid() {
			return fmt.Errorf("strategy.signals[%d].type is invalid", i)
		}
		if !ctypes.SignalScope(s.Scope).Valid() {
			return fmt.Errorf("strategy.signals[%d].scope is invalid", i)
		}
	}
	return nil
}
