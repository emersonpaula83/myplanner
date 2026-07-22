DROP TABLE IF EXISTS sync_schedules;

CREATE TABLE sync_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fonte_dados_id UUID NOT NULL UNIQUE REFERENCES fonte_dados(id) ON DELETE CASCADE,
    project_keys JSONB NOT NULL DEFAULT '[]',
    horarios JSONB NOT NULL DEFAULT '[]',
    ativo BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
