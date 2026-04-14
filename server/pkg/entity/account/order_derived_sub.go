package account

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/repos/orders"
	"github.com/wangliang139/NovaForge/server/pkg/repos/positions"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/logger"
	"github.com/wangliang139/mow/snowflake"
)

// publishVsAcctSnapshotsFromDB 从子账户 DB 投影发布 BalanceSnapshot；可选发布合约 PositionSnapshot（现货不落仓位类事件）。
func (e *Entity) publishVsAcctSnapshotsFromDB(ctx context.Context, accountID string, exchange ctypes.Exchange) error {
	if e == nil || accountID == "" || !exchange.IsValid() {
		return nil
	}

	if err := e.publishVsAcctAssetSnapshots(ctx, accountID, exchange); err != nil {
		return err
	}

	if err := e.publishVsAcctPositionSnapshots(ctx, accountID, exchange); err != nil {
		return err
	}

	return nil
}

func (e *Entity) publishVsAcctAssetSnapshots(ctx context.Context, accountID string, exchange ctypes.Exchange) error {
	if e == nil || accountID == "" || !exchange.IsValid() {
		return nil
	}

	scope := []ctypes.WalletType{}
	switch exchange {
	case ctypes.ExchangeBinance, ctypes.ExchangeBinanceTest:
		scope = []ctypes.WalletType{ctypes.WalletTypeFund, ctypes.WalletTypeSpot, ctypes.WalletTypeFuture, ctypes.WalletTypeMargin}
	case ctypes.ExchangeOkx, ctypes.ExchangeOkxTest:
		scope = []ctypes.WalletType{ctypes.WalletTypeFund, ctypes.WalletTypeTrade}
	}

	var rows []*ctypes.Asset
	var err error
	if len(scope) > 0 {
		rows, err = e.getAssetsByScope(ctx, accountID, scope)
	} else {
		rows, err = e.GetAssets(ctx, accountID)
		if err != nil {
			return err
		}
		for _, a := range rows {
			if a == nil {
				continue
			}
			if !slices.Contains(scope, a.WalletType) {
				scope = append(scope, a.WalletType)
			}
		}
	}
	if err != nil {
		return err
	}

	ts := time.Now()
	snap := &ctypes.BalanceSnapshot{Scope: scope, Assets: make([]*ctypes.AssetEvent, 0, len(rows))}
	for _, a := range rows {
		if a == nil {
			continue
		}
		bal := a.Balance
		locked := a.Locked()
		snap.Assets = append(snap.Assets, &ctypes.AssetEvent{
			WalletType: a.WalletType,
			Code:       a.Code,
			Balance:    &bal,
			Locked:     &locked,
			UpdatedTs:  a.UpdatedTs,
		})
		if !a.UpdatedTs.IsZero() && a.UpdatedTs.After(ts) {
			ts = a.UpdatedTs
		}
	}

	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccount,
		Account: lo.ToPtr(accountID),
	}
	if err := e.PublishEvent(ctx, ctypes.NewMessage(exchange, selector, snap, ts)); err != nil {
		return err
	}

	return nil
}

func (e *Entity) publishVsAcctPositionSnapshots(ctx context.Context, accountID string, exchange ctypes.Exchange) error {
	if e == nil || accountID == "" || !exchange.IsValid() {
		return nil
	}

	ts := time.Now()

	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccount,
		Account: lo.ToPtr(accountID),
	}

	allPos, err := e.GetPositions(ctx, accountID)
	if err != nil {
		return err
	}
	fut := make([]*ctypes.Position, 0, len(allPos))
	for _, p := range allPos {
		if p != nil && p.Exchange == exchange && p.Symbol.Type == ctypes.MarketTypeFuture {
			fut = append(fut, p)
		}
	}
	posTs := ts
	for _, p := range fut {
		if p != nil && !p.UpdatedTs.IsZero() && p.UpdatedTs.After(posTs) {
			posTs = p.UpdatedTs
		}
	}
	posSnap := ctypes.PositionSnapshot{Positions: fut}
	if err := e.PublishEvent(ctx, ctypes.NewMessage(exchange, selector, posSnap, posTs)); err != nil {
		return err
	}
	return nil
}

type mergedAssetDelta struct {
	wt            ctypes.WalletType
	code          string
	total, locked decimal.Decimal
}

func mergeVsFillAssetDelta(m map[string]*mergedAssetDelta, wt ctypes.WalletType, code string, totalD, lockedD decimal.Decimal) {
	c := strings.ToUpper(strings.TrimSpace(code))
	if c == "" {
		return
	}
	key := string(wt) + ":" + c
	a := m[key]
	if a == nil {
		a = &mergedAssetDelta{wt: wt, code: c}
		m[key] = a
	}
	a.total = a.total.Add(totalD)
	a.locked = a.locked.Add(lockedD)
}

// driveBalanceAndPositionEventByOrderIfNeeded 在订单落库事务提交之后调用：virtual_sub 且本轮有成交增量时，
// 派生 account_raw（BalanceUpdate / PositionsUpdate）经 PublishEvent 入队，由账户消费者异步 handleAccountMessage 落库。
func (e *Entity) driveBalanceAndPositionEventByOrderIfNeeded(ctx context.Context, order *ctypes.Order, prev *orders.Order) error {
	if e == nil || order == nil || order.AccountID == "" {
		return nil
	}
	acct, err := e.GetAccount(ctx, order.AccountID)
	if err != nil {
		return err
	}
	if acct == nil || acct.AccountType != ctypes.AccountTypeVirtualSub {
		return nil
	}
	return e.applyVirtualSubOrderFillDerivedRawAccounts(ctx, order, prev)
}

// 加仓：返回新仓位量和新的平均入场价
// 减仓：返回新仓位量和原来的平均入场价
func calcFuturePositionAfterOrderFill(side ctypes.PositionSide, isBuy bool, fillQty decimal.Decimal, prevAmt decimal.Decimal, prevEntry decimal.Decimal, fillAvgPrice decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	if !fillQty.GreaterThan(decimal.Zero) {
		return prevAmt, prevEntry
	}
	switch side {
	case ctypes.PositionSideLong:
		if isBuy {
			newAmt := prevAmt.Add(fillQty)
			return newAmt, prevEntry.Mul(prevAmt).Add(fillAvgPrice.Mul(fillQty)).Div(newAmt)
		}
		newAmt := prevAmt.Sub(fillQty)
		newEntry := prevEntry
		if newAmt.LessThan(decimal.Zero) {
			newAmt = decimal.Zero
			newEntry = decimal.Zero
		}
		return newAmt, newEntry
	case ctypes.PositionSideShort:
		if isBuy {
			newAmt := prevAmt.Sub(fillQty)
			newEntry := prevEntry
			if newAmt.LessThan(decimal.Zero) {
				newAmt = decimal.Zero
				newEntry = decimal.Zero
			}
			return newAmt, newEntry
		}
		newAmt := prevAmt.Add(fillQty)
		return newAmt, prevEntry.Mul(prevAmt).Add(fillAvgPrice.Mul(fillQty)).Div(newAmt)
	}
	return prevAmt, prevEntry
}

func (e *Entity) buildFuturePositionAfterOrderFill(ctx context.Context, accountID string, exchange ctypes.Exchange, order *ctypes.Order, fillQtyDelta decimal.Decimal, fillAvgPrice decimal.Decimal) (*ctypes.Position, error) {
	if order.Symbol.Type != ctypes.MarketTypeFuture {
		return nil, nil
	}
	posSide := positions.PositionSideLONG
	if order.Side == ctypes.PositionSideShort {
		posSide = positions.PositionSideSHORT
	}
	row, err := e.db.PositionsRepo.GetPosition(ctx, positions.GetPositionParams{
		AccountID: accountID,
		Exchange:  exchange.String(),
		Symbol:    order.Symbol.String(),
		Side:      posSide,
	})
	if err != nil {
		return nil, err
	}
	prevAmt := decimal.Zero
	prevEntry := decimal.Zero
	lev := int32(0)
	if row != nil {
		prevAmt = utils.Decimal.PgNumericToDecimal(row.Qty)
		prevEntry = utils.Decimal.PgNumericToDecimal(row.EntryPrice)
		lev = row.Leverage
	}
	newAmt, newEntry := calcFuturePositionAfterOrderFill(order.Side, order.IsBuy, fillQtyDelta, prevAmt, prevEntry, fillAvgPrice)
	if newAmt.Equal(prevAmt) {
		return nil, nil
	}
	logger.Ctx(ctx).Info().
		Str("account_id", accountID).
		Str("exchange", exchange.String()).
		Str("order_id", order.OrderID.String()).
		Str("symbol", order.Symbol.String()).
		Str("side", order.Side.String()).
		Str("fill_qty_delta", fillQtyDelta.String()).
		Str("fill_avg_price", fillAvgPrice.String()).
		Str("new_amt", newAmt.String()).
		Str("new_entry", newEntry.String()).
		Str("prev_amt", prevAmt.String()).
		Str("prev_entry", prevEntry.String()).
		Time("updated_ts", order.UpdatedTs).
		Msg("buildFuturePositionAfterOrderFill")
	return &ctypes.Position{
		AccountID:  accountID,
		Exchange:   exchange,
		Symbol:     order.Symbol,
		Side:       order.Side,
		Amount:     newAmt,
		EntryPrice: newEntry,
		Leverage:   int(lev),
		UpdatedTs:  order.UpdatedTs,
	}, nil
}

// applyVirtualSubOrderFillDerivedRawAccounts 根据订单与 prev 行计算与 sendOrderDerivedFillEvent 一致的成交增量，构造增量 BalanceUpdate（现货划转+手续费；合约已实现盈亏+手续费）及合约仓位 PositionsUpdate，
// 以 StreamTypeAccountRaw 经 PublishEvent 写入账户原始流，异步解耦落库与下游 Publish。
func (e *Entity) applyVirtualSubOrderFillDerivedRawAccounts(ctx context.Context, order *ctypes.Order, prev *orders.Order) error {
	prevExecutedQty := decimal.Zero
	prevFee := decimal.Zero
	prevPnl := decimal.Zero
	prevExecQuote := decimal.Zero
	if prev != nil {
		prevExecutedQty = utils.Decimal.PgNumericToDecimal(prev.ExecutedQty)
		prevFee = utils.Decimal.PgNumericToDecimal(prev.Fee)
		prevPnl = utils.Decimal.PgNumericToDecimal(prev.RealizedPnl)
		prevExecQuote = utils.Decimal.PgNumericToDecimal(prev.ExecutedPrice)
	}
	fillQtyDelta := order.ExecutedQty.Sub(prevExecutedQty)
	if !fillQtyDelta.GreaterThan(decimal.Zero) {
		return nil
	}
	currentFee := decimal.Zero
	if order.Fee != nil {
		currentFee = *order.Fee
	}
	feeDelta := currentFee.Sub(prevFee)
	currentPnl := decimal.Zero
	if order.RealizedPnl != nil {
		currentPnl = *order.RealizedPnl
	}
	pnlDelta := currentPnl.Sub(prevPnl)
	feeMag := feeDelta
	if feeMag.IsNegative() {
		feeMag = feeMag.Neg()
	}
	feeAsset := ""
	if order.FeeAsset != nil {
		feeAsset = strings.ToUpper(strings.TrimSpace(*order.FeeAsset))
	}

	ts := order.UpdatedTs

	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccountRaw,
		Account: lo.ToPtr(order.AccountID),
	}

	deltas := make(map[string]*mergedAssetDelta)

	switch order.Symbol.Type {
	case ctypes.MarketTypeSpot:
		wt := ctypes.GetWalletType(order.Exchange, order.Symbol.Type)
		base := strings.ToUpper(order.Symbol.Base)
		quote := strings.ToUpper(order.Symbol.Quote)
		execQuoteDelta := order.ExecutedQuoteQty.Sub(prevExecQuote)
		if execQuoteDelta.IsZero() && order.AvgPrice.GreaterThan(decimal.Zero) {
			execQuoteDelta = fillQtyDelta.Mul(order.AvgPrice)
		}
		if order.IsBuy {
			mergeVsFillAssetDelta(deltas, wt, quote, execQuoteDelta.Neg(), decimal.Zero)
			mergeVsFillAssetDelta(deltas, wt, base, fillQtyDelta, decimal.Zero)
		} else {
			mergeVsFillAssetDelta(deltas, wt, base, fillQtyDelta.Neg(), decimal.Zero)
			mergeVsFillAssetDelta(deltas, wt, quote, execQuoteDelta, decimal.Zero)
		}
		if feeMag.GreaterThan(decimal.Zero) && feeAsset != "" {
			mergeVsFillAssetDelta(deltas, wt, feeAsset, feeMag.Neg(), decimal.Zero)
		}

	case ctypes.MarketTypeFuture:
		q := strings.ToUpper(order.Symbol.Quote)
		fw := ctypes.GetWalletType(order.Exchange, order.Symbol.Type)
		if !pnlDelta.IsZero() {
			mergeVsFillAssetDelta(deltas, fw, q, pnlDelta, decimal.Zero)
		}
		if feeMag.GreaterThan(decimal.Zero) {
			mergeVsFillAssetDelta(deltas, fw, q, feeMag.Neg(), decimal.Zero)
		}
	}

	assets := make([]*ctypes.AssetEvent, 0, len(deltas))
	for _, a := range deltas {
		if a.total.IsZero() && a.locked.IsZero() {
			continue
		}
		balCopy := a.total
		lockCopy := a.locked
		assets = append(assets, &ctypes.AssetEvent{
			WalletType: a.wt,
			Code:       a.code,
			Balance:    &balCopy,
			Locked:     &lockCopy,
			UpdatedTs:  ts,
		})
	}

	detailBytes, err := sonic.Marshal(map[string]string{
		"source":   "virtual_sub_order_fill",
		"order_id": order.OrderID.String(),
	})
	if err != nil {
		detailBytes = nil
	}

	if len(assets) > 0 {
		bu := &ctypes.BalanceUpdate{
			EventID: snowflake.Generate().String(),
			Type:    ctypes.UpdateTypeIncrement,
			Reason:  ctypes.LedgerReasonFill,
			Assets:  assets,
			Detail:  json.RawMessage(detailBytes),
		}
		if err := e.PublishEvent(ctx, ctypes.NewMessage(order.Exchange, selector, bu, ts)); err != nil {
			logger.Ctx(ctx).Err(err).Str("account_id", order.AccountID).Msg("virtual_sub derived BalanceUpdate publish")
			return err
		}
	}

	if order.Symbol.Type != ctypes.MarketTypeFuture {
		return nil
	}
	fillExecQuoteDelta := order.ExecutedQuoteQty.Sub(prevExecQuote)
	fillAvgPrice := decimal.Zero
	if fillQtyDelta.GreaterThan(decimal.Zero) && fillExecQuoteDelta.GreaterThan(decimal.Zero) {
		fillAvgPrice = fillExecQuoteDelta.Div(fillQtyDelta)
	}
	if !fillAvgPrice.GreaterThan(decimal.Zero) {
		return errors.New("fillAvgPrice is zero")
	}
	pos, err := e.buildFuturePositionAfterOrderFill(ctx, order.AccountID, order.Exchange, order, fillQtyDelta, fillAvgPrice)
	if err != nil {
		return err
	}
	if pos == nil {
		return nil
	}
	pu := &ctypes.PositionsUpdate{
		EventID:   snowflake.Generate().String(),
		Type:      ctypes.UpdateTypeSnapshot,
		Reason:    string(ctypes.LedgerReasonFill),
		Positions: []*ctypes.Position{pos},
	}
	if err := e.PublishEvent(ctx, ctypes.NewMessage(order.Exchange, selector, pu, ts)); err != nil {
		logger.Ctx(ctx).Err(err).Str("account_id", order.AccountID).Msg("virtual_sub derived PositionsUpdate publish")
		return err
	}
	return nil
}
