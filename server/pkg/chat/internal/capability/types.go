package capability

import (
	"context"

	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
)

type Handler func(context.Context, map[string]any, domain.Env) (any, error)

// ToolDef 描述一个工具的 schema 及其运行时处理器。
// Handler 为 nil 时该条目仅用于 prompt 构建（只读元数据）。
type ToolDef struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema,omitempty"`
	OutputSchema map[string]any `json:"outputSchema,omitempty"`
	// Handler 是工具的执行函数，序列化时自动跳过。
	Handler Handler `json:"-"`
}

// SkillDef 描述一个技能的 schema 及其运行时处理器。
// CallName 为可被模型直接调用的函数名（建议以 "skill." 前缀命名空间隔离）。
// Handler 为 nil 时该条目仅用于 prompt 构建（只读元数据）。
type SkillDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
	Detail      map[string]any `json:"detail,omitempty"`
	// Handler 是技能的执行函数，序列化时自动跳过。
	Handler Handler `json:"-"`
}
