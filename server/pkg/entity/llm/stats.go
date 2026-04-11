package llm

import (
	"context"

	"github.com/wangliang139/NovaForge/server/pkg/repos/llm_completion"
)

// TaskStatsResult 聚合统计结果
type TaskStatsResult struct {
	TotalCount    int64
	SuccessCount  int64
	FailCount     int64
	SuccessRate   float64
	SceneStats    []SceneStatsResult
}

// SceneStatsResult 按 scene 的统计
type SceneStatsResult struct {
	SceneKey      string
	SceneID       int64
	TotalCount    int64
	SuccessCount  int64
	FailCount     int64
	SuccessRate   float64
	AvgDurationMs float64
}

// GetCompletionStats 获取 LLM 调用统计（指定时间窗口）
func (e *Entity) GetCompletionStats(ctx context.Context, startTs, endTs int64) (*TaskStatsResult, error) {
	row, err := e.db.LlmCompletionRepo.GetCompletionStats(ctx, llm_completion.GetCompletionStatsParams{
		StartTs: startTs,
		EndTs:   endTs,
	})
	if err != nil {
		return nil, err
	}
	if row == nil {
		return &TaskStatsResult{
			TotalCount:  0,
			SuccessCount: 0,
			FailCount:   0,
			SuccessRate: 0,
			SceneStats:  nil,
		}, nil
	}

	successRate := 0.0
	if row.TotalCount > 0 {
		successRate = float64(row.SuccessCount) / float64(row.TotalCount) * 100
	}

	sceneRows, err := e.db.LlmCompletionRepo.GetCompletionStatsByScene(ctx, llm_completion.GetCompletionStatsBySceneParams{
		StartTs: startTs,
		EndTs:   endTs,
	})
	if err != nil {
		return nil, err
	}

	sceneStats := make([]SceneStatsResult, 0, len(sceneRows))
	for _, r := range sceneRows {
		sr := 0.0
		if r.TotalCount > 0 {
			sr = float64(r.SuccessCount) / float64(r.TotalCount) * 100
		}
		sceneStats = append(sceneStats, SceneStatsResult{
			SceneKey:      r.SceneKey,
			SceneID:       r.SceneID,
			TotalCount:    r.TotalCount,
			SuccessCount:  r.SuccessCount,
			FailCount:     r.FailCount,
			SuccessRate:   sr,
			AvgDurationMs: r.AvgDurationMs,
		})
	}

	return &TaskStatsResult{
		TotalCount:   row.TotalCount,
		SuccessCount: row.SuccessCount,
		FailCount:    row.FailCount,
		SuccessRate:  successRate,
		SceneStats:   sceneStats,
	}, nil
}
