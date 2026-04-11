create type public.document_catalog as enum ('airdrop', 'api', 'cryptocurrency_listing', 'cryptocurrency_delisting', 'activity', 'news', 'flash_news', 'other');

create type public.document_format as enum ('markdown', 'txt', 'html');

create type public.document_status as enum ('draft', 'draft_failed', 'pending', 'pending_failed', 'active', 'archived', 'deduped', 'timeout');

create table public.document (
    id bigserial primary key,
    source varchar(64) not null,
    provider varchar(64) not null,
    catalog document_catalog not null,
    title varchar not null,
    content varchar not null,
    ai_title varchar not null default '',
    ai_summary varchar not null default '',
    ai_tags varchar[] not null default '{}',
    ai_coins varchar[] not null default '{}',
    ai_influence varchar not null default '',
    ai_influence_score int not null default 0,
    ai_sentiment int not null default 0,
    format document_format not null,
    authors varchar[] not null default '{}',
    lang varchar(16) not null default 'zh',
    url varchar not null,
    md5 varchar not null,
    published_at timestamp with time zone not null,
    status document_status default 'pending' not null,
    err_msg varchar(1024) not null default '',
    deduped_by bigint not null default 0,
    embedding vector(1536),
    created_at timestamp with time zone default now() not null,
    updated_at timestamp with time zone default now() not null
);

create unique index document_md5_uk on public.document (md5);

create index concurrently idx_document_bm25 on public.document using bm25 (
    id,
    source,
    provider,
    catalog,
    title,
    content,
    ai_title,
    ai_summary,
    ai_tags,
    ai_coins,
    ai_influence,
    ai_influence_score,
    ai_sentiment,
    format,
    authors,
    lang,
    url,
    md5,
    published_at,
    status,
    err_msg,
    deduped_by,
    created_at,
    updated_at
) with (key_field = 'id', text_fields = '{
        "title": {
            "tokenizer": {"type": "icu"}
        },
        "content": {
            "tokenizer": {"type": "icu"}
        },
        "ai_title": {
            "tokenizer": {"type": "icu"}
        },
        "ai_summary": {
            "tokenizer": {"type": "icu"}
        },
        "ai_influence": {
            "tokenizer": {"type": "icu"}
        }
    }');

CREATE INDEX idx_document_vector ON public.document USING hnsw (embedding vector_cosine_ops)
WITH (
    M = 32,               -- 每个节点最大连接数
    ef_construction = 256 -- 索引构建精度
);
