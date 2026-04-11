package converter

import (
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/action/model"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	stypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// FillTypes2Gql 将运行时成交事件转为 GraphQL model.Fill。
func FillTypes2Gql(f *stypes.Fill) *model.Fill {
	if f == nil {
		return nil
	}
	side := model.PositionSideLong
	switch f.Side {
	case stypes.PositionSideShort:
		side = model.PositionSideShort
	}
	return &model.Fill{
		Exchange:      f.Exchange,
		Symbol:        f.Symbol.String(),
		OrderID:       f.OrderID.String(),
		ClientOrderID: f.ClientOrderID.String(),
		TradeID:       f.TradeID,
		Side:          side,
		IsBuy:         f.IsBuy,
		Qty:           f.Qty.String(),
		Price:         f.Price.String(),
		Fee:           f.Fee.String(),
		FeeAsset:      f.FeeAsset,
		RealizedPnl:   f.RealizedPnl.String(),
		IsMaker:       f.IsMaker,
		Ts:            int(f.Ts.UnixMilli()),
	}
}

// OrderTypes2Gql 将运行时订单模型转为 GraphQL model.Order。
func OrderTypes2Gql(o *stypes.Order) *model.Order {
	if o == nil {
		return nil
	}
	conditions := make([]*model.OrderCondition, 0, len(o.Conditions))
	for i := range o.Conditions {
		conditions = append(conditions, OrderConditionTypes2Gql(&o.Conditions[i]))
	}

	side := model.PositionSideLong
	switch o.Side {
	case stypes.PositionSideLong:
		side = model.PositionSideLong
	case stypes.PositionSideShort:
		side = model.PositionSideShort
	}

	ot := model.OrderTypeLimit
	switch o.OrderType {
	case stypes.OrderTypeMarket:
		ot = model.OrderTypeMarket
	case stypes.OrderTypeLimit:
		ot = model.OrderTypeLimit
	case stypes.OrderTypeUnknown:
		ot = model.OrderTypeNone
	}

	algo := model.AlgoTypeNone
	switch o.AlgoType {
	case stypes.AlgoTypeNone:
		algo = model.AlgoTypeNone
	case stypes.AlgoTypeConditional:
		algo = model.AlgoTypeConditional
	case stypes.AlgoTypeTrailing:
		algo = model.AlgoTypeTrailing
	case stypes.AlgoTypeOCO:
		algo = model.AlgoTypeOco
	case stypes.AlgoTypeTWAP:
		algo = model.AlgoTypeTwap
	case stypes.AlgoTypeIceberg:
		algo = model.AlgoTypeIceberg
	case stypes.AlgoTypeChase:
		algo = model.AlgoTypeChase
	default:
		algo = model.AlgoTypeNone
	}

	src := model.OrderSourceUser
	switch o.Source {
	case stypes.OrderSourceUser:
		src = model.OrderSourceUser
	case stypes.OrderSourceStrategy:
		src = model.OrderSourceStrategy
	case stypes.OrderSourceLiquidation:
		src = model.OrderSourceLiquidation
	case stypes.OrderSourceADL:
		src = model.OrderSourceAdl
	}

	st := OrderStatusTypes2Gql(o.Status)

	var workingTs int
	if o.WorkingTs != nil {
		workingTs = int(o.WorkingTs.UnixMilli())
	}
	var finishedTs int
	if o.FinishedTs != nil {
		finishedTs = int(o.FinishedTs.UnixMilli())
	}

	return &model.Order{
		AccountID:        o.AccountID,
		BotID:            int(o.BotID),
		Exchange:         o.Exchange,
		Symbol:           o.Symbol.String(),
		ClientOrderID:    o.ClientOrderID.String(),
		OrderID:          o.OrderID.String(),
		DrivedOrderID:    o.DrivedOrderID.String(),
		Side:             side,
		IsBuy:            o.IsBuy,
		OrderType:        ot,
		AlgoType:         algo,
		Source:           src,
		Price:            o.Price.String(),
		OriginalQty:      o.OriginalQty.String(),
		ExecutedQty:      o.ExecutedQty.String(),
		OriginalQuoteQty: o.OriginalQuoteQty.String(),
		ExecutedQuoteQty: o.ExecutedQuoteQty.String(),
		AvgPrice:         o.AvgPrice.String(),
		PriceWorkingType: string(o.PriceWorkingType),
		PriceMode:        o.PriceMode,
		Status:           st,
		TimeInForce:      string(o.TimeInForce),
		ReduceOnly:       o.ReduceOnly,
		ClosePosition:    o.ClosePosition,
		PostOnly:         o.PostOnly,
		Conditions:       conditions,
		IsWorking:        o.IsWorking,
		WorkingTs:        workingTs,
		RejectReason:     o.RejectReason,
		CreatedTs:        int(o.CreatedTs.UnixMilli()),
		UpdatedTs:        int(o.UpdatedTs.UnixMilli()),
		FinishedTs:       finishedTs,
		Locked:           decPtrStr(o.Locked),
		LockedAsset:      o.LockedAsset,
		Fee:              decPtrStr(o.Fee),
		FeeAsset:         o.FeeAsset,
		RealizedPnl:      decPtrStr(o.RealizedPnl),
		PnlAsset:         o.PnlAsset,
	}
}

func decPtrStr(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.String()
	return &s
}

func OrderConditionTypes2Gql(c *stypes.OrderCondition) *model.OrderCondition {
	if c == nil {
		return nil
	}
	return &model.OrderCondition{
		TriggerType:      string(c.TriggerType),
		OrderPrice:       c.OrderPrice.String(),
		CallbackDistance: c.CallbackDistance.String(),
		CallbackRate:     c.CallbackRate.String(),
		ActivationPrice:  c.ActivationPrice.String(),
		PriceWorkingType: string(c.PriceWorkingType),
		PriceMode:        c.PriceMode,
		IsTrailing:       c.IsTrailing,
		Activated:        c.Activated,
		ActivatedTs: func() int {
			if c.ActivatedTs == nil {
				return 0
			}
			return int(c.ActivatedTs.UnixMilli())
		}(),
	}
}

func OrderTypeTypes2Gql(orderType types.OrderType) model.OrderType {
	switch orderType {
	case types.OrderTypeMarket:
		return model.OrderTypeMarket
	case types.OrderTypeLimit:
		return model.OrderTypeLimit
	}
	return model.OrderTypeMarket
}

func AlgoTypeTypes2Gql(algoType types.AlgoType) model.AlgoType {
	switch algoType {
	case types.AlgoTypeNone:
		return model.AlgoTypeNone
	case types.AlgoTypeConditional:
		return model.AlgoTypeConditional
	case types.AlgoTypeTrailing:
		return model.AlgoTypeTrailing
	case types.AlgoTypeOCO:
		return model.AlgoTypeOco
	case types.AlgoTypeTWAP:
		return model.AlgoTypeTwap
	case types.AlgoTypeIceberg:
		return model.AlgoTypeIceberg
	case types.AlgoTypeChase:
		return model.AlgoTypeChase
	}
	return model.AlgoTypeNone
}

func OrderSourceTypes2Gql(source types.OrderSource) model.OrderSource {
	switch source {
	case types.OrderSourceUser:
		return model.OrderSourceUser
	case types.OrderSourceStrategy:
		return model.OrderSourceStrategy
	case types.OrderSourceLiquidation:
		return model.OrderSourceLiquidation
	case types.OrderSourceADL:
		return model.OrderSourceAdl
	}
	return model.OrderSourceUser
}

func OrderStatusTypes2Gql(status types.OrderStatus) model.OrderStatus {
	switch status {
	case types.OrderStatusNew:
		return model.OrderStatusNew
	case types.OrderStatusPending:
		return model.OrderStatusPending
	case types.OrderStatusWorking:
		return model.OrderStatusWorking
	case types.OrderStatusPartialDone:
		return model.OrderStatusPartialDone
	case types.OrderStatusDone:
		return model.OrderStatusDone
	case types.OrderStatusCanceled:
		return model.OrderStatusCanceled
	case types.OrderStatusRejected:
		return model.OrderStatusRejected
	case types.OrderStatusExpired:
		return model.OrderStatusExpired
	}
	return model.OrderStatusNew
}

func OrderTypeGql2Types(orderType *model.OrderType) types.OrderType {
	if orderType == nil {
		return types.OrderTypeUnknown
	}
	switch *orderType {
	case model.OrderTypeMarket:
		return types.OrderTypeMarket
	case model.OrderTypeLimit:
		return types.OrderTypeLimit
	}
	return types.OrderTypeUnknown
}

func OrderSourceGql2Types(source *model.OrderSource) *types.OrderSource {
	if source == nil {
		return nil
	}
	switch *source {
	case model.OrderSourceUser:
		return lo.ToPtr(types.OrderSourceUser)
	case model.OrderSourceStrategy:
		return lo.ToPtr(types.OrderSourceStrategy)
	case model.OrderSourceLiquidation:
		return lo.ToPtr(types.OrderSourceLiquidation)
	case model.OrderSourceAdl:
		return lo.ToPtr(types.OrderSourceADL)
	}
	return nil
}

func OrderStatusGql2Types(status *model.OrderStatus) types.OrderStatus {
	if status == nil {
		return types.OrderStatusNew
	}
	switch *status {
	case model.OrderStatusNew:
		return types.OrderStatusNew
	case model.OrderStatusPending:
		return types.OrderStatusPending
	case model.OrderStatusWorking:
		return types.OrderStatusWorking
	case model.OrderStatusPartialDone:
		return types.OrderStatusPartialDone
	case model.OrderStatusDone:
		return types.OrderStatusDone
	case model.OrderStatusCanceled:
		return types.OrderStatusCanceled
	case model.OrderStatusRejected:
		return types.OrderStatusRejected
	case model.OrderStatusExpired:
		return types.OrderStatusExpired
	}
	return types.OrderStatusNew
}
