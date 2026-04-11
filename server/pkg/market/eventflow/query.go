package eventflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wangliang139/NovaForge/server/pkg/internal/chsdk"
)

// QueryFilter 支持按账户、流类型、交易所、事件类型等筛选；所有字段均可选，至少建议指定时间或 ID 范围以控制扫描量。
type QueryFilter struct {
	AccountID string // 为空表示不过滤账户（查所有 event）
	Stream    string // 如 "account_raw", "account", "ticker", "kline", "trade", "depth", "mark_price" 等，空表示全部
	Exchange  string // 交易所，空表示全部
	EventKind string // event_kind，空表示全部
	Topic     string // topic 前缀或精确匹配，空表示全部
	StartID   *int64
	StartTs   *time.Time
	Limit     int
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

	return &Querier{
		client: client,
		table:  tableName,
	}, nil
}

func (q *Querier) Query(ctx context.Context, filter QueryFilter) ([]EventRecord, error) {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	if filter.Limit > 1000 {
		filter.Limit = 1000
	}

	var sb strings.Builder
	args := make([]any, 0, 8)

	sb.WriteString("SELECT id, account_id, exchange, stream, topic, event_kind, ts, receive_at, publish_at, ingest_at, payload FROM ")
	sb.WriteString(q.table)

	var conds []string
	if filter.AccountID != "" {
		conds = append(conds, "account_id = ?")
		args = append(args, filter.AccountID)
	}
	if filter.Stream != "" {
		conds = append(conds, "stream = ?")
		args = append(args, filter.Stream)
	}
	if filter.Exchange != "" {
		conds = append(conds, "exchange = ?")
		args = append(args, filter.Exchange)
	}
	if filter.EventKind != "" {
		conds = append(conds, "event_kind = ?")
		args = append(args, filter.EventKind)
	}
	if filter.Topic != "" {
		conds = append(conds, "topic = ?")
		args = append(args, filter.Topic)
	}
	if filter.StartTs != nil && !filter.StartTs.IsZero() {
		conds = append(conds, "ts >= ?")
		args = append(args, filter.StartTs)
	}
	if filter.StartID != nil && *filter.StartID > 0 {
		conds = append(conds, "id > ?")
		args = append(args, *filter.StartID)
	}
	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}

	sb.WriteString(" ORDER BY ts ASC, id ASC LIMIT ?")
	args = append(args, filter.Limit)

	rows, err := q.client.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	records := make([]EventRecord, 0, filter.Limit)
	for rows.Next() {
		var record EventRecord
		if err := rows.Scan(
			&record.ID,
			&record.AccountID,
			&record.Exchange,
			&record.Stream,
			&record.Topic,
			&record.EventKind,
			&record.Ts,
			&record.ReceiveAt,
			&record.PublishAt,
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
