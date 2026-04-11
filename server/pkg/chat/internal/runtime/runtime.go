package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/wangliang139/NovaForge/server/pkg/chat/domain"
	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/capability"
	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/tools"
)

// FormatToolError 将错误序列化为发给 LLM 的 JSON 文本（统一 code/message）。
func FormatToolError(err error) string {
	if err == nil {
		return `{}`
	}
	if errors.Is(err, context.DeadlineExceeded) {
		raw, _ := json.Marshal(map[string]string{
			"code":    "tool_timeout",
			"message": "tool execution timed out",
		})
		return string(raw)
	}
	var re *domain.RuntimeError
	if errors.As(err, &re) {
		raw, mErr := json.Marshal(map[string]string{"code": re.Code, "message": re.Message})
		if mErr != nil {
			return `{"code":"marshal_failed","message":"internal"}`
		}
		return string(raw)
	}
	raw, _ := json.Marshal(map[string]string{"code": "runtime_error", "message": err.Error()})
	return string(raw)
}



// Runtime 是工具/技能的统一调用接口。
type Runtime interface {
	Call(ctx context.Context, name string, args map[string]any) (any, error)
}

// WhitelistRuntime 根据 entries.go 中注册的 handler 分发调用。
type WhitelistRuntime struct {
	handlers map[string]func(context.Context, map[string]any) (any, error)
}

// NewRuntime 以给定依赖构建运行时，并同步更新 capability 注册表（供 prompt 构建使用）。
func NewRuntime(env domain.Env) *WhitelistRuntime {
	tools, skills := tools.BuildEntries()
	capability.SetRegistered(tools, skills)

	handlers := make(map[string]func(context.Context, map[string]any) (any, error), len(tools)+len(skills))
	for _, t := range tools {
		if t.Handler != nil {
			handler := t.Handler
			handlers[t.Name] = func(ctx context.Context, args map[string]any) (any, error) {
				return handler(ctx, args, env)
			}
		}
	}
	for _, s := range skills {
		if s.Name != "" && s.Handler != nil {
			handler := s.Handler
			handlers[s.Name] = func(ctx context.Context, args map[string]any) (any, error) {
				return handler(ctx, args, env)
			}
		}
	}
	return &WhitelistRuntime{handlers: handlers}
}

// NewWhitelistRuntime 创建无外部依赖的运行时（适用于测试或无需 KV 的场景）。
func NewWhitelistRuntime() *WhitelistRuntime {
	return NewRuntime(domain.Env{})
}

func (r *WhitelistRuntime) Call(ctx context.Context, name string, args map[string]any) (any, error) {
	handler, ok := r.handlers[name]
	if !ok {
		return nil, domain.NewRuntimeError("unsupported_tool", fmt.Sprintf("unsupported tool: %s", name))
	}
	return handler(ctx, args)
}

func ParseToolArguments(raw string) (map[string]any, error) {
	out := map[string]any{}
	if raw == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func MarshalToolResult(result any) string {
	if result == nil {
		return "{}"
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return `{"error":"marshal_tool_result_failed"}`
	}
	return string(raw)
}
