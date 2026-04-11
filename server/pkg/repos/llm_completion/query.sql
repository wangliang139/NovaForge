-- name: Create :one
-- -- timeout: 3s
insert into public.llm_completion (
  session_id, scene_id, prompt_id, scene_key, platform, provider, model, variables, messages, question, answer, error, duration, tokens, status
) values (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
) returning *;

-- name: GetCompletionStats :one
-- -- timeout: 5s
SELECT
  COUNT(*)::bigint AS total_count,
  COUNT(*) FILTER (WHERE error = '')::bigint AS success_count,
  COUNT(*) FILTER (WHERE error != '')::bigint AS fail_count,
  COALESCE(AVG(duration) FILTER (WHERE error = ''), 0)::double precision AS avg_duration_ms
FROM public.llm_completion
WHERE created_at >= to_timestamp(sqlc.arg('start_ts')::bigint)::timestamptz
  AND created_at < to_timestamp(sqlc.arg('end_ts')::bigint)::timestamptz;

-- name: GetCompletionStatsByScene :many
-- -- timeout: 5s
SELECT
  scene_key,
  scene_id,
  COUNT(*)::bigint AS total_count,
  COUNT(*) FILTER (WHERE error = '')::bigint AS success_count,
  COUNT(*) FILTER (WHERE error != '')::bigint AS fail_count,
  COALESCE(AVG(duration) FILTER (WHERE error = ''), 0)::double precision AS avg_duration_ms
FROM public.llm_completion
WHERE created_at >= to_timestamp(sqlc.arg('start_ts')::bigint)::timestamptz
  AND created_at < to_timestamp(sqlc.arg('end_ts')::bigint)::timestamptz
GROUP BY scene_key, scene_id;
