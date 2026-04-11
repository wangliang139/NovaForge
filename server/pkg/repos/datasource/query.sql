-- name: CreateDataSource :one
-- -- timeout: 1s
INSERT INTO datasources (name, description, type, exchange, symbol, props, start_ts, end_ts)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetById :one
-- -- timeout: 1s
SELECT * FROM datasources WHERE id = $1 AND deleted_at IS NULL;

-- name: ListDatasources :many
-- -- timeout: 5s
SELECT * FROM datasources
WHERE deleted_at IS NULL
  AND (sqlc.narg('type')::signal_type IS NULL OR type = sqlc.narg('type')::signal_type)
  AND (sqlc.narg('exchange')::varchar IS NULL OR exchange = sqlc.narg('exchange')::varchar)
  AND (sqlc.narg('symbol')::varchar IS NULL OR symbol = sqlc.narg('symbol')::varchar)
ORDER BY created_at DESC
OFFSET sqlc.arg('offset')::int8
LIMIT sqlc.arg('limit')::int8;

-- name: CountDatasources :one
-- -- timeout: 5s
SELECT COUNT(*) FROM datasources
WHERE deleted_at IS NULL
  AND (sqlc.narg(type)::signal_type IS NULL OR type = sqlc.narg(type)::signal_type)
  AND (sqlc.narg(exchange)::varchar IS NULL OR exchange = sqlc.narg(exchange)::varchar)
  AND (sqlc.narg(symbol)::varchar IS NULL OR symbol = sqlc.narg(symbol)::varchar);

-- name: UpdateDatasource :one
-- -- timeout: 1s
UPDATE datasources
SET 
  name = COALESCE($2, name),
  description = COALESCE($3, description),
  start_ts = COALESCE($4, start_ts),
  end_ts = COALESCE($5, end_ts),
  updated_at = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteDatasource :exec
-- -- timeout: 1s
UPDATE datasources
SET deleted_at = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted_at IS NULL;
