-- name: CreateBot :one
-- -- timeout: 1s
INSERT INTO bots (
    strategy_id,
    strategy_version,
    account_id,
    exchange,
    mode,
    name,
    "desc",
    config,
    symbols,
    status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetBot :one
-- -- timeout: 1s
SELECT * FROM bots
WHERE id = $1
AND deleted_at is null;

-- name: ListBots :many
-- -- timeout: 1s
SELECT * FROM bots
WHERE (sqlc.narg('id')::INT IS NULL OR id = sqlc.narg('id'))
  AND (sqlc.narg('strategy_id')::VARCHAR IS NULL OR strategy_id = sqlc.narg('strategy_id'))
  AND (sqlc.narg('mode')::run_mode IS NULL OR mode = sqlc.narg('mode'))
  AND (sqlc.narg('status')::bot_status IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('exchange')::VARCHAR IS NULL OR exchange = sqlc.narg('exchange'))
  AND (sqlc.narg('account_id')::VARCHAR IS NULL OR account_id = sqlc.narg('account_id'))
  AND (sqlc.narg('name')::VARCHAR IS NULL OR name LIKE '%' || sqlc.narg('name') || '%')
  AND (sqlc.narg('created_at_start')::TIMESTAMPTZ IS NULL OR created_at >= sqlc.narg('created_at_start'))
  AND (sqlc.narg('created_at_end')::TIMESTAMPTZ IS NULL OR created_at <= sqlc.narg('created_at_end'))
  AND deleted_at is null
ORDER BY created_at DESC;

-- name: CountBots :one
-- -- timeout: 1s
SELECT COUNT(*) FROM bots
WHERE (sqlc.narg('id')::INT IS NULL OR id = sqlc.narg('id'))
  AND (sqlc.narg('strategy_id')::VARCHAR IS NULL OR strategy_id = sqlc.narg('strategy_id'))
  AND (sqlc.narg('mode')::run_mode IS NULL OR mode = sqlc.narg('mode'))
  AND (sqlc.narg('status')::bot_status IS NULL OR status = sqlc.narg('status'))
  AND (sqlc.narg('exchange')::VARCHAR IS NULL OR exchange = sqlc.narg('exchange'))
  AND (sqlc.narg('account_id')::VARCHAR IS NULL OR account_id = sqlc.narg('account_id'))
  AND (sqlc.narg('name')::VARCHAR IS NULL OR name LIKE '%' || sqlc.narg('name') || '%')
  AND (sqlc.narg('created_at_start')::TIMESTAMPTZ IS NULL OR created_at >= sqlc.narg('created_at_start'))
  AND (sqlc.narg('created_at_end')::TIMESTAMPTZ IS NULL OR created_at <= sqlc.narg('created_at_end'))
  AND deleted_at IS NULL;

-- name: CountActiveBotsForParentAccount :one
-- -- timeout: 1s
SELECT COUNT(*)::bigint
FROM bots b
WHERE b.deleted_at IS NULL
  AND (
    b.account_id = $1
    OR EXISTS (
      SELECT 1
      FROM public.account a
      WHERE a.id = b.account_id
        AND a.parent_account_id = $1
        AND a.deleted_at IS NULL
        AND a.account_type = 'virtual_sub'
    )
  );

-- name: UpdateBotStatus :one
-- -- timeout: 1s
UPDATE bots
SET status = $2,
    error_message = $3,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
AND deleted_at is null
RETURNING *;

-- name: UpdateBotStrategyVersion :one
-- -- timeout: 1s
UPDATE bots
SET strategy_version = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
AND status != 'running'
AND deleted_at is null
RETURNING *;

-- name: DeleteBot :one
-- -- timeout: 1s
UPDATE bots
SET deleted_at = CURRENT_TIMESTAMP
WHERE id = $1 AND status != 'running' AND deleted_at is null
RETURNING *;

-- name: GetBotByAccountID :one
-- -- timeout: 1s
SELECT * FROM bots
WHERE account_id = $1
AND deleted_at is null;

-- name: UpdateBotStorage :one
-- -- timeout: 1s
UPDATE bots
SET storage = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
AND deleted_at is null
RETURNING *;

-- name: UpdateBot :one
-- -- timeout: 1s
UPDATE bots
SET name = $2,
    "desc" = $3,
    symbols = $4,
    config = $5,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1
AND deleted_at is null
RETURNING *;