package extractor

// RuleType 提取规则类型
type RuleType string

const (
	RuleTypeRegex RuleType = "regex" // 正则表达式
	RuleTypeXPath RuleType = "xpath" // XPath（用于HTML/XML）
)

// Rule 单个字段的提取规则
type Rule struct {
	Type    RuleType `json:"type"`    // regex 或 xpath
	Pattern string   `json:"pattern"` // 匹配模式
	Group   int      `json:"group"`   // 捕获组索引（仅用于regex，默认1）
}
