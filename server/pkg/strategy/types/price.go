package types

import ctypes "github.com/wangliang139/llt-trade/server/pkg/types"

type PriceRouteItem struct {
	Symbol  ctypes.Symbol
	Reverse bool
}
