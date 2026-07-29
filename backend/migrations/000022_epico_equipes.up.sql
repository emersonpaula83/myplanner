CREATE TABLE epico_equipes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    epico_id UUID NOT NULL REFERENCES tarefas(id) ON DELETE CASCADE,
    equipe_id UUID NOT NULL REFERENCES equipes(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(epico_id, equipe_id)
);
CREATE INDEX idx_epico_equipes_epico ON epico_equipes(epico_id);
CREATE INDEX idx_epico_equipes_equipe ON epico_equipes(equipe_id);
