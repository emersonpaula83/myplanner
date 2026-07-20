DROP INDEX IF EXISTS idx_tarefas_entrada_sprint;
ALTER TABLE tarefas DROP COLUMN IF EXISTS data_entrada_sprint;
