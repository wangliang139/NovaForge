package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/wangliang139/NovaForge/server/pkg/internal/chsdk"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/logging"
	"github.com/wangliang139/mow/snowflake"
)

const (
	defaultConsoleLogDatabase = "trade"
	consoleLogTableName       = "bot_console_log"
)

type ConsoleLogFilter struct {
	BotID     int32
	Limit     int
	Cursor    string
	StartTime *time.Time
	EndTime   *time.Time
	Level     *string
}

type ConsoleLogRecord struct {
	ID         int64
	BotID      int32
	Level      string
	Message    string
	Ts         time.Time
	CreatedAt  time.Time
}

type ClickhouseStorage struct {
	store *chsdk.Client
	table string

	botID      int32
	timeout    time.Duration
}

func NewClickhouseStorage(botId int32, timeout time.Duration, client *chsdk.Client) (*ClickhouseStorage, error) {
	if botId == 0 {
		return nil, fmt.Errorf("bot id is required")
	}
	if timeout <= 0 {
		timeout = 1 * time.Second
	}
	if client == nil {
		return nil, fmt.Errorf("client is required")
	}

	dbName := strings.TrimSpace(client.Conf.Database)
	if dbName == "" {
		dbName = defaultConsoleLogDatabase
	}
	return &ClickhouseStorage{
		store:      client,
		table:      fmt.Sprintf("%s.%s", dbName, consoleLogTableName),
		botID:      botId,
		timeout:    timeout,
	}, nil
}

func (l *ClickhouseStorage) Write(ctx context.Context, entry logging.Entry) error {
	if l == nil || l.store == nil {
		return fmt.Errorf("console log store is not initialized")
	}

	record := ConsoleLogRecord{
		ID:         snowflake.Generate().Int64(),
		BotID:      l.botID,
		Level:      entry.Level,
		Message:    entry.Message,
		Ts:         entry.Ts,
		CreatedAt:  time.Now(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()
	if err := l.Insert(ctx, record); err != nil {
		log.Warn().Err(err).Int32("bot_id", l.botID).Msg("failed to write console log to clickhouse")
	}
	return nil
}

func (l *ClickhouseStorage) Insert(ctx context.Context, record ConsoleLogRecord) error {
	if l == nil || l.store == nil {
		return fmt.Errorf("console log store is not initialized")
	}
	query := fmt.Sprintf(
		"INSERT INTO %s (id, bot_id, level, message, ts, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		l.table,
	)
	return l.store.Exec(ctx, query, record.ID, record.BotID, record.Level, record.Message, record.Ts, record.CreatedAt)
}

func (l *ClickhouseStorage) List(ctx context.Context, filter ConsoleLogFilter) ([]ConsoleLogRecord, string, error) {
	if l == nil || l.store == nil {
		return nil, "", fmt.Errorf("console log store is not initialized")
	}
	if filter.BotID == 0 {
		return nil, "", fmt.Errorf("bot id is required")
	}
	if filter.Limit <= 0 {
		filter.Limit = 100
	}

	var sb strings.Builder
	args := make([]any, 0, 10)
	sb.WriteString("SELECT id, bot_id, level, message, ts, created_at FROM ")
	sb.WriteString(l.table)
	sb.WriteString(" WHERE bot_id = ?")
	args = append(args, filter.BotID)

	if filter.StartTime != nil {
		sb.WriteString(" AND ts >= ?")
		args = append(args, *filter.StartTime)
	}
	if filter.EndTime != nil {
		sb.WriteString(" AND ts <= ?")
		args = append(args, *filter.EndTime)
	}
	if filter.Level != nil {
		sb.WriteString(" AND level = ?")
		args = append(args, *filter.Level)
	}
	if filter.Cursor != "" {
		id, err := strconv.ParseInt(filter.Cursor, 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		sb.WriteString(" AND id < ?")
		args = append(args, id)
	}

	sb.WriteString(" ORDER BY id DESC LIMIT ?")
	args = append(args, filter.Limit)

	rows, err := l.store.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	logs := make([]ConsoleLogRecord, 0, filter.Limit)
	for rows.Next() {
		var record ConsoleLogRecord
		if err := rows.Scan(&record.ID, &record.BotID, &record.Level, &record.Message, &record.Ts, &record.CreatedAt); err != nil {
			return nil, "", err
		}
		logs = append(logs, record)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(logs) == 0 {
		return logs, "", nil
	}

	last := logs[len(logs)-1]
	return logs, strconv.FormatInt(last.ID, 10), nil
}
