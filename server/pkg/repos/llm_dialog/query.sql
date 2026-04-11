-- name: Create :one
-- -- timeout: 3s
insert into public.llm_dialog (
  id,
  session_id,
  seq,
  dialog_id,
  role,
  status,
  content_text,
  parts,
  context_meta,
  stats,
  provider,
  model,
  can_regenerate,
  error_code,
  error_message,
  visible,
  started_at,
  completed_at
) values (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
) returning *;

-- name: GetByID :one
-- -- timeout: 3s
select *
from public.llm_dialog
where id = $1
  and deleted_at is null;

-- name: ListBySessionID :many
-- -- timeout: 5s
select *
from public.llm_dialog
where session_id = $1
  and deleted_at is null
  and (role <> 'answer' or visible = true)
order by seq asc;

-- name: GetLatestAnswerBySessionID :one
-- -- timeout: 3s
select *
from public.llm_dialog
where session_id = $1
  and deleted_at is null
  and role = 'answer'
  and visible = true
order by seq desc
limit 1;

-- name: UpdateCanRegenerateBySessionID :exec
-- -- timeout: 3s
update public.llm_dialog
set can_regenerate = case when id = $2 then true else false end,
    updated_at = now()
where session_id = $1
  and deleted_at is null
  and role = 'answer'
  and visible = true;

-- name: StartAnswerStream :one
-- -- timeout: 3s
update public.llm_dialog
set status = 'streaming',
    started_at = coalesce(sqlc.narg('started_at')::timestamptz, now()),
    completed_at = null,
    updated_at = now()
where id = sqlc.arg('id')
  and deleted_at is null
returning *;

-- name: UpdateAnswerResult :one
-- -- timeout: 3s
update public.llm_dialog
set status = sqlc.arg('status'),
    content_text = sqlc.arg('content_text'),
    parts = sqlc.arg('parts'),
    context_meta = sqlc.arg('context_meta'),
    stats = sqlc.arg('stats'),
    provider = sqlc.arg('provider'),
    model = sqlc.arg('model'),
    error_code = sqlc.arg('error_code'),
    error_message = sqlc.arg('error_message'),
    completed_at = sqlc.narg('completed_at')::timestamptz,
    updated_at = now()
where id = sqlc.arg('id')
  and deleted_at is null
returning *;

-- name: HideAnswerByID :one
-- -- timeout: 3s
update public.llm_dialog
set visible = false,
    can_regenerate = false,
    updated_at = now()
where id = sqlc.arg('id')
  and role = 'answer'
  and deleted_at is null
returning *;

-- name: DeleteBySessionID :execrows
-- -- timeout: 3s
update public.llm_dialog
set deleted_at = now(),
    updated_at = now()
where session_id = $1
  and deleted_at is null;
