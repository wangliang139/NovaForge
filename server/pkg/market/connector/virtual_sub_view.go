package connector

import (
	"context"
	"strings"

	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

// VirtualSubAccountReader 为虚拟子账户 connector 视图提供子账户维度的读路径（通常由 account.Entity 实现）。
// 定义在 connector 包以避免 connector import account 的循环依赖。
type VirtualSubAccountReader interface {
	GetAssets(ctx context.Context, accountID string) ([]*ctypes.Asset, error)
	GetPositions(ctx context.Context, accountID string) ([]*ctypes.Position, error)
	GetOpenOrders(ctx context.Context, accountID string, symbol *ctypes.Symbol) ([]*ctypes.Order, error)
	GetOrder(ctx context.Context, accountID string, symbol string, clientOrderID, exchangeOrderID string) (*ctypes.Order, error)
}

// virtualSubConnectorView 在父账户真实交易所 connector 之上叠加子账户表视图：
// Account/Balance/Positions/GetOrders/GetOrder 读 reader；其余方法透传内嵌 base。
// 不修改各交易所 Connector 实现。
type virtualSubConnectorView struct {
	mdtypes.Connector
	reader   VirtualSubAccountReader
	subID    string
	parentID string
}

// NewVirtualSubConnectorView 用已有 GetConnector 得到的 base 包装为子账户视图 connector。
// parentID 保留供调用方溯源，当前实现未使用。
func NewVirtualSubConnectorView(base mdtypes.Connector, reader VirtualSubAccountReader, subID, parentID string) mdtypes.Connector {
	return &virtualSubConnectorView{
		Connector: base,
		reader:    reader,
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
	assets, err := v.reader.GetAssets(ctx, v.subID)
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
	all, err := v.reader.GetPositions(ctx, v.subID)
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
	return v.reader.GetOpenOrders(ctx, v.subID, symbol)
}

func (v *virtualSubConnectorView) GetOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error) {
	oid := strings.TrimSpace(orderId)
	if oid == "" {
		return nil, nil
	}
	return v.reader.GetOrder(ctx, v.subID, symbol.String(), "", oid)
}
