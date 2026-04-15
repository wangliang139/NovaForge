-- 账户资金流水表（账户维度，记录所有余额变化）
CREATE TABLE IF NOT EXISTS ledgers (
    id BIGSERIAL PRIMARY KEY,
    account_id VARCHAR(64) NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    asset VARCHAR(32) NOT NULL,
    wallet_type wallet_type NOT NULL,
    total DECIMAL(32, 18), -- 总量
    frozen DECIMAL(32, 18), -- 冻结余额
    total_delta DECIMAL(32, 18), -- 总量增量
    frozen_delta DECIMAL(32, 18), -- 冻结余额增量
    type VARCHAR(64) NOT NULL, -- 事件类型（规范化 code）
    detail JSONB, -- 事件详情（JSON）
    is_effective boolean NOT NULL DEFAULT TRUE, -- 是否生效
    ts TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_ledgers_account_id ON ledgers(account_id);
CREATE INDEX idx_ledgers_asset ON ledgers(asset);
CREATE INDEX idx_ledgers_ts ON ledgers(ts);
CREATE INDEX idx_ledgers_created_at ON ledgers(created_at);
