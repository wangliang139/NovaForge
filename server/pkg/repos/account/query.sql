-- name: Create :one
-- -- invalidate : [GetById, GetByName, GetDefaultAccounts]
-- -- timeout: 1s
INSERT INTO public.account (id, name, exchange, config, api_key, api_secret, passphrase, algorithm, tags, status, account_type, parent_account_id, multi_bot_mode)
VALUES ($1,
        $2,
        $3,
        $4, -- config
        $5,
        $6,
        $7,
        $8,
        $9,
        $10,
        $11,
        $12,
        $13)
returning *;

-- name: Update :one
-- -- invalidate : [GetById, GetByName, GetDefaultAccounts]
-- -- timeout: 1s
UPDATE public.account
SET name       = $2,
    api_key    = $3,
    api_secret = $4,
    passphrase = $5,
    algorithm  = $6,
    tags       = $7,
    status     = $8,
    exchange   = $9,
    account_type = $10,
    multi_bot_mode = $11,
    updated_at = now()
WHERE id = $1
  AND deleted_at IS NULL
returning *;

-- name: UpdateRiskConfig :one
-- -- invalidate : [GetById, GetByName, GetDefaultAccounts]
-- -- timeout: 1s
UPDATE public.account
SET config = jsonb_set(
      coalesce(config, '{}'::jsonb),
      '{risk}',
      coalesce(sqlc.arg('risk')::jsonb, '{}'::jsonb),
      true
    ),
    updated_at = now()
WHERE id = sqlc.arg('id')
  AND deleted_at IS NULL
RETURNING *;

-- name: GetById :one
-- -- cache: 60s
-- -- timeout: 5s
SELECT *
FROM public.account
WHERE id = $1
  AND deleted_at IS NULL;

-- name: GetByName :one
-- -- cache: 60s
-- -- timeout: 5s
SELECT *
FROM public.account
WHERE name = $1
  AND deleted_at IS NULL;

-- name: GetDefaultAccounts :many
-- -- cache: 60s
-- -- timeout: 1s
SELECT *
FROM public.account
WHERE status = 'online'
  AND exchange = $1
  AND tags @> array ['default']::varchar[]
  AND deleted_at IS NULL;

-- name: QueryAccountsCount :one
-- -- timeout: 5s
SELECT COUNT(1)
FROM public.account
WHERE id = coalesce(sqlc.narg('id')::varchar, id)
  AND exchange = coalesce(sqlc.narg('exchange')::exchange, exchange)
  AND (sqlc.narg('name')::text IS NULL OR name ILIKE concat('%', sqlc.narg('name')::text, '%'))
  AND status = coalesce(sqlc.narg('status')::account_status, status)
  AND (sqlc.narg('tags')::varchar[] is null or tags @> sqlc.narg('tags')::varchar[])
  AND deleted_at IS NULL
  AND created_at BETWEEN sqlc.arg('created_at_start')::timestamptz AND sqlc.arg('created_at_end')::timestamptz
  AND account_type = coalesce(sqlc.narg('account_type')::account_type, account_type);

-- name: QueryAccounts :many
-- -- timeout: 5s
SELECT *
FROM public.account
WHERE id = coalesce(sqlc.narg('id')::varchar, id)
  AND exchange = coalesce(sqlc.narg('exchange')::exchange, exchange)
  AND (sqlc.narg('name')::text IS NULL OR name ILIKE concat('%', sqlc.narg('name')::text, '%'))
  AND status = coalesce(sqlc.narg('status')::account_status, status)
  AND (sqlc.narg('tags')::varchar[] is null or tags @> sqlc.narg('tags')::varchar[])
  AND deleted_at IS NULL
  AND created_at BETWEEN sqlc.arg('created_at_start')::timestamptz AND sqlc.arg('created_at_end')::timestamptz
  AND account_type = coalesce(sqlc.narg('account_type')::account_type, account_type)
ORDER BY id DESC
OFFSET sqlc.arg('offset')::int8 LIMIT sqlc.arg('limit')::int8;

-- name: ListVirtualSubByParent :many
-- -- timeout: 5s
SELECT *
FROM public.account
WHERE parent_account_id = $1
  AND deleted_at IS NULL
ORDER BY created_at ASC;

-- name: ListVirtualSubByParentAsOf :many
-- -- timeout: 5s
-- 在 as_of 时点仍挂在父下的子账户（含 as_of 之后才软删的），用于 multi_bot 按订单/事件时点稳定分摊。
SELECT *
FROM public.account
WHERE parent_account_id = $1
  AND created_at <= $2
  AND (deleted_at IS NULL OR deleted_at > $2)
ORDER BY created_at ASC;

-- name: DeleteAccount :execrows
-- -- timeout: 5s
UPDATE public.account
SET deleted_at = now()
WHERE id = $1
  AND deleted_at IS NULL;

-- name: ListAccounts :many
-- -- timeout: 5s
SELECT *
FROM public.account
WHERE status = $1
  AND deleted_at IS NULL;