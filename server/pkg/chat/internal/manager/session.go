package manager

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/stumble/wpgx"
	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
	llmentity "github.com/wangliang139/llt-trade/server/pkg/entity/llm"
	repo_session "github.com/wangliang139/llt-trade/server/pkg/repos/llm_session"
	"github.com/wangliang139/mow/logger"
)

func isPlaceholderSessionTitle(title string) bool {
	t := strings.TrimSpace(title)
	return t == "" || t == "新对话"
}

func (m *Manager) buildSessionTitleFromQuestion(ctx context.Context, question string) string {
	qText := strings.TrimSpace(question)
	if len([]rune(qText)) <= domain.SessionTitleSummaryThreshold {
		return SuggestSessionTitle(qText)
	}
	// 长问题再交给 LLM 做标题压缩；始终仅基于 question。
	if m.llm == nil {
		return SuggestSessionTitle(qText)
	}
	resp, err := m.llm.Completion(ctx, &llmentity.CompletionRequest{
		SceneKey: domain.SceneKeyAISessionTitle,
		Variables: map[string]any{
			"question": qText,
		},
	})
	if err != nil {
		logger.Ctx(ctx).Err(err).Str("scene", domain.SceneKeyAISessionTitle).Msg("session title: llm completion failed")
		return SuggestSessionTitle(qText)
	}
	title := strings.TrimSpace(resp.Result)
	if title == "" {
		return SuggestSessionTitle(qText)
	}
	return ClampSessionTitle(title)
}

// GenerateSessionTitle 在首轮问答完成后生成标题。
// 仅在标题仍为占位时生效；若首轮未完成则静默跳过。
func (m *Manager) GenerateSessionTitle(ctx context.Context, userID, sessionID int64) (string, error) {
	session, err := m.RequireSession(ctx, userID, sessionID)
	if err != nil {
		return "", err
	}
	if !isPlaceholderSessionTitle(session.Title) {
		return strings.TrimSpace(session.Title), nil
	}

	dialogs, err := m.db.LlmDialogRepo.ListBySessionID(ctx, sessionID)
	if err != nil {
		return "", err
	}

	var firstQuestionID int64
	var firstQuestionText string
	for i := range dialogs {
		d := dialogs[i]
		if d.Role == domain.RoleQuestion && d.Seq == 1 && d.Visible {
			firstQuestionID = d.ID
			firstQuestionText = strings.TrimSpace(d.ContentText)
			break
		}
	}
	if firstQuestionID <= 0 || firstQuestionText == "" {
		return strings.TrimSpace(session.Title), nil
	}

	firstTurnCompleted := false
	for i := range dialogs {
		d := dialogs[i]
		if d.Role == domain.RoleAnswer && d.DialogID == firstQuestionID && d.Visible && d.Status == domain.StatusCompleted {
			firstTurnCompleted = true
			break
		}
	}
	if !firstTurnCompleted {
		return strings.TrimSpace(session.Title), nil
	}

	title := m.buildSessionTitleFromQuestion(ctx, firstQuestionText)

	latest, err := m.db.LlmSessionRepo.GetByID(ctx, sessionID)
	if err != nil || latest == nil {
		return "", err
	}
	if !isPlaceholderSessionTitle(latest.Title) {
		return strings.TrimSpace(latest.Title), nil
	}
	if _, err := m.db.LlmSessionRepo.UpdateTitle(ctx, repo_session.UpdateTitleParams{
		ID:    sessionID,
		Title: title,
	}); err != nil {
		logger.Ctx(ctx).Err(err).Int64("session_id", sessionID).Msg("session title: update failed")
		return "", err
	}
	return title, nil
}

func (m *Manager) UpdateSessionTitle(ctx context.Context, userID, sessionID int64, title string) (string, error) {
	if _, err := m.RequireSession(ctx, userID, sessionID); err != nil {
		return "", err
	}
	nextTitle := ClampSessionTitle(title)
	if _, err := m.db.LlmSessionRepo.UpdateTitle(ctx, repo_session.UpdateTitleParams{
		ID:    sessionID,
		Title: nextTitle,
	}); err != nil {
		return "", err
	}
	return nextTitle, nil
}

func (m *Manager) DeleteSession(ctx context.Context, userID, sessionID int64) error {
	_, err := m.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		sessionRepo := m.db.LlmSessionRepo.WithTx(tx)
		dialogRepo := m.db.LlmDialogRepo.WithTx(tx)

		session, err := sessionRepo.LockByID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		// 已删除或不存在，按幂等删除处理为成功。
		if session == nil {
			return nil, nil
		}
		if session.UserID != userID {
			return nil, fmt.Errorf("forbidden")
		}
		if _, err := dialogRepo.DeleteBySessionID(ctx, sessionID); err != nil {
			return nil, err
		}
		if _, err := sessionRepo.DeleteByID(ctx, sessionID); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return err
}

func SuggestSessionTitle(content string) string {
	return ClampSessionTitle(strings.TrimSpace(content))
}

// ClampSessionTitle 将标题截断为适合列表展示的宽度；空字符串会得到「新对话」。
func ClampSessionTitle(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "新对话"
	}
	runes := []rune(trimmed)
	if len(runes) <= domain.SessionTitleSummaryThreshold {
		return trimmed
	}
	return string(runes[:domain.SessionTitleSummaryThreshold]) + "..."
}
