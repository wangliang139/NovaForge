-- 账户权益快照表（每小时）
CREATE TABLE IF NOT EXISTS equity (
    id BIGSERIAL PRIMARY KEY,
    account_id VARCHAR(64) NOT NULL,
    ts TIMESTAMPTZ NOT NULL,
    notional DECIMAL(32, 18) NOT NULL DEFAULT 0,
    unrealized_profit DECIMAL(32, 18) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_equity_account_id_ts ON equity(account_id, ts);
CREATE INDEX idx_equity_ts ON equity(ts);
