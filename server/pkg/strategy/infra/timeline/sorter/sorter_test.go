package sorter

import (
	"testing"
	"time"

	"github.com/wangliang139/llt-trade/server/pkg/strategy/types"
)

func TestSorterConfig_Compare_TotalOrder(t *testing.T) {
	ts := time.Date(2025, 12, 18, 0, 0, 0, 0, time.UTC)

	cfg := DefaultSorterConfig()
	// 构造带 Signal 的 message，避免 nil deref；并确保 Type 优先级生效。
	a := types.NewMessageWithSource(types.SignalSourceDatasource, "b", 2, &types.KlineSignal{
		BaseSignal: types.BaseSignal{Ts: ts},
		IsClosed:   true,
	}, false)
	b := types.NewMessageWithSource(types.SignalSourceDatasource, "a", 1, &types.TradeSignal{
		BaseSignal: types.BaseSignal{Ts: ts},
	}, false)

	// kline 优先级高于 trade（数值更小），因此 a < b
	if got := cfg.Compare(a, b); got >= 0 {
		t.Fatalf("expected a < b, got %d", got)
	}

	// 同 Type：SourceID tie-break
	c := types.NewMessageWithSource(types.SignalSourceDatasource, "a", 10, &types.KlineSignal{
		BaseSignal: types.BaseSignal{Ts: ts},
		IsClosed:   true,
	}, false)
	d := types.NewMessageWithSource(types.SignalSourceDatasource, "b", 1, &types.KlineSignal{
		BaseSignal: types.BaseSignal{Ts: ts},
		IsClosed:   true,
	}, false)
	if got := cfg.Compare(c, d); got >= 0 {
		t.Fatalf("expected c < d by SourceID, got %d", got)
	}

	// 同 SourceID：SourceSeq tie-break
	e := types.NewMessageWithSource(types.SignalSourceDatasource, "a", 1, &types.KlineSignal{
		BaseSignal: types.BaseSignal{Ts: ts},
		IsClosed:   true,
	}, false)
	f := types.NewMessageWithSource(types.SignalSourceDatasource, "a", 2, &types.KlineSignal{
		BaseSignal: types.BaseSignal{Ts: ts},
		IsClosed:   true,
	}, false)
	if got := cfg.Compare(e, f); got >= 0 {
		t.Fatalf("expected e < f by SourceSeq, got %d", got)
	}
}
