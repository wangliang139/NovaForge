-- Symbol 级权益快照表（按账户维度，每小时与 equity 同步）
CREATE TABLE IF NOT EXISTS symbol_equity (
    id BIGSERIAL PRIMARY KEY,
    account_id VARCHAR(64) NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    symbol VARCHAR(64) NOT NULL,
    net_value DECIMAL(32, 8) NOT NULL,
    base_currency VARCHAR(16) NOT NULL DEFAULT 'USDT',
    ts TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_symbol_equity_uk ON symbol_equity(account_id, exchange, symbol, ts);
CREATE INDEX idx_symbol_equity_account_ts ON symbol_equity(account_id, ts);
CREATE INDEX idx_symbol_equity_symbol ON symbol_equity(account_id, exchange, symbol);
