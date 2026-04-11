-- name: Upsert :one
-- -- invalidate: [GetByKey]
-- -- timeout: 1s
INSERT INTO public.kv (key, value)
VALUES ($1,
        $2)
ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = now()
returning *;

-- name: GetByKey :one
-- -- cache: 60s
-- -- timeout: 3s
SELECT * FROM public.kv WHERE key = $1;