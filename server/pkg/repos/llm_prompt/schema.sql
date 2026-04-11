-- 表：LLM模型配置表
-- 用于定义每个场景下的具体模型配置，支持AB测试分流
create table public.llm_prompt (
    id bigserial primary key,
    scene_key varchar(64) not null,                 -- 场景键
    scene_id bigint not null,                       -- 场景ID
    platform varchar(32) not null,                 -- 平台（如openai, anthropic, google等）
    name varchar(128) not null,               
    model varchar(128) not null,               -- 模型（如gpt-4, claude-3等）
    providers varchar[] not null default '{}', -- 提供者（如openai, anthropic, google等）
    config jsonb not null default '{}',       -- 模型配置
    messages jsonb not null default '{}', -- 提示词
    timeout int not null default 0, -- 超时时间（秒）
    weight int not null default 100,   -- 权重（0-100），用于AB测试
    variants varchar[] not null default '{}', -- 词条（variants），用于通过 scene_key:variant 精确定位
    enabled boolean not null default true, -- 是否启用
    deleted_at timestamp with time zone,            -- 逻辑删除时间
    created_at timestamp with time zone default now() not null,
    updated_at timestamp with time zone default now() not null
);

create index idx_llm_prompt_scene_key on public.llm_prompt (scene_key);
create index idx_llm_prompt_enabled on public.llm_prompt (enabled);
create index idx_llm_prompt_deleted_at on public.llm_prompt (deleted_at);
create index idx_llm_prompt_variants on public.llm_prompt using gin (variants);

