# Tarefas Não Planejadas na Sprint — Design Spec

## Objetivo

Mostrar informação sobre tarefas que entram na sprint sem planejamento prévio (após a sprint iniciar), contabilizar suas horas, e exibir um disclaimer com média histórica para orientar o planejamento de capacidade.

## Critério de "Tarefa Não Planejada"

Uma tarefa é considerada não planejada quando:

1. `data_entrada_sprint > sprint.data_inicio` — tarefa foi movida para a sprint após o início (extraído do changelog JIRA, campo "Sprint")
2. `data_entrada_sprint IS NULL AND data_criacao > sprint.data_inicio` — fallback: tarefa criada após início da sprint e sem dado de changelog

Ambos os critérios são aplicados (OR).

## Schema & Migration

Migration `000008_tarefa_entrada_sprint`:

```sql
ALTER TABLE tarefas ADD COLUMN data_entrada_sprint TIMESTAMPTZ;
CREATE INDEX idx_tarefas_entrada_sprint ON tarefas(sprint_id, data_entrada_sprint);
```

- Coluna nullable — tarefas sem sprint ou sem dado de changelog ficam NULL
- Index composto para queries de não-planejadas por sprint

## Sync — Extração do Changelog

Nova função `extractSprintEntryDate(changelog, sprintName) *time.Time` em `sync.go`:

- Percorre `histories` buscando `Field == "Sprint"` onde `toString` contém nome da sprint atual
- Retorna a **última** data de entrada (tarefa pode ter saído e voltado)
- Compara por `strings.Contains(item.ToString, sprintName)`

No `processIssue`, após resolver `sprintID`:

1. Se changelog existe e sprint existe → chamar `extractSprintEntryDate`
2. Se retornou nil → fallback: usar `data_criacao` como `data_entrada_sprint`
3. Passar valor para `UpsertTarefaParams.DataEntradaSprint`

Dados retroativos preenchidos no próximo sync completo.

## API

### Endpoint: `GET /api/v1/sprints/{id}/unplanned?equipe={equipeID}`

Resposta:

```json
{
  "sprint_atual": {
    "total_tarefas": 25,
    "tarefas_nao_planejadas": 5,
    "percentual_nao_planejadas": 20.0,
    "horas_nao_planejadas": 36.0,
    "horas_total_sprint": 180.0
  },
  "media_historica": {
    "sprints_analisadas": 8,
    "media_horas_nao_planejadas": 28.5,
    "media_percentual_nao_planejadas": 15.8,
    "capacidade_media_sprint": 180.0,
    "percentual_alocacao_sugerido": 84.2
  },
  "equipe_nome": "Devops Varejo"
}
```

### Lógica `media_historica`

1. Buscar últimas 8 sprints fechadas (`estado = 'closed'`) do mesmo `projeto_id`, ordenadas por `data_fim DESC`, excluindo sprint atual
2. Para cada sprint: contar tarefas onde critério não-planejada é verdadeiro
3. Somar `estimativa_tempo` dessas tarefas (converter de segundos para horas: `/ 3600`)
4. `capacidade_media_sprint` = `dias_uteis × 6h × membros_equipe` (mesma fórmula usada em capacity)
5. `percentual_alocacao_sugerido = 100 - (media_horas_nao_planejadas / capacidade_media_sprint * 100)`, arredondado para inteiro

### Filtro por equipe

Quando `equipeID` informado, considerar apenas tarefas cujo `responsavel_id` pertence à equipe (JOIN `equipe_membros`).

### Camadas

- **Repository:** `GetUnplannedStats(ctx, sprintID, equipeID)` + `GetHistoricalUnplanned(ctx, projetoID, equipeID, limit)`
- **Service:** `GetUnplannedAnalysis(ctx, sprintID, equipeID)`
- **Handler:** `GetUnplanned` no `SprintHandler`

### Horas

Usar `estimativa_tempo` (segundos, converter para horas). Sem fallback para `tempo_gasto`.

## Frontend

### Disclaimer azul

- **Quando mostrar:** Se `media_historica.sprints_analisadas >= 3`
- **Posição:** Banner acima dos cards de capacity, abaixo do seletor de sprint
- **Texto:**
  > "Baseada nas últimas N sprints do time '{equipe_nome}', existe uma média de XX horas de tarefas incluídas no decorrer da Sprint sem planejamento prévio. Considere manter apenas YY% de horas alocadas para cada membro desta equipe no planejamento de Sprints."
- **Visual:** Fundo azul claro (`#e3f2fd`), borda esquerda azul (`#2196F3`), ícone ℹ

### Badge em tarefas não planejadas

Nos cards de membro (capacity), tarefas identificadas como não-planejadas recebem tag visual pequena para diferenciação.

### Fetch

Request para `/api/v1/sprints/{id}/unplanned?equipe={equipeID}` em paralelo com capacity ao selecionar sprint.
