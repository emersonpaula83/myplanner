CREATE TABLE sync_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fonte_dados_id UUID NOT NULL REFERENCES fonte_dados(id) ON DELETE CASCADE,
    project_key VARCHAR(50) NOT NULL,
    horarios JSONB NOT NULL DEFAULT '[]',
    ativo BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(fonte_dados_id, project_key)
);

ALTER TABLE sync_logs ADD COLUMN IF NOT EXISTS origem VARCHAR(20) NOT NULL DEFAULT 'manual';
