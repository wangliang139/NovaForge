-- 依赖 positions.position_side（由 sqlc schema 列表加载 positions/schema.sql）

CREATE TABLE IF NOT EXISTS account_position_snapshot (
    id           BIGSERIAL PRIMARY KEY,
    account_id   VARCHAR(32)                           NOT NULL,
    exchange     VARCHAR(32)                           NOT NULL,
    symbol       VARCHAR(64)                           NOT NULL,
    side         position_side                         NOT NULL,
    qty          DECIMAL(32, 8)                        NOT NULL DEFAULT 0,
    entry_price  DECIMAL(32, 8)                        NOT NULL DEFAULT 0,
    leverage     INT                                   NOT NULL DEFAULT 1,
    effective_ts TIMESTAMPTZ                           NOT NULL,
    created_at   TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_account_position_snapshot_lookup
    ON account_position_snapshot (account_id, exchange, symbol, side, effective_ts DESC);

CREATE INDEX IF NOT EXISTS idx_account_position_snapshot_account_ts
    ON account_position_snapshot (account_id, effective_ts DESC);
