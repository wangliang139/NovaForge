package converter

import (
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/repos/orders"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
)

func OrderDb2Types(item orders.Order) (*ctypes.Order, error) {
	symbol, err := ctypes.ParseSymbol(item.Symbol)
	if err != nil {
		return nil, err
	}
	side := ctypes.PositionSideLong
	if item.Side == orders.OrderSideSHORT {
		side = ctypes.PositionSideShort
	}
	orderType := ctypes.OrderTypeMarket
	if item.OrderType == orders.OrderTypeLIMIT {
		orderType = ctypes.OrderTypeLimit
	}

	// 转换 algo_type
	algoType := ctypes.AlgoType(item.AlgoType)

	// 转换 source
	source := ctypes.OrderSource(item.Source)

	// 转换 conditions
	var conditions []ctypes.OrderCondition
	if len(item.Conditions) > 0 {
		if err := sonic.Unmarshal(item.Conditions, &conditions); err != nil {
			return nil, fmt.Errorf("unmarshal conditions: %w", err)
		}
	}

	// 处理拒绝原因
	rejectReason := ""
	if item.RejectReason != nil {
		rejectReason = *item.RejectReason
	}

	executedQty := utils.Decimal.PgNumericToDecimal(item.ExecutedQty)
	executedQuoteQty := utils.Decimal.PgNumericToDecimal(item.ExecutedPrice)
	avgPrice := utils.Decimal.PgNumericToDecimal(item.AvgPrice)

	isWorking := false
	if item.WorkingTs != nil && !item.WorkingTs.IsZero() {
		isWorking = true
	}

	var locked, fee, realizedPnl *decimal.Decimal
	if item.Locked.Valid {
		locked = lo.ToPtr(utils.Decimal.PgNumericToDecimal(item.Locked))
	}
	if item.Fee.Valid {
		fee = lo.ToPtr(utils.Decimal.PgNumericToDecimal(item.Fee))
	}
	if item.RealizedPnl.Valid {
		realizedPnl = lo.ToPtr(utils.Decimal.PgNumericToDecimal(item.RealizedPnl))
	}

	return &ctypes.Order{
		BotID:            int64(item.BotID),
		AccountID:        item.AccountID,
		OrderID:          ctypes.OrderId(item.OrderID),
		ClientOrderID:    ctypes.OrderId(item.ClientOrderID),
		DrivedOrderID:    ctypes.OrderId(item.DrivedOrderID),
		Exchange:         ctypes.Exchange(item.Exchange),
		Symbol:           symbol,
		Side:             side,
		IsBuy:            item.IsBuy,
		OrderType:        orderType,
		AlgoType:         algoType,
		Source:           source,
		ReduceOnly:       item.ReduceOnly,
		PostOnly:         item.PostOnly,
		TimeInForce:      ctypes.TimeInForce(item.Tif),
		Price:            utils.Decimal.PgNumericToDecimal(item.Price),
		OriginalQty:      utils.Decimal.PgNumericToDecimal(item.Quantity),
		ExecutedQty:      executedQty,
		ExecutedQuoteQty: executedQuoteQty,
		AvgPrice:         avgPrice,
		Conditions:       conditions,
		Status:           ctypes.OrderStatus(item.Status),
		RejectReason:     rejectReason,
		IsWorking:        isWorking,
		WorkingTs:        item.WorkingTs,
		CreatedTs:        item.CreatedTs,
		UpdatedTs:        item.UpdatedTs,
		FinishedTs:       item.FinishedTs,
		Locked:           locked,
		LockedAsset:      item.LockedAsset,
		Fee:              fee,
		FeeAsset:         item.FeeAsset,
		RealizedPnl:      realizedPnl,
		PnlAsset:         item.PnlAsset,
	}, nil
}
