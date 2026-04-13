package account

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
)

func TestEntityImplementsSubConnectorDataSource(t *testing.T) {
	var _ subConnectorDataSource = (*Entity)(nil)
}

// stubBase 实现 mdtypes.Connector，避免测试依赖 connector 工厂与全局 KV。
type stubBase struct {
	parentAccount *ctypes.AccountBo
	isPrivate     bool
}

func (stubBase) Exchange() ctypes.Exchange { return ctypes.ExchangeBinance }

func (stubBase) Subscribe(ctx context.Context, selector ctypes.StreamSelector) (*mdtypes.StreamHandle, error) {
	return nil, nil
}

func (stubBase) Supports(selector ctypes.StreamSelector) bool { return false }

func (stubBase) GetMarkets(ctx context.Context, tps []ctypes.MarketType) ([]*ctypes.Market, error) {
	return nil, nil
}

func (stubBase) GetMarket(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Market, error) {
	return nil, nil
}

func (stubBase) Prices(ctx context.Context, marketType *ctypes.MarketType) ([]*ctypes.Price, error) {
	return nil, nil
}

func (stubBase) Price(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Price, error) {
	return nil, nil
}

func (stubBase) BookPrices(ctx context.Context, marketType *ctypes.MarketType) ([]*ctypes.BookPrice, error) {
	return nil, nil
}

func (stubBase) BookPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.BookPrice, error) {
	return nil, nil
}

func (stubBase) MarkPrices(ctx context.Context) ([]*ctypes.MarkPrice, error) { return nil, nil }

func (stubBase) MarkPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.MarkPrice, error) {
	return nil, nil
}

func (stubBase) IndexPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.IndexPrice, error) {
	return nil, nil
}

func (stubBase) IndexComponent(ctx context.Context, symbol ctypes.Symbol) (*ctypes.IndexComponent, error) {
	return nil, nil
}

func (stubBase) Ticker(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Ticker, error) { return nil, nil }

func (stubBase) Trades(ctx context.Context, symbol ctypes.Symbol, limit int) ([]*ctypes.Trade, error) {
	return nil, nil
}

func (stubBase) Depth(ctx context.Context, symbol ctypes.Symbol, limit int) (*ctypes.OrderBook, error) {
	return nil, nil
}

func (stubBase) Klines(ctx context.Context, symbol ctypes.Symbol, interval ctypes.Interval, limit int) ([]*ctypes.Kline, error) {
	return nil, nil
}

func (stubBase) HisKlines(ctx context.Context, symbol ctypes.Symbol, interval ctypes.Interval, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.Kline, error) {
	return nil, nil
}

func (stubBase) FundingRate(ctx context.Context, symbol ctypes.Symbol) (*ctypes.FundingRate, error) {
	return nil, nil
}

func (stubBase) HisFundingRates(ctx context.Context, symbol ctypes.Symbol, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.FundingRate, error) {
	return nil, nil
}

func (stubBase) OpenInterest(ctx context.Context, symbol ctypes.Symbol) (decimal.Decimal, error) {
	return decimal.Zero, nil
}

func (s stubBase) IsPrivate() bool { return s.isPrivate }

func (s stubBase) Account(ctx context.Context) (*ctypes.AccountBo, error) {
	if s.parentAccount != nil {
		return s.parentAccount, nil
	}
	return &ctypes.AccountBo{
		Exchange:        ctypes.ExchangeBinance,
		Uid:             "parent-exchange-uid",
		IsSpotEnabled:   true,
		IsFutureEnabled: true,
	}, nil
}

func (stubBase) Balance(ctx context.Context) (*ctypes.Balance, error) { return &ctypes.Balance{}, nil }

func (stubBase) Positions(ctx context.Context, mt *ctypes.MarketType) ([]*ctypes.Position, error) {
	return nil, nil
}

func (stubBase) SymbolConfig(ctx context.Context, symbol ctypes.Symbol) (*ctypes.SymbolConfig, error) {
	return nil, nil
}

func (stubBase) GetOrders(ctx context.Context, symbol *ctypes.Symbol) ([]*ctypes.Order, error) { return nil, nil }

func (stubBase) GetOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error) {
	return nil, nil
}

func (stubBase) CalcOrderFee(ctx context.Context, order ctypes.Order) (*decimal.Decimal, *string, error) {
	return nil, nil, nil
}

func (stubBase) PlaceOrder(ctx context.Context, input mdtypes.PlaceOrderInput) (*mdtypes.PlaceOrderResult, error) {
	return nil, nil
}

func (stubBase) CancelOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) error { return nil }

func (stubBase) SetLeverage(ctx context.Context, symbol ctypes.Symbol, leverage int) (int, error) {
	return leverage, nil
}

func (stubBase) GetLeverageBracket(ctx context.Context, symbol ctypes.Symbol, markPrice decimal.Decimal) (*ctypes.LeverageBracket, error) {
	return nil, nil
}

type mockSubConnectorSrc struct {
	lastAccountID string

	assets    []*types.Asset
	positions []*ctypes.Position
	open      []*ctypes.Order
	oneOrder  *ctypes.Order
	oneErr    error
}

func (m *mockSubConnectorSrc) GetAssets(ctx context.Context, accountID string) ([]*types.Asset, error) {
	m.lastAccountID = accountID
	return m.assets, nil
}

func (m *mockSubConnectorSrc) GetPositions(ctx context.Context, accountID string) ([]*ctypes.Position, error) {
	m.lastAccountID = accountID
	return m.positions, nil
}

func (m *mockSubConnectorSrc) GetOpenOrders(ctx context.Context, accountID string, symbol *ctypes.Symbol) ([]*ctypes.Order, error) {
	m.lastAccountID = accountID
	return m.open, nil
}

func (m *mockSubConnectorSrc) GetOrder(ctx context.Context, accountID string, symbol string, clientOrderID, exchangeOrderID string) (*ctypes.Order, error) {
	m.lastAccountID = accountID
	if m.oneErr != nil {
		return nil, m.oneErr
	}
	return m.oneOrder, nil
}

func TestVirtualSubConnectorView_BalanceUsesSubAccount(t *testing.T) {
	base := stubBase{isPrivate: true}
	src := &mockSubConnectorSrc{
		assets: []*types.Asset{
			{
				AccountID:  "sub-1",
				WalletType: ctypes.WalletTypeSpot,
				Code:       "USDT",
				Balance:    decimal.RequireFromString("10"),
				Frozened:   decimal.RequireFromString("1"),
				UpdatedTs:  time.Now(),
			},
		},
	}
	subID := "sub-1"
	v := newVirtualSubConnectorView(base, src, subID, "parent-1")

	bal, err := v.Balance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if src.lastAccountID != subID {
		t.Fatalf("GetAssets accountID: got %q want %q", src.lastAccountID, subID)
	}
	if bal == nil || len(bal.Assets) != 1 {
		t.Fatalf("balance assets: %+v", bal)
	}
	if bal.Assets[0].AccountID != subID || bal.Assets[0].Code != "USDT" {
		t.Fatalf("asset bo: %+v", bal.Assets[0])
	}
	if !bal.Assets[0].Locked.Equal(decimal.RequireFromString("1")) {
		t.Fatalf("locked: %s", bal.Assets[0].Locked)
	}
}

func TestVirtualSubConnectorView_GetOrdersUsesSubAccount(t *testing.T) {
	base := stubBase{isPrivate: true}
	src := &mockSubConnectorSrc{open: []*ctypes.Order{{OrderID: "o1"}}}
	v := newVirtualSubConnectorView(base, src, "sub-2", "parent-1")
	out, err := v.GetOrders(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if src.lastAccountID != "sub-2" {
		t.Fatalf("account: got %q", src.lastAccountID)
	}
	if len(out) != 1 || out[0].OrderID != "o1" {
		t.Fatalf("orders: %+v", out)
	}
}

func TestVirtualSubConnectorView_GetOrderUsesSubAccount(t *testing.T) {
	base := stubBase{isPrivate: true}
	sym := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)
	src := &mockSubConnectorSrc{
		oneOrder: &ctypes.Order{OrderID: "ex-9", Symbol: sym},
	}
	v := newVirtualSubConnectorView(base, src, "sub-3", "parent-1")
	o, err := v.GetOrder(context.Background(), sym, "ex-9")
	if err != nil {
		t.Fatal(err)
	}
	if src.lastAccountID != "sub-3" {
		t.Fatalf("account: got %q", src.lastAccountID)
	}
	if o == nil || o.OrderID != "ex-9" {
		t.Fatalf("order: %+v", o)
	}
}

func TestVirtualSubConnectorView_PositionsFiltersMarketType(t *testing.T) {
	base := stubBase{isPrivate: true}
	spot := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)
	fut := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeFuture)
	src := &mockSubConnectorSrc{
		positions: []*ctypes.Position{
			{Symbol: spot, Amount: decimal.NewFromInt(1)},
			{Symbol: fut, Amount: decimal.NewFromInt(2)},
		},
	}
	v := newVirtualSubConnectorView(base, src, "sub-4", "parent-1")
	mt := ctypes.MarketTypeFuture
	out, err := v.Positions(context.Background(), &mt)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Symbol.Type != ctypes.MarketTypeFuture {
		t.Fatalf("got %+v", out)
	}
}

func TestVirtualSubConnectorView_PriceDelegatesToBase(t *testing.T) {
	base := stubBase{isPrivate: true}
	src := &mockSubConnectorSrc{}
	v := newVirtualSubConnectorView(base, src, "sub-5", "parent-1")
	sym := ctypes.NewSymbol("BTC", "USDT", ctypes.MarketTypeSpot)
	_, err := v.Price(context.Background(), sym)
	if err != nil {
		t.Fatal(err)
	}
}

func TestVirtualSubConnectorView_AccountOverridesUID(t *testing.T) {
	base := stubBase{
		isPrivate: true,
		parentAccount: &ctypes.AccountBo{
			Exchange:        ctypes.ExchangeBinance,
			Uid:             "parent-exchange-uid",
			IsSpotEnabled:   true,
			IsFutureEnabled: false,
		},
	}
	src := &mockSubConnectorSrc{}
	v := newVirtualSubConnectorView(base, src, "sub-6", "parent-1")
	bo, err := v.Account(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bo == nil {
		t.Fatal("nil AccountBo")
	}
	if bo.Uid != "sub-6" {
		t.Fatalf("Uid: got %q want sub-6", bo.Uid)
	}
	if !bo.IsSpotEnabled || bo.IsFutureEnabled {
		t.Fatalf("flags from parent: %+v", bo)
	}
}
