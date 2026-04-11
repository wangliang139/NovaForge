create table public.tg_channel (
    id bigint primary key,
    name varchar(255) not null default '',
    title varchar(512) not null default '',
    broadcast boolean not null default false,
    source varchar(64) not null,
    catalog document_catalog not null,
    extract_cfg jsonb not null default '{}',
    enabled boolean not null default false,
    created_at timestamp with time zone default now() not null,
    updated_at timestamp with time zone default now() not null
);

create unique index tg_channel_id_uk on public.tg_channel (id);

