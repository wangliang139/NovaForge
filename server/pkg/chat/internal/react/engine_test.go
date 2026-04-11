package react

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/wangliang139/NovaForge/server/pkg/chat/domain"
	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/runtime"
)

func TestRun_SkillDetailToolCallThenFinalText(t *testing.T) {
	callN := 0
	deps := Dependencies{
		BuildChatParams: func(model string, maxOut int32, messages []domain.ChatMessage) openai.ChatCompletionNewParams {
			return openai.ChatCompletionNewParams{Model: model}
		},
		Complete: func(ctx context.Context, provider string, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			callN++
			if callN == 1 {
				return decodeCompletion(t, `{
					"id":"cmpl_1",
					"model":"test-model",
					"choices":[
						{
							"index":0,
							"finish_reason":"tool_calls",
							"message":{
								"role":"assistant",
								"tool_calls":[
									{
										"id":"call_1",
										"type":"function",
										"function":{
											"name":"get_skill_detail",
											"arguments":"{\"skill_name\":\"获取策略开发手册\"}"
										}
									}
								]
							}
						}
					],
					"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
				}`)
			}
			return decodeCompletion(t, `{
				"id":"cmpl_2",
				"model":"test-model",
				"choices":[
					{
						"index":0,
						"finish_reason":"stop",
						"message":{
							"role":"assistant",
							"content":"已获取策略开发手册，请按模板开始。"
						}
					}
				],
				"usage":{"prompt_tokens":20,"completion_tokens":10,"total_tokens":30}
			}`)
		},
		SanitizeText: func(s string) string { return s },
		Runtime:      runtime.NewWhitelistRuntime(),
	}

	var events []domain.DeltaEvent
	emit := func(eventType, phase string, delta map[string]any, meta map[string]any) error {
		events = append(events, domain.DeltaEvent{
			Type:  eventType,
			Phase: phase,
			Delta: delta,
			Meta:  meta,
		})
		return nil
	}

	out := Run(context.Background(), deps, emit, Input{
		ContextResult: domain.ContextBuildResult{
			Messages: []domain.ChatMessage{{Role: "user", Content: "请给我策略开发手册"}},
		},
		Provider: "openrouter",
		Model:    "minimax/minimax-m2.7",
	})

	if out.CompletedWithError {
		t.Fatalf("unexpected error: %s", out.ErrorMessage)
	}
	if out.ToolCallCount < 1 {
		t.Fatalf("expected at least 1 tool call, got %d", out.ToolCallCount)
	}
	if out.AnswerText == "" {
		t.Fatalf("expected non-empty final answer")
	}

	hasToolCall := false
	hasToolResult := false
	hasFinalText := false
	for _, e := range events {
		if e.Type == domain.EventToolCall && e.Delta["toolName"] == "get_skill_detail" {
			hasToolCall = true
		}
		if e.Type == domain.EventToolResult && e.Delta["toolName"] == "get_skill_detail" {
			hasToolResult = true
		}
		if e.Type == domain.EventText {
			hasFinalText = true
		}
	}
	if !hasToolCall || !hasToolResult || !hasFinalText {
		t.Fatalf("missing expected event chain, toolCall=%v toolResult=%v finalText=%v", hasToolCall, hasToolResult, hasFinalText)
	}
	var sawState bool
	for _, e := range events {
		if e.Meta != nil && e.Meta["executionState"] != nil {
			sawState = true
			break
		}
	}
	if !sawState {
		t.Fatalf("expected executionState in event meta")
	}
}

func TestRun_ToolErrorThenFinalText(t *testing.T) {
	callN := 0
	deps := Dependencies{
		BuildChatParams: func(model string, maxOut int32, messages []domain.ChatMessage) openai.ChatCompletionNewParams {
			return openai.ChatCompletionNewParams{Model: model}
		},
		Complete: func(ctx context.Context, provider string, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
			callN++
			if callN == 1 {
				return decodeCompletion(t, `{
					"id":"cmpl_1",
					"model":"test-model",
					"choices":[{
						"index":0,
						"finish_reason":"tool_calls",
						"message":{
							"role":"assistant",
							"tool_calls":[{
								"id":"call_bad",
								"type":"function",
								"function":{
									"name":"echo_json",
									"arguments":"{}"
								}
							}]
						}
					}],
					"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
				}`)
			}
			return decodeCompletion(t, `{
				"id":"cmpl_2",
				"model":"test-model",
				"choices":[{
					"index":0,
					"finish_reason":"stop",
					"message":{"role":"assistant","content":"工具失败已处理，继续作答。"}
				}],
				"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}
			}`)
		},
		SanitizeText: func(s string) string { return s },
		Runtime:      runtime.NewWhitelistRuntime(),
	}
	var toolResults []domain.DeltaEvent
	emit := func(eventType, phase string, delta map[string]any, meta map[string]any) error {
		if eventType == domain.EventToolResult {
			toolResults = append(toolResults, domain.DeltaEvent{Type: eventType, Delta: delta, Meta: meta})
		}
		return nil
	}
	out := Run(context.Background(), deps, emit, Input{
		ContextResult: domain.ContextBuildResult{
			Messages: []domain.ChatMessage{{Role: "user", Content: "测试"}},
		},
		Provider: "openrouter",
		Model:    "test-model",
	})
	if out.CompletedWithError {
		t.Fatalf("unexpected error: %s / %s", out.ErrorCode, out.ErrorMessage)
	}
	if out.AnswerText == "" {
		t.Fatalf("expected answer after recoverable tool error")
	}
	if len(toolResults) < 1 || toolResults[0].Delta["recoverable"] != true {
		t.Fatalf("expected recoverable tool_result, got %#v", toolResults)
	}
}

func decodeCompletion(t *testing.T, raw string) (*openai.ChatCompletion, error) {
	t.Helper()
	var out openai.ChatCompletion
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("failed to decode completion json: %v", err)
	}
	return &out, nil
}

// 模拟 ChatCompletionAccumulator：只填充 union 内联字段，不依赖 JSON.raw（AsFunction 会读不到名称）。
func TestToolCallFromUnion_accumulatorStyle(t *testing.T) {
	tc := openai.ChatCompletionMessageToolCallUnion{
		ID: "call_x",
		Function: openai.ChatCompletionMessageFunctionToolCallFunction{
			Name:      "skill.search_documents",
			Arguments: `{"query":"btc"}`,
		},
	}
	id, name, args := toolCallFromUnion(tc)
	if id != "call_x" || name != "skill.search_documents" || args != `{"query":"btc"}` {
		t.Fatalf("got id=%q name=%q args=%q", id, name, args)
	}
	msg := openai.ChatCompletionMessage{ToolCalls: []openai.ChatCompletionMessageToolCallUnion{tc}}
	normalizeToolCallUnions(&msg)
	if msg.ToolCalls[0].Type != "function" {
		t.Fatalf("expected type function, got %q", msg.ToolCalls[0].Type)
	}
}

func TestNormalizeToolCallUnions_custom(t *testing.T) {
	tc := openai.ChatCompletionMessageToolCallUnion{
		ID: "c1",
		Custom: openai.ChatCompletionMessageCustomToolCallCustom{
			Name:  "my_tool",
			Input: "{}",
		},
	}
	msg := openai.ChatCompletionMessage{ToolCalls: []openai.ChatCompletionMessageToolCallUnion{tc}}
	normalizeToolCallUnions(&msg)
	if msg.ToolCalls[0].Type != "custom" {
		t.Fatalf("expected type custom, got %q", msg.ToolCalls[0].Type)
	}
	id, name, args := toolCallFromUnion(msg.ToolCalls[0])
	if name != "my_tool" || args != "{}" {
		t.Fatalf("custom tool: name=%q args=%q", name, args)
	}
	_ = id
}
