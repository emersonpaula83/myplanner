CREATE TABLE checkpoints (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    equipe_id   UUID REFERENCES equipes(id) ON DELETE CASCADE,
    nome        VARCHAR(15) NOT NULL,
    resumo      VARCHAR(50) NOT NULL,
    data_inicio DATE NOT NULL,
    data_fim    DATE,
    cor         VARCHAR(7) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_checkpoints_equipe ON checkpoints(equipe_id);
CREATE INDEX idx_checkpoints_data ON checkpoints(data_inicio);
