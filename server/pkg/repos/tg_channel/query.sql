-- name: GetById :one
-- -- timeout: 1s
-- -- cache: 60s
SELECT * 
FROM public.tg_channel
WHERE id = sqlc.arg('id');

-- name: QueryList :many
-- -- timeout: 2s
SELECT * 
FROM public.tg_channel
WHERE 
    (sqlc.narg('id')::bigint IS NULL OR id = sqlc.narg('id')::bigint)
    AND (sqlc.narg('name')::varchar IS NULL OR name LIKE '%' || sqlc.narg('name')::varchar || '%')
    AND (sqlc.narg('source')::varchar IS NULL OR source = sqlc.narg('source')::varchar)
    AND (sqlc.narg('catalog')::document_catalog IS NULL OR catalog = sqlc.narg('catalog')::document_catalog)
    AND (sqlc.narg('enabled')::boolean IS NULL OR enabled = sqlc.narg('enabled')::boolean)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit')
OFFSET sqlc.arg('offset');

-- name: CountList :one
-- -- timeout: 2s
SELECT COUNT(*) 
FROM public.tg_channel
WHERE 
    (sqlc.narg('id')::bigint IS NULL OR id = sqlc.narg('id')::bigint)
    AND (sqlc.narg('name')::varchar IS NULL OR name LIKE '%' || sqlc.narg('name')::varchar || '%')
    AND (sqlc.narg('source')::varchar IS NULL OR source = sqlc.narg('source')::varchar)
    AND (sqlc.narg('catalog')::document_catalog IS NULL OR catalog = sqlc.narg('catalog')::document_catalog)
    AND (sqlc.narg('enabled')::boolean IS NULL OR enabled = sqlc.narg('enabled')::boolean);

-- name: Create :one
-- -- timeout: 2s
-- -- invalidate : [GetById]
INSERT INTO public.tg_channel (
    id,
    name,
    title,
    broadcast,
    source,
    catalog,
    extract_cfg,
    enabled,
    created_at,
    updated_at
) VALUES (
    sqlc.arg('id'),
    sqlc.arg('name'),
    sqlc.arg('title'),
    sqlc.arg('broadcast'),
    sqlc.arg('source'),
    sqlc.arg('catalog'),
    sqlc.arg('extract_cfg'),
    sqlc.arg('enabled'),
    now(),
    now()
)
RETURNING *;

-- name: Update :one
-- -- timeout: 2s
-- -- invalidate : [GetById]
UPDATE public.tg_channel
SET 
    name = coalesce(sqlc.narg('name'), name),
    title = coalesce(sqlc.narg('title'), title),
    broadcast = coalesce(sqlc.narg('broadcast'), broadcast),
    source = coalesce(sqlc.narg('source'), source),
    catalog = coalesce(sqlc.narg('catalog'), catalog),
    extract_cfg = coalesce(sqlc.narg('extract_cfg'), extract_cfg),
    enabled = coalesce(sqlc.narg('enabled'), enabled),
    updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING *;
