ALTER TABLE tarefas ADD COLUMN removido_em TIMESTAMPTZ NULL;
ALTER TABLE tarefas ADD COLUMN motivo_remocao TEXT NULL;
CREATE INDEX idx_tarefas_removido ON tarefas(removido_em) WHERE removido_em IS NOT NULL;
