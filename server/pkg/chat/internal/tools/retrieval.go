package tools

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/wangliang139/llt-trade/server/pkg/chat/domain"
	"github.com/wangliang139/llt-trade/server/pkg/internal/consts"
	"github.com/wangliang139/llt-trade/server/pkg/internal/zai"
	"github.com/wangliang139/llt-trade/server/pkg/repos/document"
	"github.com/wangliang139/mow/logger"
)

const (
	DefaultEmbeddingThreshold = 0.7
)

type rrfEntry struct {
	ID    int64
	Score float64
}

type retrievalCandidate struct {
	ID          int64
	Title       string
	Summary     string
	Content     string
	URL         string
	Tags        []string
	Coins       []string
	PublishedAt string

	SemanticScore float64
	BM25Score     float64
	RRFScore      float64
}

func SearchDocuments(ctx context.Context, args map[string]any, env domain.Env) (any, error) {
	if env.DB == nil {
		return nil, domain.NewRuntimeError("dependency_missing", "document retrieval dependencies are not configured")
	}

	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return nil, domain.NewRuntimeError("invalid_argument", "query must be non-empty string")
	}
	query = strings.TrimSpace(query)

	publishedWithinDays := int64(30)
	if v, exists := args["published_within_days"]; exists {
		n, err := asInt64(v)
		if err != nil {
			return nil, domain.NewRuntimeError("invalid_argument", "published_within_days must be integer")
		}
		publishedWithinDays = n
	}
	if publishedWithinDays <= 0 {
		publishedWithinDays = 30
	}
	if publishedWithinDays > 365 {
		publishedWithinDays = 365
	}

	topK := int64(5)
	if v, exists := args["top_k"]; exists {
		n, err := asInt64(v)
		if err != nil {
			return nil, domain.NewRuntimeError("invalid_argument", "top_k must be integer")
		}
		topK = n
	}
	if topK <= 0 {
		topK = 5
	}
	if topK > 20 {
		topK = 20
	}

	lambda := 0.7
	if v, exists := args["lambda"]; exists {
		n, err := asFloat64(v)
		if err != nil {
			return nil, domain.NewRuntimeError("invalid_argument", "lambda must be number")
		}
		lambda = n
	}
	lambda = clamp(lambda, 0, 1)

	now := time.Now()
	publishedAtStart := now.Add(-time.Duration(publishedWithinDays) * 24 * time.Hour)
	candidateTopK := int32(maxInt64(topK*4, topK))
	if candidateTopK < 20 {
		candidateTopK = 20
	}
	if candidateTopK > 80 {
		candidateTopK = 80
	}

	baseCtx := context.WithoutCancel(ctx)

	// keywords：仅用于 BM25 检索，语义检索始终使用原始 query。
	keywords, _ := args["keywords"].(string)
	keywords = strings.TrimSpace(keywords)
	var bm25Query *string
	if keywords != "" {
		bm25Query = &keywords
	}

	wg := sync.WaitGroup{}
	wg.Add(2)

	var semanticRows []document.SemanticSearchRow
	var queryRows []document.QueryDocumentsRow

	go func() {
		defer wg.Done()
		response, err := env.ZaiEngine.Caller().
			WithPlatform(zai.PlatformTypeOpenRouter).
			CreateEmbeddings(baseCtx, openai.EmbeddingNewParams{
				Model:          openai.EmbeddingModel("qwen/qwen3-embedding-8b"),
				Input:          openai.EmbeddingNewParamsInputUnion{OfString: openai.String(query)},
				Dimensions:     openai.Int(consts.EmbedDimensions),
				EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
			})
		if err != nil {
			logger.Ctx(ctx).Err(err).Msg("failed to create embeddings")
			return
		}
		if len(response.Data) == 0 || len(response.Data[0].Embedding) == 0 {
			logger.Ctx(ctx).Err(domain.NewRuntimeError("empty_response", "empty response from zai engine")).Msg("failed to create embeddings")
			return
		}

		embedding := make([]float32, len(response.Data[0].Embedding))
		for i, v := range response.Data[0].Embedding {
			embedding[i] = float32(v)
		}

		semanticRows, err = env.DB.DocumentRepo.SemanticSearch(baseCtx, document.SemanticSearchParams{
			Embedding:        embedding,
			Threshold:        DefaultEmbeddingThreshold,
			PublishedAtStart: publishedAtStart,
			PublishedAtEnd:   now,
			Status:           document.DocumentStatusActive,
			TopK:             candidateTopK,
		})
		if err != nil {
			logger.Ctx(ctx).Err(err).Msg("failed to semantic search")
		}
	}()

	go func() {
		defer wg.Done()
		var err error
		queryRows, err = env.DB.DocumentRepo.QueryDocuments(baseCtx, document.QueryDocumentsParams{
			Keyword:          bm25Query,
			PublishedAtStart: &publishedAtStart,
			PublishedAtEnd:   &now,
			Status: document.NullDocumentStatus{
				DocumentStatus: document.DocumentStatusActive,
				Valid:          true,
			},
			Offset: 0,
			Limit:  candidateTopK,
		})
		if err != nil {
			logger.Ctx(ctx).Err(err).Msg("failed to query documents")
		}
	}()

	wg.Wait()

	semanticRanks := make([]rrfEntry, 0, len(semanticRows))
	bm25Ranks := make([]rrfEntry, 0, len(queryRows))
	candidateMap := make(map[int64]retrievalCandidate, len(semanticRows)+len(queryRows))

	for _, row := range semanticRows {
		semanticRanks = append(semanticRanks, rrfEntry{ID: row.ID, Score: float64(row.Similarity)})
		candidateMap[row.ID] = retrievalCandidate{
			ID:            row.ID,
			Title:         row.Title,
			Summary:       firstNonEmpty(row.AiSummary, row.AiTitle),
			Content:       row.Content,
			URL:           row.Url,
			Tags:          row.AiTags,
			Coins:         row.AiCoins,
			SemanticScore: float64(row.Similarity),
			PublishedAt:   row.PublishedAt.Format(time.RFC3339),
		}
	}

	for _, row := range queryRows {
		score := asBM25Score(row.Score)
		bm25Ranks = append(bm25Ranks, rrfEntry{ID: row.ID, Score: score})
		existing := candidateMap[row.ID]
		if existing.ID == 0 {
			existing = retrievalCandidate{
				ID:          row.ID,
				Title:       row.Title,
				Summary:     firstNonEmpty(row.AiSummary, row.AiTitle),
				Content:     row.Content,
				URL:         row.Url,
				Tags:        row.AiTags,
				Coins:       row.AiCoins,
				PublishedAt: row.PublishedAt.Format(time.RFC3339),
			}
		}
		existing.BM25Score = score
		candidateMap[row.ID] = existing
	}

	fused := rrfFuse(semanticRanks, bm25Ranks)
	if len(fused) == 0 {
		return map[string]any{
			"skill": map[string]any{
				"callName": "skill.search_documents",
				"name":     "检索资讯文档",
			},
			"query": query,
			"items": []map[string]any{},
		}, nil
	}

	maxRRF := fused[0].Score
	candidates := make([]retrievalCandidate, 0, len(fused))
	for _, item := range fused {
		c, ok := candidateMap[item.ID]
		if !ok {
			continue
		}
		if maxRRF > 0 {
			c.RRFScore = item.Score / maxRRF
		} else {
			c.RRFScore = item.Score
		}
		candidates = append(candidates, c)
	}

	finalItems := mmrRerank(candidates, lambda, int(topK))
	respItems := make([]map[string]any, 0, len(finalItems))
	for i, item := range finalItems {
		respItems = append(respItems, map[string]any{
			"rank":           i + 1,
			"id":             item.ID,
			"title":          item.Title,
			"summary":        firstNonEmpty(item.Summary, trimmed(item.Content, 180)),
			"url":            item.URL,
			"semantic_score": roundFloat(item.SemanticScore, 6),
			"bm25_score":     roundFloat(item.BM25Score, 6),
			"rrf_score":      roundFloat(item.RRFScore, 6),
			"tags":           item.Tags,
			"coins":          item.Coins,
			"published_at":   item.PublishedAt,
		})
	}

	return map[string]any{
		"skill": map[string]any{
			"callName": "skill.search_documents",
			"name":     "检索资讯文档",
		},
		"query": query,
		"params": map[string]any{
			"publishedWithinDays": publishedWithinDays,
			"topK":                topK,
			"lambda":              lambda,
		},
		"candidates": map[string]any{
			"semantic": len(semanticRows),
			"bm25":     len(queryRows),
			"fused":    len(candidates),
		},
		"items": respItems,
	}, nil
}

func rrfFuse(semanticRanks, bm25Ranks []rrfEntry) []rrfEntry {
	const rrfK = 60.0
	if len(semanticRanks) == 0 && len(bm25Ranks) == 0 {
		return nil
	}

	scoreMap := make(map[int64]float64, len(semanticRanks)+len(bm25Ranks))
	for i, item := range semanticRanks {
		scoreMap[item.ID] += 1.0 / (rrfK + float64(i+1))
	}
	for i, item := range bm25Ranks {
		scoreMap[item.ID] += 1.0 / (rrfK + float64(i+1))
	}

	out := make([]rrfEntry, 0, len(scoreMap))
	for id, score := range scoreMap {
		out = append(out, rrfEntry{ID: id, Score: score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	return out
}

func mmrRerank(candidates []retrievalCandidate, lambda float64, topK int) []retrievalCandidate {
	if len(candidates) == 0 || topK <= 0 {
		return nil
	}
	if topK > len(candidates) {
		topK = len(candidates)
	}

	lambda = clamp(lambda, 0, 1)
	remaining := make([]retrievalCandidate, len(candidates))
	copy(remaining, candidates)
	selected := make([]retrievalCandidate, 0, topK)

	for len(selected) < topK && len(remaining) > 0 {
		bestIdx := 0
		bestScore := math.Inf(-1)
		for i, cand := range remaining {
			penalty := 0.0
			for _, picked := range selected {
				sim := jaccardSimilarity(mergedFeatures(cand.Tags, cand.Coins), mergedFeatures(picked.Tags, picked.Coins))
				if sim > penalty {
					penalty = sim
				}
			}
			score := lambda*cand.RRFScore - (1-lambda)*penalty
			if score > bestScore {
				bestScore = score
				bestIdx = i
			}
		}
		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}
	return selected
}

func jaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	setA := make(map[string]struct{}, len(a))
	for _, v := range a {
		n := strings.TrimSpace(strings.ToLower(v))
		if n != "" {
			setA[n] = struct{}{}
		}
	}
	if len(setA) == 0 {
		return 0
	}

	setB := make(map[string]struct{}, len(b))
	for _, v := range b {
		n := strings.TrimSpace(strings.ToLower(v))
		if n != "" {
			setB[n] = struct{}{}
		}
	}
	if len(setB) == 0 {
		return 0
	}

	intersection := 0
	for k := range setA {
		if _, ok := setB[k]; ok {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union <= 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func mergedFeatures(tags, coins []string) []string {
	out := make([]string, 0, len(tags)+len(coins))
	out = append(out, tags...)
	out = append(out, coins...)
	return out
}

func clamp(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
