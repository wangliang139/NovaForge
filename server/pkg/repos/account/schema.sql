create type public.account_status as enum (
    'online',
    'offline'
    );

create type public.exchange as enum (
    'binance',
    'okx',
    'binance_test',
    'okx_test'
    );

create type public.algorithm as enum (
    'none',
    'hmac',
    'ed25519',
    'rsa'
    );

create type public.account_type as enum (
    'real',
    'virtual',
    'virtual_sub'
    );

create table public.account
(
    id         varchar(32) primary key,
    name       varchar(128)                           not null,
    exchange   public.exchange                        not null default 'binance',
    config     jsonb                                  not null default '{}',
    api_key    text                                   not null,
    api_secret text                                   not null,
    passphrase text                                   not null default '',
    algorithm  public.algorithm                       not null default 'hmac',
    tags       varchar(64)[],
    status     public.account_status                  not null,
    account_type public.account_type                  not null default 'real',
    parent_account_id varchar(32),
    multi_bot_mode boolean                            not null default false,
    deleted_at timestamp with time zone,            -- 逻辑删除时间
    created_at timestamp with time zone default now() not null,
    updated_at timestamp with time zone default now() not null
);

create unique index account_name_idx on public.account (name) WHERE deleted_at IS NULL;
create index idx_account_deleted_at on public.account (deleted_at);
create index idx_account_status on public.account (status);
create index idx_account_account_type on public.account (account_type);
create index idx_account_created_at on public.account (created_at);
create index idx_account_parent_account_id on public.account (parent_account_id) where deleted_at is null;