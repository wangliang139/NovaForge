package document

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/bytedance/sonic"
	"github.com/samber/lo"
	"github.com/wangliang139/llt-trade/server/pkg/internal/gateio"
	"github.com/wangliang139/llt-trade/server/pkg/repos/calendar"
	"github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/utils"
	"github.com/wangliang139/mow/logger"
	"go.uber.org/ratelimit"
)

func (e *Entity) SyncGateCalendar(ctx context.Context, input SyncCalendarInput) (*SyncCalendarOutput, error) {
	logger.Ctx(ctx).Info().Msgf("start sync gateio calendar, input: %+v", input)

	gio, err := e.gateioClientFor(ctx)
	if err != nil {
		return nil, fmt.Errorf("gateio client: %w", err)
	}

	// 默认查询前1周 - 后1月的数据
	var (
		now              = time.Now().Unix()
		startTime *int64 = lo.ToPtr(now - 7*24*60*60)
		endTime   *int64 = lo.ToPtr(now + 14*24*60*60)
	)

	if input.StartDate != nil {
		dt, err := time.Parse(time.DateOnly, *input.StartDate)
		if err != nil {
			return nil, fmt.Errorf("failed to parse start date: %w", err)
		}
		startTime = lo.ToPtr(dt.Unix())
	}
	if input.EndDate != nil {
		dt, err := time.Parse(time.DateOnly, *input.EndDate)
		if err != nil {
			return nil, fmt.Errorf("failed to parse end date: %w", err)
		}
		endTime = lo.ToPtr(dt.Unix())
	}

	rl := ratelimit.New(1)

	page := 1
	batchSize := 100

	count := 0
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context done")
		default:
		}
		rl.Take()
		logger.Ctx(ctx).Info().Msgf("start sync gateio calendar page: %d", page)
		response, err := gio.GetFutureList(ctx, page, batchSize, *startTime, *endTime)
		if err != nil {
			return nil, err
		}

		for _, future := range response.Data.Items {
			err = e.saveGateCalendar(ctx, future)
			if err != nil {
				logger.Ctx(ctx).Info().Msgf("failed to save gateio calendar: %v", err)
				logger.Ctx(ctx).Err(err).Msg("failed to save gateio calendar")
				return nil, err
			}
			count++
		}
		if page >= response.Data.PageInfo.TotalPage {
			break
		}
		page++
	}

	return &SyncCalendarOutput{
		Count: count,
	}, nil
}

func (e *Entity) saveGateCalendar(ctx context.Context, future *gateio.Future) error {
	var (
		extBytes []byte
		err      error
	)
	if future.ExtensionInfo != nil {
		ext := types.EconomicCalendarExtension{
			Unit:      future.ExtensionInfo.Unit,
			Actual:    future.ExtensionInfo.Actual,
			Previous:  future.ExtensionInfo.Previous,
			Consensus: future.ExtensionInfo.Consensus,
		}
		extBytes, err = sonic.Marshal(ext)
		if err != nil {
			return fmt.Errorf("failed to marshal extension info: %w", err)
		}
	}

	_type := calendar.CalendarTypeOther
	if future.Category != "" {
		switch future.Category {
		case "1030001":
			_type = calendar.CalendarTypeProjectEvent
		case "1030002":
			_type = calendar.CalendarTypeEconomicData
		case "1030003":
			_type = calendar.CalendarTypeTokenUnlock
		case "1030004":
			_type = calendar.CalendarTypeSummitEvent
		case "1030022":
			_type = calendar.CalendarTypeFinancing
		}
	}

	md5, err := utils.Hash.Md5(fmt.Sprintf("%s%s%s%s", calendar.CalendarSourceGateio, _type, lo.FromPtrOr(future.Country, ""), future.Title))
	if err != nil {
		return fmt.Errorf("failed to generate md5: %w", err)
	}

	// 如果 sid 重复，则更新指定sid记录
	po, err := e.db.CalendarRepo.GetBySid(ctx, calendar.GetBySidParams{
		Source: calendar.CalendarSourceGateio,
		Sid:    strconv.Itoa(future.ID),
	})
	if err != nil {
		return fmt.Errorf("failed to get gateio calendar by sid: %w", err)
	}
	if po != nil {
		_, err = e.db.CalendarRepo.UpdateBySid(ctx, calendar.UpdateBySidParams{
			DateID:      utils.Datetime.UnixToDateID(future.PubTime),
			Source:      calendar.CalendarSourceGateio,
			Sid:         strconv.Itoa(future.ID),
			Type:        _type,
			Category:    mapGateioCategory(future.SecondCategory),
			Title:       future.Title,
			Content:     future.ContentText,
			Project:     future.ProjectName,
			Symbol:      future.Symbol,
			Country:     future.Country,
			Url:         fmt.Sprintf("https://www.gate.com/zh%s", future.TargetUrl),
			Ext:         extBytes,
			Importance:  1,
			PublishedAt: time.Unix(future.PubTime, 0),
			Md5:         md5,
		})
	} else {
		_, err = e.db.CalendarRepo.Upsert(ctx, calendar.UpsertParams{
			DateID:      utils.Datetime.UnixToDateID(future.PubTime),
			Source:      calendar.CalendarSourceGateio,
			Sid:         strconv.Itoa(future.ID),
			Type:        _type,
			Category:    mapGateioCategory(future.SecondCategory),
			Title:       future.Title,
			Content:     future.ContentText,
			Project:     future.ProjectName,
			Symbol:      future.Symbol,
			Country:     future.Country,
			Url:         fmt.Sprintf("https://www.gate.com/zh%s", future.TargetUrl),
			Ext:         extBytes,
			Importance:  1,
			PublishedAt: time.Unix(future.PubTime, 0),
			Md5:         md5,
		})
	}

	if err != nil {
		return fmt.Errorf("failed to save gateio calendar: %w", err)
	}
	return nil
}

func mapGateioCategory(category string) string {
	switch category {
	case "1030001":
		return "ProjectEvents"
	case "1030002":
		return "EconomicData"
	case "1030003":
		return "TokenUnlock"
	case "1030004":
		return "SummitEvents"
	case "1030005":
		return "NewProductRelease"
	case "1030006":
		return "AMA"
	case "1030007":
		return "NFT"
	case "1030008":
		return "Partnership"
	case "1030009":
		return "Branding"
	case "1030011":
		return "Usa"
	case "1030012":
		return "China"
	case "1030013":
		return "Russia"
	case "1030014":
		return "Singapore"
	case "1030015":
		return "Vietnam"
	case "1030016":
		return "France"
	case "1030017":
		return "Brazil"
	case "1030018":
		return "Spain"
	case "1030019":
		return "Portugal"
	case "1030020":
		return "Turkey"
	case "1030022":
		return "Financing"
	case "1030021", "1030010", "":
		return "Other"
	}
	return category
}
