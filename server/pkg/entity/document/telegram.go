package document

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/bytedance/sonic"
	"github.com/microcosm-cc/bluemonday"
	"github.com/samber/lo"
	"github.com/wangliang139/NovaForge/server/pkg/internal/extractor"
	"github.com/wangliang139/NovaForge/server/pkg/internal/push"
	"github.com/wangliang139/NovaForge/server/pkg/repos/document"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/logger"
	"github.com/yuin/goldmark"
)

var location = time.FixedZone("Asia/Shanghai", 8*3600)

func (e *Entity) sendDocumentToTelegram(ctx context.Context, message types.Document) error {
	if message.AiInfluenceScore <= 1 {
		return nil
	}

	args, err := buildDocumentNotifyArgs(message)
	if err != nil {
		return err
	}

	retryTimes := 3
	retryDelay := 1 * time.Second
	for i := 0; i < retryTimes; i++ {
		err = push.NotifyByTemplate(ctx, push.NotifyByTemplateRequest{
			SceneKey:   "document.summary",
			Vars: args,
		})
		if err != nil {
			logger.Ctx(ctx).Err(err).Int64("document_id", message.Id).Msg("failed to push document, retry...")
			time.Sleep(retryDelay)
			continue
		}
		break
	}
	return err
}

func buildDocumentNotifyArgs(doc types.Document) (map[string]any, error) {
	// AiSentiment
	sentiment := ""
	switch doc.AiSentiment {
	case 2:
		sentiment = "🔥🔥"
	case 1:
		sentiment = "🔥"
	case -1:
		sentiment = "😞"
	case -2:
		sentiment = "😞😞"
	}

	sb := strings.Builder{}
	if len(sentiment) > 0 {
		sb.WriteString(fmt.Sprintf("<b>【%s】</b> %s\n\n", doc.AiTitle, sentiment))
	} else {
		sb.WriteString(fmt.Sprintf("<b>【%s】</b>\n\n", doc.AiTitle))
	}

	// AiTags
	tags := make([]string, 0, len(doc.AiTags))
	if len(doc.AiTags) > 0 {
		for _, tag := range doc.AiTags {
			safeTag := html.EscapeString(tag)
			tags = append(tags, fmt.Sprintf("<code>#%s</code>", safeTag))
		}
		sb.WriteString(fmt.Sprintf("🏷️ %s\n\n", strings.Join(tags, " ")))
	}

	content := doc.AiSummary
	if len(doc.AiSummary) > 0 {
		// markdown to html
		if doc.Format == document.DocumentFormatMarkdown {
			var buf bytes.Buffer
			if err := goldmark.Convert([]byte(content), &buf); err != nil {
				return nil, err
			}
			content = buf.String()
		}
		content = cleanHTMLForTelegram(content)
		sb.WriteString(fmt.Sprintf("<blockquote>%s</blockquote>\n\n", content))
	}

	incluenceEmoji := "0️⃣"
	switch doc.AiInfluenceScore {
	case 1:
		incluenceEmoji = "1️⃣"
	case 2:
		incluenceEmoji = "2️⃣"
	case 3:
		incluenceEmoji = "3️⃣"
	case 4:
		incluenceEmoji = "4️⃣"
	case 5:
		incluenceEmoji = "5️⃣"
	}
	sb.WriteString(fmt.Sprintf("%s 影响：<b>%s</b>\n", incluenceEmoji, html.EscapeString(doc.AiInfluence)))

	sourceText := GetSourceText(doc.Source)
	sb.WriteString(fmt.Sprintf("📌 来自：%s", sourceText))

	authorsText := ""
	if len(doc.Authors) > 0 {
		formatedAuthors := []string{}
		for _, author := range doc.Authors {
			if len(author) > 0 {
				formatedAuthors = append(formatedAuthors, fmt.Sprintf("<code>%s</code>", author))
			}
		}
		if len(formatedAuthors) > 0 {
			authorsText = strings.Join(formatedAuthors, ",")
		}
	}
	if len(authorsText) > 0 {
		sb.WriteString(fmt.Sprintf(" | %s", authorsText))
	}
	sb.WriteString("\n")

	localtime := doc.PublishedAt.In(location).Format("2006-01-02 15:04:05")
	sb.WriteString(fmt.Sprintf("🕒 %s | 🔗 <a href=\"%s\">查看详情</a>", localtime, doc.Url))

	return map[string]any{
		"title": doc.AiTitle,
		"sentiment": sentiment,
		"tags": tags,
		"content": content,
		"influence": doc.AiInfluence,
		"influenceScore": doc.AiInfluenceScore,
		"source": sourceText,
		"authors": authorsText,
		"url": doc.Url,
		"time": localtime,
		"html": sb.String(),
	}, nil
}

func cleanHTMLForTelegram(input string) string {
	// 仅允许 Telegram 支持的标签
	policy := bluemonday.NewPolicy()
	policy.AllowElements("b", "strong", "i", "em", "u", "ins", "s", "strike", "del", "span", "code", "pre", "blockquote", "a", "tg-spoiler", "tg-emoji")
	policy.AllowAttrs("href").OnElements("a")
	policy.AllowAttrs("class").OnElements("span", "code")
	policy.AllowAttrs("emoji-id").OnElements("tg-emoji")
	policy.RequireNoFollowOnLinks(false)
	return policy.Sanitize(input)
}

func (e *Entity) CosumeTgSubscribedMessage(ctx context.Context, message *telegram.NewMessage, channel *telegram.Channel) (*string, error) {
	if len(message.RawText(false)) == 0 {
		return nil, nil
	}

	channelCfg, err := e.db.TgChannelRepo.GetById(ctx, channel.ID)
	if err != nil {
		return nil, errors.New("failed to get channel config")
	}
	if channelCfg == nil || !channelCfg.Enabled {
		return nil, nil
	}

	var extractCfg types.ExtractCfg
	if err := sonic.Unmarshal(channelCfg.ExtractCfg, &extractCfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal extract cfg: %w", err)
	}

	extractResult, err := extract(ctx, extractCfg, message.RawText(false))
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("failed to extract")
	}
	if extractResult.Filtered {
		logger.Ctx(ctx).Info().Msg("message filtered by filter regex")
		return nil, nil
	}

	var (
		title       = extractResult.Title
		content     = extractResult.Content
		url         = extractResult.Url
		publishedAt = extractResult.PublishedAt
	)
	if title == nil {
		title = lo.ToPtr(message.RawText(false))
	}
	if content == nil {
		content = lo.ToPtr(message.RawText(false))
	}
	if publishedAt == nil {
		publishedAt = lo.ToPtr(time.Unix(int64(message.Date()), 0))
	}

	provider := fmt.Sprintf("telegram:%d", channel.ID)

	dateId := utils.Datetime.TimeToDateID(*publishedAt)
	md5, err := utils.Hash.Md5(fmt.Sprintf("%s\n%s\n%d\n%s", channelCfg.Source, provider, dateId, *content))
	if err != nil {
		return nil, fmt.Errorf("failed to get md5: %w", err)
	}

	po, err := e.SaveDocument(ctx, &types.Document{
		Source:      types.DocumentSource(channelCfg.Source),
		Provider:    provider,
		Catalog:     document.DocumentCatalog(channelCfg.Catalog),
		Title:       lo.FromPtr(title),
		Content:     lo.FromPtr(content),
		Format:      document.DocumentFormatHtml,
		Md5:         md5,
		Url:         lo.FromPtr(url),
		PublishedAt: lo.FromPtr(publishedAt),
		Status:      document.DocumentStatusPending,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save document: %w", err)
	}
	if po == nil {
		return nil, nil
	}
	return lo.ToPtr(fmt.Sprintf("%d", po.Id)), nil
}

func extract(ctx context.Context, extractCfg types.ExtractCfg, message string) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	if len(message) == 0 {
		return result, nil
	}

	// 过滤不需要的消息
	for _, filterRegex := range extractCfg.FilterRegexs {
		if len(filterRegex) > 0 {
			re, err := extractor.GetRegexp(filterRegex)
			if err != nil {
				logger.Ctx(ctx).Err(err).Msg("invalid filter regex")
				continue
			}
			if !re.MatchString(message) {
				continue
			}
			result.Filtered = true
			return result, nil
		}
	}

	if len(extractCfg.Plans) == 0 {
		return result, nil
	}

	sort.SliceStable(extractCfg.Plans, func(i, j int) bool {
		return extractCfg.Plans[i].SeqNo < extractCfg.Plans[j].SeqNo
	})

	for _, plan := range extractCfg.Plans {
		if len(plan.MatchRegex) > 0 {
			re, err := extractor.GetRegexp(plan.MatchRegex)
			if err != nil {
				logger.Ctx(ctx).Err(err).Msg("invalid match regex")
				continue
			}
			if !re.MatchString(message) {
				continue
			}
		}
		logger.Ctx(ctx).Info().Int32("seq_no", plan.SeqNo).Msg("match plan")
		result.HitPlan = lo.ToPtr(plan.SeqNo)
		for _, field := range plan.Fields {
			originalText := message
			// if field.Rule.Type == extractor.RuleTypeXPath {
			// 	originalText = fmt.Sprintf("<p>%s</p>", originalText)
			// }
			text, err := extractor.Extract(originalText, "html", field.Rule)
			if err != nil {
				logger.Ctx(ctx).Err(err).Msg("failed to extract structure")
			} else {
				if text != nil {
					text = lo.ToPtr(strings.TrimSpace(*text))
				}
				switch field.Key {
				case "title":
					result.Title = text
				case "content":
					result.Content = text
				case "url":
					result.Url = text
				case "published_at":
					if text != nil && len(field.TimeFormat) > 0 {
						layout, tmStr, err := parseTimeFormat(*text, field.TimeFormat)
						if err != nil {
							logger.Ctx(ctx).Error().Err(err).Msg("failed to parse time format")
						} else {
							tm, err := time.ParseInLocation(layout, tmStr, time.Local)
							if err != nil {
								logger.Ctx(ctx).Error().Err(err).Msg("failed to parse published_at")
							} else {
								result.PublishedAt = lo.ToPtr(tm)
							}
						}
					}
				}
			}
		}
		break
	}
	return result, nil
}

func parseTimeFormat(source, template string) (string, string, error) {
	// 正则匹配 {.*}
	re := regexp.MustCompile(`(\{.*?\})`)
	matches := re.FindAllString(template, -1)

	// 去掉所有参数后比较 source 和 template 是否匹配
	rt := template
	for _, match := range matches {
		rt = strings.Replace(rt, match, "", 1)
	}

	if utf8.RuneCountInString(source) != utf8.RuneCountInString(rt) {
		return "", "", fmt.Errorf("source and template mismatch")
	}
	_, err := time.Parse(rt, source)
	if err != nil {
		return "", "", fmt.Errorf("source and template mismatch: %w", err)
	}

	now := time.Now()
	var (
		i = 0
		j = 0

		sourceRunes   = []rune(source)
		templateRunes = []rune(template)

		layoutBuilder strings.Builder
		resultBuilder strings.Builder
	)

	for i < len(templateRunes) {
		if templateRunes[i] == '{' {
			// Find the closing brace
			end := strings.IndexRune(string(templateRunes[i:]), '}')
			if end == -1 {
				// No closing brace, append the rest
				layoutBuilder.WriteString(string(templateRunes[i:]))
				break
			}
			end += i // Convert to absolute index

			// Extract content inside braces
			content := string(templateRunes[i+1 : end])

			if strings.HasPrefix(content, "$") {
				var (
					varName      = content[1:]
					defaultValue *string
				)
				if strings.IndexRune(varName, ':') > 0 {
					parts := strings.SplitN(varName, ":", 2)
					varName = parts[0]
					defaultValue = lo.ToPtr(parts[1])
				}
				// Variable replacement
				switch varName {
				case "YYYY":
					layoutBuilder.WriteString("2006")
					if defaultValue != nil {
						resultBuilder.WriteString(*defaultValue)
					} else {
						resultBuilder.WriteString(fmt.Sprintf("%04d", now.Year()))
					}
				case "MM":
					layoutBuilder.WriteString("01")
					if defaultValue != nil {
						resultBuilder.WriteString(*defaultValue)
					} else {
						resultBuilder.WriteString(fmt.Sprintf("%02d", now.Month()))
					}
				case "DD":
					layoutBuilder.WriteString("02")
					if defaultValue != nil {
						resultBuilder.WriteString(*defaultValue)
					} else {
						resultBuilder.WriteString(fmt.Sprintf("%02d", now.Day()))
					}
				case "hh":
					layoutBuilder.WriteString("15")
					if defaultValue != nil {
						resultBuilder.WriteString(*defaultValue)
					} else {
						resultBuilder.WriteString(fmt.Sprintf("%02d", now.Hour()))
					}
				case "mi":
					layoutBuilder.WriteString("04")
					if defaultValue != nil {
						resultBuilder.WriteString(*defaultValue)
					} else {
						resultBuilder.WriteString(fmt.Sprintf("%02d", now.Minute()))
					}
				case "ss":
					layoutBuilder.WriteString("05")
					if defaultValue != nil {
						resultBuilder.WriteString(*defaultValue)
					} else {
						resultBuilder.WriteString(fmt.Sprintf("%02d", now.Second()))
					}
				default:
					// Unsupported variable, keep original
					layoutBuilder.WriteString(string(templateRunes[i : end+1]))
					resultBuilder.WriteString(string(templateRunes[i : end+1]))
				}
			} else {
				// Constant replacement
				layoutBuilder.WriteString(content)
				resultBuilder.WriteString(content)
			}

			i = end + 1 // Skip past the closing brace
		} else {
			// Regular character
			layoutBuilder.WriteRune(templateRunes[i])
			resultBuilder.WriteRune(sourceRunes[j])
			i++
			j++
		}
	}

	return layoutBuilder.String(), resultBuilder.String(), nil
}
