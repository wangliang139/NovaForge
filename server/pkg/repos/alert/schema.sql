create type public.alert_frequency_t as enum ('repeat', 'once');
create type public.alert_status_t as enum ('active', 'error');
create type public.alert_window_t as enum ('5m', '1h', '4h', '24h');

create table if not exists public.alert
(
    id                varchar(64) primary key,
    exchange          varchar(32)                         not null,
    symbol            varchar(64)                         not null,
    type              varchar(64)                         not null,
    frequency         public.alert_frequency_t            not null,
    price             decimal(32, 12),
    alert_window      public.alert_window_t,
    percent           decimal(32, 12),
    remark            text,
    cooldown_seconds  integer                  default 60 not null,
    status            public.alert_status_t    default 'active' not null,
    last_triggered_at timestamp with time zone,
    trigger_count     bigint                   default 0 not null,
    created_at        timestamp with time zone default now() not null,
    updated_at        timestamp with time zone default now() not null,
    deleted_at        timestamp with time zone
);

alter table public.alert
    add column if not exists deleted_at timestamp with time zone;

drop index if exists uq_alert_unique_rule;
create unique index if not exists uq_alert_unique_rule on public.alert (
    exchange,
    symbol,
    type,
    frequency,
    price,
    alert_window,
    percent
) nulls not distinct
where deleted_at is null;

drop index if exists idx_alert_exchange_symbol;
create index if not exists idx_alert_exchange_symbol on public.alert (exchange, symbol)
where deleted_at is null;

drop index if exists idx_alert_status;
create index if not exists idx_alert_status on public.alert (status)
where deleted_at is null;

