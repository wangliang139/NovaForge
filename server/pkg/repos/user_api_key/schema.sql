-- API Key（用户级，供 MCP / 自动化客户端）
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

CREATE UNIQUE INDEX IF NOT EXISTS user_api_keys_key_lookup_active_idx
    ON user_api_keys (key_lookup) WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS user_api_keys_user_name_active_idx
    ON user_api_keys (user_id, name) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS user_api_keys_user_id_active_idx
    ON user_api_keys (user_id) WHERE deleted_at IS NULL;
