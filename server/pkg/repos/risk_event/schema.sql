create table public.risk_event
(
    id          bigserial primary key,
    account_id  varchar(32)                         not null,
    exchange    varchar(32)                         not null,
    rule        varchar(64)                         not null,
    risk_index  decimal(32, 8),
    payload     jsonb,
    created_at  timestamp with time zone default now() not null
);

create index idx_risk_event_account_created_at on public.risk_event (account_id, created_at desc);

