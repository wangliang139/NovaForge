package account

import (
	"context"
	"strings"

	"github.com/shopspring/decimal"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// subConnectorDataSource 为 virtualSubConnectorView 提供子账户维度的 DB 读路径。
// *Entity 实现该接口。
type subConnectorDataSource interface {
	GetAssets(ctx context.Context, accountID string) ([]*types.Asset, error)
	GetPositions(ctx context.Context, accountID string) ([]*ctypes.Position, error)
	GetOpenOrders(ctx context.Context, accountID string, symbol *ctypes.Symbol) ([]*ctypes.Order, error)
	GetOrder(ctx context.Context, accountID string, symbol string, clientOrderID, exchangeOrderID string) (*ctypes.Order, error)
}

// virtualSubConnectorView 将父账户 API connector 与「子账户表视图」解耦：
// 私有查询 Account/Balance/Positions/GetOrders/GetOrder 读子表；其余方法透传内嵌 base。
type virtualSubConnectorView struct {
	mdtypes.Connector
	src      subConnectorDataSource
	subID    string
	parentID string
}

func newVirtualSubConnectorView(base mdtypes.Connector, src subConnectorDataSource, subID, parentID string) mdtypes.Connector {
	return &virtualSubConnectorView{
		Connector: base,
		src:       src,
		subID:     subID,
		parentID:  parentID,
	}
}

func (v *virtualSubConnectorView) Account(ctx context.Context) (*ctypes.AccountBo, error) {
	pBo, err := v.Connector.Account(ctx)
	if err != nil {
		return nil, err
	}
	if pBo == nil {
		return &ctypes.AccountBo{
			Exchange: v.Connector.Exchange(),
			Uid:      v.subID,
		}, nil
	}
	return &ctypes.AccountBo{
		Exchange:        pBo.Exchange,
		Uid:             v.subID,
		IsSpotEnabled:   pBo.IsSpotEnabled,
		IsFutureEnabled: pBo.IsFutureEnabled,
	}, nil
}

func (v *virtualSubConnectorView) Balance(ctx context.Context) (*ctypes.Balance, error) {
	assets, err := v.src.GetAssets(ctx, v.subID)
	if err != nil {
		return nil, err
	}
	out := &ctypes.Balance{
		Assets: make([]*ctypes.AssetBo, 0, len(assets)),
	}
	for _, a := range assets {
		if a == nil {
			continue
		}
		out.Assets = append(out.Assets, &ctypes.AssetBo{
			AccountID:  v.subID,
			WalletType: a.WalletType,
			Code:       a.Code,
			Balance:    a.Balance,
			Locked:     a.Locked(),
			UpdatedTs:  a.UpdatedTs,
		})
	}
	return out, nil
}

func (v *virtualSubConnectorView) Positions(ctx context.Context, mt *ctypes.MarketType) ([]*ctypes.Position, error) {
	all, err := v.src.GetPositions(ctx, v.subID)
	if err != nil {
		return nil, err
	}
	if mt == nil {
		return all, nil
	}
	filtered := make([]*ctypes.Position, 0, len(all))
	for _, p := range all {
		if p != nil && p.Symbol.Type == *mt {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

func (v *virtualSubConnectorView) GetOrders(ctx context.Context, symbol *ctypes.Symbol) ([]*ctypes.Order, error) {
	return v.src.GetOpenOrders(ctx, v.subID, symbol)
}

func (v *virtualSubConnectorView) GetOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error) {
	oid := strings.TrimSpace(orderId)
	if oid == "" {
		return nil, nil
	}
	return v.src.GetOrder(ctx, v.subID, symbol.String(), "", oid)
}

func (v *virtualSubConnectorView) SymbolConfig(ctx context.Context, symbol ctypes.Symbol) (*ctypes.SymbolConfig, error) {
	return v.Connector.SymbolConfig(ctx, symbol)
}

func (v *virtualSubConnectorView) CalcOrderFee(ctx context.Context, order ctypes.Order) (*decimal.Decimal, *string, error) {
	return v.Connector.CalcOrderFee(ctx, order)
}

func (v *virtualSubConnectorView) PlaceOrder(ctx context.Context, input mdtypes.PlaceOrderInput) (*mdtypes.PlaceOrderResult, error) {
	return v.Connector.PlaceOrder(ctx, input)
}

func (v *virtualSubConnectorView) CancelOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) error {
	return v.Connector.CancelOrder(ctx, symbol, orderId)
}
