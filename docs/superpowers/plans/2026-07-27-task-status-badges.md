# Task Status Badges Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add colored status badges to tasks in the allocation modal, import Jira flagged field, fix % calculation and sprint loading bugs, and add emojis to section titles.

**Architecture:** Add `marcacao` column to `tarefas`, discover and sync Jira's flagged custom field using existing custom field pipeline, expose `status_categoria` and `marcacao` via `GetEpicTasks` → `TaskAllocation` JSON, render badges in frontend using existing `.capacity-tarefa-status` CSS pattern.

**Tech Stack:** Go/PostgreSQL backend, vanilla JS frontend (var/function only, no ES6+)

## Global Constraints

- Frontend: `var`/`function` ONLY. No `const`, `let`, arrow functions, template literals, `async/await`.
- XSS: use `esc()` for text, `escAttr()` for attributes.
- CSS custom properties: `--surface`, `--text-primary`, `--accent`, `--border`, `--text-secondary`, `--blue`, `--blue-soft`, `--accent-soft`, `--chip-bg`, `--text-tertiary`.
- Dark mode: `@media (prefers-color-scheme: dark)` + `:root[data-theme="dark"]` + `:root[data-theme="light"]`.
- DB status_categoria values are lowercase Jira keys: `done`, `indeterminate`, `new`.
- Jira flagged field is a custom field (e.g. `customfield_10021`) discovered via `DiscoverCustomFields`. Value is `[{"value":"Impediment"}]` when flagged, `nil`/absent when not.
- Never commit/push without explicit user consent.

---

### Task 1: Migration + Backend (marcacao column, GetEpicTasks fields)

**Files:**
- Create: `backend/migrations/000023_tarefa_marcacao.up.sql`
- Create: `backend/migrations/000023_tarefa_marcacao.down.sql`
- Modify: `backend/internal/repository/allocation.go:39-54` (TaskAllocationRow struct)
- Modify: `backend/internal/repository/allocation.go:253-286` (GetEpicTasks query + scan)
- Modify: `backend/internal/service/allocation.go:47-62` (TaskAllocation struct)
- Modify: `backend/internal/service/allocation.go:338-359` (taskRowToAllocation)

**Interfaces:**
- Produces: `TaskAllocationRow` gains `StatusCategoria *string` and `Marcacao bool` fields
- Produces: `TaskAllocation` gains `StatusCategoria *string json:"status_categoria"` and `Marcacao bool json:"marcacao"` fields
- Produces: `taskRowToAllocation` maps new fields through

- [ ] **Step 1: Create up migration**

Create `backend/migrations/000023_tarefa_marcacao.up.sql`:
```sql
ALTER TABLE tarefas ADD COLUMN marcacao BOOLEAN NOT NULL DEFAULT false;
```

- [ ] **Step 2: Create down migration**

Create `backend/migrations/000023_tarefa_marcacao.down.sql`:
```sql
ALTER TABLE tarefas DROP COLUMN marcacao;
```

- [ ] **Step 3: Add fields to TaskAllocationRow**

In `backend/internal/repository/allocation.go`, add two fields to `TaskAllocationRow` struct after `ResponsavelAvatar`:
```go
type TaskAllocationRow struct {
	TarefaID          uuid.UUID
	NumeroTicket      string
	Resumo            string
	Tipo              string
	TipoDemanda       *string
	Status            string
	EstimativaTempo   *int
	SprintID          *uuid.UUID
	SprintNome        *string
	SprintInicio      *time.Time
	SprintFim         *time.Time
	ResponsavelID     *uuid.UUID
	ResponsavelNome   *string
	ResponsavelAvatar *string
	StatusCategoria   *string
	Marcacao          bool
}
```

- [ ] **Step 4: Update GetEpicTasks SQL and Scan**

In `backend/internal/repository/allocation.go`, update the `GetEpicTasks` method:

Change the SELECT from:
```sql
SELECT
    t.id, t.numero_ticket, t.resumo, t.tipo, t.tipo_demanda, t.status,
    t.estimativa_tempo,
    t.sprint_id, s.nome, s.data_inicio, s.data_fim,
    t.responsavel_id, m.nome, m.avatar_url
```
to:
```sql
SELECT
    t.id, t.numero_ticket, t.resumo, t.tipo, t.tipo_demanda, t.status,
    t.estimativa_tempo,
    t.sprint_id, s.nome, s.data_inicio, s.data_fim,
    t.responsavel_id, m.nome, m.avatar_url,
    t.status_categoria, t.marcacao
```

Update the Scan call to add the two new fields at the end:
```go
if err := rows.Scan(
    &t.TarefaID, &t.NumeroTicket, &t.Resumo, &t.Tipo, &t.TipoDemanda, &t.Status,
    &t.EstimativaTempo,
    &t.SprintID, &t.SprintNome, &t.SprintInicio, &t.SprintFim,
    &t.ResponsavelID, &t.ResponsavelNome, &t.ResponsavelAvatar,
    &t.StatusCategoria, &t.Marcacao,
); err != nil {
```

- [ ] **Step 5: Add fields to TaskAllocation service struct**

In `backend/internal/service/allocation.go`, add two fields to `TaskAllocation` struct after `ResponsavelAvatar`:
```go
StatusCategoria   *string    `json:"status_categoria"`
Marcacao          bool       `json:"marcacao"`
```

- [ ] **Step 6: Update taskRowToAllocation**

In `backend/internal/service/allocation.go`, add the mappings inside `taskRowToAllocation`:
```go
func taskRowToAllocation(t repository.TaskAllocationRow) TaskAllocation {
	ta := TaskAllocation{
		TarefaID:          t.TarefaID,
		NumeroTicket:      t.NumeroTicket,
		Resumo:            t.Resumo,
		Tipo:              t.Tipo,
		TipoDemanda:       t.TipoDemanda,
		Status:            t.Status,
		SprintID:          t.SprintID,
		SprintNome:        t.SprintNome,
		SprintInicio:      t.SprintInicio,
		SprintFim:         t.SprintFim,
		ResponsavelID:     t.ResponsavelID,
		ResponsavelNome:   t.ResponsavelNome,
		ResponsavelAvatar: t.ResponsavelAvatar,
		StatusCategoria:   t.StatusCategoria,
		Marcacao:          t.Marcacao,
	}
	if t.EstimativaTempo != nil && *t.EstimativaTempo > 0 {
		h := float64(*t.EstimativaTempo) / 3600.0
		ta.EstimativaHoras = &h
	}
	return ta
}
```

- [ ] **Step 7: Run migration and verify build**

```bash
cd /home/emerson/code/myplanner/backend
migrate -path migrations -database "postgres://myplanner@localhost:5432/myplanner?sslmode=disable" up
go build ./...
```

- [ ] **Step 8: Test endpoint**

Start server, then test:
```bash
TOKEN=$(curl -s http://localhost:9091/api/v1/auth/login -H 'Content-Type: application/json' -d '{"email":"admin@myplanner.local","senha":"Totvs@123"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
# Pick an epic ID from the DB
EPIC=$(psql -h localhost -U myplanner -d myplanner -t -c "SELECT id FROM tarefas WHERE tipo IN ('Épico','Epico') LIMIT 1;" | tr -d ' ')
curl -s "http://localhost:9091/api/v1/allocation/projects/$EPIC" -H "Authorization: Bearer $TOKEN" | python3 -m json.tool | head -40
```

Verify response includes `status_categoria` and `marcacao` in task objects.

- [ ] **Step 9: Commit**

```bash
git add backend/migrations/000023_tarefa_marcacao.up.sql backend/migrations/000023_tarefa_marcacao.down.sql backend/internal/repository/allocation.go backend/internal/service/allocation.go
git commit -m "feat(allocation): add marcacao column and expose status_categoria in task API"
```

---

### Task 2: Jira Sync — import flagged field

**Files:**
- Modify: `backend/internal/jira/client.go:454-460` (knownCustomFields maps)
- Modify: `backend/internal/repository/sync.go:22-52` (UpsertTarefaParams struct)
- Modify: `backend/internal/repository/sync.go:135-176` (UpsertTarefa SQL)
- Modify: `backend/internal/service/sync.go:816-834` (custom field extraction)

**Interfaces:**
- Consumes: `extractCustomFields` already captures `customfield_*` keys into `JiraIssue.Fields.CustomFields`
- Produces: `UpsertTarefaParams.Marcacao bool` field, persisted to `tarefas.marcacao`

- [ ] **Step 1: Add flagged to custom field discovery maps**

In `backend/internal/jira/client.go`, update both maps:

```go
var knownCustomFieldsByID = map[string]string{
	"customfield_12930": "tipo_demanda",
	"customfield_10021": "flagged",
}

var knownCustomFieldsByName = map[string]string{
	"tipo de demanda": "tipo_demanda",
	"flagged":         "flagged",
}
```

- [ ] **Step 2: Add Marcacao to UpsertTarefaParams**

In `backend/internal/repository/sync.go`, add to the `UpsertTarefaParams` struct after `DataEntradaSprint`:
```go
Marcacao           bool
```

- [ ] **Step 3: Update UpsertTarefa SQL**

In `backend/internal/repository/sync.go`, update the INSERT to include `marcacao`:

In the column list (line ~146), add `marcacao` after `data_entrada_sprint`:
```
data_entrada_sprint, marcacao)
```

Add `$28` to the VALUES:
```
$16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28)
```

In the ON CONFLICT DO UPDATE, add:
```
marcacao = EXCLUDED.marcacao,
```
(add after the `data_entrada_sprint` line)

Add `t.Marcacao` to the Scan arguments at the end (after `t.DataEntradaSprint`):
```go
t.DataEntradaSprint, t.Marcacao).Scan(&id)
```

- [ ] **Step 4: Extract flagged in sync service**

In `backend/internal/service/sync.go`, after the `tipoDemanda` extraction block (around line 834), add flagged extraction:

```go
	var marcacao bool
	if fonte.CustomFieldMap != nil {
		var cfMap map[string]string
		if err := json.Unmarshal(fonte.CustomFieldMap, &cfMap); err == nil {
			for fieldID, fieldName := range cfMap {
				if fieldName == "flagged" {
					if val, ok := f.CustomFields[fieldID]; ok && val != nil {
						if arr, ok := val.([]any); ok && len(arr) > 0 {
							marcacao = true
						} else if b, ok := val.(bool); ok {
							marcacao = b
						}
					}
				}
			}
		}
	}
```

Then add `Marcacao: marcacao,` to the params struct literal (after `DataEntradaSprint`).

- [ ] **Step 5: Build and verify**

```bash
cd /home/emerson/code/myplanner/backend
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add backend/internal/jira/client.go backend/internal/repository/sync.go backend/internal/service/sync.go
git commit -m "feat(sync): import Jira flagged field as marcacao"
```

---

### Task 3: Bugfix — pct_no_projeto calculation

**Files:**
- Modify: `backend/internal/service/allocation.go:280-311` (GetProjectDetail pessoa loop)

**Interfaces:**
- Consumes: `people []PersonAllocationRow` (has `HorasNoProjeto` per person), `tasks []TaskAllocationRow` (has `EstimativaTempo` per task)
- Produces: `PersonAllocation.PctNoProjeto` now = `person_hours / total_project_hours * 100`

- [ ] **Step 1: Calculate total project hours from tasks**

In `backend/internal/service/allocation.go`, in the `GetProjectDetail` method, replace the block at lines 280-311. Remove the `GetPersonTotalAllocatedHours` call and calculate total project hours from the tasks array instead:

Replace this block:
```go
	membroIDs := make([]uuid.UUID, len(people))
	for i, p := range people {
		membroIDs[i] = p.MembroID
	}

	totalHoursMap, err := s.repo.GetPersonTotalAllocatedHours(ctx, membroIDs)
	if err != nil {
		s.logger.Warn("getting total allocated hours", zap.Error(err))
		totalHoursMap = make(map[uuid.UUID]float64)
	}

	pessoas := make([]PersonAllocation, 0, len(people))
	for _, p := range people {
		pctNoProjeto := 0.0
		totalHours := totalHoursMap[p.MembroID]
		if totalHours > 0 {
			pctNoProjeto = p.HorasNoProjeto / totalHours * 100
		}
		avatarURL := ""
		if p.AvatarURL != nil {
			avatarURL = *p.AvatarURL
		}
		pessoas = append(pessoas, PersonAllocation{
			MembroID:       p.MembroID,
			Nome:           p.Nome,
			HorasNoProjeto: p.HorasNoProjeto,
			HorasCapTotal:  totalHours,
			PctNoProjeto:   pctNoProjeto,
			AvatarURL:      avatarURL,
		})
	}
```

With:
```go
	var totalProjectHours float64
	for _, t := range tasks {
		if t.EstimativaTempo != nil && *t.EstimativaTempo > 0 {
			totalProjectHours += float64(*t.EstimativaTempo) / 3600.0
		}
	}

	pessoas := make([]PersonAllocation, 0, len(people))
	for _, p := range people {
		pctNoProjeto := 0.0
		if totalProjectHours > 0 {
			pctNoProjeto = p.HorasNoProjeto / totalProjectHours * 100
		}
		avatarURL := ""
		if p.AvatarURL != nil {
			avatarURL = *p.AvatarURL
		}
		pessoas = append(pessoas, PersonAllocation{
			MembroID:       p.MembroID,
			Nome:           p.Nome,
			HorasNoProjeto: p.HorasNoProjeto,
			HorasCapTotal:  totalProjectHours,
			PctNoProjeto:   pctNoProjeto,
			AvatarURL:      avatarURL,
		})
	}
```

- [ ] **Step 2: Build**

```bash
cd /home/emerson/code/myplanner/backend
go build ./...
```

- [ ] **Step 3: Test manually**

Start server, open a project modal. Verify that person percentages sum to ~100% (or less if some tasks are unassigned).

- [ ] **Step 4: Commit**

```bash
git add backend/internal/service/allocation.go
git commit -m "fix(allocation): calculate pct_no_projeto using project total hours, not person global hours"
```

---

### Task 4: Frontend — CSS, badges, emojis, sprint bugfix

**Files:**
- Modify: `frontend/index.html` — CSS section (~line 637-640), JS section (~lines 5948, 6024-6050, 6162-6182, 6248-6281, 6374-6412)

**Interfaces:**
- Consumes: Task JSON now includes `status_categoria` (values: `"done"`, `"indeterminate"`, `"new"` or null) and `marcacao` (boolean)
- Produces: Visual badges in modal, emoji section titles, sprint reload on equipe change

- [ ] **Step 1: Add blocked badge CSS**

In `frontend/index.html`, after line 640 (`.capacity-tarefa-status.todo` rule), add:
```css
.capacity-tarefa-status.blocked { background: rgba(220,53,69,0.15); color: #dc3545; }
```

- [ ] **Step 2: Add taskStatusBadgeHtml helper**

In `frontend/index.html`, before the `renderAllocationBoxes` function (before line ~6027), add:
```javascript
function taskStatusBadgeHtml(task) {
  if (task.marcacao) return '<span class="capacity-tarefa-status blocked">Bloqueada</span>';
  var cat = task.status_categoria || '';
  if (cat === 'done') return '<span class="capacity-tarefa-status done">Concluído</span>';
  if (cat === 'indeterminate') return '<span class="capacity-tarefa-status inprogress">Em Andamento</span>';
  return '<span class="capacity-tarefa-status todo">Backlog</span>';
}
```

Note: uses `done`/`indeterminate`/`new` (lowercase Jira status category keys stored in DB), NOT `Done`/`In Progress`.

- [ ] **Step 3: Add emojis to section titles**

In `frontend/index.html`, update the `sectionNames` array in `renderAllocationBoxes` (around line 6036):

Change:
```javascript
  var sectionNames = [
    {key: 'Meta', label: 'Metas'},
    {key: 'Compromisso', label: 'Compromissos'},
    {key: 'Iniciativa', label: 'Iniciativas'}
  ];
```
To:
```javascript
  var sectionNames = [
    {key: 'Meta', label: '🎯 Metas'},
    {key: 'Compromisso', label: '🤝 Compromissos'},
    {key: 'Iniciativa', label: '⬆️ Iniciativas'}
  ];
```

(These are Unicode escapes for 🎯, 🤝, ⬆️ — avoids encoding issues.)

- [ ] **Step 4: Add badge to renderAllocTaskEditable**

In `frontend/index.html`, in the `renderAllocTaskEditable` function (around line 6380), after the ticket span:
```javascript
  html += '<span class="alloc-task-ticket">' + esc(t.numero_ticket) + '</span>';
```
Add the badge right after:
```javascript
  html += taskStatusBadgeHtml(t);
```

- [ ] **Step 5: Add badge to Tarefas Planejadas (read-only)**

In `frontend/index.html`, in the `renderAllocModal` function, in the planned tasks loop (around line 6270), after the ticket span:
```javascript
      html += '<span class="alloc-task-ticket">' + esc(t.numero_ticket) + '</span>';
```
Add:
```javascript
      html += taskStatusBadgeHtml(t);
```

- [ ] **Step 6: Fix sprint loading — reset on equipe change**

In `frontend/index.html`, add a variable to track the last equipe used for sprints. Near the `allocSprints` declaration (line ~6162):

Change:
```javascript
var allocSprints = [];
```
To:
```javascript
var allocSprints = [];
var allocSprintsEquipeId = '';
```

- [ ] **Step 7: Reset allocSprints in onAllocFilterChange**

In `frontend/index.html`, in `onAllocFilterChange` (around line 5948), after `allocEquipeId` is set, add sprint reset:

Change:
```javascript
function onAllocFilterChange() {
  var rawEquipe = document.getElementById('alloc-equipe').value;
  allocEquipeId = rawEquipe === 'todas' ? '' : rawEquipe;
  allocProdutoId = document.getElementById('alloc-produto').value;
```
To:
```javascript
function onAllocFilterChange() {
  var rawEquipe = document.getElementById('alloc-equipe').value;
  var newEquipeId = rawEquipe === 'todas' ? '' : rawEquipe;
  if (newEquipeId !== allocEquipeId) {
    allocSprints = [];
    allocSprintsEquipeId = '';
  }
  allocEquipeId = newEquipeId;
  allocProdutoId = document.getElementById('alloc-produto').value;
```

- [ ] **Step 8: Fix openProjectModal to always reload sprints when equipe changed**

In `frontend/index.html`, update the `openProjectModal` function (around line 6174):

Change:
```javascript
  if (allocSprints.length === 0 && allocEquipeId) {
    api('/allocation/sprints?equipe_id=' + allocEquipeId).then(function(sprints) {
      allocSprints = sprints || [];
      fetchAndRenderModal(epicId);
    });
  } else {
    fetchAndRenderModal(epicId);
  }
```
To:
```javascript
  var needSprints = allocEquipeId && (allocSprints.length === 0 || allocSprintsEquipeId !== allocEquipeId);
  if (needSprints) {
    api('/allocation/sprints?equipe_id=' + allocEquipeId).then(function(sprints) {
      allocSprints = sprints || [];
      allocSprintsEquipeId = allocEquipeId;
      fetchAndRenderModal(epicId);
    });
  } else {
    fetchAndRenderModal(epicId);
  }
```

- [ ] **Step 9: Test in browser**

1. Open Alocacao tab, select equipe + product, open a project
2. Verify badges appear on all 3 task sections (Nao Alocadas, Parciais, Planejadas)
3. Verify section titles show emojis: 🎯 Metas, 🤝 Compromissos, ⬆️ Iniciativas
4. Change equipe, open a project — verify sprint dropdown shows sprints from new equipe
5. Check dark mode — verify badge colors are readable

- [ ] **Step 10: Commit**

```bash
git add frontend/index.html
git commit -m "feat(allocation): add task status badges, emoji section titles, fix sprint loading per equipe"
```
