-- 策略快照表
CREATE TABLE IF NOT EXISTS snapshots (
    id serial PRIMARY KEY,
    strategy_id varchar(64) NOT NULL,
    parent_id int NOT NULL DEFAULT -1,
    version VARCHAR(64) NOT NULL,
    code TEXT NOT NULL,
    params JSONB NOT NULL, -- 策略参数定义
    signals JSONB NOT NULL, -- 策略运行所需 signals 等需求定义（版本级元数据）
    is_active boolean NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_snapshots_strategy_id ON snapshots(strategy_id);
CREATE INDEX idx_snapshots_created_at ON snapshots(created_at);
CREATE UNIQUE INDEX idx_snapshots_strategy_id_version ON snapshots(strategy_id, version) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_snapshots_strategy_id_active ON snapshots(strategy_id, is_active) WHERE deleted_at IS NULL AND is_active = TRUE;