CREATE INDEX idx_membros_team ON membros(team) WHERE team IS NOT NULL AND ativo = true;

CREATE INDEX idx_tarefas_responsavel_periodo ON tarefas(responsavel_id, data_atualizado)
    WHERE responsavel_id IS NOT NULL;

CREATE INDEX idx_tarefas_tipo_demanda ON tarefas(tipo_demanda)
    WHERE tipo_demanda IS NOT NULL;

CREATE INDEX idx_disponibilidade_periodo ON disponibilidade(membro_id, data_inicio, data_fim);
