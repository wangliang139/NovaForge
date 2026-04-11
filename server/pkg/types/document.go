package types

import (
	"time"

	"github.com/wangliang139/llt-trade/server/pkg/internal/extractor"
	"github.com/wangliang139/llt-trade/server/pkg/repos/calendar"
	"github.com/wangliang139/llt-trade/server/pkg/repos/document"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type QueryDocumentsInput struct {
	Offset           int64
	Limit            int64
	Source           *string
	Provider         *string
	Catalog          *document.DocumentCatalog
	Status           *document.DocumentStatus
	PublishedAtStart *timestamppb.Timestamp
	PublishedAtEnd   *timestamppb.Timestamp
	Keyword          *string
	Tag              *string
	Coin             *string
	InfluenceScore   *int32
	Sentiment        *int32
	Id               *int64
}

type Document struct {
	Id               int64                    `json:"id,omitempty"`
	Source           DocumentSource           `json:"source,omitempty"`
	Provider         string                   `json:"provider,omitempty"`
	Catalog          document.DocumentCatalog `json:"catalog,omitempty"`
	Title            string                   `json:"title,omitempty"`
	Content          string                   `json:"content,omitempty"`
	AiTitle          string                   `json:"ai_title,omitempty"`
	AiSummary        string                   `json:"ai_summary,omitempty"`
	AiTags           []string                 `json:"ai_tags,omitempty"`
	AiCoins          []string                 `json:"ai_coins,omitempty"`
	AiInfluence      string                   `json:"ai_influence,omitempty"`
	AiInfluenceScore int32                    `json:"ai_influence_score,omitempty"`
	AiSentiment      int32                    `json:"ai_sentiment,omitempty"`
	Format           document.DocumentFormat  `json:"format,omitempty"`
	Authors          []string                 `json:"authors,omitempty"`
	Lang             string                   `json:"lang,omitempty"`
	Url              string                   `json:"url,omitempty"`
	Md5              string                   `json:"md5,omitempty"`
	PublishedAt      time.Time                `json:"published_at,omitempty"`
	Status           document.DocumentStatus  `json:"status,omitempty"`
	ErrMsg           string                   `json:"err_msg,omitempty"`
	DedupedBy        int64                    `json:"deduped_by,omitempty"`
	CreatedAt        time.Time                `json:"created_at,omitempty"`
	UpdatedAt        time.Time                `json:"updated_at,omitempty"`
}

type DocumentSource string

const (
	DocumentSourceBinance       DocumentSource = "binance"
	DocumentSourceOkx           DocumentSource = "okx"
	DocumentSourceJin10         DocumentSource = "jin10"
	DocumentSourceJinse         DocumentSource = "jinse"
	DocumentSourceTheblockbeats DocumentSource = "theblockbeats"
	DocumentSourceTmtpost       DocumentSource = "tmtpost"
	DocumentSourceSlowmist      DocumentSource = "slowmist"
	DocumentSourceBloomberg     DocumentSource = "bloomberg"
	DocumentSourceGelonghui     DocumentSource = "gelonghui"
	DocumentSourceForesightnews DocumentSource = "foresightnews"
	DocumentSourceWallstreet    DocumentSource = "wallstreet"
	DocumentSourceCailianshe    DocumentSource = "cailianshe"
	DocumentSourceFollowin      DocumentSource = "followin"
	DocumentSourceFastbull      DocumentSource = "fastbull"
	DocumentSourceTwitter       DocumentSource = "twitter"
	DocumentSourceHuxiu         DocumentSource = "huxiu"
	DocumentSourceCoindesk      DocumentSource = "coindesk"
	DocumentSourcePanews        DocumentSource = "panews"
	DocumentSourceZaobao        DocumentSource = "zaobao"
	DocumentSourceWublock       DocumentSource = "wublock"
	DocumentSourceCointime      DocumentSource = "cointime"
	DocumentSourceCjmb          DocumentSource = "cjmb"
	DocumentSourceChaincatcher  DocumentSource = "chaincatcher"
	DocumentSourceCjkx          DocumentSource = "cjkx"
	DocumentSourceTechflow      DocumentSource = "techflow"
	DocumentSourceBqkx          DocumentSource = "bqkx"
	DocumentSourceBimi          DocumentSource = "bimi"
	DocumentSourceXhqcankao     DocumentSource = "xhqcankao"
	DocumentSourcePpbbb         DocumentSource = "ppbbb"
	DocumentSourceLoopDNS       DocumentSource = "loopdns"
	DocumentSourceCjzx          DocumentSource = "cjzx"
	DocumentSourceOdaily        DocumentSource = "odaily"
	DocumentSourceLslbd         DocumentSource = "lslbd"
	DocumentSourceBitpush       DocumentSource = "bitpush"
	DocumentSourceZhuxinshe     DocumentSource = "zhuxinshe"
	DocumentSourceZaihuapd      DocumentSource = "zaihuapd"
	DocumentSourceBWEnews       DocumentSource = "BWEnews"
	DocumentSourceGodlyNews     DocumentSource = "GodlyNews"
	DocumentSourceFencha        DocumentSource = "fencha"
)

// Channel 业务层的 Channel 类型
type Channel struct {
	ID         int64
	Name       string
	Title      string
	Broadcast  bool
	Source     string
	Catalog    document.DocumentCatalog
	ExtractCfg ExtractCfg
	Enabled    bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ExtractCfg 提取配置
type ExtractCfg struct {
	Plans        []ExtractPlan `json:"plans"`
	FilterRegexs []string      `json:"filter_regexs,omitempty"`
}

type ExtractPlan struct {
	SeqNo      int32          `json:"seq_no"`
	MatchRegex string         `json:"match_regex"`
	Fields     []ExtractField `json:"fields"`
}

// StructureField 提取字段定义
type ExtractField struct {
	Key        string         `json:"key"`
	Rule       extractor.Rule `json:"rule"`
	TimeFormat string         `json:"time_format,omitempty"`
}

// QueryChannelsInput 查询输入参数
type QueryChannelsInput struct {
	Limit   int32
	Offset  int32
	ID      *int64
	Name    *string
	Source  *string
	Catalog *document.DocumentCatalog
	Enabled *bool
}

// CreateChannelInput 创建输入参数
type CreateChannelInput struct {
	ID         int64
	Name       string
	Title      string
	Broadcast  bool
	Source     string
	Catalog    document.DocumentCatalog
	ExtractCfg ExtractCfg
	Enabled    bool
}

// UpdateChannelInput 更新输入参数
type UpdateChannelInput struct {
	ID         int64
	Name       *string
	Title      *string
	Broadcast  *bool
	Source     *string
	Catalog    *document.DocumentCatalog
	ExtractCfg *ExtractCfg
	Enabled    *bool
}

// ExtractResult 测试提取的返回结果
type ExtractResult struct {
	Filtered    bool
	HitPlan     *int32
	Title       *string
	Content     *string
	Url         *string
	PublishedAt *time.Time
}

// --- documentsvc API ---

type QueryDocumentsRequest struct {
	Offset           int64
	Limit            int64
	Id               *int64
	Keyword          *string
	Source           *string
	Provider         *string
	Catalog          *document.DocumentCatalog
	Status           *document.DocumentStatus
	Tag              *string
	Coin             *string
	InfluenceScore   *int32
	Sentiment        *int32
	PublishedAtStart *timestamppb.Timestamp
	PublishedAtEnd   *timestamppb.Timestamp
}

type QueryDocumentsResponse struct {
	Count     int64
	Documents []*Document
}

type QueryCalendarRequest struct {
	DateID        int32
	Source        *calendar.CalendarSource
	Type          *calendar.CalendarType
	Category      *string
	Country       *string
	MinImportance *int32
}

type QueryCalendarResponse struct {
	Calendars []*Calendar
}

type GetDocumentRequest struct {
	Id int64
}

type GetDocumentResponse struct {
	Document *Document
}

type ArchiveDocumentsRequest struct {
	Id int64
}

type ArchiveDocumentsResponse struct {
	Document *Document
}

type DocumentStatsSummary struct {
	TotalCount            int64
	SuccessCount          int64
	SuccessRate           float64
	AvgPublishToIngestSec float64
	AvgIngestToSuccessSec float64
}

type ChannelDocumentCount struct {
	Source        string
	Provider      string
	DocumentCount int64
	SuccessCount  int64
}

type GetDocumentStatsRequest struct {
	StartTs int64
	EndTs   int64
}

type GetDocumentStatsResponse struct {
	Stats         *DocumentStatsSummary
	ChannelCounts []*ChannelDocumentCount
}

type QueryChannelsRequest struct {
	Offset  int32
	Limit   int32
	Id      *int64
	Name    *string
	Source  *string
	Catalog *document.DocumentCatalog
	Enabled *bool
}

type QueryChannelsResponse struct {
	TotalCount int64
	Channels   []*Channel
}

type GetChannelRequest struct {
	Id int64
}

type GetChannelResponse struct {
	Channel *Channel
}

type CreateChannelRequest struct {
	Id         int64
	Name       string
	Title      string
	Broadcast  bool
	Source     string
	Catalog    document.DocumentCatalog
	ExtractCfg *ExtractCfg
	Enabled    bool
}

type CreateChannelResponse struct {
	Channel *Channel
}

type UpdateChannelRequest struct {
	Id         int64
	Name       *string
	Title      *string
	Broadcast  *bool
	Source     *string
	Catalog    *document.DocumentCatalog
	ExtractCfg *ExtractCfg
	Enabled    *bool
}

type UpdateChannelResponse struct {
	Channel *Channel
}

type TestExtractRequest struct {
	Text       string
	ExtractCfg *ExtractCfg
}

type TestExtractResponse struct {
	Filtered    bool
	HitPlan     *int32
	Title       *string
	Content     *string
	Url         *string
	PublishedAt *time.Time
}

type CompareDocumentsRequest struct {
	LeftId  int64
	RightId int64
}

type CompareDocumentsResponse struct {
	Similarity float64
}
