CREATE TABLE projeto_encerramentos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    epic_id UUID NOT NULL REFERENCES tarefas(id),
    descricao TEXT NOT NULL,
    data_encerramento DATE NOT NULL,
    encerrado_por TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(epic_id)
);
