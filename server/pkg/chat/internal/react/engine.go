package react

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/shared"
	zlog "github.com/rs/zerolog"
	"github.com/wangliang139/NovaForge/server/pkg/chat/domain"
	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/capability"
	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/runtime"
	"github.com/wangliang139/mow/logger"
)

const (
	DefaultMaxSteps     = 8
	DefaultMaxToolCalls = 16
	DefaultToolTimeout  = 300 * time.Second
)

// Policy 可由配置注入；零值在 Run 内会回落到 Default* 常量。
type Policy struct {
	MaxSteps                 int
	MaxToolCalls             int
	ToolTimeout              time.Duration
	MaxConsecutiveToolErrors int // 0=关闭；达到后下一轮 tool_choice=none
}

func (p Policy) withDefaults() Policy {
	if p.MaxSteps <= 0 {
		p.MaxSteps = DefaultMaxSteps
	}
	if p.MaxToolCalls <= 0 {
		p.MaxToolCalls = DefaultMaxToolCalls
	}
	if p.ToolTimeout <= 0 {
		p.ToolTimeout = DefaultToolTimeout
	}
	if p.MaxConsecutiveToolErrors < 0 {
		p.MaxConsecutiveToolErrors = 0
	}
	return p
}

type Emitter func(eventType, phase string, delta map[string]any, meta map[string]any) error

type Dependencies struct {
	BuildChatParams func(model string, maxOut int32, messages []domain.ChatMessage) openai.ChatCompletionNewParams
	Complete        func(ctx context.Context, provider string, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
	// CompleteStream 非 nil 时优先用流式完成一轮推理：工具轮与最终答复的正文均可按 token 推送 EventText（append）。
	CompleteStream func(ctx context.Context, provider string, params openai.ChatCompletionNewParams) (*ssestream.Stream[openai.ChatCompletionChunk], error)
	SanitizeText   func(string) string
	Runtime        runtime.Runtime
}

type Input struct {
	ContextResult domain.ContextBuildResult
	Provider      string
	Model         string
	Policy        Policy
	Log           StreamLogFields
}

type Result struct {
	AnswerText         string
	Parts              []domain.DialogPart
	FinishReason       string
	StepCount          int32
	ToolCallCount      int32
	ActualModel        string
	ActualProvider     string
	PromptTokens       int32
	CompletionTokens   int32
	TotalTokens        int32
	ErrorCode          string
	ErrorMessage       string
	CompletedWithError bool
}

func emitWithState(emit Emitter, state domain.ExecutionState, typ, phase string, delta map[string]any, meta map[string]any) error {
	if meta == nil {
		meta = map[string]any{}
	}
	meta["executionState"] = string(state)
	return emit(typ, phase, delta, meta)
}

func Run(ctx context.Context, deps Dependencies, emit Emitter, in Input) Result {
	pol := in.Policy.withDefaults()
	state := domain.ExecIdle
	result := Result{
		ActualProvider: in.Provider,
		ActualModel:    in.Model,
		FinishReason:   "stop",
	}
	params := deps.BuildChatParams(in.Model, domain.MaxOutputTokens, in.ContextResult.Messages)
	params.ParallelToolCalls = openai.Bool(false)

	var finalText strings.Builder
	consecutiveFail := 0
	degradedNext := false

	metaBase := func() map[string]any {
		m := map[string]any{
			"truncated": in.ContextResult.ContextMeta.Truncated,
		}
		if in.ContextResult.ContextMeta.ToolsPromptCompact {
			m["toolsPromptCompact"] = true
		}
		if in.ContextResult.ContextMeta.SkillsPromptCompact {
			m["skillsPromptCompact"] = true
		}
		return m
	}

	for step := 1; step <= pol.MaxSteps; step++ {
		var roundDegraded bool
		if degradedNext {
			state = domain.ExecDegradedNoTools
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: openai.String("none"),
			}
			params.Tools = nil
			degradedNext = false
			roundDegraded = true
			logReact(in.Log, state, "react_degraded_no_tools_round", func(e *zlog.Event) {
				e.Int("step", step).Int("consecutive_tool_failures", consecutiveFail)
			})
		} else {
			state = domain.ExecModelCalling
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: openai.String("auto"),
			}
			params.Tools = buildOpenAITools(capability.ListCallableToolsFull())
		}

		result.StepCount = int32(step)
		_ = emitWithState(emit, state, domain.EventThinking, "thinking", map[string]any{
			"text":   fmt.Sprintf("step %d: 正在分析并决定是否调用工具...", step),
			"append": false,
		}, metaBase())
		result.Parts = append(result.Parts, domain.DialogPart{
			Type:           domain.EventThinking,
			ExecutionState: state,
			Text:           fmt.Sprintf("step %d: 正在分析并决定是否调用工具...", step),
			Append:         false,
		})

		logger.Ctx(ctx).Debug().Str("component", "chat.react").Any("request", params).Msg("llm request")

		tModel := time.Now()
		var choice openai.ChatCompletionChoice
		var roundContent strings.Builder

		if deps.CompleteStream != nil {
			streamParams := params
			streamParams.StreamOptions = openai.ChatCompletionStreamOptionsParam{
				IncludeUsage: openai.Bool(true),
			}
			llmStream, sErr := deps.CompleteStream(ctx, in.Provider, streamParams)
			if sErr != nil {
				recordModelLatency(ctx, time.Since(tModel).Seconds())
				state = domain.ExecFailed
				result.CompletedWithError = true
				result.ErrorCode = "provider_error"
				result.ErrorMessage = sErr.Error()
				recordStep(ctx, "provider_error")
				logReact(in.Log, state, "react_stream_open_error", func(e *zlog.Event) {
					e.Int("step", step).Int64("duration_ms", time.Since(tModel).Milliseconds()).Err(sErr)
				})
				return result
			}
			defer llmStream.Close()

			var acc openai.ChatCompletionAccumulator
			var streamErr error
			for llmStream.Next() {
				chunk := llmStream.Current()
				if !acc.AddChunk(chunk) {
					streamErr = fmt.Errorf("stream accumulate failed (id mismatch or invalid chunk)")
					break
				}
				if chunk.Model != "" {
					result.ActualModel = chunk.Model
				}
				if chunk.Usage.TotalTokens > 0 {
					result.PromptTokens = int32(chunk.Usage.PromptTokens)
					result.CompletionTokens = int32(chunk.Usage.CompletionTokens)
					result.TotalTokens = int32(chunk.Usage.TotalTokens)
				}
				for _, ch := range chunk.Choices {
					if ch.FinishReason != "" {
						result.FinishReason = string(ch.FinishReason)
					}
					if ch.Delta.Content != "" {
						t := deps.SanitizeText(ch.Delta.Content)
						if t != "" {
							roundContent.WriteString(t)
							_ = emitWithState(emit, domain.ExecStreamingAnswer, domain.EventText, "final", map[string]any{
								"text":   t,
								"append": true,
							}, metaBase())
						}
					}
					if ch.Delta.Refusal != "" {
						t := deps.SanitizeText(ch.Delta.Refusal)
						if t != "" {
							roundContent.WriteString(t)
							_ = emitWithState(emit, domain.ExecStreamingAnswer, domain.EventText, "final", map[string]any{
								"text":   t,
								"append": true,
							}, metaBase())
						}
					}
				}
			}
			if streamErr == nil {
				streamErr = llmStream.Err()
			}
			recordModelLatency(ctx, time.Since(tModel).Seconds())
			logger.Ctx(ctx).Err(streamErr).Str("component", "chat.react").Any("accumulated", acc.ChatCompletion).Msg("llm stream done")

			if streamErr != nil {
				state = domain.ExecFailed
				result.CompletedWithError = true
				result.ErrorCode = "provider_error"
				result.ErrorMessage = streamErr.Error()
				recordStep(ctx, "provider_error")
				logReact(in.Log, state, "react_stream_error", func(e *zlog.Event) {
					e.Int("step", step).Int64("duration_ms", time.Since(tModel).Milliseconds()).Err(streamErr)
				})
				return result
			}
			if len(acc.Choices) == 0 {
				state = domain.ExecFailed
				result.CompletedWithError = true
				result.ErrorCode = "empty_choices"
				result.ErrorMessage = "provider returned empty choices"
				recordStep(ctx, "empty_choices")
				logReact(in.Log, state, "react_empty_choices", func(e *zlog.Event) {
					e.Int("step", step).Int64("duration_ms", time.Since(tModel).Milliseconds())
				})
				return result
			}
			choice = acc.Choices[0]
			if acc.Usage.TotalTokens > 0 {
				result.PromptTokens = int32(acc.Usage.PromptTokens)
				result.CompletionTokens = int32(acc.Usage.CompletionTokens)
				result.TotalTokens = int32(acc.Usage.TotalTokens)
			}
			if acc.Model != "" {
				result.ActualModel = acc.Model
			}
			if choice.FinishReason != "" {
				result.FinishReason = string(choice.FinishReason)
			}
		} else {
			resp, err := deps.Complete(ctx, in.Provider, params)
			logger.Ctx(ctx).Err(err).Str("component", "chat.react").Any("response", resp).Msg("llm response")

			recordModelLatency(ctx, time.Since(tModel).Seconds())
			if err != nil {
				state = domain.ExecFailed
				result.CompletedWithError = true
				result.ErrorCode = "provider_error"
				result.ErrorMessage = err.Error()
				recordStep(ctx, "provider_error")
				logReact(in.Log, state, "react_provider_error", func(e *zlog.Event) {
					e.Int("step", step).Int64("duration_ms", time.Since(tModel).Milliseconds()).Err(err)
				})
				return result
			}
			if resp.Model != "" {
				result.ActualModel = resp.Model
			}
			if resp.Usage.TotalTokens > 0 {
				result.PromptTokens = int32(resp.Usage.PromptTokens)
				result.CompletionTokens = int32(resp.Usage.CompletionTokens)
				result.TotalTokens = int32(resp.Usage.TotalTokens)
			}
			if len(resp.Choices) == 0 {
				state = domain.ExecFailed
				result.CompletedWithError = true
				result.ErrorCode = "empty_choices"
				result.ErrorMessage = "provider returned empty choices"
				recordStep(ctx, "empty_choices")
				logReact(in.Log, state, "react_empty_choices", func(e *zlog.Event) {
					e.Int("step", step).Int64("duration_ms", time.Since(tModel).Milliseconds())
				})
				return result
			}
			choice = resp.Choices[0]
			if choice.FinishReason != "" {
				result.FinishReason = string(choice.FinishReason)
			}
		}

		if roundDegraded {
			consecutiveFail = 0
		}

		modelRoundMs := time.Since(tModel).Milliseconds()
		if len(choice.Message.ToolCalls) > 0 {
			state = domain.ExecAwaitingTools
			recordStep(ctx, "tool_calls")
			logReact(in.Log, state, "react_model_returned_tools", func(e *zlog.Event) {
				e.Int("step", step).Int64("duration_ms", modelRoundMs).Int("tool_call_batch", len(choice.Message.ToolCalls))
			})
			toolRoundPreamble := strings.TrimSpace(roundContent.String())
			if toolRoundPreamble == "" {
				toolRoundPreamble = strings.TrimSpace(deps.SanitizeText(choice.Message.Content))
			}
			if toolRoundPreamble != "" {
				result.Parts = append(result.Parts, domain.DialogPart{
					Type:           domain.EventText,
					ExecutionState: domain.ExecAwaitingTools,
					Text:           toolRoundPreamble,
					Append:         false,
				})
			}
			// 必须先写入本轮 assistant（含 tool_calls），再依次写入各 tool 结果；否则后续请求里工具输出悬空，
			// 模型无法关联「曾发起过哪些调用」，极易对同一工具重复发起相同或近似请求直至 step 上限。
			normalizeToolCallUnions(&choice.Message)
			params.Messages = append(params.Messages, choice.Message.ToParam())
			for _, tc := range choice.Message.ToolCalls {
				if result.ToolCallCount >= int32(pol.MaxToolCalls) {
					state = domain.ExecFailed
					result.CompletedWithError = true
					result.ErrorCode = "loop_limit"
					result.ErrorMessage = "tool call limit reached"
					recordStep(ctx, "loop_limit_tools")
					logReact(in.Log, state, "react_tool_limit", func(e *zlog.Event) {
						e.Int("step", step).Int32("tool_calls", result.ToolCallCount)
					})
					return result
				}
				toolCallID, toolName, argStr := toolCallFromUnion(tc)
				toolArgs, parseErr := runtime.ParseToolArguments(argStr)

				result.ToolCallCount++
				state = domain.ExecToolRunning
				_ = emitWithState(emit, state, domain.EventToolCall, "action", map[string]any{
					"toolCallId": toolCallID,
					"toolName":   toolName,
					"arguments":  toolArgs,
					"status":     "started",
				}, metaBase())
				result.Parts = append(result.Parts, domain.DialogPart{
					Type:           domain.EventToolCall,
					ExecutionState: state,
					ToolCallID:     toolCallID,
					ToolName:       toolName,
					Arguments:      toolArgs,
					Status:         "started",
				})

				tTool := time.Now()
				if parseErr != nil {
					synErr := runtime.SyntheticToolErrorFromParse(parseErr)
					code := runtime.ErrorCodeFromError(synErr)
					recordToolCall(ctx, "error")
					recordToolFailure(ctx, code)
					recordToolLatency(ctx, time.Since(tTool).Seconds())
					if runtime.IsRecoverableToolError(synErr) {
						consecutiveFail++
					}
					state = domain.ExecToolObserved
					_ = emitWithState(emit, state, domain.EventToolResult, "observation", map[string]any{
						"toolCallId":  toolCallID,
						"toolName":    toolName,
						"message":     synErr.Error(),
						"status":      "error",
						"errorCode":   code,
						"recoverable": true,
					}, metaBase())
					result.Parts = append(result.Parts, domain.DialogPart{
						Type:           domain.EventToolResult,
						ExecutionState: state,
						ToolCallID:     toolCallID,
						ToolName:       toolName,
						Message:        synErr.Error(),
						Status:         "error",
					})
					params.Messages = append(params.Messages, openai.ChatCompletionMessageParamUnion{
						OfTool: &openai.ChatCompletionToolMessageParam{
							ToolCallID: toolCallID,
							Content: openai.ChatCompletionToolMessageParamContentUnion{
								OfString: openai.String(runtime.FormatSyntheticParseError(parseErr)),
							},
						},
					})
					logReact(in.Log, state, "react_tool_parse_error", func(e *zlog.Event) {
						e.Int("step", step).Str("tool", toolName).Str("tool_call_id", toolCallID).Str("error_code", code).
							Int("consecutive_failures", consecutiveFail).Int64("duration_ms", time.Since(tTool).Milliseconds())
					})
					continue
				}

				toolCtx, cancel := context.WithTimeout(ctx, pol.ToolTimeout)
				toolRes, toolErr := deps.Runtime.Call(toolCtx, toolName, toolArgs)
				cancel()
				recordToolLatency(ctx, time.Since(tTool).Seconds())

				if toolErr != nil {
					code := runtime.ErrorCodeFromError(toolErr)
					rec := runtime.IsRecoverableToolError(toolErr)
					recordToolCall(ctx, "error")
					recordToolFailure(ctx, code)
					if rec {
						consecutiveFail++
					}
					state = domain.ExecToolObserved
					_ = emitWithState(emit, state, domain.EventToolResult, "observation", map[string]any{
						"toolCallId":  toolCallID,
						"toolName":    toolName,
						"message":     toolErr.Error(),
						"status":      "error",
						"errorCode":   code,
						"recoverable": rec,
					}, metaBase())
					result.Parts = append(result.Parts, domain.DialogPart{
						Type:           domain.EventToolResult,
						ExecutionState: state,
						ToolCallID:     toolCallID,
						ToolName:       toolName,
						Message:        toolErr.Error(),
						Status:         "error",
					})
					params.Messages = append(params.Messages, openai.ChatCompletionMessageParamUnion{
						OfTool: &openai.ChatCompletionToolMessageParam{
							ToolCallID: toolCallID,
							Content: openai.ChatCompletionToolMessageParamContentUnion{
								OfString: openai.String(runtime.FormatToolError(toolErr)),
							},
						},
					})
					logReact(in.Log, state, "react_tool_error", func(e *zlog.Event) {
						e.Int("step", step).Str("tool", toolName).Str("tool_call_id", toolCallID).Str("error_code", code).
							Bool("recoverable", rec).Int("consecutive_failures", consecutiveFail).
							Int64("duration_ms", time.Since(tTool).Milliseconds())
					})
					continue
				}

				consecutiveFail = 0
				recordToolCall(ctx, "ok")
				state = domain.ExecToolObserved
				_ = emitWithState(emit, state, domain.EventToolCall, "action", map[string]any{
					"toolCallId": toolCallID,
					"toolName":   toolName,
					"arguments":  toolArgs,
					"status":     "completed",
				}, metaBase())
				_ = emitWithState(emit, state, domain.EventToolResult, "observation", map[string]any{
					"toolCallId": toolCallID,
					"toolName":   toolName,
					"result":     toolRes,
					"status":     "completed",
				}, metaBase())
				result.Parts = append(result.Parts,
					domain.DialogPart{Type: domain.EventToolCall, ExecutionState: state, ToolCallID: toolCallID, ToolName: toolName, Arguments: toolArgs, Status: "completed"},
					domain.DialogPart{Type: domain.EventToolResult, ExecutionState: state, ToolCallID: toolCallID, ToolName: toolName, Result: toolRes, Status: "completed"},
				)
				params.Messages = append(params.Messages, openai.ChatCompletionMessageParamUnion{
					OfTool: &openai.ChatCompletionToolMessageParam{
						ToolCallID: toolCallID,
						Content: openai.ChatCompletionToolMessageParamContentUnion{
							OfString: openai.String(runtime.MarshalToolResult(toolRes)),
						},
					},
				})
			}
			if pol.MaxConsecutiveToolErrors > 0 && consecutiveFail >= pol.MaxConsecutiveToolErrors {
				degradedNext = true
				logReact(in.Log, domain.ExecAwaitingTools, "react_scheduling_degraded_round", func(e *zlog.Event) {
					e.Int("step", step).Int("threshold", pol.MaxConsecutiveToolErrors)
				})
			}
			continue
		}

		state = domain.ExecStreamingAnswer
		var text string
		if deps.CompleteStream != nil {
			text = strings.TrimSpace(roundContent.String())
		} else {
			text = deps.SanitizeText(choice.Message.Content)
		}
		if text != "" {
			finalText.WriteString(text)
			if deps.CompleteStream == nil {
				_ = emitWithState(emit, state, domain.EventText, "final", map[string]any{
					"text":   text,
					"append": true,
				}, metaBase())
			}
			result.Parts = append(result.Parts, domain.DialogPart{
				Type:           domain.EventText,
				ExecutionState: state,
				Text:           text,
				Append:         true,
			})
		}
		consecutiveFail = 0
		result.AnswerText = strings.TrimSpace(finalText.String())
		state = domain.ExecCompleted
		recordStep(ctx, "completed_text")
		logReact(in.Log, state, "react_completed", func(e *zlog.Event) {
			e.Int("step", step).Str("finish_reason", result.FinishReason).Int32("tool_calls", result.ToolCallCount).
				Int64("duration_ms", modelRoundMs)
		})
		return result
	}

	state = domain.ExecFailed
	result.CompletedWithError = true
	result.ErrorCode = "loop_limit"
	result.ErrorMessage = "react step limit reached"
	recordStep(ctx, "loop_limit_steps")
	logReact(in.Log, state, "react_step_limit", func(e *zlog.Event) {
		e.Int("max_steps", pol.MaxSteps)
	})
	return result
}

// toolCallFromUnion 从 ToolCall 并集读取 id、名称与参数字符串。
// 流式 ChatCompletionAccumulator 只把增量写在 union 的 Function/Custom 字段上，往往不填充 JSON.raw；
// 若使用 AsFunction()（依赖 raw 反序列化）会得到空的 Name，前端即显示「未知工具」。
func toolCallFromUnion(tc openai.ChatCompletionMessageToolCallUnion) (toolCallID, toolName, argStr string) {
	toolCallID = strings.TrimSpace(tc.ID)
	toolName = strings.TrimSpace(tc.Function.Name)
	if toolName == "" {
		toolName = strings.TrimSpace(tc.Custom.Name)
	}
	argStr = tc.Function.Arguments
	if strings.TrimSpace(argStr) == "" {
		argStr = tc.Custom.Input
	}
	return toolCallID, toolName, argStr
}

// normalizeToolCallUnions 为累加得到的 tool call 补全 Type，否则 ToAssistantMessageParam 里 AsAny() 为 nil，
// 下一轮请求会丢失 assistant.tool_calls。
func normalizeToolCallUnions(msg *openai.ChatCompletionMessage) {
	if msg == nil {
		return
	}
	for i := range msg.ToolCalls {
		tc := &msg.ToolCalls[i]
		if strings.TrimSpace(tc.Type) != "" {
			continue
		}
		if strings.TrimSpace(tc.Function.Name) != "" || strings.TrimSpace(tc.Function.Arguments) != "" {
			tc.Type = "function"
			continue
		}
		if strings.TrimSpace(tc.Custom.Name) != "" || strings.TrimSpace(tc.Custom.Input) != "" {
			tc.Type = "custom"
		}
	}
}

func buildOpenAITools(defs []capability.ToolDef) []openai.ChatCompletionToolUnionParam {
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(defs))
	for _, def := range defs {
		out = append(out, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        def.Name,
			Description: openai.String(def.Description),
			Parameters:  def.InputSchema,
		}))
	}
	return out
}
