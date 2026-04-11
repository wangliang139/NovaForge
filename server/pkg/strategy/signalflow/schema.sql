-- Bot Signal Flow Table
-- Purpose: Record bot-scoped signals for debugging and auditing
-- Note: Schema is aligned with llt-data-api eventflow fields.

CREATE TABLE IF NOT EXISTS trade.bot_signal_flow (
    id Int64,
    bot_id Int32,
    account_id String,
    exchange LowCardinality(String),
    stream LowCardinality(String) COMMENT 'signal type, e.g. kline/ticker/order/balance',
    topic String,
    event_kind LowCardinality(String) COMMENT 'signal kind, e.g. kline/fill/order_snapshot',
    ts DateTime64(3) COMMENT 'signal timestamp',
    inbound_at DateTime64(3) COMMENT 'signal received timestamp (dispatcher)',
    outbound_at DateTime64(3) COMMENT 'signal published timestamp (dispatcher)',
    receive_at DateTime64(3) COMMENT 'signal received timestamp (local)',
    ingest_at DateTime64(3) COMMENT 'ingestion timestamp',
    payload String COMMENT 'signal JSON'
) ENGINE = MergeTree
PARTITION BY toYYYYMMDD(ts)
ORDER BY (bot_id, stream, ts, id)
TTL ts + INTERVAL 7 DAY
SETTINGS index_granularity = 8192;
