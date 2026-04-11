package calculator

import (
	"errors"
	"fmt"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/llt-trade/server/pkg/types"
)

var one = decimal.NewFromInt(1)

func CalcInitialMargin(notional decimal.Decimal, leverage decimal.Decimal) decimal.Decimal {
	if leverage.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	return notional.Div(leverage)
}

func CalcMaintMargin(notional decimal.Decimal, leverageBracket *types.LeverageBracket) decimal.Decimal {
	mmr, cum := SelectMmrAndCum(notional, leverageBracket)
	return notional.Mul(mmr).Sub(cum)
}

func SelectMmrAndCum(notional decimal.Decimal, leverageBracket *types.LeverageBracket) (decimal.Decimal, decimal.Decimal) {
	for _, brkt := range leverageBracket.Brackets {
		if notional.GreaterThanOrEqual(brkt.MinNotional) && notional.LessThan(brkt.MaxNotional) {
			return brkt.Mmr, brkt.Cum
		}
	}
	return decimal.Zero, decimal.Zero
}

// CalcMarketOrderLocked 计算市价单锁定资产
// 公式：baseQty * lastPrice * beta / 100
// beta 为保证金比例，105% 表示 105% 的保证金
func CalcSpotOrderAssetLocked(order types.Order, ticker *types.Ticker) (*decimal.Decimal, *string, error) {
	if !order.Symbol.IsValid() {
		return nil, nil, fmt.Errorf("invalid order symbol")
	}

	remainingQty := order.OriginalQty.Sub(order.ExecutedQty)
	if remainingQty.LessThan(decimal.Zero) {
		remainingQty = decimal.Zero
	}
	if remainingQty.IsZero() {
		return nil, nil, nil
	}

	switch order.OrderType {
	case types.OrderTypeLimit:
		locked := remainingQty.Mul(order.Price)
		if order.IsBuy {
			return lo.ToPtr(locked), lo.ToPtr(order.Symbol.Quote), nil
		}
		return lo.ToPtr(locked), lo.ToPtr(order.Symbol.Base), nil
	case types.OrderTypeMarket:
		if !order.IsBuy {
			return lo.ToPtr(remainingQty), lo.ToPtr(order.Symbol.Base), nil
		}
		if ticker == nil || ticker.LastPrice.IsZero() {
			return nil, nil, fmt.Errorf("ticker is nil or last price is zero")
		}
		beta := decimal.NewFromInt(105)
		locked := remainingQty.Mul(ticker.LastPrice).Mul(beta).Div(decimal.NewFromInt(100))
		return lo.ToPtr(locked), lo.ToPtr(order.Symbol.Quote), nil
	}
	return nil, nil, nil
}

func CalcLiquidationPrice(symbol *types.SymbolConfig, leverageBracket *types.LeverageBracket, walletAsset types.Asset, positions []*types.Position, markPrice decimal.Decimal) (decimal.Decimal, error) {
	if symbol == nil {
		return decimal.Zero, errors.New("symbol is nil")
	}
	if markPrice.IsZero() {
		return decimal.Zero, errors.New("mark price is zero")
	}
	if leverageBracket == nil {
		return decimal.Zero, errors.New("leverage bracket is nil")
	}

	netAmount, netEntryPrice, netSide, err := getNetPosition(positions, symbol.Symbol)
	if err != nil {
		return decimal.Zero, err
	}
	if netAmount.IsZero() {
		return decimal.Zero, nil
	}

	liquidationPrice := decimal.Zero
	for _, brkt := range leverageBracket.Brackets {
		mmr := brkt.Mmr
		if mmr.IsZero() {
			continue
		}
		if netSide == types.PositionSideLong {
			denominator := netAmount.Mul(mmr.Sub(one))
			if denominator.IsZero() {
				continue
			}
			numerator := walletAsset.Balance.Sub(netAmount.Mul(netEntryPrice))
			liquidationPrice = numerator.Div(denominator)
		} else {
			denominator := netAmount.Mul(mmr.Add(one))
			if denominator.IsZero() {
				continue
			}
			numerator := walletAsset.Balance.Add(netAmount.Mul(netEntryPrice))
			liquidationPrice = numerator.Div(denominator)
		}
		if liquidationPrice.IsNegative() || liquidationPrice.IsZero() {
			continue
		}
		notional := netAmount.Mul(liquidationPrice)
		minNotional := brkt.MinNotional
		maxNotional := brkt.MaxNotional
		if notional.GreaterThanOrEqual(minNotional) && notional.LessThan(maxNotional) {
			break
		}
		liquidationPrice = decimal.Zero
	}

	if liquidationPrice.IsNegative() {
		liquidationPrice = decimal.Zero
	}
	if !liquidationPrice.IsZero() {
		precision := symbol.Market.PricePrecision
		if precision >= 0 {
			liquidationPrice = liquidationPrice.RoundCeil(int32(precision))
		}
	}
	return liquidationPrice, nil
}

func getNetPosition(all []*types.Position, symbol types.Symbol) (decimal.Decimal, decimal.Decimal, types.PositionSide, error) {
	netAmount := decimal.Zero
	netCost := decimal.Zero
	totalMargin := decimal.Zero
	for _, p := range all {
		if p == nil || p.Amount.IsZero() || !p.Symbol.Equal(symbol) || p.Leverage <= 0 {
			continue
		}
		signedAmount := p.Amount
		if p.Side == types.PositionSideShort {
			signedAmount = signedAmount.Neg()
		}
		netAmount = netAmount.Add(signedAmount)
		netCost = netCost.Add(signedAmount.Mul(p.EntryPrice))
		totalMargin = totalMargin.Add(p.Amount.Mul(p.EntryPrice).Div(decimal.NewFromInt(int64(p.Leverage))))
	}

	if netAmount.IsZero() {
		return decimal.Zero, decimal.Zero, types.PositionSideLong, nil
	}

	side := types.PositionSideLong
	if netAmount.IsNegative() {
		side = types.PositionSideShort
	}

	netEntryPrice := netCost.Div(netAmount)
	return netAmount.Abs(), netEntryPrice.Abs(), side, nil
}
