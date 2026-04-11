-- 表：LLM 完成任务日志
create type public.llm_completion_status as enum ('active', 'deleted', 'overridden');

create table if not exists public.llm_completion (
    id bigserial primary key,
    session_id bigint not null default 0,
    scene_key varchar(128) not null default '',
    scene_id bigint not null,
    prompt_id bigint not null,
    platform varchar(32) not null,
    provider varchar(32) not null,
    model varchar(128) not null,
    variables jsonb not null default '{}',
    messages jsonb not null default '{}',
    question text not null default '',
    answer text not null default '',
    error text not null default '',
    duration int not null default 0,
    tokens jsonb not null default '{}',
    status llm_completion_status not null default 'active',
    created_at timestamp with time zone default now() not null,
    updated_at timestamp with time zone default now() not null
);

create index if not exists idx_llm_completion_scene_id on public.llm_completion (scene_id);
create index if not exists idx_llm_completion_prompt_id on public.llm_completion (prompt_id);
create index if not exists idx_llm_completion_status on public.llm_completion (status);
create index if not exists idx_llm_completion_created_at on public.llm_completion (created_at);
