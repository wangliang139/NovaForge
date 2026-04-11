package streamsvc

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/wangliang139/llt-trade/server/pkg/entity"
	"github.com/wangliang139/llt-trade/server/pkg/repos"
	"github.com/wangliang139/llt-trade/server/pkg/types"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/mow/errors"
)

type Service struct {
	db *repos.Entity
}

func New(db *repos.Entity) (*Service, error) {
	return &Service{
		db: db,
	}, nil
}

func (s *Service) EnsureSubscription(ctx context.Context, req *ctypes.EnsureSubscriptionRequest) (*ctypes.EnsureSubscriptionResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	ex := req.Exchange
	if !ex.IsValid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid exchange: %s", req.Exchange))
	}
	if req.Selector == nil {
		return nil, errors.New(errors.InvalidArgument, "selector is required")
	}
	selector := ctypes.StreamSelector{
		Stream: req.Selector.Stream,
	}
	if req.Selector.Interval != nil {
		selector.Interval = req.Selector.Interval
	}
	if req.Selector.AccountId != nil {
		selector.Account = req.Selector.AccountId
	}
	if req.Selector.Symbol != nil {
		smb, err := ctypes.ParseSymbol(*req.Selector.Symbol)
		if err != nil {
			return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %s", *req.Selector.Symbol))
		}
		selector.Symbol = &smb
	}
	subscription, err := entity.Market.EnsureSubscription(ctx, ex, selector)
	if err != nil {
		return nil, err
	}
	return &ctypes.EnsureSubscriptionResponse{
		Subscription: subscription,
	}, nil
}

func (s *Service) ReleaseSubscription(ctx context.Context, req *ctypes.ReleaseSubscriptionRequest) (*ctypes.ReleaseSubscriptionResponse, error) {
	if req == nil || strings.TrimSpace(req.ID) == "" {
		return nil, errors.New(errors.InvalidArgument, "id is required")
	}
	success, err := entity.Market.ReleaseSubscription(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	return &ctypes.ReleaseSubscriptionResponse{
		Success: success,
	}, nil
}

func (s *Service) ListActiveSubscriptions(ctx context.Context, req *ctypes.ListActiveSubscriptionsRequest) (*ctypes.ListActiveSubscriptionsResponse, error) {
	var exchange *ctypes.Exchange
	var symbol *string
	var accountID *string
	if req != nil {
		exchange = req.Exchange
		symbol = req.Symbol
		accountID = req.AccountId
	}
	list, err := entity.Market.ListSubscriptions(exchange, symbol, accountID)
	if err != nil {
		return nil, err
	}
	return &ctypes.ListActiveSubscriptionsResponse{
		Subscriptions: list,
	}, nil
}

func (s *Service) GetStreamStats(ctx context.Context, req *ctypes.GetStreamStatsRequest) (*ctypes.GetStreamStatsResponse, error) {
	windowHours := int32(1)
	if req != nil && req.WindowHours > 0 {
		windowHours = req.WindowHours
	}
	list := entity.Market.GetConnectorStreamStats(int(windowHours))
	stats := make([]ctypes.StreamConnectorStats, 0, len(list))
	for _, st := range list {
		ex := ctypes.Exchange(st.Exchange)
		if !ex.IsValid() {
			if parsed, err := ctypes.ParseExchange(st.Exchange); err == nil {
				ex = parsed
			}
		}
		stats = append(stats, ctypes.StreamConnectorStats{
			Exchange:       ex,
			Stream:         st.Stream,
			EventCount:     st.EventCount,
			AvgLatencyMs:   st.AvgLatencyMs,
			MaxLatencyMs:   st.MaxLatencyMs,
			ReconnectCount: st.ReconnectCount,
		})
	}
	return &ctypes.GetStreamStatsResponse{Stats: stats}, nil
}

func (s *Service) SubscribeStream(ctx context.Context, req *ctypes.SubscribeStreamRequest) (<-chan *types.SubscribeStreamResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	if !req.StreamType.Valid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("unsupported stream type: %s", req.StreamType))
	}
	switch req.StreamType {
	case ctypes.StreamTypeSocial:
		return s.SubscribeSocialStream(ctx, req)
	case ctypes.StreamTypeAccount:
		return s.SubscribeAccountStream(ctx, req)
	case ctypes.StreamTypeTicker,
		ctypes.StreamTypeTrade,
		ctypes.StreamTypeDepth,
		ctypes.StreamTypeKline,
		ctypes.StreamTypeMarkPrice:
		return s.SubscribeMarketStream(ctx, req)
	default:
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("unsupported stream type: %s", req.StreamType.String()))
	}
}

func (s *Service) GetConnectorInfo(ctx context.Context, req *ctypes.GetConnectorInfoRequest) (*ctypes.GetConnectorInfoResponse, error) {
	if req == nil {
		return nil, errors.New(errors.InvalidArgument, "request is required")
	}
	if !req.Exchange.IsValid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid exchange: %s", req.Exchange))
	}
	conn, err := entity.Account.GetConnector(ctx, req.Exchange, req.AccountId)
	if err != nil {
		return nil, err
	}
	return &ctypes.GetConnectorInfoResponse{
		Exchange:  conn.Exchange(),
		IsPrivate: conn.IsPrivate(),
	}, nil
}

func (s *Service) SubscribeSocialStream(ctx context.Context, req *ctypes.SubscribeStreamRequest) (<-chan *types.SubscribeStreamResponse, error) {
	ch := make(chan *types.SubscribeStreamResponse, 1)
	docCh := entity.Document.DocActiveCh()
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case doc, ok := <-docCh:
				if !ok {
					return
				}
				if doc == nil {
					continue
				}
				resp := &ctypes.SubscribeStreamResponse{
					Envelope: &ctypes.Envelope{
						Topic:     "social",
						Stream:    ctypes.StreamTypeSocial,
						Ts:        doc.PublishedAt.UnixMilli(),
						ReceiveAt: doc.CreatedAt.UnixMilli(),
						PublishAt: doc.UpdatedAt.UnixMilli(),
						Payload:   types.NewSocialMessage(doc),
					},
				}
				select {
				case ch <- resp:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return ch, nil
}

func (s *Service) SubscribeAccountStream(ctx context.Context, req *ctypes.SubscribeStreamRequest) (<-chan *types.SubscribeStreamResponse, error) {
	accountID := strings.TrimSpace(lo.FromPtr(req.AccountId))
	if accountID == "" {
		return nil, errors.New(errors.InvalidArgument, "account_id is required for account stream")
	}
	var ex ctypes.Exchange
	if req.Exchange != nil && req.Exchange.IsValid() {
		ex = *req.Exchange
	} else {
		acct, err := entity.Account.GetAccount(ctx, accountID)
		if err != nil {
			return nil, err
		}
		if acct == nil {
			return nil, errors.New(errors.NotFound, fmt.Sprintf("account not found: %s", accountID))
		}
		ex = acct.Exchange
	}
	ch := make(chan *types.SubscribeStreamResponse, 1024)
	go func() {
		defer close(ch)
		selector := ctypes.StreamSelector{
			Stream:  ctypes.StreamTypeAccount,
			Account: &accountID,
		}
		topic := ctypes.TopicName(ex, selector)
		envCh, cancel := entity.Market.Engine().SubscribeTopic(topic, 256)
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case env, ok := <-envCh:
				if !ok {
					return
				}
				if env == nil {
					continue
				}
				select {
				case ch <- &ctypes.SubscribeStreamResponse{Envelope: env}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return ch, nil
}

func (s *Service) SubscribeMarketStream(ctx context.Context, req *ctypes.SubscribeStreamRequest) (<-chan *types.SubscribeStreamResponse, error) {
	if req.Exchange == nil || !req.Exchange.IsValid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid exchange: %v", req.Exchange))
	}
	ex := *req.Exchange
	if strings.TrimSpace(req.Symbol) == "" {
		return nil, errors.New(errors.InvalidArgument, "symbol is required")
	}
	symbol, err := ctypes.ParseSymbol(req.Symbol)
	if err != nil || !symbol.IsValid() {
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("invalid symbol: %s", req.Symbol))
	}

	streamType := req.StreamType
	switch streamType {
	case ctypes.StreamTypeTicker, ctypes.StreamTypeTrade, ctypes.StreamTypeDepth, ctypes.StreamTypeKline, ctypes.StreamTypeMarkPrice, ctypes.StreamTypeSocial:
	default:
		return nil, errors.New(errors.InvalidArgument, fmt.Sprintf("unsupported stream type: %s", streamType.String()))
	}

	selector := ctypes.StreamSelector{
		Stream: streamType,
		Symbol: &symbol,
	}
	if streamType == ctypes.StreamTypeKline {
		var interval ctypes.Interval
		if req.Interval != nil {
			interval = *req.Interval
		}
		selector.Interval = &interval
	}
	if err := selector.Validate(); err != nil {
		return nil, errors.New(errors.InvalidArgument, err.Error())
	}

	subscription, err := entity.Market.EnsureSubscription(ctx, ex, selector)
	if err != nil {
		return nil, err
	}

	ch := make(chan *types.SubscribeStreamResponse, 1024)
	go func() {
		defer close(ch)
		defer func() {
			_, _ = entity.Market.ReleaseSubscription(ctx, subscription.ID)
		}()

		topic := ctypes.TopicName(ex, selector)
		envCh, cancel := entity.Market.Engine().SubscribeTopic(topic, 256)
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case env, ok := <-envCh:
				if !ok {
					return
				}
				if env == nil {
					continue
				}
				select {
				case ch <- &ctypes.SubscribeStreamResponse{Envelope: env}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return ch, nil
}
