ALTER TABLE tarefas ADD COLUMN parent_id UUID REFERENCES tarefas(id) ON DELETE SET NULL;
ALTER TABLE tarefas ADD COLUMN apelido VARCHAR(15);
ALTER TABLE tarefas ADD COLUMN data_inicio_execucao TIMESTAMPTZ;

CREATE INDEX idx_tarefas_parent ON tarefas(parent_id);
CREATE INDEX idx_tarefas_tipo_team_epico ON tarefas(tipo, team) WHERE tipo = 'Épico';
