CREATE TABLE IF NOT EXISTS ds_items (
    id bigserial PRIMARY KEY,
    ds_id int NOT NULL,
    data jsonb NOT NULL,
    ts TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

create index idx_ds_items_ds_id on ds_items(ds_id);
create index idx_ds_items_ts on ds_items(ts);