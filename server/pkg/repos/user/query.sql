-- name: GetFirstActiveUser :one
-- -- timeout: 1s
SELECT * FROM users
WHERE (status IS NULL OR status = 'active')
ORDER BY id ASC
LIMIT 1;

-- name: GetUserByID :one
-- -- timeout: 1s
SELECT * FROM users
WHERE id = $1
LIMIT 1;

-- name: CreateUser :one
-- -- timeout: 1s
INSERT INTO users (
    name,
    avatar,
    username,
    password_hash,
    access,
    status
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: UpdateUser :one
-- -- timeout: 1s
UPDATE users SET
    name = COALESCE(sqlc.narg(name), name),
    avatar = COALESCE(sqlc.narg(avatar), avatar),
    username = COALESCE(sqlc.narg(username), username),
    access = COALESCE(sqlc.narg(access), access),
    status = COALESCE(sqlc.narg(status), status),
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdatePasswordHash :one
-- -- timeout: 1s
UPDATE users SET
    password_hash = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListUsers :many
-- -- timeout: 1s
SELECT * FROM users
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;
