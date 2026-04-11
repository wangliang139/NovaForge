package document

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bsm/redislock"
	"github.com/bytedance/sonic"
	"github.com/kelseyhightower/envconfig"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/wangliang139/llt-trade/server/pkg/converter"
	"github.com/wangliang139/llt-trade/server/pkg/entity/llm"
	"github.com/wangliang139/llt-trade/server/pkg/internal/gateio"
	"github.com/wangliang139/llt-trade/server/pkg/internal/language"
	"github.com/wangliang139/llt-trade/server/pkg/internal/limiter"
	"github.com/wangliang139/llt-trade/server/pkg/internal/zai"
	"github.com/wangliang139/llt-trade/server/pkg/repos"
	"github.com/wangliang139/llt-trade/server/pkg/repos/calendar"
	"github.com/wangliang139/llt-trade/server/pkg/repos/document"
	"github.com/wangliang139/llt-trade/server/pkg/repos/tg_channel"
	"github.com/wangliang139/llt-trade/server/pkg/settings"
	"github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/mow/executors"
	"github.com/wangliang139/mow/logger"
	"github.com/wangliang139/mow/snowflake"
	"golang.org/x/time/rate"
)

type Config struct {
	DocumentConsumerGroup string `split_words:"true" default:"document-consumer-group"`
	DocumentPendingTopic  string `split_words:"true" default:"document-pending-topic"`
	DocumentDraftedTopic  string `split_words:"true" default:"document-drafted-topic"`
	DocumentActiveTopic   string `split_words:"true" default:"document-active-topic"`

	EmbeddingModel string `split_words:"true" envconfig:"EMBEDDING_MODEL" default:"qwen/qwen3-embedding-8b"`

	DocumentCollection              string        `split_words:"true" default:"documents"`
	SemanticFilterThreshold         float32       `split_words:"true" default:"0.8"`
	DocumentFilterTimeWindow        time.Duration `split_words:"true" default:"30m"`
	DocumentSummaryContentMaxTokens int           `split_words:"true" default:"2048"`

	IsDebug bool `split_words:"true"`
}

type Entity struct {
	cfg   Config
	db    *repos.Entity
	cache redis.UniversalClient

	dlock    *redislock.Client
	executor *executors.Executor

	gateioMu       sync.Mutex
	gateioProxyKey string
	gateioClient   *gateio.Client
	zaiEngine      *zai.Engine
	llm            *llm.Entity

	alarmLimiter *limiter.MultiLimiter

	// docActiveCh 供 SubscribeStream(stream_type=SOCIAL) 接收并转发；ListenDocumentActiveEvent 写入
	docActiveCh chan *types.Document

	ctx        context.Context
	cancelFunc context.CancelFunc
}

func New(db *repos.Entity, cache redis.UniversalClient, zaiEngine *zai.Engine, executor *executors.Executor, llm *llm.Entity) *Entity {
	cfg := Config{}
	envconfig.MustProcess("DOC_ENTITY", &cfg)
	log.Info().Msgf("doc entity cfg: %+v", cfg)

	dlock := redislock.New(cache)

	ctx, cancel := context.WithCancel(context.Background())
	e := &Entity{
		cfg:        cfg,
		db:         db,
		cache:      cache,
		dlock:      dlock,
		executor:   executor,
		ctx:        ctx,
		cancelFunc: cancel,
		zaiEngine:  zaiEngine,
		llm:        llm,
		alarmLimiter: limiter.NewMultiLimiter(
			rate.NewLimiter(rate.Every(time.Minute), 1),
			rate.NewLimiter(rate.Every(30*time.Minute/10), 10),
		),
		docActiveCh: make(chan *types.Document),
	}
	return e
}

func (e *Entity) Start() error {
	uuid := snowflake.Generate().String()
	go e.ListenDocumentPendingEvent(e.ctx, uuid)
	go e.ListenDocumentActiveEvent(e.ctx, uuid)
	return nil
}

func (e *Entity) Stop() error {
	e.cancelFunc()
	return nil
}

func (e *Entity) gateioClientFor(ctx context.Context) (*gateio.Client, error) {
	proxy, err := settings.GetHttpProxyURL(ctx)
	if err != nil {
		return nil, err
	}
	e.gateioMu.Lock()
	defer e.gateioMu.Unlock()
	if e.gateioClient != nil && e.gateioProxyKey == proxy {
		return e.gateioClient, nil
	}
	e.gateioClient = gateio.NewClient(proxy)
	e.gateioProxyKey = proxy
	return e.gateioClient, nil
}

func (e *Entity) DocActiveCh() <-chan *types.Document {
	return e.docActiveCh
}

func (e *Entity) QueryDocuments(ctx context.Context, payload *types.QueryDocumentsRequest) ([]*types.Document, error) {
	var (
		publishedAtStart *time.Time
		publishedAtEnd   *time.Time
	)
	if payload.PublishedAtStart != nil {
		t := payload.PublishedAtStart.AsTime()
		publishedAtStart = &t
	}
	if payload.PublishedAtEnd != nil {
		t := payload.PublishedAtEnd.AsTime()
		publishedAtEnd = &t
	}

	catalog := document.NullDocumentCatalog{}
	if payload.Catalog != nil {
		catalog.DocumentCatalog = *payload.Catalog
		catalog.Valid = true
	}

	sts := document.NullDocumentStatus{}
	if payload.Status != nil {
		sts.DocumentStatus = *payload.Status
		sts.Valid = true
	}

	rows, err := e.db.DocumentRepo.QueryDocuments(ctx, document.QueryDocumentsParams{
		ID:               payload.Id,
		Keyword:          payload.Keyword,
		Source:           payload.Source,
		Provider:         payload.Provider,
		Catalog:          catalog,
		PublishedAtStart: publishedAtStart,
		PublishedAtEnd:   publishedAtEnd,
		Status:           sts,
		Tag:              payload.Tag,
		Coin:             payload.Coin,
		InfluenceScore:   payload.InfluenceScore,
		Sentiment:        payload.Sentiment,
		Offset:           int32(payload.Offset),
		Limit:            int32(payload.Limit),
	})
	if err != nil {
		return nil, err
	}

	result := make([]*types.Document, len(rows))
	for i, row := range rows {
		result[i] = converter.DocumentQueryDocumentsRowRepo2Types(&row)
	}

	return result, nil
}

func (e *Entity) QueryDocumentsCount(ctx context.Context, payload *types.QueryDocumentsRequest) (int64, error) {
	var (
		publishedAtStart *time.Time
		publishedAtEnd   *time.Time
	)
	if payload.PublishedAtStart != nil {
		t := payload.PublishedAtStart.AsTime()
		publishedAtStart = &t
	}
	if payload.PublishedAtEnd != nil {
		t := payload.PublishedAtEnd.AsTime()
		publishedAtEnd = &t
	}

	catalog := document.NullDocumentCatalog{}
	if payload.Catalog != nil {
		catalog.DocumentCatalog = *payload.Catalog
		catalog.Valid = true
	}

	sts := document.NullDocumentStatus{}
	if payload.Status != nil {
		sts.DocumentStatus = *payload.Status
		sts.Valid = true
	}

	count, err := e.db.DocumentRepo.CountDocuments(ctx, document.CountDocumentsParams{
		ID:               payload.Id,
		Keyword:          payload.Keyword,
		Status:           sts,
		Source:           payload.Source,
		Provider:         payload.Provider,
		Catalog:          catalog,
		PublishedAtStart: publishedAtStart,
		PublishedAtEnd:   publishedAtEnd,
		Tag:              payload.Tag,
		Coin:             payload.Coin,
		InfluenceScore:   payload.InfluenceScore,
		Sentiment:        payload.Sentiment,
	})
	if err != nil {
		return 0, err
	}
	return *count, nil
}

func (e *Entity) QueryCalendar(ctx context.Context, payload *types.QueryCalendarRequest) ([]*types.Calendar, error) {
	source := calendar.NullCalendarSource{}
	if payload.Source != nil {
		source.CalendarSource = *payload.Source
		source.Valid = true
	}
	_type := calendar.NullCalendarType{}
	if payload.Type != nil {
		_type.CalendarType = *payload.Type
		_type.Valid = true
	}

	rows, err := e.db.CalendarRepo.QueryByDateId(ctx, calendar.QueryByDateIdParams{
		DateID:        payload.DateID,
		Source:        source,
		Type:          _type,
		Category:      payload.Category,
		Country:       payload.Country,
		MinImportance: payload.MinImportance,
		Offset:        0,
		Limit:         1000,
	})
	if err != nil {
		return nil, err
	}

	result := make([]*types.Calendar, len(rows))
	for i, row := range rows {
		result[i] = converter.CalendarRepo2Types(&row)
	}
	return result, nil
}

func (e *Entity) GetDocument(ctx context.Context, id int64) (*types.Document, error) {
	row, err := e.db.DocumentRepo.GetById(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	return converter.DocumentGetByIdRowRepo2Types(row), nil
}

func (e *Entity) ArchiveDocument(ctx context.Context, id int64) (*types.Document, error) {
	doc, err := e.db.DocumentRepo.GetById(ctx, id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("document not found")
	}
	if doc.Status != document.DocumentStatusPending && doc.Status != document.DocumentStatusActive {
		return nil, fmt.Errorf("document status is not pending or active")
	}

	row, err := e.db.DocumentRepo.ArchiveDocument(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("document status is not pending or active")
	}
	return converter.DocumentArchiveDocumentRowRepo2Types(row), nil
}

func (e *Entity) SaveDocument(ctx context.Context, doc *types.Document) (*types.Document, error) {
	// Trim title in content
	doc.Title = strings.TrimSpace(doc.Title)
	doc.Content = strings.TrimSpace(doc.Content)
	doc.Content = strings.TrimPrefix(doc.Content, doc.Title)
	doc.Content = strings.TrimPrefix(doc.Content, fmt.Sprintf("【%s】", doc.Title))
	doc.Content = strings.TrimSpace(doc.Content)

	// check if the document is already saved
	exist, err := e.db.DocumentRepo.GetByMd5(ctx, doc.Md5)
	if err != nil {
		return nil, err
	}
	if exist != nil {
		return nil, nil
	}

	// dedup by title
	var (
		status    = doc.Status
		errMsg    string
		dedupedBy int64
	)
	row, err := e.db.DocumentRepo.GetByTitle(ctx, document.GetByTitleParams{
		Title:       doc.Title,
		PublishedAt: doc.PublishedAt.Add(-e.cfg.DocumentFilterTimeWindow),
		Status: document.NullDocumentStatus{
			DocumentStatus: document.DocumentStatusActive,
			Valid:          true,
		},
	})
	if err != nil {
		return nil, err
	}
	if row != nil {
		dedupedBy = row.ID
		status = document.DocumentStatusDeduped
		errMsg = "deduped by title"
	}

	lang := language.Detect(doc.Title)

	// save the document
	po, err := e.db.DocumentRepo.Create(ctx, document.CreateParams{
		Source:      string(doc.Source),
		Provider:    doc.Provider,
		Catalog:     doc.Catalog,
		Title:       doc.Title,
		Content:     doc.Content,
		Format:      doc.Format,
		Authors:     lo.Ternary(doc.Authors != nil, doc.Authors, []string{}),
		Lang:        lang.String(),
		Url:         doc.Url,
		Md5:         doc.Md5,
		Status:      status,
		DedupedBy:   dedupedBy,
		ErrMsg:      errMsg,
		PublishedAt: doc.PublishedAt,
	})
	if err != nil {
		return nil, err
	}

	// send to Redis Stream
	if status == document.DocumentStatusDraft || status == document.DocumentStatusPending {
		go func() {
			ctx := context.WithoutCancel(ctx)
			message := converter.DocumentRepo2Types(po)
			topic := e.cfg.DocumentDraftedTopic
			if doc.Status == document.DocumentStatusPending {
				topic = e.cfg.DocumentPendingTopic
			}
			if err := e.writeDocStream(ctx, topic, message); err != nil {
				logger.Ctx(ctx).Err(err).Msg("failed send document event to redis stream")
			}
		}()
	}

	return converter.DocumentRepo2Types(po), nil
}

// writeDocStream writes a document event to Redis Stream using configured prefix and topic.
func (e *Entity) writeDocStream(ctx context.Context, topic string, msg any) error {
	if e.cache == nil {
		return fmt.Errorf("redis cache is nil")
	}
	bytes, err := sonic.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal document event: %w", err)
	}
	streamKey := topic
	return e.cache.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: 200,
		Approx: true,
		ID:     "*",
		Values: map[string]any{
			"payload": bytes,
		},
	}).Err()
}

func (e *Entity) CountChannels(ctx context.Context, input *types.QueryChannelsInput) (int64, error) {
	var nullCatalog tg_channel.NullDocumentCatalog
	if input.Catalog != nil {
		nullCatalog.DocumentCatalog = tg_channel.DocumentCatalog(*input.Catalog)
		nullCatalog.Valid = true
	}

	count, err := e.db.TgChannelRepo.CountList(ctx, tg_channel.CountListParams{
		ID:      input.ID,
		Name:    input.Name,
		Source:  input.Source,
		Catalog: nullCatalog,
		Enabled: input.Enabled,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count list: %w", err)
	}
	return *count, nil
}

func (e *Entity) QueryChannels(ctx context.Context, input *types.QueryChannelsInput) ([]*types.Channel, error) {
	if input.Limit <= 0 {
		input.Limit = 20
	}
	if input.Limit > 100 {
		input.Limit = 100
	}

	var nullCatalog tg_channel.NullDocumentCatalog
	if input.Catalog != nil {
		nullCatalog.DocumentCatalog = tg_channel.DocumentCatalog(*input.Catalog)
		nullCatalog.Valid = true
	}

	list, err := e.db.TgChannelRepo.QueryList(ctx, tg_channel.QueryListParams{
		ID:      input.ID,
		Name:    input.Name,
		Source:  input.Source,
		Catalog: nullCatalog,
		Enabled: input.Enabled,
		Limit:   input.Limit,
		Offset:  input.Offset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query list: %w", err)
	}

	channels := make([]*types.Channel, 0, len(list))
	for _, item := range list {
		channel, err := converter.ChannelRepo2Types(&item)
		if err != nil {
			logger.Ctx(ctx).Err(err).Int64("id", item.ID).Msg("failed to convert tg channel")
			continue
		}
		channels = append(channels, channel)
	}

	return channels, nil
}

// GetChannelById 根据 ID 获取
func (e *Entity) GetChannelById(ctx context.Context, id int64) (*types.Channel, error) {
	po, err := e.db.TgChannelRepo.GetById(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get by id: %w", err)
	}
	if po == nil {
		return nil, fmt.Errorf("tg channel not found")
	}

	return converter.ChannelRepo2Types(po)
}

// CreateChannel 创建
func (e *Entity) CreateChannel(ctx context.Context, input *types.CreateChannelInput) (*types.Channel, error) {
	extractCfg, err := sonic.Marshal(input.ExtractCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal extract cfg: %w", err)
	}

	po, err := e.db.TgChannelRepo.Create(ctx, tg_channel.CreateParams{
		ID:         input.ID,
		Name:       input.Name,
		Title:      input.Title,
		Broadcast:  input.Broadcast,
		Source:     input.Source,
		Catalog:    tg_channel.DocumentCatalog(input.Catalog),
		ExtractCfg: extractCfg,
		Enabled:    input.Enabled,
	}, lo.ToPtr(input.ID))
	if err != nil {
		return nil, fmt.Errorf("failed to create: %w", err)
	}

	return converter.ChannelRepo2Types(po)
}

// UpdateChannel 更新
func (e *Entity) UpdateChannel(ctx context.Context, input *types.UpdateChannelInput) (*types.Channel, error) {
	extractCfg, err := sonic.Marshal(input.ExtractCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal extract cfg: %w", err)
	}

	catalog := tg_channel.NullDocumentCatalog{}
	if input.Catalog != nil {
		catalog.DocumentCatalog = tg_channel.DocumentCatalog(*input.Catalog)
		catalog.Valid = true
	}

	po, err := e.db.TgChannelRepo.Update(ctx, tg_channel.UpdateParams{
		ID:         input.ID,
		Name:       input.Name,
		Title:      input.Title,
		Broadcast:  input.Broadcast,
		Source:     input.Source,
		Catalog:    catalog,
		ExtractCfg: extractCfg,
		Enabled:    input.Enabled,
	}, lo.ToPtr(input.ID))
	if err != nil {
		return nil, fmt.Errorf("failed to update: %w", err)
	}

	return converter.ChannelRepo2Types(po)
}

// TestChannelExtract 测试文本提取
func (e *Entity) TestChannelExtract(ctx context.Context, extractCfg types.ExtractCfg, text string) (*types.ExtractResult, error) {
	return extract(ctx, extractCfg, text)
}

func (e *Entity) CleanOldDocuments(ctx context.Context, input CleanOldDocumentsInput) (*CleanOldDocumentsOutput, error) {
	retainDays := input.RetainDays
	if retainDays == nil || *retainDays <= 0 {
		retainDays = lo.ToPtr(7)
	}

	cutoffTime := time.Now().AddDate(0, 0, -*retainDays)
	logger.Ctx(ctx).Info().Msgf("start cleaning documents older than %d days, cutoff_time: %s", *retainDays, cutoffTime)

	countPtr, err := e.db.DocumentRepo.DeleteOldDocuments(ctx, cutoffTime)
	if err != nil {
		return nil, fmt.Errorf("failed to delete old documents: %w", err)
	}

	count := int64(0)
	if countPtr != nil {
		count = *countPtr
	}

	logger.Ctx(ctx).Info().Msgf("cleaned old documents, deleted_count: %d", count)
	return &CleanOldDocumentsOutput{DeletedCount: count}, nil
}
