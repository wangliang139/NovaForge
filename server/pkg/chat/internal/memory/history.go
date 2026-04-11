package memory

import (
	"strings"

	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
	repo_dialog "github.com/wangliang139/llt-trade/server/pkg/repos/llm_dialog"
)

// AppendCompletedTurns 将目标回答之前的已完成问答轮次追加到 messages，并维护 IncludedDialogIDs。
func AppendCompletedTurns(
	dialogs []repo_dialog.LlmDialog,
	targetAnswerID int64,
	messages []domain.ChatMessage,
	meta *domain.ContextMeta,
) ([]domain.ChatMessage, string) {
	var questionText string
	for i := range dialogs {
		dialog := dialogs[i]
		if dialog.ID == targetAnswerID {
			break
		}
		if strings.TrimSpace(dialog.ContentText) == "" {
			continue
		}

		switch dialog.Role {
		case domain.RoleQuestion:
			messages = append(messages, domain.ChatMessage{
				Role:    "user",
				Content: dialog.ContentText,
			})
			questionText = dialog.ContentText
		case domain.RoleAnswer:
			messages = append(messages, domain.ChatMessage{
				Role:    "assistant",
				Content: dialog.ContentText,
			})
		default:
			continue
		}
		meta.IncludedDialogIDs = append(meta.IncludedDialogIDs, domain.Int64String(dialog.ID))
	}
	return messages, questionText
}
