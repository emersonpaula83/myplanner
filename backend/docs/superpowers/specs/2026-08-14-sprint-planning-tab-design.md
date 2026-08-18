# Sprint Planning Tab — Design Spec

## Goal

Add a "Planning" tab to the sprint detail view that appears when the active sprint is within 3 days of ending. The tab lets coordinators and team members plan the next sprint: reassign tasks between people via drag-and-drop, fill required fields, bulk-move tasks, and push all changes to Jira in a single batch operation with real-time progress feedback.

## Scope

### In Scope
- New "Planning" tab in sprint detail (visible ≤3 days before active sprint ends)
- Plan the **next future sprint** — tasks already exist there
- Person cards with drag-and-drop task reallocation (SortableJS)
- "Unassigned" card as task pool at the top
- Overcapacity detection modal with before/after comparison
- Required fields validation modal (Tipo de Demanda, Estimativa)
- Header stats: Tasks (Total, Allocated, Unallocated) + Hours (Available, Pending, Allocated)
- Multi-select with bulk actions: Alocar, Mover próxima Sprint, Voltar para Backlog
- Jira task type SVG icons across entire sprint module (Acompanhamento + Planning)
- Client-side capacity cache with instant recalculation
- "Finalizar Planning" button with batch Jira push and progress modal
- New backend endpoints: next sprint lookup, planning apply, planning progress
- New Jira client method: RemoveFromSprint

### Out of Scope
- Tipo de Cliente field
- Despriorizar action
- Database schema changes for capacity storage (calculated on-the-fly)

## Architecture

**Approach: Full Client Cache.** Frontend loads capacity data from the backend once, stores all planning changes in a JS `planningState` object, recalculates capacity instantly on the client, and submits the complete changeset to the backend only when the user clicks "Finalizar Planning". The backend processes operations sequentially (DB + Jira) and reports progress via polling.

**Tech stack:** Vanilla JS (existing SPA), SortableJS (~10kb, CDN-free inline), Go backend, PostgreSQL, Jira REST API v3 + Agile API.

## Global Constraints

- Frontend is a single monolithic HTML file (`frontend/index.html`, ~9,140 lines)
- No framework — vanilla JS with DOM manipulation
- Sprint states from Jira: `"active"`, `"closed"`, `"future"`
- Capacity formula: `horasPorDia = 6.0`, business days exclude weekends + holidays
- Status buckets (must match backend `GetCapacity` exactly):
  - `statusExecutado`: Code Review, Teste, Validacao do Solicitante, Deploy, Concluido
  - `statusAmbos`: Teste, Validacao do Solicitante, Deploy
  - `statusPendente`: Backlog, Desenvolvimento, Em Desenvolvimento, A Fazer
  - Skip: Cancelado, Rejeitada
- Only active sprints show the Planning tab
- Planning tab visibility: `data_fim - now <= 3 days`
- Never auto-commit/push without explicit user consent

---

## 1. Tab Visibility Logic

**Client-side check.** When `openSprintCapacity()` renders tabs for an active sprint:

```
const daysLeft = (new Date(sprint.data_fim) - new Date()) / (1000*60*60*24)
const showPlanning = sprint.estado === 'active' && daysLeft <= 3
```

- Tab renders with highlighted/accent styling (badge, different background color)
- Non-active sprints never show the tab
- Tab label: "Planning"

## 2. Tab Layout

### 2.1 Header Stats Bar

Single row, two color groups:

**Tasks group (primary/blue tones):**
- Total de Tarefas
- Tarefas Alocadas (have responsavel_id)
- Tarefas Nao Alocadas (no responsavel_id)

**Hours group (secondary/green tones):**
- Horas Disponíveis Sprint (sum of all team members' horasDisponiveis)
- Horas Pendentes Alocação (hours from tasks without responsavel_id)
- Horas Alocadas (sum of allocated hours per capacity formula)

All stats update instantly on every change.

### 2.2 Bulk Action Bar

Visible only when `selected.size >= 1`. Buttons:

- **Alocar** — opens member selection modal
- **Mover próxima Sprint** — moves selected tasks to next future sprint (or fallback to backlog if none exists)
- **Voltar para Backlog** — removes selected tasks from sprint

### 2.3 Task Cards Area

**"Não Alocadas" card** — top position, distinct styling (dashed border or muted background). Shows all tasks with `responsavel_id = null`. Acts as a SortableJS drop zone.

**Person cards** — one per team member, sorted by `percentual_alocacao` DESC. Each shows:
- Avatar (image or initials), name
- Capacity bar with percentage, color-coded (green <80%, amber 80-100%, red >100%)
- Hours breakdown: alocadas / disponíveis
- Overcapacity badge when >100%
- List of assigned tasks

**Each task row:**
- Checkbox for multi-select
- Jira type SVG icon (16x16)
- Ticket key (linked)
- Summary (truncated)
- Hours (or "—" if no estimate)
- Draggable handle

## 3. Drag & Drop (SortableJS)

### 3.1 Setup

All cards (Não Alocadas + person cards) share one SortableJS `group: 'planning'`. Each card's task list is a connected sortable container.

### 3.2 On Drop Event

Sequence:
1. Identify source member (or unassigned) and destination member (or unassigned)
2. Check required fields on the task (Tipo de Demanda, Estimativa):
   - If empty → open Required Fields modal. On cancel → revert drag.
3. Recalculate destination member's capacity
4. If destination >100% → open Overcapacity modal. On cancel → revert drag.
5. Update `planningState`: move task ID between member's `tarefaIds` arrays
6. Recalculate both source and destination capacity
7. Update header stats
8. Re-render affected cards

### 3.3 Dragging to "Não Alocadas"

Removes `responsavelId` from task. No overcapacity check needed. Updates stats.

## 4. Modals

### 4.1 Overcapacity Modal

Triggered when a drop or bulk allocation would push a member above 100%.

Content:
- Warning message: "{Nome} ficará {X}% overcapacity. Continuar?"
- Before/After comparison table:
  - Horas Alocadas: before → after
  - Horas Disponíveis: unchanged
  - Alocação %: before → after
  - Tarefas: count before → after
- Buttons: Cancelar (revert), Continuar (accept)

### 4.2 Required Fields Modal

Triggered when task being allocated has empty Tipo de Demanda or Estimativa.

Content:
- Task identification: key + summary
- Tipo de Demanda dropdown: Compromisso, Meta, Iniciativa
- Estimativa input: hours (converted to seconds internally: `hours * 3600`)
- Only shows fields that are actually empty
- For bulk operations: lists all tasks with missing fields, fill in batch
- Buttons: Cancelar (abort allocation), Confirmar (save to planningState)

### 4.3 Bulk Allocate Modal

Triggered by "Alocar" button when tasks are selected.

Content:
- "Alocar N tarefas"
- List of team members with current capacity (name, %, hours bar)
- Select one member as destination
- After selection → triggers required fields check → triggers overcapacity check
- Buttons: Cancelar, Alocar

### 4.4 Move to Next Sprint

When "Mover próxima Sprint" is clicked:
1. Call `GET /sprints/{nextSprintId}/next?equipe_id=xxx` to find the sprint after the one being planned
2. If no next sprint exists → show message: "Não há próxima Sprint para o projeto. Deseja mover para o backlog?"
3. If exists → confirm: "Mover N tarefas para {Sprint Name}?"
4. On confirm: tasks removed from planningState view, added to `movedNextSprint` list
5. Stats update instantly

### 4.5 Finalizar Planning Modal

Triggered by "Finalizar Planning" button.

Content:
- Title: "Finalizando Planning — {Sprint Name}"
- Progress bar with percentage
- Operation list with per-item status icons:
  - ✅ done
  - 🔄 running
  - ⬜ pending
  - ❌ error
- Counter: "X de Y operações concluídas"
- Error details inline if any operation fails
- Buttons: "Cancelar" during execution (stops remaining, keeps completed), "Fechar" when done

Flow:
1. `POST /sprints/{id}/planning/apply` with changeset → returns `job_id`
2. Poll `GET /sprints/{id}/planning/progress?job_id=xxx` every 1 second
3. Update modal UI with each poll response
4. When `finished: true` → show summary, change button to "Fechar"
5. On close → reload sprint data (re-sync from DB)

## 5. Client-Side Capacity Engine

### 5.1 planningState Structure

```js
window.planningState = {
  sprint: { id, nome, dataInicio, dataFim, jiraId },
  nextNextSprint: { id, nome, jiraId } | null,
  diasUteis: Number,
  feriados: [...],
  
  tasks: {
    "uuid": {
      id, key, resumo, tipo, status,
      responsavelId, estimativaTempo, tipoDemanda,
      // originals for diff
      originalResponsavelId, originalEstimativa, originalTipoDemanda,
      originalSprintId
    }
  },
  
  members: {
    "uuid": {
      id, nome, avatarUrl, horasDisponiveis, ausencias,
      tarefaIds: [...]
    }
  },
  
  unassigned: [...],       // task IDs without responsavel
  movedBacklog: [...],     // task IDs removed from sprint
  movedNextSprint: [...],  // task IDs moved to next-next sprint
  
  selected: new Set()      // multi-select state
}
```

### 5.2 Capacity Recalculation

Function `recalcMemberCapacity(memberId)`:

```
For each task in member.tarefaIds:
  skip if status in [Cancelado, Rejeitada]
  skip if task in movedBacklog or movedNextSprint
  
  if status in statusPendente → horasAlocadas += estimativaTempo/3600
  if status in statusExecutado → horasExecutadas += estimativaTempo/3600
  if status in statusAmbos → horasAmbos += estimativaTempo/3600

// statusPendente tasks contribute to "pure" allocation
// statusAmbos tasks count in BOTH allocated and executed
// For percentage: use (horasAlocadas - horasAmbos) to avoid double-counting
horasAlocadasPura = horasAlocadas - horasAmbos

percentual = horasAlocadasPura / horasDisponiveis * 100
overcapacity = percentual > 100
```

Mirrors backend `GetCapacity` logic exactly.

### 5.3 Changeset Builder

`buildChangeset()` compares current state vs originals:

- `reassigned`: tasks where `responsavelId !== originalResponsavelId`
- `estimated`: tasks where `estimativaTempo !== originalEstimativa`
- `tipo_demanda`: tasks where `tipoDemanda !== originalTipoDemanda`
- `moved_next_sprint`: task IDs in `movedNextSprint` array
- `moved_backlog`: task IDs in `movedBacklog` array

Only changed fields are sent. Minimal operations.

## 6. Backend — New Components

### 6.1 New Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/sprints/{id}/next` | Find next future sprint for same board. Query param: `equipe_id`. Returns 404 if none. |
| POST | `/api/v1/sprints/{id}/planning/apply` | Submit planning changeset. Returns `{ job_id }`. |
| GET | `/api/v1/sprints/{id}/planning/progress` | Poll job progress. Query param: `job_id`. Returns operation list with statuses. |

### 6.2 Planning Service

New file: `internal/service/planning.go`

**PlanningService struct** — depends on: SprintRepository, FonteDadosStore, Jira client factory.

**Methods:**
- `GetNextSprint(ctx, sprintID, equipeID)` — finds next future sprint by board_id + data_inicio ordering
- `Apply(ctx, sprintID, fonteDadosID, changes)` — creates job, processes in goroutine
- `GetProgress(ctx, jobID)` — returns current job state

**Job storage:** In-memory `sync.Map` keyed by job ID. Jobs auto-expire after 1 hour.

### 6.3 Planning Repository

New file: `internal/repository/planning.go`

**Methods:**
- `GetNextSprint(ctx, boardID, currentDataInicio)` — `SELECT ... FROM sprints WHERE board_id = $1 AND estado = 'future' AND data_inicio > $2 ORDER BY data_inicio ASC LIMIT 1`
- `UpdateTarefaResponsavel(ctx, tarefaID, responsavelID)` — already exists in sprint.go, reuse
- `UpdateTarefaEstimativa(ctx, tarefaID, segundos)` — `UPDATE tarefas SET estimativa_tempo = $2 WHERE id = $1`
- `UpdateTarefaTipoDemanda(ctx, tarefaID, valor)` — `UPDATE tarefas SET tipo_demanda = $2 WHERE id = $1`
- `MoveTarefaToSprint(ctx, tarefaID, sprintID)` — `UPDATE tarefas SET sprint_id = $2 WHERE id = $1`
- `RemoveTarefaFromSprint(ctx, tarefaID)` — `UPDATE tarefas SET sprint_id = NULL WHERE id = $1`

### 6.4 New Jira Client Method

```go
RemoveFromSprint(ctx context.Context, issueKey string) error
```

Implementation: `PUT /rest/api/3/issue/{issueKey}` with body `{"fields":{"sprint":null}}`.

Added to `Client` interface in `internal/jira/client.go`.

### 6.5 Planning Handler

New file: `internal/handler/planning.go`

Routes registered in `cmd/api/main.go`:
```go
r.Get("/sprints/{id}/next", planningHandler.GetNextSprint)
r.Post("/sprints/{id}/planning/apply", planningHandler.Apply)
r.Get("/sprints/{id}/planning/progress", planningHandler.GetProgress)
```

### 6.6 Apply Processing Flow

For each operation in the changeset (sequential):

1. **Reassign:** `repo.UpdateTarefaResponsavel()` → `jiraClient.AssignIssue()`
2. **Estimate:** `repo.UpdateTarefaEstimativa()` → `jiraClient.UpdateTimeEstimate()`
3. **Tipo Demanda:** `repo.UpdateTarefaTipoDemanda()` → Jira custom field update via `PUT /rest/api/3/issue/{key}`
4. **Move Next Sprint:** `repo.MoveTarefaToSprint()` → `jiraClient.MoveToSprint()`
5. **Move Backlog:** `repo.RemoveTarefaFromSprint()` → `jiraClient.RemoveFromSprint()`

Each operation updates job progress. On individual failure: mark ❌, log error, continue to next operation.

## 7. Jira Task Type Icons

### 7.1 SVG Icon Map

Function `getTaskTypeIcon(tipo)` returns inline SVG string (16x16). Mapping:

| Tipo | Shape | Color |
|------|-------|-------|
| Story / História | Bookmark | `#63BA3C` (green) |
| Bug | Circle | `#E5493A` (red) |
| Task / Tarefa | Checkbox square | `#4BADE8` (blue) |
| Sub-task / Sub-tarefa | Small checkbox | `#4BADE8` (blue) |
| Epic / Épico | Lightning bolt | `#904EE2` (purple) |
| Improvement / Melhoria | Arrow up | `#63BA3C` (green) |
| Incidente | Exclamation circle | `#E5493A` (red) |
| Default (unknown) | Question circle | `#6B778C` (gray) |

### 7.2 Application Scope

Icons added to task rows in:
- **Planning tab** — each task in person cards and unassigned card
- **Acompanhamento tab** — existing `renderCapacityCard()` task rows (evolution of sprint module)

## 8. Error Handling

- **Network failure loading planning data:** Show error toast, keep Planning tab but with "Retry" button
- **Drag to invalid target:** SortableJS revert animation, no state change
- **Jira operation failure during Finalizar:** Mark individual operation ❌, continue others, show error summary at end
- **Job polling failure:** Retry polling up to 3 times, then show "Connection lost" message with manual refresh option
- **Concurrent planning:** No lock mechanism — last write wins. Acceptable for v1 since planning is typically done by one person.

## 9. Testing Strategy

### Backend
- Unit tests for PlanningService methods (mock Jira client + repository)
- Unit tests for new repository queries
- Unit tests for `RemoveFromSprint` Jira client method
- Integration test for Apply flow (mock Jira, real DB)

### Frontend
- Manual testing: drag-and-drop flow, modal interactions, capacity recalculation
- Verify capacity numbers match backend `GetCapacity` for same data
- Test edge cases: all tasks unassigned, member at exactly 100%, empty sprint
