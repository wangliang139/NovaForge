package account

import (
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

type PositionKey struct {
	ExSymbol ctypes.ExSymbol
	Side     ctypes.PositionSide
}

// String 返回PositionKey的字符串表示（用于调试）
func (k PositionKey) String() string {
	return k.ExSymbol.String() + ":" + string(k.Side)
}
