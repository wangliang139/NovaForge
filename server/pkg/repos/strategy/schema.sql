create type strategy_status as enum ('draft', 'active', 'inactive');

-- 策略表
CREATE TABLE IF NOT EXISTS strategies (
    id varchar(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    status strategy_status NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_strategies_name ON strategies(name) WHERE deleted_at IS NULL;
CREATE INDEX idx_strategies_created_at ON strategies(created_at);
