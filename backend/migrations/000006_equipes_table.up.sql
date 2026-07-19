CREATE TABLE IF NOT EXISTS equipes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nome VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Migrate existing equipe names from equipe_membros to equipes table
INSERT INTO equipes (nome)
SELECT DISTINCT equipe FROM equipe_membros
ON CONFLICT (nome) DO NOTHING;

-- Add equipe_id column to equipe_membros
ALTER TABLE equipe_membros ADD COLUMN equipe_id UUID;

-- Populate equipe_id from equipes table
UPDATE equipe_membros em
SET equipe_id = e.id
FROM equipes e
WHERE e.nome = em.equipe;

-- Make equipe_id NOT NULL and add FK
ALTER TABLE equipe_membros ALTER COLUMN equipe_id SET NOT NULL;
ALTER TABLE equipe_membros ADD CONSTRAINT fk_equipe_membros_equipe FOREIGN KEY (equipe_id) REFERENCES equipes(id) ON DELETE CASCADE;

-- Drop old equipe string column and unique constraint
ALTER TABLE equipe_membros DROP CONSTRAINT equipe_membros_equipe_membro_id_key;
DROP INDEX IF EXISTS idx_equipe_membros_equipe;
ALTER TABLE equipe_membros DROP COLUMN equipe;

-- New unique constraint and index
ALTER TABLE equipe_membros ADD CONSTRAINT equipe_membros_equipe_id_membro_id_key UNIQUE (equipe_id, membro_id);
CREATE INDEX idx_equipe_membros_equipe_id ON equipe_membros(equipe_id);
