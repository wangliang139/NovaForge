package coordinator

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/rs/zerolog/log"
	"github.com/wangliang139/NovaForge/server/pkg/chat/domain"
	chatcfg "github.com/wangliang139/NovaForge/server/pkg/chat/internal/config"
	chatctx "github.com/wangliang139/NovaForge/server/pkg/chat/internal/context"
	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/manager"
	chatreact "github.com/wangliang139/NovaForge/server/pkg/chat/internal/react"
	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/runtime"
	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/sse"
	"github.com/wangliang139/NovaForge/server/pkg/internal/zai"
	repo_dialog "github.com/wangliang139/NovaForge/server/pkg/repos/llm_dialog"
	repo_session "github.com/wangliang139/NovaForge/server/pkg/repos/llm_session"
)

type Coordinator struct {
	mgr *manager.Manager
	cfg chatcfg.Config
}

func New(mgr *manager.Manager, cfg chatcfg.Config) *Coordinator {
	return &Coordinator{mgr: mgr, cfg: cfg}
}

// ChatStream 统一对外入口：解析「要推流的那条答案」、可选创建/再生数据，再输出 SSE。
// 首包为 ready（sessionId / dialogId），之后与 answerStream 一致。
func (c *Coordinator) ChatStream(ctx context.Context, req domain.ChatRequest) (<-chan domain.DeltaEvent, error) {
	sessionID, answerID, err := c.resolveStreamTarget(ctx, req)
	if err != nil {
		return nil, err
	}

	// 首轮发送时请求体可能不带 sessionId/dialogId，resolveStreamTarget 会创建并得到真实 ID，此处必须用解析结果而非 req。
	session, err := c.mgr.RequireSession(ctx, req.UserID, sessionID)
	if err != nil {
		return nil, err
	}
	dialog, err := c.mgr.DB().LlmDialogRepo.GetByID(ctx, answerID)
	if err != nil {
		return nil, err
	}
	if dialog == nil || dialog.SessionID != sessionID || dialog.Role != domain.RoleAnswer {
		return nil, fmt.Errorf("answer dialog not found")
	}
	if !dialog.Visible {
		return nil, fmt.Errorf("answer dialog not found")
	}

	// React 多步会连续产生较多事件；缓冲过小易阻塞 pump 协程，与 HTTP 刷盘脱节。
	out := make(chan domain.DeltaEvent, 64)
	writer := NewEventWriter(out, domain.Int64String(sessionID), domain.Int64String(answerID))

	if dialog.Status == domain.StatusCompleted && strings.TrimSpace(dialog.ContentText) != "" {
		go func() {
			defer close(out)
			_ = replayDialog(dialog, writer)
		}()
		return out, nil
	}
	if dialog.Status == domain.StatusStreaming {
		return nil, fmt.Errorf("answer is already streaming")
	}

	dialogs, err := c.mgr.DB().LlmDialogRepo.ListBySessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	contextResult := chatctx.Build(session, dialogs, answerID, c.cfg)

	startedAt := time.Now()
	if _, err := c.mgr.DB().LlmDialogRepo.StartAnswerStream(ctx, repo_dialog.StartAnswerStreamParams{
		StartedAt: &startedAt,
		ID:        answerID,
	}); err != nil {
		return nil, err
	}
	if _, err := c.mgr.DB().LlmSessionRepo.UpdateStatus(ctx, repo_session.UpdateStatusParams{
		ID:     sessionID,
		Status: domain.StatusStreaming,
	}); err != nil {
		return nil, err
	}

	provider := strings.TrimSpace(dialog.Provider)
	if provider == "" {
		provider = domain.DefaultProvider
	}
	model := strings.TrimSpace(dialog.Model)
	if model == "" {
		model = domain.DefaultModel
	}

	// 发送 ready 事件
	out <- readyEvent(sessionID, answerID)

	go func() {
		defer close(out)
		c.pumpCompletionStream(ctx, req.UserID, sessionID, answerID, writer, contextResult, provider, model)
	}()

	return out, nil
}

// resolveStreamTarget 根据 ChatRequest 得到最终要推流的 sessionId 与答案 dialogId（可能先写库）。
func (c *Coordinator) resolveStreamTarget(ctx context.Context, req domain.ChatRequest) (sessionID, answerID int64, err error) {
	userID := req.UserID
	content := strings.TrimSpace(req.Content)

	switch {
	case req.Regenerate:
		if req.SessionID <= 0 || req.DialogID <= 0 {
			return 0, 0, fmt.Errorf("sessionId and dialogId required for regenerate")
		}
		dto, regenErr := c.mgr.Regenerate(ctx, domain.RegenerateRequest{
			UserID:    userID,
			SessionID: req.SessionID,
			DialogID:  req.DialogID,
			Model:     req.Model,
		})
		if regenErr != nil {
			return 0, 0, regenErr
		}
		aid, parseErr := strconv.ParseInt(dto.ID, 10, 64)
		if parseErr != nil {
			return 0, 0, parseErr
		}
		return req.SessionID, aid, nil
	case content == "":
		if req.SessionID <= 0 || req.DialogID <= 0 {
			return 0, 0, fmt.Errorf("sessionId and dialogId required for replay")
		}
		return req.SessionID, req.DialogID, nil
	default:
		if req.DialogID != 0 {
			return 0, 0, fmt.Errorf("dialogId must be empty when sending a new message")
		}
		if req.SessionID <= 0 {
			return c.createFirstTurnTarget(ctx, userID, content, req.Model)
		}
		return c.appendTurnTarget(ctx, userID, req.SessionID, content, req.Model)
	}
}

func (c *Coordinator) createFirstTurnTarget(ctx context.Context, userID int64, content, model string) (sessionID, answerID int64, err error) {
	resp, err := c.mgr.CreateFirstDialog(ctx, userID, content, model)
	if err != nil {
		return 0, 0, err
	}
	return parseCreateResponseTarget(resp)
}

func (c *Coordinator) appendTurnTarget(ctx context.Context, userID, sessionID int64, content, model string) (int64, int64, error) {
	resp, err := c.mgr.CreateDialog(ctx, domain.CreateDialogRequest{
		UserID:    userID,
		SessionID: sessionID,
		Content:   content,
		Model:     model,
	})
	if err != nil {
		return 0, 0, err
	}
	aid, err := strconv.ParseInt(resp.Answer.ID, 10, 64)
	if err != nil {
		return 0, 0, err
	}
	return sessionID, aid, nil
}

func parseCreateResponseTarget(resp *domain.CreateDialogResponse) (sessionID, answerID int64, err error) {
	sid, err := strconv.ParseInt(resp.Answer.SessionID, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid session id")
	}
	aid, err := strconv.ParseInt(resp.Answer.ID, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid dialog id")
	}
	return sid, aid, nil
}

// pumpCompletionStream 在 DB 已标记 streaming 后执行：started → LLM → finalize → done。
func (c *Coordinator) pumpCompletionStream(
	ctx context.Context,
	userID, sessionID, dialogID int64,
	writer *eventWriter,
	contextResult domain.ContextBuildResult,
	provider, model string,
) {
	if !c.isSessionAlive(ctx, sessionID) {
		return
	}
	_ = writer.Write(domain.EventStarted, "control", map[string]any{
		"status":      domain.StatusStreaming,
		"contextMeta": contextResult.ContextMeta,
	}, map[string]any{
		"provider": provider,
		"model":    model,
	})

	if c.cfg.ReactEnabled {
		log.Info().
			Str("component", "chat.coordinator").
			Int64("session_id", sessionID).
			Int64("dialog_id", dialogID).
			Int64("user_id", userID).
			Str("provider", provider).
			Str("model", model).
			Msg("react_stream_start")
		reactOut := chatreact.Run(ctx, chatreact.Dependencies{
			BuildChatParams: c.mgr.BuildChatParams,
			Complete: func(ctx context.Context, provider string, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
				return c.mgr.Engine().Caller().WithPlatform(InferProviderFromString(provider)).CreateChatCompletion(ctx, params)
			},
			CompleteStream: func(ctx context.Context, provider string, params openai.ChatCompletionNewParams) (*ssestream.Stream[openai.ChatCompletionChunk], error) {
				return c.mgr.Engine().Caller().WithPlatform(InferProviderFromString(provider)).CreateChatCompletionStream(ctx, params)
			},
			SanitizeText: SanitizeStreamText,
			Runtime: runtime.NewRuntime(domain.Env{
				DB:        c.mgr.DB(),
				ZaiEngine: c.mgr.Engine(),
				Model:     model,
			}),
		}, writer.Write, chatreact.Input{
			ContextResult: contextResult,
			Provider:      provider,
			Model:         model,
			Log: chatreact.StreamLogFields{
				SessionID: sessionID,
				DialogID:  dialogID,
				UserID:    userID,
			},
			Policy: chatreact.Policy{
				MaxSteps:                 c.cfg.MaxSteps,
				MaxToolCalls:             c.cfg.MaxToolCalls,
				ToolTimeout:              c.cfg.ToolTimeout(),
				MaxConsecutiveToolErrors: c.cfg.MaxConsecutiveToolErrors,
			},
		})
		if reactOut.CompletedWithError {
			_, _ = c.mgr.FinalizeAnswer(
				ctx, sessionID, dialogID, contextResult.ContextMeta,
				strings.TrimSpace(reactOut.AnswerText), reactOut.Parts, domain.StatusError,
				reactOut.ErrorCode, reactOut.ErrorMessage,
				reactOut.ActualProvider, reactOut.ActualModel, "error",
				writer.LastSeq(), reactOut.StepCount, reactOut.ToolCallCount,
				reactOut.PromptTokens, reactOut.CompletionTokens, reactOut.TotalTokens,
			)
			_ = writer.Write(domain.EventError, "control", map[string]any{
				"code":      reactOut.ErrorCode,
				"message":   reactOut.ErrorMessage,
				"retryable": true,
				"source":    "react",
			}, nil)
			return
		}
		updated, err := c.mgr.FinalizeAnswer(
			ctx, sessionID, dialogID, contextResult.ContextMeta,
			reactOut.AnswerText, reactOut.Parts, domain.StatusCompleted,
			"", "", reactOut.ActualProvider, reactOut.ActualModel, reactOut.FinishReason,
			writer.LastSeq(), reactOut.StepCount, reactOut.ToolCallCount,
			reactOut.PromptTokens, reactOut.CompletionTokens, reactOut.TotalTokens,
		)
		if err != nil {
			return
		}
		st := repo_dialog.ParseDialogStats(updated.Stats)
		_ = writer.Write(domain.EventDone, "control", map[string]any{
			"status":       domain.StatusCompleted,
			"finishReason": st.FinishReason,
			"usage": map[string]any{
				"promptTokens":     st.PromptTokens,
				"completionTokens": st.CompletionTokens,
				"totalTokens":      st.TotalTokens,
			},
			"steps":     st.StepCount,
			"toolCalls": st.ToolCallCount,
		}, nil)
		return
	}

	c.pumpDirectCompletionStream(ctx, sessionID, dialogID, writer, contextResult, provider, model)
}

func (c *Coordinator) pumpDirectCompletionStream(
	ctx context.Context,
	sessionID, dialogID int64,
	writer *eventWriter,
	contextResult domain.ContextBuildResult,
	provider, model string,
) {
	params := c.mgr.BuildChatParams(model, domain.MaxOutputTokens, contextResult.Messages)
	llmStream, err := c.mgr.Engine().Caller().WithPlatform(InferProviderFromString(provider)).CreateChatCompletionStream(ctx, params)
	if err != nil {
		_, _ = c.mgr.FinalizeAnswer(ctx, sessionID, dialogID, contextResult.ContextMeta, "", nil, domain.StatusError, "provider_error", err.Error(), provider, model, "", writer.LastSeq(), 1, 0, 0, 0, 0)
		_ = writer.Write(domain.EventError, "control", map[string]any{
			"code":      "provider_error",
			"message":   err.Error(),
			"retryable": true,
			"source":    "provider",
		}, nil)
		return
	}
	defer llmStream.Close()

	var contentBuilder strings.Builder
	finishReason := "stop"
	actualModel := model
	actualProvider := provider
	var promptTokens, completionTokens, totalTokens int32

	for llmStream.Next() {
		chunk := llmStream.Current()
		if chunk.Model != "" {
			actualModel = chunk.Model
		}
		if chunk.Usage.TotalTokens > 0 {
			promptTokens = int32(chunk.Usage.PromptTokens)
			completionTokens = int32(chunk.Usage.CompletionTokens)
			totalTokens = int32(chunk.Usage.TotalTokens)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				text := SanitizeStreamText(choice.Delta.Content)
				if text == "" {
					continue
				}
				contentBuilder.WriteString(text)
				_ = writer.Write(domain.EventText, "final", map[string]any{
					"text":   text,
					"append": true,
				}, nil)
			}
			if choice.Delta.Refusal != "" {
				text := SanitizeStreamText(choice.Delta.Refusal)
				if text == "" {
					continue
				}
				contentBuilder.WriteString(text)
				_ = writer.Write(domain.EventText, "final", map[string]any{
					"text":   text,
					"append": true,
				}, nil)
			}
			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}
		}
	}

	if err := llmStream.Err(); err != nil {
		_, _ = c.mgr.FinalizeAnswer(ctx, sessionID, dialogID, contextResult.ContextMeta, contentBuilder.String(), domain.ParseAnswerParts(contentBuilder.String()), domain.StatusError, "stream_error", err.Error(), actualProvider, actualModel, "error", writer.LastSeq(), 1, 0, promptTokens, completionTokens, totalTokens)
		_ = writer.Write(domain.EventError, "control", map[string]any{
			"code":      "stream_error",
			"message":   err.Error(),
			"retryable": true,
			"source":    "provider",
		}, nil)
		return
	}

	answerText := strings.TrimSpace(contentBuilder.String())
	answerParts := domain.ParseAnswerParts(answerText)
	updated, err := c.mgr.FinalizeAnswer(ctx, sessionID, dialogID, contextResult.ContextMeta, answerText, answerParts, domain.StatusCompleted, "", "", actualProvider, actualModel, finishReason, writer.LastSeq(), 1, 0, promptTokens, completionTokens, totalTokens)
	if err != nil {
		return
	}

	st := repo_dialog.ParseDialogStats(updated.Stats)
	_ = writer.Write(domain.EventDone, "control", map[string]any{
		"status":       domain.StatusCompleted,
		"finishReason": st.FinishReason,
		"usage": map[string]any{
			"promptTokens":     st.PromptTokens,
			"completionTokens": st.CompletionTokens,
			"totalTokens":      st.TotalTokens,
		},
		"steps":     st.StepCount,
		"toolCalls": st.ToolCallCount,
	}, nil)
}

func (c *Coordinator) isSessionAlive(ctx context.Context, sessionID int64) bool {
	session, err := c.mgr.DB().LlmSessionRepo.GetByID(ctx, sessionID)
	return err == nil && session != nil
}

func replayDialog(dialog *repo_dialog.LlmDialog, writer *eventWriter) error {
	contextMeta := domain.DecodeContextMeta(dialog.ContextMeta)
	if contextMeta == nil {
		contextMeta = &domain.ContextMeta{
			Strategy: "recent_plus_summary",
		}
	}
	if err := writer.Write(domain.EventStarted, "control", map[string]any{
		"status":      dialog.Status,
		"contextMeta": contextMeta,
	}, map[string]any{
		"replayed": true,
		"model":    dialog.Model,
		"provider": dialog.Provider,
	}); err != nil {
		return err
	}

	for _, part := range domain.DecodeParts(dialog.Parts) {
		if err := writer.Write(part.Type, sse.PhaseForPart(part.Type), sse.DeltaFromPart(part), nil); err != nil {
			return err
		}
	}
	st := repo_dialog.ParseDialogStats(dialog.Stats)
	return writer.Write(domain.EventDone, "control", map[string]any{
		"status":       dialog.Status,
		"finishReason": st.FinishReason,
		"usage": map[string]any{
			"promptTokens":     st.PromptTokens,
			"completionTokens": st.CompletionTokens,
			"totalTokens":      st.TotalTokens,
		},
		"steps":     st.StepCount,
		"toolCalls": st.ToolCallCount,
	}, nil)
}

func readyEvent(sessionID, dialogID int64) domain.DeltaEvent {
	sSid := domain.Int64String(sessionID)
	sDid := domain.Int64String(dialogID)
	return domain.DeltaEvent{
		V:         1,
		ID:        fmt.Sprintf("%s:%s:ready", sSid, sDid),
		SessionID: sSid,
		DialogID:  sDid,
		Seq:       0,
		Type:      domain.EventReady,
		Phase:     "control",
		Ts:        time.Now().Unix(),
		Delta: map[string]any{
			"sessionId": sSid,
			"dialogId":  sDid,
		},
	}
}

func InferProviderFromString(provider string) zai.PlatformType {
	p := strings.TrimSpace(strings.ToLower(provider))
	switch p {
	case "", domain.DefaultProvider, "open-router":
		return zai.PlatformTypeOpenRouter
	default:
		return zai.PlatformTypeOpenRouter
	}
}
