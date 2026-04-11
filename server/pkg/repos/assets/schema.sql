create type wallet_type as enum ('fund', 'trade', 'spot', 'future', 'margin');

-- 账户资产余额快照表（账户维度）
CREATE TABLE IF NOT EXISTS assets (
    account_id VARCHAR(64) NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    asset VARCHAR(16) NOT NULL,
    wallet_type wallet_type NOT NULL, -- 钱包类型
    total DECIMAL(32, 8) NOT NULL DEFAULT 0, -- 总量
    frozen DECIMAL(32, 8) NOT NULL DEFAULT 0, -- 冻结余额
    order_occupied DECIMAL(32, 8) NOT NULL DEFAULT 0, -- 订单占用余额
    avg_price DECIMAL(32, 8), -- 持仓均价，按 asset/USDT 计价；USDT 默认 1；变少时不修改
    last_updated_ts TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, asset, wallet_type)
);

CREATE INDEX idx_assets_account_id ON assets(account_id);
CREATE INDEX idx_assets_exchange ON assets(exchange);
CREATE INDEX idx_assets_asset ON assets(asset);
CREATE INDEX idx_assets_wallet_type ON assets(wallet_type);
CREATE INDEX idx_assets_acct_asset_wt ON assets(account_id, asset, wallet_type);
