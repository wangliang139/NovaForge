create type signal_type as enum ('kline', 'trade', 'depth', 'ticker', 'social', 'timer', 'order', 'position', 'balance', 'risk', 'system');

CREATE TABLE IF NOT EXISTS datasources (
    id serial PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    type signal_type NOT NULL,
    exchange varchar(32) NOT NULL,
    symbol varchar(64) NOT NULL,
    props jsonb NOT NULL,
    start_ts TIMESTAMPTZ NOT NULL,
    end_ts TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

create index idx_datasources_exchange_symbol on datasources(exchange, symbol);
create index idx_datasource_type on datasources(type);
