# Planning Carryover — Design Spec

## Problem

The planning tab shows the next sprint's capacity and task allocation, but doesn't account for unfinished tasks from the previous (current/active) sprint. These tasks will carry over and consume member time in the next sprint, making capacity calculations inaccurate.

## Solution

Extend the planning view to fetch, display, and account for carryover tasks from the previous sprint. Carryover hours affect member allocation percentages and the global "Horas Restantes" calculation.

## Carryover Rules

Tasks from the previous sprint are included/excluded based on status:

| Status | Rule |
|--------|------|
| Backlog, A Fazer, Em Desenvolvimento, Desenvolvimento | **100%** of estimated hours |
| Teste | **50%** of estimated hours |
| Code Review | Excluded |
| Deploy | Excluded |
| Validação do Solicitante | Excluded |
| Concluído | Excluded |
| Cancelado | Excluded |
| Rejeitada | Excluded |

Rationale: the closer a task is to completion, the less time it will consume in the next sprint. Tasks in Teste have progressed significantly — 50% accounts for remaining test/fix cycles. Code Review, Deploy, and Validação are near-done — excluded entirely.

## Backend Changes

### Endpoint: `GET /sprints/{id}/next`

The `{id}` parameter is the current sprint. The endpoint already queries the next sprint. Extend the response to include carryover data.

**New fields in response:**

```json
{
  "sprint": { "..." : "..." },
  "membros": [ "..." ],
  "carryover_tasks": [
    {
      "id": "uuid",
      "numero_ticket": "PROJ-123",
      "resumo": "Task summary",
      "status": "Em Desenvolvimento",
      "tipo": "Story",
      "prioridade": "Alta",
      "horas": 8.0,
      "horas_carryover": 8.0,
      "membro_id": "uuid",
      "membro_nome": "Davi",
      "projeto_chave": "PROJ"
    }
  ],
  "horas_carryover_total": 24.0
}
```

- `horas`: original task estimate
- `horas_carryover`: effective hours (50% for Teste, 100% for others)
- Only includes tasks assigned to members who belong to the target equipe

**Query logic:**

1. Get tasks from the current sprint (`{id}` parameter) with `status NOT IN ('Concluído', 'Cancelado', 'Rejeitada', 'Code Review', 'Deploy', 'Validação do Solicitante')`
2. Filter to members in the target equipe
3. Apply 50% multiplier to Teste tasks
4. Return as `carryover_tasks` array and sum as `horas_carryover_total`

### Service: `planning.go`

Add carryover query after fetching next sprint data. New repository method `GetCarryoverTasks(ctx, sprintID, membroIDs)` returns tasks matching the status filter.

### Repository: `planning.go`

New method:

```sql
SELECT t.id, t.numero_ticket, t.resumo, t.status, t.tipo, t.prioridade,
       t.estimativa_tempo, t.responsavel_id, m.nome as membro_nome,
       p.chave as projeto_chave
FROM tarefas t
JOIN membros m ON m.id = t.responsavel_id
LEFT JOIN projetos p ON p.id = t.projeto_id
WHERE t.sprint_id = $1
  AND t.responsavel_id = ANY($2)
  AND t.status NOT IN ('Concluído', 'Cancelado', 'Rejeitada', 'Code Review', 'Deploy', 'Validação do Solicitante')
```

## Frontend Changes

### State: `planningState`

New fields:
- `carryoverTasks`: array of carryover task objects (from API)
- `carryoverByMember`: map of `membro_id` -> array of carryover tasks
- `horasCarryoverTotal`: sum of all carryover hours

Populated when loading planning data from `/sprints/{id}/next` response.

### Header Stat: "Horas Carryover"

New stat box positioned after "Horas Alocadas":

```
Total Horas Sprint | Horas Alocadas | Horas Carryover | Horas Nao Alocadas | Horas Restantes p/ Alocacao | Qtde Cards | HeadCount
```

- CSS class: `hours` (same as other hour stats)
- Tooltip: "Horas de tarefas pendentes da sprint anterior que vao consumir tempo (Teste conta 50%)"
- Value: `horasCarryoverTotal` from state

### `calcPlanningStats()`

Add carryover sum:

```javascript
var horasCarryover = 0;
(s.carryoverTasks || []).forEach(function(ct) {
  horasCarryover += ct.horas_carryover;
});
```

Update `horasRestantes`:

```javascript
var horasRestantes = horasDisponiveis - horasAlocadas - horasCarryover;
```

Return includes `horasCarryover`.

### `recalcMemberCapacity()`

Add per-member carryover:

```javascript
var horasCarryover = 0;
(s.carryoverByMember[memberId] || []).forEach(function(ct) {
  horasCarryover += ct.horas_carryover;
});
var pct = (horasAlocPura + horasCarryover) / horasDisponiveis;
```

Return includes `horasCarryover`.

### Member Card Detail

When member has carryover hours > 0:

```
"64.0h alocadas + 8.0h carryover / 72.0h disponiveis"
```

When no carryover:

```
"64.0h alocadas / 72.0h disponiveis"
```

### Member Card Task List

Carryover tasks mixed into member's task list. Each carryover task rendered with:

- Badge: `Carryover` — amber/orange background, visually distinct
- Same row format as regular planning tasks (ticket number, summary, hours, status)
- Hours shown as effective hours (e.g., "4.0h" for an 8h Teste task at 50%)
- Non-draggable (can't reassign carryover tasks in planning view — they belong to previous sprint)

### `updatePlanningStats()`

Add flash animation for `ps-horas-carryover` element, same pattern as other stats.

## Constraints

- Carryover tasks are **read-only** in planning view — no drag-drop, no reassignment
- Carryover tasks are NOT counted in `qtdeCards` (they belong to previous sprint)
- Badge "Carryover" uses amber/orange color that works in both light and dark themes via CSS variables
- 50% multiplier for Teste is applied backend-side in `horas_carryover` field — frontend uses this directly
