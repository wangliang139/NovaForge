-- name: Create :one
-- -- timeout: 3s
insert into public.llm_session (
  id,
  user_id,
  title,
  summary,
  last_dialog_id,
  dialog_count,
  turn_count,
  stats,
  last_dialog_at,
  status
) values (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) returning *;

-- name: GetByID :one
-- -- timeout: 3s
select *
from public.llm_session
where id = $1
  and deleted_at is null;

-- name: LockByID :one
-- -- timeout: 3s
select *
from public.llm_session
where id = $1
  and deleted_at is null
for update;

-- name: ListByUserID :many
-- -- timeout: 5s
select *
from public.llm_session
where user_id = $1
  and deleted_at is null
order by updated_at desc
limit $2 offset $3;

-- name: UpdateStatus :one
-- -- timeout: 3s
update public.llm_session
set status = $2,
    updated_at = now()
where id = $1
returning *;

-- name: UpdateTitle :one
-- -- timeout: 3s
update public.llm_session
set title = $2,
    updated_at = now()
where id = $1
returning *;

-- name: UpdateSummary :one
-- -- timeout: 3s
update public.llm_session
set summary = $2,
    updated_at = now()
where id = $1
returning *;

-- name: UpdateActivity :one
-- -- timeout: 3s
update public.llm_session
set last_dialog_id = sqlc.arg('last_dialog_id'),
    dialog_count = dialog_count + sqlc.arg('dialog_count_delta'),
    turn_count = turn_count + sqlc.arg('turn_count_delta'),
    last_dialog_at = sqlc.arg('last_dialog_at'),
    updated_at = now()
where id = sqlc.arg('id')
returning *;

-- name: DeleteByID :execrows
-- -- timeout: 3s
update public.llm_session
set deleted_at = now(),
    updated_at = now()
where id = $1
  and deleted_at is null;

-- name: SetRegenerateAnswerID :one
-- -- timeout: 3s
update public.llm_session
set last_dialog_id = sqlc.arg('last_dialog_id'),
    last_dialog_at = sqlc.arg('last_dialog_at'),
    updated_at = now()
where id = sqlc.arg('id')
returning *;
