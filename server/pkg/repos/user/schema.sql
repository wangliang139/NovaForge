-- users 表：单用户 / 本地账户
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(255),
    name VARCHAR(255) NOT NULL,
    avatar VARCHAR(512),
    password_hash TEXT,
    access VARCHAR(50) DEFAULT 'user',  -- user, admin
    status VARCHAR(50) DEFAULT 'active', -- active, disabled
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users(username) WHERE username IS NOT NULL;
