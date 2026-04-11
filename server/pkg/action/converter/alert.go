package converter


import (
	"github.com/wangliang139/llt-trade/server/pkg/action/model"
	"github.com/wangliang139/llt-trade/server/pkg/service/alertsvc"
)

func ToGqlAlertItem(item alertsvc.AlertItem) *model.AlertItem {
	var price, percent *string
	if item.Price != nil {
		v := item.Price.String()
		price = &v
	}
	if item.Percent != nil {
		v := item.Percent.String()
		percent = &v
	}
	var lastTriggeredAt *int
	if item.LastTriggeredAt != nil {
		v := int(item.LastTriggeredAt.UnixMilli())
		lastTriggeredAt = &v
	}
	return &model.AlertItem{
		ID:              item.ID,
		Exchange:        item.Exchange,
		Symbol:          item.Symbol,
		Type:            model.AlertType(item.Type),
		Frequency:       model.AlertFrequency(item.Frequency),
		Price:           price,
		Window:          item.Window,
		Percent:         percent,
		Remark:          item.Remark,
		CooldownSeconds: item.CooldownSeconds,
		Status:          model.AlertStatus(item.Status),
		LastTriggeredAt: lastTriggeredAt,
		TriggerCount:    item.TriggerCount,
		CreatedAt:       int(item.CreatedAt.UnixMilli()),
		UpdatedAt:       int(item.UpdatedAt.UnixMilli()),
	}
}