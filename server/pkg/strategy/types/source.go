package types

import (
	"context"
	"time"

	types "github.com/wangliang139/llt-trade/server/pkg/types"
)

type Cursor struct {
	ID int64 // tie-breaker（如自增 id）
	Ts time.Time
}

// Source 统一表达外部事件源（datasource/market/timer 等）。
//
// 语义约束（重要）：
// - 同一 source 必须保证自身输出按 (Ts, SourceSeq) 单调不降；否则合并正确性不成立。
// - exhausted: ok=false, err=nil
// - failed: err!=nil
type Source interface {
	ID() string

	Spec() SignalSpec
	IsDerived() bool
	Datasource() *types.DataSource

	// Peek 返回当前 head（不推进）。
	Peek(ctx context.Context) (ev *Message, ok bool, err error)

	// Next 消费当前 head 并推进到下一个。
	Next(ctx context.Context) (ev *Message, ok bool, err error)

	// Watermark 表示该 source 当前可保证“不再返回早于 ts 的事件”的界限。
	// 第一版回测可不使用（ok=false）。
	Watermark(ctx context.Context) (ts time.Time, ok bool, err error)

	Close() error
}

type Fetcher interface {
	ID() string
	Spec() SignalSpec
	Fetch(ctx context.Context, cursor Cursor, limit int) ([]*Message, Cursor, error)
}

type SourceKind string

const (
	SourceKindDb    SourceKind = "db"
	SourceKindTimer SourceKind = "timer"
)

type SourceConfig interface {
	GetKind() SourceKind
}

// TimerSourceConfig 描述回测 timer 事件源配置。
type TimerSourceConfig struct {
	// SignalID 可选：用于覆盖 source 输出的 SourceID/SID，使其与策略 SignalDefinition.id 对齐。
	SignalID string
	Interval time.Duration
	// StartTime/EndTime 可选：若为 nil 则使用 RunBacktestInput 的全局时间范围
	StartTime *time.Time
	EndTime   *time.Time
	Topic     string
}

func (c *TimerSourceConfig) GetKind() SourceKind {
	return SourceKindTimer
}

type DbSourceConfig struct {
	Datasource *types.DataSource
	// SignalID 可选：用于覆盖 source 输出的 SourceID/SID，使其与策略 SignalDefinition.id 对齐。
	SignalID string
}

func (c *DbSourceConfig) GetKind() SourceKind {
	return SourceKindDb
}
