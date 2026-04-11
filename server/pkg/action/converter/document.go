package converter

import (
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/samber/lo"
	"github.com/wangliang139/llt-trade/server/pkg/internal/extractor"
	"github.com/wangliang139/llt-trade/server/pkg/action/model"
	"github.com/wangliang139/llt-trade/server/pkg/repos/calendar"
	"github.com/wangliang139/llt-trade/server/pkg/repos/document"
	"github.com/wangliang139/llt-trade/server/pkg/types"
)

func DocumentCatalogGql2Repo(c *model.DocumentCatalog) document.DocumentCatalog {
	if c == nil {
		return ""
	}
	return DocumentCatalogModelToRepo(*c)
}

// DocumentCatalogModelToRepo maps GraphQL enum to persistence catalog.
func DocumentCatalogModelToRepo(c model.DocumentCatalog) document.DocumentCatalog {
	switch c {
	case model.DocumentCatalogAirdrop:
		return document.DocumentCatalogAirdrop
	case model.DocumentCatalogAPI:
		return document.DocumentCatalogApi
	case model.DocumentCatalogCryptocurrencyListing:
		return document.DocumentCatalogCryptocurrencyListing
	case model.DocumentCatalogCryptocurrencyDelisting:
		return document.DocumentCatalogCryptocurrencyDelisting
	case model.DocumentCatalogActivity:
		return document.DocumentCatalogActivity
	case model.DocumentCatalogNews:
		return document.DocumentCatalogNews
	case model.DocumentCatalogFlashNews:
		return document.DocumentCatalogFlashNews
	case model.DocumentCatalogOther:
		return document.DocumentCatalogOther
	default:
		return ""
	}
}

func DocumentStatusGql2Repo(s *model.DocumentStatus) document.DocumentStatus {
	if s == nil {
		return ""
	}
	switch *s {
	case model.DocumentStatusDraft:
		return document.DocumentStatusDraft
	case model.DocumentStatusDraftFailed:
		return document.DocumentStatusDraftFailed
	case model.DocumentStatusPending:
		return document.DocumentStatusPending
	case model.DocumentStatusPendingFailed:
		return document.DocumentStatusPendingFailed
	case model.DocumentStatusActive:
		return document.DocumentStatusActive
	case model.DocumentStatusArchived:
		return document.DocumentStatusArchived
	case model.DocumentStatusDeduped:
		return document.DocumentStatusDeduped
	case model.DocumentStatusTimeout:
		return document.DocumentStatusTimeout
	default:
		return ""
	}
}

func CalendarSourceGql2Repo(s *model.CalendarSource) *calendar.CalendarSource {
	if s == nil {
		return nil
	}
	v := calendar.CalendarSource(*s)
	return &v
}

func CalendarTypeGql2Repo(t *model.CalendarType) *calendar.CalendarType {
	if t == nil {
		return nil
	}
	switch *t {
	case model.CalendarTypeEconomicData:
		return lo.ToPtr(calendar.CalendarTypeEconomicData)
	case model.CalendarTypeProjectEvent:
		return lo.ToPtr(calendar.CalendarTypeProjectEvent)
	case model.CalendarTypeTokenUnlock:
		return lo.ToPtr(calendar.CalendarTypeTokenUnlock)
	case model.CalendarTypeSummitEvent:
		return lo.ToPtr(calendar.CalendarTypeSummitEvent)
	case model.CalendarTypeFinancing:
		return lo.ToPtr(calendar.CalendarTypeFinancing)
	case model.CalendarTypeEvents:
		return lo.ToPtr(calendar.CalendarTypeEvents)
	case model.CalendarTypeOther:
		return lo.ToPtr(calendar.CalendarTypeOther)
	}
	return nil
}

func DocumentCatalogRepo2Gql(catalog document.DocumentCatalog) model.DocumentCatalog {
	switch catalog {
	case document.DocumentCatalogAirdrop:
		return model.DocumentCatalogAirdrop
	case document.DocumentCatalogApi:
		return model.DocumentCatalogAPI
	case document.DocumentCatalogCryptocurrencyListing:
		return model.DocumentCatalogCryptocurrencyListing
	case document.DocumentCatalogCryptocurrencyDelisting:
		return model.DocumentCatalogCryptocurrencyDelisting
	case document.DocumentCatalogActivity:
		return model.DocumentCatalogActivity
	case document.DocumentCatalogNews:
		return model.DocumentCatalogNews
	case document.DocumentCatalogFlashNews:
		return model.DocumentCatalogFlashNews
	case document.DocumentCatalogOther:
		return model.DocumentCatalogOther
	default:
		return model.DocumentCatalogUnspecified
	}
}

func DocumentFormatRepo2Gql(f document.DocumentFormat) model.DocumentFormat {
	switch f {
	case document.DocumentFormatMarkdown:
		return model.DocumentFormatMarkdown
	case document.DocumentFormatTxt:
		return model.DocumentFormatTxt
	case document.DocumentFormatHtml:
		return model.DocumentFormatHTML
	default:
		return model.DocumentFormatUnspecified
	}
}

func DocumentStatusRepo2Gql(s document.DocumentStatus) model.DocumentStatus {
	switch s {
	case document.DocumentStatusDraft:
		return model.DocumentStatusDraft
	case document.DocumentStatusDraftFailed:
		return model.DocumentStatusDraftFailed
	case document.DocumentStatusPending:
		return model.DocumentStatusPending
	case document.DocumentStatusPendingFailed:
		return model.DocumentStatusPendingFailed
	case document.DocumentStatusActive:
		return model.DocumentStatusActive
	case document.DocumentStatusArchived:
		return model.DocumentStatusArchived
	case document.DocumentStatusDeduped:
		return model.DocumentStatusDeduped
	case document.DocumentStatusTimeout:
		return model.DocumentStatusTimeout
	default:
		return model.DocumentStatusUnspecified
	}
}

func DocumentTypes2Gql(d *types.Document) *model.Document {
	if d == nil {
		return nil
	}
	return &model.Document{
		ID:               strconv.FormatInt(d.Id, 10),
		Source:           string(d.Source),
		Provider:         d.Provider,
		Catalog:          DocumentCatalogRepo2Gql(d.Catalog),
		Title:            d.Title,
		Content:          d.Content,
		AiTitle:          d.AiTitle,
		AiSummary:        d.AiSummary,
		AiTags:           d.AiTags,
		AiCoins:          d.AiCoins,
		AiInfluence:      d.AiInfluence,
		AiInfluenceScore: int(d.AiInfluenceScore),
		AiSentiment:      int(d.AiSentiment),
		URL:              d.Url,
		Authors:          d.Authors,
		Format:           DocumentFormatRepo2Gql(d.Format),
		Lang:             d.Lang,
		Md5:              d.Md5,
		Status:           DocumentStatusRepo2Gql(d.Status),
		ErrMsg:           d.ErrMsg,
		DedupedBy:        lo.If(d.DedupedBy == 0, "").Else(strconv.FormatInt(d.DedupedBy, 10)),
		PublishedAt:      int(d.PublishedAt.Unix()),
		CreatedAt:        int(d.CreatedAt.Unix()),
		UpdatedAt:        int(d.UpdatedAt.Unix()),
	}
}

func CalendarRepoSource2Gql(s calendar.CalendarSource) model.CalendarSource {
	return model.CalendarSource(s)
}

func CalendarRepoType2Gql(t calendar.CalendarType) model.CalendarType {
	return model.CalendarType(t)
}

func CalendarTypes2Gql(c *types.Calendar) *model.Calendar {
	if c == nil {
		return nil
	}
	var ext model.CalendarExtention
	if len(c.Ext) > 0 && c.Type == calendar.CalendarTypeEconomicData {
		var e types.EconomicCalendarExtension
		if err := sonic.Unmarshal(c.Ext, &e); err == nil {
			ext = &model.EconomicCalendarExtention{
				Unit:      e.Unit,
				Actual:    e.Actual,
				Previous:  e.Previous,
				Consensus: e.Consensus,
			}
		}
	}
	return &model.Calendar{
		ID:          strconv.FormatInt(c.ID, 10),
		DateID:      int(c.DateID),
		Source:      CalendarRepoSource2Gql(c.Source),
		Type:        CalendarRepoType2Gql(c.Type),
		Sid:         c.Sid,
		Category:    c.Category,
		Country:     c.Country,
		Project:     c.Project,
		Symbol:      c.Symbol,
		Title:       c.Title,
		Content:     c.Content,
		Importance:  int(c.Importance),
		URL:         lo.ToPtr(c.Url),
		Ext:         ext,
		PublishedAt: int(c.PublishedAt.Unix()),
		CreatedAt:   int(c.CreatedAt.Unix()),
		UpdatedAt:   int(c.UpdatedAt.Unix()),
	}
}

func extractRuleTypeGql2Extractor(gql model.ExtractRuleType) extractor.RuleType {
	switch gql {
	case model.ExtractRuleTypeRegex:
		return extractor.RuleTypeRegex
	case model.ExtractRuleTypeXpath:
		return extractor.RuleTypeXPath
	default:
		return ""
	}
}

func extractRuleTypeExtractor2Gql(t extractor.RuleType) model.ExtractRuleType {
	switch t {
	case extractor.RuleTypeRegex:
		return model.ExtractRuleTypeRegex
	case extractor.RuleTypeXPath:
		return model.ExtractRuleTypeXpath
	default:
		return model.ExtractRuleTypeUnspecified
	}
}

func ExtractCfgGql2Types(cfg *model.ExtractCfgInput) *types.ExtractCfg {
	if cfg == nil {
		return nil
	}
	plans := make([]types.ExtractPlan, 0, len(cfg.Plans))
	for _, plan := range cfg.Plans {
		fields := make([]types.ExtractField, 0, len(plan.Fields))
		for _, field := range plan.Fields {
			rule := extractor.Rule{}
			if field.Rule != nil {
				rule.Type = extractRuleTypeGql2Extractor(field.Rule.Type)
				rule.Pattern = field.Rule.Pattern
				rule.Group = field.Rule.Group
			}
			tf := ""
			if field.TimeFormat != nil {
				tf = *field.TimeFormat
			}
			fields = append(fields, types.ExtractField{
				Key:        field.Key,
				Rule:       rule,
				TimeFormat: tf,
			})
		}
		seq := int32(plan.SeqNo)
		match := ""
		if plan.MatchRegex != nil {
			match = *plan.MatchRegex
		}
		plans = append(plans, types.ExtractPlan{
			SeqNo:      seq,
			MatchRegex: match,
			Fields:     fields,
		})
	}
	return &types.ExtractCfg{
		Plans:        plans,
		FilterRegexs: cfg.FilterRegexs,
	}
}

func ExtractCfgTypes2Gql(cfg *types.ExtractCfg) *model.ExtractCfg {
	if cfg == nil {
		return nil
	}
	plans := make([]*model.ExtractPlan, 0, len(cfg.Plans))
	for _, plan := range cfg.Plans {
		fields := make([]*model.ExtractField, 0, len(plan.Fields))
		for _, field := range plan.Fields {
			tf := field.TimeFormat
			fields = append(fields, &model.ExtractField{
				Key: field.Key,
				Rule: &model.ExtractRule{
					Type:    extractRuleTypeExtractor2Gql(field.Rule.Type),
					Pattern: field.Rule.Pattern,
					Group:   field.Rule.Group,
				},
				TimeFormat: &tf,
			})
		}
		plans = append(plans, &model.ExtractPlan{
			SeqNo:      int(plan.SeqNo),
			MatchRegex: lo.ToPtr(plan.MatchRegex),
			Fields:     fields,
		})
	}
	return &model.ExtractCfg{
		Plans:        plans,
		FilterRegexs: cfg.FilterRegexs,
	}
}

func ChannelTypes2Gql(ch *types.Channel) *model.Channel {
	if ch == nil {
		return nil
	}
	return &model.Channel{
		ID:         strconv.FormatInt(ch.ID, 10),
		Name:       ch.Name,
		Title:      ch.Title,
		Broadcast:  ch.Broadcast,
		Source:     ch.Source,
		Catalog:    DocumentCatalogRepo2Gql(ch.Catalog),
		ExtractCfg: ExtractCfgTypes2Gql(&ch.ExtractCfg),
		Enabled:    ch.Enabled,
		CreatedAt:  int(ch.CreatedAt.Unix()),
		UpdatedAt:  int(ch.UpdatedAt.Unix()),
	}
}
