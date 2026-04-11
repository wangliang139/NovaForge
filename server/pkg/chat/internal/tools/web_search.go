package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
	"github.com/wangliang139/llt-trade/server/pkg/settings"
	"github.com/wangliang139/mow/logger"
)

const (
	tavilyAPIURL          = "https://api.tavily.com/search"
	tavilyDefaultMaxItems = 5
	tavilyMaxItemsLimit   = 20
	tavilyQueryMaxLen     = 400
)

type tavilyRequest struct {
	Query            string        `json:"query"`
	SearchDepth      string        `json:"search_depth,omitempty"`
	MaxResults       int           `json:"max_results,omitempty"`
	Topic            string        `json:"topic,omitempty"`
	TimeRange        string        `json:"time_range,omitempty"`
	StartDate        string        `json:"start_date,omitempty"`
	EndDate          string        `json:"end_date,omitempty"`
	IncludeAnswer    bool          `json:"include_answer,omitempty"`
	IncludeRaw       bool          `json:"include_raw_content,omitempty"`
	IncludeImages    bool          `json:"include_images,omitempty"`
	IncludeFavIcon   bool          `json:"include_favicon,omitempty"`
	IncludeDomains   []string      `json:"include_domains,omitempty"`
	ExcludeDomains   []string      `json:"exclude_domains,omitempty"`
	Country          string        `json:"country,omitempty"`
	AutoParameters   bool          `json:"auto_parameters,omitempty"`
	ExactMatch       bool          `json:"exact_match,omitempty"`
	IncludeUsage     bool          `json:"include_usage,omitempty"`
	SafeSearch       bool          `json:"safe_search,omitempty"`
	ChunksPerSource  int           `json:"chunks_per_source,omitempty"`
	SearchDepthAlias string        `json:"-"`
	RawExtra         interface{}   `json:"-"`
}

type tavilyResponse struct {
	Query         string         `json:"query"`
	Answer        string         `json:"answer,omitempty"`
	Results       []any          `json:"results"`
	Images        []any          `json:"images,omitempty"`
	AutoParams    map[string]any `json:"auto_parameters,omitempty"`
	ResponseTime  float64        `json:"response_time"`
	Usage         map[string]any `json:"usage,omitempty"`
	RequestID     string         `json:"request_id,omitempty"`
	RawUnderlying map[string]any `json:"-"`
}

// WebSearch 使用 Tavily Search API 进行 Web 搜索。
// 参考文档：
//   - https://docs.tavily.com/documentation/api-reference/endpoint/search
//   - https://docs.tavily.com/documentation/best-practices/best-practices-search
func WebSearch(ctx context.Context, args map[string]any, env domain.Env) (any, error) {
	_ = env // 当前实现不依赖 DB/ZaiEngine，仅依赖 settings 中的 Tavily API Key。

	apiKey, ok := settings.Get(ctx, settings.KeyTavilyAPIKey)
	if !ok || strings.TrimSpace(apiKey) == "" {
		return nil, domain.NewRuntimeError("feature_disabled", "tavily web search is not configured")
	}

	rawQuery, ok := args["query"].(string)
	if !ok || strings.TrimSpace(rawQuery) == "" {
		return nil, domain.NewRuntimeError("invalid_argument", "query must be non-empty string")
	}
	query := strings.TrimSpace(rawQuery)
	if len([]rune(query)) > tavilyQueryMaxLen {
		runes := []rune(query)
		query = string(runes[:tavilyQueryMaxLen])
	}

	req := tavilyRequest{
		Query:       query,
		MaxResults:  tavilyDefaultMaxItems,
		SearchDepth: "basic",
		Topic:       "general",
	}

	if v, ok := args["max_results"]; ok {
		if n, err := asInt64(v); err == nil {
			if n < 1 {
				n = 1
			}
			if n > tavilyMaxItemsLimit {
				n = tavilyMaxItemsLimit
			}
			req.MaxResults = int(n)
		}
	}

	if v, ok := args["search_depth"].(string); ok {
		depth := strings.TrimSpace(v)
		switch depth {
		case "advanced", "basic", "fast", "ultra-fast":
			req.SearchDepth = depth
		}
	}

	if v, ok := args["topic"].(string); ok {
		topic := strings.TrimSpace(v)
		switch topic {
		case "general", "news", "finance":
			req.Topic = topic
		}
	}

	if v, ok := args["time_range"].(string); ok {
		tr := strings.TrimSpace(v)
		switch tr {
		case "day", "week", "month", "year", "d", "w", "m", "y":
			req.TimeRange = tr
		}
	}

	if v, ok := args["start_date"].(string); ok {
		req.StartDate = strings.TrimSpace(v)
	}
	if v, ok := args["end_date"].(string); ok {
		req.EndDate = strings.TrimSpace(v)
	}

	if v, ok := args["include_answer"].(bool); ok {
		req.IncludeAnswer = v
	}
	if v, ok := args["include_raw_content"].(bool); ok {
		req.IncludeRaw = v
	}
	if v, ok := args["include_images"].(bool); ok {
		req.IncludeImages = v
	}
	if v, ok := args["include_favicon"].(bool); ok {
		req.IncludeFavIcon = v
	}

	if v, ok := args["include_domains"]; ok {
		if list, okCast := v.([]any); okCast {
			req.IncludeDomains = extractStringSlice(list)
		}
	}
	if v, ok := args["exclude_domains"]; ok {
		if list, okCast := v.([]any); okCast {
			req.ExcludeDomains = extractStringSlice(list)
		}
	}

	if v, ok := args["country"].(string); ok {
		req.Country = strings.TrimSpace(v)
	}

	if v, ok := args["auto_parameters"].(bool); ok {
		req.AutoParameters = v
	}
	if v, ok := args["exact_match"].(bool); ok {
		req.ExactMatch = v
	}
	if v, ok := args["include_usage"].(bool); ok {
		req.IncludeUsage = v
	}
	if v, ok := args["safe_search"].(bool); ok {
		req.SafeSearch = v
	}

	if v, ok := args["chunks_per_source"]; ok {
		if n, err := asInt64(v); err == nil {
			if n < 1 {
				n = 1
			}
			if n > 3 {
				n = 3
			}
			req.ChunksPerSource = int(n)
		}
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, domain.NewRuntimeError("internal_error", "failed to encode tavily request")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tavilyAPIURL, bytes.NewReader(payload))
	if err != nil {
		return nil, domain.NewRuntimeError("internal_error", "failed to build tavily request")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))

	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	start := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("tavily search request failed")
		return nil, domain.NewRuntimeError("upstream_error", "tavily search request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, domain.NewRuntimeError("unauthorized", "tavily api key unauthorized")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, domain.NewRuntimeError("rate_limited", "tavily rate limit exceeded")
	}
	if resp.StatusCode >= 400 {
		return nil, domain.NewRuntimeError("upstream_error", fmt.Sprintf("tavily returned status %d", resp.StatusCode))
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		logger.Ctx(ctx).Err(err).Msg("failed to decode tavily response")
		return nil, domain.NewRuntimeError("upstream_error", "failed to decode tavily response")
	}

	elapsed := time.Since(start).Seconds()

	return map[string]any{
		"skill": map[string]any{
			"callName": "skill.web_search",
			"name":     "Web 搜索（Tavily）",
		},
		"query":  query,
		"params": buildTavilyParamsSummary(req),
		"stats": map[string]any{
			"elapsedSeconds": elapsed,
		},
		"raw": raw,
	}, nil
}

func extractStringSlice(in []any) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func buildTavilyParamsSummary(req tavilyRequest) map[string]any {
	return map[string]any{
		"searchDepth": req.SearchDepth,
		"maxResults":  req.MaxResults,
		"topic":       req.Topic,
		"timeRange":   req.TimeRange,
		"startDate":   req.StartDate,
		"endDate":     req.EndDate,
	}
}

