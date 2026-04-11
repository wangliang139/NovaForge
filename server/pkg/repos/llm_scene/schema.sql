-- 表：LLM场景配置表
-- 用于定义不同的LLM使用场景及其默认配置
create table public.llm_scene (
    id bigserial primary key,
    key varchar(64) not null,           -- 场景键，用于API调用
    name varchar(128) not null,                -- 场景名称，用于展示
    description varchar(512) not null default '',    -- 场景描述
    config jsonb not null default '{}',       -- 模型配置
    messages jsonb not null default '{}', -- 提示词
    timeout int not null default 0, -- 超时时间（秒）
    response_format jsonb not null, -- 响应格式
    enabled boolean not null default true,          -- 是否启用
    deleted_at timestamp with time zone,            -- 逻辑删除时间
    created_at timestamp with time zone default now() not null,
    updated_at timestamp with time zone default now() not null
);

create unique index idx_llm_scene_key on public.llm_scene (key) WHERE deleted_at IS NULL;
create index idx_llm_scene_enabled on public.llm_scene (enabled);
create index idx_llm_scene_deleted_at on public.llm_scene (deleted_at);

