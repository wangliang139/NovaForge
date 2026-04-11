-- name: Create :one
-- -- timeout: 1s
INSERT INTO public.llm_scene (key, name, description, config, messages, timeout, response_format, enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
returning *;

-- name: Update :one
-- -- timeout: 1s
UPDATE public.llm_scene
SET name = coalesce(sqlc.narg('name'), name),
    description = coalesce(sqlc.narg('description'), description),
    config = coalesce(sqlc.narg('config')::jsonb, config),
    messages = coalesce(sqlc.narg('messages')::jsonb, messages),
    timeout = coalesce(sqlc.narg('timeout'), timeout),
    response_format = coalesce(sqlc.narg('response_format')::jsonb, response_format),
    enabled = coalesce(sqlc.narg('enabled'), enabled),
    updated_at = now()
WHERE id = sqlc.arg('id') AND deleted_at IS NULL
returning *;

-- name: GetByID :one
-- -- timeout: 1s
SELECT * FROM public.llm_scene
WHERE id = sqlc.arg('id') AND deleted_at IS NULL;

-- name: GetByKey :one
-- -- timeout: 1s
SELECT * FROM public.llm_scene
WHERE key = sqlc.arg('key') AND deleted_at IS NULL;

-- name: List :many
-- -- timeout: 2s
SELECT * FROM public.llm_scene
WHERE deleted_at IS NULL
  AND enabled = coalesce(sqlc.narg('enabled')::boolean, enabled)
ORDER BY id DESC
OFFSET sqlc.arg('offset')::int LIMIT sqlc.arg('limit')::int;

-- name: Count :one
-- -- timeout: 1s
SELECT COUNT(1) FROM public.llm_scene
WHERE deleted_at IS NULL
  AND enabled = coalesce(sqlc.narg('enabled')::boolean, enabled);

-- name: Delete :execrows
-- -- timeout: 1s
UPDATE public.llm_scene
SET deleted_at = now(),
    updated_at = now()
WHERE id = sqlc.arg('id') AND deleted_at IS NULL;