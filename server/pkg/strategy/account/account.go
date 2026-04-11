package account

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/strategy"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/proxy"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// LiveAccountEngine 实盘账户引擎
type LiveAccountEngine struct{}

var _ strategy.AccountEngine = (*LiveAccountEngine)(nil)

func NewLiveAccountEngine() *LiveAccountEngine {
	return &LiveAccountEngine{}
}

func (e *LiveAccountEngine) GetAsset(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string) (*ctypes.AssetBo, error) {
	if accountID == "" {
		return nil, fmt.Errorf("account id is required")
	}
	if asset == "" {
		return nil, fmt.Errorf("asset is required")
	}

	balance, err := proxy.GetBalance(ctx, accountID)
	if err != nil {
		return nil, err
	}

	if balance != nil {
		for _, a := range balance.Assets {
			if a != nil && strings.EqualFold(a.Code, asset) {
				return a, nil
			}
		}
	}

	return &ctypes.AssetBo{
		AccountID:  accountID,
		WalletType: ctypes.WalletTypeTrade,
		Code:       asset,
		Balance:    decimal.Zero,
		Locked:     decimal.Zero,
	}, nil
}

// GetBalance 批量获取账户下所有资产余额
func (e *LiveAccountEngine) GetBalance(ctx context.Context, accountID string) ([]*ctypes.AssetBo, error) {
	if accountID == "" {
		return nil, fmt.Errorf("account id is required")
	}

	balance, err := proxy.GetBalance(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if balance == nil || len(balance.Assets) == 0 {
		return []*ctypes.AssetBo{}, nil
	}
	return balance.Assets, nil
}

func (e *LiveAccountEngine) FreezeFunds(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string, amount decimal.Decimal, order *ctypes.Order) error {
	return proxy.FreezeFunds(ctx, accountID, symbol, asset, amount, order)
}

func (e *LiveAccountEngine) UnfreezeFunds(ctx context.Context, accountID string, symbol ctypes.Symbol, asset string, amount decimal.Decimal, order *ctypes.Order) error {
	return proxy.UnfreezeFunds(ctx, accountID, symbol, asset, amount, order)
}

func (e *LiveAccountEngine) GetPositions(ctx context.Context, accountID string) ([]*ctypes.Position, error) {
	if accountID == "" {
		return nil, fmt.Errorf("account id is required")
	}
	positions, err := proxy.GetPositions(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if positions == nil {
		return []*ctypes.Position{}, nil
	}
	return positions, nil
}

func (e *LiveAccountEngine) GetPosition(ctx context.Context, accountID string, symbol ctypes.Symbol, side ctypes.PositionSide) (*ctypes.Position, error) {
	positions, err := e.GetPositions(ctx, accountID)
	if err != nil {
		return nil, err
	}
	for _, p := range positions {
		if p == nil {
			continue
		}
		if p.Symbol == symbol && p.Side == side {
			return p, nil
		}
	}
	return &ctypes.Position{
		AccountID:  accountID,
		Symbol:     symbol,
		Side:       side,
		Amount:     decimal.Zero,
		EntryPrice: decimal.Zero,
	}, nil
}

func (e *LiveAccountEngine) SetLeverage(ctx context.Context, accountID string, symbol ctypes.Symbol, leverage int) error {
	_, err := proxy.SetLeverage(ctx, accountID, symbol, leverage)
	return err
}

func (e *LiveAccountEngine) GetLeverage(ctx context.Context, accountID string, symbol ctypes.Symbol) (int, error) {
	return proxy.GetLeverage(ctx, accountID, symbol)
}

func (e *LiveAccountEngine) GetSymbolConfig(ctx context.Context, accountID string, symbol ctypes.Symbol) (*ctypes.SymbolConfig, error) {
	return proxy.GetSymbolConfig(ctx, accountID, symbol)
}
