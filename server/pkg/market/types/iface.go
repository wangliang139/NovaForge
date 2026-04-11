package types

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
)

type Connector interface {
	// streaming methods
	Subscribe(ctx context.Context, selector ctypes.StreamSelector) (*StreamHandle, error)
	Supports(selector ctypes.StreamSelector) bool

	// public methods
	Exchange() ctypes.Exchange
	GetMarkets(ctx context.Context, tps []ctypes.MarketType) ([]*ctypes.Market, error)
	GetMarket(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Market, error)

	// 最新成交价格
	Prices(ctx context.Context, marketType *ctypes.MarketType) ([]*ctypes.Price, error)
	Price(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Price, error)
	// 盘口价格
	BookPrices(ctx context.Context, marketType *ctypes.MarketType) ([]*ctypes.BookPrice, error)
	BookPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.BookPrice, error)
	// 标记价格
	MarkPrices(ctx context.Context) ([]*ctypes.MarkPrice, error)
	MarkPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.MarkPrice, error)
	// 指数价格
	IndexPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.IndexPrice, error)
	// 指数构成
	IndexComponent(ctx context.Context, symbol ctypes.Symbol) (*ctypes.IndexComponent, error)
	Ticker(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Ticker, error)
	Trades(ctx context.Context, symbol ctypes.Symbol, limit int) ([]*ctypes.Trade, error)
	Depth(ctx context.Context, symbol ctypes.Symbol, limit int) (*ctypes.OrderBook, error)
	Klines(ctx context.Context, symbol ctypes.Symbol, interval ctypes.Interval, limit int) ([]*ctypes.Kline, error)
	HisKlines(ctx context.Context, symbol ctypes.Symbol, interval ctypes.Interval, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.Kline, error)
	// 资金费率
	FundingRate(ctx context.Context, symbol ctypes.Symbol) (*ctypes.FundingRate, error)
	HisFundingRates(ctx context.Context, symbol ctypes.Symbol, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.FundingRate, error)
	// 未平仓合约数
	OpenInterest(ctx context.Context, symbol ctypes.Symbol) (decimal.Decimal, error)

	// private methods
	IsPrivate() bool
	Account(ctx context.Context) (*ctypes.AccountBo, error)
	Balance(ctx context.Context) (*ctypes.Balance, error)
	Positions(ctx context.Context, mt *ctypes.MarketType) ([]*ctypes.Position, error)
	SymbolConfig(ctx context.Context, symbol ctypes.Symbol) (*ctypes.SymbolConfig, error)
	GetOrders(ctx context.Context, symbol *ctypes.Symbol) ([]*ctypes.Order, error)
	// GetHisOrders 底层接口参数不统一，且跨接口 limit 不好实现，先注释掉
	// GetHisOrders(ctx context.Context, symbol ctypes.Symbol, startTs time.Time, endTs time.Time, limit int) ([]*ctypes.Order, error)
	GetOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error)
	// CalcOrderFee 根据订单信息计算手续费
	CalcOrderFee(ctx context.Context, order ctypes.Order) (*decimal.Decimal, *string, error)

	// trade methods
	PlaceOrder(ctx context.Context, input PlaceOrderInput) (*PlaceOrderResult, error)
	CancelOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) error

	SetLeverage(ctx context.Context, symbol ctypes.Symbol, leverage int) (int, error)
	GetLeverageBracket(ctx context.Context, symbol ctypes.Symbol, markPrice decimal.Decimal) (*ctypes.LeverageBracket, error)
}
