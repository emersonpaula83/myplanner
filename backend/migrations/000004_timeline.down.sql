DROP INDEX IF EXISTS idx_tarefas_tipo_team_epico;
DROP INDEX IF EXISTS idx_tarefas_parent;
ALTER TABLE tarefas DROP COLUMN IF EXISTS data_inicio_execucao;
ALTER TABLE tarefas DROP COLUMN IF EXISTS apelido;
ALTER TABLE tarefas DROP COLUMN IF EXISTS parent_id;
