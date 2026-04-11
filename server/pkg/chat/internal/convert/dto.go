package convert

import (
	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
	repo_dialog "github.com/wangliang139/llt-trade/server/pkg/repos/llm_dialog"
	repo_session "github.com/wangliang139/llt-trade/server/pkg/repos/llm_session"
)

func SessionDTOFromRepo(row *repo_session.LlmSession) domain.SessionDTO {
	lastDialogID := ""
	if row.LastDialogID > 0 {
		lastDialogID = domain.Int64String(row.LastDialogID)
	}
	return domain.SessionDTO{
		ID:           domain.Int64String(row.ID),
		UserID:       domain.Int64String(row.UserID),
		Title:        row.Title,
		Status:       row.Status,
		Summary:      row.Summary,
		LastDialogID: lastDialogID,
		DialogCount:  row.DialogCount,
		TurnCount:    row.TurnCount,
		LastDialogAt: domain.ToUnix(row.LastDialogAt),
		CreatedAt:    row.CreatedAt.Unix(),
		UpdatedAt:    row.UpdatedAt.Unix(),
	}
}

func DialogDTOFromRepo(row *repo_dialog.LlmDialog) domain.DialogDTO {
	st := repo_dialog.ParseDialogStats(row.Stats)
	return domain.DialogDTO{
		ID:               domain.Int64String(row.ID),
		SessionID:        domain.Int64String(row.SessionID),
		DialogID:         domain.Int64String(row.DialogID),
		Role:             row.Role,
		Status:           row.Status,
		ContentText:      row.ContentText,
		Parts:            domain.DecodeParts(row.Parts),
		ContextMeta:      domain.DecodeContextMeta(row.ContextMeta),
		Seq:              row.Seq,
		Provider:         row.Provider,
		Model:            row.Model,
		PromptTokens:     st.PromptTokens,
		CompletionTokens: st.CompletionTokens,
		TotalTokens:      st.TotalTokens,
		CanRegenerate:    row.CanRegenerate,
		ErrorCode:        row.ErrorCode,
		ErrorMessage:     row.ErrorMessage,
		StartedAt:        domain.ToUnix(row.StartedAt),
		CompletedAt:      domain.ToUnix(row.CompletedAt),
		CreatedAt:        row.CreatedAt.Unix(),
		UpdatedAt:        row.UpdatedAt.Unix(),
	}
}
