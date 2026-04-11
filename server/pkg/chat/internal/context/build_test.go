package chatctx

import (
	"strings"
	"testing"

	chatcfg "github.com/wangliang139/llt-trade/server/pkg/chat/internal/config"
	repo_session "github.com/wangliang139/llt-trade/server/pkg/repos/llm_session"
	// 触发 tools.init()，将内置能力注册到 capability 注册表。
	_ "github.com/wangliang139/llt-trade/server/pkg/chat/internal/tools"
)

func TestBuild_toolsPromptBudgetUsesCompactLayer(t *testing.T) {
	session := &repo_session.LlmSession{}
	cfg := chatcfg.Default()
	cfg.ToolsPromptBudgetBytes = 80
	out := Build(session, nil, 0, cfg)
	if !out.ContextMeta.ToolsPromptCompact {
		t.Fatalf("expected ToolsPromptCompact when budget is tiny")
	}
	var toolsLine string
	for _, m := range out.Messages {
		if m.Role == "system" && strings.HasPrefix(m.Content, "TOOLS_FULL(JSON): ") {
			toolsLine = strings.TrimPrefix(m.Content, "TOOLS_FULL(JSON): ")
			break
		}
	}
	if toolsLine == "" {
		t.Fatalf("tools message not found")
	}
	// 极限预算下应为仅工具名数组或带 note 的降级对象
	if !strings.Contains(toolsLine, "now_iso8601") {
		t.Fatalf("expected tool names in degraded prompt, got %q", toolsLine)
	}
}

func TestBuild_skillsPromptBudgetMarksCompact(t *testing.T) {
	session := &repo_session.LlmSession{}
	cfg := chatcfg.Default()
	cfg.SkillsPromptBudgetBytes = 20
	out := Build(session, nil, 0, cfg)
	if !out.ContextMeta.SkillsPromptCompact {
		t.Fatalf("expected SkillsPromptCompact with tiny skills budget")
	}
}
