
-- =============================================================================
-- 以下为 ClickHouse 表，需在 ClickHouse 实例上执行，非 PostgreSQL
-- =============================================================================

CREATE DATABASE IF NOT EXISTS trade;

-- llt-data-api: eventflow (ClickHouse)
CREATE TABLE IF NOT EXISTS trade.event_flow (
    id Int64,
    account_id String,
    exchange LowCardinality(String),
    stream LowCardinality(String),
    topic String,
    event_kind LowCardinality(String),
    ts DateTime64(3),
    receive_at DateTime64(3),
    publish_at DateTime64(3),
    ingest_at DateTime64(3),
    payload String
) ENGINE = MergeTree
PARTITION BY toYYYYMMDD(ts)
ORDER BY (account_id, stream, ts, id)
TTL ts + INTERVAL 7 DAY
SETTINGS index_granularity = 8192;

-- llt-strategy-api: signalflow (ClickHouse)
CREATE TABLE IF NOT EXISTS trade.bot_signal_flow (
    id Int64,
    bot_id Int32,
    account_id String,
    exchange LowCardinality(String),
    stream LowCardinality(String),
    topic String,
    event_kind LowCardinality(String),
    ts DateTime64(3),
    inbound_at DateTime64(3),
    outbound_at DateTime64(3),
    receive_at DateTime64(3),
    ingest_at DateTime64(3),
    payload String
) ENGINE = MergeTree
PARTITION BY toYYYYMMDD(ts)
ORDER BY (bot_id, stream, ts, id)
TTL ts + INTERVAL 7 DAY
SETTINGS index_granularity = 8192;


CREATE TABLE IF NOT EXISTS trade.bot_console_log (
    id Int64,
    bot_id Int32,
    strategy_id String,
    level LowCardinality(String),
    message String,
    ts DateTime64(3),
    created_at DateTime64(3)
) ENGINE = MergeTree
PARTITION BY bot_id
ORDER BY (bot_id, ts, id);
