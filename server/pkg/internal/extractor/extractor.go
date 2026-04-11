package extractor

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/antchfx/htmlquery"
	"github.com/antchfx/xmlquery"
	"github.com/samber/lo"
	"golang.org/x/net/html"
)

var (
	regexpCache = make(map[string]*regexp.Regexp)
	_mu         = sync.RWMutex{}
)

func GetRegexp(pattern string) (*regexp.Regexp, error) {
	// 第一次检查（读锁保护）
	_mu.RLock()
	re, exists := regexpCache[pattern]
	_mu.RUnlock()
	if exists {
		return re, nil
	}

	// 获取写锁
	_mu.Lock()
	defer _mu.Unlock()

	// 第二次检查（避免重复创建）
	if re, ok := regexpCache[pattern]; ok {
		return re, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}
	regexpCache[pattern] = re
	return re, nil
}

// Extract 从消息中提取字段
// message: 原始消息内容（可能是纯文本或HTML）
// format: 消息格式（"text", "html", "xml"）
func Extract(message string, format string, rule Rule) (*string, error) {
	return extract(message, format, &rule)
}

func extract(message string, format string, rule *Rule) (*string, error) {
	switch rule.Type {
	case RuleTypeRegex:
		return extractByRegex(message, rule)
	case RuleTypeXPath:
		return extractByXPath(message, format, rule)
	default:
		return nil, fmt.Errorf("unsupported extract rule type: %s", rule.Type)
	}
}

// extractByRegex 使用正则表达式提取
func extractByRegex(message string, rule *Rule) (*string, error) {
	re, err := GetRegexp(rule.Pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	matches := re.FindStringSubmatch(message)
	if len(matches) == 0 {
		return nil, nil
	}

	group := rule.Group
	if group <= 0 {
		group = 1 // 默认使用第一个捕获组
	}

	if group >= len(matches) {
		return nil, nil
	}

	return lo.ToPtr(matches[group]), nil
}

// extractByXPath 使用XPath提取
func extractByXPath(message string, format string, rule *Rule) (*string, error) {
	var doc any
	var err error

	switch format {
	case "html":
		doc, err = htmlquery.Parse(strings.NewReader(message))
	case "xml":
		doc, err = xmlquery.Parse(strings.NewReader(message))
	default:
		return nil, fmt.Errorf("xpath requires html or xml format, got: %s", format)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse document: %w", err)
	}

	var result *string
	switch format {
	case "html":
		nodes := htmlquery.Find(doc.(*html.Node), rule.Pattern)
		if len(nodes) == 1 {
			node := nodes[0]
			result = lo.ToPtr(htmlquery.InnerText(node))
		} else if len(nodes) > 1 {
			results := lo.Map(nodes, func(node *html.Node, _ int) string {
				return htmlquery.InnerText(node)
			})
			result = lo.ToPtr(strings.Join(results, ""))
		}
	case "xml":
		nodes := xmlquery.Find(doc.(*xmlquery.Node), rule.Pattern)
		if len(nodes) == 1 {
			node := nodes[0]
			result = lo.ToPtr(node.Data)
		} else if len(nodes) > 1 {
			results := lo.Map(nodes, func(node *xmlquery.Node, _ int) string {
				return node.Data
			})
			result = lo.ToPtr(strings.Join(results, ""))
		}
	}

	return result, nil
}
