package types

import (
	"context"
)

// ExternalMerger 将多路 ExternalSource 做 k-way merge，输出按时间点分帧的外部事件集合。
type ExternalMerger interface {
	PeekFrame(ctx context.Context) (frame *Frame, ok bool, err error)
	NextFrame(ctx context.Context) (frame *Frame, ok bool, err error)
	Close() error
}
