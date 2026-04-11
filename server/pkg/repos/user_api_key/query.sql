-- name: GetUserApiKeyByLookup :one
-- -- timeout: 2s
SELECT * FROM user_api_keys
WHERE key_lookup = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: ListUserApiKeysByUserID :many
-- -- timeout: 2s
SELECT * FROM user_api_keys
WHERE user_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: CountActiveUserApiKeysByUserIDAndName :one
-- -- timeout: 2s
SELECT COUNT(*)::bigint AS count FROM user_api_keys
WHERE user_id = $1 AND name = $2 AND deleted_at IS NULL;

-- name: CountActiveUserApiKeysByUserID :one
-- -- timeout: 2s
SELECT COUNT(*)::bigint AS count FROM user_api_keys
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: CreateUserApiKey :one
-- -- timeout: 2s
INSERT INTO user_api_keys (
    user_id,
    name,
    key_lookup,
    secret_hash,
    key_prefix,
    permissions
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: SoftDeleteUserApiKey :execrows
-- -- timeout: 2s
UPDATE user_api_keys
SET deleted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL;
