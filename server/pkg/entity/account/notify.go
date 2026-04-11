package account

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/internal/push"
	"github.com/wangliang139/NovaForge/server/pkg/repos/orders"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
)

func (e *Entity) maybeNotifyOrderTelegram(ctx context.Context, prev *orders.Order, ord *ctypes.Order) {
	if ord == nil {
		return
	}
	curStatus := ctypes.OrderStatus(strings.ToUpper(strings.TrimSpace(string(ord.Status))))
	if curStatus == "" {
		curStatus = ctypes.OrderStatusNew
	}

	var isCreate, isFinishedTransition bool
	if prev == nil {
		isCreate = true
	} else {
		prevStatus := ctypes.OrderStatus(strings.ToUpper(strings.TrimSpace(prev.Status)))
		if !prevStatus.IsFinished() && curStatus.IsFinished() {
			isFinishedTransition = true
		}
	}

	if !isCreate && !isFinishedTransition {
		return
	}

	title := "📋 新订单"
	if isFinishedTransition {
		title = "✅ 订单已结束"
	}

	accountDisplay := ord.AccountID
	if ord.AccountID != "" {
		acct, err := e.GetAccount(ctx, ord.AccountID)
		if err != nil {
			logger.Ctx(ctx).Warn().Err(err).Str("account_id", ord.AccountID).Msg("failed to get account for telegram notify")
		} else if acct != nil && strings.TrimSpace(acct.Name) != "" {
			accountDisplay = fmt.Sprintf("%s (%s)", acct.Name, ord.AccountID)
		}
	}

	args := buildOrderNotifyArgs(title, ord, curStatus, accountDisplay)

	go func() {
		sendCtx := context.WithoutCancel(ctx)
		err := push.NotifyByTemplate(sendCtx, push.NotifyByTemplateRequest{
			SceneKey: "trade.order",
			Vars:     args,
		})
		if err != nil {
			logger.Ctx(sendCtx).Warn().Err(err).Str("account_id", ord.AccountID).Msg("failed to send trade push")
		}
	}()
}

func buildOrderNotifyArgs(title string, ord *ctypes.Order, status ctypes.OrderStatus, accountDisplay string) map[string]any {
	symbol := ord.Symbol.String()

	source := string(ord.Source)
	switch ord.Source {
	case ctypes.OrderSourceUser:
		source = "👤"
	case ctypes.OrderSourceStrategy:
		source = "🤖"
	case ctypes.OrderSourceLiquidation:
		source = "⚠️"
	case ctypes.OrderSourceADL:
		source = "📉"
	case "":
		source = "❓"
	}

	direction := ""
	if ord.Symbol.Type == ctypes.MarketTypeSpot {
		if ord.IsBuy {
			direction = "买入"
		} else {
			direction = "卖出"
		}
	} else {
		if ord.IsBuy && ord.Side == ctypes.PositionSideLong {
			direction = "开多"
		} else if !ord.IsBuy && ord.Side == ctypes.PositionSideShort {
			direction = "开空"
		} else if ord.IsBuy && ord.Side == ctypes.PositionSideShort {
			direction = "平空"
		} else if !ord.IsBuy && ord.Side == ctypes.PositionSideLong {
			direction = "平多"
		}
	}

	price := string(ord.OrderType)
	switch ord.OrderType {
	case ctypes.OrderTypeMarket:
		price = "市价"
	case ctypes.OrderTypeLimit:
		if !ord.Price.IsZero() {
			price = ord.Price.String()
		} else {
			price = "限价"
		}
	case "":
		price = "-"
	}

	amount := "-"
	if ord.OriginalQuoteQty.GreaterThan(decimal.Zero) {
		amount = fmt.Sprintf("%s (%s)", ord.OriginalQuoteQty.Round(8).String(), ord.Symbol.Quote)
	} else if ord.OriginalQty.GreaterThan(decimal.Zero) {
		amount = fmt.Sprintf("%s (%s)", ord.OriginalQty.Round(8).String(), ord.Symbol.Base)
	}

	ts := time.Now().Format("2006-01-02 15:04:05")
	return map[string]any{
		"title":     title,
		"account":   accountDisplay,
		"exchange":  ord.Exchange.String(),
		"symbol":    symbol,
		"orderId":   ord.OrderID.String(),
		"amount":    amount,
		"source":    source,
		"direction": direction,
		"price":     price,
		"status":    string(status),
		"time":      ts,
	}
}
