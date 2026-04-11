package types

import ctypes "github.com/wangliang139/NovaForge/server/pkg/types"

type PriceRouteItem struct {
	Symbol  ctypes.Symbol
	Reverse bool
}
