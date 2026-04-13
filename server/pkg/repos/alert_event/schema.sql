create table if not exists public.alert_event
(
    id             varchar(64) primary key,
    alert_id       varchar(64)                         not null references public.alert (id) on delete cascade,
    exchange       varchar(32)                         not null,
    symbol         varchar(64)                         not null,
    type           varchar(64)                         not null,
    frequency      varchar(16)                         not null,
    target_price   decimal(32, 12),
    alert_window   varchar(16),
    percent        decimal(32, 12),
    baseline_price decimal(32, 12),
    trigger_price  decimal(32, 12)                     not null,
    triggered_at   timestamp with time zone default now() not null,
    notify_result  varchar(16)                         not null,
    error_message  text,
    meta           jsonb
);

create index if not exists idx_alert_event_alert_time on public.alert_event (alert_id, triggered_at desc);
create index if not exists idx_alert_event_exchange_symbol_time on public.alert_event (exchange, symbol, triggered_at desc);
