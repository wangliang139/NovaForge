package eventflow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/rs/zerolog/log"
	ctypes "github.com/wangliang139/llt-trade/server/pkg/types"
	"github.com/wangliang139/llt-trade/server/pkg/internal/chsdk"
	"github.com/wangliang139/mow/snowflake"
)

const (
	defaultTableName   = "event_flow"
	defaultBatchSize   = 100
	defaultFlushPeriod = 2 * time.Second
)

type EventRecord struct {
	ID        int64
	AccountID string
	Exchange  string
	Stream    string
	Topic     string
	EventKind string
	Ts        time.Time
	ReceiveAt time.Time
	PublishAt time.Time
	IngestAt  time.Time
	Payload   string
}

type Recorder struct {
	client *chsdk.Client
	table  string

	ch        chan EventRecord
	batchSize int
	flushTick *time.Ticker

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewRecorder(client *chsdk.Client) (*Recorder, error) {
	if client == nil {
		return nil, fmt.Errorf("clickhouse client is required")
	}

	dbName := client.Conf.Database
	if dbName == "" {
		dbName = "trade"
	}
	tableName := fmt.Sprintf("%s.%s", dbName, defaultTableName)

	ctx, cancel := context.WithCancel(context.Background())
	r := &Recorder{
		client:    client,
		table:     tableName,
		ch:        make(chan EventRecord, 1000),
		batchSize: defaultBatchSize,
		flushTick: time.NewTicker(defaultFlushPeriod),
		ctx:       ctx,
		cancel:    cancel,
	}

	r.wg.Add(1)
	go r.run()

	return r, nil
}

func (r *Recorder) Record(envelope *ctypes.Envelope) {
	if envelope == nil {
		return
	}

	eventKind := inferEventKind(envelope.Payload)
	payloadJSON, err := sonic.Marshal(envelope)
	if err != nil {
		log.Warn().Err(err).Msg("failed to marshal envelope for recording")
		return
	}

	accountID := ""
	if envelope.Account != nil {
		accountID = *envelope.Account
	}

	id := snowflake.Generate().Int64()
	record := EventRecord{
		ID:        id,
		AccountID: accountID,
		Exchange:  envelope.Exchange,
		Stream:    envelope.Stream.String(),
		Topic:     envelope.Topic,
		EventKind: eventKind,
		Ts:        time.UnixMilli(envelope.Ts),
		ReceiveAt: time.UnixMilli(envelope.ReceiveAt),
		PublishAt: time.UnixMilli(envelope.PublishAt),
		IngestAt:  time.Now(),
		Payload:   string(payloadJSON),
	}

	select {
	case r.ch <- record:
	case <-r.ctx.Done():
	default:
		log.Warn().Msg("event recorder channel full, dropping record")
	}
}

func (r *Recorder) Close() error {
	r.cancel()
	r.flushTick.Stop()
	close(r.ch)
	r.wg.Wait()
	return nil
}

func (r *Recorder) run() {
	defer r.wg.Done()

	batch := make([]EventRecord, 0, r.batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := r.insertBatch(batch); err != nil {
			log.Err(err).Int("count", len(batch)).Msg("failed to insert event batch")
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-r.ctx.Done():
			flush()
			return
		case <-r.flushTick.C:
			flush()
		case record, ok := <-r.ch:
			if !ok {
				flush()
				return
			}
			batch = append(batch, record)
			if len(batch) >= r.batchSize {
				flush()
			}
		}
	}
}

func (r *Recorder) insertBatch(records []EventRecord) error {
	if len(records) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	batch, err := r.client.PrepareBatch(ctx, fmt.Sprintf("INSERT INTO %s", r.table))
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, record := range records {
		err := batch.Append(
			record.ID,
			record.AccountID,
			record.Exchange,
			record.Stream,
			record.Topic,
			record.EventKind,
			record.Ts,
			record.ReceiveAt,
			record.PublishAt,
			record.IngestAt,
			record.Payload,
		)
		if err != nil {
			return fmt.Errorf("append record: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("send batch: %w", err)
	}

	return nil
}

func inferEventKind(msg *ctypes.Message) string {
	if msg == nil {
		return "unknown"
	}
	switch {
	case msg.Ticker != nil:
		return "ticker"
	case msg.Trade != nil:
		return "trade"
	case msg.Depth != nil:
		return "depth"
	case msg.Kline != nil:
		return "kline"
	case msg.MarkPrice != nil:
		return "mark_price"
	case msg.BalanceSnapshot != nil:
		return "balance_snapshot"
	case msg.BalanceUpdate != nil:
		return "balance_update"
	case msg.PositionSnapshot != nil:
		return "position_snapshot"
	case msg.PositionsUpdate != nil:
		return "positions_update"
	case msg.Order != nil:
		return "order"
	case msg.SymbolLeverage != nil:
		return "symbol_leverage"
	case msg.Fill != nil:
		return "fill"
	default:
		return "unknown"
	}
}
