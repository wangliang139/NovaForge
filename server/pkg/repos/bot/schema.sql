create type run_mode as enum ('live', 'paper', 'backtest');

create type bot_status as enum ('running', 'stopped', 'error');

-- Bot实例表
CREATE TABLE IF NOT EXISTS bots (
    id serial PRIMARY KEY,
    strategy_id VARCHAR(64) NOT NULL,
    strategy_version VARCHAR(32) NOT NULL,
    account_id VARCHAR(64) NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    mode run_mode NOT NULL,
    name VARCHAR(128) NOT NULL,
    "desc" VARCHAR(512) NOT NULL default '',
    symbols VARCHAR(64)[] NOT NULL, -- symbols
    config JSONB, -- params/signals/config
    storage JSONB NOT NULL DEFAULT '{}', -- 策略变量存储
    status bot_status NOT NULL DEFAULT 'stopped',
    error_message TEXT NOT NULL default '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_bots_strategy_version ON bots(strategy_id, strategy_version);
CREATE UNIQUE INDEX idx_bots_account_id ON bots(account_id) where deleted_at is null;
CREATE INDEX idx_bots_mode ON bots(mode);
CREATE INDEX idx_bots_status ON bots(status);
