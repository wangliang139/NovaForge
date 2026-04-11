create type public.calendar_source as enum ('gateio','jin10');

create type public.calendar_type as enum ('economic_data', 'project_event', 'token_unlock', 'summit_event', 'financing', 'events', 'other');

create table public.calendar (
    id bigserial primary key,
    date_id int not null,
    sid varchar not null,
    source calendar_source not null,
    type calendar_type not null,
    category varchar(64) not null,
    country varchar,
    project varchar,
    symbol varchar,
    title varchar not null,
    content varchar not null,
    importance int not null default 1,
    url varchar not null,
    ext jsonb,
    published_at timestamp with time zone not null,
    md5 varchar not null,
    created_at timestamp with time zone default now() not null,
    updated_at timestamp with time zone default now() not null
);

create unique index calendar_source_sid_uk on public.calendar (source, sid);
create unique index calendar_md5_uk on public.calendar (md5);