# Incluir Tarefas na Sprint — Design Spec

## Goal

Allow users in the Planning tab to search for Jira tickets by key, preview results, and add selected tasks to the current planning sprint. Also make the Planning header (title + stats + action bar) sticky so it stays visible while scrolling.

## Architecture

Two new backend endpoints on the planning handler. One new Jira client method (`SearchIssuesByKeys`). Frontend: new modal, sticky header wrapper, "Incluir Tarefas" button in the action bar.

## Scope

- Search tasks by ticket key (e.g. PROJ-123)
- Local DB search first, Jira API fallback for keys not found locally
- Import Jira-only results into DB via new `UpsertTarefaFromJira` repo method
- Move included tasks to sprint in both DB and Jira (`MoveToSprint`)
- Sticky Planning header (title + stats + action bar)
- "Incluir Tarefas" button always visible in action bar

## Global Constraints

- Monolithic vanilla JS frontend (`frontend/index.html`)
- Go backend with chi router, pgx, zap logger
- `var` for globals, `function` declarations (no ES6 modules)
- CSS custom properties for theming (light/dark)
- All user-facing text in Portuguese
- No commits without explicit user consent

---

## Backend

### New Jira Client Method

**`SearchIssuesByKeys(ctx, projectKey string, issueKeys []string) ([]JiraIssue, error)`**

- JQL: `project = {projectKey} AND key IN ({keys})`
- Same fields and pagination as `GetProjectIssues`
- Added to `Client` interface in `jira/client.go`

### Endpoint 1: Search Tasks

**`POST /sprints/{id}/search-tasks`**

Request:
```json
{
  "ticket_keys": ["PROJ-123", "PROJ-456", "PROJ-789"]
}
```

Handler flow:
1. Parse sprint ID from URL, get user ID from auth context
2. Load sprint from DB to get `projeto_chave` and `fonte_dados_id`
3. Normalize input keys (trim, uppercase)
4. Query `tarefas` table: `WHERE numero_ticket IN ($keys) AND projeto_id = sprint.projeto_id`
5. Separate found (local) keys from not-found keys
6. For not-found keys: call `SearchIssuesByKeys` on Jira API, then upsert each into DB via a new `UpsertTarefaFromJira(ctx, projetoID, jiraIssue)` repository method (simpler than `processIssue` — only inserts/updates the `tarefas` row with basic fields, no member cache or sprint cache needed)
7. Re-query imported tasks to get their DB IDs
8. Check which found tasks already have `sprint_id = this sprint`
9. Return response

Response:
```json
{
  "found": [
    {
      "id": "uuid",
      "key": "PROJ-123",
      "resumo": "Fix login bug",
      "tipo": "Bug",
      "status": "To Do",
      "prioridade": "High",
      "responsavel_nome": "Joao",
      "source": "local"
    }
  ],
  "not_found": ["PROJ-789"],
  "already_in_sprint": [
    {
      "id": "uuid",
      "key": "PROJ-100",
      "resumo": "Already here",
      "tipo": "Story",
      "status": "In Progress"
    }
  ]
}
```

### Endpoint 2: Include Tasks in Sprint

**`POST /sprints/{id}/include-tasks`**

Request:
```json
{
  "tarefa_ids": ["uuid1", "uuid2"]
}
```

Handler flow:
1. Parse sprint ID, get user ID
2. Load sprint to get `jira_id` (sprint Jira ID) and `fonte_dados_id`
3. For each tarefa ID:
   a. `UPDATE tarefas SET sprint_id = $sprintID, data_entrada_sprint = NOW()`
   b. Get Jira client for fonte_dados_id
   c. `MoveToSprint(ctx, sprint.jira_id, tarefa.numero_ticket)`
4. Re-query all included tasks as `PlanningTarefa` structs
5. Return tasks in same shape as `NextSprintResult.Tarefas`

Response:
```json
{
  "tarefas": [
    {
      "id": "uuid",
      "numero_ticket": "PROJ-123",
      "resumo": "Fix login bug",
      "tipo": "Bug",
      "status": "To Do",
      "prioridade": "High",
      "estimativa_tempo": 14400,
      "responsavel_id": null,
      "projeto_chave": "PROJ"
    }
  ]
}
```

### New Repository Methods

In `planning.go`:
- `SearchTarefasByKeys(ctx, projetoID uuid.UUID, keys []string) ([]SearchTarefaResult, error)` — query `tarefas` joined with members for responsavel_nome
- `MoveTarefasToSprint(ctx, sprintID uuid.UUID, tarefaIDs []uuid.UUID) error` — batch update sprint_id + data_entrada_sprint
- `GetTarefasByIDs(ctx, ids []uuid.UUID) ([]PlanningTarefa, error)` — fetch specific tasks as PlanningTarefa

### New Service Methods

In `planning.go`:
- `SearchTasks(ctx, sprintID uuid.UUID, ticketKeys []string) (*SearchTasksResult, error)` — orchestrates local search + Jira fallback
- `IncludeTasks(ctx, sprintID uuid.UUID, tarefaIDs []uuid.UUID) ([]PlanningTarefa, error)` — orchestrates DB update + Jira MoveToSprint

### Route Registration

```
r.Post("/sprints/{id}/search-tasks", planningHandler.SearchTasks)
r.Post("/sprints/{id}/include-tasks", planningHandler.IncludeTasks)
```

Both routes inside the existing authenticated+equipe group.

---

## Frontend

### Sticky Header

Wrap the 3 existing blocks (title row, stats bar, action bar) in a single container div:

```css
.planning-sticky-header {
  position: sticky;
  top: 0;
  z-index: 10;
  background: var(--surface-primary);
  padding-bottom: 8px;
  border-bottom: 1px solid var(--border-subtle);
}
```

### Action Bar Changes

The existing `planning-bulk-bar` currently shows/hides based on selection. Restructure:

- Action bar is always visible (not `display:none`)
- Left side: bulk actions (Alocar, Mover, Voltar) — still show/hide based on selection, wrapped in a span
- Right side: "Incluir Tarefas" button — always visible

```html
<div class="planning-action-bar">
  <div class="planning-bulk-actions" id="planning-bulk-actions" style="display:none">
    <span class="planning-bulk-count" id="planning-bulk-count">0 selecionadas</span>
    <button class="btn-bulk" onclick="bulkAlocar()">Alocar</button>
    <button class="btn-bulk" onclick="bulkMoverProximaSprint()">Mover proxima Sprint</button>
    <button class="btn-bulk" onclick="bulkVoltarBacklog()">Voltar para Backlog</button>
  </div>
  <button class="btn-sm primary" onclick="openAddTasksModal()">+ Incluir Tarefas</button>
</div>
```

### Modal "Incluir Tarefas na Sprint"

New modal HTML added to the modals section.

**Structure:**
- Title: "Incluir Tarefas na Sprint"
- Textarea: placeholder "Cole as chaves dos tickets (ex: PROJ-123, PROJ-456)"
- Button: "Pesquisar"
- Results area (hidden until search):
  - "Encontrados" section — checkboxes, key badge, resumo, tipo, status, source tag (local/jira)
  - "Ja na sprint" section — same display but disabled, "Ja na sprint" tag
  - "Nao encontrados" section — list of keys in red
- Footer: "Incluir na Sprint (N)" button + "Fechar" button

**JS Functions:**
- `openAddTasksModal()` — opens modal, clears previous state
- `searchTasksForSprint()` — reads textarea, parses keys, POST to `/sprints/{id}/search-tasks`, renders results
- `includeTasksInSprint()` — collects checked task IDs, POST to `/sprints/{id}/include-tasks`, inserts returned tasks into `planningState.tasks` + `unassigned`, closes modal, re-renders planning tab via `renderPlanningTab()`
- `toggleAddTaskCheck(id)` — toggle checkbox, update "Incluir na Sprint (N)" count

**State:**
- `_addTasksResults` — array of found tasks from search
- `_addTasksSelected` — Set of selected task IDs

---

## Error Handling

- Jira API failure during search: return local results + error message for Jira-only keys (partial success)
- Jira API failure during include (MoveToSprint): skip that task, return partial success with error list
- Empty input: disable Pesquisar button
- No results: "Nenhum ticket encontrado" message
- Network error: standard alert with error message

---

## Testing

- Backend: integration tests for search (local found, Jira fallback, already-in-sprint) and include (DB update + Jira mock)
- Frontend: manual test checklist in plan
