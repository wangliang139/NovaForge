-- =============================================================================
-- NovaForge - 统一数据库部署脚本 (PostgreSQL)
-- 合并自: server/pkg/repos/*/schema.sql
-- 使用: psql -f deploy/postgres.sql
-- 依赖: pgvector (document.embedding)；document 的 bm25 索引需 pg_bm25
-- 说明: 枚举用 duplicate_object 捕获实现幂等；表/索引使用 IF NOT EXISTS；可重复执行
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 扩展
-- -----------------------------------------------------------------------------
CREATE EXTENSION IF NOT EXISTS vector;

-- -----------------------------------------------------------------------------
-- 枚举类型（幂等：已存在则跳过）
-- -----------------------------------------------------------------------------
DO $$ BEGIN CREATE TYPE public.wallet_type AS ENUM ('fund', 'trade', 'spot', 'future', 'margin'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.signal_type AS ENUM ('kline', 'trade', 'depth', 'ticker', 'social', 'timer', 'order', 'position', 'balance', 'risk', 'system'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.strategy_status AS ENUM ('draft', 'active', 'inactive'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.run_mode AS ENUM ('live', 'paper', 'backtest'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.bot_status AS ENUM ('running', 'stopped', 'error'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.order_side AS ENUM ('LONG', 'SHORT'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.order_type AS ENUM ('MARKET', 'LIMIT', 'UNKNOWN'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.time_in_force AS ENUM ('GTC', 'IOC', 'FOK'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.algo_type AS ENUM ('NONE', 'CONDITIONAL', 'TRAILING', 'OCO', 'TWAP', 'ICEBERG', 'CHASE', 'UNKNOWN'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.order_source AS ENUM ('USER', 'STRATEGY', 'LIQUIDATION', 'ADL'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.position_side AS ENUM ('LONG', 'SHORT'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.account_status AS ENUM ('online', 'offline'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.exchange AS ENUM ('binance', 'okx', 'binance_test', 'okx_test'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.algorithm AS ENUM ('none', 'hmac', 'ed25519', 'rsa'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.account_type AS ENUM ('real', 'virtual', 'virtual_sub'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_enum e
    JOIN pg_type t ON t.oid = e.enumtypid
    JOIN pg_namespace n ON n.oid = t.typnamespace
    WHERE n.nspname = 'public' AND t.typname = 'account_type' AND e.enumlabel = 'virtual_sub'
  ) THEN
    ALTER TYPE public.account_type ADD VALUE 'virtual_sub';
  END IF;
END $$;
DO $$ BEGIN CREATE TYPE public.calendar_source AS ENUM ('gateio', 'jin10'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.calendar_type AS ENUM ('economic_data', 'project_event', 'token_unlock', 'summit_event', 'financing', 'events', 'other'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.document_catalog AS ENUM ('airdrop', 'api', 'cryptocurrency_listing', 'cryptocurrency_delisting', 'activity', 'news', 'flash_news', 'other'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.document_format AS ENUM ('markdown', 'txt', 'html'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.document_status AS ENUM ('draft', 'draft_failed', 'pending', 'pending_failed', 'active', 'archived', 'deduped', 'timeout'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE TYPE public.llm_completion_status AS ENUM ('active', 'deleted', 'overridden'); EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- -----------------------------------------------------------------------------
-- 表（按依赖大致排序；无外键声明，顺序仅便于阅读）
-- -----------------------------------------------------------------------------

-- user
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(255),
    name VARCHAR(255) NOT NULL,
    avatar VARCHAR(512),
    password_hash TEXT,
    access VARCHAR(50) DEFAULT 'user',
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- strategy
CREATE TABLE IF NOT EXISTS strategies (
    id varchar(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    status public.strategy_status NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

-- datasource
CREATE TABLE IF NOT EXISTS datasources (
    id serial PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    type public.signal_type NOT NULL,
    exchange varchar(32) NOT NULL,
    symbol varchar(64) NOT NULL,
    props jsonb NOT NULL,
    start_ts TIMESTAMPTZ NOT NULL,
    end_ts TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

-- account
CREATE TABLE IF NOT EXISTS public.account (
    id varchar(32) PRIMARY KEY,
    name varchar(128) NOT NULL,
    exchange public.exchange NOT NULL DEFAULT 'binance',
    config jsonb NOT NULL DEFAULT '{}',
    api_key text NOT NULL,
    api_secret text NOT NULL,
    passphrase text NOT NULL DEFAULT '',
    algorithm public.algorithm NOT NULL DEFAULT 'hmac',
    tags varchar(64)[],
    status public.account_status NOT NULL,
    account_type public.account_type NOT NULL DEFAULT 'real',
    parent_account_id varchar(32),
    multi_bot_mode boolean NOT NULL DEFAULT false,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE public.account ADD COLUMN IF NOT EXISTS parent_account_id varchar(32);
ALTER TABLE public.account ADD COLUMN IF NOT EXISTS multi_bot_mode boolean NOT NULL DEFAULT false;

-- 约束改由应用层校验；若旧环境曾创建过 DB CHECK，升级时删除
ALTER TABLE public.account DROP CONSTRAINT IF EXISTS account_parent_multi_chk;

-- assets
CREATE TABLE IF NOT EXISTS assets (
    account_id VARCHAR(64) NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    asset VARCHAR(16) NOT NULL,
    wallet_type public.wallet_type NOT NULL,
    total DECIMAL(32, 8) NOT NULL DEFAULT 0,
    frozen DECIMAL(32, 8) NOT NULL DEFAULT 0,
    order_occupied DECIMAL(32, 8) NOT NULL DEFAULT 0,
    avg_price DECIMAL(32, 8),
    last_updated_ts TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, asset, wallet_type)
);

-- positions
CREATE TABLE IF NOT EXISTS positions (
    account_id VARCHAR(64) NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    symbol VARCHAR(64) NOT NULL,
    side public.position_side NOT NULL,
    qty DECIMAL(32, 8) NOT NULL DEFAULT 0,
    entry_price DECIMAL(32, 8) NOT NULL DEFAULT 0,
    leverage INT NOT NULL DEFAULT 1,
    updated_ts TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, exchange, symbol, side)
);

-- equity
CREATE TABLE IF NOT EXISTS equity (
    id BIGSERIAL PRIMARY KEY,
    account_id VARCHAR(64) NOT NULL,
    ts TIMESTAMPTZ NOT NULL,
    notional DECIMAL(32, 8) NOT NULL DEFAULT 0,
    unrealized_profit DECIMAL(32, 8) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- symbol_equity
CREATE TABLE IF NOT EXISTS symbol_equity (
    id BIGSERIAL PRIMARY KEY,
    account_id VARCHAR(64) NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    symbol VARCHAR(64) NOT NULL,
    net_value DECIMAL(32, 8) NOT NULL,
    base_currency VARCHAR(16) NOT NULL DEFAULT 'USDT',
    ts TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ledgers
CREATE TABLE IF NOT EXISTS ledgers (
    id BIGSERIAL PRIMARY KEY,
    account_id VARCHAR(64) NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    asset VARCHAR(32) NOT NULL,
    wallet_type public.wallet_type NOT NULL,
    total DECIMAL(32, 8),
    frozen DECIMAL(32, 8),
    total_delta DECIMAL(32, 8),
    frozen_delta DECIMAL(32, 8),
    type VARCHAR(64) NOT NULL,
    detail JSONB,
    is_effective boolean NOT NULL DEFAULT TRUE,
    ts TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- snapshot
CREATE TABLE IF NOT EXISTS snapshots (
    id serial PRIMARY KEY,
    strategy_id varchar(64) NOT NULL,
    parent_id int NOT NULL DEFAULT -1,
    version VARCHAR(64) NOT NULL,
    code TEXT NOT NULL,
    params JSONB NOT NULL,
    signals JSONB NOT NULL,
    is_active boolean NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

-- bot
CREATE TABLE IF NOT EXISTS bots (
    id serial PRIMARY KEY,
    strategy_id VARCHAR(64) NOT NULL,
    strategy_version VARCHAR(32) NOT NULL,
    account_id VARCHAR(64) NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    mode public.run_mode NOT NULL,
    name VARCHAR(128) NOT NULL,
    "desc" VARCHAR(512) NOT NULL DEFAULT '',
    symbols VARCHAR(64)[] NOT NULL,
    config JSONB,
    storage JSONB NOT NULL DEFAULT '{}',
    status public.bot_status NOT NULL DEFAULT 'stopped',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

-- orders
CREATE TABLE IF NOT EXISTS orders (
    id BIGSERIAL PRIMARY KEY,
    bot_id int NOT NULL,
    account_id VARCHAR(64) NOT NULL,
    order_id VARCHAR(128) NOT NULL,
    client_order_id VARCHAR(128) NOT NULL,
    drived_order_id VARCHAR(128) NOT NULL,
    order_type public.order_type NOT NULL,
    algo_type public.algo_type NOT NULL,
    source public.order_source NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    symbol VARCHAR(64) NOT NULL,
    side public.order_side NOT NULL,
    is_buy BOOLEAN NOT NULL,
    price DECIMAL(32, 8),
    quantity DECIMAL(32, 8),
    executed_qty DECIMAL(32, 8),
    executed_price DECIMAL(32, 8),
    avg_price DECIMAL(32, 8),
    reduce_only BOOLEAN NOT NULL,
    post_only BOOLEAN NOT NULL,
    tif public.time_in_force NOT NULL,
    conditions JSONB,
    detail JSONB,
    status varchar(32) NOT NULL,
    reject_reason VARCHAR(128),
    created_ts TIMESTAMPTZ NOT NULL,
    working_ts TIMESTAMPTZ,
    finished_ts TIMESTAMPTZ,
    updated_ts TIMESTAMPTZ NOT NULL,
    locked DECIMAL(32, 8),
    locked_asset VARCHAR(32),
    fee DECIMAL(32, 8),
    fee_asset VARCHAR(32),
    realized_pnl DECIMAL(32, 8),
    pnl_asset VARCHAR(16),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- backtest
CREATE TABLE IF NOT EXISTS backtest_results (
    id VARCHAR(64) PRIMARY KEY,
    job_id VARCHAR(64) NOT NULL,
    strategy_id VARCHAR(64) NOT NULL,
    strategy_version VARCHAR(32) NOT NULL,
    exchange VARCHAR(32) NOT NULL,
    symbol VARCHAR(64) NOT NULL,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    initial_balance DECIMAL(32, 8) NOT NULL,
    final_balance DECIMAL(32, 8) NOT NULL,
    total_pnl DECIMAL(32, 8) NOT NULL,
    total_trades INT NOT NULL DEFAULT 0,
    win_trades INT NOT NULL DEFAULT 0,
    loss_trades INT NOT NULL DEFAULT 0,
    win_rate DECIMAL(5, 2),
    sharpe_ratio DECIMAL(10, 4),
    max_drawdown DECIMAL(10, 4),
    result_data JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ds_items
CREATE TABLE IF NOT EXISTS ds_items (
    id bigserial PRIMARY KEY,
    ds_id int NOT NULL,
    data jsonb NOT NULL,
    ts TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- document
CREATE TABLE IF NOT EXISTS public.document (
    id bigserial PRIMARY KEY,
    source varchar(64) NOT NULL,
    provider varchar(64) NOT NULL,
    catalog public.document_catalog NOT NULL,
    title varchar NOT NULL,
    content varchar NOT NULL,
    ai_title varchar NOT NULL DEFAULT '',
    ai_summary varchar NOT NULL DEFAULT '',
    ai_tags varchar[] NOT NULL DEFAULT '{}',
    ai_coins varchar[] NOT NULL DEFAULT '{}',
    ai_influence varchar NOT NULL DEFAULT '',
    ai_influence_score int NOT NULL DEFAULT 0,
    ai_sentiment int NOT NULL DEFAULT 0,
    format public.document_format NOT NULL,
    authors varchar[] NOT NULL DEFAULT '{}',
    lang varchar(16) NOT NULL DEFAULT 'zh',
    url varchar NOT NULL,
    md5 varchar NOT NULL,
    published_at timestamp with time zone NOT NULL,
    status public.document_status DEFAULT 'pending' NOT NULL,
    err_msg varchar(1024) NOT NULL DEFAULT '',
    deduped_by bigint NOT NULL DEFAULT 0,
    embedding vector(1536),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

-- calendar
CREATE TABLE IF NOT EXISTS public.calendar (
    id bigserial PRIMARY KEY,
    date_id int NOT NULL,
    sid varchar NOT NULL,
    source public.calendar_source NOT NULL,
    type public.calendar_type NOT NULL,
    category varchar(64) NOT NULL,
    country varchar,
    project varchar,
    symbol varchar,
    title varchar NOT NULL,
    content varchar NOT NULL,
    importance int NOT NULL DEFAULT 1,
    url varchar NOT NULL,
    ext jsonb,
    published_at timestamp with time zone NOT NULL,
    md5 varchar NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

-- kv
CREATE TABLE IF NOT EXISTS public.kv (
    id bigserial PRIMARY KEY,
    key varchar NOT NULL,
    value varchar NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

-- llm_scene
CREATE TABLE IF NOT EXISTS public.llm_scene (
    id bigserial PRIMARY KEY,
    key varchar(64) NOT NULL,
    name varchar(128) NOT NULL,
    description varchar(512) NOT NULL DEFAULT '',
    config jsonb NOT NULL DEFAULT '{}',
    messages jsonb NOT NULL DEFAULT '{}',
    timeout int NOT NULL DEFAULT 0,
    response_format jsonb NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

-- llm_prompt
CREATE TABLE IF NOT EXISTS public.llm_prompt (
    id bigserial PRIMARY KEY,
    scene_key varchar(64) NOT NULL,
    scene_id bigint NOT NULL,
    platform varchar(32) NOT NULL,
    name varchar(128) NOT NULL,
    model varchar(128) NOT NULL,
    providers varchar[] NOT NULL DEFAULT '{}',
    config jsonb NOT NULL DEFAULT '{}',
    messages jsonb NOT NULL DEFAULT '{}',
    timeout int NOT NULL DEFAULT 0,
    weight int NOT NULL DEFAULT 100,
    variants varchar[] NOT NULL DEFAULT '{}',
    enabled boolean NOT NULL DEFAULT true,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

-- llm_session
CREATE TABLE IF NOT EXISTS public.llm_session (
    id bigint PRIMARY KEY,
    user_id bigint NOT NULL,
    title varchar(256) NOT NULL DEFAULT '',
    summary text NOT NULL DEFAULT '',
    last_dialog_id bigint NOT NULL DEFAULT 0,
    dialog_count int NOT NULL DEFAULT 0,
    turn_count int NOT NULL DEFAULT 0,
    stats jsonb NOT NULL DEFAULT '{}',
    last_dialog_at timestamp with time zone,
    status varchar(32) NOT NULL DEFAULT 'idle',
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

-- llm_dialog
CREATE TABLE IF NOT EXISTS public.llm_dialog (
    id bigint PRIMARY KEY,
    session_id bigint NOT NULL,
    dialog_id bigint NOT NULL,
    seq int NOT NULL,
    role varchar(16) NOT NULL,
    status varchar(32) NOT NULL DEFAULT 'pending',
    content_text text NOT NULL DEFAULT '',
    parts jsonb NOT NULL DEFAULT '[]',
    context_meta jsonb NOT NULL DEFAULT '{}',
    stats jsonb NOT NULL DEFAULT '{}',
    provider varchar(64) NOT NULL DEFAULT '',
    model varchar(128) NOT NULL DEFAULT '',
    can_regenerate boolean NOT NULL DEFAULT false,
    error_code varchar(64) NOT NULL DEFAULT '',
    error_message text NOT NULL DEFAULT '',
    visible boolean NOT NULL DEFAULT true,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone
);

-- llm_completion
CREATE TABLE IF NOT EXISTS public.llm_completion (
    id bigserial PRIMARY KEY,
    session_id bigint NOT NULL DEFAULT 0,
    scene_key varchar(128) NOT NULL DEFAULT '',
    scene_id bigint NOT NULL,
    prompt_id bigint NOT NULL,
    platform varchar(32) NOT NULL,
    provider varchar(32) NOT NULL,
    model varchar(128) NOT NULL,
    variables jsonb NOT NULL DEFAULT '{}',
    messages jsonb NOT NULL DEFAULT '{}',
    question text NOT NULL DEFAULT '',
    answer text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT '',
    duration int NOT NULL DEFAULT 0,
    tokens jsonb NOT NULL DEFAULT '{}',
    status public.llm_completion_status NOT NULL DEFAULT 'active',
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

-- tg_channel (依赖 document_catalog)
CREATE TABLE IF NOT EXISTS public.tg_channel (
    id bigint PRIMARY KEY,
    name varchar(255) NOT NULL DEFAULT '',
    title varchar(512) NOT NULL DEFAULT '',
    broadcast boolean NOT NULL DEFAULT false,
    source varchar(64) NOT NULL,
    catalog public.document_catalog NOT NULL,
    extract_cfg jsonb NOT NULL DEFAULT '{}',
    enabled boolean NOT NULL DEFAULT false,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

-- user_api_key
CREATE TABLE IF NOT EXISTS user_api_keys (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    key_lookup VARCHAR(32) NOT NULL,
    secret_hash TEXT NOT NULL,
    key_prefix VARCHAR(64) NOT NULL,
    permissions TEXT[] NOT NULL DEFAULT ARRAY['query']::TEXT[],
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- risk_event
CREATE TABLE IF NOT EXISTS public.risk_event (
    id bigserial PRIMARY KEY,
    account_id varchar(32) NOT NULL,
    exchange varchar(32) NOT NULL,
    rule varchar(64) NOT NULL,
    risk_index decimal(32, 8),
    payload jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

-- -----------------------------------------------------------------------------
-- 索引（IF NOT EXISTS / CONCURRENTLY IF NOT EXISTS）
-- -----------------------------------------------------------------------------
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users(username) WHERE username IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_strategies_name ON strategies(name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_strategies_created_at ON strategies(created_at);

CREATE INDEX IF NOT EXISTS idx_datasources_exchange_symbol ON datasources(exchange, symbol);
CREATE INDEX IF NOT EXISTS idx_datasource_type ON datasources(type);

CREATE UNIQUE INDEX IF NOT EXISTS account_name_idx ON public.account (name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_account_deleted_at ON public.account (deleted_at);
CREATE INDEX IF NOT EXISTS idx_account_status ON public.account (status);
CREATE INDEX IF NOT EXISTS idx_account_account_type ON public.account (account_type);
CREATE INDEX IF NOT EXISTS idx_account_created_at ON public.account (created_at);
CREATE INDEX IF NOT EXISTS idx_account_parent_account_id ON public.account (parent_account_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_assets_account_id ON assets(account_id);
CREATE INDEX IF NOT EXISTS idx_assets_exchange ON assets(exchange);
CREATE INDEX IF NOT EXISTS idx_assets_asset ON assets(asset);
CREATE INDEX IF NOT EXISTS idx_assets_wallet_type ON assets(wallet_type);
CREATE INDEX IF NOT EXISTS idx_assets_acct_asset_wt ON assets(account_id, asset, wallet_type);

CREATE INDEX IF NOT EXISTS idx_positions_account_id ON positions(account_id);
CREATE INDEX IF NOT EXISTS idx_positions_exchange ON positions(exchange);
CREATE INDEX IF NOT EXISTS idx_positions_symbol ON positions(symbol);
CREATE INDEX IF NOT EXISTS idx_positions_side ON positions(side);
CREATE INDEX IF NOT EXISTS idx_positions_updated_at ON positions(updated_at);
CREATE INDEX IF NOT EXISTS idx_positions_acct_ex_sym_side ON positions(account_id, exchange, symbol, side);

CREATE INDEX IF NOT EXISTS idx_equity_account_id_ts ON equity(account_id, ts);
CREATE INDEX IF NOT EXISTS idx_equity_ts ON equity(ts);

CREATE UNIQUE INDEX IF NOT EXISTS idx_symbol_equity_uk ON symbol_equity(account_id, exchange, symbol, ts);
CREATE INDEX IF NOT EXISTS idx_symbol_equity_account_ts ON symbol_equity(account_id, ts);
CREATE INDEX IF NOT EXISTS idx_symbol_equity_symbol ON symbol_equity(account_id, exchange, symbol);

CREATE INDEX IF NOT EXISTS idx_ledgers_account_id ON ledgers(account_id);
CREATE INDEX IF NOT EXISTS idx_ledgers_asset ON ledgers(asset);
CREATE INDEX IF NOT EXISTS idx_ledgers_ts ON ledgers(ts);
CREATE INDEX IF NOT EXISTS idx_ledgers_created_at ON ledgers(created_at);

CREATE INDEX IF NOT EXISTS idx_snapshots_strategy_id ON snapshots(strategy_id);
CREATE INDEX IF NOT EXISTS idx_snapshots_created_at ON snapshots(created_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_snapshots_strategy_id_version ON snapshots(strategy_id, version) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_snapshots_strategy_id_active ON snapshots(strategy_id, is_active) WHERE deleted_at IS NULL AND is_active = TRUE;

CREATE INDEX IF NOT EXISTS idx_bots_strategy_version ON bots(strategy_id, strategy_version);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bots_account_id ON bots(account_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_bots_mode ON bots(mode);
CREATE INDEX IF NOT EXISTS idx_bots_status ON bots(status);

CREATE INDEX IF NOT EXISTS idx_orders_bot_id ON orders(bot_id);
CREATE INDEX IF NOT EXISTS idx_orders_account_id ON orders(account_id);
CREATE INDEX IF NOT EXISTS idx_orders_symbol ON orders(symbol);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
CREATE INDEX IF NOT EXISTS idx_orders_created_at ON orders(created_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_account_cl_order_id ON orders(account_id, client_order_id) WHERE client_order_id <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_account_order_id ON orders(account_id, order_id);

CREATE INDEX IF NOT EXISTS idx_backtest_results_job_id ON backtest_results(job_id);
CREATE INDEX IF NOT EXISTS idx_backtest_results_strategy ON backtest_results(strategy_id, strategy_version);
CREATE INDEX IF NOT EXISTS idx_backtest_results_created_at ON backtest_results(created_at);

CREATE INDEX IF NOT EXISTS idx_ds_items_ds_id ON ds_items(ds_id);
CREATE INDEX IF NOT EXISTS idx_ds_items_ts ON ds_items(ts);

CREATE UNIQUE INDEX IF NOT EXISTS document_md5_uk ON public.document (md5);
-- 需安装 pg_bm25；若在无扩展环境部署可暂时注释本段（不用 CONCURRENTLY 以便整文件可放在单事务中执行）
CREATE INDEX IF NOT EXISTS idx_document_bm25 ON public.document USING bm25 (
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
) WITH (key_field = 'id', text_fields = '{
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
CREATE INDEX IF NOT EXISTS idx_document_vector ON public.document USING hnsw (embedding vector_cosine_ops)
WITH (M = 32, ef_construction = 256);

CREATE UNIQUE INDEX IF NOT EXISTS calendar_source_sid_uk ON public.calendar (source, sid);
CREATE UNIQUE INDEX IF NOT EXISTS calendar_md5_uk ON public.calendar (md5);

CREATE UNIQUE INDEX IF NOT EXISTS kv_key_uk ON public.kv (key);

CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_scene_key ON public.llm_scene (key) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_llm_scene_enabled ON public.llm_scene (enabled);
CREATE INDEX IF NOT EXISTS idx_llm_scene_deleted_at ON public.llm_scene (deleted_at);

CREATE INDEX IF NOT EXISTS idx_llm_prompt_scene_key ON public.llm_prompt (scene_key);
CREATE INDEX IF NOT EXISTS idx_llm_prompt_enabled ON public.llm_prompt (enabled);
CREATE INDEX IF NOT EXISTS idx_llm_prompt_deleted_at ON public.llm_prompt (deleted_at);
CREATE INDEX IF NOT EXISTS idx_llm_prompt_variants ON public.llm_prompt USING gin (variants);

CREATE INDEX IF NOT EXISTS idx_llm_session_user_id_created_at ON public.llm_session (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_session_user_id_updated_at ON public.llm_session (user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_session_status ON public.llm_session (status);

CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_dialog_session_id_seq_unique ON public.llm_dialog (session_id, seq);
CREATE INDEX IF NOT EXISTS idx_llm_dialog_session_id_dialog_id ON public.llm_dialog (session_id, dialog_id);
CREATE INDEX IF NOT EXISTS idx_llm_dialog_session_id_created_at ON public.llm_dialog (session_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_dialog_role_status ON public.llm_dialog (role, status);

CREATE INDEX IF NOT EXISTS idx_llm_completion_scene_id ON public.llm_completion (scene_id);
CREATE INDEX IF NOT EXISTS idx_llm_completion_prompt_id ON public.llm_completion (prompt_id);
CREATE INDEX IF NOT EXISTS idx_llm_completion_status ON public.llm_completion (status);
CREATE INDEX IF NOT EXISTS idx_llm_completion_created_at ON public.llm_completion (created_at);

CREATE UNIQUE INDEX IF NOT EXISTS tg_channel_id_uk ON public.tg_channel (id);

CREATE UNIQUE INDEX IF NOT EXISTS user_api_keys_key_lookup_active_idx ON user_api_keys (key_lookup) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS user_api_keys_user_name_active_idx ON user_api_keys (user_id, name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS user_api_keys_user_id_active_idx ON user_api_keys (user_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_risk_event_account_created_at ON public.risk_event (account_id, created_at DESC);
