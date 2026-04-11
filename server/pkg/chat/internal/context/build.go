package chatctx

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
	"github.com/wangliang139/llt-trade/server/pkg/chat/internal/capability"
	chatcfg "github.com/wangliang139/llt-trade/server/pkg/chat/internal/config"
	"github.com/wangliang139/llt-trade/server/pkg/chat/internal/memory"
	repo_dialog "github.com/wangliang139/llt-trade/server/pkg/repos/llm_dialog"
	repo_session "github.com/wangliang139/llt-trade/server/pkg/repos/llm_session"
)

const systemPrompt = `你是 LLT Trade 的智能助手。

要求：
1. 优先直接回答用户问题，内容清晰、准确、简洁。
2. 如果输出代码，请使用标准 Markdown fenced code block。
3. 如果无法确定答案，明确说明不确定之处。
4. 当前版本默认不主动暴露内部思维链。
5. 当你需要精确外部信息时，可以通过函数工具获取事实并结合结果作答；不需要工具时直接回答。
6. 在决定是否调用函数工具前，先检查对话中是否已经存在相同工具、相近参数的调用结果；如已存在且环境未明显变化，应优先复用历史结果并据此作答，避免无意义的重复调用同一工具。
7. 当你认为必须再次调用同一工具时，请在调用前用一句自然语言说明理由（例如“由于时间区间不同，需要重新检索最新数据”），然后再发起新的工具调用。`

func marshalJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func shortenRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxRunes]) + "…"
}

func toolsJSONForPrompt(budget int, meta *domain.ContextMeta) string {
	if budget <= 0 {
		return marshalJSON(capability.ListToolsFull())
	}
	full := marshalJSON(capability.ListToolsFull())
	if len(full) <= budget {
		return full
	}
	compact := marshalJSON(capability.ListToolsCompact())
	meta.ToolsPromptCompact = true
	meta.Truncated = true
	if len(compact) <= budget {
		return compact
	}
	names := marshalJSON(capability.ListToolNamesOnly())
	if len(names) <= budget {
		return names
	}
	return fmt.Sprintf(
		`{"toolNames":%s,"note":"system prompt budget exceeded; use attached function tools for schemas"}`,
		marshalJSON(capability.ListToolNamesOnly()),
	)
}

func skillsJSONForPrompt(budget int, meta *domain.ContextMeta) string {
	brief := capability.ListSkillsBrief()
	if budget <= 0 {
		return marshalJSON(brief)
	}
	raw := marshalJSON(brief)
	if len(raw) <= budget {
		return raw
	}
	short := make([]capability.SkillDef, 0, len(brief))
	for _, s := range brief {
		short = append(short, capability.SkillDef{
			Name:        s.Name,
			Description: shortenRunes(s.Description, 100),
		})
	}
	raw = marshalJSON(short)
	meta.SkillsPromptCompact = true
	meta.Truncated = true
	if len(raw) <= budget {
		return raw
	}
	type nameOnly struct {
		Name string `json:"name"`
	}
	narrow := make([]nameOnly, 0, len(brief))
	for _, s := range brief {
		narrow = append(narrow, nameOnly{Name: s.Name})
	}
	return marshalJSON(narrow)
}

// Build 组装发往 LLM 的上下文：系统提示、可选摘要、memory 中的历史轮次。
func Build(session *repo_session.LlmSession, dialogs []repo_dialog.LlmDialog, targetAnswerID int64, cfg chatcfg.Config) domain.ContextBuildResult {
	result := domain.ContextBuildResult{
		Messages: []domain.ChatMessage{
			{Role: "system", Content: systemPrompt},
		},
		ContextMeta: domain.ContextMeta{
			Strategy:             "recent_plus_summary",
			IncludedDialogIDs:    []string{},
			ToolContextIncluded:  false,
			ReservedOutputTokens: 0,
		},
	}
	now := time.Now()
	result.Messages = append(result.Messages, domain.ChatMessage{
		Role: "system",
		Content: fmt.Sprintf(
			"当前时间信息（默认可直接使用，无需额外工具）：utc=%s local=%s",
			now.UTC().Format(time.RFC3339),
			now.Format(time.RFC3339),
		),
	})
	toolsLine := toolsJSONForPrompt(cfg.ToolsPromptBudgetBytes, &result.ContextMeta)
	result.Messages = append(result.Messages, domain.ChatMessage{
		Role:    "system",
		Content: fmt.Sprintf("TOOLS_FULL(JSON): %s", toolsLine),
	})
	skillsLine := skillsJSONForPrompt(cfg.SkillsPromptBudgetBytes, &result.ContextMeta)
	result.Messages = append(result.Messages, domain.ChatMessage{
		Role:    "system",
		Content: fmt.Sprintf("SKILLS_BRIEF(JSON): %s", skillsLine),
	})
	result.Messages = append(result.Messages, domain.ChatMessage{
		Role:    "system",
		Content: "规则：当你需要某个 skill 的完整信息时，先调用 get_skill_detail(skill_name)；拿到 detail 后再选择具体 tool 执行。",
	})
	result.ContextMeta.ToolContextIncluded = true

	if summary := strings.TrimSpace(session.Summary); summary != "" {
		result.Messages = append(result.Messages, domain.ChatMessage{
			Role:    "system",
			Content: fmt.Sprintf("以下是历史对话摘要，请在必要时参考：\n%s", summary),
		})
		result.ContextMeta.SummaryUsed = true
	}

	result.Messages, result.QuestionText = memory.AppendCompletedTurns(
		dialogs, targetAnswerID, result.Messages, &result.ContextMeta,
	)
	result.ContextMeta.InputTokens = memory.EstimateInputTokens(result.Messages)
	return result
}
