create type position_side as enum ('LONG', 'SHORT');

-- 账户持仓快照表（账户维度）
CREATE TABLE IF NOT EXISTS positions (
    account_id VARCHAR(64) NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    symbol VARCHAR(64) NOT NULL,
    side position_side NOT NULL, -- LONG / SHORT
    qty DECIMAL(32, 8) NOT NULL DEFAULT 0, -- 持仓数量（有符号，正数为多仓，负数为空仓）
    entry_price DECIMAL(32, 8) NOT NULL DEFAULT 0, -- 开仓均价
    leverage INT NOT NULL DEFAULT 1, -- 杠杆倍数
    updated_ts TIMESTAMPTZ NOT NULL, -- 仓位更新时间
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, exchange, symbol, side)
);

CREATE INDEX idx_positions_account_id ON positions(account_id);
CREATE INDEX idx_positions_exchange ON positions(exchange);
CREATE INDEX idx_positions_symbol ON positions(symbol);
CREATE INDEX idx_positions_side ON positions(side);
CREATE INDEX idx_positions_updated_at ON positions(updated_at);
CREATE INDEX idx_positions_acct_ex_sym_side ON positions(account_id, exchange, symbol, side);
