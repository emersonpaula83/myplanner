ALTER TABLE equipe_membros DROP CONSTRAINT IF EXISTS equipe_membros_equipe_id_membro_id_key;
DROP INDEX IF EXISTS idx_equipe_membros_equipe_id;
ALTER TABLE equipe_membros DROP CONSTRAINT IF EXISTS fk_equipe_membros_equipe;

ALTER TABLE equipe_membros ADD COLUMN equipe VARCHAR(255);
UPDATE equipe_membros em SET equipe = e.nome FROM equipes e WHERE e.id = em.equipe_id;
ALTER TABLE equipe_membros ALTER COLUMN equipe SET NOT NULL;
ALTER TABLE equipe_membros DROP COLUMN equipe_id;

ALTER TABLE equipe_membros ADD CONSTRAINT equipe_membros_equipe_membro_id_key UNIQUE (equipe, membro_id);
CREATE INDEX idx_equipe_membros_equipe ON equipe_membros(equipe);

DROP TABLE IF EXISTS equipes;
