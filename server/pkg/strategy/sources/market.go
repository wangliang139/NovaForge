package sources

import (
	"context"

	"github.com/wangliang139/llt-trade/server/pkg/strategy/proxy"
	ss "github.com/wangliang139/llt-trade/server/pkg/strategy/signal"
	stypes "github.com/wangliang139/llt-trade/server/pkg/strategy/types"
)

type klineFetcher struct {
	spec ss.KlineSignalSpec
}

var _ stypes.Fetcher = (*klineFetcher)(nil)

func (f *klineFetcher) ID() string {
	return f.spec.GetID()
}

func (f *klineFetcher) Spec() stypes.SignalSpec {
	return &f.spec
}

func (f *klineFetcher) Fetch(ctx context.Context, cursor stypes.Cursor, limit int) ([]*stypes.Message, stypes.Cursor, error) {
	klines, err := proxy.GetHisKlines(ctx, *f.spec.GetExchange(), *f.spec.GetSymbol(), f.spec.Interval, &cursor.Ts, nil, &limit)
	if err != nil {
		return nil, stypes.Cursor{}, err
	}
	if len(klines) == 0 {
		return nil, cursor, nil
	}
	events := make([]*stypes.Message, 0, len(klines))
	base := uint64(cursor.ID)
	for i, kline := range klines {
		// 产生两条事件：
		// - bar_open: Ts=OpenTs（用于 next open 撮合市价单）
		// - bar_close(KlineSignal): Ts=CloseTs（用于 bar 级撮合限价单/更新收盘价）
		//
		// 注意：由于 cursor 以 CloseTs 推进，第一根 bar 可能出现 OpenTs < cursor.StartTs 的情况；
		// 为避免时间线倒退（推进到回测窗口之外），这里跳过早于 spec.StartTs 的 bar_open。
		seqOpen := base + uint64(i)*2 + 1
		seqClose := base + uint64(i)*2 + 2

		if !kline.OpenTs.Before(f.spec.GetStartTs()) {
			events = append(events, stypes.NewMessageWithSource(
				stypes.SignalSourceDatasource,
				f.ID(),
				seqOpen,
				&stypes.KlineSignal{
					BaseSignal: stypes.BaseSignal{
						ID:       f.ID(),
						Exchange: f.spec.GetExchange(),
						Symbol:   f.spec.GetSymbol(),
						Ts:       kline.OpenTs,
					},
					Interval: f.spec.Interval,
					Open:     kline.Open,
					IsClosed: false,
				},
				false,
			))
		}

		events = append(events, stypes.NewMessageWithSource(
			stypes.SignalSourceDatasource,
			f.ID(),
			seqClose,
			&stypes.KlineSignal{
				BaseSignal: stypes.BaseSignal{
					ID:       f.ID(),
					Exchange: f.spec.GetExchange(),
					Symbol:   f.spec.GetSymbol(),
					Ts:       kline.CloseTs,
				},
				Interval: f.spec.Interval,
				Open:     kline.Open,
				High:     kline.High,
				Low:      kline.Low,
				Close:    kline.Close,
				Volume:   kline.Volume,
				OpenTs:   kline.OpenTs.UnixMilli(),
				IsClosed: true,
			},
			false,
		))
	}
	cursor.ID = klines[len(klines)-1].CloseTs.UnixMilli()
	cursor.Ts = klines[len(klines)-1].CloseTs
	return events, cursor, nil
}

// NewKlineFetcher 创建 kline fetcher
func NewKlineFetcher(spec ss.KlineSignalSpec) stypes.Fetcher {
	return &klineFetcher{
		spec: spec,
	}
}

func NewKlineSource(spec ss.KlineSignalSpec, isDerived bool) *CursorFetchSource {
	fetcher := NewKlineFetcher(spec)
	return NewCursorFetchSource(fetcher, stypes.Cursor{
		ID: 0,
		Ts: spec.GetStartTs(),
	}, 100, 10, 200, isDerived)
}
