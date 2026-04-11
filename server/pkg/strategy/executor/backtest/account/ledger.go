package account

import (
	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

type AssetLedger struct {
	Asset     string          // USDT / USDC / BTC
	Precision int             // 资产数量精度
	Available decimal.Decimal // 可用余额
	Locked    decimal.Decimal // 冻结余额
	// 总余额 = 可用余额 + 冻结余额

	UnrealizedPnL decimal.Decimal // 未实现盈亏
	RealizedPnL   decimal.Decimal // 已实现盈亏

	MarginUsed  decimal.Decimal // 已占用保证金（由持仓计算）

	// 合约持仓（按symbol-side组织）
	Positions map[PositionKey]*ctypes.Position

	// 现货 WAC 成本跟踪（按 exSymbol 组织）
	SpotCostBasis map[ctypes.ExSymbolKey]*SpotCostBasis
}

// SpotCostBasis 现货 WAC 成本跟踪
// 维度：accountID + exSymbol（在 AssetLedger 内按 exSymbol.Key() 索引）
type SpotCostBasis struct {
	ExSymbol       ctypes.ExSymbol
	Qty            decimal.Decimal // 持仓数量（以 base 计）
	TotalCostQuote decimal.Decimal // 总成本（以 quote 计价）
	AvgCostQuote   decimal.Decimal // 平均成本价（quote/base）
}

func (a *AssetLedger) Balance() decimal.Decimal {
	return a.Available.Add(a.Locked)
}

// NewAssetLedger 创建新的AssetLedger
func NewAssetLedger(asset string) *AssetLedger {
	return NewAssetLedgerWithPrecision(asset, defaultAssetPrecision)
}

// NewAssetLedgerWithPrecision 创建新的AssetLedger并指定精度
func NewAssetLedgerWithPrecision(asset string, precision int) *AssetLedger {
	if precision <= 0 {
		precision = defaultAssetPrecision
	}
	return &AssetLedger{
		Asset:         asset,
		Precision:     precision,
		Available:     decimal.Zero,
		Locked:        decimal.Zero,
		UnrealizedPnL: decimal.Zero,
		RealizedPnL:   decimal.Zero,
		MarginUsed:    decimal.Zero,
		Positions:     make(map[PositionKey]*ctypes.Position),
		SpotCostBasis: make(map[ctypes.ExSymbolKey]*SpotCostBasis),
	}
}

// GetSpotCostBasis 获取现货成本跟踪（如果不存在，返回零值）
func (a *AssetLedger) GetSpotCostBasis(exSymbol ctypes.ExSymbol) *SpotCostBasis {
	key := exSymbol.Key()
	if cb, ok := a.SpotCostBasis[key]; ok {
		return cb
	}
	return &SpotCostBasis{
		ExSymbol:       exSymbol,
		Qty:            decimal.Zero,
		TotalCostQuote: decimal.Zero,
		AvgCostQuote:   decimal.Zero,
	}
}

// UpdateSpotCostBasis 更新现货成本跟踪
// 买入：增加 qty 和 totalCostQuote
// 卖出：减少 qty 和 totalCostQuote，返回已实现盈亏（以 quote 计价）
func (a *AssetLedger) UpdateSpotCostBasis(exSymbol ctypes.ExSymbol, isBuy bool, qty, price decimal.Decimal) decimal.Decimal {
	key := exSymbol.Key()
	cb := a.SpotCostBasis[key]
	if cb == nil {
		cb = &SpotCostBasis{
			ExSymbol:       exSymbol,
			Qty:            decimal.Zero,
			TotalCostQuote: decimal.Zero,
			AvgCostQuote:   decimal.Zero,
		}
		a.SpotCostBasis[key] = cb
	}

	realizedPnlQuote := decimal.Zero

	if isBuy {
		// 买入：增加持仓和成本
		cb.TotalCostQuote = cb.TotalCostQuote.Add(qty.Mul(price))
		cb.Qty = cb.Qty.Add(qty)
		if !cb.Qty.IsZero() {
			cb.AvgCostQuote = cb.TotalCostQuote.Div(cb.Qty)
		}
	} else {
		// 卖出：计算已实现盈亏并减少持仓
		if !cb.AvgCostQuote.IsZero() {
			realizedPnlQuote = qty.Mul(price.Sub(cb.AvgCostQuote))
		}
		// 按比例减少总成本
		if !cb.Qty.IsZero() {
			costReduction := cb.TotalCostQuote.Mul(qty).Div(cb.Qty)
			cb.TotalCostQuote = cb.TotalCostQuote.Sub(costReduction)
		}
		cb.Qty = cb.Qty.Sub(qty)
		// 更新平均成本价
		if cb.Qty.IsZero() || cb.Qty.IsNegative() {
			cb.Qty = decimal.Zero
			cb.TotalCostQuote = decimal.Zero
			cb.AvgCostQuote = decimal.Zero
		} else if !cb.Qty.IsZero() {
			cb.AvgCostQuote = cb.TotalCostQuote.Div(cb.Qty)
		}
	}

	// 清理零持仓
	if cb.Qty.IsZero() {
		delete(a.SpotCostBasis, key)
	}

	return realizedPnlQuote
}

// updateAvailable 更新可用余额（应在修改Balance或Locked后调用）
func (a *AssetLedger) updateAvailable() {
	if a.Available.LessThan(decimal.Zero) {
		a.Available = decimal.Zero
	}
	if a.Locked.LessThan(decimal.Zero) {
		a.Locked = decimal.Zero
	}
	a.Available = formatAmountWithPrecision(a.Available, a.Precision)
	a.Locked = formatAmountWithPrecision(a.Locked, a.Precision)
}
