package manager

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/jackc/pgx/v5"
	"github.com/openai/openai-go/v3"
	"github.com/stumble/wpgx"
	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
	"github.com/wangliang139/llt-trade/server/pkg/chat/internal/convert"
	llmentity "github.com/wangliang139/llt-trade/server/pkg/entity/llm"
	"github.com/wangliang139/llt-trade/server/pkg/internal/zai"
	"github.com/wangliang139/llt-trade/server/pkg/repos"
	repo_dialog "github.com/wangliang139/llt-trade/server/pkg/repos/llm_dialog"
	repo_session "github.com/wangliang139/llt-trade/server/pkg/repos/llm_session"
	"github.com/wangliang139/llt-trade/server/pkg/settings"
	"github.com/wangliang139/mow/snowflake"
)

type Manager struct {
	db     *repos.Entity
	engine *zai.Engine
	llm    llmEntity
}

// llmEntity 仅依赖 entity.Llm.Completion，便于测试注入。
type llmEntity interface {
	Completion(ctx context.Context, req *llmentity.CompletionRequest) (*llmentity.CompletionResponse, error)
}

func New(db *repos.Entity, engine *zai.Engine, llm llmEntity) *Manager {
	return &Manager{db: db, engine: engine, llm: llm}
}

func (m *Manager) DB() *repos.Entity { return m.db }

func (m *Manager) Engine() *zai.Engine { return m.engine }

func (m *Manager) ResolveTurnLlm(ctx context.Context, session *repo_session.LlmSession, reqModel string) (provider, model string) {
	provider = domain.DefaultProvider
	model = strings.TrimSpace(reqModel)
	if model == "" {
		if cfg, err := settings.GetLlmProviderConfig(ctx); err == nil {
			if mod := strings.TrimSpace(cfg.DefaultModel); mod != "" {
				model = mod
			}
		}
		if model == "" {
			model = domain.DefaultModel
		}
	}
	return provider, model
}

func (m *Manager) ListModels(ctx context.Context) ([]domain.LlmModelItem, error) {
	cfg, err := settings.GetLlmProviderConfig(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.OpenRouterAPIKey) == "" {
		return []domain.LlmModelItem{}, nil
	}
	page, err := m.engine.Caller().WithPlatform(zai.PlatformTypeOpenRouter).Models(ctx)
	if err != nil {
		return nil, err
	}
	if page == nil || len(page.Data) == 0 {
		return []domain.LlmModelItem{}, nil
	}
	out := make([]domain.LlmModelItem, 0, len(page.Data))
	for _, mod := range page.Data {
		out = append(out, domain.LlmModelItem{
			ID:      mod.ID,
			OwnedBy: mod.OwnedBy,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *Manager) ListSessions(ctx context.Context, userID int64, limit, offset int32) ([]domain.SessionDTO, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := m.db.LlmSessionRepo.ListByUserID(ctx, repo_session.ListByUserIDParams{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	items := make([]domain.SessionDTO, 0, len(rows))
	for i := range rows {
		items = append(items, convert.SessionDTOFromRepo(&rows[i]))
	}
	return items, nil
}

func (m *Manager) GetSession(ctx context.Context, userID, sessionID int64) (*domain.SessionDetailDTO, error) {
	session, err := m.RequireSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	dialogs, err := m.db.LlmDialogRepo.ListBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	items := make([]domain.DialogDTO, 0, len(dialogs))
	for i := range dialogs {
		items = append(items, convert.DialogDTOFromRepo(&dialogs[i]))
	}
	return &domain.SessionDetailDTO{
		Session: convert.SessionDTOFromRepo(session),
		Dialogs: items,
	}, nil
}

func (m *Manager) CreateDialog(ctx context.Context, req domain.CreateDialogRequest) (*domain.CreateDialogResponse, error) {
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	now := time.Now()
	return m.createDialogTx(ctx, req.UserID, req.SessionID, content, now, req.Model)
}

func (m *Manager) createDialogTx(ctx context.Context, userID, sessionID int64, content string, now time.Time, reqModel string) (*domain.CreateDialogResponse, error) {
	result, err := m.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		sessionRepo := m.db.LlmSessionRepo.WithTx(tx)
		dialogRepo := m.db.LlmDialogRepo.WithTx(tx)

		session, err := sessionRepo.LockByID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if session == nil {
			return nil, fmt.Errorf("session not found")
		}
		return m.insertQuestionAnswerPair(ctx, sessionRepo, dialogRepo, session, userID, content, now, reqModel)
	})
	if err != nil {
		return nil, err
	}
	resp, _ := result.(*domain.CreateDialogResponse)
	return resp, nil
}

// CreateFirstDialog 单事务创建会话并写入首轮问答（用于 /chat/new 首次发送）。
func (m *Manager) CreateFirstDialog(ctx context.Context, userID int64, content, reqModel string) (*domain.CreateDialogResponse, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	now := time.Now()
	sessionID := snowflake.Generate().Int64()
	title := "新对话"

	result, err := m.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		sessionRepo := m.db.LlmSessionRepo.WithTx(tx)
		dialogRepo := m.db.LlmDialogRepo.WithTx(tx)

		if _, err := sessionRepo.Create(ctx, repo_session.CreateParams{
			ID:           sessionID,
			UserID:       userID,
			Title:        title,
			Status:       domain.StatusIdle,
			Summary:      "",
			LastDialogID: 0,
			DialogCount:  0,
			TurnCount:    0,
			Stats:        []byte("{}"),
			LastDialogAt: &now,
		}); err != nil {
			return nil, err
		}

		session, err := sessionRepo.LockByID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if session == nil {
			return nil, fmt.Errorf("session not found")
		}
		return m.insertQuestionAnswerPair(ctx, sessionRepo, dialogRepo, session, userID, content, now, reqModel)
	})
	if err != nil {
		return nil, err
	}
	resp, _ := result.(*domain.CreateDialogResponse)
	return resp, nil
}

func (m *Manager) insertQuestionAnswerPair(
	ctx context.Context,
	sessionRepo *repo_session.Queries,
	dialogRepo *repo_dialog.Queries,
	session *repo_session.LlmSession,
	userID int64,
	content string,
	now time.Time,
	reqModel string,
) (*domain.CreateDialogResponse, error) {
	if session.UserID != userID {
		return nil, fmt.Errorf("forbidden")
	}
	sessionID := session.ID

	turnProvider, turnModel := m.ResolveTurnLlm(ctx, session, reqModel)

	questionID := snowflake.Generate().Int64()
	answerID := snowflake.Generate().Int64()
	dialogID := questionID
	questionSeq := session.DialogCount + 1
	answerSeq := session.DialogCount + 2

	question, err := dialogRepo.Create(ctx, repo_dialog.CreateParams{
		ID:          questionID,
		SessionID:   sessionID,
		DialogID:    dialogID,
		Role:        domain.RoleQuestion,
		Seq:         questionSeq,
		Status:      domain.StatusCompleted,
		ContentText: content,
		Parts:       MarshalParts(domain.NormalizeQuestionParts(content)),
		ContextMeta: []byte("{}"),
		Stats:       repo_dialog.EmptyDialogStatsJSON,
		Visible:     true,
	})
	if err != nil {
		return nil, err
	}

	answer, err := dialogRepo.Create(ctx, repo_dialog.CreateParams{
		ID:            answerID,
		SessionID:     sessionID,
		DialogID:      dialogID,
		Role:          domain.RoleAnswer,
		Seq:           answerSeq,
		Status:        domain.StatusPending,
		ContentText:   "",
		Parts:         []byte("[]"),
		ContextMeta:   []byte("{}"),
		Stats:         repo_dialog.EmptyDialogStatsJSON,
		Provider:      turnProvider,
		Model:         turnModel,
		CanRegenerate: true,
		Visible:       true,
	})
	if err != nil {
		return nil, err
	}

	if _, err := sessionRepo.UpdateActivity(ctx, repo_session.UpdateActivityParams{
		LastDialogID:     answerID,
		DialogCountDelta: 2,
		TurnCountDelta:   1,
		LastDialogAt:     &now,
		ID:               sessionID,
	}); err != nil {
		return nil, err
	}
	if err := dialogRepo.UpdateCanRegenerateBySessionID(ctx, repo_dialog.UpdateCanRegenerateBySessionIDParams{
		SessionID: sessionID,
		ID:        answerID,
	}); err != nil {
		return nil, err
	}

	return &domain.CreateDialogResponse{
		Question: convert.DialogDTOFromRepo(question),
		Answer:   convert.DialogDTOFromRepo(answer),
	}, nil
}

func (m *Manager) Regenerate(ctx context.Context, req domain.RegenerateRequest) (*domain.DialogDTO, error) {
	userID := req.UserID
	sessionID := req.SessionID
	targetAnswerID := req.DialogID

	now := time.Now()
	result, err := m.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		sessionRepo := m.db.LlmSessionRepo.WithTx(tx)
		dialogRepo := m.db.LlmDialogRepo.WithTx(tx)

		session, err := sessionRepo.LockByID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if session == nil {
			return nil, fmt.Errorf("session not found")
		}
		if session.UserID != userID {
			return nil, fmt.Errorf("forbidden")
		}

		latest, err := dialogRepo.GetLatestAnswerBySessionID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if latest == nil || latest.ID != targetAnswerID {
			return nil, fmt.Errorf("only latest answer can regenerate")
		}

		prev := latest
		turnProvider, turnModel := m.ResolveTurnLlm(ctx, session, req.Model)

		hidden, err := dialogRepo.HideAnswerByID(ctx, targetAnswerID)
		if err != nil {
			return nil, err
		}
		if hidden == nil {
			return nil, fmt.Errorf("answer not found")
		}

		newAnswerID := snowflake.Generate().Int64()
		newSeq := session.DialogCount + 1
		answer, err := dialogRepo.Create(ctx, repo_dialog.CreateParams{
			ID:            newAnswerID,
			SessionID:     sessionID,
			DialogID:      prev.DialogID,
			Role:          domain.RoleAnswer,
			Seq:           newSeq,
			Status:        domain.StatusPending,
			ContentText:   "",
			Parts:         []byte("[]"),
			ContextMeta:   []byte("{}"),
			Stats:         repo_dialog.EmptyDialogStatsJSON,
			Provider:      turnProvider,
			Model:         turnModel,
			CanRegenerate: true,
			Visible:       true,
		})
		if err != nil {
			return nil, err
		}

		if _, err := sessionRepo.UpdateActivity(ctx, repo_session.UpdateActivityParams{
			LastDialogID:     newAnswerID,
			DialogCountDelta: 1,
			TurnCountDelta:   0,
			LastDialogAt:     &now,
			ID:               sessionID,
		}); err != nil {
			return nil, err
		}
		if err := dialogRepo.UpdateCanRegenerateBySessionID(ctx, repo_dialog.UpdateCanRegenerateBySessionIDParams{
			SessionID: sessionID,
			ID:        newAnswerID,
		}); err != nil {
			return nil, err
		}
		if _, err := sessionRepo.SetRegenerateAnswerID(ctx, repo_session.SetRegenerateAnswerIDParams{
			LastDialogID: newAnswerID,
			LastDialogAt: &now,
			ID:           sessionID,
		}); err != nil {
			return nil, err
		}

		dto := convert.DialogDTOFromRepo(answer)
		return &dto, nil
	})
	if err != nil {
		return nil, err
	}
	resp, _ := result.(*domain.DialogDTO)
	return resp, nil
}

func (m *Manager) RequireSession(ctx context.Context, userID, sessionID int64) (*repo_session.LlmSession, error) {
	row, err := m.db.LlmSessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("session not found")
	}
	if row.UserID != userID {
		return nil, fmt.Errorf("forbidden")
	}
	return row, nil
}

func (m *Manager) BuildChatParams(model string, maxOut int32, messages []domain.ChatMessage) openai.ChatCompletionNewParams {
	params := openai.ChatCompletionNewParams{
		Model:               strings.TrimSpace(model),
		MaxCompletionTokens: openai.Int(int64(maxOut)),
	}
	if params.Model == "" {
		params.Model = domain.DefaultModel
	}
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			params.Messages = append(params.Messages, openai.ChatCompletionMessageParamUnion{
				OfSystem: &openai.ChatCompletionSystemMessageParam{
					Content: openai.ChatCompletionSystemMessageParamContentUnion{
						OfString: openai.String(msg.Content),
					},
				},
			})
		case "assistant":
			params.Messages = append(params.Messages, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &openai.ChatCompletionAssistantMessageParam{
					Content: openai.ChatCompletionAssistantMessageParamContentUnion{
						OfString: openai.String(msg.Content),
					},
				},
			})
		default:
			params.Messages = append(params.Messages, openai.ChatCompletionMessageParamUnion{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{
						OfString: openai.String(msg.Content),
					},
				},
			})
		}
	}
	return params
}

func MarshalParts(parts []domain.DialogPart) []byte {
	if len(parts) == 0 {
		return []byte("[]")
	}
	raw, err := sonic.Marshal(parts)
	if err != nil {
		return []byte("[]")
	}
	return raw
}

func MarshalContextMeta(meta domain.ContextMeta) []byte {
	raw, err := sonic.Marshal(meta)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func (m *Manager) FinalizeAnswer(ctx context.Context, sessionID, dialogID int64, contextMeta domain.ContextMeta, answerText string, answerParts []domain.DialogPart, status, errorCode, errorMessage, provider, model, finishReason string, lastEventSeq, stepCount, toolCallCount, promptTokens, completionTokens, totalTokens int32) (*repo_dialog.LlmDialog, error) {
	completedAt := time.Now()
	stats := repo_dialog.MarshalDialogStats(repo_dialog.DialogStats{
		StepCount:        stepCount,
		ToolCallCount:    toolCallCount,
		FinishReason:     finishReason,
		LastEventSeq:     lastEventSeq,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	})
	updated, err := m.db.LlmDialogRepo.UpdateAnswerResult(ctx, repo_dialog.UpdateAnswerResultParams{
		ID:           dialogID,
		Status:       status,
		ContentText:  answerText,
		Parts:        MarshalParts(answerParts),
		ContextMeta:  MarshalContextMeta(contextMeta),
		Stats:        stats,
		Provider:     provider,
		Model:        model,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		CompletedAt:  &completedAt,
	})
	if err != nil {
		return nil, err
	}

	targetStatus := domain.StatusIdle
	if status == domain.StatusError {
		targetStatus = domain.StatusError
	}
	if _, err := m.db.LlmSessionRepo.UpdateStatus(ctx, repo_session.UpdateStatusParams{
		ID:     sessionID,
		Status: targetStatus,
	}); err != nil {
		return nil, err
	}
	return updated, nil
}
