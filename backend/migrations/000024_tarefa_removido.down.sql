DROP INDEX IF EXISTS idx_tarefas_removido;
ALTER TABLE tarefas DROP COLUMN motivo_remocao;
ALTER TABLE tarefas DROP COLUMN removido_em;
