package converter

import (
	"github.com/samber/lo"
	"github.com/wangliang139/llt-trade/server/pkg/action/model"
	"github.com/wangliang139/llt-trade/server/pkg/repos/calendar"
)

func CalendarSourceGql2Pb(source *model.CalendarSource) *calendar.CalendarSource {
	if source == nil {
		return nil
	}
	switch *source {
	case model.CalendarSourceGateio:
		return lo.ToPtr(calendar.CalendarSourceGateio)
	case model.CalendarSourceJin10:
		return lo.ToPtr(calendar.CalendarSourceJin10)
	}
	return nil
}

func CalendarSourcePb2Gql(source calendar.CalendarSource) model.CalendarSource {
	switch source {
	case calendar.CalendarSourceGateio:
		return model.CalendarSourceGateio
	case calendar.CalendarSourceJin10:
		return model.CalendarSourceJin10
	}
	return model.CalendarSourceUnspecified
}

func CalendarTypeGql2Pb(_type *model.CalendarType) *calendar.CalendarType {
	if _type == nil {
		return nil
	}
	switch *_type {
	case model.CalendarTypeEconomicData:
		return lo.ToPtr(calendar.CalendarTypeEconomicData)
	case model.CalendarTypeProjectEvent:
		return lo.ToPtr(calendar.CalendarTypeProjectEvent)
	case model.CalendarTypeTokenUnlock:
		return lo.ToPtr(calendar.CalendarTypeTokenUnlock)
	case model.CalendarTypeSummitEvent:
		return lo.ToPtr(calendar.CalendarTypeSummitEvent)
	case model.CalendarTypeFinancing:
		return lo.ToPtr(calendar.CalendarTypeFinancing)
	case model.CalendarTypeEvents:
		return lo.ToPtr(calendar.CalendarTypeEvents)
	case model.CalendarTypeOther:
		return lo.ToPtr(calendar.CalendarTypeOther)
	}
	return nil
}

func CalendarTypePb2Gql(_type calendar.CalendarType) model.CalendarType {
	switch _type {
	case calendar.CalendarTypeEconomicData:
		return model.CalendarTypeEconomicData
	case calendar.CalendarTypeProjectEvent:
		return model.CalendarTypeProjectEvent
	case calendar.CalendarTypeTokenUnlock:
		return model.CalendarTypeTokenUnlock
	case calendar.CalendarTypeSummitEvent:
		return model.CalendarTypeSummitEvent
	case calendar.CalendarTypeFinancing:
		return model.CalendarTypeFinancing
	case calendar.CalendarTypeEvents:
		return model.CalendarTypeEvents
	case calendar.CalendarTypeOther:
		return model.CalendarTypeOther
	}
	return model.CalendarTypeOther
}
