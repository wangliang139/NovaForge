package documentsvc

import (
	"context"
	"fmt"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
	"github.com/wangliang139/llt-trade/server/pkg/entity"
	"github.com/wangliang139/llt-trade/server/pkg/repos/document"
	"github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/mow/errors"
	"github.com/wangliang139/mow/logger"
)

type Config struct{}

func (c *Config) String() string {
	if c == nil {
		return "nil"
	}
	tmp := *c
	return fmt.Sprintf("%+v", tmp)
}

type Service struct {
	cfg Config
}

func New() (*Service, error) {
	var cfg Config
	envconfig.MustProcess("document_svc", &cfg)
	log.Info().Msgf("document service config: %+v", &cfg)
	return &Service{
		cfg: cfg,
	}, nil
}

func (s *Service) QueryDocuments(ctx context.Context, request *types.QueryDocumentsRequest) (*types.QueryDocumentsResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("QueryDocuments params")
	if request.Offset < 0 {
		return nil, errors.New(errors.InvalidArgument, "offset is invalid")
	}
	if request.Limit <= 0 {
		return nil, errors.New(errors.InvalidArgument, "limit is invalid")
	}

	count, err := entity.Document.QueryDocumentsCount(ctx, request)
	if err != nil {
		return nil, err
	}

	resp := &types.QueryDocumentsResponse{
		Count: count,
	}

	if request.Offset >= count {
		return resp, nil
	}

	documents, err := entity.Document.QueryDocuments(ctx, request)
	if err != nil {
		return nil, err
	}

	resp.Documents = make([]*types.Document, len(documents))
	copy(resp.Documents, documents)
	return resp, nil
}

func (s *Service) QueryCalendar(ctx context.Context, request *types.QueryCalendarRequest) (*types.QueryCalendarResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("QueryCalendar params")

	calendars, err := entity.Document.QueryCalendar(ctx, request)
	if err != nil {
		return nil, err
	}

	resp := &types.QueryCalendarResponse{
		Calendars: make([]*types.Calendar, len(calendars)),
	}

	copy(resp.Calendars, calendars)
	return resp, nil
}

func (s *Service) GetDocument(ctx context.Context, request *types.GetDocumentRequest) (*types.GetDocumentResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("GetDocument params")

	doc, err := entity.Document.GetDocument(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	return &types.GetDocumentResponse{
		Document: doc,
	}, nil
}

func (s *Service) ArchiveDocuments(ctx context.Context, request *types.ArchiveDocumentsRequest) (*types.ArchiveDocumentsResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("ArchiveDocuments params")

	doc, err := entity.Document.ArchiveDocument(ctx, request.Id)
	if err != nil {
		return nil, err
	}
	return &types.ArchiveDocumentsResponse{
		Document: doc,
	}, nil
}

func (s *Service) GetDocumentStats(ctx context.Context, request *types.GetDocumentStatsRequest) (*types.GetDocumentStatsResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("GetDocumentStats params")

	result, err := entity.Document.GetDocumentStats(ctx, request.StartTs, request.EndTs)
	if err != nil {
		return nil, err
	}

	channelCounts := make([]*types.ChannelDocumentCount, 0, len(result.ChannelDocumentCounts))
	for _, c := range result.ChannelDocumentCounts {
		channelCounts = append(channelCounts, &types.ChannelDocumentCount{
			Source:        c.Source,
			Provider:      c.Provider,
			DocumentCount: c.DocumentCount,
			SuccessCount:  c.SuccessCount,
		})
	}

	return &types.GetDocumentStatsResponse{
		Stats: &types.DocumentStatsSummary{
			TotalCount:            result.TotalCount,
			SuccessCount:          result.SuccessCount,
			SuccessRate:           result.SuccessRate,
			AvgPublishToIngestSec: result.AvgPublishToIngestSec,
			AvgIngestToSuccessSec: result.AvgIngestToSuccessSec,
		},
		ChannelCounts: channelCounts,
	}, nil
}

// QueryChannels 查询列表
func (s *Service) QueryChannels(ctx context.Context, request *types.QueryChannelsRequest) (*types.QueryChannelsResponse, error) {
	log.Debug().Interface("request", request).Msg("QueryChannels params")

	if request.Offset < 0 {
		return nil, errors.New(errors.InvalidArgument, "offset is invalid")
	}
	if request.Limit <= 0 {
		request.Limit = 20
	}
	if request.Limit > 100 {
		request.Limit = 100
	}

	var catalog *document.DocumentCatalog
	if request.Catalog != nil {
		catalog = request.Catalog
	}

	input := &types.QueryChannelsInput{
		Limit:   request.Limit,
		Offset:  request.Offset,
		ID:      request.Id,
		Name:    request.Name,
		Source:  request.Source,
		Catalog: catalog,
		Enabled: request.Enabled,
	}

	count, err := entity.Document.CountChannels(ctx, input)
	if err != nil {
		return nil, err
	}

	if int64(request.Offset) >= count {
		return &types.QueryChannelsResponse{
			TotalCount: count,
		}, nil
	}

	channels, err := entity.Document.QueryChannels(ctx, input)
	if err != nil {
		return nil, err
	}

	resp := &types.QueryChannelsResponse{
		TotalCount: count,
		Channels:   make([]*types.Channel, 0, len(channels)),
	}

	for _, channel := range channels {
		resp.Channels = append(resp.Channels, channel)
	}

	return resp, nil
}

// GetChannel 获取详情
func (s *Service) GetChannel(ctx context.Context, request *types.GetChannelRequest) (*types.GetChannelResponse, error) {
	log.Debug().Interface("request", request).Msg("GetTgChannel params")

	if request.Id <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is invalid")
	}

	channel, err := entity.Document.GetChannelById(ctx, request.Id)
	if err != nil {
		return nil, err
	}

	return &types.GetChannelResponse{
		Channel: channel,
	}, nil
}

// CreateTgChannel 创建
func (s *Service) CreateChannel(ctx context.Context, request *types.CreateChannelRequest) (*types.CreateChannelResponse, error) {
	log.Debug().Interface("request", request).Msg("CreateTgChannel params")

	id := request.Id
	if id <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}

	name := strings.TrimSpace(request.Name)
	if len(name) == 0 {
		return nil, errors.New(errors.InvalidArgument, "name is required")
	}

	title := strings.TrimSpace(request.Title)
	if len(title) == 0 {
		return nil, errors.New(errors.InvalidArgument, "title is required")
	}

	source := strings.TrimSpace(request.Source)
	if len(source) == 0 {
		return nil, errors.New(errors.InvalidArgument, "source is required")
	}

	if !request.Catalog.Valid() {
		return nil, errors.New(errors.InvalidArgument, "catalog is required")
	}

	extractCfg := request.ExtractCfg
	if extractCfg == nil {
		extractCfg = &types.ExtractCfg{}
	}

	input := &types.CreateChannelInput{
		ID:         id,
		Name:       name,
		Title:      title,
		Broadcast:  request.Broadcast,
		Source:     source,
		Catalog:    request.Catalog,
		ExtractCfg: *extractCfg,
		Enabled:    request.Enabled,
	}

	channel, err := entity.Document.CreateChannel(ctx, input)
	if err != nil {
		return nil, err
	}

	return &types.CreateChannelResponse{
		Channel: channel,
	}, nil
}

// UpdateTgChannel 更新
func (s *Service) UpdateChannel(ctx context.Context, request *types.UpdateChannelRequest) (*types.UpdateChannelResponse, error) {
	log.Debug().Interface("request", request).Msg("UpdateTgChannel params")

	if request.Id <= 0 {
		return nil, errors.New(errors.InvalidArgument, "id is invalid")
	}

	input := &types.UpdateChannelInput{
		ID:         request.Id,
		Name:       request.Name,
		Title:      request.Title,
		Broadcast:  request.Broadcast,
		Source:     request.Source,
		Catalog:    request.Catalog,
		ExtractCfg: request.ExtractCfg,
		Enabled:    request.Enabled,
	}

	channel, err := entity.Document.UpdateChannel(ctx, input)
	if err != nil {
		return nil, err
	}

	return &types.UpdateChannelResponse{
		Channel: channel,
	}, nil
}

// TestExtract 测试文本提取
func (s *Service) TestExtract(ctx context.Context, request *types.TestExtractRequest) (*types.TestExtractResponse, error) {
	log.Debug().Interface("request", request).Msg("TestExtract params")

	if len(request.Text) == 0 {
		return nil, errors.New(errors.InvalidArgument, "text is required")
	}

	extractCfg := request.ExtractCfg
	if extractCfg == nil {
		return nil, errors.New(errors.InvalidArgument, "extract cfg is required")
	}
	result, err := entity.Document.TestChannelExtract(ctx, *extractCfg, request.Text)
	if err != nil {
		return nil, err
	}

	return &types.TestExtractResponse{
		Filtered:    result.Filtered,
		HitPlan:     result.HitPlan,
		Title:       result.Title,
		Content:     result.Content,
		Url:         result.Url,
		PublishedAt: result.PublishedAt,
	}, nil
}

func (s *Service) CompareDocuments(ctx context.Context, request *types.CompareDocumentsRequest) (*types.CompareDocumentsResponse, error) {
	logger.Ctx(ctx).Debug().Interface("request", request).Msg("CompareDocuments params")

	if request.LeftId <= 0 || request.RightId <= 0 {
		return nil, errors.New(errors.InvalidArgument, "document id is invalid")
	}

	score, err := entity.Document.CompareDocuments(ctx, request.LeftId, request.RightId)
	if err != nil {
		return nil, err
	}

	return &types.CompareDocumentsResponse{
		Similarity: float64(score),
	}, nil
}
