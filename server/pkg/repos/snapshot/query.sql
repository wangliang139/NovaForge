-- name: CreateSnapshot :one
-- -- timeout: 1s
INSERT INTO snapshots (strategy_id, parent_id, version, code, params, signals, is_active, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetById :one
-- -- timeout: 1s
SELECT * FROM snapshots
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateParams :one
-- -- timeout: 1s
UPDATE snapshots
SET params = $1, 
    updated_at = CURRENT_TIMESTAMP
WHERE id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: GetByStrategyIdAndVersion :one
-- -- timeout: 1s
SELECT * FROM snapshots
WHERE strategy_id = $1 AND version = $2 AND deleted_at IS NULL;

-- name: InactivateSnapshot :execrows
-- -- timeout: 1s
UPDATE snapshots
SET is_active = FALSE
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetActicedSnapshot :one
-- -- timeout: 1s
SELECT * FROM snapshots
WHERE strategy_id = $1 AND is_active = TRUE AND deleted_at IS NULL
LIMIT 1;

-- name: GetLatestSnapshot :one
-- -- timeout: 1s
SELECT * FROM snapshots
WHERE strategy_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: DeleteById :execrows
-- -- timeout: 1s
UPDATE snapshots
SET deleted_at = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted_at IS NULL;

-- name: DeleteByStrategyId :execrows
-- -- timeout: 1s
UPDATE snapshots
SET deleted_at = CURRENT_TIMESTAMP
WHERE strategy_id = $1 AND deleted_at IS NULL;