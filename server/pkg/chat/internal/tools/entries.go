package tools

// entries.go 是所有工具（Tool）和技能（Skill）的单一来源：
// schema（名称、描述、参数定义）与 handler（执行逻辑）定义在同一处。
// 新增/修改能力只需编辑本文件，无需同步修改其他文件。

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
	"github.com/wangliang139/llt-trade/server/pkg/chat/internal/capability"
)

const (
	ToolNowISO8601     = "now_iso8601"
	ToolEchoJSON       = "echo_json"
	ToolGetSkillDetail = "get_skill_detail"
)

// init 在包加载时以空依赖预注册 schema，确保 capability 注册表在首次使用前即可用
// （例如：context/build.go 在 NewRuntime 之前调用 ListToolsFull 时不会得到空列表）。
func init() {
	t, s := BuildEntries()
	capability.SetRegistered(t, s)
}

// buildEntries 返回完整的工具和技能定义（含运行时 handler）。
// NewRuntime 会以真实依赖再次调用此函数并重新注册，以确保 handler 拥有真实的外部依赖。
func BuildEntries() ([]capability.ToolDef, []capability.SkillDef) {
	tools := []capability.ToolDef{
		{
			Name:        "now_iso8601",
			Description: "Get current server time in UTC ISO8601 format.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			OutputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"now": map[string]any{"type": "string"},
				},
			},
			Handler: func(_ context.Context, _ map[string]any, _ domain.Env) (any, error) {
				return map[string]any{
					"now": time.Now().UTC().Format(time.RFC3339Nano),
				}, nil
			},
		},
		{
			Name:        "echo_json",
			Description: "Echo input payload for structured verification.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"payload": map[string]any{"type": "object"},
				},
				"required": []string{"payload"},
			},
			Handler: func(_ context.Context, args map[string]any, _ domain.Env) (any, error) {
				payload, ok := args["payload"]
				if !ok {
					return nil, domain.NewRuntimeError("invalid_argument", "missing required field: payload")
				}
				return map[string]any{"payload": payload}, nil
			},
		},
		{
			Name:        "get_skill_detail",
			Description: "Get full detail of a skill by name for progressive disclosure.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"skill_name": map[string]any{"type": "string"},
				},
				"required": []string{"skill_name"},
			},
			Handler: func(_ context.Context, args map[string]any, _ domain.Env) (any, error) {
				rawName, ok := args["skill_name"]
				if !ok {
					return nil, domain.NewRuntimeError("invalid_argument", "missing required field: skill_name")
				}
				name, ok := rawName.(string)
				if !ok || name == "" {
					return nil, domain.NewRuntimeError("invalid_argument", "skill_name must be non-empty string")
				}
				skill, found := capability.GetSkillDetail(name)
				if !found {
					return map[string]any{
						"code":    "skill_not_found",
						"message": fmt.Sprintf("skill not found: %s", name),
					}, nil
				}
				return map[string]any{
					"name":        skill.Name,
					"description": skill.Description,
					"detail":      skill.Detail,
				}, nil
			},
		},
	}

	skills := []capability.SkillDef{
		{
			Name:        "skill.get_strategy_development_manual",
			Description: "当用户需要获取策略开发手册、关键约束与示例入口时使用。",
			Detail: map[string]any{
				"display_name": "获取策略开发手册",
				"usage":        "该技能提供了访问策略开发手册的能力。无需入参，返回值为策略开发手册的 markdown 格式内容。",
			},
			Handler: func(ctx context.Context, _ map[string]any, env domain.Env) (any, error) {
				if env.DB == nil {
					return nil, domain.NewRuntimeError("dependency_missing", "kv repo is not configured")
				}
				row, err := env.DB.KvRepo.GetByKey(ctx, "doc.strategy.guide")
				if err != nil {
					return nil, err
				}
				if row == nil || strings.TrimSpace(row.Value) == "" {
					return map[string]any{
						"code":    "doc_not_found",
						"message": "kv key doc.strategy.guide not found or empty",
						"skill": map[string]any{
							"callName": "skill.get_strategy_development_manual",
							"name":     "获取策略开发手册",
						},
					}, nil
				}
				return map[string]any{
					"skill": map[string]any{
						"callName": "skill.get_strategy_development_manual",
						"name":     "获取策略开发手册",
					},
					"content": row.Value,
				}, nil
			},
		},
		{
			Name:        "skill.generate_strategy",
			Description: "当用户需要根据策略开发手册与需求生成策略时使用，返回结构化策略信息（名称、代码、参数、信号、描述）。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "用户的策略需求描述。",
					},
					"current_strategy": map[string]any{
						"type":        "object",
						"description": "可选：当前已有策略（用于多轮调整）。",
					},
				},
				"required": []string{"query"},
			},
			Detail: map[string]any{
				"display_name": "生成策略",
				"usage":        "输入 query；可选传入 current_strategy 进行增量调整。输出固定包含 name/code/params/signals/description。",
			},
			Handler: GenerateStrategy,
		},
		{
			Name:        "skill.web_search",
			Description: "当用户需要进行 Web 搜索（资讯、新闻、通用信息）时使用，基于 Tavily Search API。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "搜索查询语句，建议控制在 400 字符以内，可为自然语言问题或关键词。",
					},
					"search_depth": map[string]any{
						"type":        "string",
						"description": "搜索深度：advanced | basic | fast | ultra-fast，默认 basic。",
					},
					"max_results": map[string]any{
						"type":        "integer",
						"description": "返回结果数量，1-20，默认 5。",
						"default":     5,
					},
					"topic": map[string]any{
						"type":        "string",
						"description": "搜索主题：general | news | finance，默认 general。",
					},
					"time_range": map[string]any{
						"type":        "string",
						"description": "相对时间范围：day/week/month/year 或 d/w/m/y。",
					},
					"start_date": map[string]any{
						"type":        "string",
						"description": "起始日期 YYYY-MM-DD。",
					},
					"end_date": map[string]any{
						"type":        "string",
						"description": "结束日期 YYYY-MM-DD。",
					},
					"include_answer": map[string]any{
						"type":        "boolean",
						"description": "是否让 Tavily 直接生成简要回答，默认 false。",
						"default":     false,
					},
					"include_raw_content": map[string]any{
						"type":        "boolean",
						"description": "是否返回页面原文内容（markdown/text），默认 false。",
						"default":     false,
					},
					"include_images": map[string]any{
						"type":        "boolean",
						"description": "是否返回相关图片信息，默认 false。",
						"default":     false,
					},
					"include_favicon": map[string]any{
						"type":        "boolean",
						"description": "是否返回每条结果的 favicon URL，默认 false。",
						"default":     false,
					},
					"include_domains": map[string]any{
						"type":        "array",
						"description": "只允许的域名白名单，例如 [\"linkedin.com/in\"]。",
						"items":       map[string]any{"type": "string"},
					},
					"exclude_domains": map[string]any{
						"type":        "array",
						"description": "需要排除的域名，例如 [\"example.com\"]。",
						"items":       map[string]any{"type": "string"},
					},
					"country": map[string]any{
						"type":        "string",
						"description": "可选：提升某个国家结果权重，例如 united states、china。",
					},
					"auto_parameters": map[string]any{
						"type":        "boolean",
						"description": "是否让 Tavily 自动调整搜索参数（可能增加成本），默认 false。",
						"default":     false,
					},
					"exact_match": map[string]any{
						"type":        "boolean",
						"description": "是否只匹配带引号的精确短语，适合人名/公司名等精确检索。",
						"default":     false,
					},
					"include_usage": map[string]any{
						"type":        "boolean",
						"description": "是否返回本次调用的 credit 使用信息。",
						"default":     false,
					},
					"safe_search": map[string]any{
						"type":        "boolean",
						"description": "是否过滤不安全内容（Enterprise 功能）。",
						"default":     false,
					},
					"chunks_per_source": map[string]any{
						"type":        "integer",
						"description": "当 search_depth=advanced 时，每个来源返回的 chunk 数量（1-3）。",
					},
				},
				"required": []string{"query"},
			},
			Detail: map[string]any{
				"display_name": "Web 搜索（Tavily）",
				"usage":        "当需要实时 Web 搜索时使用该技能。遵循 Tavily 最佳实践：保持 query 简洁（<400 字符），复杂问题可拆分为多个子查询。需要在服务环境变量中配置 TAVILY_API_KEY。",
			},
			Handler: WebSearch,
		},
		{
			Name:        "skill.search_documents",
			Description: "当用户需要资讯检索时使用，融合语义检索与 BM25 召回，并通过 MMR 提升结果多样性。",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "自然语言检索词",
					},
					"keywords": map[string]any{
						"type":        "string",
						"description": "可选的 BM25 专用关键词（不影响语义检索），例如由 LLM 从 query 中提取出的短语/代号；为空时默认回退为 query。",
					},
					"published_within_days": map[string]any{
						"type":        "integer",
						"description": "检索时间窗口（天）",
						"default":     30,
					},
					"top_k": map[string]any{
						"type":        "integer",
						"description": "返回文档条数",
						"default":     5,
					},
					"lambda": map[string]any{
						"type":        "number",
						"description": "MMR 相关性与多样性平衡系数，1 为最相关，0 为最多样",
						"default":     0.7,
					},
				},
				"required": []string{"query"},
			},
			Detail: map[string]any{
				"display_name": "检索资讯文档",
				"usage":        "输入 query 后执行双路召回：语义检索（pgvector）+ BM25（ParadeDB），使用 RRF 融合后再用 MMR 去重，返回更相关且多样的文档。",
			},
			Handler: SearchDocuments,
		},
	}

	return tools, skills
}

func asInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int:
		return int64(n), nil
	case int8:
		return int64(n), nil
	case int16:
		return int64(n), nil
	case int32:
		return int64(n), nil
	case int64:
		return n, nil
	case float32:
		if math.Trunc(float64(n)) != float64(n) {
			return 0, fmt.Errorf("not integer")
		}
		return int64(n), nil
	case float64:
		if math.Trunc(n) != n {
			return 0, fmt.Errorf("not integer")
		}
		return int64(n), nil
	case string:
		return strconv.ParseInt(strings.TrimSpace(n), 10, 64)
	default:
		return 0, fmt.Errorf("unsupported type")
	}
}

func asFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case int:
		return float64(n), nil
	case int8:
		return float64(n), nil
	case int16:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case float32:
		return float64(n), nil
	case float64:
		return n, nil
	case string:
		return strconv.ParseFloat(strings.TrimSpace(n), 64)
	default:
		return 0, fmt.Errorf("unsupported type")
	}
}

func asBM25Score(v any) float64 {
	switch n := v.(type) {
	case nil:
		return 0
	case float32:
		return float64(n)
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func trimmed(s string, n int) string {
	src := strings.TrimSpace(s)
	if src == "" || n <= 0 {
		return ""
	}
	runes := []rune(src)
	if len(runes) <= n {
		return src
	}
	return string(runes[:n]) + "..."
}

func roundFloat(v float64, digits int) float64 {
	if digits < 0 {
		return v
	}
	p := math.Pow10(digits)
	return math.Round(v*p) / p
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
