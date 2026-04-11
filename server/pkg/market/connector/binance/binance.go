package binance

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	binance "github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/common"
	futures "github.com/adshao/go-binance/v2/futures"
	portfolio "github.com/adshao/go-binance/v2/portfolio"
	"github.com/adshao/go-binance/v2/portfolio_pro"
	portfolioPro "github.com/adshao/go-binance/v2/portfolio_pro"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/common/calculator"
	"github.com/wangliang139/NovaForge/server/pkg/market/connector/cache"
	"github.com/wangliang139/NovaForge/server/pkg/market/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
	"github.com/wangliang139/mow/number"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/sync/errgroup"
)

var (
	DefaultAdjustedFundingRateCap       = "0.02"
	DefaultAdjustedFundingRateFloor     = "-0.02"
	DefaultFundingIntervalHours     int = 8
)

type Config struct {
	UseDemo  bool
	ProxyURL string
	Buffer   int
}

func (c *Config) applyDefaults() {
	if c.Buffer <= 0 {
		c.Buffer = 256
	}
}

type Connector struct {
	cfg Config

	account  *types.ApiAccount
	exchange ctypes.Exchange

	mode  types.ConnectMode
	mu    sync.RWMutex
	cache *cache.Cache

	timeOffset int64

	spotClient         *binance.Client
	futureClient       *futures.Client
	portfolioClient    *portfolio.Client
	portfolioProClient *portfolioPro.Client

	muMarkets sync.Mutex
	markets   map[string]ctypes.Symbol

	isPortfolioMode *bool
}

var _ types.Connector = &Connector{}

func New(cfg Config, account *types.ApiAccount) (*Connector, error) {
	cfg.applyDefaults()

	exchange := ctypes.ExchangeBinance
	if cfg.UseDemo {
		exchange = ctypes.ExchangeBinanceTest
	}

	var apiKey, apiSecret string
	if account != nil {
		apiKey = account.ApiKey
		apiSecret = account.ApiSecret
		if account.DescryptFn != nil {
			var err error
			apiSecret, err = account.DescryptFn(apiSecret)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt api secret: %w", err)
			}
		}
	}

	mode := types.ConnectModePublic
	if account != nil {
		mode = types.ConnectModePrivate
	}

	keyType := common.KeyTypeHmac
	if account != nil {
		switch account.AuthAlgorithm {
		case "hmac":
			keyType = common.KeyTypeHmac
		case "ed25519":
			keyType = common.KeyTypeEd25519
		case "rsa":
			keyType = common.KeyTypeRsa
		}
	}

	transport := http.DefaultTransport

	if cfg.ProxyURL != "" {
		u, err := url.Parse(cfg.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse proxy url: %w", err)
		}
		transport = &http.Transport{
			Proxy: http.ProxyURL(u),
		}
	}
	httpClient := &http.Client{
		Transport: otelhttp.NewTransport(transport),
	}
	c := &Connector{
		cfg:      cfg,
		exchange: exchange,
		mode:     mode,
		account:  account,
		cache:    cache.NewCache(5*time.Minute, 10*time.Minute),
	}

	if cfg.ProxyURL != "" {
		c.spotClient = binance.NewProxiedClient(apiKey, apiSecret, cfg.ProxyURL)
		c.futureClient = futures.NewProxiedClient(apiKey, apiSecret, cfg.ProxyURL)
		c.portfolioClient = portfolio.NewProxiedClient(apiKey, apiSecret, cfg.ProxyURL)
		c.portfolioProClient = portfolioPro.NewProxiedClient(apiKey, apiSecret, cfg.ProxyURL)
	} else {
		c.spotClient = binance.NewClient(apiKey, apiSecret)
		c.futureClient = futures.NewClient(apiKey, apiSecret)
		c.portfolioClient = portfolio.NewClient(apiKey, apiSecret)
		c.portfolioProClient = portfolioPro.NewClient(apiKey, apiSecret)
	}

	c.spotClient.KeyType = keyType
	c.spotClient.HTTPClient = httpClient
	c.futureClient.KeyType = keyType
	c.futureClient.HTTPClient = httpClient
	c.portfolioClient.KeyType = keyType
	c.portfolioClient.HTTPClient = httpClient
	c.portfolioProClient.KeyType = keyType
	c.portfolioProClient.HTTPClient = httpClient

	if cfg.UseDemo {
		c.spotClient.SetUseDemo()
		c.futureClient.SetUseDemo()
	}

	c.SetupCache()
	return c, nil
}

func (c *Connector) IsPrivate() bool {
	return c.mode == types.ConnectModePrivate
}

func (c *Connector) PlaceOrder(ctx context.Context, input types.PlaceOrderInput) (*types.PlaceOrderResult, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}
	if !input.Symbol.IsValid() {
		return nil, fmt.Errorf("invalid symbol: %s", input.Symbol)
	}
	if !input.OrderType.Valid() {
		return nil, fmt.Errorf("invalid order type: %s", input.OrderType)
	}

	symbolStr := FormatSymbol(input.Symbol)
	switch input.Symbol.Type {
	case ctypes.MarketTypeSpot:
		var side binance.SideType
		if input.IsBuy {
			side = binance.SideTypeBuy
		} else {
			side = binance.SideTypeSell
		}
		var ot binance.OrderType
		switch input.OrderType {
		case ctypes.OrderTypeMarket:
			ot = binance.OrderTypeMarket
		case ctypes.OrderTypeLimit:
			ot = binance.OrderTypeLimit
		default:
			return nil, fmt.Errorf("unsupported order type: %s", input.OrderType)
		}
		svc := c.spotClient.NewCreateOrderService().Symbol(symbolStr).Side(side).Type(ot)
		if input.ClientOrderID != nil && strings.TrimSpace(input.ClientOrderID.String()) != "" {
			svc = svc.NewClientOrderID(strings.TrimSpace(input.ClientOrderID.String()))
		}

		if input.OrderType == ctypes.OrderTypeLimit {
			if input.Price == nil || !input.Price.GreaterThan(decimal.Zero) {
				return nil, fmt.Errorf("price must be > 0 for limit order")
			}
			if input.Quantity == nil || !input.Quantity.GreaterThan(decimal.Zero) {
				return nil, fmt.Errorf("quantity must be > 0 for limit order")
			}
			svc = svc.Price(input.Price.String()).Quantity(input.Quantity.String())
			// Binance spot limit 需要 timeInForce；默认 GTC
			tif := binance.TimeInForceTypeGTC
			if input.TimeInForce != nil {
				tif = binance.TimeInForceType(strings.ToUpper(input.TimeInForce.String()))
			}
			svc = svc.TimeInForce(tif)
		} else {
			// market order
			if input.Quantity != nil {
				if !input.Quantity.GreaterThan(decimal.Zero) {
					return nil, fmt.Errorf("quantity must be > 0")
				}
				svc = svc.Quantity(input.Quantity.String())
			} else if input.QuoteQty != nil {
				if !input.QuoteQty.GreaterThan(decimal.Zero) {
					return nil, fmt.Errorf("quote_qty must be > 0")
				}
				svc = svc.QuoteOrderQty(input.QuoteQty.String())
			} else {
				return nil, fmt.Errorf("quantity or quote_qty is required")
			}
		}

		resp, err := svc.Do(ctx)
		if err != nil {
			return nil, err
		}
		status, ok := MapOrderStatus2Types[string(resp.Status)]
		if !ok {
			status = ctypes.OrderStatusNew
		}
		return &types.PlaceOrderResult{
			OrderID:       ctypes.OrderId(strconv.FormatInt(resp.OrderID, 10)),
			ClientOrderID: ctypes.OrderId(resp.ClientOrderID),
			Status:        status,
		}, nil

	case ctypes.MarketTypeFuture:
		if input.Quantity == nil || !input.Quantity.GreaterThan(decimal.Zero) {
			return nil, fmt.Errorf("quantity must be > 0")
		}

		isLimit := input.OrderType == ctypes.OrderTypeLimit
		if isLimit && (input.Price == nil || !input.Price.GreaterThan(decimal.Zero)) {
			return nil, fmt.Errorf("price must be > 0 for limit order")
		}

		// hedge mode 下 positionSide 必传；这里按请求 Side 设置（LONG/SHORT）
		positionSide := ctypes.PositionSideLong
		if input.Side != "" {
			positionSide = input.Side
		}
		if positionSide != ctypes.PositionSideLong && positionSide != ctypes.PositionSideShort {
			return nil, fmt.Errorf("invalid position side: %s", positionSide)
		}

		if c.IsPortfolioMode() {
			var side portfolio.SideType
			if input.IsBuy {
				side = portfolio.SideTypeBuy
			} else {
				side = portfolio.SideTypeSell
			}
			var ps portfolio.PositionSideType
			if positionSide == ctypes.PositionSideShort {
				ps = portfolio.PositionSideTypeShort
			} else {
				ps = portfolio.PositionSideTypeLong
			}
			var ot portfolio.OrderType
			switch input.OrderType {
			case ctypes.OrderTypeMarket:
				ot = portfolio.OrderTypeMarket
			case ctypes.OrderTypeLimit:
				ot = portfolio.OrderTypeLimit
			default:
				return nil, fmt.Errorf("unsupported order type: %s", input.OrderType)
			}

			svc := c.portfolioClient.NewUMOrderService().
				Symbol(symbolStr).
				Side(side).
				PositionSide(ps).
				Type(ot).
				Quantity(input.Quantity.String())
			if input.ClientOrderID != nil && strings.TrimSpace(input.ClientOrderID.String()) != "" {
				svc = svc.NewClientOrderID(strings.TrimSpace(input.ClientOrderID.String()))
			}

			if isLimit {
				tif := portfolio.TimeInForceTypeGTC
				if input.TimeInForce != nil {
					tif = portfolio.TimeInForceType(strings.ToUpper(input.TimeInForce.String()))
				}
				svc = svc.TimeInForce(tif).Price(input.Price.String())
			}
			if input.ReduceOnly != nil && ctypes.IsReducePosition(positionSide, input.IsBuy) {
				svc = svc.ReduceOnly(*input.ReduceOnly)
			}
			resp, err := svc.Do(ctx)
			if err != nil {
				return nil, err
			}
			status, ok := MapOrderStatus2Types[string(resp.Status)]
			if !ok {
				status = ctypes.OrderStatusNew
			}
			return &types.PlaceOrderResult{
				OrderID:       ctypes.OrderId(strconv.FormatInt(resp.OrderID, 10)),
				ClientOrderID: ctypes.OrderId(resp.ClientOrderID),
				Status:        status,
			}, nil
		}

		var side futures.SideType
		if input.IsBuy {
			side = futures.SideTypeBuy
		} else {
			side = futures.SideTypeSell
		}
		var ps futures.PositionSideType
		if positionSide == ctypes.PositionSideShort {
			ps = futures.PositionSideTypeShort
		} else {
			ps = futures.PositionSideTypeLong
		}
		var ot futures.OrderType
		switch input.OrderType {
		case ctypes.OrderTypeMarket:
			ot = futures.OrderTypeMarket
		case ctypes.OrderTypeLimit:
			ot = futures.OrderTypeLimit
		default:
			return nil, fmt.Errorf("unsupported order type: %s", input.OrderType)
		}

		svc := c.futureClient.NewCreateOrderService().
			Symbol(symbolStr).
			Side(side).
			PositionSide(ps).
			Type(ot).
			Quantity(input.Quantity.String())
		if input.ClientOrderID != nil && strings.TrimSpace(input.ClientOrderID.String()) != "" {
			svc = svc.NewClientOrderID(strings.TrimSpace(input.ClientOrderID.String()))
		}

		if isLimit {
			tif := futures.TimeInForceTypeGTC
			if input.TimeInForce != nil {
				tif = futures.TimeInForceType(strings.ToUpper(input.TimeInForce.String()))
			}
			svc = svc.TimeInForce(tif).Price(input.Price.String())
		}
		if input.ReduceOnly != nil && ctypes.IsReducePosition(positionSide, input.IsBuy) {
			isDualSidePosition, err := c.IsDualSidePositionMode(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get position mode")
			}
			if !isDualSidePosition {
				svc = svc.ReduceOnly(*input.ReduceOnly)
			}
		}
		// if input.ClosePosition != nil {
		// 	svc = svc.ClosePosition(*input.ClosePosition)
		// }

		resp, err := svc.Do(ctx)
		if err != nil {
			return nil, err
		}
		status, ok := MapOrderStatus2Types[string(resp.Status)]
		if !ok {
			status = ctypes.OrderStatusNew
		}
		return &types.PlaceOrderResult{
			OrderID:       ctypes.OrderId(strconv.FormatInt(resp.OrderID, 10)),
			ClientOrderID: ctypes.OrderId(resp.ClientOrderID),
			Status:        status,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported market type: %s", input.Symbol.Type)
	}
}

func (c *Connector) CancelOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) error {
	if !c.IsPrivate() {
		return fmt.Errorf("connector is not private mode")
	}
	if !symbol.IsValid() {
		return fmt.Errorf("invalid symbol: %s", symbol)
	}
	orderId = strings.TrimSpace(orderId)
	if orderId == "" {
		return fmt.Errorf("order_id is required")
	}

	symbolStr := FormatSymbol(symbol)
	orderIDInt, err := strconv.ParseInt(orderId, 10, 64)
	if err != nil {
		return fmt.Errorf("parse order_id: %w", err)
	}

	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		_, err := c.spotClient.NewCancelOrderService().Symbol(symbolStr).OrderID(orderIDInt).Do(ctx)
		return err
	case ctypes.MarketTypeFuture:
		if c.IsPortfolioMode() {
			_, err := c.portfolioClient.NewUMCancelOrderService().Symbol(symbolStr).OrderID(orderIDInt).Do(ctx)
			return err
		}
		_, err := c.futureClient.NewCancelOrderService().Symbol(symbolStr).OrderID(orderIDInt).Do(ctx)
		return err
	default:
		return fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
}

func (c *Connector) IsPortfolioMode() bool {
	if !c.IsPrivate() {
		return false
	}

	if c.isPortfolioMode != nil {
		return *c.isPortfolioMode
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isPortfolioMode != nil {
		return *c.isPortfolioMode
	}

	account, err := c.getAccountInfo(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("failed to get account info")
		return false
	}
	c.isPortfolioMode = &account.IsPortfolioMode
	return *c.isPortfolioMode
}

func (c *Connector) IsDualSidePositionMode(ctx context.Context) (bool, error) {
	if !c.IsPrivate() {
		return false, fmt.Errorf("connector is not private mode")
	}

	if c.IsPortfolioMode() {
		positionMode, err := fetchWithType[*portfolio.PositionMode](c, ctx, CacheKeyUMPositionMode)
		if err != nil {
			return false, err
		}
		if positionMode == nil {
			return false, fmt.Errorf("position mode is nil")
		}
		return positionMode.DualSidePosition, nil
	} else {
		positionMode, err := fetchWithType[*futures.PositionMode](c, ctx, CacheKeyFuturePositionMode)
		if err != nil {
			return false, err
		}
		if positionMode == nil {
			return false, fmt.Errorf("position mode is nil")
		}
		return positionMode.DualSidePosition, nil
	}
}

func (c *Connector) Exchange() ctypes.Exchange {
	return c.exchange
}

func (c *Connector) ParseSymbol(ctx context.Context, symbol string, tp ctypes.MarketType) (ctypes.Symbol, error) {
	c.muMarkets.Lock()
	if c.markets == nil {
		markets, err := c.GetMarkets(ctx, ctypes.AllMarketTypes())
		if err != nil {
			c.muMarkets.Unlock()
			return ctypes.Symbol{}, fmt.Errorf("failed to load markets: %w", err)
		}
		c.markets = make(map[string]ctypes.Symbol)
		for _, market := range markets {
			key := FormatSymbol(market.Symbol)
			c.markets[key] = market.Symbol
		}
	}
	markets := c.markets
	c.muMarkets.Unlock()

	sym, ok := markets[symbol]
	if !ok {
		return ctypes.Symbol{}, fmt.Errorf("symbol not found: %s", symbol)
	}
	return ctypes.Symbol{
		Base:  sym.Base,
		Quote: sym.Quote,
		Type:  tp,
	}, nil
}

func (c *Connector) Supports(selector ctypes.StreamSelector) bool {
	switch selector.Stream {
	case
		ctypes.StreamTypeTicker,
		ctypes.StreamTypeTrade,
		ctypes.StreamTypeDepth,
		ctypes.StreamTypeKline,
		ctypes.StreamTypeMarkPrice,
		ctypes.StreamTypeAccount,
		ctypes.StreamTypeAccountRaw:
		return true
	default:
		return false
	}
}

func (c *Connector) Subscribe(ctx context.Context, selector ctypes.StreamSelector) (*types.StreamHandle, error) {
	switch selector.Stream {
	case ctypes.StreamTypeTicker:
		return c.subscribeTicker(ctx, selector)
	case ctypes.StreamTypeTrade:
		return c.subscribeTrade(ctx, selector)
	case ctypes.StreamTypeDepth:
		return c.subscribeDepth(ctx, selector)
	case ctypes.StreamTypeKline:
		return c.subscribeKline(ctx, selector)
	case ctypes.StreamTypeMarkPrice:
		return c.subscribeMarkPrice(ctx, selector)
	case ctypes.StreamTypeAccount, ctypes.StreamTypeAccountRaw:
		return c.subscribeAccount(ctx, selector)
	default:
		return nil, fmt.Errorf("unsupported selector: %s", selector.Stream)
	}
}

func (c *Connector) subscribeTicker(ctx context.Context, selector ctypes.StreamSelector) (*types.StreamHandle, error) {
	if selector.Symbol == nil {
		return nil, fmt.Errorf("ticker selector requires symbol")
	}
	symbol := *selector.Symbol
	streamSymbol := FormatSymbol(symbol)
	out := make(chan *ctypes.Message, c.cfg.Buffer)
	errCh := make(chan error, 1)
	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		doneC, stopC, err := c.spotClient.WsCombinedMarketStatServe([]string{streamSymbol}, func(evt *binance.WsMarketStatEvent) {
			ticker, nerr := ConvertSpotTicker2Types(c.Exchange(), symbol, evt)
			if nerr != nil {
				select {
				case errCh <- nerr:
				default:
				}
				return
			}
			msg := ctypes.NewMessage(c.Exchange(), selector, ticker, ticker.Ts)
			select {
			case <-ctx.Done():
			case out <- msg:
			}
		}, func(wsErr error) {
			select {
			case errCh <- wsErr:
			default:
			}
		})
		if err != nil {
			return nil, err
		}
		return types.BuildHandle(ctx, out, errCh, stopC, doneC), nil
	case ctypes.MarketTypeFuture:
		doneC, stopC, err := c.futureClient.WsMarketTickerServe(streamSymbol, func(evt *futures.WsMarketTickerEvent) {
			ticker, nerr := ConvertFutureTicker2Types(c.Exchange(), symbol, evt)
			if nerr != nil {
				select {
				case errCh <- nerr:
				default:
				}
				return
			}
			msg := ctypes.NewMessage(c.Exchange(), selector, ticker, ticker.Ts)
			select {
			case <-ctx.Done():
			case out <- msg:
			}
		}, func(wsErr error) {
			select {
			case errCh <- wsErr:
			default:
			}
		})
		if err != nil {
			return nil, err
		}
		return types.BuildHandle(ctx, out, errCh, stopC, doneC), nil
	}
	return nil, fmt.Errorf("unsupported symbol type: %s", symbol.Type)
}

func (c *Connector) subscribeTrade(ctx context.Context, selector ctypes.StreamSelector) (*types.StreamHandle, error) {
	if selector.Symbol == nil {
		return nil, fmt.Errorf("trade selector requires symbol")
	}
	symbol := *selector.Symbol
	streamSymbol := FormatSymbol(symbol)
	out := make(chan *ctypes.Message, c.cfg.Buffer)
	errCh := make(chan error, 1)

	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		doneC, stopC, err := c.spotClient.WsCombinedAggTradeServe([]string{streamSymbol}, func(evt *binance.WsAggTradeEvent) {
			trade, nerr := ConvertSpotTrade2Types(c.Exchange(), symbol, evt)
			if nerr != nil {
				select {
				case errCh <- nerr:
				default:
				}
				return
			}
			msg := ctypes.NewMessage(c.Exchange(), selector, trade, trade.Ts)
			select {
			case <-ctx.Done():
			case out <- msg:
			}
		}, func(wsErr error) {
			select {
			case errCh <- wsErr:
			default:
			}
		})
		if err != nil {
			return nil, err
		}
		return types.BuildHandle(ctx, out, errCh, stopC, doneC), nil
	case ctypes.MarketTypeFuture:
		doneC, stopC, err := c.futureClient.WsCombinedAggTradeServe([]string{streamSymbol}, func(evt *futures.WsAggTradeEvent) {
			trade, nerr := ConvertFutureTrade2Types(c.Exchange(), symbol, evt)
			if nerr != nil {
				select {
				case errCh <- nerr:
				default:
				}
				return
			}
			msg := ctypes.NewMessage(c.Exchange(), selector, trade, trade.Ts)
			select {
			case <-ctx.Done():
			case out <- msg:
			}
		}, func(wsErr error) {
			select {
			case errCh <- wsErr:
			default:
			}
		})
		if err != nil {
			return nil, err
		}
		return types.BuildHandle(ctx, out, errCh, stopC, doneC), nil
	}
	return nil, fmt.Errorf("unsupported symbol type: %s", symbol.Type)
}

func (c *Connector) subscribeDepth(ctx context.Context, selector ctypes.StreamSelector) (*types.StreamHandle, error) {
	if selector.Symbol == nil {
		return nil, fmt.Errorf("depth selector requires symbol")
	}
	symbol := *selector.Symbol
	streamSymbol := FormatSymbol(symbol)
	out := make(chan *ctypes.Message, c.cfg.Buffer)
	errCh := make(chan error, 1)

	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		doneC, stopC, err := c.spotClient.WsCombinedDepthServe([]string{streamSymbol}, func(evt *binance.WsDepthEvent) {
			book, nerr := ConvertSpotDepth2Types(c.Exchange(), symbol, evt)
			if nerr != nil {
				select {
				case errCh <- nerr:
				default:
				}
				return
			}
			msg := ctypes.NewMessage(c.Exchange(), selector, book, book.Ts)
			select {
			case <-ctx.Done():
			case out <- msg:
			}
		}, func(wsErr error) {
			select {
			case errCh <- wsErr:
			default:
			}
		})
		if err != nil {
			return nil, err
		}
		return types.BuildHandle(ctx, out, errCh, stopC, doneC), nil
	case ctypes.MarketTypeFuture:
		doneC, stopC, err := c.futureClient.WsDiffDepthServeWithRate(streamSymbol, 500*time.Millisecond, func(evt *futures.WsDepthEvent) {
			book, nerr := ConvertFutureDepth2Types(c.Exchange(), symbol, evt)
			if nerr != nil {
				select {
				case errCh <- nerr:
				default:
				}
				return
			}
			msg := ctypes.NewMessage(c.Exchange(), selector, book, book.Ts)
			select {
			case <-ctx.Done():
			case out <- msg:
			}
		}, func(wsErr error) {
			select {
			case errCh <- wsErr:
			default:
			}
		})
		if err != nil {
			return nil, err
		}
		return types.BuildHandle(ctx, out, errCh, stopC, doneC), nil
	}
	return nil, fmt.Errorf("unsupported symbol type: %s", symbol.Type)
}

func (c *Connector) subscribeKline(ctx context.Context, selector ctypes.StreamSelector) (*types.StreamHandle, error) {
	if selector.Interval == nil {
		return nil, fmt.Errorf("kline selector requires interval")
	}
	if selector.Symbol == nil {
		return nil, fmt.Errorf("kline selector requires symbol")
	}
	symbol := *selector.Symbol
	itv, err := ParseInterval(*selector.Interval)
	if err != nil {
		return nil, err
	}
	streamSymbol := FormatSymbol(symbol)
	out := make(chan *ctypes.Message, c.cfg.Buffer)
	errCh := make(chan error, 1)
	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		doneC, stopC, err := c.spotClient.WsCombinedKlineServe(map[string]string{
			streamSymbol: itv.String(),
		}, func(evt *binance.WsKlineEvent) {
			kline, nerr := ConvertSpotKline2Types(c.Exchange(), symbol, *selector.Interval, evt)
			if nerr != nil {
				select {
				case errCh <- nerr:
				default:
				}
				return
			}
			msg := ctypes.NewMessage(c.Exchange(), selector, kline, time.UnixMilli(evt.Time))
			select {
			case <-ctx.Done():
			case out <- msg:
			}
		}, func(wsErr error) {
			select {
			case errCh <- wsErr:
			default:
			}
		})
		if err != nil {
			return nil, err
		}
		return types.BuildHandle(ctx, out, errCh, stopC, doneC), nil
	case ctypes.MarketTypeFuture:
		doneC, stopC, err := c.futureClient.WsCombinedKlineServe(map[string]string{
			streamSymbol: itv.String(),
		}, func(evt *futures.WsKlineEvent) {
			kline, nerr := ConvertFutureKline2Types(c.Exchange(), symbol, *selector.Interval, evt)
			if nerr != nil {
				select {
				case errCh <- nerr:
				default:
				}
				return
			}
			msg := ctypes.NewMessage(c.Exchange(), selector, kline, time.UnixMilli(evt.Time))
			select {
			case <-ctx.Done():
			case out <- msg:
			}
		}, func(wsErr error) {
			select {
			case errCh <- wsErr:
			default:
			}
		})
		if err != nil {
			return nil, err
		}
		return types.BuildHandle(ctx, out, errCh, stopC, doneC), nil
	}
	return nil, fmt.Errorf("unsupported symbol type: %s", symbol.Type)
}

// subscribeMarkPrice 订阅合约标记价格事件，更新速度：1s
func (c *Connector) subscribeMarkPrice(ctx context.Context, selector ctypes.StreamSelector) (*types.StreamHandle, error) {
	if selector.Symbol == nil {
		return nil, fmt.Errorf("mark price selector requires symbol")
	}
	symbol := *selector.Symbol
	if symbol.Type != ctypes.MarketTypeFuture {
		return nil, fmt.Errorf("mark price selector requires future symbol")
	}
	streamSymbol := FormatSymbol(symbol)
	out := make(chan *ctypes.Message, c.cfg.Buffer)
	errCh := make(chan error, 1)
	doneC, stopC, err := c.futureClient.WsCombinedMarkPriceServeWithRate(map[string]time.Duration{streamSymbol: 1 * time.Second}, func(evt *futures.WsMarkPriceEvent) {
		markPrice := &ctypes.MarkPrice{
			Exchange:  c.Exchange(),
			Symbol:    symbol,
			MarkPrice: number.DecimalFromString(evt.MarkPrice),
			Ts:        time.UnixMilli(evt.Time),
		}
		msg := ctypes.NewMessage(c.Exchange(), selector, markPrice, markPrice.Ts)
		select {
		case <-ctx.Done():
		case out <- msg:
		}
	}, func(wsErr error) {
		select {
		case errCh <- wsErr:
		default:
		}
	})
	if err != nil {
		return nil, err
	}
	return types.BuildHandle(ctx, out, errCh, stopC, doneC), nil
}

func (c *Connector) subscribeAccount(ctx context.Context, selector ctypes.StreamSelector) (*types.StreamHandle, error) {
	out := make(chan *ctypes.Message, c.cfg.Buffer)
	errCh := make(chan error, 1)

	// 现货用户数据流
	spotDoneC, spotStopC, err := c.listenSpotAccount(ctx, selector, out, errCh)
	if err != nil {
		return nil, err
	}

	var futureDoneC, futureStopC chan struct{}
	if c.IsPortfolioMode() {
		futureDoneC, futureStopC, err = c.listenPortfolioAccount(ctx, selector, out, errCh)
	} else {
		futureDoneC, futureStopC, err = c.listenFutureAccount(ctx, selector, out, errCh)
	}
	if err != nil {
		if spotStopC != nil {
			close(spotStopC)
		}
		return nil, err
	}

	// 合并 doneC / stopC：对外只暴露一组通道
	mergedDoneC := make(chan struct{})
	mergedStopC := make(chan struct{})

	var once sync.Once
	// 任一底层 doneC 关闭时，关闭 mergedDoneC
	go func() {
		if spotDoneC != nil {
			<-spotDoneC
			once.Do(func() {
				close(mergedDoneC)
			})
		}
	}()
	go func() {
		if futureDoneC != nil {
			<-futureDoneC
			once.Do(func() {
				close(mergedDoneC)
			})
		}
	}()

	// 对外 stop 会触发两路底层 stopC
	go func() {
		<-mergedStopC
		if spotStopC != nil {
			close(spotStopC)
		}
		if futureStopC != nil {
			close(futureStopC)
		}
	}()

	return types.BuildHandle(ctx, out, errCh, mergedStopC, mergedDoneC), nil
}

func (c *Connector) listenSpotAccount(ctx context.Context, selector ctypes.StreamSelector, outCh chan *ctypes.Message, errCh chan error) (doneC, stopC chan struct{}, err error) {
	if c.account == nil {
		return nil, nil, fmt.Errorf("account is required")
	}

	return c.spotClient.WsUserDataServeSignature(func(evt *binance.WsUserDataEvent) {
		log.Info().Str("account_id", c.account.ID).
			Str("exchange", c.Exchange().String()).
			Str("market", "spot").
			Str("event_type", string(evt.Event)).
			Interface("event", evt).
			Msg("account event")
		ts := time.UnixMilli(evt.Time)
		switch evt.Event {
		case binance.UserDataEventTypeOutboundAccountPosition:
			ts := time.UnixMilli(evt.AccountUpdate.AccountUpdateTime)
			assets := make([]*ctypes.AssetEvent, 0, len(evt.AccountUpdate.WsAccountUpdates))
			for _, update := range evt.AccountUpdate.WsAccountUpdates {
				_free := decimal.RequireFromString(update.Free)
				_locked := decimal.RequireFromString(update.Locked)
				asset := &ctypes.AssetEvent{
					WalletType: ctypes.WalletTypeSpot,
					Code:       update.Asset,
					Balance:    lo.ToPtr(_free.Add(_locked)),
					Locked:     lo.ToPtr(_locked),
					UpdatedTs:  ts,
				}
				assets = append(assets, asset)
			}
			if len(assets) > 0 {
				payload := &ctypes.BalanceUpdate{
					Type:   ctypes.UpdateTypeSnapshot,
					Reason: ctypes.NormalizeLedgerReason(c.exchange, "outbound account position update"),
					Assets: assets,
				}
				c.send(ctx, outCh, selector, payload, ts)
			}
		case binance.UserDataEventTypeBalanceUpdate:
			ts := time.UnixMilli(evt.BalanceUpdate.TransactionTime)
			_balance := decimal.RequireFromString(evt.BalanceUpdate.Change)
			payload := &ctypes.BalanceUpdate{
				Type:   ctypes.UpdateTypeIncrement,
				Reason: ctypes.NormalizeLedgerReason(c.exchange, "balance update"),
				Assets: []*ctypes.AssetEvent{
					{
						WalletType: ctypes.WalletTypeSpot,
						Code:       evt.BalanceUpdate.Asset,
						Balance:    &_balance,
						UpdatedTs:  ts,
					},
				},
			}
			c.send(ctx, outCh, selector, payload, ts)
		case binance.UserDataEventTypeExecutionReport:
			symbol, err := c.ParseSymbol(ctx, evt.OrderUpdate.Symbol, ctypes.MarketTypeSpot)
			if err != nil {
				log.Err(err).Str("symbol", evt.OrderUpdate.Symbol).Msg("failed to parse symbol")
				return
			}
			payload, err := c.ConvertWsSpotOrderUpdate2Types(ctx, symbol, &evt.OrderUpdate)
			if err != nil {
				log.Err(err).Msg("failed to convert ws spot order update to types")
				return
			}
			c.send(ctx, outCh, selector, payload, ts)
		case binance.UserDataEventTypeExternalLockUpdate:
			ts := time.UnixMilli(evt.ExternalLockUpdate.TransactionTime)
			_locked := decimal.RequireFromString(evt.ExternalLockUpdate.Delta)
			payload := &ctypes.BalanceUpdate{
				Type:   ctypes.UpdateTypeIncrement,
				Reason: ctypes.NormalizeLedgerReason(c.exchange, "external lock update"),
				Assets: []*ctypes.AssetEvent{
					{
						WalletType: ctypes.WalletTypeSpot,
						Code:       evt.ExternalLockUpdate.Asset,
						Locked:     lo.ToPtr(_locked),
						UpdatedTs:  ts,
					},
				},
			}
			c.send(ctx, outCh, selector, payload, ts)
		case "eventStreamTerminated":
			errCh <- fmt.Errorf("event stream terminated")
			return
		default:
			log.Error().Str("event_type", string(evt.Event)).Str("selector", selector.Key()).Msg("unsupported event type")
		}
	}, func(wsErr error) {
		select {
		case errCh <- wsErr:
		default:
		}
	})
}

func (c *Connector) send(ctx context.Context, outCh chan *ctypes.Message, selector ctypes.StreamSelector, payload any, ts time.Time) {
	message := ctypes.NewMessage(c.Exchange(), selector, payload, ts)
	select {
	case <-ctx.Done():
	case outCh <- message:
	}
}

func (c *Connector) listenFutureAccount(ctx context.Context, selector ctypes.StreamSelector, outCh chan *ctypes.Message, errCh chan error) (doneC, stopC chan struct{}, err error) {
	// 合约用户数据流（基于 futures listenKey）
	listenKey, err := c.futureClient.NewStartUserStreamService().Do(ctx)
	if err != nil {
		return nil, nil, err
	}

	doneC, stopC, err = c.futureClient.WsUserDataServe(listenKey, func(evt *futures.WsUserDataEvent) {
		log.Info().Str("account_id", c.account.ID).
			Str("exchange", c.Exchange().String()).
			Str("market", "future").
			Str("event_type", string(evt.Event)).
			Interface("event", evt).
			Msg("account event")
		var (
			ts      = time.UnixMilli(evt.TransactionTime)
			payload any
		)
		switch evt.Event {
		case futures.UserDataEventTypeAccountConfigUpdate:
			symbol, err := c.ParseSymbol(ctx, evt.AccountConfigUpdate.Symbol, ctypes.MarketTypeFuture)
			if err != nil {
				log.Err(err).Str("symbol", evt.AccountConfigUpdate.Symbol).Msg("failed to parse symbol")
				return
			}
			c.send(ctx, outCh, selector, &ctypes.SymbolLeverage{
				Exchange:  c.Exchange(),
				Symbol:    symbol,
				Side:      ctypes.PositionSideLong,
				Leverage:  int(evt.AccountConfigUpdate.Leverage),
				UpdatedTs: ts,
			}, ts)
			c.send(ctx, outCh, selector, &ctypes.SymbolLeverage{
				Exchange:  c.Exchange(),
				Symbol:    symbol,
				Side:      ctypes.PositionSideShort,
				Leverage:  int(evt.AccountConfigUpdate.Leverage),
				UpdatedTs: ts,
			}, ts)
		case futures.UserDataEventTypeAccountUpdate:
			if len(evt.AccountUpdate.Balances) > 0 {
				balancePayload := &ctypes.BalanceUpdate{
					Type:   ctypes.UpdateTypeIncrement,
					Reason: ctypes.NormalizeLedgerReason(c.exchange, string(evt.AccountUpdate.Reason)),
				}
				for _, balance := range evt.AccountUpdate.Balances {
					asset := &ctypes.AssetEvent{
						WalletType: ctypes.WalletTypeFuture,
						Code:       balance.Asset,
						Balance:    lo.ToPtr(decimal.RequireFromString(balance.ChangeBalance)),
						UpdatedTs:  ts,
					}
					balancePayload.Assets = append(balancePayload.Assets, asset)
				}
				c.send(ctx, outCh, selector, balancePayload, ts)
			}

			if len(evt.AccountUpdate.Positions) > 0 {
				positionPayload := &ctypes.PositionsUpdate{
					Type:      ctypes.UpdateTypeSnapshot,
					Reason:    string(evt.AccountUpdate.Reason),
					Positions: make([]*ctypes.Position, 0, len(evt.AccountUpdate.Positions)),
				}
				for _, position := range evt.AccountUpdate.Positions {
					symbol, err := c.ParseSymbol(ctx, position.Symbol, ctypes.MarketTypeFuture)
					if err != nil {
						log.Err(err).Str("symbol", position.Symbol).Msg("failed to parse symbol")
						continue
					}
					payload := &ctypes.Position{
						Symbol:     symbol,
						Side:       ctypes.ParsePositionSide(string(position.Side)),
						Isolated:   position.MarginType == futures.MarginTypeIsolated,
						Amount:     number.DecimalFromString(position.Amount).Abs(),
						EntryPrice: number.DecimalFromString(position.EntryPrice),
						UpdatedTs:  ts,
					}
					positionPayload.Positions = append(positionPayload.Positions, payload)
				}
				if len(positionPayload.Positions) > 0 {
					c.send(ctx, outCh, selector, positionPayload, ts)
				}
			}
			return
		case futures.UserDataEventTypeOrderTradeUpdate:
			symbol, err := c.ParseSymbol(ctx, evt.OrderTradeUpdate.Symbol, ctypes.MarketTypeFuture)
			if err != nil {
				log.Err(err).Str("symbol", evt.OrderTradeUpdate.Symbol).Msg("failed to parse symbol")
				return
			}
			payload, err = c.ConvertWsFutureOrder2Types(ctx, symbol, &evt.OrderTradeUpdate, ts)
			if err != nil {
				log.Err(err).Msg("failed to convert ws future order update to types")
				return
			}
		case futures.UserDataEventTypeAlgoUpdate:
			symbol, err := c.ParseSymbol(ctx, evt.AlgoUpdate.Symbol, ctypes.MarketTypeFuture)
			if err != nil {
				log.Err(err).Str("symbol", evt.AlgoUpdate.Symbol).Msg("failed to parse symbol")
				return
			}
			payload, err = c.ConvertWsFutureAlgoOrder2Types(ctx, symbol, &evt.AlgoUpdate, ts)
			if err != nil {
				log.Err(err).Msg("failed to convert ws future algo order update to types")
				return
			}
		case futures.UserDataEventTypeConditionalOrderTriggerReject:
			symbol, err := c.ParseSymbol(ctx, evt.ConditionalOrderTriggerReject.Symbol, ctypes.MarketTypeFuture)
			if err != nil {
				log.Err(err).Str("symbol", evt.ConditionalOrderTriggerReject.Symbol).Msg("failed to parse symbol")
				return
			}
			orderID := strconv.FormatInt(evt.ConditionalOrderTriggerReject.OrderId, 10)
			payload = &ctypes.Order{
				Symbol:       symbol,
				OrderID:      ctypes.OrderId(orderID),
				RejectReason: evt.ConditionalOrderTriggerReject.RejectReason,
				UpdatedTs:    ts,
			}
		case futures.UserDataEventTypeTradeLite:
			return
		default:
			log.Error().Msgf("unsupported event type: %s", evt.Event)
			return
		}
		message := ctypes.NewMessage(c.Exchange(), selector, payload, ts)
		select {
		case <-ctx.Done():
		case outCh <- message:
		}
	}, func(wsErr error) {
		select {
		case errCh <- wsErr:
		default:
		}
	})
	if err != nil {
		return nil, nil, err
	}

	// 定时刷新 listenKey
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Minute):
				err := c.futureClient.NewKeepaliveUserStreamService().ListenKey(listenKey).Do(ctx)
				if err != nil {
					log.Err(err).Msg("failed to keepalive future account")
				}
			}
		}
	}()

	return
}

func (c *Connector) listenPortfolioAccount(ctx context.Context, selector ctypes.StreamSelector, outCh chan *ctypes.Message, errCh chan error) (doneC, stopC chan struct{}, err error) {
	listenKey, err := c.portfolioClient.NewStartUserStreamService().Do(ctx)
	if err != nil {
		return nil, nil, err
	}

	handler := &WsUserDataHandler{
		c:     c,
		outCh: outCh,
		errCh: errCh,
	}
	doneC, stopC, err = c.portfolioClient.WsUserDataServe(listenKey, handler, func(wsErr error) {
		select {
		case errCh <- wsErr:
		default:
		}
	})
	if err != nil {
		return nil, nil, err
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Minute):
				err := c.portfolioClient.NewKeepaliveUserStreamService().ListenKey(listenKey).Do(ctx)
				if err != nil {
					log.Err(err).Msg("failed to keepalive unify account")
				}
			}
		}
	}()

	return
}

type WsUserDataHandler struct {
	c     *Connector
	outCh chan *ctypes.Message
	errCh chan error
}

// 合约杠杆倍数等账户配置更新推送
func (h *WsUserDataHandler) HandleFuturesAccountConfigUpdate(evt *portfolio.WsFuturesAccountConfigUpdate) {
	if evt == nil {
		return
	}

	// 目前只支持 U本位合约
	if evt.BusinessUnit != "UM" {
		return
	}

	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("market", "future").
		Str("event_type", "futures_account_config_update").
		Interface("event", evt).
		Msg("account event")

	symbol, err := h.c.ParseSymbol(context.Background(), evt.AccountConfig.Symbol, ctypes.MarketTypeFuture)
	if err != nil {
		log.Err(err).Str("symbol", evt.AccountConfig.Symbol).Msg("failed to parse symbol")
		return
	}

	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccountRaw,
		Symbol:  &symbol,
		Account: lo.ToPtr(h.c.account.ID),
	}

	h.outCh <- ctypes.NewMessage(h.c.Exchange(), selector, &ctypes.SymbolLeverage{
		Exchange:  h.c.Exchange(),
		Symbol:    symbol,
		Side:      ctypes.PositionSideShort,
		Leverage:  int(evt.AccountConfig.Leverage),
		UpdatedTs: time.UnixMilli(evt.TransactionTime),
	}, time.UnixMilli(evt.EventTime))

	h.outCh <- ctypes.NewMessage(h.c.Exchange(), selector, &ctypes.SymbolLeverage{
		Exchange:  h.c.Exchange(),
		Symbol:    symbol,
		Side:      ctypes.PositionSideLong,
		Leverage:  int(evt.AccountConfig.Leverage),
		UpdatedTs: time.UnixMilli(evt.TransactionTime),
	}, time.UnixMilli(evt.EventTime))
}

// 合约Balance和Position更新推送
func (h *WsUserDataHandler) HandleFuturesAccountUpdate(evt *portfolio.WsFuturesAccountUpdate) {
	if evt == nil {
		return
	}
	if evt.BusinessUnit != "UM" {
		return
	}

	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("market", "future").
		Str("event_type", "futures_account_update").
		Interface("event", evt).
		Msg("account event")

	reasonCode := ctypes.NormalizeLedgerReason(h.c.exchange, string(evt.AccountData.ReasonType))

	ctx := context.Background()

	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccountRaw,
		Account: lo.ToPtr(h.c.account.ID),
	}

	ts := time.UnixMilli(evt.TransactionTime)

	// 余额更新：按资产生成 BalanceUpdate 事件
	if len(evt.AccountData.Balances) > 0 {
		balancePayload := &ctypes.BalanceUpdate{
			Type:   ctypes.UpdateTypeSnapshot,
			Reason: reasonCode,
		}
		for _, b := range evt.AccountData.Balances {
			balance := decimal.RequireFromString(b.WalletBalance)
			asset := &ctypes.AssetEvent{
				WalletType: ctypes.WalletTypeFuture,
				Code:       b.Asset,
				Balance:    lo.ToPtr(balance),
				UpdatedTs:  ts,
			}
			balancePayload.Assets = append(balancePayload.Assets, asset)
		}
		if len(balancePayload.Assets) > 0 {
			h.outCh <- ctypes.NewMessage(
				h.c.Exchange(),
				selector,
				balancePayload,
				ts,
			)
		}
	}

	if len(evt.AccountData.Positions) > 0 {
		// 持仓更新：按合约生成 Position 事件
		positionPayload := &ctypes.PositionsUpdate{}
		for _, p := range evt.AccountData.Positions {
			symbol, err := h.c.ParseSymbol(ctx, p.Symbol, ctypes.MarketTypeFuture)
			if err != nil {
				log.Err(err).Str("symbol", p.Symbol).Msg("failed to parse symbol")
				continue
			}

			amount, err := decimal.NewFromString(p.PositionAmount)
			if err != nil {
				log.Err(err).Str("entry_price", p.EntryPrice).Msg("failed to parse entry price")
				continue
			}
			entryPrice, err := decimal.NewFromString(p.EntryPrice)
			if err != nil {
				log.Err(err).Str("entry_price", p.EntryPrice).Msg("failed to parse entry price")
				continue
			}
			// WsFuturesPosition 里没有直接给杠杆/维持保证金等字段，这里只填基础字段
			position := &ctypes.Position{
				Symbol:     symbol,
				Side:       ctypes.ParsePositionSide(string(p.PositionSide)),
				Isolated:   false,
				Amount:     amount.Abs(),
				EntryPrice: entryPrice,
				UpdatedTs:  ts,
			}
			positionPayload.Positions = append(positionPayload.Positions, position)
		}
		h.outCh <- ctypes.NewMessage(
			h.c.Exchange(),
			selector,
			positionPayload,
			ts,
		)
	}
}

// 合约订单/交易更新推送
func (h *WsUserDataHandler) HandleFuturesOrderUpdate(evt *portfolio.WsFuturesOrderUpdate) {
	if evt == nil {
		return
	}
	if evt.BusinessUnit != "UM" {
		return
	}

	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("market", "future").
		Str("event_type", "futures_order_update").
		Interface("event", evt).
		Msg("account event")

	symbol, err := h.c.ParseSymbol(context.Background(), evt.Order.Symbol, ctypes.MarketTypeFuture)
	if err != nil {
		log.Err(err).Str("symbol", evt.Order.Symbol).Msg("failed to parse symbol")
		return
	}

	t := time.UnixMilli(evt.TransactionTime)

	order, err := h.c.ConvertWsPortfolioUMOrder2Types(context.Background(), symbol, &evt.Order, t)
	if err != nil {
		log.Err(err).Str("symbol", symbol.String()).Msg("failed to convert portfolio future order")
		return
	}

	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccountRaw,
		Symbol:  &symbol,
		Account: lo.ToPtr(h.c.account.ID),
	}

	h.outCh <- ctypes.NewMessage(
		h.c.Exchange(),
		selector,
		order,
		t,
	)
}

// 合约条件订单/交易更新推送
func (h *WsUserDataHandler) HandleConditionalOrderTradeUpdate(evt *portfolio.WsConditionalOrderTradeUpdate) {
	if evt == nil {
		return
	}

	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("market", "future").
		Str("event_type", "conditional_order_trade_update").
		Interface("event", evt).
		Msg("account event")

	symbol, err := h.c.ParseSymbol(context.Background(), evt.Order.Symbol, ctypes.MarketTypeFuture)
	if err != nil {
		log.Err(err).Str("symbol", evt.Order.Symbol).Msg("failed to parse symbol")
		return
	}

	t := time.UnixMilli(evt.TransTime)

	order, err := h.c.ConvertWsPortfolioUMAlgoOrder2Types(context.Background(), symbol, evt, t)
	if err != nil {
		log.Err(err).Str("symbol", symbol.String()).Msg("failed to convert portfolio future order")
		return
	}

	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccountRaw,
		Symbol:  &symbol,
		Account: lo.ToPtr(h.c.account.ID),
	}

	h.outCh <- ctypes.NewMessage(
		h.c.Exchange(),
		selector,
		order,
		t,
	)
}

// 杠杆账户余额更新事件
func (h *WsUserDataHandler) HandleMarginBalanceUpdate(evt *portfolio.WsMarginBalanceUpdate) {
	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("market", "future").
		Str("event_type", "margin_balance_update").
		Interface("event", evt).
		Msg("account event")
}

// 杠杆账户全仓挂单占用事件
func (h *WsUserDataHandler) HandleOpenOrderLossUpdate(evt *portfolio.WsOpenOrderLossUpdate) {
	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("market", "future").
		Str("event_type", "open_order_loss_update").
		Interface("event", evt).
		Msg("account event")
}

// 杠杆账户更新事件
func (h *WsUserDataHandler) HandleMarginAccountUpdate(evt *portfolio.WsMarginAccountUpdate) {
	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("market", "future").
		Str("event_type", "margin_account_update").
		Interface("event", evt).
		Msg("account event")
}

// 杠杆账户订单事件
func (h *WsUserDataHandler) HandleMarginOrderUpdate(evt *portfolio.WsMarginOrderUpdate) {
	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("market", "future").
		Str("event_type", "margin_order_update").
		Interface("event", evt).
		Msg("account event")
}

// 杠杆账户负债更新
func (h *WsUserDataHandler) HandleLiabilityUpdate(evt *portfolio.WsLiabilityUpdate) {
	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("market", "future").
		Str("event_type", "liability_update").
		Interface("event", evt).
		Msg("account event")
}

// 账户风险状态变动事件
func (h *WsUserDataHandler) HandleRiskLevelChange(evt *portfolio.WsRiskLevelChange) {
	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("market", "future").
		Str("event_type", "risk_level_change").
		Interface("event", evt).
		Msg("account event")
}

// listenKey过期推送
func (h *WsUserDataHandler) HandleListenKeyExpired(*portfolio.WsListenKeyExpired) {}

func (c *Connector) GetMarkets(ctx context.Context, tps []ctypes.MarketType) ([]*ctypes.Market, error) {
	if len(tps) == 0 {
		return nil, nil
	}

	ch := make(chan []*ctypes.Market, len(tps))
	grp, ctx := errgroup.WithContext(ctx)
	for _, t := range tps {
		grp.Go(func() error {
			var (
				err     error
				markets []*ctypes.Market
			)
			switch t {
			case ctypes.MarketTypeSpot:
				markets, err = c.getSpotMarkets(ctx)
			case ctypes.MarketTypeFuture:
				markets, err = c.getFutureMarkets(ctx)
			}
			if err != nil {
				return err
			}
			ch <- markets
			return nil
		})
	}

	if err := grp.Wait(); err != nil {
		return nil, err
	}

	close(ch)

	results := make([]*ctypes.Market, 0, len(tps))
	for markets := range ch {
		results = append(results, markets...)
	}

	return results, nil
}

func (c *Connector) getSpotMarkets(ctx context.Context) ([]*ctypes.Market, error) {
	markets, err := fetchWithType[*binance.ExchangeInfo](c, ctx, CacheKeySpotExchangeInfoV3)
	if err != nil {
		return nil, err
	}
	results := make([]*ctypes.Market, 0, len(markets.Symbols))
	for _, market := range markets.Symbols {
		market, err := ConvertSpotMarket2Types(c.Exchange(), &market)
		if err != nil {
			continue
		}
		results = append(results, market)
	}
	return results, nil
}

func (c *Connector) getFutureMarkets(ctx context.Context) ([]*ctypes.Market, error) {
	markets, err := fetchWithType[*futures.ExchangeInfo](c, ctx, CacheKeyFutureExchangeInfo)
	if err != nil {
		return nil, err
	}
	results := make([]*ctypes.Market, 0, len(markets.Symbols))
	symbolSet := make(map[string]bool)
	for _, market := range markets.Symbols {
		if market.ContractType != futures.ContractTypePerpetual {
			continue
		}
		if symbolSet[market.Symbol] {
			log.Info().Str("symbol", market.Symbol).Msg("duplicate symbol")
			continue
		}
		symbolSet[market.Symbol] = true
		mkt, err := ConvertFutureMarket2Types(c.Exchange(), &market)
		if err != nil {
			continue
		}
		results = append(results, mkt)
	}
	return results, nil
}

func (c *Connector) GetMarket(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Market, error) {
	return c.getSymbolMarket(ctx, symbol)
}

func (c *Connector) getSymbolMarket(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Market, error) {
	// switch symbol.Type {
	// case ctypes.MarketTypeSpot:
	// 	market, err := fetchWithType[*binance.ExchangeInfo](c, ctx, CacheKeySpotSymbolInfoV3, FormatSymbol(symbol))
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	if len(market.Symbols) == 0 {
	// 		return nil, nil
	// 	}
	// 	return ConvertSpotMarket2Types(c.Exchange(), &market.Symbols[0])
	// case ctypes.MarketTypeFuture:
	markets, err := c.GetMarkets(ctx, []ctypes.MarketType{symbol.Type})
	if err != nil {
		return nil, err
	}
	for _, market := range markets {
		if market.Symbol == symbol {
			return market, nil
		}
	}
	// }
	return nil, nil
}

func (c *Connector) Ticker(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Ticker, error) {
	_symbol := FormatSymbol(symbol)

	result := &ctypes.Ticker{
		Symbol:   symbol,
		Exchange: c.Exchange(),
	}

	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		symbolTickers, err := fetchWithType[[]*binance.SymbolTicker](c, ctx, CacheKeySpotSymbolTicker, _symbol)
		if err != nil {
			return nil, err
		}
		if len(symbolTickers) == 0 {
			return nil, nil
		}
		result.LastPrice = number.DecimalFromString(symbolTickers[0].LastPrice)
		result.Open24 = number.DecimalFromString(symbolTickers[0].OpenPrice)
		result.High24 = number.DecimalFromString(symbolTickers[0].HighPrice)
		result.Low24 = number.DecimalFromString(symbolTickers[0].LowPrice)
		result.Volume24 = number.DecimalFromString(symbolTickers[0].Volume)
		result.QuoteVolume24 = number.DecimalFromString(symbolTickers[0].QuoteVolume)
		result.Avg24 = number.DecimalFromString(symbolTickers[0].WeightedAvgPrice)
		result.Ts = time.UnixMilli(symbolTickers[0].CloseTime)
		return result, nil
	case ctypes.MarketTypeFuture:
		symbolTickers, err := fetchWithType[[]*futures.PriceChangeStats](c, ctx, CacheKeyFuturePriceChangeStats, _symbol)
		if err != nil {
			return nil, err
		}
		if len(symbolTickers) == 0 {
			return nil, nil
		}
		result.LastPrice = number.DecimalFromString(symbolTickers[0].LastPrice)
		result.Open24 = number.DecimalFromString(symbolTickers[0].OpenPrice)
		result.High24 = number.DecimalFromString(symbolTickers[0].HighPrice)
		result.Low24 = number.DecimalFromString(symbolTickers[0].LowPrice)
		result.Volume24 = number.DecimalFromString(symbolTickers[0].Volume)
		result.QuoteVolume24 = number.DecimalFromString(symbolTickers[0].QuoteVolume)
		result.Avg24 = number.DecimalFromString(symbolTickers[0].WeightedAvgPrice)
		result.Ts = time.UnixMilli(symbolTickers[0].CloseTime)
		return result, nil
	}
	return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
}

func (c *Connector) Prices(ctx context.Context, marketType *ctypes.MarketType) ([]*ctypes.Price, error) {
	if marketType != nil {
		return c.getPrices(ctx, *marketType)
	}
	result := make([]*ctypes.Price, 0)
	mu := sync.Mutex{}
	errgrp := errgroup.Group{}
	errgrp.Go(func() error {
		prices, err := c.getPrices(ctx, ctypes.MarketTypeSpot)
		if err != nil {
			return err
		}
		mu.Lock()
		result = append(result, prices...)
		mu.Unlock()
		return nil
	})
	errgrp.Go(func() error {
		prices, err := c.getPrices(ctx, ctypes.MarketTypeFuture)
		if err != nil {
			return err
		}
		mu.Lock()
		result = append(result, prices...)
		mu.Unlock()
		return nil
	})
	if err := errgrp.Wait(); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Connector) Price(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Price, error) {
	_symbol := FormatSymbol(symbol)
	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		prices, err := fetchWithType[[]*binance.SymbolPrice](c, ctx, CacheKeySpotTickerPrice, _symbol)
		if err != nil {
			return nil, err
		}
		if len(prices) == 0 {
			return nil, nil
		}
		return &ctypes.Price{
			Exchange: c.Exchange(),
			Symbol:   symbol,
			Price:    number.DecimalFromString(prices[0].Price),
			Ts:       time.Now(),
		}, nil
	case ctypes.MarketTypeFuture:
		prices, err := fetchWithType[[]*futures.SymbolPrice](c, ctx, CacheKeyFutureTickerPrice, _symbol)
		if err != nil {
			return nil, err
		}
		if len(prices) == 0 {
			return nil, nil
		}
		return &ctypes.Price{
			Exchange: c.Exchange(),
			Symbol:   symbol,
			Price:    number.DecimalFromString(prices[0].Price),
			Ts:       time.Now(),
		}, nil
	}
	return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
}

func (c *Connector) getPrices(ctx context.Context, marketType ctypes.MarketType) ([]*ctypes.Price, error) {
	switch marketType {
	case ctypes.MarketTypeSpot:
		prices, err := fetchWithType[[]*binance.SymbolPrice](c, ctx, CacheKeySpotPrices)
		if err != nil {
			return nil, err
		}
		result := make([]*ctypes.Price, 0, len(prices))
		for _, price := range prices {
			symbol, err := c.ParseSymbol(ctx, price.Symbol, marketType)
			if err != nil {
				continue
			}
			result = append(result, &ctypes.Price{
				Exchange: c.Exchange(),
				Symbol:   symbol,
				Price:    number.DecimalFromString(price.Price),
				Ts:       time.Now(),
			})
		}
		return result, nil
	case ctypes.MarketTypeFuture:
		prices, err := fetchWithType[[]*futures.SymbolPrice](c, ctx, CacheKeyFuturePrices)
		if err != nil {
			return nil, err
		}
		result := make([]*ctypes.Price, 0, len(prices))
		for _, price := range prices {
			symbol, err := c.ParseSymbol(ctx, price.Symbol, marketType)
			if err != nil {
				continue
			}
			result = append(result, &ctypes.Price{
				Exchange: c.Exchange(),
				Symbol:   symbol,
				Price:    number.DecimalFromString(price.Price),
				Ts:       time.Now(),
			})
		}
		return result, nil
	}
	return nil, fmt.Errorf("unsupported market type: %s", marketType)
}

func (c *Connector) BookPrices(ctx context.Context, marketType *ctypes.MarketType) ([]*ctypes.BookPrice, error) {
	if marketType != nil {
		return c.getBookPrices(ctx, *marketType)
	}
	result := make([]*ctypes.BookPrice, 0)
	mu := sync.Mutex{}
	errgrp := errgroup.Group{}
	errgrp.Go(func() error {
		tickers, err := c.getBookPrices(ctx, ctypes.MarketTypeSpot)
		if err != nil {
			return err
		}
		mu.Lock()
		result = append(result, tickers...)
		mu.Unlock()
		return nil
	})
	errgrp.Go(func() error {
		tickers, err := c.getBookPrices(ctx, ctypes.MarketTypeFuture)
		if err != nil {
			return err
		}
		mu.Lock()
		result = append(result, tickers...)
		mu.Unlock()
		return nil
	})
	if err := errgrp.Wait(); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Connector) getBookPrices(ctx context.Context, marketType ctypes.MarketType) ([]*ctypes.BookPrice, error) {
	switch marketType {
	case ctypes.MarketTypeSpot:
		bookTickers, err := fetchWithType[[]*binance.BookTicker](c, ctx, CacheKeySpotBookTickers)
		if err != nil {
			return nil, err
		}
		bookPrices := make([]*ctypes.BookPrice, 0, len(bookTickers))
		var timeCost time.Duration
		for _, bookTicker := range bookTickers {
			sTime := time.Now()
			symbol, err := c.ParseSymbol(ctx, bookTicker.Symbol, marketType)
			timeCost += time.Since(sTime)
			if err != nil {
				continue
			}
			bookPrices = append(bookPrices, &ctypes.BookPrice{
				Exchange: c.Exchange(),
				Symbol:   symbol,
				BidPrice: number.DecimalFromString(bookTicker.BidPrice),
				BidQty:   number.DecimalFromString(bookTicker.BidQuantity),
				AskPrice: number.DecimalFromString(bookTicker.AskPrice),
				AskQty:   number.DecimalFromString(bookTicker.AskQuantity),
				Ts:       time.Now(),
			})
		}
		logger.Ctx(ctx).Info().Dur("time_cost", timeCost).Msg("get ticker prices")
		return bookPrices, nil
	case ctypes.MarketTypeFuture:
		bookTickers, err := fetchWithType[[]*futures.BookTicker](c, ctx, CacheKeyFutureBookTickers)
		if err != nil {
			return nil, err
		}
		bookPrices := make([]*ctypes.BookPrice, 0, len(bookTickers))
		for _, bookTicker := range bookTickers {
			symbol, err := c.ParseSymbol(ctx, bookTicker.Symbol, ctypes.MarketTypeFuture)
			if err != nil {
				continue
			}
			bookPrices = append(bookPrices, &ctypes.BookPrice{
				Exchange: c.Exchange(),
				Symbol:   symbol,
				BidPrice: number.DecimalFromString(bookTicker.BidPrice),
				BidQty:   number.DecimalFromString(bookTicker.BidQuantity),
				AskPrice: number.DecimalFromString(bookTicker.AskPrice),
				AskQty:   number.DecimalFromString(bookTicker.AskQuantity),
				Ts:       time.Now(),
			})
		}
		return bookPrices, nil
	}
	return nil, nil
}

func (c *Connector) BookPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.BookPrice, error) {
	_symbol := FormatSymbol(symbol)
	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		bookTickers, err := fetchWithType[[]*binance.BookTicker](c, ctx, CacheKeySpotBookTicker, _symbol)
		if err != nil {
			return nil, err
		}
		if len(bookTickers) == 0 {
			return nil, nil
		}
		return &ctypes.BookPrice{
			Exchange: c.Exchange(),
			Symbol:   symbol,
			BidPrice: number.DecimalFromString(bookTickers[0].BidPrice),
			BidQty:   number.DecimalFromString(bookTickers[0].BidQuantity),
			AskPrice: number.DecimalFromString(bookTickers[0].AskPrice),
			AskQty:   number.DecimalFromString(bookTickers[0].AskQuantity),
			Ts:       time.Now(),
		}, nil
	case ctypes.MarketTypeFuture:
		bookTickers, err := fetchWithType[[]*futures.BookTicker](c, ctx, CacheKeyFutureBookTicker, _symbol)
		if err != nil {
			return nil, err
		}
		if len(bookTickers) == 0 {
			return nil, nil
		}
		return &ctypes.BookPrice{
			Exchange: c.Exchange(),
			Symbol:   symbol,
			BidPrice: number.DecimalFromString(bookTickers[0].BidPrice),
			BidQty:   number.DecimalFromString(bookTickers[0].BidQuantity),
			AskPrice: number.DecimalFromString(bookTickers[0].AskPrice),
			AskQty:   number.DecimalFromString(bookTickers[0].AskQuantity),
			Ts:       time.UnixMilli(bookTickers[0].Time),
		}, nil
	}
	return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
}

func (c *Connector) MarkPrices(ctx context.Context) ([]*ctypes.MarkPrice, error) {
	result := make([]*ctypes.MarkPrice, 0)
	markPrices, err := fetchWithType[[]*futures.PremiumIndex](c, ctx, CacheKeyFutureMarkPrices)
	if err != nil {
		return nil, err
	}
	for _, markPrice := range markPrices {
		symbol, err := c.ParseSymbol(ctx, markPrice.Symbol, ctypes.MarketTypeFuture)
		if err != nil {
			continue
		}
		result = append(result, &ctypes.MarkPrice{
			Exchange:  c.Exchange(),
			Symbol:    symbol,
			MarkPrice: number.DecimalFromString(markPrice.MarkPrice),
			Ts:        time.UnixMilli(markPrice.Time),
		})
	}
	return result, nil
}

func (c *Connector) MarkPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.MarkPrice, error) {
	if symbol.Type != ctypes.MarketTypeFuture {
		return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
	_symbol := FormatSymbol(symbol)
	premiumIndexs, err := fetchWithType[[]*futures.PremiumIndex](c, ctx, CacheKeyFutureSymbolPremiumIndex, _symbol)
	if err != nil {
		return nil, err
	}
	if len(premiumIndexs) == 0 {
		return nil, nil
	}
	return &ctypes.MarkPrice{
		Exchange:  c.Exchange(),
		Symbol:    symbol,
		MarkPrice: number.DecimalFromString(premiumIndexs[0].MarkPrice),
		Ts:        time.UnixMilli(premiumIndexs[0].Time),
	}, nil
}

func (c *Connector) IndexPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.IndexPrice, error) {
	if symbol.Type != ctypes.MarketTypeFuture {
		return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
	_symbol := FormatSymbol(symbol)
	premiumIndexs, err := fetchWithType[[]*futures.PremiumIndex](c, ctx, CacheKeyFutureSymbolPremiumIndex, _symbol)
	if err != nil {
		return nil, err
	}
	if len(premiumIndexs) == 0 {
		return nil, nil
	}
	return &ctypes.IndexPrice{
		Exchange:   c.Exchange(),
		Symbol:     symbol,
		IndexPrice: number.DecimalFromString(premiumIndexs[0].IndexPrice),
		Ts:         time.UnixMilli(premiumIndexs[0].Time),
	}, nil
}

func (c *Connector) IndexComponent(ctx context.Context, symbol ctypes.Symbol) (*ctypes.IndexComponent, error) {
	if symbol.Type != ctypes.MarketTypeFuture {
		return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
	_symbol := FormatSymbol(symbol)
	indexConstituents, err := fetchWithType[*futures.ConstituentsServiceRsp](c, ctx, CacheKeyIndexComponent, _symbol)
	if err != nil {
		return nil, err
	}
	if indexConstituents == nil {
		return nil, nil
	}
	result := &ctypes.IndexComponent{
		Exchange: c.Exchange(),
		Symbol:   symbol,
		Ts:       time.UnixMilli(int64(indexConstituents.Time)),
	}
	for _, component := range indexConstituents.Constituents {
		result.Components = append(result.Components, struct {
			Exchange string          `json:"exchange,omitempty"`
			Symbol   string          `json:"symbol,omitempty"`
			Price    decimal.Decimal `json:"price,omitempty"`
			Weight   decimal.Decimal `json:"weight,omitempty"`
		}{
			Exchange: component.Exchange,
			Symbol:   component.Symbol,
			Price:    number.DecimalFromString(component.Price),
			Weight:   number.DecimalFromString(component.Weight),
		})
	}
	return result, nil
}

func (c *Connector) Trades(ctx context.Context, symbol ctypes.Symbol, limit int) ([]*ctypes.Trade, error) {
	_symbol := FormatSymbol(symbol)

	result := make([]*ctypes.Trade, 0, limit)

	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		trades, err := c.spotClient.NewRecentTradesV3Service().Symbol(_symbol).Limit(limit).Do(ctx)
		if err != nil {
			return nil, err
		}
		for _, t := range trades {
			result = append(result, &ctypes.Trade{
				Symbol:  symbol,
				TradeID: strconv.FormatInt(t.ID, 10),
				Price:   number.DecimalFromString(t.Price),
				Size:    number.DecimalFromString(t.Quantity),
				IsBuy:   !t.IsBuyerMaker,
				Ts:      time.UnixMilli(t.Time),
			})
		}
		return result, nil
	case ctypes.MarketTypeFuture:
		trades, err := c.futureClient.NewRecentTradesService().Symbol(_symbol).Limit(limit).Do(ctx)
		if err != nil {
			return nil, err
		}
		for _, t := range trades {
			result = append(result, &ctypes.Trade{
				Symbol:  symbol,
				TradeID: strconv.FormatInt(t.ID, 10),
				Price:   number.DecimalFromString(t.Price),
				Size:    number.DecimalFromString(t.Quantity),
				IsBuy:   !t.IsBuyerMaker,
				Ts:      time.UnixMilli(t.Time),
			})
		}
		return result, nil
	}
	return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
}

func (c *Connector) Depth(ctx context.Context, symbol ctypes.Symbol, limit int) (*ctypes.OrderBook, error) {
	_symbol := FormatSymbol(symbol)

	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		depth, err := c.spotClient.NewDepthService().Symbol(_symbol).Limit(limit).Do(ctx)
		if err != nil {
			return nil, err
		}
		snapshot := &ctypes.OrderBook{
			Symbol: symbol,
			Bids:   make([]ctypes.OrderBookLevel, 0, len(depth.Bids)),
			Asks:   make([]ctypes.OrderBookLevel, 0, len(depth.Asks)),
			SeqId:  depth.LastUpdateID,
		}
		for _, bid := range depth.Bids {
			snapshot.Bids = append(snapshot.Bids, ctypes.OrderBookLevel{
				Price: number.DecimalFromString(bid.Price),
				Size:  number.DecimalFromString(bid.Quantity),
			})
		}
		for _, ask := range depth.Asks {
			snapshot.Asks = append(snapshot.Asks, ctypes.OrderBookLevel{
				Price: number.DecimalFromString(ask.Price),
				Size:  number.DecimalFromString(ask.Quantity),
			})
		}
		return snapshot, nil
	case ctypes.MarketTypeFuture:
		depth, err := c.futureClient.NewDepthService().Symbol(_symbol).Limit(limit).Do(ctx)
		if err != nil {
			return nil, err
		}
		snapshot := &ctypes.OrderBook{
			Symbol: symbol,
			Bids:   make([]ctypes.OrderBookLevel, 0, len(depth.Bids)),
			Asks:   make([]ctypes.OrderBookLevel, 0, len(depth.Asks)),
			SeqId:  depth.LastUpdateID,
		}
		for _, bid := range depth.Bids {
			snapshot.Bids = append(snapshot.Bids, ctypes.OrderBookLevel{
				Price: number.DecimalFromString(bid.Price),
				Size:  number.DecimalFromString(bid.Quantity),
			})
		}
		for _, ask := range depth.Asks {
			snapshot.Asks = append(snapshot.Asks, ctypes.OrderBookLevel{
				Price: number.DecimalFromString(ask.Price),
				Size:  number.DecimalFromString(ask.Quantity),
			})
		}
		return snapshot, nil
	}
	return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
}

func (c *Connector) Klines(ctx context.Context, symbol ctypes.Symbol, interval ctypes.Interval, limit int) ([]*ctypes.Kline, error) {
	return c.HisKlines(ctx, symbol, interval, nil, nil, &limit)
}

func (c *Connector) HisKlines(ctx context.Context, symbol ctypes.Symbol, interval ctypes.Interval, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.Kline, error) {
	if limit == nil || *limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than 0")
	}
	if *limit > 1000 {
		return nil, fmt.Errorf("limit must be less than or equal to 1000")
	}

	_symbol := FormatSymbol(symbol)
	_interval, err := ParseInterval(interval)
	if err != nil {
		return nil, err
	}

	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		svc := c.spotClient.NewKlinesService().Symbol(_symbol).Interval(_interval.String()).Limit(*limit)
		if startTs != nil {
			svc = svc.StartTime(startTs.UnixMilli())
		}
		if endTs != nil {
			svc = svc.EndTime(endTs.UnixMilli())
		}
		klines, err := svc.Do(ctx)
		if err != nil {
			return nil, err
		}
		if len(klines) == 0 {
			return nil, nil
		}
		result := make([]*ctypes.Kline, 0, len(klines))
		for _, k := range klines {
			result = append(result, &ctypes.Kline{
				Symbol:      symbol,
				Interval:    interval,
				Open:        number.DecimalFromString(k.Open),
				Close:       number.DecimalFromString(k.Close),
				High:        number.DecimalFromString(k.High),
				Low:         number.DecimalFromString(k.Low),
				Volume:      number.DecimalFromString(k.Volume),
				QuoteVolume: number.DecimalFromString(k.QuoteAssetVolume),
				Trades:      k.TradeNum,
				OpenTs:      time.UnixMilli(k.OpenTime),
				CloseTs:     time.UnixMilli(k.CloseTime),
			})
		}
		return result, nil
	case ctypes.MarketTypeFuture:
		svc := c.futureClient.NewKlinesService().Symbol(_symbol).Interval(_interval.String()).Limit(*limit)
		if startTs != nil {
			svc = svc.StartTime(startTs.UnixMilli())
		}
		if endTs != nil {
			svc = svc.EndTime(endTs.UnixMilli())
		}
		klines, err := svc.Do(ctx)
		if err != nil {
			return nil, err
		}
		if len(klines) == 0 {
			return nil, nil
		}
		result := make([]*ctypes.Kline, 0, len(klines))
		for _, k := range klines {
			result = append(result, &ctypes.Kline{
				Symbol:      symbol,
				Interval:    interval,
				Open:        number.DecimalFromString(k.Open),
				Close:       number.DecimalFromString(k.Close),
				High:        number.DecimalFromString(k.High),
				Low:         number.DecimalFromString(k.Low),
				Volume:      number.DecimalFromString(k.Volume),
				QuoteVolume: number.DecimalFromString(k.QuoteAssetVolume),
				Trades:      k.TradeNum,
				OpenTs:      time.UnixMilli(k.OpenTime),
				CloseTs:     time.UnixMilli(k.CloseTime),
			})
		}
		return result, nil
	}
	return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
}

func (c *Connector) FundingRate(ctx context.Context, symbol ctypes.Symbol) (*ctypes.FundingRate, error) {
	if symbol.Type != ctypes.MarketTypeFuture {
		return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
	_symbol := FormatSymbol(symbol)
	premiumIndex, err := fetchWithType[[]*futures.PremiumIndex](c, ctx, CacheKeyFutureSymbolPremiumIndex, _symbol)
	if err != nil {
		return nil, err
	}
	if len(premiumIndex) == 0 {
		return nil, nil
	}
	return &ctypes.FundingRate{
		Exchange:        c.Exchange(),
		Symbol:          symbol,
		FundingRate:     number.DecimalFromString(premiumIndex[0].LastFundingRate),
		InterestRate:    number.DecimalFromString(premiumIndex[0].InterestRate),
		NextFundingTime: time.UnixMilli(premiumIndex[0].NextFundingTime),
		Ts:              time.UnixMilli(premiumIndex[0].Time),
	}, nil
}

func (c *Connector) getFundingInfo(ctx context.Context, symbol ctypes.Symbol) (*ctypes.FundingInfo, error) {
	if symbol.Type != ctypes.MarketTypeFuture {
		return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
	_symbol := FormatSymbol(symbol)
	fundingInfo, err := fetchWithType[[]*futures.FundingRateInfo](c, ctx, CacheKeyFutureFundingInfo)
	if err != nil {
		return nil, err
	}
	for _, fundingInfo := range fundingInfo {
		if fundingInfo.Symbol == _symbol {
			return &ctypes.FundingInfo{
				Symbol:                   symbol,
				AdjustedFundingRateCap:   fundingInfo.AdjustedFundingRateCap,
				AdjustedFundingRateFloor: fundingInfo.AdjustedFundingRateFloor,
				FundingIntervalHours:     lo.ToPtr(int(fundingInfo.FundingIntervalHours)),
			}, nil
		}
	}
	return &ctypes.FundingInfo{
		Symbol:                   symbol,
		AdjustedFundingRateCap:   DefaultAdjustedFundingRateCap,
		AdjustedFundingRateFloor: DefaultAdjustedFundingRateFloor,
		FundingIntervalHours:     lo.ToPtr(DefaultFundingIntervalHours),
	}, nil
}

func (c *Connector) HisFundingRates(ctx context.Context, symbol ctypes.Symbol, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.FundingRate, error) {
	if symbol.Type != ctypes.MarketTypeFuture {
		return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
	_symbol := FormatSymbol(symbol)
	svc := c.futureClient.NewFundingRateService().Symbol(_symbol)
	if startTs != nil {
		svc = svc.StartTime(startTs.UnixMilli())
	}
	if endTs != nil {
		svc = svc.EndTime(endTs.UnixMilli())
	}
	if limit != nil {
		svc = svc.Limit(*limit)
	}
	fundingRates, err := svc.Do(ctx)
	if err != nil {
		return nil, err
	}
	if len(fundingRates) == 0 {
		return nil, nil
	}
	result := make([]*ctypes.FundingRate, 0, len(fundingRates))
	for _, fundingRate := range fundingRates {
		result = append(result, &ctypes.FundingRate{
			Exchange:    c.Exchange(),
			Symbol:      symbol,
			FundingRate: number.DecimalFromString(fundingRate.FundingRate),
			Ts:          time.UnixMilli(fundingRate.FundingTime),
		})
	}
	return result, nil
}

func (c *Connector) OpenInterest(ctx context.Context, symbol ctypes.Symbol) (decimal.Decimal, error) {
	if symbol.Type != ctypes.MarketTypeFuture {
		return decimal.Zero, fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
	_symbol := FormatSymbol(symbol)
	openInterest, err := c.futureClient.NewGetOpenInterestService().Symbol(_symbol).Do(ctx)
	if err != nil {
		return decimal.Zero, err
	}
	return decimal.RequireFromString(openInterest.OpenInterest), nil
}

func (c *Connector) Account(ctx context.Context) (*ctypes.AccountBo, error) {
	accountInfo, err := c.getAccountInfo(ctx)
	if err != nil {
		return nil, err
	}
	return accountInfo.ToTypesAccount(c.exchange), nil
}

func (c *Connector) getAccountInfo(ctx context.Context) (*Account, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	var err error

	// 钱包-账户信息 /sapi/v1/account/info
	var sapiAccountInfo *binance.SapiAccountInfo
	if !c.cfg.UseDemo {
		sapiAccountInfo, err = fetchWithType[*binance.SapiAccountInfo](c, ctx, CacheKeySapiAccount)
		if err != nil {
			return nil, err
		}
	} else {
		sapiAccountInfo = &binance.SapiAccountInfo{
			IsMarginEnabled:                true,
			IsFutureEnabled:                true,
			IsOptionsEnabled:               false,
			IsPortfolioMarginRetailEnabled: false,
		}
	}

	// API 权限
	var apiKeyPermission *binance.APIKeyPermission
	if !c.cfg.UseDemo {
		apiKeyPermission, err = fetchWithType[*binance.APIKeyPermission](c, ctx, CacheKeyApiKeyPermission)
		if err != nil {
			return nil, err
		}
	} else {
		apiKeyPermission = &binance.APIKeyPermission{
			EnableReading:                true,
			EnableFutures:                true,
			EnableMargin:                 true,
			EnableSpotAndMarginTrading:   true,
			EnablePortfolioMarginTrading: false,
		}
	}

	// 现货账户信息
	spotAccount, err := fetchWithType[*binance.Account](c, ctx, CacheKeySpotAccountV3)
	if err != nil {
		return nil, err
	}

	var (
		futureAccount    *futures.AccountV3
		futureAcctConfig *FutureAcctConfig
		portfolioAccount *portfolio_pro.Account
	)

	// 统一账户模式
	if sapiAccountInfo.IsPortfolioMarginRetailEnabled {
		// 统一账户专业版-查询账户信息 /sapi/v1/portfolio/account
		portfolioAccount, err = fetchWithType[*portfolio_pro.Account](c, ctx, CacheKeyPortfolioAccountPro)
		if err != nil {
			return nil, err
		}
		if apiKeyPermission.EnablePortfolioMarginTrading {
			// 统一账户-查询 U本位合约 账户配置 /papi/v1/um/accountConfig
			umAccountConfig, err := fetchWithType[*portfolio.UMAccountConfig](c, ctx, CacheKeyUMAccountConfig)
			if err != nil {
				return nil, err
			}
			futureAcctConfig = &FutureAcctConfig{
				CanDeposit:        umAccountConfig.CanDeposit,
				CanTrade:          umAccountConfig.CanTrade,
				CanWithdraw:       umAccountConfig.CanWithdraw,
				DualSidePosition:  umAccountConfig.DualSidePosition,
				FeeTier:           umAccountConfig.FeeTier,
				MultiAssetsMargin: umAccountConfig.MultiAssetsMargin,
				UpdateTime:        umAccountConfig.UpdateTime,
			}
		}
	}

	// 经典合约账户
	if sapiAccountInfo.IsFutureEnabled && !sapiAccountInfo.IsPortfolioMarginRetailEnabled {
		// 查询合约账户信息
		futureAccount, err = fetchWithType[*futures.AccountV3](c, ctx, CacheKeyFutureAccountV3)
		if err != nil {
			return nil, err
		}

		// 合约账户配置 /fapi/v1/accountConfig
		contractAccountConfig, err := fetchWithType[*futures.AccountConfig](c, ctx, CacheKeyFutureAccountConfig)
		if err != nil {
			return nil, err
		}
		futureAcctConfig = &FutureAcctConfig{
			CanDeposit:        contractAccountConfig.CanDeposit,
			CanTrade:          contractAccountConfig.CanTrade,
			CanWithdraw:       contractAccountConfig.CanWithdraw,
			DualSidePosition:  contractAccountConfig.DualSidePosition,
			FeeTier:           contractAccountConfig.FeeTier,
			MultiAssetsMargin: contractAccountConfig.MultiAssetsMargin,
			UpdateTime:        contractAccountConfig.UpdateTime,
		}
	}

	return &Account{
		VipLevel:         sapiAccountInfo.VipLevel,
		IsSpotEnabled:    true,
		IsFutureEnabled:  sapiAccountInfo.IsFutureEnabled,
		IsPortfolioMode:  sapiAccountInfo.IsPortfolioMarginRetailEnabled,
		ApiKeyPermission: apiKeyPermission,
		SpotAccount:      spotAccount,
		FutureAccount:    futureAccount,
		FutureAcctConfig: futureAcctConfig,
		PortfolioAccount: portfolioAccount,
	}, nil
}

func (c *Connector) Balance(ctx context.Context) (*ctypes.Balance, error) {
	balance, err := c.balance(ctx)
	if err != nil {
		return nil, err
	}
	return balance.ToTypesBalance(), nil
}

func (c *Connector) balance(ctx context.Context) (*Balance, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	account, err := c.getAccountInfo(ctx)
	if err != nil {
		return nil, err
	}

	balance := &Balance{
		account: account,
	}

	errgrp := errgroup.Group{}

	if !c.cfg.UseDemo {
		errgrp.Go(func() error {
			// 查询用户钱包余额(按账户汇总，以传入币种计算) /sapi/v1/asset/wallet/balance
			balance.AssetBalance, err = c.spotClient.NewWalletBalanceService().QuoteAsset("USDT").Do(ctx)
			return err
		})
	} else {
		balance.AssetBalance = []*binance.WalletBalance{}
	}

	if !c.cfg.UseDemo {
		errgrp.Go(func() error {
			// 查询资金账户 /sapi/v1/asset/get-funding-asset
			var err error
			balance.FundingBalance, err = c.spotClient.NewGetFundingAssetService().Do(ctx)
			return err
		})
	} else {
		balance.FundingBalance = []binance.FundingAsset{}
	}

	// 查询每日资产快照 /sapi/v1/accountSnapshot
	// snapshot, err := client.NewGetAccountSnapshotService().Type("SPOT").Do(context.Background())
	// if err != nil {
	// 	log.Error().Err(err).Msg("error")
	// 	return
	// }
	// log.Info().Interface("snapshot", snapshot).Msg("snapshot")

	// 查询现货账户
	errgrp.Go(func() error {
		var err error
		balance.SpotAccount, err = c.spotClient.NewGetAccountService().OmitZeroBalances(true).Do(ctx)
		return err
	})

	if account.IsPortfolioMode {
		errgrp.Go(func() error {
			// 统一账户专业版-查询账户余额 /sapi/v1/portfolio/balance
			var err error
			balance.PortfolioBalance, err = c.portfolioProClient.NewGetAccountBalanceService().Do(ctx)
			return err
		})
		errgrp.Go(func() error {
			// 统一账户专业版-查询账户余额 /sapi/v1/portfolio/balance
			var err error
			balance.PortfolioUmBalance, err = c.portfolioClient.NewGetUMAccountDetailV2Service().Do(ctx)
			return err
		})
	}

	if account.IsFutureEnabled && !account.IsPortfolioMode {
		errgrp.Go(func() error {
			// 查询合约账户余额 ?? UM 合约账户余额？
			var err error
			balance.FutureAccount, err = c.futureClient.NewGetAccountV3Service().Do(ctx)
			return err
		})
	}

	if err := errgrp.Wait(); err != nil {
		return nil, err
	}

	balance.UpdatedTs = time.Now()
	return balance, nil
}

func (c *Connector) getSymbolConfig(ctx context.Context, symbol string, tp ctypes.MarketType) (*SymbolConfig, error) {
	symbolConfigs, err := c.getAllSymbolConfigs(ctx, tp)
	if err != nil {
		return nil, err
	}
	for _, symbolConfig := range symbolConfigs {
		if symbolConfig.Symbol == symbol {
			return symbolConfig, nil
		}
	}
	return nil, fmt.Errorf("not found symbol config: %s", symbol)
}

func (c *Connector) getAllSymbolConfigs(ctx context.Context, tp ctypes.MarketType) ([]*SymbolConfig, error) {
	result := make([]*SymbolConfig, 0)
	switch tp {
	case ctypes.MarketTypeFuture:
		if c.IsPortfolioMode() {
			symbolConfigs, err := fetchWithType[[]*portfolio.UMSymbolConfig](c, ctx, CacheKeyUMSymbolConfig)
			if err != nil {
				return nil, err
			}
			for _, symbolConfig := range symbolConfigs {
				result = append(result, &SymbolConfig{
					Symbol:           symbolConfig.Symbol,
					Leverage:         symbolConfig.Leverage,
					IsIsolated:       symbolConfig.MarginType != "CROSSED",
					IsAutoAddMargin:  symbolConfig.IsAutoAddMargin,
					MaxNotionalValue: number.DecimalFromString(symbolConfig.MaxNotionalValue),
				})
			}
		} else {
			symbolConfigs, err := fetchWithType[[]*futures.SymbolConfig](c, ctx, CacheKeyFutureSymbolConfig)
			if err != nil {
				return nil, err
			}
			for _, symbolConfig := range symbolConfigs {
				result = append(result, &SymbolConfig{
					Symbol:           symbolConfig.Symbol,
					Leverage:         symbolConfig.Leverage,
					IsIsolated:       symbolConfig.MarginType != "CROSSED",
					IsAutoAddMargin:  symbolConfig.IsAutoAddMargin,
					MaxNotionalValue: number.DecimalFromString(symbolConfig.MaxNotionalValue),
				})
			}
		}
	}
	return result, nil
}

// 查询杠杆分层标准，目前仅支持U本位合约
func (c *Connector) getLeverageBrackets(ctx context.Context) ([]*LeverageBracket, error) {
	if !c.IsPrivate() {
		return nil, nil
	}

	brackets := make([]*LeverageBracket, 0)
	if c.IsPortfolioMode() {
		leverageBrackets, err := fetchWithType[[]*portfolio.LeverageBracket](c, ctx, CacheKeyUMLeverageBracket)
		if err != nil {
			return nil, err
		}
		for _, bracket := range leverageBrackets {
			bks := make([]Bracket, 0)
			for _, brkt := range bracket.Brackets {
				bks = append(bks, Bracket{
					Bracket:          brkt.Bracket,
					InitialLeverage:  brkt.InitialLeverage,
					NotionalCap:      brkt.NotionalCap,
					NotionalFloor:    brkt.NotionalFloor,
					MaintMarginRatio: brkt.MaintMarginRatio,
					Cum:              brkt.Cum,
				})
			}
			brackets = append(brackets, &LeverageBracket{
				Symbol:       bracket.Symbol,
				NotionalCoef: bracket.NotionalCoef,
				Brackets:     bks,
			})
		}
	} else {
		leverageBrackets, err := fetchWithType[[]*futures.LeverageBracket](c, ctx, CacheKeyFutureLeverageBracket)
		if err != nil {
			return nil, err
		}
		for _, bracket := range leverageBrackets {
			bks := make([]Bracket, 0)
			for _, brkt := range bracket.Brackets {
				bks = append(bks, Bracket{
					Bracket:          brkt.Bracket,
					InitialLeverage:  brkt.InitialLeverage,
					NotionalCap:      brkt.NotionalCap,
					NotionalFloor:    brkt.NotionalFloor,
					MaintMarginRatio: brkt.MaintMarginRatio,
					Cum:              brkt.Cum,
				})
			}
			brackets = append(brackets, &LeverageBracket{
				Symbol:   bracket.Symbol,
				Brackets: bks,
			})
		}
	}
	return brackets, nil
}

func (c *Connector) getLeverageBracket(ctx context.Context, symbol ctypes.Symbol) (*LeverageBracket, error) {
	leverageBrackets, err := c.getLeverageBrackets(ctx)
	if err != nil {
		return nil, err
	}
	if len(leverageBrackets) == 0 {
		return nil, nil
	}
	symbolStr := FormatSymbol(symbol)
	for _, bracket := range leverageBrackets {
		if bracket.Symbol == symbolStr {
			return bracket, nil
		}
	}
	return nil, nil
}

func (c *Connector) GetLeverageBracket(ctx context.Context, symbol ctypes.Symbol, markPrice decimal.Decimal) (*ctypes.LeverageBracket, error) {
	bracket, err := c.getLeverageBracket(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if bracket == nil {
		return nil, nil
	}

	result := &ctypes.LeverageBracket{
		Symbol:   symbol,
		Brackets: make([]ctypes.Bracket, 0),
	}
	for _, brkt := range bracket.Brackets {
		result.Brackets = append(result.Brackets, ctypes.Bracket{
			Bracket:     brkt.Bracket,
			MaxLeverage: float32(brkt.InitialLeverage),
			MinNotional: decimal.NewFromFloat(brkt.NotionalFloor),
			MaxNotional: decimal.NewFromFloat(brkt.NotionalCap),
			Mmr:         decimal.NewFromFloat(brkt.MaintMarginRatio),
			Cum:         decimal.NewFromFloat(brkt.Cum),
		})
	}
	return result, nil
}

func (c *Connector) SetLeverage(ctx context.Context, symbol ctypes.Symbol, leverage int) (int, error) {
	if !c.IsPrivate() {
		return 0, fmt.Errorf("connector is not private mode")
	}

	_symbol := FormatSymbol(symbol)
	if c.IsPortfolioMode() {
		resp, err := c.portfolioClient.NewChangeUMInitialLeverageService().Symbol(_symbol).Leverage(leverage).Do(ctx)
		if err != nil {
			return 0, err
		}
		return resp.Leverage, nil
	} else {
		resp, err := c.futureClient.NewChangeLeverageService().Symbol(_symbol).Leverage(leverage).Do(ctx)
		if err != nil {
			return 0, nil
		}
		return resp.Leverage, nil
	}
}

func (c *Connector) SymbolConfig(ctx context.Context, symbol ctypes.Symbol) (*ctypes.SymbolConfig, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	_symbol := FormatSymbol(symbol)

	result := &ctypes.SymbolConfig{
		Exchange: c.Exchange(),
		Symbol:   symbol,
	}

	market, err := c.getSymbolMarket(ctx, symbol)
	if err != nil {
		return nil, err
	}
	result.Market = *market

	key := fmt.Sprintf("%s:commission:rates:%s", strings.ToLower(string(symbol.Type)), symbol.String())

	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		// 获取当前现货账户指定交易对的佣金费率
		data, err := c.cache.Get(ctx, key, 5*time.Minute, func(ctx context.Context, params ...any) (any, error) {
			return c.spotClient.NewGetCommissionRatesService().Symbol(_symbol).Do(ctx)
		})
		if err != nil {
			return nil, err
		}
		response, ok := data.(*binance.CommissionRatesResponse)
		if !ok {
			return nil, fmt.Errorf("invalid commission rates: %T", data)
		}
		result.MakerCommission = number.DecimalFromString(response.StandardCommission.Maker)
		result.TakerCommission = number.DecimalFromString(response.StandardCommission.Taker)
		// TODO: BNB 折扣？税费？
	case ctypes.MarketTypeFuture:
		// 合约交易对配置
		symbolConfig, err := c.getSymbolConfig(ctx, _symbol, ctypes.MarketTypeFuture)
		if err != nil {
			return nil, err
		}
		if symbolConfig != nil {
			result.IsolatedMarginEnabled = symbolConfig.IsIsolated
			result.CrossMarginEnabled = !symbolConfig.IsIsolated
			result.IsAutoAddMargin = symbolConfig.IsAutoAddMargin
			result.CrossLeverage[0] = symbolConfig.Leverage
			result.CrossLeverage[1] = symbolConfig.Leverage
			result.MaxNotionalValue = symbolConfig.MaxNotionalValue
		}

		// 获取指定交易对的佣金费率
		if c.IsPortfolioMode() {
			data, err := c.cache.Get(ctx, key, 5*time.Minute, func(ctx context.Context, params ...any) (any, error) {
				return c.portfolioClient.NewGetUMCommissionRateService().Symbol(_symbol).Do(ctx)
			})
			if err != nil {
				return nil, err
			}
			response, ok := data.(*portfolio.CommissionRate)
			if !ok {
				return nil, fmt.Errorf("invalid commission rates: %T", data)
			}
			result.MakerCommission = number.DecimalFromString(response.MakerCommissionRate)
			result.TakerCommission = number.DecimalFromString(response.TakerCommissionRate)
		} else {
			data, err := c.cache.Get(ctx, key, 5*time.Minute, func(ctx context.Context, params ...any) (any, error) {
				return c.futureClient.NewCommissionRateService().Symbol(_symbol).Do(ctx)
			})
			if err != nil {
				return nil, err
			}
			response, ok := data.(*futures.CommissionRate)
			if !ok {
				return nil, fmt.Errorf("invalid commission rates: %T", data)
			}
			result.MakerCommission = number.DecimalFromString(response.MakerCommissionRate)
			result.TakerCommission = number.DecimalFromString(response.TakerCommissionRate)
		}
	}
	return result, nil
}

// CalcOrderFee 根据订单信息计算手续费（maker/taker 费率）
func (c *Connector) CalcOrderFee(ctx context.Context, order ctypes.Order) (*decimal.Decimal, *string, error) {
	if !order.Symbol.IsValid() {
		return nil, nil, fmt.Errorf("invalid order symbol")
	}
	if order.ExecutedQty.LessThanOrEqual(decimal.Zero) && order.ExecutedQuoteQty.LessThanOrEqual(decimal.Zero) {
		return nil, nil, nil
	}

	symbolCfg, err := c.SymbolConfig(ctx, order.Symbol)
	if err != nil {
		return nil, nil, err
	}
	if symbolCfg == nil {
		return nil, nil, fmt.Errorf("symbol config not found")
	}

	fee, asset, err := c.calcOrderFee(ctx, order, symbolCfg)
	if err != nil {
		return nil, nil, err
	}
	return fee, asset, nil
}

func (c *Connector) calcOrderFee(ctx context.Context, order ctypes.Order, symbolConfig *ctypes.SymbolConfig) (*decimal.Decimal, *string, error) {
	if symbolConfig == nil {
		return nil, nil, fmt.Errorf("symbol config not found")
	}
	if !order.Symbol.IsValid() {
		return nil, nil, fmt.Errorf("invalid order symbol")
	}
	if order.ExecutedQty.LessThanOrEqual(decimal.Zero) && order.ExecutedQuoteQty.LessThanOrEqual(decimal.Zero) {
		return nil, nil, nil
	}

	feeRate := symbolConfig.TakerCommission
	if order.PostOnly {
		feeRate = symbolConfig.MakerCommission
	}
	if feeRate.IsZero() {
		return nil, nil, nil
	}

	executedQuoteQty := order.ExecutedQuoteQty
	if executedQuoteQty.LessThanOrEqual(decimal.Zero) && order.ExecutedQty.GreaterThan(decimal.Zero) && order.AvgPrice.GreaterThan(decimal.Zero) {
		executedQuoteQty = order.ExecutedQty.Mul(order.AvgPrice)
	}

	switch order.Symbol.Type {
	case ctypes.MarketTypeSpot:
		if order.IsBuy {
			return lo.ToPtr(order.ExecutedQty.Mul(feeRate).Neg()), lo.ToPtr(order.Symbol.Base), nil
		}
		return lo.ToPtr(executedQuoteQty.Mul(feeRate).Neg()), lo.ToPtr(order.Symbol.Quote), nil
	case ctypes.MarketTypeFuture:
		return lo.ToPtr(executedQuoteQty.Mul(feeRate).Neg()), lo.ToPtr(order.Symbol.Quote), nil
	}
	return nil, nil, fmt.Errorf("unsupported market type: %s", order.Symbol.Type)
}

func (c *Connector) calculateOrder(ctx context.Context, order *ctypes.Order) {
	if order == nil {
		return
	}

	symbolCfg, err := c.SymbolConfig(ctx, order.Symbol)
	if err != nil {
		logger.Ctx(ctx).Err(err).Msg("get symbol config failed")
		return
	}
	if symbolCfg == nil {
		logger.Ctx(ctx).Error().Str("symbol", order.Symbol.String()).Msg("symbol config not found")
		return
	}

	// 计算交易已产生的手续费（仅计算已成交部分）
	if order.Fee == nil && order.ExecutedQty.GreaterThan(decimal.Zero) {
		fee, asset, err := c.calcOrderFee(ctx, *order, symbolCfg)
		if err != nil {
			logger.Ctx(ctx).Err(err).Msg("calc order fee failed")
			return
		}
		order.Fee = fee
		order.FeeAsset = asset
	}
}

func (c *Connector) Positions(ctx context.Context, mt *ctypes.MarketType) ([]*ctypes.Position, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	account, err := c.getAccountInfo(ctx)
	if err != nil {
		return nil, err
	}

	if mt != nil {
		return c.getPositions(ctx, account, *mt)
	}

	positions := make([]*ctypes.Position, 0)
	mu := sync.Mutex{}

	errgrp := errgroup.Group{}
	errgrp.Go(func() error {
		pos, err := c.getPositions(ctx, account, ctypes.MarketTypeFuture)
		if err != nil {
			return err
		}
		mu.Lock()
		positions = append(positions, pos...)
		mu.Unlock()
		return nil
	})

	if err := errgrp.Wait(); err != nil {
		return nil, err
	}

	return positions, nil
}

func (c *Connector) getPositions(ctx context.Context, account *Account, mt ctypes.MarketType) ([]*ctypes.Position, error) {
	positions := make([]*ctypes.Position, 0)

	switch mt {
	case ctypes.MarketTypeFuture:
		if account.IsFutureEnabled && !account.IsPortfolioMode {
			// 查询合约账户仓位风险 /fapi/v1/positionRisk
			futureRisk, err := fetchWithType[[]*futures.PositionRiskV3](c, ctx, CacheKeyFuturePositionRisk)
			if err != nil {
				return nil, err
			}

			for _, risk := range futureRisk {
				symbol, err := c.ParseSymbol(ctx, risk.Symbol, ctypes.MarketTypeFuture)
				if err != nil {
					return nil, err
				}
				symbolConfig, err := c.getSymbolConfig(ctx, risk.Symbol, ctypes.MarketTypeFuture)
				if err != nil {
					return nil, err
				}
				position := &FuturePosition{
					symbol:           symbol,
					side:             ConvertPositionSide2Types(risk.PositionSide),
					amount:           number.DecimalFromString(risk.PositionAmt).Abs(),
					entryPrice:       number.DecimalFromString(risk.EntryPrice),
					markPrice:        number.DecimalFromString(risk.MarkPrice),
					liquidationPrice: number.DecimalFromString(risk.LiquidationPrice),
					notional:         number.DecimalFromString(risk.Notional).Abs(),
					leverage:         symbolConfig.Leverage,
					initialMargin:    number.DecimalFromString(risk.InitialMargin),
					maintMargin:      number.DecimalFromString(risk.MaintMargin),
					unRealizedProfit: number.DecimalFromString(risk.UnRealizedProfit),
					updateTime:       time.UnixMilli(risk.UpdateTime),
				}

				positions = append(positions, position.ToTypesPosition())
			}
		}

		if account.IsPortfolioMode && account.ApiKeyPermission.EnablePortfolioMarginTrading {
			// 统一账户-用户UM持仓风险 /papi/v1/um/positionRisk
			positionRisk, err := fetchWithType[[]*portfolio.UMPositionRisk](c, ctx, CacheKeyUMPositionRisk)
			if err != nil {
				return nil, err
			}
			for _, risk := range positionRisk {
				symbol, err := c.ParseSymbol(ctx, risk.Symbol, ctypes.MarketTypeFuture)
				if err != nil {
					return nil, err
				}

				symbolConfig, err := c.getSymbolConfig(ctx, risk.Symbol, ctypes.MarketTypeFuture)
				if err != nil {
					return nil, err
				}

				amount := number.DecimalFromString(risk.PositionAmt).Abs()
				entryPrice := number.DecimalFromString(risk.EntryPrice)
				markPrice := number.DecimalFromString(risk.MarkPrice)
				notional := number.DecimalFromString(risk.Notional).Abs()

				leverageBracket, err := c.GetLeverageBracket(ctx, symbol, markPrice)
				if err != nil {
					return nil, err
				}

				leverage := decimal.NewFromInt(int64(symbolConfig.Leverage))
				initialMargin := calculator.CalcInitialMargin(notional, leverage)
				maintMargin := calculator.CalcMaintMargin(notional, leverageBracket)
				position := &FuturePosition{
					symbol:           symbol,
					side:             ConvertPositionSide2Types(risk.PositionSide),
					amount:           amount,
					entryPrice:       entryPrice,
					markPrice:        markPrice,
					liquidationPrice: number.DecimalFromString(risk.LiquidationPrice),
					notional:         notional,
					leverage:         symbolConfig.Leverage,
					initialMargin:    initialMargin,
					maintMargin:      maintMargin,
					unRealizedProfit: number.DecimalFromString(risk.UnRealizedProfit),
					updateTime:       time.UnixMilli(risk.UpdateTime),
				}
				positions = append(positions, position.ToTypesPosition())
			}
		}
	default:
		return nil, fmt.Errorf("unsupported market type: %s", mt)
	}
	return positions, nil
}

func (c *Connector) GetOrders(ctx context.Context, symbol *ctypes.Symbol) ([]*ctypes.Order, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	if symbol != nil {
		symbolStr := FormatSymbol(*symbol)
		return c.getOpenOrders(ctx, &symbolStr, symbol.Type)
	}

	result := make([]*ctypes.Order, 0)
	errgrp := errgroup.Group{}
	errgrp.Go(func() error {
		orders, err := c.getOpenOrders(ctx, nil, ctypes.MarketTypeSpot)
		if err != nil {
			return err
		}
		result = append(result, orders...)
		return nil
	})
	errgrp.Go(func() error {
		orders, err := c.getOpenOrders(ctx, nil, ctypes.MarketTypeFuture)
		if err != nil {
			return err
		}
		result = append(result, orders...)
		return nil
	})

	if err := errgrp.Wait(); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Connector) getOpenOrders(ctx context.Context, symbol *string, tp ctypes.MarketType) ([]*ctypes.Order, error) {
	result := make([]*ctypes.Order, 0)
	switch tp {
	case ctypes.MarketTypeSpot:
		service := c.spotClient.NewListOpenOrdersService()
		if symbol != nil {
			service = service.Symbol(*symbol)
		}
		orders, err := service.Do(ctx)
		if err != nil {
			return nil, err
		}
		for _, order := range orders {
			symbol, err := c.ParseSymbol(ctx, order.Symbol, ctypes.MarketTypeSpot)
			if err != nil {
				return nil, err
			}
			o, err := c.ConvertSpotOrder2Types(ctx, symbol, order)
			if err != nil {
				return nil, err
			}
			result = append(result, o)
		}
	case ctypes.MarketTypeFuture:
		mu := sync.Mutex{}
		group := errgroup.Group{}
		if c.IsPortfolioMode() {
			group.Go(func() error {
				service := c.portfolioClient.NewUMOpenConditionalOrdersService()
				if symbol != nil {
					service = service.Symbol(*symbol)
				}
				orders, err := service.Do(ctx)
				if err != nil {
					return err
				}
				for _, order := range orders {
					symbol, err := c.ParseSymbol(ctx, order.Symbol, ctypes.MarketTypeFuture)
					if err != nil {
						return err
					}
					o, err := c.ConvertPortfolioUMAlgoOpenOrder2Types(ctx, symbol, order)
					if err != nil {
						return err
					}
					mu.Lock()
					result = append(result, o)
					mu.Unlock()
				}
				return nil
			})
			group.Go(func() error {
				service := c.portfolioClient.NewUMOpenOrdersService()
				if symbol != nil {
					service = service.Symbol(*symbol)
				}
				orders, err := service.Do(ctx)
				if err != nil {
					return err
				}
				for _, order := range orders {
					symbol, err := c.ParseSymbol(ctx, order.Symbol, ctypes.MarketTypeFuture)
					if err != nil {
						return err
					}
					o, err := c.ConvertPortfolioUMOpenOrder2Types(ctx, symbol, order)
					if err != nil {
						return err
					}
					mu.Lock()
					result = append(result, o)
					mu.Unlock()
				}
				return nil
			})
		} else {
			group.Go(func() error {
				service := c.futureClient.NewListOpenAlgoOrdersService()
				if symbol != nil {
					service = service.Symbol(*symbol)
				}
				orders, err := service.Do(ctx)
				if err != nil {
					return err
				}
				for _, order := range orders {
					symbol, err := c.ParseSymbol(ctx, order.Symbol, ctypes.MarketTypeFuture)
					if err != nil {
						return err
					}
					o, err := c.ConvertFutureAlgoOrder2Types(ctx, symbol, &order)
					if err != nil {
						return err
					}
					mu.Lock()
					result = append(result, o)
					mu.Unlock()
				}
				return nil
			})
			group.Go(func() error {
				service := c.futureClient.NewListOpenOrdersService()
				if symbol != nil {
					service = service.Symbol(*symbol)
				}
				orders, err := service.Do(ctx)
				if err != nil {
					return err
				}
				for _, order := range orders {
					symbol, err := c.ParseSymbol(ctx, order.Symbol, ctypes.MarketTypeFuture)
					if err != nil {
						return err
					}
					o, err := c.ConvertFutureOrder2Types(ctx, symbol, order)
					if err != nil {
						return err
					}
					mu.Lock()
					result = append(result, o)
					mu.Unlock()
				}
				return nil
			})
		}

		if err := group.Wait(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (c *Connector) GetHisOrders(ctx context.Context, symbol ctypes.Symbol, startTs time.Time, endTs time.Time, limit int) ([]*ctypes.Order, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	result := make([]*ctypes.Order, 0)

	symbolStr := FormatSymbol(symbol)
	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		orders, err := c.spotClient.NewListOrdersService().Symbol(symbolStr).StartTime(startTs.UnixMilli()).EndTime(endTs.UnixMilli()).Limit(limit).Do(ctx)
		if err != nil {
			return nil, err
		}
		for _, order := range orders {
			order, err := c.ConvertSpotOrder2Types(ctx, symbol, order)
			if err != nil {
				return nil, err
			}
			result = append(result, order)
		}
	case ctypes.MarketTypeFuture:
		mu := sync.Mutex{}
		group := errgroup.Group{}
		if c.IsPortfolioMode() {
			group.Go(func() error {
				// 统一账户-用户UM订单 /papi/v1/um/order
				orders, err := c.portfolioClient.NewUMAllOrdersService().Symbol(symbolStr).StartTime(startTs.UnixMilli()).EndTime(endTs.UnixMilli()).Limit(limit).Do(ctx)
				if err != nil {
					return err
				}
				ords := make([]*ctypes.Order, 0)
				for _, order := range orders {
					order, err := c.ConvertPortfolioUMAllOrder2Types(ctx, symbol, order)
					if err != nil {
						return err
					}
					ords = append(ords, order)
				}
				mu.Lock()
				result = append(result, ords...)
				mu.Unlock()
				return nil
			})
			group.Go(func() error {
				// 条件订单
				algoOrders, err := c.portfolioClient.NewUMAllConditionalOrdersService().Symbol(symbolStr).StartTime(startTs.UnixMilli()).EndTime(endTs.UnixMilli()).Limit(limit).Do(ctx)
				if err != nil {
					return err
				}
				ords := make([]*ctypes.Order, 0)
				for _, order := range algoOrders {
					order, err := c.ConvertPortfolioUMAlgoOrder2Types(ctx, symbol, order)
					if err != nil {
						return err
					}
					ords = append(ords, order)
				}
				mu.Lock()
				result = append(result, ords...)
				mu.Unlock()
				return nil
			})
		} else {
			group.Go(func() error {
				// 普通订单
				orders, err := c.futureClient.NewListOrdersService().Symbol(symbolStr).StartTime(startTs.UnixMilli()).EndTime(endTs.UnixMilli()).Limit(limit).Do(ctx)
				if err != nil {
					return err
				}
				ords := make([]*ctypes.Order, 0)
				for _, order := range orders {
					order, err := c.ConvertFutureOrder2Types(ctx, symbol, order)
					if err != nil {
						return err
					}
					ords = append(ords, order)
				}
				mu.Lock()
				result = append(result, ords...)
				mu.Unlock()
				return nil
			})
			group.Go(func() error {
				// 条件订单
				algoOrders, err := c.futureClient.NewListAllAlgoOrdersService().Symbol(symbolStr).StartTime(startTs.UnixMilli()).EndTime(endTs.UnixMilli()).Limit(limit).Do(ctx)
				if err != nil {
					return err
				}
				ords := make([]*ctypes.Order, 0)
				for _, order := range algoOrders {
					order, err := c.ConvertFutureAlgoOrder2Types(ctx, symbol, &order)
					if err != nil {
						return err
					}
					ords = append(ords, order)
				}
				mu.Lock()
				result = append(result, ords...)
				mu.Unlock()
				return nil
			})
		}
		if err := group.Wait(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
	return result, nil
}

func (c *Connector) GetOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	orderIdInt, err := strconv.ParseInt(orderId, 10, 64)
	if err != nil {
		return nil, err
	}

	symbolStr := FormatSymbol(symbol)
	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		order, err := c.spotClient.NewGetOrderService().Symbol(symbolStr).OrderID(orderIdInt).Do(ctx)
		if err != nil {
			return nil, err
		}
		result, err := c.ConvertSpotOrder2Types(ctx, symbol, order)
		if err != nil {
			return nil, err
		}
		return result, nil
	case ctypes.MarketTypeFuture:
		var result *ctypes.Order
		var err error
		wg := sync.WaitGroup{}
		wg.Add(2)
		if c.IsPortfolioMode() {
			go func() {
				defer wg.Done()
				var orders []*portfolio.UMAllOrdersResponse
				orders, err := c.portfolioClient.NewUMAllOrdersService().Symbol(symbolStr).OrderID(orderIdInt).Do(ctx)
				if err != nil {
					return
				}
				for _, order := range orders {
					if order.OrderID == orderIdInt { // 接口有bug，会返回其他订单，这里过滤下
						result, err = c.ConvertPortfolioUMAllOrder2Types(ctx, symbol, order)
					}
				}
			}()
			// 条件订单
			go func() {
				defer wg.Done()
				var algoOrders []*portfolio.UMConditionalOrderResponse
				algoOrders, err := c.portfolioClient.NewUMAllConditionalOrdersService().Symbol(symbolStr).StrategyID(orderIdInt).Do(ctx)
				if err != nil {
					return
				}
				for _, order := range algoOrders {
					if order.StrategyID == orderIdInt {
						result, err = c.ConvertPortfolioUMAlgoOrder2Types(ctx, symbol, order)
					}
				}
			}()
		} else {
			go func() {
				// 普通订单
				defer wg.Done()
				var orders []*futures.Order
				orders, err = c.futureClient.NewListOrdersService().Symbol(symbolStr).OrderID(orderIdInt).Do(ctx)
				if err != nil {
					return
				}
				for _, order := range orders {
					if order.OrderID == orderIdInt {
						result, err = c.ConvertFutureOrder2Types(ctx, symbol, order)
					}
				}
			}()
			go func() {
				// 条件订单
				defer wg.Done()
				var algoOrders []futures.GetAlgoOrderResp
				algoOrders, err = c.futureClient.NewListAllAlgoOrdersService().Symbol(symbolStr).AlgoID(orderIdInt).Do(ctx)
				if err != nil {
					return
				}
				for _, order := range algoOrders {
					if order.AlgoId == orderIdInt {
						result, err = c.ConvertFutureAlgoOrder2Types(ctx, symbol, &order)
					}
				}
			}()
		}
		wg.Wait()
		if result != nil {
			return result, nil
		}
		return nil, err
	default:
		return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
}
