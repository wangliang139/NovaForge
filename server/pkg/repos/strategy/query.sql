-- name: CreateStrategy :one
-- -- timeout: 1s
INSERT INTO strategies (id, name, description, status)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetById :one
-- -- timeout: 1s
SELECT * FROM strategies
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetByName :one
-- -- timeout: 1s
SELECT * FROM strategies
WHERE name = $1 AND deleted_at IS NULL;

-- name: ListStrategies :many
-- -- timeout: 1s
SELECT * FROM strategies
WHERE (sqlc.narg('id')::VARCHAR IS NULL OR id = sqlc.narg('id'))
  AND (sqlc.narg('status')::strategy_status IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('name')::VARCHAR IS NULL OR name LIKE '%' || sqlc.narg('name') || '%')
  AND (sqlc.narg('created_at_start')::TIMESTAMPTZ IS NULL OR created_at >= sqlc.narg('created_at_start'))
  AND (sqlc.narg('created_at_end')::TIMESTAMPTZ IS NULL OR created_at <= sqlc.narg('created_at_end'))
  AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: CountStrategies :one
-- -- timeout: 1s
SELECT COUNT(*) FROM strategies WHERE deleted_at IS NULL;

-- name: UpdateStrategy :one
-- -- timeout: 1s
UPDATE strategies
SET description = COALESCE(sqlc.narg(description), description),
    name = COALESCE(sqlc.narg(name), name),
    status = COALESCE(sqlc.narg(status), status),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: DeleteStrategy :execrows
-- -- timeout: 1s
UPDATE strategies
SET deleted_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: LockStrategy :one
-- -- timeout: 1s
SELECT * FROM strategies
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateStrategyStatus :execrows
-- -- timeout: 1s
UPDATE strategies
SET status = sqlc.arg(status)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;