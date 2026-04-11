create table if not exists public.llm_session (
    id bigint primary key,
    user_id bigint not null,
    title varchar(256) not null default '',
    summary text not null default '',
    last_dialog_id bigint not null default 0,
    dialog_count int not null default 0,
    turn_count int not null default 0,
    stats jsonb not null default '{}',
    last_dialog_at timestamp with time zone,
    status varchar(32) not null default 'idle',
    created_at timestamp with time zone default now() not null,
    updated_at timestamp with time zone default now() not null,
    deleted_at timestamp with time zone
);

create index if not exists idx_llm_session_user_id_created_at on public.llm_session (user_id, created_at desc);
create index if not exists idx_llm_session_user_id_updated_at on public.llm_session (user_id, updated_at desc);
create index if not exists idx_llm_session_status on public.llm_session (status);
