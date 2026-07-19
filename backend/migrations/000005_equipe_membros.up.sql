CREATE TABLE IF NOT EXISTS equipe_membros (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    equipe VARCHAR(255) NOT NULL,
    membro_id UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(equipe, membro_id)
);

CREATE INDEX idx_equipe_membros_equipe ON equipe_membros(equipe);
CREATE INDEX idx_equipe_membros_membro ON equipe_membros(membro_id);
