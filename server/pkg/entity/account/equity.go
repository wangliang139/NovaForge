package account

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/precision"
	"github.com/wangliang139/NovaForge/server/pkg/repos/equity"
	"github.com/wangliang139/NovaForge/server/pkg/repos/symbol_equity"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
)

func (e *Entity) CreateEquity(ctx context.Context, accountID string, ts time.Time, notional, unrealized decimal.Decimal) (*equity.Equity, error) {
	if accountID == "" {
		return nil, fmt.Errorf("accountID is required")
	}
	return e.db.EquityRepo.CreateEquity(ctx, equity.CreateEquityParams{
		AccountID:        accountID,
		Ts:               ts,
		Notional:         precision.DecimalToPgNumeric(notional),
		UnrealizedProfit: precision.DecimalToPgNumeric(unrealized),
	})
}

func (e *Entity) GetEquityBeforeTs(ctx context.Context, accountID string, ts time.Time) (*ctypes.Equity, error) {
	if accountID == "" {
		return nil, fmt.Errorf("accountID is required")
	}
	row, err := e.db.EquityRepo.GetEquityBeforeTs(ctx, equity.GetEquityBeforeTsParams{
		AccountID: accountID,
		Ts:        ts,
	})
	if err != nil {
		return nil, err
	}
	return &ctypes.Equity{
		ID:               row.ID,
		AccountID:        row.AccountID,
		Ts:               row.Ts,
		Notional:         utils.Decimal.PgNumericToDecimal(row.Notional),
		UnRealizedProfit: utils.Decimal.PgNumericToDecimal(row.UnrealizedProfit),
		CreatedAt:        row.CreatedAt,
	}, nil
}

func (e *Entity) QueryEquities(ctx context.Context, accountID string, startTs, endTs time.Time) ([]*ctypes.Equity, error) {
	if accountID == "" {
		return nil, fmt.Errorf("accountID is required")
	}
	rows, err := e.db.EquityRepo.ListEquityByAccountAndRange(ctx, equity.ListEquityByAccountAndRangeParams{
		AccountID: accountID,
		Ts:        startTs,
		Ts_2:      endTs,
	})
	if err != nil {
		return nil, err
	}
	result := make([]*ctypes.Equity, 0, len(rows))
	for _, row := range rows {
		item := row
		result = append(result, &ctypes.Equity{
			ID:               item.ID,
			AccountID:        item.AccountID,
			Ts:               item.Ts,
			Notional:         utils.Decimal.PgNumericToDecimal(item.Notional),
			UnRealizedProfit: utils.Decimal.PgNumericToDecimal(item.UnrealizedProfit),
			CreatedAt:        item.CreatedAt,
		})
	}
	return result, nil
}
func (e *Entity) CalculateAccountEquity(ctx context.Context, accountID string, exchange ctypes.Exchange) (decimal.Decimal, decimal.Decimal, error) {
	assets, err := e.GetAssets(ctx, accountID)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	positions, err := e.GetPositions(ctx, accountID)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	return e.calculateAccountEquity(ctx, exchange, assets, positions)
}

func (e *Entity) calculateAccountEquity(ctx context.Context, exchange ctypes.Exchange, assets []*types.Asset, positions []*ctypes.Position) (decimal.Decimal, decimal.Decimal, error) {
	if len(assets) == 0 && len(positions) == 0 {
		return decimal.Zero, decimal.Zero, nil
	}

	provider := e.engine.GetMarketProvider()
	prices, err := provider.GetPrices(ctx, exchange)
	if err != nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("get ticker prices: %w", err)
	}

	priceMap := make(map[string]decimal.Decimal)
	for _, tp := range prices {
		if tp.Symbol.Quote != "USDT" {
			continue
		}
		priceMap[tp.Symbol.Base] = tp.Price
	}

	assetNotional := decimal.Zero
	for _, asset := range assets {
		if asset == nil {
			continue
		}
		if asset.IsEmpty() {
			continue
		}
		if asset.Code == "USDT" {
			assetNotional = assetNotional.Add(asset.Balance)
			continue
		}
		if price, ok := priceMap[asset.Code]; ok {
			assetNotional = assetNotional.Add(asset.Balance.Mul(price))
		}
	}

	unrealized := decimal.Zero
	if len(positions) > 0 {
		markPrices, err := provider.GetMarkPrices(ctx, exchange)
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		markPriceMap := make(map[string]decimal.Decimal)
		for _, mp := range markPrices {
			markPriceMap[mp.Symbol.String()] = mp.MarkPrice
		}
		for _, pos := range positions {
			if pos == nil || pos.Amount.IsZero() {
				continue
			}
			markPrice, ok := markPriceMap[pos.Symbol.String()]
			if !ok {
				continue
			}
			var upl decimal.Decimal
			if pos.Side == ctypes.PositionSideLong {
				upl = pos.Amount.Mul(markPrice.Sub(pos.EntryPrice))
			} else {
				upl = pos.Amount.Mul(pos.EntryPrice.Sub(markPrice))
			}

			if pos.Symbol.Quote == "USDT" {
				unrealized = unrealized.Add(upl)
				continue
			}
			if price, ok := priceMap[pos.Symbol.Quote]; ok {
				unrealized = unrealized.Add(upl.Mul(price))
			}
		}
	}

	return assetNotional.Add(unrealized), unrealized, nil
}

// refreshSymbolEquity 刷新账户各标的权益快照（按持仓计算，与 RefreshAccountEquity 一起触发）
func (e *Entity) refreshSymbolEquity(ctx context.Context, accountID, exchangeStr string, ts time.Time) error {
	exchange, err := ctypes.ParseExchange(exchangeStr)
	if err != nil {
		return fmt.Errorf("parse exchange: %w", err)
	}
	positions, err := e.GetPositions(ctx, accountID)
	if err != nil {
		return fmt.Errorf("get positions: %w", err)
	}
	if len(positions) == 0 {
		return nil
	}

	provider := e.engine.GetMarketProvider()
	prices, err := provider.GetPrices(ctx, exchange)
	if err != nil {
		return fmt.Errorf("get book prices: %w", err)
	}
	priceMap := make(map[string]decimal.Decimal)
	for _, tp := range prices {
		if tp.Symbol.Quote != "USDT" {
			continue
		}
		priceMap[tp.Symbol.Base] = tp.Price
	}
	markPrices, err := provider.GetMarkPrices(ctx, exchange)
	if err != nil {
		return fmt.Errorf("get mark prices: %w", err)
	}
	markPriceMap := make(map[string]decimal.Decimal)
	for _, mp := range markPrices {
		markPriceMap[mp.Symbol.String()] = mp.MarkPrice
	}

	baseCurrency := "USDT"
	for _, pos := range positions {
		if pos == nil || pos.Amount.IsZero() {
			continue
		}
		symStr := pos.Symbol.String()
		notional := pos.Notional
		if notional.IsZero() {
			markPrice, ok := markPriceMap[symStr]
			if !ok {
				continue
			}
			notional = pos.Amount.Mul(markPrice)
			if pos.Symbol.Quote != "USDT" {
				if price, ok := priceMap[pos.Symbol.Quote]; ok {
					notional = notional.Mul(price)
				}
			}
		}
		_, err := e.db.SymbolEquityRepo.UpsertSymbolEquity(ctx, symbol_equity.UpsertSymbolEquityParams{
			AccountID:    accountID,
			Exchange:     exchangeStr,
			Symbol:       symStr,
			NetValue:     precision.DecimalToPgNumeric(notional),
			BaseCurrency: baseCurrency,
			Ts:           ts,
		})
		if err != nil {
			return fmt.Errorf("upsert symbol equity %s: %w", symStr, err)
		}
	}
	return nil
}
