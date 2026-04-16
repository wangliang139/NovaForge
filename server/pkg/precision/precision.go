// Package precision 定义交易金额在存储与跨模块边界上的统一精度策略。
//
// 设计要点（与业务约定对齐）：
//   - 内部计算尽量保持 shopspring/decimal 的完整精度，避免 float64 参与金额运算。
//   - 写入 PostgreSQL 的 DECIMAL(32,18) 等字段前，在应用层显式收敛到 StorageScale，避免依赖数据库隐式舍入。
//   - 交易所 tick/step 对齐仍在 misc/entity order normalizer 中 floor；本包负责“存储层最终舍入”。
package precision

import (
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/wangliang139/NovaForge/server/pkg/internal/consts"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
)

// DefaultMarketOrderFreezeFactor 现货市价买单冻结估算系数（历史默认 1.05）。
var DefaultMarketOrderFreezeFactor = decimal.RequireFromString("1.05")

// BacktestDefaultMarketOrderFreezeFactor 回测/撮合侧默认冻结因子（历史默认 1.2）。
var BacktestDefaultMarketOrderFreezeFactor = decimal.RequireFromString("1.2")

// ReservationReleaseDust 预留资金释放时视为“已耗尽”的绝对阈值（兼容历史 0.0001）。
var ReservationReleaseDust = decimal.RequireFromString("0.0001")

// FinalizeForStorage 落库前统一舍入到 StorageScale（半远离零，与 decimal.Round 一致）。
func FinalizeForStorage(d decimal.Decimal) decimal.Decimal {
	if d.IsZero() {
		return d
	}
	return d.Round(consts.DefaultAssetPrecision)
}

// DecimalToPgNumeric 先 FinalizeForStorage 再转为 pg NUMERIC，供资金类字段落库统一使用。
func DecimalToPgNumeric(d decimal.Decimal) pgtype.Numeric {
	return utils.Decimal.DecimalToPgNumeric(FinalizeForStorage(d))
}

// FinalizePtr 对可选指针做存储舍入；nil 保持不变。
func FinalizePtr(d *decimal.Decimal) *decimal.Decimal {
	if d == nil {
		return nil
	}
	v := FinalizeForStorage(*d)
	return &v
}

// FinalizeOrderSnapshotForDB 在写入订单快照（UpsertOrder）前收敛所有金额字段。
func FinalizeOrderSnapshotForDB(o *ctypes.Order) {
	if o == nil {
		return
	}
	o.Price = FinalizeForStorage(o.Price)
	o.OriginalQty = FinalizeForStorage(o.OriginalQty)
	o.ExecutedQty = FinalizeForStorage(o.ExecutedQty)
	o.OriginalQuoteQty = FinalizeForStorage(o.OriginalQuoteQty)
	o.ExecutedQuoteQty = FinalizeForStorage(o.ExecutedQuoteQty)
	o.AvgPrice = FinalizeForStorage(o.AvgPrice)
	o.Locked = FinalizePtr(o.Locked)
	o.Fee = FinalizePtr(o.Fee)
	o.RealizedPnl = FinalizePtr(o.RealizedPnl)
}

// Notional 高精度名义价值 price * qty（不在此处舍入）。
func Notional(price, qty decimal.Decimal) decimal.Decimal {
	return price.Mul(qty)
}

// FeeFromNotional fee = notional * rate（不在此处舍入）。
func FeeFromNotional(notional, rate decimal.Decimal) decimal.Decimal {
	return notional.Mul(rate)
}

// FeeFromBaseQty fee = baseQty * rate（现货买方向常用，不在此处舍入）。
func FeeFromBaseQty(baseQty, rate decimal.Decimal) decimal.Decimal {
	return baseQty.Mul(rate)
}
