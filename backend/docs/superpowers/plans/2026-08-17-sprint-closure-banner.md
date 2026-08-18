# Sprint Closure Banner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show an informational banner on the Sprints tab when a sprint was recently closed, linking directly to the Sprint Review page.

**Architecture:** Add `data_conclusao` to the existing `SprintListItem` struct/query (no new endpoints or migrations). Frontend detects recently closed sprints from the listing response, renders a dismissible banner (localStorage), with a link that opens the sprint detail directly on the Review tab.

**Tech Stack:** Go (chi, pgx), vanilla JS, CSS

## Global Constraints

- Monolithic vanilla JS frontend (`frontend/index.html`)
- Go backend with chi router, pgx, zap logger
- `var` for globals, `function` declarations (no ES6 modules)
- CSS custom properties for theming (light/dark)
- All user-facing text in Portuguese
- No commits without explicit user consent
- Changes applied directly to main at `/home/emerson/code/myplanner/` via python/cp (worktree isolation)

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `backend/internal/repository/sprint.go` | Modify | Add `DataConclusao` to struct + SQL + Scan |
| `frontend/index.html` | Modify | Banner CSS + detection logic + dismiss + review link |

---

### Task 1: Add `data_conclusao` to SprintListItem

**Files:**
- Modify: `backend/internal/repository/sprint.go:26-37` (struct), `133-136` (SQL), `200` (Scan)

**Interfaces:**
- Consumes: existing `sprints.data_conclusao` column in DB
- Produces: `SprintListItem.DataConclusao *time.Time` serialized as `"data_conclusao"` in JSON response

- [ ] **Step 1: Add field to SprintListItem struct**

In `backend/internal/repository/sprint.go`, add `DataConclusao` after `DataFim` (line 31):

```go
type SprintListItem struct {
	ID            uuid.UUID  `json:"id"`
	Nome          string     `json:"nome"`
	Estado        *string    `json:"estado"`
	DataInicio    *time.Time `json:"data_inicio"`
	DataFim       *time.Time `json:"data_fim"`
	DataConclusao *time.Time `json:"data_conclusao"`
	TotalTarefas  int        `json:"total_tarefas"`
	ProjetoChave  *string    `json:"projeto_chave,omitempty"`
	ProjetoNome   *string    `json:"projeto_nome,omitempty"`
	FonteDadosID  *uuid.UUID `json:"fonte_dados_id,omitempty"`
	ProjetoID     *uuid.UUID `json:"projeto_id,omitempty"`
}
```

- [ ] **Step 2: Add `s.data_conclusao` to SQL SELECT in `listSprints()`**

In `backend/internal/repository/sprint.go`, modify the query at line 133-136:

```go
query := `
    SELECT s.id, s.nome, s.estado, s.data_inicio, s.data_fim, s.data_conclusao,
           (SELECT COUNT(*) FROM tarefas t WHERE t.sprint_id = s.id) AS total_tarefas,
           p.chave, p.nome, s.fonte_dados_id, s.projeto_id
    FROM sprints s
    INNER JOIN projetos p ON p.id = s.projeto_id
    WHERE 1=1
`
```

- [ ] **Step 3: Add `&item.DataConclusao` to Scan in `listSprints()`**

In `backend/internal/repository/sprint.go`, modify the Scan call at line 200:

```go
if err := rows.Scan(&item.ID, &item.Nome, &item.Estado, &item.DataInicio, &item.DataFim, &item.DataConclusao, &item.TotalTarefas, &item.ProjetoChave, &item.ProjetoNome, &item.FonteDadosID, &item.ProjetoID); err != nil {
```

- [ ] **Step 4: Verify build passes**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: BUILD OK

Note: `ListByProjeto()` (line 87) has its own query and Scan that do NOT include `data_conclusao`. This is fine — the struct field will be `nil` for that code path. Only `listSprints()` (used by the Sprints tab API) needs the field populated.

---

### Task 2: Frontend — Banner CSS + Detection + Dismiss + Review Link

**Files:**
- Modify: `frontend/index.html`
  - CSS insertion after line 475 (after `:root[data-theme="light"] .unplanned-badge`)
  - JS modification in `loadSprints()` at lines 3665-3678
  - JS new functions before `// === INIT ===` at line 10427

**Interfaces:**
- Consumes: `SprintListItem.data_conclusao` from `GET /sprints` response (Task 1)
- Consumes: existing `openSprintCapacity(sprintId)` and `showSprintTab(tabName)` functions
- Produces: `openSprintReview(sprintId)` — opens sprint detail directly on Review tab
- Produces: `dismissSprintClosure(sprintId)` — removes banner and stores dismiss in localStorage

- [ ] **Step 1: Add banner CSS**

Insert after line 475 (`:root[data-theme="light"] .unplanned-badge { ... }`):

```css
.sprint-closure-banner { background: #e3f2fd; border-left: 4px solid #1976d2; border-radius: 6px; padding: 12px 16px; margin-bottom: 12px; font-size: 13px; color: #1565c0; line-height: 1.5; display: flex; align-items: center; gap: 10px; }
.sprint-closure-banner-icon { font-size: 18px; flex-shrink: 0; }
.sprint-closure-banner-text { flex: 1; }
.sprint-closure-banner-text a { color: inherit; font-weight: 600; text-decoration: underline; cursor: pointer; }
.sprint-closure-banner-close { background: none; border: none; cursor: pointer; font-size: 16px; color: inherit; opacity: 0.6; padding: 4px; }
.sprint-closure-banner-close:hover { opacity: 1; }
```

- [ ] **Step 2: Add dark theme CSS**

Insert immediately after the light-mode banner CSS:

```css
@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) .sprint-closure-banner { background: #0d47a120; border-left-color: #42a5f5; color: #90caf9; } }
:root[data-theme="dark"] .sprint-closure-banner { background: #0d47a120; border-left-color: #42a5f5; color: #90caf9; }
:root[data-theme="light"] .sprint-closure-banner { background: #e3f2fd; border-left-color: #1976d2; color: #1565c0; }
```

- [ ] **Step 3: Add banner detection logic in `loadSprints()`**

Modify `loadSprints()` — replace the block at lines 3665-3678 (from `let html = '<div class="sprint-list">';` through `el.innerHTML = html;`) with:

```javascript
    var bannerHtml = '';
    var now = new Date();
    var threeDaysAgo = new Date(now.getTime() - 3 * 24 * 60 * 60 * 1000);
    var selEquipe = document.getElementById('sprints-equipe');
    var equipeName = selEquipe.options[selEquipe.selectedIndex].text;
    sprints.forEach(function(s) {
      if (s.estado !== 'closed' || !s.data_conclusao) return;
      var closedAt = new Date(s.data_conclusao);
      if (closedAt < threeDaysAgo) return;
      if (localStorage.getItem('dismissed-sprint-closure-' + s.id)) return;
      var diasSemana = ['domingo','segunda-feira','terça-feira','quarta-feira','quinta-feira','sexta-feira','sábado'];
      var diaSemana = diasSemana[closedAt.getDay()];
      var dia = closedAt.getDate();
      bannerHtml += '<div class="sprint-closure-banner" id="sprint-closure-' + s.id + '">' +
        '<span class="sprint-closure-banner-icon">📋</span>' +
        '<span class="sprint-closure-banner-text">' +
          'A sprint "' + esc(s.nome) + '" do time "' + esc(equipeName) + '" foi encerrada na ' +
          diaSemana + ' (dia ' + dia + '). ' +
          '<a onclick="openSprintReview(\'' + s.id + '\')">Clique aqui para visualizar a Review</a>' +
        '</span>' +
        '<button class="sprint-closure-banner-close" onclick="event.stopPropagation();dismissSprintClosure(\'' + s.id + '\')" title="Fechar">✕</button>' +
      '</div>';
    });
    let html = bannerHtml + '<div class="sprint-list">';
    sprints.forEach(s => {
      const estado = s.estado || 'future';
      const dates = formatSprintDates(s.data_inicio, s.data_fim);
      const projeto = s.projeto_chave ? '<span style="color:var(--text-tertiary);font-size:11px">' + esc(s.projeto_chave) + '</span> ' : '';
      html += '<div class="sprint-item" id="sprint-item-' + s.id + '" onclick="openSprintCapacity(\'' + s.id + '\')">' +
        '<div class="sprint-dot ' + estado + '"></div>' +
        '<div class="sprint-info"><div class="sprint-name">' + projeto + esc(s.nome) + '</div><div class="sprint-dates">' + dates + '</div>' +
        '<div class="sprint-ausencias-preview" id="sprint-aus-' + s.id + '"></div></div>' +
        '<div class="sprint-meta"><span class="sprint-state-badge ' + estado + '">' + formatEstado(estado) + '</span><br><span>' + s.total_tarefas + ' tarefas</span></div>' +
      '</div>';
    });
    html += '</div>';
    el.innerHTML = html;
```

Note: the `event.stopPropagation()` on the close button prevents any parent click handlers from firing.

- [ ] **Step 4: Add helper functions**

Insert before `// === INIT ===` (line 10427):

```javascript
async function openSprintReview(sprintId) {
  await openSprintCapacity(sprintId);
  showSprintTab('review');
}

function dismissSprintClosure(sprintId) {
  localStorage.setItem('dismissed-sprint-closure-' + sprintId, 'true');
  var el = document.getElementById('sprint-closure-' + sprintId);
  if (el) el.remove();
}
```

- [ ] **Step 5: Verify build still passes**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: BUILD OK (frontend is static HTML, no build step)

- [ ] **Step 6: Manual test checklist**

1. Run `./dev.sh` and open Sprints tab
2. Select an equipe that has a recently closed sprint (closed within 3 days)
3. Verify banner appears above sprint list with correct sprint name, team name, day of week, and date
4. Verify "Clique aqui para visualizar a Review" link is underlined and clickable
5. Click the link — verify it opens sprint detail directly on the Review tab
6. Navigate back to sprint list, verify banner still shows
7. Click the ✕ button — verify banner disappears
8. Reload page — verify dismissed banner does NOT reappear
9. Switch to dark mode — verify banner colors adapt
10. Select an equipe with no recently closed sprints — verify no banner appears
