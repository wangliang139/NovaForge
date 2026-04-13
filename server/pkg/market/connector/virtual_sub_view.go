package connector

import (
	"context"
	"errors"
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
	// AttributeOrdersFromParent / AttributeOrderFromParent：父 multi_bot 订单归因到指定 virtual_sub（与 account 包 P2 逻辑一致）。
	// 无派发至 subID 时返回空切片 / nil，由包装层决定是否回退子表查询。
	AttributeOrdersFromParent(ctx context.Context, parentID, subID string, exchange ctypes.Exchange, symbol *ctypes.Symbol, parentOrders []*ctypes.Order) ([]*ctypes.Order, error)
	AttributeOrderFromParent(ctx context.Context, parentID, subID string, exchange ctypes.Exchange, symbol ctypes.Symbol, parentOrder *ctypes.Order) (*ctypes.Order, error)
}

// virtualSubConnectorView 在父账户真实交易所 connector 之上叠加子账户表视图：
// Account/Balance/Positions 读 reader；GetOrders/GetOrder 先拉父 connector 再在 reader 上做多 Bot 归因，无命中时回退子表。
// 其余方法透传内嵌 base。不修改各交易所 Connector 实现。
type virtualSubConnectorView struct {
	mdtypes.Connector
	reader   VirtualSubAccountReader
	subID    string
	parentID string
}

// NewVirtualSubConnectorView 用已有 GetConnector 得到的 base 包装为子账户视图 connector。
// parentID 为父 real 账户 id，用于订单归因；subID 为 virtual_sub 账户 id。
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
	parentOrders, err := v.Connector.GetOrders(ctx, symbol)
	if err != nil {
		return nil, err
	}
	attributed, err := v.reader.AttributeOrdersFromParent(ctx, v.parentID, v.subID, v.Connector.Exchange(), symbol, parentOrders)
	if err != nil {
		return nil, err
	}
	if len(attributed) > 0 {
		return attributed, nil
	}
	return v.reader.GetOpenOrders(ctx, v.subID, symbol)
}

func (v *virtualSubConnectorView) GetOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error) {
	oid := strings.TrimSpace(orderId)
	if oid == "" {
		return nil, nil
	}
	parentOrder, err := v.Connector.GetOrder(ctx, symbol, oid)
	if err != nil {
		return nil, err
	}
	attributed, err := v.reader.AttributeOrderFromParent(ctx, v.parentID, v.subID, v.Connector.Exchange(), symbol, parentOrder)
	if err != nil {
		return nil, err
	}
	if attributed != nil {
		return attributed, nil
	}
	return v.reader.GetOrder(ctx, v.subID, symbol.String(), "", oid)
}

func (v *virtualSubConnectorView) CancelOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) error {
	return v.Connector.CancelOrder(ctx, symbol, orderId)
}

func (v *virtualSubConnectorView) PlaceOrder(ctx context.Context, input mdtypes.PlaceOrderInput) (*mdtypes.PlaceOrderResult, error) {
	return nil, errors.New("place order is not supported for virtual sub account")
}