package capability

import (
	"strings"
	"sync"
)

var (
	mu               sync.RWMutex
	registeredTools  []ToolDef
	registeredSkills []SkillDef
)

// SetRegistered 覆盖能力注册表（工具 + 技能）。
// 由 tools 包在构建 Runtime 时调用，调用者保证 schema 字段完整。
func SetRegistered(tools []ToolDef, skills []SkillDef) {
	mu.Lock()
	defer mu.Unlock()
	registeredTools = tools
	registeredSkills = skills
}

func ListToolsFull() []ToolDef {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]ToolDef, len(registeredTools))
	copy(out, registeredTools)
	return out
}

// ListToolsCompact 仅保留 name/description 与最小 JSON Schema，用于 system prompt 预算降级。
func ListToolsCompact() []ToolDef {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]ToolDef, 0, len(registeredTools))
	for _, t := range registeredTools {
		out = append(out, ToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		})
	}
	return out
}

// ListToolNamesOnly 仅工具名列表（最小体积）。
func ListToolNamesOnly() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registeredTools))
	for _, t := range registeredTools {
		out = append(out, t.Name)
	}
	return out
}

func ListSkillsBrief() []SkillDef {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]SkillDef, 0, len(registeredSkills))
	for _, s := range registeredSkills {
		out = append(out, SkillDef{
			Name:        s.Name,
			Description: s.Description,
		})
	}
	return out
}

func GetSkillDetail(name string) (SkillDef, bool) {
	mu.RLock()
	defer mu.RUnlock()
	n := strings.TrimSpace(name)
	for _, s := range registeredSkills {
		if strings.EqualFold(strings.TrimSpace(s.Name), n) {
			return s, true
		}
		if alias, ok := s.Detail["display_name"].(string); ok {
			if strings.EqualFold(strings.TrimSpace(alias), n) {
				return s, true
			}
		}
	}
	return SkillDef{}, false
}

func GetSkillByName(name string) (SkillDef, bool) {
	mu.RLock()
	defer mu.RUnlock()
	n := strings.TrimSpace(name)
	for _, s := range registeredSkills {
		if strings.EqualFold(strings.TrimSpace(s.Name), n) {
			return s, true
		}
	}
	return SkillDef{}, false
}

// ListCallableToolsFull 返回模型可直接调用的 function tools：工具 + 技能（以 CallName 形式暴露）。
func ListCallableToolsFull() []ToolDef {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]ToolDef, 0, len(registeredTools)+len(registeredSkills))
	out = append(out, registeredTools...)
	for _, s := range registeredSkills {
		if strings.TrimSpace(s.Name) == "" {
			continue
		}
		schema := s.InputSchema
		if schema == nil {
			schema = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}
		out = append(out, ToolDef{
			Name:        s.Name,
			Description: s.Description,
			InputSchema: schema,
		})
	}
	return out
}
