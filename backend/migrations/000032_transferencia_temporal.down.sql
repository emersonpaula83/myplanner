DROP TABLE IF EXISTS historico_salario;
DROP INDEX IF EXISTS idx_equipe_membros_temporal;
DROP INDEX IF EXISTS equipe_membros_active_unique;
ALTER TABLE equipe_membros ADD CONSTRAINT equipe_membros_equipe_id_membro_id_key UNIQUE (equipe_id, membro_id);
ALTER TABLE equipe_membros DROP COLUMN IF EXISTS data_saida;
ALTER TABLE equipe_membros DROP COLUMN IF EXISTS data_entrada;
