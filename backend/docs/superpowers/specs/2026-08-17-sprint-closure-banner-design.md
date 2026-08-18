# Sprint Closure Banner — Design Spec

## Goal

When a sprint transitions from "active" to "closed" during Jira sync, show an informational banner at the top of the Sprints tab notifying the user and linking directly to the Sprint Review page (which displays the snapshot captured at closure time).

## Architecture

Frontend-only detection using `data_conclusao` (already in DB, not yet in listing response). No new tables, endpoints, or migrations. Banner visibility controlled by a 3-calendar-day window + localStorage dismissal.

## Scope

- Add `data_conclusao` to `SprintListItem` struct and SQL query
- New CSS banner component (light/dark theme)
- Detection logic in `loadSprints()`: closed sprint + recent `data_conclusao` + not dismissed
- Dismiss via localStorage per sprint ID
- Link navigates to sprint detail → Review tab

## Global Constraints

- Monolithic vanilla JS frontend (`frontend/index.html`)
- Go backend with chi router, pgx, zap logger
- `var` for globals, `function` declarations (no ES6 modules)
- CSS custom properties for theming (light/dark)
- All user-facing text in Portuguese
- No commits without explicit user consent

---

## Backend

### SprintListItem Change

Add `DataConclusao` to the existing struct in `repository/sprint.go`:

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

### SQL Change

In `listSprints()`, add `s.data_conclusao` to the SELECT clause and to `rows.Scan()`:

```sql
SELECT s.id, s.nome, s.estado, s.data_inicio, s.data_fim, s.data_conclusao,
       (SELECT COUNT(*) FROM tarefas t WHERE t.sprint_id = s.id) AS total_tarefas,
       p.chave, p.nome, s.fonte_dados_id, s.projeto_id
FROM sprints s
...
```

No new endpoints, no migrations.

---

## Frontend

### Banner CSS

New class `sprint-closure-banner`, following the existing `unplanned-disclaimer` pattern:

```css
.sprint-closure-banner {
  background: #e3f2fd;
  border-left: 4px solid #1976d2;
  border-radius: 6px;
  padding: 12px 16px;
  margin-bottom: 12px;
  font-size: 13px;
  color: #1565c0;
  line-height: 1.5;
  display: flex;
  align-items: center;
  gap: 10px;
}
.sprint-closure-banner-icon { font-size: 18px; flex-shrink: 0; }
.sprint-closure-banner-text { flex: 1; }
.sprint-closure-banner-text a {
  color: inherit;
  font-weight: 600;
  text-decoration: underline;
  cursor: pointer;
}
.sprint-closure-banner-close {
  background: none;
  border: none;
  cursor: pointer;
  font-size: 16px;
  color: inherit;
  opacity: 0.6;
  padding: 4px;
}
.sprint-closure-banner-close:hover { opacity: 1; }
```

Dark theme variants:
```css
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) .sprint-closure-banner {
    background: #0d47a120;
    border-left-color: #42a5f5;
    color: #90caf9;
  }
}
:root[data-theme="dark"] .sprint-closure-banner {
  background: #0d47a120;
  border-left-color: #42a5f5;
  color: #90caf9;
}
```

### Detection Logic

In `loadSprints()`, after building the sprint list HTML, before setting `el.innerHTML`:

```javascript
var now = new Date();
var threeDaysAgo = new Date(now.getTime() - 3 * 24 * 60 * 60 * 1000);
var bannerHtml = '';

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
    '<button class="sprint-closure-banner-close" onclick="dismissSprintClosure(\'' + s.id + '\')" title="Fechar">✕</button>' +
  '</div>';
});
```

Prepend `bannerHtml` before the sprint list in `el.innerHTML`.

### Helper Functions

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

### Multiple Closures

If multiple sprints closed within the 3-day window, each gets its own banner. They stack vertically above the sprint list.

---

## Error Handling

- `data_conclusao` is null for old sprints: filtered out by the null check
- localStorage unavailable (private browsing): banner shows every time (graceful degradation)
- `openSprintCapacity` fails: standard error handling in that function (shows error in content area)

---

## Testing

- Backend: verify `data_conclusao` appears in `GET /sprints` response for closed sprints
- Frontend: manually close a sprint in Jira, run sync, verify banner appears
- Dismiss: click X, reload page, verify banner stays dismissed
- Expiry: verify banner does not appear for sprints closed > 3 days ago
- Review link: click "Clique aqui", verify it opens sprint detail on Review tab with snapshot data
