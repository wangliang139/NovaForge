package signalflow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/rs/zerolog/log"
	"github.com/wangliang139/NovaForge/server/pkg/internal/chsdk"
	stypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
	"github.com/wangliang139/mow/snowflake"
)

const (
	defaultTableName   = "bot_signal_flow"
	defaultBatchSize   = 200
	defaultFlushPeriod = 2 * time.Second
)

type botSignal struct {
	BotID  int32
	Signal stypes.Signal
}

type SignalRecord struct {
	ID         int64
	BotID      int32
	AccountID  string
	Exchange   string
	Stream     string
	Topic      string
	EventKind  string
	Ts         time.Time
	InboundAt  time.Time
	OutboundAt time.Time
	ReceiveAt  time.Time
	IngestAt   time.Time
	Payload    string
}

// Recorder records bot-scoped signals into ClickHouse asynchronously in batches.
type Recorder struct {
	client *chsdk.Client
	table  string

	ch        chan botSignal
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
		ch:        make(chan botSignal, 2000),
		batchSize: defaultBatchSize,
		flushTick: time.NewTicker(defaultFlushPeriod),
		ctx:       ctx,
		cancel:    cancel,
	}

	r.wg.Add(1)
	go r.run()

	return r, nil
}

func (r *Recorder) Record(botID int32, signal stypes.Signal) {
	record := botSignal{
		BotID:  botID,
		Signal: signal,
	}

	select {
	case r.ch <- record:
	case <-r.ctx.Done():
	default:
		log.Warn().Int32("bot_id", botID).Msg("signal recorder channel full, dropping record")
	}
}

func (r *Recorder) Close() error {
	if r == nil {
		return nil
	}
	r.cancel()
	if r.flushTick != nil {
		r.flushTick.Stop()
	}
	if r.ch != nil {
		close(r.ch)
	}
	r.wg.Wait()
	if r.client != nil {
		if err := r.client.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close clickhouse client (signal recorder)")
		}
	}
	return nil
}

func (r *Recorder) run() {
	defer r.wg.Done()

	batch := make([]botSignal, 0, r.batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := r.insertBatch(batch); err != nil {
			log.Err(err).Int("count", len(batch)).Msg("failed to insert signal batch")
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

func (r *Recorder) insertBatch(records []botSignal) error {
	if len(records) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := fmt.Sprintf(
		"INSERT INTO %s (id, bot_id, account_id, exchange, stream, topic, event_kind, ts, inbound_at, outbound_at, receive_at, ingest_at, payload)",
		r.table,
	)
	batch, err := r.client.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, record := range records {
		signal := record.Signal
		botID := record.BotID
		payloadJSON, err := sonic.Marshal(signal)
		if err != nil {
			log.Err(err).Int32("bot_id", botID).Msg("failed to marshal signal for recording")
			continue
		}

		now := time.Now()
		accountID := ""
		if signal.GetAccountID() != nil {
			accountID = *signal.GetAccountID()
		}
		exchange := ""
		if signal.GetExchange() != nil {
			exchange = signal.GetExchange().String()
		}
		topic := ""
		if signal.GetTopic() != nil {
			topic = *signal.GetTopic()
		}

		if err := batch.Append(
			snowflake.Generate().Int64(),
			botID,
			accountID,
			exchange,
			signal.GetType().String(),
			topic,
			string(signal.GetKind()),
			signal.GetTimestamp(),
			signal.GetInboundAt(),
			signal.GetOutboundAt(),
			signal.GetReceiveAt(),
			now,
			string(payloadJSON),
		); err != nil {
			return fmt.Errorf("append record: %w", err)
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("send batch: %w", err)
	}

	return nil
}
