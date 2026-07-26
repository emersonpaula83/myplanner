# Disclaimer Pizza Charts — Design Spec

## Objetivo

Ao clicar nos disclaimers de sprint (Bugs/Incidentes e Tarefas Nao Planejadas), abrir modal com grafico(s) de pizza mostrando distribuicao por produto/tipo_demanda. Hover no slice mostra card flutuante com detalhes das tarefas.

## Requisitos

1. Disclaimer 🔥 (Bugs/Incidentes) → modal com 1 pizza: distribuicao por produto (componente JIRA)
2. Disclaimer ⚡ (Tarefas Nao Planejadas) → modal com 2 pizzas side by side:
   - Esquerda: por Tipo de Demanda (Iniciativa/Compromisso/META/null→"Nao classificado")
   - Direita: por Produto (componente JIRA)
3. Proporcao dos slices: horas estimadas (estimativa_tempo), nao contagem
4. Tarefa com multiplos componentes conta em cada produto (soma slices > 100% possivel)
5. Tarefas sem componente agrupadas em slice "Sem componente"
6. Hover no slice → card flutuante com tabela: Ticket | Descricao (truncada 60 chars) | Relator
7. Card posicionado proximo ao cursor, some no mouseleave
8. Pizza em SVG inline (sem libs externas)

## Backend

### Novo endpoint

`GET /sprints/{id}/disclaimer-tasks?type=manutencao|outras`

Response:
```json
{
  "tarefas": [
    {
      "id": "uuid",
      "numero_ticket": "PROJ-123",
      "resumo": "Fix login bug",
      "tipo": "Bug",
      "tipo_demanda": "Compromisso",
      "estimativa_tempo": 14400,
      "relator_nome": "Joao Silva",
      "produtos": ["Produto A", "Produto B"]
    }
  ]
}
```

### Repository query

```sql
SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.tipo_demanda,
       t.estimativa_tempo, m.nome AS relator_nome
FROM tarefas t
LEFT JOIN membros m ON m.id = t.relator_id
WHERE t.sprint_id = $1
  AND <filtro_por_type>
ORDER BY t.numero_ticket
```

Filtros:
- `type=manutencao`: `(LOWER(t.tipo) IN ('bug') OR LOWER(t.tipo) LIKE '%incidente%')`
- `type=outras`: tarefas nao planejadas — `t.data_entrada_sprint > sprint.data_inicio AND NOT (LOWER(t.tipo) IN ('bug') OR LOWER(t.tipo) LIKE '%incidente%')`

Produtos buscados separadamente por tarefa ou via lateral join:
```sql
SELECT tp.tarefa_id, p.nome
FROM tarefa_produtos tp
JOIN produtos p ON p.id = tp.produto_id
WHERE tp.tarefa_id = ANY($1)
```

### Novo struct

```go
type TarefaDisclaimerDetail struct {
    ID              uuid.UUID  `json:"id"`
    NumeroTicket    string     `json:"numero_ticket"`
    Resumo          string     `json:"resumo"`
    Tipo            string     `json:"tipo"`
    TipoDemanda     *string    `json:"tipo_demanda"`
    EstimativaTempo int        `json:"estimativa_tempo"`
    RelatorNome     *string    `json:"relator_nome"`
    Produtos        []string   `json:"produtos"`
}
```

### Interface EquipeStore / SprintStore

Adicionar:
- `GetDisclaimerTasks(ctx context.Context, sprintID uuid.UUID, taskType string) ([]domain.TarefaDisclaimerDetail, error)`

### Handler

`GetDisclaimerTasks` em `handler/sprint.go`:
- Valida `type` query param (manutencao|outras)
- Chama repo
- Retorna JSON

### Rota

`r.Get("/sprints/{id}/disclaimer-tasks", sprintHandler.GetDisclaimerTasks)`

## Frontend

### Modal Bugs/Incidentes (🔥 disclaimer click)

- Titulo: "Bugs e Incidentes por Produto"
- 1 pizza SVG — slices por produto, proporcao por horas estimadas
- Legenda lateral: cor + nome produto + % + total horas
- Botao "Fechar"

### Modal Tarefas Nao Planejadas (⚡ disclaimer click)

- Titulo: "Tarefas Nao Planejadas"
- 2 pizzas side by side:
  - Esquerda: "Por Tipo de Demanda"
  - Direita: "Por Produto"
- Mesma mecanica de legenda

### Pizza SVG

- Cada slice = `<path>` com arco calculado via sin/cos
- Paleta fixa rotativa (8-10 cores distintas, tematicas dark/light)
- Hover: slice highlight (opacity 0.8 → 1, leve scale)
- Centro: total horas formatado
- Labels % nos slices > 10%

### Card flutuante (hover)

- Posicionado via `mousemove` no slice
- Tabela: Ticket | Descricao (truncada 60 chars) | Relator
- Z-index 1000
- Background: `var(--surface)`, border: `var(--border)`, shadow
- Some no `mouseleave`

### CSS novo

- `.pie-modal-content` — layout flex para side-by-side
- `.pie-chart-wrap` — container de cada pizza + legenda
- `.pie-legend` — lista de itens da legenda
- `.pie-legend-item` — cor + label + valor
- `.pie-tooltip-card` — card flutuante hover
- Reutiliza `.modal-overlay` / `.modal` existentes

### Disclaimers clicaveis

Tornar disclaimers existentes clicaveis:
- Adicionar `cursor: pointer` e `onclick` nos divs de disclaimer
- 🔥 disclaimer: `onclick="openDisclaimerModal(sprintID, 'manutencao')"`
- ⚡ disclaimer: `onclick="openDisclaimerModal(sprintID, 'outras')"`

## Fora de escopo

- Animacoes de transicao na pizza
- Export/download do grafico
- Filtros adicionais dentro do modal
- Drill-down ao clicar no slice (so hover)
- Alteracoes na sync JIRA (relator e produtos ja sao importados)
- Migration (campos ja existem)
