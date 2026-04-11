package sorter

import (
	"strings"
	"time"

	"github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

// SorterConfig 定义严格全序（Total Order）的可配置语义。
//
// Compare(a,b) 的排序维度（从高到低）：
// 1) Ts 升序
// 2) SignalTypePriority 升序（数值越小优先级越高）
// 3) ScopePriority 升序（可选，默认 0）
// 4) SourceID 字典序（仅稳定 tie-break，不承载语义）
// 5) SourceSeq 升序
type SorterConfig struct {
	SignalTypePriority map[types.SignalType]int
	ScopePriority      func(scope *types.SignalScope) int
}

func DefaultSorterConfig() SorterConfig {
	return SorterConfig{
		SignalTypePriority: map[types.SignalType]int{
			types.SignalTypeKline:     10,
			types.SignalTypeTrade:     20,
			types.SignalTypeDepth:     30,
			types.SignalTypeTicker:    40,
			types.SignalTypeMarkPrice: 45,
			types.SignalTypeSocial:    50,
			types.SignalTypeTimer:     60,
			types.SignalTypeFill:      65,
			types.SignalTypeOrder:     70,
			types.SignalTypePosition:  80,
			types.SignalTypeBalance:   90,
			types.SignalTypeRisk:      100,
			types.SignalTypeSystem:    110,
		},
		ScopePriority: func(scope *types.SignalScope) int {
			_ = scope
			return 0
		},
	}
}

func (c SorterConfig) typePriority(t types.SignalType) int {
	if c.SignalTypePriority == nil {
		return 0
	}
	if p, ok := c.SignalTypePriority[t]; ok {
		return p
	}
	// 未配置的类型排在最后，避免“意外抢跑”造成 lookahead。
	return 1_000_000
}

func (c SorterConfig) scopePriority(scope *types.SignalScope) int {
	if c.ScopePriority == nil {
		return 0
	}
	return c.ScopePriority(scope)
}

// Compare 返回：
// -1: a < b
//
//	0: a == b（在 TotalOrder 维度上完全相同）
//
// +1: a > b
func (c SorterConfig) Compare(a, b *types.Message) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	if !a.Ts.Equal(b.Ts) {
		if a.Ts.Before(b.Ts) {
			return -1
		}
		return 1
	}

	ap := c.typePriority(a.Type())
	bp := c.typePriority(b.Type())
	if ap != bp {
		if ap < bp {
			return -1
		}
		return 1
	}

	as := c.scopePriority(a.Scope())
	bs := c.scopePriority(b.Scope())
	if as != bs {
		if as < bs {
			return -1
		}
		return 1
	}

	aid := strings.TrimSpace(a.SourceID)
	bid := strings.TrimSpace(b.SourceID)
	if aid != bid {
		if aid < bid {
			return -1
		}
		return 1
	}

	if a.SourceSeq != b.SourceSeq {
		if a.SourceSeq < b.SourceSeq {
			return -1
		}
		return 1
	}

	// 兜底：不引入额外非语义字段，返回相等。
	return 0
}

// SameTs 仅用于 frame 聚合判断（不引入额外排序语义）。
func SameTs(a, b time.Time) bool {
	return a.Equal(b)
}
