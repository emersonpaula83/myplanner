ALTER TABLE tarefas ADD COLUMN data_entrada_sprint TIMESTAMPTZ;
CREATE INDEX idx_tarefas_entrada_sprint ON tarefas(sprint_id, data_entrada_sprint);
