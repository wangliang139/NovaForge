-- Event Flow Table (all streams: account, market, etc.)
-- Purpose: Record all event streams for debugging and auditing
-- Used by: llt-data-api event flow recorder

CREATE TABLE IF NOT EXISTS trade.event_flow (
    id Int64,
    account_id String COMMENT 'empty for non-account streams (ticker, kline, trade, depth, mark_price, etc.)',
    exchange LowCardinality(String),
    stream LowCardinality(String) COMMENT 'account_raw, account, ticker, trade, depth, kline, mark_price, social',
    topic String,
    event_kind LowCardinality(String) COMMENT 'ticker, trade, depth, kline, mark_price, balance_snapshot, balance_update, position_snapshot, positions_update, order, symbol_leverage, fill, unknown',
    ts DateTime64(3) COMMENT 'event timestamp',
    receive_at DateTime64(3) COMMENT 'event received timestamp',
    publish_at DateTime64(3) COMMENT 'event published timestamp',
    ingest_at DateTime64(3) COMMENT 'ingestion timestamp',
    payload String COMMENT 'full envelope JSON'
) ENGINE = MergeTree
PARTITION BY toYYYYMMDD(ts)
ORDER BY (account_id, stream, ts, id)
TTL ts + INTERVAL 7 DAY
SETTINGS index_granularity = 8192;

-- Example queries:
-- 1. All events for an account in a time range:
--    SELECT * FROM trade.event_flow
--    WHERE account_id = 'xxx' AND ts >= '2024-01-01' AND ts < '2024-01-02'
--    ORDER BY ts, id LIMIT 100;
--
-- 2. All market events by stream (no account_id):
--    SELECT * FROM trade.event_flow
--    WHERE stream = 'kline' AND ts >= now() - INTERVAL 10 MINUTE
--    ORDER BY ts, id LIMIT 100;
--
-- 3. Count events by type:
--    SELECT event_kind, count() FROM trade.event_flow
--    WHERE ts >= now() - INTERVAL 1 HOUR
--    GROUP BY event_kind;
