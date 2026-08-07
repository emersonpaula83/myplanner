CREATE TABLE review_destinatarios (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    equipe_id UUID NOT NULL REFERENCES equipes(id) ON DELETE CASCADE,
    tipo VARCHAR(10) NOT NULL CHECK (tipo IN ('email', 'whatsapp')),
    valor VARCHAR(255) NOT NULL,
    nome VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_review_dest_unique ON review_destinatarios(equipe_id, tipo, valor);
