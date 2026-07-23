ALTER TABLE membros ADD COLUMN cargo VARCHAR(50)
  CHECK (cargo IN (
    'coordenador_desenvolvimento',
    'po_produto',
    'gerente_tecnologia',
    'gerente_executivo',
    'scrum_master',
    'agile_master',
    'desenvolvedor'
  ));

CREATE TABLE membro_produtos (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  membro_id UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
  produto_id UUID NOT NULL REFERENCES produtos(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(membro_id, produto_id)
);

CREATE INDEX idx_membro_produtos_membro ON membro_produtos(membro_id);
CREATE INDEX idx_membro_produtos_produto ON membro_produtos(produto_id);
