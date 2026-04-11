package signalflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wangliang139/NovaForge/server/pkg/internal/chsdk"
)

type QueryFilter struct {
	BotID   int32
	Stream  string // signal type, e.g. "kline"; empty for all
	StartID *int64
	StartTs *time.Time
	Limit   int
}

type Querier struct {
	client *chsdk.Client
	table  string
}

func NewQuerier(client *chsdk.Client) (*Querier, error) {
	if client == nil {
		return nil, fmt.Errorf("clickhouse client is required")
	}
	dbName := client.Conf.Database
	if dbName == "" {
		dbName = "trade"
	}
	tableName := fmt.Sprintf("%s.%s", dbName, defaultTableName)
	return &Querier{client: client, table: tableName}, nil
}

func (q *Querier) Query(ctx context.Context, filter QueryFilter) ([]SignalRecord, error) {
	if q == nil || q.client == nil {
		return nil, fmt.Errorf("querier is not initialized")
	}
	if filter.BotID <= 0 {
		return nil, fmt.Errorf("bot_id is required")
	}
	if filter.Limit <= 0 {
		filter.Limit = 200
	}
	if filter.Limit > 1000 {
		filter.Limit = 1000
	}

	var sb strings.Builder
	args := make([]any, 0, 6)

	sb.WriteString("SELECT id, bot_id, account_id, exchange, stream, topic, event_kind, ts, inbound_at, outbound_at, receive_at, ingest_at, payload FROM ")
	sb.WriteString(q.table)
	sb.WriteString(" WHERE bot_id = ?")
	args = append(args, filter.BotID)

	if filter.Stream != "" {
		sb.WriteString(" AND stream = ?")
		args = append(args, filter.Stream)
	}
	if filter.StartTs != nil && !filter.StartTs.IsZero() {
		sb.WriteString(" AND ts >= ?")
		args = append(args, filter.StartTs)
	}
	if filter.StartID != nil && *filter.StartID > 0 {
		sb.WriteString(" AND id > ?")
		args = append(args, *filter.StartID)
	}

	sb.WriteString(" ORDER BY id ASC, ts ASC LIMIT ?")
	args = append(args, filter.Limit)

	rows, err := q.client.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	records := make([]SignalRecord, 0, filter.Limit)
	for rows.Next() {
		var record SignalRecord
		if err := rows.Scan(
			&record.ID,
			&record.BotID,
			&record.AccountID,
			&record.Exchange,
			&record.Stream,
			&record.Topic,
			&record.EventKind,
			&record.Ts,
			&record.InboundAt,
			&record.OutboundAt,
			&record.ReceiveAt,
			&record.IngestAt,
			&record.Payload,
		); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return records, nil
}

// StatsFilter 统计过滤条件
type StatsFilter struct {
	StartTs time.Time
	EndTs   time.Time
	BotID   int32 // 0 表示全部
}

// SignalTypeStats 按 bot_id、stream 聚合的统计
type SignalTypeStats struct {
	BotID        int32
	Stream       string
	EventCount   uint64
	AvgLatencyMs float64
	MaxLatencyMs int64
}

// QueryStats 按 bot_id、stream 聚合统计（ClickHouse）
func (q *Querier) QueryStats(ctx context.Context, filter StatsFilter) ([]SignalTypeStats, error) {
	if q == nil || q.client == nil {
		return nil, fmt.Errorf("querier is not initialized")
	}

	var sb strings.Builder
	args := make([]any, 0, 4)

	sb.WriteString("SELECT bot_id, stream, COUNT(*) AS event_count, ")
	sb.WriteString("COALESCE(avg(dateDiff('millisecond', ts, ingest_at)), 0) AS avg_latency_ms, ")
	sb.WriteString("COALESCE(max(dateDiff('millisecond', ts, ingest_at)), 0) AS max_latency_ms ")
	sb.WriteString("FROM ")
	sb.WriteString(q.table)
	sb.WriteString(" WHERE ts >= ? AND ts < ?")
	args = append(args, filter.StartTs, filter.EndTs)

	if filter.BotID > 0 {
		sb.WriteString(" AND bot_id = ?")
		args = append(args, filter.BotID)
	}

	sb.WriteString(" GROUP BY bot_id, stream ORDER BY bot_id, stream")

	rows, err := q.client.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var results []SignalTypeStats
	for rows.Next() {
		var s SignalTypeStats
		if err := rows.Scan(&s.BotID, &s.Stream, &s.EventCount, &s.AvgLatencyMs, &s.MaxLatencyMs); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		results = append(results, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return results, nil
}
