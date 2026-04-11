package sources

import (
	"context"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	ctypes "github.com/wangliang139/NovaForge/server/pkg/types"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
	"github.com/wangliang139/NovaForge/server/pkg/repos/ds_items"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	"github.com/wangliang139/NovaForge/server/pkg/types"
)

type DbSource struct {
	db *repos.Entity

	datasource *types.DataSource
	spec       stypes.SignalSpec
	isDerived  bool

	// Source support
	mu        sync.Mutex
	closed    bool
	exhausted bool
	failed    error
	cursor    stypes.Cursor
	buffer    []*stypes.Message

	batchSize int
}

var _ stypes.Source = (*DbSource)(nil)

func NewDbSource(db *repos.Entity, spec stypes.SignalSpec, datasource *types.DataSource, isDerived bool) *DbSource {
	return &DbSource{
		db:         db,
		datasource: datasource,
		spec:       spec,
		isDerived:  isDerived,
		// 默认预取参数（可后续按需暴露为 option）
		batchSize: 256,
	}
}

func (d *DbSource) ID() string {
	return d.spec.GetID()
}

func (d *DbSource) Spec() stypes.SignalSpec {
	return d.spec
}

func (d *DbSource) IsDerived() bool {
	return d.isDerived
}

func (d *DbSource) Datasource() *types.DataSource {
	return d.datasource
}

func (d *DbSource) Fetch(ctx context.Context, cursor stypes.Cursor, limit int) ([]*stypes.Message, stypes.Cursor, error) {
	startTs := d.spec.GetStartTs()
	startID := int64(0)
	if cursor.Ts.After(d.spec.GetStartTs()) || cursor.Ts.Equal(d.spec.GetStartTs()) {
		startTs = cursor.Ts
		startID = cursor.ID
	}
	items, err := d.db.DsItemsRepo.GetItemsByDsIdAndTs(ctx, ds_items.GetItemsByDsIdAndTsParams{
		DsID:    d.datasource.ID,
		StartTs: startTs,
		StartID: startID,
		EndTs:   d.spec.GetEndTs(),
		Limit:   int32(limit),
	})
	if err != nil {
		return nil, cursor, err
	}
	var exchange *ctypes.Exchange
	if d.datasource.Exchange != nil {
		ex, err := ctypes.ParseExchange((*d.datasource.Exchange).String())
		if err != nil {
			return nil, cursor, err
		}
		if ex.IsValid() {
			exchange = &ex
		}
	}
	var symbol *ctypes.Symbol
	if d.datasource.Symbol != nil {
		sym, err := ctypes.ParseSymbol((*d.datasource.Symbol).String())
		if err != nil {
			return nil, cursor, err
		}
		if sym.IsValid() {
			symbol = &sym
		}
	}
	events := make([]*stypes.Message, len(items))
	base := uint64(startID)
	for i, item := range items {
		var payload stypes.KlineSignal
		if err := sonic.Unmarshal(item.Data, &payload); err != nil {
			return nil, cursor, err
		}
		payload.IsClosed = true
		scope := &stypes.SignalScope{
			Exchange: exchange,
			Symbol:   symbol,
		}
		if scope.Exchange == nil && scope.Symbol == nil {
			scope = nil
		}
		events[i] = stypes.NewMessageWithSource(
			stypes.SignalSourceDatasource,
			d.ID(),
			base+uint64(i)+1,
			&payload,
			d.isDerived,
		)
	}
	if len(items) > 0 {
		cursor.ID = items[len(items)-1].ID
		cursor.Ts = items[len(items)-1].Ts
	}
	return events, cursor, nil
}

func (d *DbSource) ensureBuffer(ctx context.Context) error {
	if d.closed || d.exhausted || d.failed != nil {
		return d.failed
	}
	if len(d.buffer) > 0 {
		return nil
	}

	limit := d.batchSize
	if limit <= 0 {
		limit = 256
	}
	startCursor := d.cursor

	events, newCursor, err := d.Fetch(ctx, startCursor, limit)
	if err != nil {
		d.failed = err
		return err
	}
	if len(events) == 0 {
		d.exhausted = true
		return nil
	}

	d.buffer = append(d.buffer, events...)
	d.cursor = newCursor
	return nil
}

func (d *DbSource) Peek(ctx context.Context) (*stypes.Message, bool, error) {
	if d == nil {
		return nil, false, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, false, context.Canceled
	}
	if err := d.ensureBuffer(ctx); err != nil {
		return nil, false, &stypes.SourceError{SourceID: d.ID(), Op: "peek", Err: err}
	}
	if len(d.buffer) == 0 {
		return nil, false, nil
	}
	return d.buffer[0], true, nil
}

func (d *DbSource) Next(ctx context.Context) (*stypes.Message, bool, error) {
	if d == nil {
		return nil, false, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, false, context.Canceled
	}
	if err := d.ensureBuffer(ctx); err != nil {
		return nil, false, &stypes.SourceError{SourceID: d.ID(), Op: "next", Err: err}
	}
	if len(d.buffer) == 0 {
		return nil, false, nil
	}
	ev := d.buffer[0]
	d.buffer = d.buffer[1:]
	return ev, true, nil
}

func (d *DbSource) Watermark(ctx context.Context) (ts time.Time, ok bool, err error) {
	_ = ctx
	return time.Time{}, false, nil
}

func (d *DbSource) Close() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()
	return nil
}
