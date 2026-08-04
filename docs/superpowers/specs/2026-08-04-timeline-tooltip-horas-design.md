# Sprint Timeline Tooltip — Mostrar Horas

## Resumo

Adicionar horas absolutas ao tooltip do Sprint Timeline, ao lado do percentual existente.

## Mudança

Arquivo: `frontend/index.html`, linhas ~4922-4926 (tooltip do sprint bar no canvas).

### Formato atual

```
Sprint 10
01/07/2026 — 14/07/2026
Alocação: 59.0%
Livre: 41.0%
Headcount: 5.0
```

### Formato novo

```
Sprint 10
01/07/2026 — 14/07/2026
Alocação: 59.0% / 80.0h
Livre: 41.0% / 55.5h
Headcount: 5.0
```

### Cálculo

- Horas alocadas: `found.d.horas_alocadas` (já disponível no objeto)
- Horas livres: `Math.max(0, found.d.horas_capacidade - found.d.horas_alocadas)`

### Backend

Nenhuma mudança. Dados `horas_alocadas` e `horas_capacidade` já retornados pela API `GetSprintsTimeline`.

## Sync de Tarefas — Verificação

Verificado que o SQL `ON CONFLICT` em `UpsertTarefa` (`backend/internal/repository/sync.go:137-174`) já atualiza todos os campos necessários:

| Campo | Atualizado? | Nota |
|-------|-------------|------|
| numero_ticket | Conflict key | Usado para match, não precisa atualizar |
| responsavel_id | Sim | `EXCLUDED.responsavel_id` |
| relator_id | Sim | `EXCLUDED.relator_id` |
| estimativa_tempo | Sim | `EXCLUDED.estimativa_tempo` |
| parent_id | Sim | `EXCLUDED.parent_id` |
| sprint_id | Sim | `EXCLUDED.sprint_id` |
| data_limite | Sim (COALESCE) | Preserva valor existente se Jira enviar NULL |
| tipo | Sim | `EXCLUDED.tipo` |
| tipo_demanda | Sim | `EXCLUDED.tipo_demanda` |

Nenhuma mudança necessária no sync.
