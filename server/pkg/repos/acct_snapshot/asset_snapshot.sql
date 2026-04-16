-- 依赖 assets.wallet_type（由 sqlc schema 列表加载 assets/schema.sql）

CREATE TABLE IF NOT EXISTS asset_snapshot (
    id              BIGSERIAL PRIMARY KEY,
    account_id      VARCHAR(32)                           NOT NULL,
    exchange        VARCHAR(32)                           NOT NULL,
    wallet_type     wallet_type                           NOT NULL,
    asset           VARCHAR(16)                           NOT NULL,
    total           DECIMAL(32, 18)                       NOT NULL DEFAULT 0,
    effective_ts    TIMESTAMPTZ                           NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_asset_snapshot_lookup
    ON asset_snapshot (account_id, exchange, asset, wallet_type, effective_ts DESC);

CREATE INDEX IF NOT EXISTS idx_asset_snapshot_account_ts
    ON asset_snapshot (account_id, effective_ts DESC);
