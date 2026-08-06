CREATE TABLE sprint_review_snapshots (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sprint_id     UUID NOT NULL REFERENCES sprints(id) ON DELETE CASCADE,
    snapshot_json JSONB NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_sprint_review_snapshot UNIQUE (sprint_id)
);
