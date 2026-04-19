package okx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/wangliang139/NovaForge/server/pkg/market/connector/cache"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/market/types"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/mow/logger"
	"github.com/wangliang139/mow/number"
	okx "github.com/wangliang139/okx-connector-go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/sync/errgroup"
)

type Config struct {
	ApiKey     string
	ApiSecret  string
	Passphrase string

	UseTestnet bool
	ProxyURL   string
	Buffer     int
}

func (c *Config) applyDefaults() {
	if c.Buffer <= 0 {
		c.Buffer = 256
	}
}

type Connector struct {
	cfg Config

	account  *mdtypes.ApiAccount
	exchange ctypes.Exchange

	mode  mdtypes.ConnectMode
	mu    sync.RWMutex
	cache *cache.Cache

	wsClient   *okx.WebsocketStreamClient
	restClient *okx.Client
}

var _ mdtypes.Connector = &Connector{}

func New(cfg Config, account *mdtypes.ApiAccount) (*Connector, error) {
	cfg.applyDefaults()

	var apiKey, apiSecret, passphrase string
	if account != nil {
		apiKey = account.ApiKey
		apiSecret = account.ApiSecret
		passphrase = account.Passphrase

		if account.DescryptFn != nil {
			var err error
			apiSecret, err = account.DescryptFn(apiSecret)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt api secret: %w", err)
			}
		}
	}

	mode := mdtypes.ConnectModePublic
	if account != nil {
		mode = mdtypes.ConnectModePrivate
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

	restClient := okx.NewClient(
		okx.WithApiAPIAuth(apiKey, apiSecret, passphrase),
		okx.WithApiHTTPClient(&http.Client{
			Transport: otelhttp.NewTransport(transport),
		}),
		okx.WithApiIsTestNet(cfg.UseTestnet),
	)

	wsOptions := []func(*okx.WebsocketStreamClient){
		okx.WithWsAPIAuth(apiKey, apiSecret, passphrase),
		okx.WithWsIsTestNet(cfg.UseTestnet),
	}
	if len(cfg.ProxyURL) > 0 {
		wsOptions = append(wsOptions, okx.WithWsProxyURL(cfg.ProxyURL))
	}
	wsClient := okx.NewWsStreamClient(wsOptions...)

	exchange := ctypes.ExchangeOkx
	if cfg.UseTestnet {
		exchange = ctypes.ExchangeOkxTest
	}

	c := &Connector{
		cfg:        cfg,
		mode:       mode,
		account:    account,
		exchange:   exchange,
		cache:      cache.NewCache(5*time.Minute, 10*time.Minute),
		wsClient:   wsClient,
		restClient: restClient,
	}
	c.SetupCache()
	return c, nil
}

func (c *Connector) IsPrivate() bool {
	return c.mode == mdtypes.ConnectModePrivate
}

func (c *Connector) PlaceOrder(ctx context.Context, input mdtypes.PlaceOrderInput) (*mdtypes.PlaceOrderResult, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}
	if !input.Symbol.IsValid() {
		return nil, fmt.Errorf("invalid symbol: %s", input.Symbol)
	}
	if !input.OrderType.Valid() {
		return nil, fmt.Errorf("invalid order type: %s", input.OrderType)
	}

	instId := Symbol2InstId(input.Symbol)

	ordType := ""
	switch input.OrderType {
	case ctypes.OrderTypeMarket:
		ordType = "market"
	case ctypes.OrderTypeLimit:
		ordType = "limit"
	default:
		return nil, fmt.Errorf("unsupported order type: %s", input.OrderType)
	}

	side := "sell"
	if input.IsBuy {
		side = "buy"
	}

	tdMode := "cash"
	posSide := ""
	if input.Symbol.Type == ctypes.MarketTypeFuture {
		tdMode = "cross"
		switch input.Side {
		case ctypes.PositionSideLong:
			posSide = "long"
		case ctypes.PositionSideShort:
			posSide = "short"
		default:
			return nil, fmt.Errorf("invalid position side: %s", input.Side)
		}
	}

	var px string
	if input.Price != nil {
		px = input.Price.String()
	}

	var sz string
	var tgtCcy string
	switch input.Symbol.Type {
	case ctypes.MarketTypeSpot:
		// OKX spot: sz 默认以 base 为单位；如需以 quote 下单，需要 tgtCcy=quote_ccy。
		if input.Quantity != nil {
			if !input.Quantity.GreaterThan(decimal.Zero) {
				return nil, fmt.Errorf("quantity must be > 0")
			}
			sz = input.Quantity.String()
			tgtCcy = "base_ccy"
		} else if input.QuoteQty != nil {
			if !input.QuoteQty.GreaterThan(decimal.Zero) {
				return nil, fmt.Errorf("quote_qty must be > 0")
			}
			sz = input.QuoteQty.String()
			tgtCcy = "quote_ccy"
		} else {
			return nil, fmt.Errorf("quantity or quote_qty is required")
		}
	case ctypes.MarketTypeFuture:
		if input.Quantity == nil {
			return nil, fmt.Errorf("quantity is required for future order")
		}
		if !input.Quantity.GreaterThan(decimal.Zero) {
			return nil, fmt.Errorf("quantity must be > 0")
		}
		metadata, err := c.getMarketSymbol(context.Background(), input.Symbol)
		if err != nil {
			return nil, fmt.Errorf("get market symbol: %w", err)
		}
		ctVal := number.DecimalFromString(metadata.CtVal)
		if !ctVal.GreaterThan(decimal.Zero) {
			return nil, fmt.Errorf("invalid ctVal: %s", metadata.CtVal)
		}
		contracts := input.Quantity.Div(ctVal)
		sz = contracts.String()
	default:
		return nil, fmt.Errorf("unsupported market type: %s", input.Symbol.Type)
	}

	svc := c.restClient.NewPlaceOrderService().
		InstId(instId).
		TdMode(tdMode).
		Side(side).
		OrdType(ordType).
		Sz(sz).
		Tag("novaforge")

	if input.ClientOrderID != nil && strings.TrimSpace(input.ClientOrderID.String()) != "" {
		svc = svc.ClOrdId(strings.TrimSpace(input.ClientOrderID.String()))
	}
	if input.Symbol.Type == ctypes.MarketTypeFuture {
		svc = svc.PosSide(posSide)
	}
	if input.Symbol.Type == ctypes.MarketTypeSpot && tgtCcy != "" {
		svc = svc.TgtCcy(tgtCcy)
	}
	if px != "" {
		svc = svc.Px(px)
	}
	if input.ReduceOnly != nil {
		svc = svc.ReduceOnly(*input.ReduceOnly)
	}

	results, err := svc.Do(ctx)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 || results[0] == nil {
		return nil, fmt.Errorf("empty okx place order result")
	}
	if results[0].SCode != "0" {
		return nil, fmt.Errorf("okx place order failed: %s", results[0].SMsg)
	}
	return &mdtypes.PlaceOrderResult{
		OrderID:       ctypes.OrderId(results[0].OrdId),
		ClientOrderID: ctypes.OrderId(results[0].ClOrdId),
		Status:        ctypes.OrderStatusNew,
	}, nil
}

func (c *Connector) CancelOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) error {
	if !c.IsPrivate() {
		return fmt.Errorf("connector is not private mode")
	}
	if !symbol.IsValid() {
		return fmt.Errorf("invalid symbol: %s", symbol)
	}
	if strings.TrimSpace(orderId) == "" {
		return fmt.Errorf("order_id is required")
	}

	instId := Symbol2InstId(symbol)
	results, err := c.restClient.NewCancelOrderService().InstId(instId).OrdId(orderId).Do(ctx)
	if err == nil && len(results) > 0 && results[0] != nil && results[0].SCode == "0" {
		return nil
	}
	// 普通撤单失败时，尝试按算法单撤销（algoId）。
	algoRes, algoErr := c.restClient.NewCancelAlgoOrderService().Orders([]okx.CancelAlgoOrderRequest{
		{AlgoId: orderId, InstId: instId},
	}).Do(ctx)
	if algoErr == nil && len(algoRes) > 0 && algoRes[0] != nil && algoRes[0].SCode == "0" {
		return nil
	}
	if err != nil {
		return err
	}
	if len(results) > 0 && results[0] != nil && results[0].SMsg != "" {
		return fmt.Errorf("okx cancel order failed: %s", results[0].SMsg)
	}
	if algoErr != nil {
		return algoErr
	}
	if len(algoRes) > 0 && algoRes[0] != nil && algoRes[0].SMsg != "" {
		return fmt.Errorf("okx cancel algo order failed: %s", algoRes[0].SMsg)
	}
	return fmt.Errorf("okx cancel order failed")
}

func (c *Connector) Exchange() ctypes.Exchange {
	return c.exchange
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

func (c *Connector) Subscribe(ctx context.Context, selector ctypes.StreamSelector) (*mdtypes.StreamHandle, error) {
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

func (c *Connector) subscribeTicker(ctx context.Context, selector ctypes.StreamSelector) (*mdtypes.StreamHandle, error) {
	symbol := selector.Symbol
	if symbol == nil {
		return nil, fmt.Errorf("symbol is required")
	}
	streamSymbol := Symbol2InstId(*symbol)
	out := make(chan *ctypes.Message, c.cfg.Buffer)
	errCh := make(chan error, 1)

	doneC, stopC, err := c.wsClient.WsTickerServe(ctx, []string{streamSymbol}, func(evt *okx.WsTickerEvent) {
		for _, tk := range evt.Data {
			ticker, nerr := c.ConvertTicker2Types(context.Background(), c.Exchange(), *symbol, &tk)
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
	return mdtypes.BuildHandle(ctx, out, errCh, stopC, doneC), nil
}

func (c *Connector) subscribeTrade(ctx context.Context, selector ctypes.StreamSelector) (*mdtypes.StreamHandle, error) {
	symbol := selector.Symbol
	if symbol == nil {
		return nil, fmt.Errorf("symbol is required")
	}
	streamSymbol := Symbol2InstId(*symbol)
	out := make(chan *ctypes.Message, c.cfg.Buffer)
	errCh := make(chan error, 1)
	doneC, stopC, err := c.wsClient.WsTradeServe(ctx, []string{streamSymbol}, func(evt *okx.WsTradeEvent) {
		for _, td := range evt.Data {
			trade, nerr := ConvertTrade2Types(c.Exchange(), *symbol, &td)
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
	return mdtypes.BuildHandle(ctx, out, errCh, stopC, doneC), nil
}

func (c *Connector) subscribeDepth(ctx context.Context, selector ctypes.StreamSelector) (*mdtypes.StreamHandle, error) {
	symbol := selector.Symbol
	if symbol == nil {
		return nil, fmt.Errorf("symbol is required")
	}
	streamSymbol := Symbol2InstId(*symbol)
	out := make(chan *ctypes.Message, c.cfg.Buffer)
	errCh := make(chan error, 1)
	doneC, stopC, err := c.wsClient.WsDepthServe(ctx, []string{streamSymbol}, okx.DepthChannelBooks, func(evt *okx.WsDepthEvent) {
		// ignore snapshot data
		if evt.Action == "snapshot" {
			return
		}
		for _, dp := range evt.Data {
			book, nerr := ConvertDepth2Types(c.Exchange(), *symbol, &dp)
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
	return mdtypes.BuildHandle(ctx, out, errCh, stopC, doneC), nil
}

func (c *Connector) subscribeKline(ctx context.Context, selector ctypes.StreamSelector) (*mdtypes.StreamHandle, error) {
	symbol := selector.Symbol
	if symbol == nil {
		return nil, fmt.Errorf("symbol is required")
	}
	if selector.Interval == nil {
		return nil, fmt.Errorf("kline selector requires interval")
	}
	itv, err := ParseInterval(*selector.Interval)
	if err != nil {
		return nil, err
	}
	channel := itv.ToOkxKlineChannel()
	if !channel.Valid() {
		return nil, fmt.Errorf("invalid kline channel: %s", channel)
	}
	streamSymbol := Symbol2InstId(*symbol)
	out := make(chan *ctypes.Message, c.cfg.Buffer)
	errCh := make(chan error, 1)
	doneC, stopC, err := c.wsClient.WsKlineServe(ctx, []string{streamSymbol}, channel, func(evt *okx.WsKlineEvent) {
		for _, k := range evt.Data {
			kline, nerr := ConvertKline2Types(c.Exchange(), *symbol, *selector.Interval, k)
			if nerr != nil {
				select {
				case errCh <- nerr:
				default:
				}
				return
			}
			now := time.Now()
			if now.After(kline.CloseTs) {
				now = kline.CloseTs
			}
			msg := ctypes.NewMessage(c.Exchange(), selector, kline, now)
			select {
			case <-ctx.Done():
			case out <- msg:
			}
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
	return mdtypes.BuildHandle(ctx, out, errCh, stopC, doneC), nil
}

func (c *Connector) subscribeMarkPrice(ctx context.Context, selector ctypes.StreamSelector) (*mdtypes.StreamHandle, error) {
	symbol := selector.Symbol
	if symbol == nil {
		return nil, fmt.Errorf("symbol is required")
	}
	streamSymbol := Symbol2InstId(*symbol)
	out := make(chan *ctypes.Message, c.cfg.Buffer)
	errCh := make(chan error, 1)
	// 控制 mark price 事件输出频率：最多每秒一次（同一个 selector 下超频事件丢弃）
	var (
		throttleMu sync.Mutex
		lastEmitAt time.Time
	)
	doneC, stopC, err := c.wsClient.WsMarkPriceServe(ctx, []string{streamSymbol}, func(evt *okx.WsMarkPriceEvent) {
		for _, mp := range evt.Data {
			now := time.Now()
			throttleMu.Lock()
			if !lastEmitAt.IsZero() && now.Sub(lastEmitAt) < time.Second {
				throttleMu.Unlock()
				continue
			}
			lastEmitAt = now
			throttleMu.Unlock()
			markPrice, nerr := ConvertMarkPrice2Types(c.Exchange(), *symbol, &mp)
			if nerr != nil {
				select {
				case errCh <- nerr:
				default:
				}
				return
			}
			msg := ctypes.NewMessage(c.Exchange(), selector, markPrice, markPrice.Ts)
			select {
			case <-ctx.Done():
			case out <- msg:
			}
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
	return mdtypes.BuildHandle(ctx, out, errCh, stopC, doneC), nil
}

func (c *Connector) subscribeAccount(ctx context.Context, _ ctypes.StreamSelector) (*mdtypes.StreamHandle, error) {
	out := make(chan *ctypes.Message, c.cfg.Buffer)
	errCh := make(chan error, 1)
	handler := &AccountEventHandler{
		c:     c,
		outCh: out,
		errCh: errCh,
	}

	// 用户数据流
	userDataDoneC, userDataStopC, err := c.wsClient.WsUserDataServe(ctx, handler, func(wsErr error) {
		select {
		case errCh <- wsErr:
		default:
		}
	})
	if err != nil {
		return nil, err
	}

	// 算法订单流
	algoOrderDoneC, algoOrderStopC, err := c.wsClient.WsBusinessDataServe(ctx, handler, func(wsErr error) {
		select {
		case errCh <- wsErr:
		default:
		}
	})
	if err != nil {
		if userDataStopC != nil {
			close(userDataStopC)
		}
		return nil, err
	}

	// 合并 doneC / stopC：对外只暴露一组通道
	mergedDoneC := make(chan struct{})
	mergedStopC := make(chan struct{})

	var once sync.Once
	// 任一底层 doneC 关闭时，关闭 mergedDoneC
	go func() {
		if userDataDoneC != nil {
			<-userDataDoneC
			once.Do(func() {
				close(mergedDoneC)
			})
		}
	}()
	go func() {
		if algoOrderDoneC != nil {
			<-algoOrderDoneC
			once.Do(func() {
				close(mergedDoneC)
			})
		}
	}()

	// 对外 stop 会触发两路底层 stopC
	go func() {
		<-mergedStopC
		if userDataStopC != nil {
			close(userDataStopC)
		}
		if algoOrderStopC != nil {
			close(algoOrderStopC)
		}
	}()

	return mdtypes.BuildHandle(ctx, out, errCh, mergedStopC, mergedDoneC), nil
}

type AccountEventHandler struct {
	c     *Connector
	outCh chan *ctypes.Message
	errCh chan error
}

func (h *AccountEventHandler) send(msg *ctypes.Message) {
	select {
	case h.outCh <- msg:
	default:
	}
}

func (h *AccountEventHandler) HandleAccountEvent(evt *okx.WsAccountEvent) {
	if evt == nil || len(evt.Data) == 0 {
		return
	}

	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("channel", evt.Arg.Channel).
		Str("event_type", evt.EventType).
		Interface("event", evt).
		Msg("account event")

	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccountRaw,
		Account: lo.ToPtr(h.c.account.ID),
	}

	// 这里只处理快照事件，增量事件在 HandleBalanceAndPositionEvent 中处理
	if evt.EventType != "snapshot" {
		return
	}

	assets := ConvertBalance2Types(evt)
	if len(assets) == 0 {
		return
	}

	h.send(ctypes.NewMessage(
		h.c.Exchange(),
		selector,
		// &ctypes.BalanceSnapshot{
		// 	Scope:  []ctypes.WalletType{ctypes.WalletTypeTrade},
		// 	Assets: assets,
		// },
		&ctypes.BalanceUpdate{ // okx 存在bug，snapshot事件不完整，使用增量事件代替
			Type:   ctypes.UpdateTypeSnapshot,
			Reason: ctypes.LedgerReasonSnapshot,
			Assets: assets,
		},
		time.Now(),
	))
}

func (h *AccountEventHandler) HandlePositionEvent(evt *okx.WsPositionEvent) {
	if evt == nil || len(evt.Data) == 0 {
		return
	}

	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("channel", evt.Arg.Channel).
		Str("event_type", evt.EventType).
		Interface("event", evt).
		Msg("account event")

	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccountRaw,
		Account: lo.ToPtr(h.c.account.ID),
	}

	positions := make([]*ctypes.Position, 0)
	for _, p := range evt.Data {
		symbol, err := ConvertInstId2Symbol(p.InstId)
		if err != nil {
			log.Err(err).Str("inst_id", p.InstId).Msg("failed to convert inst id to symbol")
			continue
		}

		metadata, err := h.c.getMarketSymbol(context.Background(), symbol)
		if err != nil {
			log.Err(err).Str("inst_id", p.InstId).Msg("failed to get market symbol")
			continue
		}

		ctVal := number.DecimalFromString(metadata.CtVal)
		avgPrice := number.DecimalFromString(p.AvgPx)
		pos := number.DecimalFromString(p.Pos).Mul(ctVal)

		leverage := 0
		if len(p.Lever) > 0 {
			leverage, err = strconv.Atoi(p.Lever)
			if err != nil {
				log.Err(err).Str("inst_id", p.InstId).Msg("failed to parse leverage")
			}
		}

		side := ctypes.PositionSideLong
		switch p.PosSide {
		case "long":
			side = ctypes.PositionSideLong
		case "short":
			side = ctypes.PositionSideShort
		case "net":
			if pos.LessThan(decimal.Zero) {
				side = ctypes.PositionSideShort
			} else {
				side = ctypes.PositionSideLong
			}
		default:
			log.Err(err).Str("inst_id", p.InstId).Msg("failed to get position side")
			continue
		}

		uTime, err := strconv.ParseInt(p.UTime, 10, 64)
		if err != nil {
			log.Err(err).Str("inst_id", p.InstId).Msg("failed to parse u time")
			continue
		}

		position := &ctypes.Position{
			Symbol:     symbol,
			Side:       side,
			Isolated:   p.MgnMode == "isolated",
			Amount:     pos.Abs(),
			EntryPrice: avgPrice,
			Leverage:   leverage,
			UpdatedTs:  time.UnixMilli(uTime),
		}
		positions = append(positions, position)
	}

	if len(positions) == 0 {
		return
	}

	if evt.EventType == "snapshot" {
		h.send(ctypes.NewMessage(
			h.c.Exchange(),
			selector,
			&ctypes.PositionSnapshot{
				Positions: positions,
			},
			time.Now(),
		))
	} else {
		h.send(ctypes.NewMessage(
			h.c.Exchange(),
			selector,
			&ctypes.PositionsUpdate{
				Type:      ctypes.UpdateTypeSnapshot,
				Positions: positions,
			},
			time.Now(),
		))
	}
}

func (h *AccountEventHandler) HandleBalanceAndPositionEvent(evt *okx.WsBalanceAndPositionEvent) {
	if evt == nil || len(evt.Data) == 0 {
		return
	}

	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("channel", evt.Arg.Channel).
		Interface("event", evt).
		Msg("account event")

	selector := ctypes.StreamSelector{
		Stream:  ctypes.StreamTypeAccountRaw,
		Account: lo.ToPtr(h.c.account.ID),
	}

	for _, data := range evt.Data {
		switch data.EventType {
		case "snapshot": // 首推快照
			continue
		case "delivered": // 交割
		case "exercised": // 行权
		case "transferred": // 划转
		case "filled": // 成交
		case "liquidation": // 强平
		case "claw_back": // 穿仓补偿
		case "adl": // ADL自动减仓
		case "funding_fee": // 资金费
		case "adjust_margin": // 调整保证金
		case "set_leverage": // 设置杠杆
		case "interest_deduction": // 扣息
		case "settlement": // 交割结算
		}

		reason := data.EventType
		pTime, err := strconv.ParseInt(data.PTime, 10, 64)
		if err != nil {
			log.Err(err).Str("p_time", data.PTime).Msg("failed to parse p time")
			continue
		}

		if len(data.BalData) > 0 {
			assets := make([]*ctypes.AssetEvent, 0, len(data.BalData))
			for _, balance := range data.BalData {
				_balance := decimal.RequireFromString(balance.CashBal)
				uTime, err := strconv.ParseInt(balance.UTime, 10, 64)
				if err != nil {
					log.Err(err).Str("u_time", balance.UTime).Msg("failed to parse u time")
					continue
				}
				assets = append(assets, &ctypes.AssetEvent{
					WalletType: ctypes.WalletTypeTrade,
					Code:       balance.Ccy,
					Balance:    &_balance,
					UpdatedTs:  time.UnixMilli(uTime),
				})
			}
			if len(assets) > 0 {
				h.send(ctypes.NewMessage(
					h.c.Exchange(),
					selector,
					&ctypes.BalanceUpdate{
						Type:   ctypes.UpdateTypeSnapshot,
						Reason: ctypes.NormalizeLedgerReason(h.c.exchange, reason),
						Assets: assets,
					},
					time.UnixMilli(pTime),
				))
			}
		}

		// if len(data.PosData) > 0 {
		// 	positions := make([]*ctypes.Position, 0, len(data.PosData))
		// 	for _, p := range data.PosData {
		// 		symbol, err := ConvertInstId2Symbol(p.InstId)
		// 		if err != nil {
		// 			continue
		// 		}

		// 		metadata, err := h.c.getMarketSymbol(context.Background(), symbol)
		// 		if err != nil {
		// 			continue
		// 		}

		// 		ctVal := number.DecimalFromString(metadata.CtVal)
		// 		avgPrice := number.DecimalFromString(p.AvgPx)
		// 		pos := number.DecimalFromString(p.Pos).Mul(ctVal)

		// 		side := ctypes.PositionSideLong
		// 		switch p.PosSide {
		// 		case "long":
		// 			side = ctypes.PositionSideLong
		// 		case "short":
		// 			side = ctypes.PositionSideShort
		// 		case "net":
		// 			if pos.LessThan(decimal.Zero) {
		// 				side = ctypes.PositionSideShort
		// 			} else {
		// 				side = ctypes.PositionSideLong
		// 			}
		// 		default:
		// 			continue
		// 		}

		// 		uTime, err := strconv.ParseInt(p.UTime, 10, 64)
		// 		if err != nil {
		// 			continue
		// 		}

		// 		position := &ctypes.Position{
		// 			Symbol:     symbol,
		// 			Side:       side,
		// 			Isolated:   p.MgnMode == "isolated",
		// 			Amount:     pos.Abs(),
		// 			EntryPrice: avgPrice,
		// 			Leverage:   0, // 这里暂时不处理杠杆，因为杠杆是动态变化的
		// 			UpdatedTs:  time.UnixMilli(uTime),
		// 		}
		// 		positions = append(positions, position)
		// 	}
		// 	if len(positions) > 0 {
		// 		h.send(ctypes.NewMessage(
		// 			h.c.Exchange(),
		// 			selector,
		// 			&ctypes.PositionsUpdate{
		// 				Type:      ctypes.UpdateTypeSnapshot,
		// 				Reason:    reason,
		// 				Positions: positions,
		// 			},
		// 			time.UnixMilli(pTime),
		// 		))
		// 	}
		// }
	}
}

func (h *AccountEventHandler) HandleOrderEvent(evt *okx.WsOrderEvent) {
	if evt == nil || len(evt.Data) == 0 {
		return
	}

	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("channel", evt.Arg.Channel).
		Str("event_type", "order").
		Interface("event", evt).
		Msg("account event")

	for _, ord := range evt.Data {
		order, err := h.c.ConvertOrder2Types(context.Background(), &ord)
		if err != nil {
			log.Err(err).Msg("failed to convert order to types")
			continue
		}

		selector := ctypes.StreamSelector{
			Stream:  ctypes.StreamTypeAccountRaw,
			Symbol:  &order.Symbol,
			Account: lo.ToPtr(h.c.account.ID),
		}

		h.send(ctypes.NewMessage(
			h.c.Exchange(),
			selector,
			order,
			order.UpdatedTs,
		))
	}
}

func (h *AccountEventHandler) HandleAlgoOrderEvent(evt *okx.WsAlgoOrderEvent) {
	if evt == nil || len(evt.Data) == 0 {
		return
	}

	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("channel", evt.Arg.Channel).
		Str("event_type", "algo order").
		Interface("event", evt).
		Msg("account event")

	for _, ord := range evt.Data {
		order, err := h.c.ConvertOrder2Types(context.Background(), &ord)
		if err != nil {
			log.Err(err).Msg("failed to convert order to types")
			continue
		}

		selector := ctypes.StreamSelector{
			Stream:  ctypes.StreamTypeAccountRaw,
			Symbol:  &order.Symbol,
			Account: lo.ToPtr(h.c.account.ID),
		}

		h.send(ctypes.NewMessage(
			h.c.Exchange(),
			selector,
			order,
			order.UpdatedTs,
		))
	}
}

func (h *AccountEventHandler) HandleAdvancedAlgoOrderEvent(evt *okx.WsAdvancedAlgoOrderEvent) {
	if evt == nil || len(evt.Data) == 0 {
		return
	}

	log.Info().Str("account_id", h.c.account.ID).
		Str("exchange", h.c.Exchange().String()).
		Str("channel", evt.Arg.Channel).
		Str("event_type", "advanced algo order").
		Interface("event", evt).
		Msg("account event")

	for _, ord := range evt.Data {
		order, err := h.c.ConvertOrder2Types(context.Background(), &ord)
		if err != nil {
			log.Err(err).Msg("failed to convert order to types")
			continue
		}

		selector := ctypes.StreamSelector{
			Stream:  ctypes.StreamTypeAccountRaw,
			Symbol:  &order.Symbol,
			Account: lo.ToPtr(h.c.account.ID),
		}

		h.send(ctypes.NewMessage(
			h.c.Exchange(),
			selector,
			order,
			order.UpdatedTs,
		))
	}
}

func (c *Connector) GetMarket(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Market, error) {
	market, err := c.getMarketSymbol(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if market == nil {
		return nil, nil
	}
	return ConvertMarket2Types(c.Exchange(), market)
}

func (c *Connector) getMarketSymbol(ctx context.Context, symbol ctypes.Symbol) (*okx.SymbolInfo, error) {
	instId := Symbol2InstId(symbol)
	instType := FormatMarketType(symbol.Type)

	markets, err := c.getMarkets(ctx, instType)
	if err != nil {
		return nil, err
	}
	for _, market := range markets {
		if market.InstId == instId {
			return market, nil
		}
	}
	return nil, fmt.Errorf("no market symbol found")
}

func (c *Connector) GetMarkets(ctx context.Context, tps []ctypes.MarketType) ([]*ctypes.Market, error) {
	if len(tps) == 0 {
		return nil, fmt.Errorf("no market types provided")
	}

	ch := make(chan []*ctypes.Market, len(tps))
	grp, ctx := errgroup.WithContext(ctx)
	for _, tp := range tps {
		target := tp
		grp.Go(func() error {
			results := make([]*ctypes.Market, 0)
			instType := FormatMarketType(target)
			markets, err := c.getMarkets(ctx, instType)
			if err != nil {
				return err
			}
			for _, market := range markets {
				mkt, err := ConvertMarket2Types(c.Exchange(), market)
				if err != nil {
					continue
				}
				// 过滤跨币种保证金模式的合约
				if mkt.Symbol.Quote == "USD" || mkt.Symbol.Quote == "USD_UM" {
					continue
				}
				if mkt.Symbol.Type != target {
					continue
				}
				results = append(results, mkt)
			}
			ch <- results
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

func (c *Connector) getMarkets(ctx context.Context, instType string) ([]*okx.SymbolInfo, error) {
	return fetchWithType[[]*okx.SymbolInfo](c, ctx, CacheKeyMarkets, instType)
}

func (c *Connector) Ticker(ctx context.Context, symbol ctypes.Symbol) (*ctypes.Ticker, error) {
	result := &ctypes.Ticker{Symbol: symbol, Exchange: c.Exchange()}

	instId := Symbol2InstId(symbol)
	ticker, err := fetchWithType[*okx.Quotation](c, ctx, CacheKeyTicker, instId)
	if err != nil {
		return nil, err
	}
	if ticker == nil {
		return nil, nil
	}
	result.LastPrice, err = decimal.NewFromString(ticker.Last)
	if err != nil {
		return nil, fmt.Errorf("parse last price: %w", err)
	}
	result.Open24, err = decimal.NewFromString(ticker.Open24H)
	if err != nil {
		return nil, fmt.Errorf("parse open24: %w", err)
	}
	result.High24, err = decimal.NewFromString(ticker.High24H)
	if err != nil {
		return nil, fmt.Errorf("parse high24: %w", err)
	}
	result.Low24, err = decimal.NewFromString(ticker.Low24H)
	if err != nil {
		return nil, fmt.Errorf("parse low24: %w", err)
	}
	var volume24, quoteVolume24 decimal.Decimal
	if symbol.Type == ctypes.MarketTypeSpot {
		volume24, err = decimal.NewFromString(ticker.Vol24H)
		if err != nil {
			return nil, err
		}
		quoteVolume24, err = decimal.NewFromString(ticker.VolCcy24H)
		if err != nil {
			return nil, err
		}
	} else {
		metadata, err := c.getMarketSymbol(ctx, symbol)
		if err != nil {
			return nil, err
		}
		volume24, err = decimal.NewFromString(ticker.VolCcy24H)
		if err != nil {
			return nil, err
		}
		volume24 = volume24.Mul(number.DecimalFromString(metadata.CtVal))
		quoteVolume24, err = decimal.NewFromString(ticker.VolCcy24H)
		if err != nil {
			return nil, err
		}
	}
	result.Volume24 = volume24
	result.QuoteVolume24 = quoteVolume24
	result.Avg24 = quoteVolume24.Div(volume24)
	ts, err := strconv.ParseInt(ticker.Ts, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse ticker time: %w", err)
	}
	result.Ts = time.UnixMilli(ts)
	return result, nil
}

func (c *Connector) getTicker(ctx context.Context, symbol ctypes.Symbol) (*okx.Quotation, error) {
	instId := Symbol2InstId(symbol)
	return fetchWithType[*okx.Quotation](c, ctx, CacheKeyTicker, instId)
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
	ticker, err := c.getTicker(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if ticker == nil {
		return nil, nil
	}
	ts, err := strconv.ParseInt(ticker.Ts, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse ticker time: %w", err)
	}
	return &ctypes.Price{
		Exchange: c.Exchange(),
		Symbol:   symbol,
		Price:    number.DecimalFromString(ticker.Last),
		Ts:       time.UnixMilli(ts),
	}, nil
}

func (c *Connector) getPrices(ctx context.Context, marketType ctypes.MarketType) ([]*ctypes.Price, error) {
	var instType string
	switch marketType {
	case ctypes.MarketTypeSpot:
		instType = "SPOT"
	case ctypes.MarketTypeFuture:
		instType = "SWAP"
	}
	prices, err := fetchWithType[[]*okx.MarketTicker](c, ctx, CacheKeyTickers, instType)
	if err != nil {
		return nil, err
	}
	result := make([]*ctypes.Price, 0, len(prices))
	for _, price := range prices {
		symbol, err := ConvertInstId2Symbol(price.InstId)
		if err != nil {
			return nil, err
		}
		ts, err := strconv.ParseInt(price.Ts, 10, 64)
		if err != nil {
			return nil, err
		}
		result = append(result, &ctypes.Price{
			Exchange: c.Exchange(),
			Symbol:   symbol,
			Price:    number.DecimalFromString(price.Last),
			Ts:       time.UnixMilli(ts),
		})
	}
	return result, nil
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

func (c *Connector) BookPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.BookPrice, error) {
	ticker, err := c.getTicker(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if ticker == nil {
		return nil, nil
	}
	ts, err := strconv.ParseInt(ticker.Ts, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse ticker time: %w", err)
	}
	return &ctypes.BookPrice{
		Exchange: c.Exchange(),
		Symbol:   symbol,
		BidPrice: number.DecimalFromString(ticker.BidPx),
		BidQty:   number.DecimalFromString(ticker.BidSz),
		AskPrice: number.DecimalFromString(ticker.AskPx),
		AskQty:   number.DecimalFromString(ticker.AskSz),
		Ts:       time.UnixMilli(ts),
	}, nil
}

func (c *Connector) getBookPrices(ctx context.Context, marketType ctypes.MarketType) ([]*ctypes.BookPrice, error) {
	var instType string
	switch marketType {
	case ctypes.MarketTypeSpot:
		instType = "SPOT"
	case ctypes.MarketTypeFuture:
		instType = "SWAP"
	default:
		return nil, fmt.Errorf("unsupported market type: %s", marketType)
	}
	bookTickers, err := fetchWithType[[]*okx.MarketTicker](c, ctx, CacheKeyTickers, instType)
	if err != nil {
		return nil, err
	}
	bookPrices := make([]*ctypes.BookPrice, 0, len(bookTickers))
	for _, bookTicker := range bookTickers {
		symbol, err := ConvertInstId2Symbol(bookTicker.InstId)
		if err != nil {
			logger.Ctx(ctx).Err(err).Str("inst_id", bookTicker.InstId).Msg("failed to convert inst id to symbol")
			continue
		}
		ts, err := strconv.ParseInt(bookTicker.Ts, 10, 64)
		if err != nil {
			return nil, err
		}
		bookPrices = append(bookPrices, &ctypes.BookPrice{
			Exchange: c.Exchange(),
			Symbol:   symbol,
			BidPrice: number.DecimalFromString(bookTicker.BidPx),
			BidQty:   number.DecimalFromString(bookTicker.BidSz),
			AskPrice: number.DecimalFromString(bookTicker.AskPx),
			AskQty:   number.DecimalFromString(bookTicker.AskSz),
			Ts:       time.UnixMilli(ts),
		})
	}
	return bookPrices, nil
}

func (c *Connector) MarkPrices(ctx context.Context) ([]*ctypes.MarkPrice, error) {
	markPrices, err := fetchWithType[[]*okx.MarkPrice](c, ctx, CacheKeyMarkPrices)
	if err != nil {
		return nil, err
	}
	result := make([]*ctypes.MarkPrice, 0, len(markPrices))
	for _, p := range markPrices {
		symbol, err := ConvertInstId2Symbol(p.InstId)
		if err != nil {
			logger.Ctx(ctx).Err(err).Str("inst_id", p.InstId).Msg("failed to convert inst id to symbol")
			continue
		}
		ts, err := strconv.ParseInt(p.Ts, 10, 64)
		if err != nil {
			return nil, err
		}
		result = append(result, &ctypes.MarkPrice{
			Exchange:  c.Exchange(),
			Symbol:    symbol,
			MarkPrice: number.DecimalFromString(p.MarkPx),
			Ts:        time.UnixMilli(ts),
		})
	}
	return result, nil
}

func (c *Connector) MarkPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.MarkPrice, error) {
	instId := Symbol2InstId(symbol)
	markPrices, err := fetchWithType[[]*okx.MarkPrice](c, ctx, CacheKeyMarkPrice, instId)
	if err != nil {
		return nil, err
	}
	if len(markPrices) == 0 {
		return nil, nil
	}
	ts, err := strconv.ParseInt(markPrices[0].Ts, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse mark price time: %w", err)
	}
	return &ctypes.MarkPrice{
		Exchange:  c.Exchange(),
		Symbol:    symbol,
		MarkPrice: number.DecimalFromString(markPrices[0].MarkPx),
		Ts:        time.UnixMilli(ts),
	}, nil
}

func (c *Connector) IndexPrice(ctx context.Context, symbol ctypes.Symbol) (*ctypes.IndexPrice, error) {
	if symbol.Type != ctypes.MarketTypeFuture {
		return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
	instId := fmt.Sprintf("%s-%s", symbol.Base, symbol.Quote)
	indexPrices, err := fetchWithType[[]*okx.IndexTicker](c, ctx, CacheKeyIndexPrice, instId)
	if err != nil {
		return nil, err
	}
	if len(indexPrices) == 0 {
		return nil, nil
	}
	ts, err := strconv.ParseInt(indexPrices[0].Ts, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse index price time: %w", err)
	}
	return &ctypes.IndexPrice{
		Exchange:   c.Exchange(),
		Symbol:     symbol,
		IndexPrice: number.DecimalFromString(indexPrices[0].IdxPx),
		Ts:         time.UnixMilli(ts),
	}, nil
}

func (c *Connector) IndexComponent(ctx context.Context, symbol ctypes.Symbol) (*ctypes.IndexComponent, error) {
	if symbol.Type != ctypes.MarketTypeFuture {
		return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
	instId := fmt.Sprintf("%s-%s", symbol.Base, symbol.Quote)
	indexComponents, err := fetchWithType[*okx.IndexComponent](c, ctx, CacheKeyIndexComponents, instId)
	if err != nil {
		return nil, err
	}
	if indexComponents == nil {
		return nil, nil
	}
	ts, err := strconv.ParseInt(indexComponents.Ts, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse index components time: %w", err)
	}
	result := &ctypes.IndexComponent{
		Exchange: c.Exchange(),
		Symbol:   symbol,
		Price:    number.DecimalFromString(indexComponents.Last),
		Ts:       time.UnixMilli(ts),
	}
	for _, component := range indexComponents.Components {
		result.Components = append(result.Components, struct {
			Exchange string          `json:"exchange,omitempty"`
			Symbol   string          `json:"symbol,omitempty"`
			Price    decimal.Decimal `json:"price,omitempty"`
			Weight   decimal.Decimal `json:"weight,omitempty"`
		}{
			Exchange: component.Exch,
			Symbol:   component.Symbol,
			Price:    number.DecimalFromString(component.SymPx),
			Weight:   number.DecimalFromString(component.Wgt),
		})
	}
	return result, nil
}

// 查询杠杆分层标准，目前仅支持U本位合约
func (c *Connector) getLeverageBrackets(ctx context.Context, instFimily string) ([]*okx.PositionTier, error) {
	brackets, err := fetchWithType[[]*okx.PositionTier](c, ctx, CacheKeyLeverageBrackets, instFimily)
	if err != nil {
		return nil, err
	}
	return brackets, nil
}

func (c *Connector) GetLeverageBracket(ctx context.Context, symbol ctypes.Symbol, markPrice decimal.Decimal) (*ctypes.LeverageBracket, error) {
	if markPrice.IsZero() {
		return nil, fmt.Errorf("mark price is zero")
	}
	if symbol.Type != ctypes.MarketTypeFuture {
		return nil, fmt.Errorf("symbol type is not future")
	}

	var (
		leverageBrackets []*okx.PositionTier
		metadata         *okx.SymbolInfo
	)
	errgrp := errgroup.Group{}
	errgrp.Go(func() error {
		var err error
		instFamily := Symbol2InstFamily(symbol)
		leverageBrackets, err = c.getLeverageBrackets(ctx, instFamily)
		if err != nil {
			return err
		}
		if len(leverageBrackets) == 0 {
			return errors.New("no leverage brackets found")
		}
		return nil
	})
	errgrp.Go(func() error {
		var err error
		metadata, err = c.getMarketSymbol(ctx, symbol)
		if err != nil {
			return err
		}
		if metadata == nil {
			return errors.New("no symbol market info found")
		}
		return nil
	})
	if err := errgrp.Wait(); err != nil {
		return nil, err
	}

	result := &ctypes.LeverageBracket{
		Symbol:   symbol,
		Brackets: make([]ctypes.Bracket, 0),
	}

	ctVal := number.DecimalFromString(metadata.CtVal)
	for _, brkt := range leverageBrackets {
		tier, err := strconv.Atoi(brkt.Tier)
		if err != nil {
			return nil, err
		}
		maxLever, err := strconv.ParseFloat(brkt.MaxLever, 64)
		if err != nil {
			return nil, err
		}
		minNotional := decimal.RequireFromString(brkt.MinSz).Mul(ctVal).Mul(markPrice)
		maxNotional := decimal.RequireFromString(brkt.MaxSz).Mul(ctVal).Mul(markPrice)
		result.Brackets = append(result.Brackets, ctypes.Bracket{
			Bracket:     tier,
			MaxLeverage: float32(maxLever),
			MinNotional: minNotional,
			MaxNotional: maxNotional,
			Mmr:         number.DecimalFromString(brkt.Mmr),
			Cum:         decimal.Zero,
		})
	}
	return result, nil
}

func (c *Connector) FundingRate(ctx context.Context, symbol ctypes.Symbol) (*ctypes.FundingRate, error) {
	if symbol.Type != ctypes.MarketTypeFuture {
		return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
	_symbol := Symbol2InstId(symbol)
	fundingRates, err := fetchWithType[[]*okx.FundingRate](c, ctx, CacheKeyFundingRate, _symbol)
	if err != nil {
		return nil, err
	}
	if len(fundingRates) == 0 {
		return nil, nil
	}
	ts, err := strconv.ParseInt(fundingRates[0].Ts, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse funding rate time: %w", err)
	}
	nextFundingTime, err := strconv.ParseInt(fundingRates[0].FundingTime, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse next funding time: %w", err)
	}
	return &ctypes.FundingRate{
		Exchange:        c.Exchange(),
		Symbol:          symbol,
		FundingRate:     number.DecimalFromString(fundingRates[0].FundingRate),
		InterestRate:    number.DecimalFromString(fundingRates[0].InterestRate),
		NextFundingTime: time.UnixMilli(nextFundingTime),
		Ts:              time.UnixMilli(ts),
	}, nil
}

func (c *Connector) HisFundingRates(ctx context.Context, symbol ctypes.Symbol, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.FundingRate, error) {
	if symbol.Type != ctypes.MarketTypeFuture {
		return nil, fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
	_symbol := Symbol2InstId(symbol)
	svc := c.restClient.NewFundingRateHistoryService().InstId(_symbol)
	if startTs != nil {
		svc = svc.Before(*startTs)
	}
	if endTs != nil {
		svc = svc.After(*endTs)
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
		fundingTime, err := strconv.ParseInt(fundingRate.FundingTime, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse funding time: %w", err)
		}
		result = append(result, &ctypes.FundingRate{
			Exchange:    c.Exchange(),
			Symbol:      symbol,
			FundingRate: number.DecimalFromString(fundingRate.RealizedRate),
			Ts:          time.UnixMilli(fundingTime),
		})
	}
	return result, nil
}

func (c *Connector) OpenInterest(ctx context.Context, symbol ctypes.Symbol) (decimal.Decimal, error) {
	if symbol.Type != ctypes.MarketTypeFuture {
		return decimal.Zero, fmt.Errorf("unsupported market type: %s", symbol.Type)
	}
	instId := Symbol2InstId(symbol)
	instType := FormatMarketType(symbol.Type)

	var openInterest okx.OpenInterest
	var ctVal decimal.Decimal

	errgrp := errgroup.Group{}
	errgrp.Go(func() error {
		openInterests, err := fetchWithType[[]*okx.OpenInterest](c, ctx, CacheKeyOpenInterest, instType, instId)
		if err != nil {
			return err
		}
		if len(openInterests) == 0 {
			return fmt.Errorf("no open interest found for symbol: %s", symbol)
		}
		openInterest = *openInterests[0]
		return nil
	})
	errgrp.Go(func() error {
		metadata, err := c.getMarketSymbol(ctx, symbol)
		if err != nil {
			return err
		}
		if metadata == nil {
			return errors.New("no symbol market info found")
		}
		ctVal = number.DecimalFromString(metadata.CtVal)
		return nil
	})
	if err := errgrp.Wait(); err != nil {
		return decimal.Zero, err
	}
	return decimal.RequireFromString(openInterest.OpenInterest).Mul(ctVal), nil
}

func (c *Connector) Trades(ctx context.Context, symbol ctypes.Symbol, limit int) ([]*ctypes.Trade, error) {
	instId := Symbol2InstId(symbol)

	results := make([]*ctypes.Trade, 0, limit)

	trades, err := c.restClient.NewMarketTradesService().InstId(instId).Limit(limit).Do(ctx)
	if err != nil {
		return nil, err
	}
	for _, trade := range trades {
		price, err := decimal.NewFromString(trade.Px)
		if err != nil {
			return nil, fmt.Errorf("parse trade price: %w", err)
		}
		size, err := decimal.NewFromString(trade.Sz)
		if err != nil {
			return nil, fmt.Errorf("parse trade size: %w", err)
		}
		ts, err := strconv.ParseInt(trade.Ts, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse trade time: %w", err)
		}
		results = append(results, &ctypes.Trade{
			Symbol:  symbol,
			TradeID: trade.TradeId,
			Price:   price,
			Size:    size,
			IsBuy:   trade.Side == "buy",
			Ts:      time.UnixMilli(ts),
		})
	}
	return results, nil
}

func (c *Connector) Depth(ctx context.Context, symbol ctypes.Symbol, limit int) (*ctypes.OrderBook, error) {
	instId := Symbol2InstId(symbol)

	var err error
	var depth *okx.Depth
	if limit <= 400 {
		depth, err = c.restClient.NewMarketDepthService().InstId(instId).Size(limit).Do(ctx)
	} else {
		depth, err = c.restClient.NewMarketDepthFullService().InstId(instId).Size(limit).Do(ctx)
	}

	if err != nil {
		return nil, err
	}

	if depth == nil {
		return nil, nil
	}

	book, err := ConvertDepth2Types(c.Exchange(), symbol, depth)
	if err != nil {
		return nil, err
	}

	return book, nil
}

func (c *Connector) Klines(ctx context.Context, symbol ctypes.Symbol, interval ctypes.Interval, limit int) ([]*ctypes.Kline, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than 0")
	}
	if limit > 300 {
		return nil, fmt.Errorf("limit must be less than 300")
	}

	instId := Symbol2InstId(symbol)
	itv, err := ParseInterval(interval)
	if err != nil {
		return nil, err
	}

	klines, err := c.restClient.NewMarketKlinesService().InstId(instId).Bar(itv.String()).Limit(limit).Do(ctx)
	if err != nil {
		return nil, err
	}
	if len(klines) == 0 {
		return nil, nil
	}
	result := make([]*ctypes.Kline, 0, len(klines))
	for _, k := range klines {
		openTs, err := strconv.ParseInt(k.Ts, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse kline time: %w", err)
		}
		duration, err := interval.Duration()
		if err != nil {
			return nil, err
		}
		closeTs := openTs + int64(duration.Milliseconds())
		result = append(result, &ctypes.Kline{
			Symbol:      symbol,
			Interval:    interval,
			Open:        number.DecimalFromString(k.Open),
			Close:       number.DecimalFromString(k.Close),
			High:        number.DecimalFromString(k.High),
			Low:         number.DecimalFromString(k.Low),
			Volume:      number.DecimalFromString(k.VolCcy),
			QuoteVolume: number.DecimalFromString(k.VolCcyQuote),
			OpenTs:      time.UnixMilli(openTs),
			CloseTs:     time.UnixMilli(closeTs),
		})
	}
	return result, nil
}

func (c *Connector) HisKlines(ctx context.Context, symbol ctypes.Symbol, interval ctypes.Interval, startTs *time.Time, endTs *time.Time, limit *int) ([]*ctypes.Kline, error) {
	if limit == nil {
		return nil, fmt.Errorf("limit is required")
	}
	if *limit > 300 {
		return nil, fmt.Errorf("limit must be less than 300")
	}

	instId := Symbol2InstId(symbol)
	itv, err := ParseInterval(interval)
	if err != nil {
		return nil, err
	}

	svc := c.restClient.NewMarketKlinesHisService().InstId(instId).Bar(itv.String()).Limit(*limit)
	if startTs != nil {
		svc = svc.Before(startTs.UnixMilli())
	}
	if endTs != nil {
		svc = svc.After(endTs.UnixMilli())
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
		openTs, err := strconv.ParseInt(k.Ts, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse kline time: %w", err)
		}
		duration, err := interval.Duration()
		if err != nil {
			return nil, err
		}
		closeTs := openTs + int64(duration.Milliseconds())
		result = append(result, &ctypes.Kline{
			Symbol:      symbol,
			Interval:    interval,
			Open:        number.DecimalFromString(k.Open),
			Close:       number.DecimalFromString(k.Close),
			High:        number.DecimalFromString(k.High),
			Low:         number.DecimalFromString(k.Low),
			Volume:      number.DecimalFromString(k.VolCcy),
			QuoteVolume: number.DecimalFromString(k.VolCcyQuote),
			OpenTs:      time.UnixMilli(openTs),
			CloseTs:     time.UnixMilli(closeTs),
		})
	}
	return result, nil
}

func (c *Connector) Account(ctx context.Context) (*ctypes.AccountBo, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	account, err := c.getAccount(ctx)
	if err != nil {
		return nil, err
	}
	return account.ToTypesAccount(c.exchange), nil
}

func (c *Connector) getAccount(ctx context.Context) (*Account, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	acctConfig, err := fetchWithType[*okx.AccountConfig](c, ctx, CacheKeyApiAccountConfig)
	if err != nil {
		return nil, err
	}

	return &Account{
		Uid:              acctConfig.Uid,
		MainUid:          acctConfig.MainUid,
		Level:            acctConfig.AcctLv,
		UserLevel:        acctConfig.Level,
		Type:             acctConfig.Type,
		PositionMode:     acctConfig.PosMode,
		RoleType:         acctConfig.RoleType,
		TraderInsts:      acctConfig.TraderInsts,
		SpotRoleType:     acctConfig.SpotRoleType,
		SpotTraderInsts:  acctConfig.SpotTraderInsts,
		ApiKeyPermission: NewAPIKeyPermission(acctConfig.Perm),
	}, nil
}

func (c *Connector) Balance(ctx context.Context) (*ctypes.Balance, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	balance, err := c.getBalance(ctx)
	if err != nil {
		return nil, err
	}
	return balance.ToTypesBalance(), nil
}

func (c *Connector) getBalance(ctx context.Context) (*Balance, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	result := &Balance{}

	errgrp := errgroup.Group{}

	errgrp.Go(func() error {
		fundingBalances, err := fetchWithType[[]*okx.FundingAssetBalance](c, ctx, CacheKeyFundingBalances)
		if err != nil {
			return err
		}
		result.FundingBalances = fundingBalances
		return nil
	})

	errgrp.Go(func() error {
		fundingValues, err := fetchWithType[*okx.FundingAssetValuation](c, ctx, CacheKeyFundingValues)
		if err != nil {
			return err
		}
		result.FundingValues = fundingValues
		return nil
	})

	errgrp.Go(func() error {
		account, err := fetchWithType[*okx.AccountBalance](c, ctx, CacheKeyTradingBalances)
		if err != nil {
			return err
		}
		result.TradingBalances = account
		return nil
	})

	if err := errgrp.Wait(); err != nil {
		return nil, err
	}
	result.UpdatedTs = time.Now()
	return result, nil
}

func (c *Connector) Positions(ctx context.Context, mt *ctypes.MarketType) ([]*ctypes.Position, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	if mt != nil && *mt == ctypes.MarketTypeSpot {
		return nil, nil
	}

	results := make([]*ctypes.Position, 0)

	service := c.restClient.NewPositionsService()
	if mt != nil {
		instType := FormatMarketType(*mt)
		service = service.InstType(instType)
	}
	positions, err := service.Do(ctx)
	if err != nil {
		return nil, err
	}
	for _, position := range positions {
		symbol, err := ConvertInstId2Symbol(position.InstId)
		if err != nil {
			return nil, err
		}

		metadata, err := c.getMarketSymbol(ctx, symbol)
		if err != nil {
			return nil, err
		}

		ctVal := number.DecimalFromString(metadata.CtVal)

		leverage, err := strconv.Atoi(position.Lever)
		if err != nil {
			return nil, err
		}
		avgPrice := number.DecimalFromString(position.AvgPx)
		pos := number.DecimalFromString(position.Pos).Mul(ctVal)
		notional := avgPrice.Mul(pos)
		side := ctypes.PositionSideLong
		switch position.PosSide {
		case "long":
			side = ctypes.PositionSideLong
		case "short":
			side = ctypes.PositionSideShort
		case "net":
			if pos.LessThan(decimal.Zero) {
				side = ctypes.PositionSideShort
			} else {
				side = ctypes.PositionSideLong
			}
		default:
			return nil, fmt.Errorf("invalid position side: %s", position.PosSide)
		}
		uTime, err := strconv.ParseInt(position.UTime, 10, 64)
		if err != nil {
			return nil, err
		}
		results = append(results, &ctypes.Position{
			Symbol:           symbol,
			Side:             side,
			Isolated:         position.MgnMode == "isolated",
			Amount:           pos.Abs(),
			EntryPrice:       avgPrice,
			MarkPrice:        number.DecimalFromString(position.MarkPx),
			LiquidationPrice: number.DecimalFromString(position.LiqPx),
			Notional:         notional,
			Leverage:         leverage,
			InitialMargin:    number.DecimalFromString(position.Imr),
			MaintMargin:      number.DecimalFromString(position.Mmr),
			UnRealizedProfit: number.DecimalFromString(position.Upl),
			UpdatedTs:        time.UnixMilli(uTime),
		})
	}

	return results, nil
}

func (c *Connector) SymbolConfig(ctx context.Context, symbol ctypes.Symbol) (*ctypes.SymbolConfig, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	// check symbol exist
	market, err := c.getMarketSymbol(ctx, symbol)
	if err != nil {
		return nil, err
	}
	mkt, err := ConvertMarket2Types(c.Exchange(), market)
	if err != nil {
		return nil, err
	}
	result := &ctypes.SymbolConfig{
		Exchange:              c.Exchange(),
		Symbol:                symbol,
		Market:                *mkt,
		IsolatedMarginEnabled: false,
		CrossMarginEnabled:    true,
		MaxNotionalValue:      number.DecimalFromString(market.PosLmtAmt),
	}

	errgrp := errgroup.Group{}
	if symbol.Type == ctypes.MarketTypeFuture {
		errgrp.Go(func() error {
			leverage, err := c.getLeverageInfo(ctx, symbol)
			if err != nil {
				return err
			}
			result.CrossLeverage = leverage
			return nil
		})
	}
	errgrp.Go(func() error {
		makerCommission, takerCommission, err := c.getSymbolCommission(ctx, symbol)
		if err != nil {
			return err
		}
		result.MakerCommission = makerCommission
		result.TakerCommission = takerCommission
		return nil
	})

	if err := errgrp.Wait(); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Connector) SetLeverage(ctx context.Context, symbol ctypes.Symbol, leverage int) (int, error) {
	if !c.IsPrivate() {
		return 0, fmt.Errorf("connector is not private mode")
	}
	instId := Symbol2InstId(symbol)
	response, err := c.restClient.NewSetLeverageService().InstId(instId).MgnMode("cross").Lever(strconv.Itoa(leverage)).Do(ctx)
	if err != nil {
		return 0, err
	}
	if len(response) == 0 {
		return 0, fmt.Errorf("no leverage info found")
	}
	return strconv.Atoi(response[0].Lever)
}

func (c *Connector) getLeverageInfo(ctx context.Context, symbol ctypes.Symbol) ([2]int, error) {
	instId := Symbol2InstId(symbol)

	response, err := fetchWithType[[]*okx.AccountLeverageInfo](c, ctx, CacheKeyAccountLeverageInfo, instId)
	if err != nil {
		return [2]int{}, err
	}
	if len(response) == 0 {
		return [2]int{}, nil
	}
	result := [2]int{}
	for _, item := range response {
		leverage, err := strconv.Atoi(item.Lever)
		if err != nil {
			return [2]int{}, fmt.Errorf("parse leverage: %w", err)
		}
		switch item.PosSide {
		case "long":
			result[0] = leverage
		case "short":
			result[1] = leverage
		case "net":
			result[0] = leverage
			result[1] = leverage
		}
	}
	return result, nil
}

func (c *Connector) getSymbolCommission(ctx context.Context, symbol ctypes.Symbol) (decimal.Decimal, decimal.Decimal, error) {
	var (
		makerCommission = decimal.Zero
		takerCommission = decimal.Zero
	)

	market, err := c.getMarketSymbol(ctx, symbol)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	if market == nil {
		return decimal.Zero, decimal.Zero, fmt.Errorf("market not found for symbol: %s", symbol.String())
	}

	tradeFee, err := fetchWithType[[]*okx.AccountTradeFee](c, ctx, CacheKeyAccountTradeFee, symbol)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}

	switch symbol.Type {
	case ctypes.MarketTypeSpot:
		makerCommission = decimal.NewFromFloat(-0.0008)
		takerCommission = decimal.NewFromFloat(-0.001)
	case ctypes.MarketTypeFuture:
		if symbol.Quote == "USDC" {
			makerCommission = decimal.NewFromFloat(-0.0002)
			takerCommission = decimal.NewFromFloat(-0.0005)
		}
	}

	if len(tradeFee) == 0 {
		return makerCommission, takerCommission, nil
	}

	for _, item := range tradeFee {
		for _, group := range item.FeeGroup {
			if group.GroupId != market.GroupId {
				continue
			}
			takerCommission = number.DecimalFromString(group.Taker)
			makerCommission = number.DecimalFromString(group.Maker)
		}
	}
	return makerCommission, takerCommission, nil
}

// CalcOrderFee 根据订单信息计算手续费（maker/taker 费率）
func (c *Connector) CalcOrderFee(ctx context.Context, order ctypes.Order) (*decimal.Decimal, *string, error) {
	if !order.Symbol.IsValid() {
		return nil, nil, fmt.Errorf("invalid order symbol")
	}
	if order.ExecutedQty.LessThanOrEqual(decimal.Zero) && order.ExecutedQuoteQty.LessThanOrEqual(decimal.Zero) {
		return nil, nil, nil
	}

	makerCommission, takerCommission, err := c.getSymbolCommission(ctx, order.Symbol)
	if err != nil {
		return nil, nil, err
	}

	feeRate := takerCommission
	if order.PostOnly {
		feeRate = makerCommission
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
			fee := order.ExecutedQty.Mul(feeRate)
			return lo.ToPtr(fee), lo.ToPtr(order.Symbol.Base), nil
		}
		fee := executedQuoteQty.Mul(feeRate)
		return lo.ToPtr(fee), lo.ToPtr(order.Symbol.Quote), nil
	case ctypes.MarketTypeFuture:
		fee := executedQuoteQty.Mul(feeRate)
		return lo.ToPtr(fee), lo.ToPtr(order.Symbol.Quote), nil
	}
	return nil, nil, fmt.Errorf("unsupported market type: %s", order.Symbol.Type)
}

func (c *Connector) GetOrders(ctx context.Context, symbol *ctypes.Symbol) ([]*ctypes.Order, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	var instId, instType *string
	if symbol != nil {
		instId = lo.ToPtr(Symbol2InstId(*symbol))
		instType = lo.ToPtr(FormatMarketType(symbol.Type))
	}

	mu := sync.Mutex{}
	result := make([]*ctypes.Order, 0)
	erp := errgroup.Group{}
	algoOrderTypes := []string{"conditional,oco", "chase", "trigger", "move_order_stop", "twap"}
	for _, tp := range algoOrderTypes {
		erp.Go(func() error {
			algoOrders, err := c.getAlgoOrders(ctx, instId, instType, tp)
			if err != nil {
				return err
			}
			for _, order := range algoOrders {
				order, err := c.ConvertOrder2Types(ctx, order)
				if err != nil {
					return err
				}
				mu.Lock()
				result = append(result, order)
				mu.Unlock()
			}
			return nil
		})
	}

	erp.Go(func() error {
		service := c.restClient.NewOpenOrdersService().Limit(100)
		if instType != nil {
			service = service.InstType(*instType)
		}
		if instId != nil {
			service = service.InstId(*instId)
		}
		orders, err := service.Do(ctx)
		if err != nil {
			return err
		}
		for _, order := range orders {
			order, err := c.ConvertOrder2Types(ctx, order)
			if err != nil {
				return err
			}
			mu.Lock()
			result = append(result, order)
			mu.Unlock()
		}
		return nil
	})

	if err := erp.Wait(); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Connector) getAlgoOrders(ctx context.Context, instId, instType *string, tp string) ([]*okx.Order, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	switch tp {
	case "conditional,oco":
	case "conditional":
	case "oco":
	case "chase":
		return nil, nil
	case "trigger":
	case "move_order_stop":
	case "twap":
	default:
		return nil, fmt.Errorf("invalid algo order type: %s", tp)
	}

	service := c.restClient.NewOpenAlgoOrdersService().OrdType(tp).Limit(100)
	if instType != nil {
		service = service.InstType(*instType)
	}
	if instId != nil {
		service = service.InstId(*instId)
	}
	return service.Do(ctx)
}

func (c *Connector) GetHisOrders(ctx context.Context, symbol ctypes.Symbol, startTs time.Time, endTs time.Time, limit int) ([]*ctypes.Order, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	instType := FormatMarketType(symbol.Type)
	instId := Symbol2InstId(symbol)

	beginTsStr := strconv.FormatInt(startTs.UnixMilli(), 10)
	endTsStr := strconv.FormatInt(endTs.UnixMilli(), 10)

	if limit > 100 {
		return nil, fmt.Errorf("limit must be less than or equal to 100")
	}

	result := make([]*ctypes.Order, 0)

	mu := sync.Mutex{}
	errgrp := errgroup.Group{}
	errgrp.Go(func() error {
		orders, err := c.restClient.NewOrders7DHistoryService().InstType(instType).InstId(instId).Begin(beginTsStr).End(endTsStr).Limit(limit).Do(ctx)
		if err != nil {
			return err
		}
		ords := make([]*ctypes.Order, 0)
		for _, order := range orders {
			order, err := c.ConvertOrder2Types(ctx, order)
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
	errgrp.Go(func() error {
		// 算法订单，暂不支持开始时间结束时间
		orders, err := c.restClient.NewOrdersAlgoHistoryService().InstType(instType).InstId(instId).Limit(limit).Do(ctx)
		if err != nil {
			return err
		}
		ords := make([]*ctypes.Order, 0)
		for _, order := range orders {
			order, err := c.ConvertOrder2Types(ctx, order)
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
	if err := errgrp.Wait(); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Connector) GetOrder(ctx context.Context, symbol ctypes.Symbol, orderId string) (*ctypes.Order, error) {
	if !c.IsPrivate() {
		return nil, fmt.Errorf("connector is not private mode")
	}

	instId := Symbol2InstId(symbol)

	var result *ctypes.Order
	var err error
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		var orders []*okx.Order
		orders, err = c.restClient.NewOrderService().InstId(instId).OrdId(orderId).Do(ctx)
		if err != nil {
			return
		}
		if len(orders) > 0 {
			result, err = c.ConvertOrder2Types(ctx, orders[0])
		}
	}()
	go func() {
		defer wg.Done()
		var orders []*okx.Order
		orders, err = c.restClient.NewAlgoOrderService().AlgoId(orderId).Do(ctx)
		if err != nil {
			return
		}
		if len(orders) > 0 {
			result, err = c.ConvertOrder2Types(ctx, orders[0])
		}
	}()
	wg.Wait()
	if result != nil {
		return result, nil
	}
	return nil, err
}
