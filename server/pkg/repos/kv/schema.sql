create table public.kv (
    id bigserial primary key,
    key varchar not null,
    value varchar not null,
    created_at timestamp with time zone default now() not null,
    updated_at timestamp with time zone default now() not null
);

create unique index kv_key_uk on public.kv (key);
