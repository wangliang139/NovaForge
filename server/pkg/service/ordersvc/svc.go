package ordersvc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stumble/wpgx"
	converter "github.com/wangliang139/NovaForge/server/pkg/converter"
	"github.com/wangliang139/NovaForge/server/pkg/entity"
	account "github.com/wangliang139/NovaForge/server/pkg/entity/account"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	ordersrepo "github.com/wangliang139/NovaForge/server/pkg/repos/orders"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/utils"
	"github.com/wangliang139/mow/errors"
	"github.com/wangliang139/mow/logger"
	"github.com/wangliang139/mow/snowflake"
)

type Service struct {
	mu sync.Mutex

	db    *repos.Entity
	cache redis.UniversalClient
}

func New(db *repos.Entity, cache redis.UniversalClient) (*Service, error) {
	return &Service{
		db:    db,
		cache: cache,
	}, nil
}

func (s *Service) GetOpenOrders(ctx context.Context, req *types.GetOpenOrdersRequest) (*types.GetOpenOrdersResponse, error) {
	if len(req.AccountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}

	var smbPtr *types.Symbol
	if req.Symbol != "" {
		smb, err := types.ParseSymbol(req.Symbol)
		if err != nil {
			return nil, errors.New(errors.InvalidArgument, err.Error())
		}
		if !smb.IsValid() {
			return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %s", req.Symbol))
		}
		smbPtr = &smb
	}

	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	orders, err := entity.Account.GetOpenOrders(ctx, acct.ID, smbPtr)
	if err != nil {
		return nil, err
	}

	return &types.GetOpenOrdersResponse{Orders: orders}, nil
}

func (s *Service) QueryOrders(ctx context.Context, req *types.QueryOrdersRequest) (*types.QueryOrdersResponse, error) {
	if len(req.AccountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}

	var smbPtr *types.Symbol
	if req.Symbol != "" {
		smb, err := types.ParseSymbol(req.Symbol)
		if err != nil {
			return nil, errors.New(errors.InvalidArgument, err.Error())
		}
		if !smb.IsValid() {
			return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %s", req.Symbol))
		}
		smbPtr = &smb
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	fetchLimit := limit + 1

	cursorCreatedTs, cursorID, err := parseOrderCursor(req.Cursor)
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, err.Error())
	}

	var orderType *types.OrderType
	if req.OrderType != nil && req.OrderType.Valid() {
		orderType = req.OrderType
	}
	var orderSource *types.OrderSource
	if req.OrderSource != nil && req.OrderSource.Valid() {
		orderSource = req.OrderSource
	}
	statuses := make([]types.OrderStatus, 0, len(req.Statuses))
	for _, st := range req.Statuses {
		if st.Valid() {
			statuses = append(statuses, st)
		}
	}

	resp := &types.QueryOrdersResponse{}

	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	filter := account.OrderCursorFilter{
		Symbol:          smbPtr,
		OrderType:       orderType,
		OrderSource:     orderSource,
		Statuses:        statuses,
		CursorCreatedTs: cursorCreatedTs,
		CursorID:        cursorID,
		Limit:           fetchLimit,
	}
	filter.BotID = req.BotID
	orders, err := entity.Account.QueryOrdersByCursor(ctx, acct.ID, filter)
	if err != nil {
		return nil, err
	}

	orders, hasMore, next := buildOrdersPage(orders, limit)
	resp.HasMore = hasMore
	if next != "" {
		resp.Next = next
	}

	resp.Orders = orders

	return resp, nil
}

func (s *Service) QueryOrdersByPage(ctx context.Context, req *types.QueryOrdersByPageRequest) (*types.QueryOrdersByPageResponse, error) {
	if len(req.AccountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}

	var smbPtr *types.Symbol
	if req.Symbol != "" {
		smb, err := types.ParseSymbol(req.Symbol)
		if err != nil {
			return nil, errors.New(errors.InvalidArgument, err.Error())
		}
		if !smb.IsValid() {
			return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %s", req.Symbol))
		}
		smbPtr = &smb
	}

	page := req.Page
	if page <= 0 {
		page = 1
	}
	size := req.Size
	if size <= 0 {
		size = 100
	}
	if size > 200 {
		size = 200
	}

	var orderType *types.OrderType
	if req.OrderType != nil && req.OrderType.Valid() {
		orderType = req.OrderType
	}
	var orderSource *types.OrderSource
	if req.OrderSource != nil && req.OrderSource.Valid() {
		orderSource = req.OrderSource
	}
	statuses := make([]types.OrderStatus, 0, len(req.Statuses))
	for _, st := range req.Statuses {
		if st.Valid() {
			statuses = append(statuses, st)
		}
	}

	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	pageFilter := account.OrderPageFilter{
		Symbol:      smbPtr,
		OrderType:   orderType,
		OrderSource: orderSource,
		Statuses:    statuses,
		Page:        page,
		Size:        size,
	}
	pageFilter.BotID = req.BotID
	if req.FinishedStartTsMs != nil && req.FinishedEndTsMs != nil &&
		*req.FinishedStartTsMs > 0 && *req.FinishedEndTsMs >= *req.FinishedStartTsMs {
		fs := time.UnixMilli(*req.FinishedStartTsMs)
		fe := time.UnixMilli(*req.FinishedEndTsMs)
		pageFilter.FinishedStartTs = &fs
		pageFilter.FinishedEndTs = &fe
	} else {
		if req.StartTs != nil && *req.StartTs > 0 {
			t := time.Unix(*req.StartTs, 0)
			pageFilter.StartTs = &t
		}
		if req.EndTs != nil && *req.EndTs > 0 {
			t := time.Unix(*req.EndTs, 0)
			pageFilter.EndTs = &t
		}
	}
	orders, totalCount, err := entity.Account.QueryOrdersByPage(ctx, acct.ID, pageFilter)
	if err != nil {
		return nil, err
	}

	return &types.QueryOrdersByPageResponse{Orders: orders, TotalCount: totalCount}, nil
}

func (s *Service) GetOrder(ctx context.Context, req *types.GetOrderRequest) (*types.GetOrderResponse, error) {
	if len(req.AccountID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	clientOrderID := req.ClientOrderID
	exchangeOrderID := req.ExchangeOrderID
	if len(clientOrderID) == 0 && len(exchangeOrderID) == 0 {
		return nil, errors.New(errors.InvalidArgument, "client_order_id or exchange_order_id is required")
	}
	smb, err := types.ParseSymbol(req.Symbol)
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, err.Error())
	}
	if !smb.IsValid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %s", req.Symbol))
	}

	o, err := entity.Account.GetOrder(ctx, req.AccountID, req.Symbol, clientOrderID, exchangeOrderID)
	if err != nil {
		return nil, err
	}

	return &types.GetOrderResponse{Order: o}, nil
}

func (s *Service) PlaceOrder(ctx context.Context, req *types.PlaceOrderRequest) (*types.PlaceOrderResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	if strings.TrimSpace(req.AccountID) == "" {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	if strings.TrimSpace(req.Symbol) == "" {
		return nil, errors.New(errors.InvalidArgument, "symbol is required")
	}
	symbol, err := types.ParseSymbol(req.Symbol)
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, err.Error())
	}
	if !symbol.IsValid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %s", req.Symbol))
	}

	orderSource := types.OrderSourceStrategy
	if req.OrderSource != nil && req.OrderSource.Valid() {
		orderSource = *req.OrderSource
	}

	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	if req.Exchange != "" {
		ex := req.Exchange
		if !ex.IsValid() {
			return nil, errors.New(errors.InvalidArgument, "invalid exchange")
		}
		if acct.Exchange != ex {
			return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("account exchange mismatch: account=%s request=%s", acct.Exchange, ex))
		}
	}

	orderType := req.OrderType
	if !orderType.Valid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid order type: %s", req.OrderType))
	}

	side := req.Side
	if !side.Valid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid side: %s", req.Side))
	}

	var (
		rawPrice    decimal.Decimal
		rawQty      decimal.Decimal
		rawQuoteQty decimal.Decimal
	)
	if req.Price != nil && strings.TrimSpace(*req.Price) != "" {
		p, err := decimal.NewFromString(strings.TrimSpace(*req.Price))
		if err != nil {
			return nil, errors.New(errors.InvalidArgument, "price is invalid")
		}
		rawPrice = p
	}
	if req.Quantity != nil && strings.TrimSpace(*req.Quantity) != "" {
		q, err := decimal.NewFromString(strings.TrimSpace(*req.Quantity))
		if err != nil {
			return nil, errors.New(errors.InvalidArgument, "quantity is invalid")
		}
		rawQty = q
	}
	if req.QuoteQty != nil && strings.TrimSpace(*req.QuoteQty) != "" {
		q, err := decimal.NewFromString(strings.TrimSpace(*req.QuoteQty))
		if err != nil {
			return nil, errors.New(errors.InvalidArgument, "quote_qty is invalid")
		}
		rawQuoteQty = q
	}

	var tif *types.TimeInForce
	if req.TimeInForce != nil && strings.TrimSpace(*req.TimeInForce) != "" {
		v := types.TimeInForce(strings.ToUpper(strings.TrimSpace(*req.TimeInForce)))
		if !v.Valid() {
			return nil, errors.New(errors.InvalidArgument, "time_in_force is invalid")
		}
		tif = &v
	}

	// 6) 生成/透传 ClientOrderID
	clientOrderID := func() types.OrderId {
		if req.ClientOrderID != nil {
			v := strings.TrimSpace(*req.ClientOrderID)
			if v != "" {
				return types.OrderId(v)
			}
		}
		return types.OrderId(snowflake.Generate().String())
	}()

	// 6) 资金/保证金校验 + 冻结估算（通过订单快照 locked 写入，驱动 frozen delta）
	order := &types.Order{
		AccountID:        acct.ID,
		BotID:            lo.FromPtr(req.BotID),
		ClientOrderID:    clientOrderID,
		Source:           orderSource,
		Exchange:         acct.Exchange,
		Symbol:           symbol,
		Side:             side,
		IsBuy:            req.IsBuy,
		OrderType:        orderType,
		Price:            rawPrice,
		OriginalQty:      rawQty,
		OriginalQuoteQty: rawQuoteQty,
		TimeInForce:      lo.FromPtr(tif),
		ReduceOnly:       lo.FromPtr(req.ReduceOnly),
		ClosePosition:    lo.FromPtr(req.ClosePosition),
	}

	result, err := entity.Order.PlaceOrder(ctx, acct, order)
	if err != nil {
		return &types.PlaceOrderResponse{Error: lo.ToPtr(err.Error())}, nil
	}

	return &types.PlaceOrderResponse{
		OrderId:       result.OrderId.String(),
		ClientOrderId: result.ClientOrderId.String(),
		Status:        result.Status,
	}, nil
}

func (s *Service) CancelOrder(ctx context.Context, req *types.CancelOrderRequest) (*types.CancelOrderResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	if strings.TrimSpace(req.AccountID) == "" {
		return nil, errors.New(errors.InvalidArgument, "account_id is required")
	}
	if strings.TrimSpace(req.Symbol) == "" {
		return nil, errors.New(errors.InvalidArgument, "symbol is required")
	}
	clientOrderID := strings.TrimSpace(req.ClientOrderID)
	orderID := strings.TrimSpace(req.OrderID)
	if clientOrderID == "" && orderID == "" {
		return nil, errors.New(errors.InvalidArgument, "client_order_id or order_id is required")
	}

	symbol, err := types.ParseSymbol(req.Symbol)
	if err != nil {
		return nil, errors.New(errors.InvalidArgument, err.Error())
	}
	if !symbol.IsValid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %s", req.Symbol))
	}

	acct, err := s.getAccount(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}

	conn, err := entity.Account.GetConnector(ctx, acct.Exchange, acct.ID)
	if err != nil {
		return &types.CancelOrderResponse{Success: false, Error: lo.ToPtr(err.Error())}, nil
	}

	// 撤单 + 解冻 + 更新订单状态尽量在一个事务中完成
	now := time.Now()

	_, err = s.db.ConnPool.Transact(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *wpgx.WTx) (any, error) {
		// 1) 锁定订单行（优先使用 client_order_id，失败则使用 order_id）
		var lockedOrder *ordersrepo.Order
		if clientOrderID != "" {
			lockedOrder, err = s.db.OrdersRepo.WithTx(tx).GetOrderByClientOrderIdWithLock(ctx, ordersrepo.GetOrderByClientOrderIdWithLockParams{
				AccountID:     acct.ID,
				ClientOrderID: clientOrderID,
			})
		}
		if (err != nil || lockedOrder == nil) && orderID != "" {
			lockedOrder, err = s.db.OrdersRepo.WithTx(tx).GetOrderByOrderIdWithLock(ctx, ordersrepo.GetOrderByOrderIdWithLockParams{
				AccountID: acct.ID,
				OrderID:   orderID,
			})
		}
		if err != nil {
			return nil, err
		}
		if lockedOrder == nil {
			return nil, fmt.Errorf("order not found")
		}

		// 2) 调用交易所撤单（使用 exchange order_id）
		if err := conn.CancelOrder(ctx, symbol, lockedOrder.OrderID); err != nil {
			return nil, err
		}

		// 3) 解冻资金
		lockedAmt := utils.Decimal.PgNumericToDecimal(lockedOrder.Locked)
		lockedAsset := ""
		if lockedOrder.LockedAsset != nil {
			lockedAsset = strings.ToUpper(strings.TrimSpace(*lockedOrder.LockedAsset))
		}
		if lockedAsset != "" && lockedAmt.GreaterThan(decimal.Zero) {
			reason := types.LedgerReasonFundsUnfreeze
			if symbol.Type == types.MarketTypeFuture {
				reason = types.LedgerReasonOrderMarginUnfreeze
			}
			order, _ := converter.OrderDb2Types(*lockedOrder)
			err := entity.Account.CheckAndApplyAssetOrderOccupiedUpdate(ctx, acct.ID, acct.Exchange, &types.AssetEvent{
				WalletType: types.GetWalletType(acct.Exchange, symbol.Type),
				Code:       lockedAsset,
				Locked:     lo.ToPtr(lockedAmt.Neg()),
				UpdatedTs:  now,
			}, reason, order)
			if err != nil {
				return nil, err
			}
		}

		// 4) 更新订单状态（CANCELED + locked=0）
		reasonStr := "canceled"
		finished := now
		_, err = s.db.OrdersRepo.WithTx(tx).CancelOrderStatusWithReason(ctx, ordersrepo.CancelOrderStatusWithReasonParams{
			AccountID:    acct.ID,
			OrderID:      lockedOrder.OrderID,
			RejectReason: &reasonStr,
			FinishedTs:   &finished,
			UpdatedTs:    now,
		})
		if err != nil {
			logger.Ctx(ctx).Err(err).Str("account_id", acct.ID).
				Str("exchange", acct.Exchange.String()).
				Str("symbol", symbol.String()).
				Str("client_order_id", clientOrderID).
				Msg("failed to cancel order status")
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	return &types.CancelOrderResponse{Success: true}, nil
}

func parseOrderCursor(cursor string) (*time.Time, *string, error) {
	if strings.TrimSpace(cursor) == "" {
		return nil, nil, nil
	}
	parts := strings.SplitN(cursor, "|", 2)
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("invalid cursor format")
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || ts <= 0 {
		return nil, nil, fmt.Errorf("invalid cursor create_ts")
	}
	id := strings.TrimSpace(parts[1])
	if id == "" {
		return nil, nil, fmt.Errorf("invalid cursor id")
	}
	createdTs := time.UnixMilli(ts)
	return &createdTs, &id, nil
}

func buildOrdersPage(orders []*types.Order, limit int32) ([]*types.Order, bool, string) {
	if limit <= 0 {
		return orders, false, ""
	}
	hasMore := int32(len(orders)) > limit
	if hasMore {
		orders = orders[:limit]
	}
	next := ""
	if len(orders) > 0 {
		last := orders[len(orders)-1]
		if !last.CreatedTs.IsZero() && last.OrderID.String() != "" {
			next = fmt.Sprintf("%d|%s", last.CreatedTs.UnixMilli(), last.OrderID.String())
		}
	}
	return orders, hasMore, next
}

// isRiskReducingOrder 判断是否为降险订单（用于豁免部分风控规则）
// 合约：reduce_only=true 或 close_position=true 视为降险
// 现货：卖出非稳定币（base 不在稳定币列表）视为降险
func (s *Service) getAccount(ctx context.Context, accountID string) (*types.Account, error) {
	acct, err := s.db.AccountRepo.GetById(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return nil, errors.New(errors.NotFound, "account not found")
	}
	return converter.AccountRepo2Types(acct), nil
}
