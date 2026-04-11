package resolver

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/wangliang139/NovaForge/server/pkg/action/converter"
	"github.com/wangliang139/NovaForge/server/pkg/action/model"
	"github.com/wangliang139/NovaForge/server/pkg/gateway/auth"
	"github.com/wangliang139/NovaForge/server/pkg/gateway/stream"
	"github.com/wangliang139/NovaForge/server/pkg/gateway/wsctx"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	mowerror "github.com/wangliang139/mow/errors"
)

var typeMap = map[model.StreamType]types.StreamType{
	model.StreamTypeTicker:    types.StreamTypeTicker,
	model.StreamTypeTrade:     types.StreamTypeTrade,
	model.StreamTypeDepth:     types.StreamTypeDepth,
	model.StreamTypeKline:     types.StreamTypeKline,
	model.StreamTypeMarkPrice: types.StreamTypeMarkPrice,
	model.StreamTypeSocial:    types.StreamTypeSocial,
	model.StreamTypeAccount:   types.StreamTypeAccount,
}

func (r *Resolver) acquireFlow(ctx context.Context, input model.StreamInput) (<-chan *types.SubscribeStreamResponse, func(), error) {
	if _, ok := auth.GetUserFromContext(ctx); !ok {
		return nil, nil, mowerror.New(mowerror.PermissionDenied, "unauthorized")
	}

	pbType, ok := typeMap[input.Type]
	if !ok {
		return nil, nil, mowerror.New(mowerror.InvalidArgument, "unsupported stream type: "+string(input.Type))
	}
	if input.Type == model.StreamTypeAccount {
		if input.Account == nil || strings.TrimSpace(*input.Account) == "" {
			return nil, nil, mowerror.New(mowerror.InvalidArgument, "account is required for account stream")
		}
	}
	var interval *types.Interval
	if input.Type == model.StreamTypeKline {
		if input.Interval == nil || strings.TrimSpace(*input.Interval) == "" {
			return nil, nil, mowerror.New(mowerror.InvalidArgument, "interval is required for kline stream")
		}
		interval = lo.ToPtr(types.Interval(*input.Interval))
		if !interval.Valid() {
			return nil, nil, mowerror.New(mowerror.InvalidArgument, fmt.Sprintf("invalid interval: %s", *input.Interval))
		}
	}

	connID, ok := wsctx.ConnIDFromContext(ctx)
	if !ok {
		return nil, nil, mowerror.New(mowerror.InvalidArgument, "websocket connection id missing")
	}
	spec := stream.FlowSpec{
		Stream:   pbType,
		Account:  input.Account,
		Exchange: input.Exchange,
		Symbol:   input.Symbol,
		Interval: interval,
	}
	return r.StreamManager.Acquire(ctx, connID, spec)
}

func (r *Resolver) subscribeMarketStream(ctx context.Context, input model.StreamInput) (<-chan *model.StreamEvent, error) {
	ch, release, err := r.acquireFlow(ctx, input)
	if err != nil {
		return nil, err
	}

	out := make(chan *model.StreamEvent, 256)
	go func() {
		defer release()
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				evt := converter.ConvertStreamEvent(input.Type, msg)
				if evt == nil {
					continue
				}
				select {
				case out <- evt:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}
