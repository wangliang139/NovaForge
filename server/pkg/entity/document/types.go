package document

import (
	"github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
)

type BinanceAnnouncementEvent struct {
	CatalogId   int64  `json:"catalogId"`
	CatalogName string `json:"catalogName"`
	PublishDate int64  `json:"publishDate"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	Disclaimer  string `json:"disclaimer"`
}

type SyncOkxAnnouncementsInput struct {
	Days *int `json:"days"`
}

type SyncOkxAnnouncementsOutput struct {
	Count int `json:"count"`
}

type ScrapOkxAnnouncementsInput struct {
	Id int64 `json:"id"`
}

type ScrapOkxAnnouncementsOutput struct {
	Scraped int `json:"scraped"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

type SyncRsshubDocumentsInput struct {
	Path string `json:"path"`

	Source   string  `json:"source"`
	Provider string  `json:"provider"`
	Catalog  string  `json:"catalog"`
	Format   *string `json:"format"`

	Params map[string]any `json:"params"`
}

type SyncRsshubDocumentsOutput struct {
	Success int `json:"success"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

type SyncCalendarInput struct {
	StartDate *string `json:"start_date"`
	EndDate   *string `json:"end_date"`
}

type SyncCalendarOutput struct {
	Count int `json:"count"`
}

var SourceMap = map[types.DocumentSource]string{
	types.DocumentSourceBinance:       "币安",
	types.DocumentSourceOkx:           "欧易",
	types.DocumentSourceJin10:         "金十数据",
	types.DocumentSourceJinse:         "金色财经",
	types.DocumentSourceTheblockbeats: "律动",
	types.DocumentSourceTmtpost:       "钛媒体",
	types.DocumentSourceSlowmist:      "慢雾",
	types.DocumentSourceBloomberg:     "彭博社",
	types.DocumentSourceGelonghui:     "格隆汇",
	types.DocumentSourceForesightnews: "Foresight News",
	types.DocumentSourceWallstreet:    "华尔街见闻",
	types.DocumentSourceCailianshe:    "财联社",
	types.DocumentSourceFollowin:      "Followin",
	types.DocumentSourceFastbull:      "FastBull",
	types.DocumentSourceTwitter:       "Twitter",
	types.DocumentSourceHuxiu:         "虎嗅",
	types.DocumentSourceCoindesk:      "CoinDesk",
	types.DocumentSourcePanews:        "PANews",
	types.DocumentSourceZaobao:        "联合早报",
	types.DocumentSourceWublock:       "吴说区块链",
	types.DocumentSourceCointime:      "CoinTime",
	types.DocumentSourceCjmb:          "财经慢报",
	types.DocumentSourceChaincatcher:  "链捕手",
	types.DocumentSourceCjkx:          "财经快讯",
	types.DocumentSourceTechflow:      "TechFlow",
	types.DocumentSourceBqkx:          "币圈快讯",
	types.DocumentSourceBimi:          "币㊙️快讯",
	types.DocumentSourceXhqcankao:     "风向旗参考快讯",
	types.DocumentSourcePpbbb:         "币圈新闻即时快讯🅥",
	types.DocumentSourceLoopDNS:       "LoopDNS资讯播报",
	types.DocumentSourceCjzx:          "财经资讯",
	types.DocumentSourceOdaily:        "Odaily",
	types.DocumentSourceLslbd:         "链上老币登",
	types.DocumentSourceBitpush:       "Bitpush",
	types.DocumentSourceZhuxinshe:     "竹新社",
	types.DocumentSourceZaihuapd:      "科技圈🎗在花频道📮",
	types.DocumentSourceBWEnews:       "方程式新闻",
	types.DocumentSourceGodlyNews:     "Yummy 😋",
	types.DocumentSourceFencha:        "分叉财经",
}

func GetSourceText(source types.DocumentSource) string {
	sourceText, ok := SourceMap[source]
	if ok {
		return sourceText
	}
	return string(source)
}

type AiSummaryResult struct {
	Title          string   `json:"title"`
	Summary        string   `json:"summary"`
	Tags           []string `json:"tags"`
	Coins          []string `json:"coins"`
	Influence      string   `json:"influence"`
	InfluenceScore int      `json:"influence_score"`
	Sentiment      int      `json:"sentiment"`
}

var AiSummaryResultSchema = utils.LLM.GenerateSchema(AiSummaryResult{})

type CleanOldDocumentsInput struct {
	RetainDays *int `json:"retain_days"`
}

type CleanOldDocumentsOutput struct {
	DeletedCount int64 `json:"deleted_count"`
}
