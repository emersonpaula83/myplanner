-- 1. Add temporal columns to equipe_membros
ALTER TABLE equipe_membros
  ADD COLUMN data_entrada TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN data_saida TIMESTAMPTZ;

-- 2. Replace UNIQUE constraint with partial unique index
ALTER TABLE equipe_membros DROP CONSTRAINT equipe_membros_equipe_id_membro_id_key;
CREATE UNIQUE INDEX equipe_membros_active_unique
  ON equipe_membros(membro_id)
  WHERE data_saida IS NULL;

-- 3. Index for temporal queries
CREATE INDEX idx_equipe_membros_temporal
  ON equipe_membros(equipe_id, data_entrada, data_saida);

-- 4. New table for mérito/promoção event records
CREATE TABLE historico_salario (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  membro_id UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
  tipo VARCHAR(20) NOT NULL CHECK (tipo IN ('merito', 'promocao')),
  cargo_anterior VARCHAR(100),
  cargo_novo VARCHAR(100),
  salario_anterior NUMERIC(12,2),
  salario_novo NUMERIC(12,2) NOT NULL,
  data_vigencia DATE NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_historico_salario_membro ON historico_salario(membro_id);
