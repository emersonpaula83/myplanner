CREATE TABLE sprint_review_analises (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sprint_id UUID NOT NULL REFERENCES sprints(id),
    equipe_id UUID NOT NULL REFERENCES equipes(id),
    produto_ids UUID[] NOT NULL DEFAULT '{}',
    analise_json JSONB NOT NULL,
    modelo VARCHAR(100) NOT NULL,
    criado_em TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(sprint_id, equipe_id, produto_ids)
);
