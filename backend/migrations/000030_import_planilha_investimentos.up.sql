ALTER TABLE membros ADD COLUMN matricula VARCHAR(20);
ALTER TABLE membros ADD COLUMN ultimo_aumento DATE;
ALTER TABLE membros ADD COLUMN gestor_id UUID REFERENCES membros(id);

CREATE TABLE import_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tipo VARCHAR(20) NOT NULL, -- 'csv' or 'sheets_url'
    url TEXT,
    gid VARCHAR(20),
    ultimo_sync TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
