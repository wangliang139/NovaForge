-- name: Create :one
-- -- timeout: 1s
INSERT INTO public.llm_prompt (scene_id, scene_key, platform, name, model, providers, config, messages, timeout, weight, variants, enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
returning *;

-- name: Update :one
-- -- timeout: 1s
UPDATE public.llm_prompt
SET name = coalesce(sqlc.narg('name'), name),
    timeout = coalesce(sqlc.narg('timeout'), timeout),
    weight = coalesce(sqlc.narg('weight'), weight),
    variants = CASE 
        WHEN sqlc.narg('variants')::varchar[] IS NULL THEN variants
        ELSE sqlc.narg('variants')::varchar[]
    END,
    enabled = coalesce(sqlc.narg('enabled'), enabled),
    updated_at = now()
WHERE id = sqlc.arg('id') AND deleted_at IS NULL
returning *;

-- name: GetByID :one
-- -- timeout: 1s
SELECT * FROM public.llm_prompt
WHERE id = sqlc.arg('id') AND deleted_at IS NULL;

-- name: GetBySceneID :many
-- -- timeout: 2s
SELECT * FROM public.llm_prompt
WHERE scene_id = sqlc.arg('scene_id')
  AND enabled = coalesce(sqlc.narg('enabled')::boolean, enabled)
  AND deleted_at IS NULL
ORDER BY weight DESC, id;

-- name: GetBySceneKey :many
-- -- timeout: 2s
SELECT * FROM public.llm_prompt
WHERE scene_key = sqlc.arg('scene_key')
  AND deleted_at IS NULL
  AND enabled = coalesce(sqlc.narg('enabled')::boolean, enabled)
ORDER BY weight DESC, id;

-- name: Count :one
-- -- timeout: 1s
SELECT COUNT(1) FROM public.llm_prompt
WHERE deleted_at IS NULL
  AND scene_id = coalesce(sqlc.narg('scene_id'), scene_id)
  AND enabled = coalesce(sqlc.narg('enabled')::boolean, enabled);

-- name: List :many
-- -- timeout: 2s
SELECT * FROM public.llm_prompt
WHERE deleted_at IS NULL
  AND scene_id = coalesce(sqlc.narg('scene_id'), scene_id)
  AND enabled = coalesce(sqlc.narg('enabled')::boolean, enabled)
ORDER BY weight DESC, id
OFFSET sqlc.arg('offset')::int LIMIT sqlc.arg('limit')::int;

-- name: Delete :execrows
-- -- timeout: 1s
UPDATE public.llm_prompt
SET deleted_at = now(),
    updated_at = now()
WHERE id = sqlc.arg('id') AND deleted_at IS NULL;

-- name: DeleteBySceneID :execrows
-- -- timeout: 1s
UPDATE public.llm_prompt
SET deleted_at = now(),
    updated_at = now()
WHERE scene_id = sqlc.arg('scene_id') AND deleted_at IS NULL;

-- name: GetBySceneKeyAndVariant :many
-- -- timeout: 2s
SELECT * FROM public.llm_prompt
WHERE scene_key = sqlc.arg('scene_key')
  AND sqlc.arg('variant')::varchar = ANY(variants)
  AND deleted_at IS NULL
  AND enabled = coalesce(sqlc.narg('enabled')::boolean, enabled)
ORDER BY weight DESC, id;