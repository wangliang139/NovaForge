package react

import (
	"strings"

	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/capability"
	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/tools"
)

func shouldInvokeBuiltinTool(question string) bool {
	q := strings.ToLower(strings.TrimSpace(question))
	if q == "" {
		return false
	}
	for _, s := range capability.ListSkillsBrief() {
		name := strings.ToLower(s.Name)
		desc := strings.ToLower(s.Description)
		if strings.Contains(q, name) || (desc != "" && strings.Contains(q, desc)) {
			return true
		}
	}
	return strings.Contains(q, "当前时间") ||
		strings.Contains(q, "现在几点") ||
		strings.Contains(q, "utc 时间") ||
		strings.Contains(q, "json 回显") ||
		strings.Contains(q, "手册") ||
		strings.Contains(q, "skill")
}

func selectBuiltinTool(question string) (string, map[string]any) {
	q := strings.ToLower(strings.TrimSpace(question))
	for _, s := range capability.ListSkillsBrief() {
		n := strings.ToLower(s.Name)
		if strings.Contains(q, n) || strings.Contains(q, "手册") || strings.Contains(q, "skill") {
			return tools.ToolGetSkillDetail, map[string]any{"skill_name": s.Name}
		}
	}
	if strings.Contains(q, "json 回显") {
		return tools.ToolEchoJSON, map[string]any{"payload": map[string]any{"question": question}}
	}
	return tools.ToolNowISO8601, map[string]any{}
}
