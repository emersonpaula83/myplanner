# Sprint Capacity Feature

## Overview

Add sprint listing and per-person capacity visualization to the Projects module. Sprints are already synced from JIRA; this feature surfaces them with capacity calculations that account for time estimates and member availability.

## Data Model (existing)

- `sprints` table: `id`, `projeto_id`, `fonte_dados_id`, `jira_id`, `nome`, `estado`, `data_inicio`, `data_fim`, `data_conclusao`, `board_id`
- `tarefas` table: `sprint_id` FK, `responsavel_id` FK, `estimativa_tempo` (seconds from JIRA)
- `disponibilidade` table: `membro_id`, `tipo`, `data_inicio`, `data_fim` (absences/vacations)

No new migrations required.

## Backend

### Endpoints

#### `GET /api/v1/projetos/:id/sprints`

Lists sprints for a project, ordered by `data_inicio DESC` (most recent first).

Query params:
- `estado` (optional): filter by sprint state (`active`, `closed`, `future`)

Response:
```json
[{
  "id": "uuid",
  "nome": "Sprint 42",
  "estado": "active",
  "data_inicio": "2026-07-07T00:00:00Z",
  "data_fim": "2026-07-21T00:00:00Z",
  "total_tarefas": 12
}]
```

#### `GET /api/v1/sprints/:id/capacity`

Calculates capacity breakdown per member for a sprint.

Response:
```json
{
  "sprint": {
    "id": "uuid",
    "nome": "Sprint 42",
    "estado": "active",
    "data_inicio": "2026-07-07T00:00:00Z",
    "data_fim": "2026-07-21T00:00:00Z"
  },
  "dias_uteis": 10,
  "membros": [{
    "membro_id": "uuid",
    "nome": "João Silva",
    "avatar_url": "https://...",
    "horas_estimadas": 48.0,
    "horas_disponiveis": 54.0,
    "percentual_alocacao": 88.9,
    "overcapacity": false,
    "ausencias": [{
      "tipo": "ferias",
      "data_inicio": "2026-07-14",
      "data_fim": "2026-07-15",
      "dias": 2
    }]
  }]
}
```

### Capacity Calculation Logic

1. **Business days**: Count weekdays (Mon-Fri) between sprint `data_inicio` and `data_fim`
2. **Per member**:
   - Query `disponibilidade` records overlapping the sprint period
   - Calculate absence days (only weekdays count)
   - `dias_disponiveis` = business_days - absence_days
   - `horas_disponiveis` = dias_disponiveis × 6 (6h/day work capacity)
   - `horas_estimadas` = SUM(tarefas.estimativa_tempo) / 3600 for tasks assigned to this member in this sprint
   - `percentual_alocacao` = (horas_estimadas / horas_disponiveis) × 100
   - `overcapacity` = percentual_alocacao > 100
3. **Edge cases**:
   - Member with 0 available hours (fully absent): show 0 available, overcapacity if any tasks assigned
   - Tasks without `estimativa_tempo`: excluded from calculation (count as 0)
   - Sprint without dates: skip capacity calc, return empty members array

### Architecture

```
handler/sprint.go  →  service/sprint.go  →  repository/sprint.go
                                          →  repository (existing disponibilidade query)
```

- `repository/sprint.go`: SQL queries (list sprints, get tasks by sprint with assignees, get disponibilidade overlap)
- `service/sprint.go`: Business logic (weekday counting, capacity math)
- `handler/sprint.go`: HTTP layer, route registration

## Frontend

### Sprint Tab

Inside the project detail view, add a "Sprints" tab alongside existing content.

**Filter chips**: `Todas` | `Ativas` | `Futuras` | `Concluídas`
- Maps to: no filter | `estado=active` | `estado=future` | `estado=closed`

**Sprint list items**:
- Sprint name
- State badge: green dot = active, blue dot = future, gray dot = closed
- Date range (formatted: "07 Jul - 21 Jul 2026")
- Task count

Ordered by `data_inicio` descending (newest first).

### Capacity Detail View

Triggered by clicking a sprint. Opens as an expandable panel or modal.

**Header**: Sprint name, state badge, date range, total business days.

**Member cards** (one per person with tasks in the sprint):
- Avatar + name
- Allocation bar: visual percentage fill (green ≤80%, amber 81-100%, red >100%)
- Text: "48h / 54h (88.9%)"
- Absence badges: pill showing "Férias 14-15 Jul" or "Folga 18 Jul"
- Overcapacity badge: red "OVERCAPACITY" pill when > 100%

## Files to Create/Modify

| File | Action |
|------|--------|
| `backend/internal/repository/sprint.go` | Create — SQL queries |
| `backend/internal/service/sprint.go` | Create — capacity logic |
| `backend/internal/handler/sprint.go` | Create — HTTP handlers |
| `backend/cmd/api/main.go` | Modify — register sprint routes |
| `frontend/index.html` | Modify — add Sprints tab + capacity UI |

## Non-Goals

- Editing sprints from within MyPlanner (read-only, data comes from JIRA sync)
- Cross-sprint capacity view (each sprint shows only its own tasks)
- Configurable hours/day (fixed at 6h)
- Sprint burndown charts (future scope)
