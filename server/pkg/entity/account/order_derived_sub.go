package account

import (
	"context"
	"encoding/json"
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
// 派生 account_raw（BalanceUpdate / PositionsUpdate）并走 handleAccountMessage，使资金/仓位与订单 Fill 对齐。
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
	prevExecutedQty := decimal.Zero
	if prev != nil {
		prevExecutedQty = utils.Decimal.PgNumericToDecimal(prev.ExecutedQty)
	}
	fillQtyDelta := order.ExecutedQty.Sub(prevExecutedQty)
	if !fillQtyDelta.GreaterThan(decimal.Zero) {
		return nil
	}
	prevExec := decimal.Zero
	if prev != nil {
		prevExec = utils.Decimal.PgNumericToDecimal(prev.ExecutedQty)
	}
	if !order.ExecutedQty.GreaterThan(prevExec) {
		return nil
	}
	return e.applyVirtualSubOrderFillDerivedRawAccounts(ctx, order, prev)
}

func futureFillPositionQtyDelta(side ctypes.PositionSide, isBuy bool, fillQty decimal.Decimal) decimal.Decimal {
	if !fillQty.GreaterThan(decimal.Zero) {
		return decimal.Zero
	}
	switch side {
	case ctypes.PositionSideLong:
		if isBuy {
			return fillQty
		}
		return fillQty.Neg()
	case ctypes.PositionSideShort:
		if isBuy {
			return fillQty.Neg()
		}
		return fillQty
	default:
		return decimal.Zero
	}
}

func (e *Entity) buildFuturePositionAfterOrderFill(ctx context.Context, accountID string, exchange ctypes.Exchange, order *ctypes.Order, fillQtyDelta decimal.Decimal) (*ctypes.Position, error) {
	if order.Symbol.Type != ctypes.MarketTypeFuture {
		return nil, nil
	}
	posSide := positions.PositionSideLONG
	if order.Side == ctypes.PositionSideShort {
		posSide = positions.PositionSideSHORT
	}
	symStr := order.Symbol.String()
	exStr := exchange.String()
	row, err := e.db.PositionsRepo.GetPosition(ctx, positions.GetPositionParams{
		AccountID: accountID,
		Exchange:  exStr,
		Symbol:    symStr,
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
	d := futureFillPositionQtyDelta(order.Side, order.IsBuy, fillQtyDelta)
	newAmt := prevAmt.Add(d)
	if newAmt.LessThan(decimal.Zero) {
		newAmt = decimal.Zero
	}
	var newEntry decimal.Decimal
	switch {
	case newAmt.IsZero():
		newEntry = decimal.Zero
	case prevAmt.IsZero():
		newEntry = order.AvgPrice
	case d.GreaterThan(decimal.Zero):
		newEntry = prevEntry.Mul(prevAmt).Add(order.AvgPrice.Mul(d)).Div(newAmt)
	default:
		newEntry = prevEntry
	}
	ctpSide := ctypes.PositionSideLong
	if posSide == positions.PositionSideSHORT {
		ctpSide = ctypes.PositionSideShort
	}
	ts := order.UpdatedTs
	if ts.IsZero() {
		ts = time.Now()
	}
	return &ctypes.Position{
		AccountID:  accountID,
		Exchange:   exchange,
		Symbol:     order.Symbol,
		Side:       ctpSide,
		Amount:     newAmt,
		EntryPrice: newEntry,
		Leverage:   int(lev),
		UpdatedTs:  ts,
	}, nil
}

// applyVirtualSubOrderFillDerivedRawAccounts 根据订单与 prev 行计算与 sendOrderDerivedFillEvent 一致的成交增量，构造增量 BalanceUpdate（现货划转+手续费；合约已实现盈亏+手续费）及合约仓位 PositionsUpdate，经 account_raw 入站处理落库并对外 Publish。
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
	if ts.IsZero() {
		ts = time.Now()
	}

	tsMillis := time.Now().UnixMilli()
	if order != nil && !order.UpdatedTs.IsZero() {
		tsMillis = order.UpdatedTs.UnixMilli()
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

		env := &ctypes.Envelope{
			Exchange: order.Exchange.String(),
			Account:  lo.ToPtr(order.AccountID),
			Stream:   ctypes.StreamTypeAccountRaw,
			Payload:  &ctypes.Message{BalanceUpdate: bu},
			Ts:       tsMillis,
		}
		if err := e.handleAccountMessage(ctx, env); err != nil {
			logger.Ctx(ctx).Err(err).Str("account_id", order.AccountID).Msg("virtual_sub derived BalanceUpdate")
			return err
		}
	}

	if order.Symbol.Type != ctypes.MarketTypeFuture {
		return nil
	}
	pos, err := e.buildFuturePositionAfterOrderFill(ctx, order.AccountID, order.Exchange, order, fillQtyDelta)
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
	env := &ctypes.Envelope{
		Exchange: order.Exchange.String(),
		Account:  lo.ToPtr(order.AccountID),
		Stream:   ctypes.StreamTypeAccountRaw,
		Payload:  &ctypes.Message{PositionsUpdate: pu},
		Ts:       tsMillis,
	}
	if err := e.handleAccountMessage(ctx, env); err != nil {
		logger.Ctx(ctx).Err(err).Str("account_id", order.AccountID).Msg("virtual_sub derived PositionsUpdate")
		return err
	}
	return nil
}
