package types

import (
	"time"

	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

// Frame 表示某个时间点 Ts 的外部事件集合（同 Ts 内已按 TotalOrder 排好）。
type Frame struct {
	Ts       time.Time
	Messages []*stypes.Message
}
