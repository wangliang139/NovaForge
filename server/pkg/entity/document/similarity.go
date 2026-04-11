package document

import (
	"context"
	"fmt"
	"math"
	"sync"
)

// CompareDocuments 返回两篇文档语义向量的余弦相似度
func (e *Entity) CompareDocuments(ctx context.Context, leftID, rightID int64) (float32, error) {
	if leftID <= 0 || rightID <= 0 {
		return 0, fmt.Errorf("invalid document id")
	}
	if leftID == rightID {
		return 1, nil
	}

	var (
		leftEmbedding  []float32
		rightEmbedding []float32
		leftErr        error
		rightErr       error
	)

	// 使用并发获取 embedding
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		leftEmbedding, leftErr = e.getOrCreateEmbedding(ctx, leftID)
	}()
	go func() {
		defer wg.Done()
		rightEmbedding, rightErr = e.getOrCreateEmbedding(ctx, rightID)
	}()

	wg.Wait()
	
	if leftErr != nil {
		return 0, fmt.Errorf("failed to get left embedding: %w", leftErr)
	}
	if rightErr != nil {
		return 0, fmt.Errorf("failed to get right embedding: %w", rightErr)
	}

	if len(leftEmbedding) == 0 {
		return 0, fmt.Errorf("left embedding is empty")
	}
	if len(rightEmbedding) == 0 {
		return 0, fmt.Errorf("right embedding is empty")
	}

	if len(leftEmbedding) != len(rightEmbedding) {
		return 0, fmt.Errorf("embedding dimension mismatch: %d vs %d", len(leftEmbedding), len(rightEmbedding))
	}

	var dot, leftNorm, rightNorm float64
	for i := range leftEmbedding {
		l := float64(leftEmbedding[i])
		r := float64(rightEmbedding[i])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}

	leftMag := math.Sqrt(leftNorm)
	rightMag := math.Sqrt(rightNorm)
	if leftMag == 0 || rightMag == 0 {
		return 0, fmt.Errorf("embedding norm is zero")
	}

	sim := dot / (leftMag * rightMag)
	return float32((sim + 1) / 2), nil
}
