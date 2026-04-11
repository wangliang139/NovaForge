package document

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/bsm/redislock"
	"github.com/bytedance/sonic"
	"github.com/openai/openai-go/v3"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/wangliang139/llt-trade/server/pkg/converter"
	"github.com/wangliang139/llt-trade/server/pkg/entity/llm"
	"github.com/wangliang139/llt-trade/server/pkg/internal/cachekey"
	"github.com/wangliang139/llt-trade/server/pkg/internal/consts"
	"github.com/wangliang139/llt-trade/server/pkg/internal/push"
	"github.com/wangliang139/llt-trade/server/pkg/internal/rstream"
	"github.com/wangliang139/llt-trade/server/pkg/internal/tictoken"
	"github.com/wangliang139/llt-trade/server/pkg/internal/zai"
	"github.com/wangliang139/llt-trade/server/pkg/repos/document"
	"github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/utils"
	"github.com/wangliang139/mow/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const TracerName = "github.com/wangliang139/llt-trade/server/pkg/entity/documents"

var tracer = otel.Tracer(TracerName)

func (e *Entity) ListenDocumentPendingEvent(ctx context.Context, consumerId string) {
	streamKey := e.cfg.DocumentPendingTopic
	group := e.cfg.DocumentConsumerGroup

	ch := rstream.Subscribe(ctx, e.cache, streamKey, group, consumerId)
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-ch:
			err := e.executor.Submit(func(ctx context.Context) {
				var (
					procErr error
					message types.Document
				)

				if procErr = sonic.Unmarshal(payload, &message); procErr != nil {
					logger.Ctx(ctx).Err(procErr).Str("consumer_id", consumerId).Msg("failed to unmarshal document pending message")
					return
				}

				ctx, span := tracer.Start(ctx, "documents.pending.consume")
				defer func() {
					span.SetAttributes(attribute.Int64("id", message.Id))
					if procErr != nil {
						span.SetStatus(codes.Error, procErr.Error())
					} else {
						span.SetStatus(codes.Ok, "success")
					}
					span.End()
				}()

				logger.Ctx(ctx).Info().Str("consumer_id", consumerId).Msg("receive document pending message")

				switch message.Source {
				case types.DocumentSourceTwitter:
					// TODO: process twitter document active event
				default:
					procErr = e.processDocumentPendingEvent(ctx, &message)
				}
				if procErr != nil {
					logger.Ctx(ctx).Err(procErr).Str("consumer_id", consumerId).Msg("failed to process pending document message")
					return
				}
			})
			if err != nil {
				log.Err(err).Msg("failed to submit pending task")
			}
		}
	}
}

func (e *Entity) processDocumentPendingEvent(ctx context.Context, message *types.Document) error {
	logger.Ctx(ctx).Info().Int64("id", message.Id).Msg("start process pending document")

	if message.PublishedAt.Before(time.Now().Add(-time.Hour * 24)) {
		_, err := e.db.DocumentRepo.UpdateStatus(ctx, document.UpdateStatusParams{
			ID:         message.Id,
			Status:     document.DocumentStatusTimeout,
			PrevStatus: document.DocumentStatusPending,
			ErrMsg:     lo.ToPtr("published at is before 24 hours ago"),
		})
		return err
	}

	// get embedding
	embedding, err := e.getOrCreateEmbedding(ctx, message.Id)
	if err != nil {
		return err
	}
	if len(embedding) == 0 {
		return errors.New("embedding is empty")
	}

	// pre semantic filter
	rows, err := e.db.DocumentRepo.SemanticSearch(ctx, document.SemanticSearchParams{
		Embedding:        embedding,
		Threshold:        e.cfg.SemanticFilterThreshold,
		PublishedAtStart: message.PublishedAt.Add(-e.cfg.DocumentFilterTimeWindow),
		PublishedAtEnd:   message.PublishedAt.Add(e.cfg.DocumentFilterTimeWindow),
		Status:           document.DocumentStatusActive,
		TopK:             1,
		Excludes:         []int64{message.Id},
	})
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		dedupedBy := rows[0].ID
		_, err := e.db.DocumentRepo.UpdateStatus(ctx, document.UpdateStatusParams{
			ID:         message.Id,
			Status:     document.DocumentStatusDeduped,
			PrevStatus: document.DocumentStatusPending,
			ErrMsg:     lo.ToPtr("deduped by semantic"),
			DedupedBy:  lo.ToPtr(dedupedBy),
		})
		return err
	}

	// summarize document
	aiResult, err := e.summarizeDocument(ctx, message)
	if err != nil {
		errMsg := err.Error()
		if len(errMsg) > 1000 {
			errMsg = utils.Strings.TruncateUTF8(errMsg, 1000) + "..."
		}
		_, err2 := e.db.DocumentRepo.UpdateStatus(ctx, document.UpdateStatusParams{
			ID:         message.Id,
			Status:     document.DocumentStatusPendingFailed,
			PrevStatus: document.DocumentStatusPending,
			ErrMsg:     &errMsg,
		})
		if err2 != nil {
			logger.Ctx(ctx).Err(err2).Msg("failed to update document status")
		}
		return err
	}

	// global lock to prevent duplicate semantic dedup
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lock, err := e.dlock.Obtain(cctx, cachekey.DocSemanticDedupLockKey, 5*time.Second, &redislock.Options{
		RetryStrategy: redislock.LinearBackoff(100 * time.Millisecond),
	})
	if err != nil {
		return fmt.Errorf("failed to obtain semantic dedup lock: %w", err)
	}
	defer lock.Release(ctx)

	opParams := document.SaveAiSummaryParams{
		ID:               message.Id,
		AiTitle:          strings.TrimSpace(aiResult.Title),
		AiSummary:        strings.TrimSpace(aiResult.Summary),
		AiTags:           aiResult.Tags,
		AiCoins:          aiResult.Coins,
		AiInfluence:      strings.TrimSpace(aiResult.Influence),
		AiInfluenceScore: int32(aiResult.InfluenceScore),
		AiSentiment:      int32(aiResult.Sentiment),
		Status:           document.DocumentStatusActive,
	}

	// post semantic dedup
	rows, err = e.db.DocumentRepo.SemanticSearch(ctx, document.SemanticSearchParams{
		Embedding:        embedding,
		Threshold:        e.cfg.SemanticFilterThreshold,
		PublishedAtStart: message.PublishedAt.Add(-e.cfg.DocumentFilterTimeWindow),
		PublishedAtEnd:   message.PublishedAt.Add(e.cfg.DocumentFilterTimeWindow),
		Status:           document.DocumentStatusActive,
		TopK:             1,
		Excludes:         []int64{message.Id},
	})
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		dedupedBy := rows[0].ID
		opParams.DedupedBy = lo.ToPtr(dedupedBy)
		opParams.Status = document.DocumentStatusDeduped
		opParams.ErrMsg = lo.ToPtr("deduped by semantic")
	}

	// save document to database
	row, err := e.db.DocumentRepo.SaveAiSummary(ctx, opParams)
	if err != nil {
		return err
	}
	if row == nil {
		return errors.New("doc not exists")
	}

	if opParams.Status != document.DocumentStatusActive {
		return nil
	}

	// 通过 channel + Redis Stream 通知后续处理（如 Telegram）
	go func() {
		// 写入 channel 供 SubscribeStream(stream_type=SOCIAL) 接收并转发
		message := converter.DocumentSaveAiSummaryRowRepo2Types(row)
		select {
		case e.docActiveCh <- message:
		default:
		}

		ctx := context.WithoutCancel(ctx)
		if err := e.writeDocStream(ctx, e.cfg.DocumentActiveTopic, message); err != nil {
			logger.Ctx(ctx).Err(err).Msg("failed send active document event to redis stream")
		}
	}()

	return nil
}

func (e *Entity) ListenDocumentActiveEvent(ctx context.Context, consumerId string) {
	streamKey := e.cfg.DocumentActiveTopic
	group := e.cfg.DocumentConsumerGroup

	ch := rstream.Subscribe(ctx, e.cache, streamKey, group, consumerId)

	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-ch:
			err := e.executor.Submit(func(ctx context.Context) {
				var (
					procErr error
					message types.Document
				)

				if procErr = sonic.Unmarshal(payload, &message); procErr != nil {
					logger.Ctx(ctx).Err(procErr).Str("consumer_id", consumerId).Msg("failed to unmarshal document active message")
					return
				}

				ctx, span := tracer.Start(ctx, "documents.active.consume")
				defer func() {
					span.SetAttributes(attribute.Int64("id", message.Id))
					if procErr != nil {
						span.SetStatus(codes.Error, procErr.Error())
					} else {
						span.SetStatus(codes.Ok, "success")
					}
					span.End()
				}()

				logger.Ctx(ctx).Info().Str("consumer_id", consumerId).Msg("receive document active message")
				procErr = e.processDocumentActiveEvent(ctx, &message)
				if procErr != nil {
					logger.Ctx(ctx).Err(procErr).Str("consumer_id", consumerId).Msg("failed to process active document message")
					return
				}
			})
			if err != nil {
				log.Err(err).Msg("failed to submit active task")
			}
		}
	}
}

func (e *Entity) processDocumentActiveEvent(ctx context.Context, message *types.Document) error {
	logger.Ctx(ctx).Info().Int64("id", message.Id).Msg("start process active document")
	if message.PublishedAt.Before(time.Now().Add(-time.Hour * 1)) {
		return nil
	}
	return e.sendDocumentToTelegram(ctx, *message)
}

func (e *Entity) summarizeDocument(ctx context.Context, message *types.Document) (*AiSummaryResult, error) {
	// 超时的消息用降级模型处理
	sceneKey := "ai_document_summary"
	if message.CreatedAt.Before(time.Now().Add(-time.Minute * 30)) {
		sceneKey = "ai_document_summary:downgrade"
	}

	// call llm to process the document
	content, err := tictoken.Truncate(message.Content, e.cfg.DocumentSummaryContentMaxTokens)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to truncate document content")
		content = message.Content
	}

	response, err := e.llm.Completion(ctx, &llm.CompletionRequest{
		SceneKey: sceneKey,
		Variables: map[string]any{
			"title":   message.Title,
			"content": content,
		},
	})

	// logger.Ctx(ctx).Debug().Interface("response", response).Msg("llm response")

	if err == nil {
		var result AiSummaryResult
		err = sonic.UnmarshalString(utils.LLM.Json(response.Result), &result)
		if err == nil {
			return &result, nil
		}
	}

	go func() {
		if !e.alarmLimiter.Allow() {
			return
		}

		ctx := context.WithoutCancel(ctx)
		errMsg := html.EscapeString(err.Error())
		if len(errMsg) > 200 {
			errMsg = utils.Strings.TruncateUTF8(errMsg, 200) + "..."
		}
		err = push.NotifyByTemplate(ctx, push.NotifyByTemplateRequest{
			SceneKey: "alarm.document.summary",
			Vars: map[string]any{
				"title":   "🚨 Service Alert",
				"time":    time.Now().In(location).Format("2006-01-02 15:04:05"),
				"message": errMsg,
			},
		})
		if err != nil {
			logger.Ctx(ctx).Err(err).Msg("failed to send alarm push")
		}
	}()

	return nil, err
}

func (e *Entity) getOrCreateEmbedding(ctx context.Context, docId int64) (embedding []float32, err error) {
	// Try to get embedding from cache
	str, err := e.cache.Get(ctx, cachekey.DocEmbeddingKey(docId)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		logger.Ctx(ctx).Err(err).Msg("failed to get embedding from cache")
	}
	if len(str) > 0 {
		err = sonic.UnmarshalString(str, &embedding)
		if err != nil {
			logger.Ctx(ctx).Err(err).Msg("failed to unmarshal embedding from cache")
		} else {
			return embedding, nil
		}
	}

	defer func() {
		if err == nil && len(embedding) > 0 {
			ctx := context.WithoutCancel(ctx)
			str, err2 := sonic.MarshalString(embedding)
			if err2 != nil {
				logger.Ctx(ctx).Err(err2).Msg("failed to marshal embedding to bytes")
			} else {
				err2 = e.cache.Set(ctx, cachekey.DocEmbeddingKey(docId), str, time.Minute*30).Err()
				if err2 != nil {
					logger.Ctx(ctx).Err(err2).Msg("failed to set embedding to cache")
				}
			}
		}
	}()

	doc, err := e.db.DocumentRepo.GetByIdWithEmbedding(ctx, docId)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, errors.New("document not found")
	}
	if doc.Embedding != nil {
		return doc.Embedding, nil
	}

	// Try to obtain lock.
	{
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		lock, err := e.dlock.Obtain(cctx, cachekey.DocEmbeddingLockKey(docId), 5*time.Second, &redislock.Options{
			RetryStrategy: redislock.LinearBackoff(100 * time.Millisecond),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to obtain embedding lock: %w", err)
		}
		defer lock.Release(ctx)
	}

	// Double check
	doc, err = e.db.DocumentRepo.GetByIdWithEmbedding(ctx, docId)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, errors.New("document not found")
	}
	if doc.Embedding != nil {
		return doc.Embedding, nil
	}

	response, err := e.zaiEngine.Caller().WithPlatform(zai.PlatformTypeOpenRouter).CreateEmbeddings(ctx, openai.EmbeddingNewParams{
		Model:          e.cfg.EmbeddingModel,
		Input:          openai.EmbeddingNewParamsInputUnion{OfString: openai.String(fmt.Sprintf("%s\n\n%s", doc.Title, doc.Content))},
		Dimensions:     openai.Int(consts.EmbedDimensions),
		EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
	})
	if err != nil {
		return nil, err
	}
	embedding = make([]float32, len(response.Data[0].Embedding))
	for i, v := range response.Data[0].Embedding {
		embedding[i] = float32(v)
	}

	// Update embedding to database
	count, err := e.db.DocumentRepo.UpdateEmbedding(ctx, document.UpdateEmbeddingParams{
		ID:        docId,
		Embedding: embedding,
	})
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, fmt.Errorf("failed to update embedding: %d", count)
	}
	return embedding, nil
}
