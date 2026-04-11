package document

import (
	"context"
	"time"

	"github.com/wangliang139/NovaForge/server/pkg/repos/document"
)

// DocumentStatsResult 文档处理统计
type DocumentStatsResult struct {
	TotalCount              int64
	SuccessCount            int64
	SuccessRate             float64
	AvgPublishToIngestSec   float64
	AvgIngestToSuccessSec   float64
	ChannelDocumentCounts   []ChannelDocumentCount
}

// ChannelDocumentCount 按 source/provider 的文档数
type ChannelDocumentCount struct {
	Source        string
	Provider      string
	DocumentCount int64
	SuccessCount  int64
}

// GetDocumentStats 获取文档处理统计（指定时间窗口，按 created_at）
func (e *Entity) GetDocumentStats(ctx context.Context, startTs, endTs int64) (*DocumentStatsResult, error) {
	createdAtStart := time.Unix(startTs, 0)
	createdAtEnd := time.Unix(endTs, 0)

	row, err := e.db.DocumentRepo.GetDocumentStats(ctx, document.GetDocumentStatsParams{
		CreatedAtStart: createdAtStart,
		CreatedAtEnd:   createdAtEnd,
	})
	if err != nil {
		return nil, err
	}
	if row == nil {
		return &DocumentStatsResult{
			TotalCount:            0,
			SuccessCount:          0,
			SuccessRate:           0,
			AvgPublishToIngestSec: 0,
			AvgIngestToSuccessSec: 0,
			ChannelDocumentCounts: nil,
		}, nil
	}

	successRate := 0.0
	if row.TotalCount > 0 {
		successRate = float64(row.SuccessCount) / float64(row.TotalCount) * 100
	}

	channelRows, err := e.db.DocumentRepo.GetDocumentCountByChannel(ctx, document.GetDocumentCountByChannelParams{
		CreatedAtStart: createdAtStart,
		CreatedAtEnd:   createdAtEnd,
	})
	if err != nil {
		return nil, err
	}
	channelCounts := make([]ChannelDocumentCount, 0, len(channelRows))
	for _, r := range channelRows {
		channelCounts = append(channelCounts, ChannelDocumentCount{
			Source:        r.Source,
			Provider:      r.Provider,
			DocumentCount: r.DocumentCount,
			SuccessCount:  r.SuccessCount,
		})
	}

	return &DocumentStatsResult{
		TotalCount:            row.TotalCount,
		SuccessCount:          row.SuccessCount,
		SuccessRate:            successRate,
		AvgPublishToIngestSec:  row.AvgPublishToIngestSec,
		AvgIngestToSuccessSec:  row.AvgIngestToSuccessSec,
		ChannelDocumentCounts: channelCounts,
	}, nil
}
