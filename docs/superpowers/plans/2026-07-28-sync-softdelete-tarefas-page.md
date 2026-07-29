# Sync Soft-Delete + Tarefas Page + Projetos Filters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect Jira-deleted cards during full sync (soft-delete), build a Tarefas guardrail page with filters and hard-delete, and add filters to Projetos page.

**Architecture:** Add `removido_em`/`motivo_remocao` columns to `tarefas`. After full sync processes all issues for a fonte, compare returned jira_ids against DB — mark absent ones as removed. New `/tarefas` endpoint with pagination + filters. New frontend page with table, filters, and delete action. Extend Projetos page with matching filters. Add `AND removido_em IS NULL` to existing allocation/timeline queries.

**Tech Stack:** Go/chi/pgx backend, PostgreSQL, vanilla JS frontend

## Global Constraints

- Frontend: `var`/`function` ONLY. No `const`, `let`, arrow functions, template literals, `async/await`.
- XSS: use `esc()` for text, `escAttr()` for attributes.
- CSS custom properties: `--surface`, `--text-primary`, `--accent`, `--border`, `--text-secondary`, `--blue`, `--blue-soft`, `--accent-soft`, `--chip-bg`, `--text-tertiary`, `--red`, `--red-soft`.
- Dark mode: `@media (prefers-color-scheme: dark)` + `:root[data-theme="dark"]` + `:root[data-theme="light"]`.
- Never commit/push without explicit user consent.
- Hard delete only allowed on tarefas where `removido_em IS NOT NULL`.
- FK cascades: `tarefa_produtos` and `epico_equipes` have `ON DELETE CASCADE`. `projeto_encerramentos` has NO cascade — must delete manually before hard-deleting an épico.

---

### Task 1: Migration + Sync Soft-Delete Logic

**Files:**
- Create: `backend/migrations/000024_tarefa_removido.up.sql`
- Create: `backend/migrations/000024_tarefa_removido.down.sql`
- Modify: `backend/internal/repository/sync.go` (add `SoftDeleteAbsentTarefas` and `UndeleteReappearedTarefas` methods)
- Modify: `backend/internal/service/sync.go:384-502` (add soft-delete call in `executSync`)

**Interfaces:**
- Produces: `SoftDeleteAbsentTarefas(ctx, fonteDadosID uuid.UUID, jiraIDs []string) (int64, error)` — marks tarefas not in jiraIDs as removed
- Produces: `UndeleteReappearedTarefas(ctx, fonteDadosID uuid.UUID, jiraIDs []string) (int64, error)` — clears removido_em for tarefas that reappeared
- Produces: `removido_em` and `motivo_remocao` columns on `tarefas` table

- [ ] **Step 1: Create up migration**

Create `backend/migrations/000024_tarefa_removido.up.sql`:
```sql
ALTER TABLE tarefas ADD COLUMN removido_em TIMESTAMPTZ NULL;
ALTER TABLE tarefas ADD COLUMN motivo_remocao TEXT NULL;
CREATE INDEX idx_tarefas_removido ON tarefas(removido_em) WHERE removido_em IS NOT NULL;
```

- [ ] **Step 2: Create down migration**

Create `backend/migrations/000024_tarefa_removido.down.sql`:
```sql
DROP INDEX IF EXISTS idx_tarefas_removido;
ALTER TABLE tarefas DROP COLUMN motivo_remocao;
ALTER TABLE tarefas DROP COLUMN removido_em;
```

- [ ] **Step 3: Add SoftDeleteAbsentTarefas to sync repository**

In `backend/internal/repository/sync.go`, add after the `UpsertTarefa` method:

```go
func (r *SyncRepository) SoftDeleteAbsentTarefas(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error) {
	if len(presentJiraIDs) == 0 {
		return 0, nil
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE tarefas
		SET removido_em = NOW(), motivo_remocao = 'removido do jira'
		WHERE fonte_dados_id = $1
		  AND jira_id != ALL($2)
		  AND removido_em IS NULL
	`, fonteDadosID, presentJiraIDs)
	if err != nil {
		return 0, fmt.Errorf("soft-deleting absent tarefas: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *SyncRepository) UndeleteReappearedTarefas(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error) {
	if len(presentJiraIDs) == 0 {
		return 0, nil
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE tarefas
		SET removido_em = NULL, motivo_remocao = NULL
		WHERE fonte_dados_id = $1
		  AND jira_id = ANY($2)
		  AND removido_em IS NOT NULL
	`, fonteDadosID, presentJiraIDs)
	if err != nil {
		return 0, fmt.Errorf("undeleting reappeared tarefas: %w", err)
	}
	return tag.RowsAffected(), nil
}
```

- [ ] **Step 4: Add soft-delete call in executSync**

In `backend/internal/service/sync.go`, in `executSync`, after the main issue processing loop (after line 492 `}`), before `totals.Projetos = len(projectCache)`, add:

```go
	// Collect all jira_ids that came from the Jira response
	allJiraIDs := make([]string, 0, len(issues))
	for _, issue := range issues {
		allJiraIDs = append(allJiraIDs, issue.ID)
	}

	// Undelete any tarefas that reappeared
	if undeleted, err := s.repo.UndeleteReappearedTarefas(ctx, fonte.ID, allJiraIDs); err != nil {
		s.logger.Warn("failed to undelete reappeared tarefas", zap.Error(err))
	} else if undeleted > 0 {
		s.logger.Info("reappeared tarefas undeleted", zap.Int64("count", undeleted))
	}

	// Soft-delete tarefas that were not in the Jira response
	if softDeleted, err := s.repo.SoftDeleteAbsentTarefas(ctx, fonte.ID, allJiraIDs); err != nil {
		s.logger.Warn("failed to soft-delete absent tarefas", zap.Error(err))
	} else if softDeleted > 0 {
		s.logger.Info("absent tarefas soft-deleted", zap.Int64("count", softDeleted))
		totals.Removidos = int(softDeleted)
	}
```

- [ ] **Step 5: Add Removidos field to SyncTotals**

In `backend/internal/repository/sync.go`, find the `SyncTotals` struct and add:
```go
Removidos int
```

- [ ] **Step 6: Run migration and build**

```bash
cd /home/emerson/code/myplanner/backend
migrate -path migrations -database "postgres://myplanner@localhost:5432/myplanner?sslmode=disable" up
go build ./...
```

- [ ] **Step 7: Test by running a full sync**

Trigger a sync from the Fontes de Dados page. Check backend logs for `absent tarefas soft-deleted` or `reappeared tarefas undeleted` messages.

```bash
psql -h localhost -U myplanner -d myplanner -c "SELECT COUNT(*) FROM tarefas WHERE removido_em IS NOT NULL;"
```

---

### Task 2: Filter removidos from existing queries

**Files:**
- Modify: `backend/internal/repository/allocation.go:144-161` (GetEpicsByEquipeAndProduto WHERE clause)
- Modify: `backend/internal/repository/allocation.go:260-273` (GetEpicTasks WHERE clause)
- Modify: `backend/internal/repository/allocation.go:297-320` (GetEpicPeople WHERE subquery)
- Modify: `backend/internal/repository/timeline.go:374-420` (ListarEpicos — 3 query branches)

**Interfaces:**
- Consumes: `removido_em` column from Task 1
- Produces: All existing UI queries exclude soft-deleted tarefas by default

- [ ] **Step 1: Add filter to GetEpicsByEquipeAndProduto**

In `backend/internal/repository/allocation.go`, in the WHERE clause of `GetEpicsByEquipeAndProduto`, after line 160 (`AND e.status NOT IN ('Cancelado', 'Rejeitada', 'Concluído')`), before the `statusClause` concatenation, add:

```sql
			  AND e.removido_em IS NULL
```

Also filter child tasks in the product EXISTS subquery (line 155-159). Change:
```sql
			WHERE c.parent_id = e.id AND LOWER(p2.nome) = ANY($2)
```
To:
```sql
			WHERE c.parent_id = e.id AND LOWER(p2.nome) = ANY($2) AND c.removido_em IS NULL
```

And filter child tasks in the count/sum subqueries (lines 126-140). Each `WHERE c.parent_id = e.id` subquery needs `AND c.removido_em IS NULL` appended.

- [ ] **Step 2: Add filter to GetEpicTasks**

In `backend/internal/repository/allocation.go`, in `GetEpicTasks`, change:
```sql
		WHERE t.parent_id = $1
		  AND t.status NOT IN ('Cancelado', 'Rejeitada')
```
To:
```sql
		WHERE t.parent_id = $1
		  AND t.status NOT IN ('Cancelado', 'Rejeitada')
		  AND t.removido_em IS NULL
```

- [ ] **Step 3: Add filter to GetEpicPeople**

In `backend/internal/repository/allocation.go`, in `GetEpicPeople`, the query selects from `tarefas t WHERE t.parent_id = $1`. Add `AND t.removido_em IS NULL` to that WHERE clause.

- [ ] **Step 4: Add filter to ListarEpicos (3 branches)**

In `backend/internal/repository/timeline.go`, `ListarEpicos` has 3 query branches (lines 385, 404, 413). In each, add `AND e.removido_em IS NULL` to the WHERE clause, after `WHERE e.tipo = 'Épico'`.

Branch 1 (line 389):
```sql
WHERE e.tipo = 'Épico'
  AND e.removido_em IS NULL
  AND (
```

Branch 2 (line 408):
```sql
WHERE e.tipo = 'Épico'
  AND e.removido_em IS NULL
```

Branch 3 (line 417):
```sql
WHERE e.tipo = 'Épico'
  AND e.removido_em IS NULL
```

- [ ] **Step 5: Build and verify**

```bash
cd /home/emerson/code/myplanner/backend
go build ./...
```

---

### Task 3: Tarefas listing endpoint + hard-delete endpoint

**Files:**
- Create: `backend/internal/repository/tarefa.go`
- Create: `backend/internal/handler/tarefa.go`
- Modify: `backend/cmd/api/main.go` (register routes)

**Interfaces:**
- Consumes: `tarefas` table with `removido_em` column from Task 1
- Produces: `GET /api/v1/tarefas` — paginated listing with filters
- Produces: `DELETE /api/v1/tarefas/{id}` — hard delete (only if removido_em IS NOT NULL)

- [ ] **Step 1: Create tarefa repository**

Create `backend/internal/repository/tarefa.go`:

```go
package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TarefaRepository struct {
	pool *pgxpool.Pool
}

func NewTarefaRepository(pool *pgxpool.Pool) *TarefaRepository {
	return &TarefaRepository{pool: pool}
}

type TarefaListRow struct {
	ID              uuid.UUID
	NumeroTicket    string
	Resumo          string
	Tipo            string
	Status          string
	TipoDemanda     *string
	ResponsavelNome *string
	ProdutoNome     *string
	EquipeNome      *string
	RemovidoEm      *time.Time
	MotivoRemocao   *string
	UpdatedAt       time.Time
}

type TarefaListFilter struct {
	EquipeID      *uuid.UUID
	ProdutoNome   *string
	ResponsavelID *uuid.UUID
	Removido      string // "sim", "nao", "todos"
	Busca         string
	Page          int
	PerPage       int
}

type TarefaListResult struct {
	Items []TarefaListRow
	Total int
}

func (r *TarefaRepository) ListTarefas(ctx context.Context, f TarefaListFilter) (*TarefaListResult, error) {
	var conditions []string
	var args []any
	argN := 1

	conditions = append(conditions, "t.tipo NOT IN ('Épico', 'Epico')")

	switch f.Removido {
	case "sim":
		conditions = append(conditions, "t.removido_em IS NOT NULL")
	case "todos":
		// no filter
	default:
		conditions = append(conditions, "t.removido_em IS NULL")
	}

	if f.EquipeID != nil {
		conditions = append(conditions, fmt.Sprintf(`t.responsavel_id IN (SELECT membro_id FROM equipe_membros WHERE equipe_id = $%d)`, argN))
		args = append(args, *f.EquipeID)
		argN++
	}

	if f.ProdutoNome != nil {
		conditions = append(conditions, fmt.Sprintf(`EXISTS (SELECT 1 FROM tarefa_produtos tp JOIN produtos p ON p.id = tp.produto_id WHERE tp.tarefa_id = t.id AND LOWER(p.nome) = LOWER($%d))`, argN))
		args = append(args, *f.ProdutoNome)
		argN++
	}

	if f.ResponsavelID != nil {
		conditions = append(conditions, fmt.Sprintf("t.responsavel_id = $%d", argN))
		args = append(args, *f.ResponsavelID)
		argN++
	}

	if f.Busca != "" {
		conditions = append(conditions, fmt.Sprintf("(t.numero_ticket ILIKE $%d OR t.resumo ILIKE $%d)", argN, argN))
		args = append(args, "%"+f.Busca+"%")
		argN++
	}

	where := "WHERE " + strings.Join(conditions, " AND ")

	countQuery := "SELECT COUNT(*) FROM tarefas t " + where
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting tarefas: %w", err)
	}

	offset := (f.Page - 1) * f.PerPage
	args = append(args, f.PerPage, offset)

	dataQuery := fmt.Sprintf(`
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.status, t.tipo_demanda,
		       m.nome,
		       (SELECT p.nome FROM tarefa_produtos tp JOIN produtos p ON p.id = tp.produto_id WHERE tp.tarefa_id = t.id LIMIT 1),
		       (SELECT eq.nome FROM equipe_membros em JOIN equipes eq ON eq.id = em.equipe_id WHERE em.membro_id = t.responsavel_id LIMIT 1),
		       t.removido_em, t.motivo_remocao, t.updated_at
		FROM tarefas t
		LEFT JOIN membros m ON m.id = t.responsavel_id
		%s
		ORDER BY t.updated_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argN, argN+1)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("listing tarefas: %w", err)
	}
	defer rows.Close()

	items := make([]TarefaListRow, 0)
	for rows.Next() {
		var row TarefaListRow
		if err := rows.Scan(
			&row.ID, &row.NumeroTicket, &row.Resumo, &row.Tipo, &row.Status, &row.TipoDemanda,
			&row.ResponsavelNome, &row.ProdutoNome, &row.EquipeNome,
			&row.RemovidoEm, &row.MotivoRemocao, &row.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning tarefa: %w", err)
		}
		items = append(items, row)
	}

	return &TarefaListResult{Items: items, Total: total}, rows.Err()
}

func (r *TarefaRepository) HardDeleteTarefa(ctx context.Context, id uuid.UUID) error {
	// Delete from projeto_encerramentos first (no cascade FK)
	_, _ = r.pool.Exec(ctx, `DELETE FROM projeto_encerramentos WHERE epic_id = $1`, id)
	tag, err := r.pool.Exec(ctx, `DELETE FROM tarefas WHERE id = $1 AND removido_em IS NOT NULL`, id)
	if err != nil {
		return fmt.Errorf("hard-deleting tarefa: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("tarefa not found or not marked as removed")
	}
	return nil
}
```

- [ ] **Step 2: Create tarefa handler**

Create `backend/internal/handler/tarefa.go`:

```go
package handler

import (
	"net/http"
	"strconv"

	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type TarefaHandler struct {
	repo   *repository.TarefaRepository
	logger *zap.Logger
}

func NewTarefaHandler(repo *repository.TarefaRepository, logger *zap.Logger) *TarefaHandler {
	return &TarefaHandler{repo: repo, logger: logger}
}

func (h *TarefaHandler) ListTarefas(w http.ResponseWriter, r *http.Request) {
	f := repository.TarefaListFilter{
		Removido: r.URL.Query().Get("removido"),
		Busca:    r.URL.Query().Get("busca"),
		Page:     1,
		PerPage:  50,
	}
	if f.Removido == "" {
		f.Removido = "nao"
	}

	if v := r.URL.Query().Get("equipe_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid equipe_id")
			return
		}
		f.EquipeID = &id
	}

	if v := r.URL.Query().Get("produto_nome"); v != "" {
		f.ProdutoNome = &v
	}

	if v := r.URL.Query().Get("responsavel_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid responsavel_id")
			return
		}
		f.ResponsavelID = &id
	}

	if v := r.URL.Query().Get("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			f.Page = p
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if pp, err := strconv.Atoi(v); err == nil && pp > 0 && pp <= 100 {
			f.PerPage = pp
		}
	}

	result, err := h.repo.ListTarefas(r.Context(), f)
	if err != nil {
		h.logger.Error("listing tarefas", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar tarefas")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"items": result.Items,
		"total": result.Total,
		"page":  f.Page,
		"per_page": f.PerPage,
	})
}

func (h *TarefaHandler) DeleteTarefa(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.repo.HardDeleteTarefa(r.Context(), id); err != nil {
		h.logger.Error("hard-deleting tarefa", zap.Error(err))
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
```

- [ ] **Step 3: Register routes in main.go**

In `backend/cmd/api/main.go`, after the allocation handler instantiation block, add:

```go
tarefaRepo := repository.NewTarefaRepository(pool)
tarefaHandler := handler.NewTarefaHandler(tarefaRepo, logger)
```

Inside the authenticated route group, add:
```go
r.Get("/tarefas", tarefaHandler.ListTarefas)
r.Delete("/tarefas/{id}", tarefaHandler.DeleteTarefa)
```

- [ ] **Step 4: Build and test endpoint**

```bash
cd /home/emerson/code/myplanner/backend
go build ./...
```

Test:
```bash
TOKEN=$(curl -s http://localhost:9091/api/v1/auth/login -H 'Content-Type: application/json' -d '{"email":"admin@myplanner.local","senha":"Totvs@123"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
curl -s "http://localhost:9091/api/v1/tarefas?removido=todos&per_page=5" -H "Authorization: Bearer $TOKEN" | python3 -m json.tool | head -20
```

---

### Task 4: Frontend — Tarefas page (sidebar, HTML, JS, CSS)

**Files:**
- Modify: `frontend/index.html` — sidebar HTML (~line 924-932), page div section (~line 1071), CSS section, JS section, navigate() function (~line 1799-1815)

**Interfaces:**
- Consumes: `GET /api/v1/tarefas` with query params from Task 3
- Consumes: `DELETE /api/v1/tarefas/{id}` from Task 3
- Consumes: `GET /api/v1/equipes` (existing)
- Consumes: `GET /api/v1/allocation/products` (existing)
- Consumes: `GET /api/v1/membros` (existing, if available — check handler)
- Produces: New sidebar item "Tarefas" inside "Projetos" group
- Produces: New page `#page-tarefas` with table, filters, pagination, delete action

- [ ] **Step 1: Add Tarefas sidebar item**

In `frontend/index.html`, inside the `sidebar-group-items` div for the Projetos group (after the Alocacao button, around line 931), add:

```html
    <button class="sidebar-item" data-page="tarefas" title="Tarefas" onclick="navigate('tarefas')">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"><path d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" stroke-width="2"/></svg>
      <span class="sidebar-item-label">Tarefas</span>
    </button>
```

- [ ] **Step 2: Update navigate() to include tarefas**

In `frontend/index.html`, in the `navigate` function (~line 1808), update the `projPages` array:

Change:
```javascript
  var projPages = ['projetos', 'alocacao'];
```
To:
```javascript
  var projPages = ['projetos', 'alocacao', 'tarefas'];
```

And add the load dispatch (after the `if (page === 'alocacao')` line):
```javascript
  if (page === 'tarefas') loadTarefas();
```

- [ ] **Step 3: Add page HTML**

In `frontend/index.html`, after the `page-alocacao` div closing tag (after `</div>` around line 1077), add:

```html
    <!-- TAREFAS -->
    <div class="page" id="page-tarefas">
      <div class="page-header-row">
        <h1 class="page-title">Tarefas</h1>
      </div>
      <div class="timeline-filters">
        <select class="filter-select" id="tarefas-equipe" onchange="onTarefaFilterChange()">
          <option value="">Todas as Equipes</option>
        </select>
        <select class="filter-select" id="tarefas-produto" onchange="onTarefaFilterChange()">
          <option value="">Todos os Produtos</option>
        </select>
        <select class="filter-select" id="tarefas-pessoa" onchange="onTarefaFilterChange()">
          <option value="">Todas as Pessoas</option>
        </select>
        <select class="filter-select" id="tarefas-removido" onchange="onTarefaFilterChange()">
          <option value="nao">Ativos</option>
          <option value="sim">Removidos</option>
          <option value="todos">Todos</option>
        </select>
        <input class="filter-select" id="tarefas-busca" placeholder="Buscar ticket ou resumo..." oninput="onTarefaBuscaChange()" autocomplete="off" style="min-width:220px">
      </div>
      <div id="tarefas-content"><div class="alloc-empty">Selecione os filtros para listar tarefas.</div></div>
      <div class="tarefas-pagination" id="tarefas-pagination"></div>
    </div>
```

- [ ] **Step 4: Add CSS for tarefas table and pagination**

In `frontend/index.html`, in the CSS section (after the allocation CSS rules, around line 840), add:

```css
.tarefas-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.tarefas-table th { text-align: left; padding: 8px 10px; border-bottom: 2px solid var(--border); color: var(--text-secondary); font-weight: 600; font-size: 12px; text-transform: uppercase; }
.tarefas-table td { padding: 8px 10px; border-bottom: 1px solid var(--border); color: var(--text-primary); }
.tarefas-table tr:hover { background: var(--accent-soft); }
.tarefas-table .removido-badge { font-size: 10px; font-weight: 600; padding: 1px 6px; border-radius: 3px; }
.tarefas-table .removido-badge.sim { background: var(--red-soft); color: var(--red); }
.tarefas-table .removido-badge.nao { background: var(--accent-soft); color: var(--accent); }
.tarefas-delete-btn { background: none; border: 1px solid var(--red); color: var(--red); padding: 3px 8px; border-radius: 4px; cursor: pointer; font-size: 11px; }
.tarefas-delete-btn:hover { background: var(--red-soft); }
.tarefas-pagination { display: flex; gap: 8px; align-items: center; justify-content: center; margin-top: 16px; font-size: 13px; color: var(--text-secondary); }
.tarefas-pagination button { padding: 6px 12px; border-radius: 6px; border: 1px solid var(--border); background: var(--surface); color: var(--text-primary); cursor: pointer; }
.tarefas-pagination button:disabled { opacity: .4; cursor: default; }
.tarefas-resumo-cell { max-width: 300px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
```

- [ ] **Step 5: Add JS — loadTarefas, filter handlers, render, pagination, delete**

In `frontend/index.html`, in the JS section, before the `// === ALOCAÇÃO ===` section, add:

```javascript
// === TAREFAS ===
var tarefasPage = 1;
var tarefasPerPage = 50;
var tarefasDebounce = null;

function loadTarefas() {
  var eqSel = document.getElementById('tarefas-equipe');
  if (eqSel.options.length <= 1) {
    api('/equipes').then(function(equipes) {
      equipes.forEach(function(eq) {
        var opt = document.createElement('option');
        opt.value = eq.id;
        opt.textContent = eq.nome;
        eqSel.appendChild(opt);
      });
    });
  }
  var prodSel = document.getElementById('tarefas-produto');
  if (prodSel.options.length <= 1) {
    api('/allocation/products').then(function(produtos) {
      if (produtos && produtos.length) {
        produtos.forEach(function(p) {
          var opt = document.createElement('option');
          opt.value = p.nome;
          opt.textContent = p.nome;
          prodSel.appendChild(opt);
        });
      }
    });
  }
  fetchTarefas();
}

function onTarefaFilterChange() {
  tarefasPage = 1;
  fetchTarefas();
}

function onTarefaBuscaChange() {
  clearTimeout(tarefasDebounce);
  tarefasDebounce = setTimeout(function() {
    tarefasPage = 1;
    fetchTarefas();
  }, 400);
}

function fetchTarefas() {
  var container = document.getElementById('tarefas-content');
  container.innerHTML = '<div class="loading"><div class="spinner"></div></div>';

  var params = '?page=' + tarefasPage + '&per_page=' + tarefasPerPage;
  var equipe = document.getElementById('tarefas-equipe').value;
  if (equipe) params += '&equipe_id=' + equipe;
  var produto = document.getElementById('tarefas-produto').value;
  if (produto) params += '&produto_nome=' + encodeURIComponent(produto);
  var removido = document.getElementById('tarefas-removido').value;
  params += '&removido=' + removido;
  var busca = document.getElementById('tarefas-busca').value;
  if (busca) params += '&busca=' + encodeURIComponent(busca);

  api('/tarefas' + params).then(function(result) {
    renderTarefasTable(result, container);
    renderTarefasPagination(result);
  }).catch(function(err) {
    container.innerHTML = '<div class="alloc-empty">Erro ao carregar tarefas.</div>';
  });
}

function renderTarefasTable(result, container) {
  if (!result.items || result.items.length === 0) {
    container.innerHTML = '<div class="alloc-empty">Nenhuma tarefa encontrada.</div>';
    return;
  }

  var html = '<table class="tarefas-table"><thead><tr>';
  html += '<th>Ticket</th><th>Resumo</th><th>Tipo</th><th>Status</th>';
  html += '<th>Produto</th><th>Responsável</th><th>Removido</th><th>Data Remoção</th><th>Última Sync</th><th></th>';
  html += '</tr></thead><tbody>';

  result.items.forEach(function(t) {
    html += '<tr>';
    html += '<td><strong>' + esc(t.NumeroTicket) + '</strong></td>';
    html += '<td class="tarefas-resumo-cell" title="' + escAttr(t.Resumo) + '">' + esc(t.Resumo) + '</td>';
    html += '<td>' + esc(t.Tipo) + '</td>';
    html += '<td>' + esc(t.Status) + '</td>';
    html += '<td>' + esc(t.ProdutoNome || '--') + '</td>';
    html += '<td>' + esc(t.ResponsavelNome || '--') + '</td>';

    if (t.RemovidoEm) {
      html += '<td><span class="removido-badge sim">Removido</span></td>';
      html += '<td>' + new Date(t.RemovidoEm).toLocaleDateString('pt-BR') + '</td>';
    } else {
      html += '<td><span class="removido-badge nao">Ativo</span></td>';
      html += '<td>--</td>';
    }
    html += '<td>' + new Date(t.UpdatedAt).toLocaleDateString('pt-BR') + '</td>';

    if (t.RemovidoEm) {
      html += '<td><button class="tarefas-delete-btn" onclick="hardDeleteTarefa(\'' + t.ID + '\', \'' + esc(t.NumeroTicket) + '\')">Excluir</button></td>';
    } else {
      html += '<td></td>';
    }
    html += '</tr>';
  });

  html += '</tbody></table>';
  container.innerHTML = html;
}

function renderTarefasPagination(result) {
  var pagination = document.getElementById('tarefas-pagination');
  var totalPages = Math.ceil(result.total / result.per_page);
  if (totalPages <= 1) { pagination.innerHTML = ''; return; }

  var html = '';
  html += '<button onclick="tarefasGoPage(' + (tarefasPage - 1) + ')"' + (tarefasPage <= 1 ? ' disabled' : '') + '>&laquo; Anterior</button>';
  html += '<span>Página ' + tarefasPage + ' de ' + totalPages + ' (' + result.total + ' tarefas)</span>';
  html += '<button onclick="tarefasGoPage(' + (tarefasPage + 1) + ')"' + (tarefasPage >= totalPages ? ' disabled' : '') + '>Próximo &raquo;</button>';
  pagination.innerHTML = html;
}

function tarefasGoPage(page) {
  tarefasPage = page;
  fetchTarefas();
}

function hardDeleteTarefa(id, ticket) {
  if (!confirm('Excluir definitivamente a tarefa ' + ticket + '? Esta ação é irreversível.')) return;
  api('/tarefas/' + id, { method: 'DELETE' }).then(function() {
    fetchTarefas();
  }).catch(function(err) {
    alert('Erro ao excluir: ' + (err.message || err));
  });
}
```

**IMPORTANT NOTE on JSON field names:** The backend returns JSON with Go-style field names by default (PascalCase: `NumeroTicket`, `RemovidoEm`, etc.) because the `TarefaListRow` struct has no json tags. The implementer MUST add json tags to the struct in `tarefa.go`:

```go
type TarefaListRow struct {
	ID              uuid.UUID  `json:"id"`
	NumeroTicket    string     `json:"numero_ticket"`
	Resumo          string     `json:"resumo"`
	Tipo            string     `json:"tipo"`
	Status          string     `json:"status"`
	TipoDemanda     *string    `json:"tipo_demanda"`
	ResponsavelNome *string    `json:"responsavel_nome"`
	ProdutoNome     *string    `json:"produto_nome"`
	EquipeNome      *string    `json:"equipe_nome"`
	RemovidoEm      *time.Time `json:"removido_em"`
	MotivoRemocao   *string    `json:"motivo_remocao"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
```

And the JS in Step 5 must use **lowercase** field names: `t.numero_ticket`, `t.removido_em`, `t.updated_at`, `t.responsavel_nome`, `t.produto_nome`, `t.id`, etc.

- [ ] **Step 6: Test in browser**

1. Navigate to Tarefas page via sidebar
2. Verify dropdown filters load (equipes, produtos)
3. Change "Removido" to "Sim" — should show soft-deleted tarefas (if any exist after a sync)
4. Change to "Todos" — should show all
5. Search by ticket number
6. Test pagination with next/previous
7. On a removed tarefa, click "Excluir" — confirm deletion
8. Verify dark mode

---

### Task 5: Projetos page — add filters (Produto, Responsável, Removido)

**Files:**
- Modify: `frontend/index.html` — Projetos page HTML (~line 1008-1017), `loadProjetos` JS (~line 2271), `filterProjetos` JS (~line 2299)
- Modify: `backend/internal/handler/timeline.go:256-276` (ListProjetos — accept new query params)
- Modify: `backend/internal/repository/timeline.go:374-420` (ListarEpicos — accept new filter params)

**Interfaces:**
- Consumes: `removido_em` column from Task 1
- Consumes: existing `/allocation/products` endpoint for product dropdown
- Produces: Projetos page with additional filters: Produto, Responsável, Removido

- [ ] **Step 1: Update Projetos page HTML filters**

In `frontend/index.html`, replace the Projetos page filters section (~lines 1010-1015):

Change:
```html
      <div class="timeline-filters">
        <select class="filter-select" id="projetos-equipe" onchange="loadProjetos()"><option value="">Todas as equipes</option></select>
        <select class="filter-select" id="projetos-status" onchange="filterProjetos()"><option value="ativos">Ativos</option><option value="encerrados">Cancelados/Encerrados</option><option value="todos">Todos</option></select>
        <input class="filter-select" id="projetos-search" placeholder="Pesquisar projetos..." oninput="filterProjetos()" autocomplete="off" style="min-width:220px">
        <button class="btn-sm" onclick="openFeriadoModal()">Cadastrar Feriados</button>
      </div>
```
To:
```html
      <div class="timeline-filters" style="flex-wrap:wrap">
        <select class="filter-select" id="projetos-equipe" onchange="loadProjetos()"><option value="">Todas as equipes</option></select>
        <select class="filter-select" id="projetos-produto" onchange="loadProjetos()"><option value="">Todos os Produtos</option></select>
        <select class="filter-select" id="projetos-removido" onchange="loadProjetos()">
          <option value="nao">Ativos</option>
          <option value="sim">Removidos</option>
          <option value="todos">Todos</option>
        </select>
        <select class="filter-select" id="projetos-status" onchange="filterProjetos()"><option value="ativos">Ativos</option><option value="encerrados">Cancelados/Encerrados</option><option value="todos">Todos</option></select>
        <input class="filter-select" id="projetos-search" placeholder="Pesquisar projetos..." oninput="filterProjetos()" autocomplete="off" style="min-width:220px">
        <button class="btn-sm" onclick="openFeriadoModal()">Cadastrar Feriados</button>
      </div>
```

- [ ] **Step 2: Update loadProjetos JS**

In `frontend/index.html`, update `loadProjetos` (~line 2271) to load product dropdown and pass new filters:

Replace the function with:
```javascript
async function loadProjetos() {
  const el = document.getElementById('projetos-content');
  el.innerHTML = '<div class="loading"><div class="spinner"></div></div>';

  var prodSel = document.getElementById('projetos-produto');
  if (prodSel.options.length <= 1) {
    api('/allocation/products').then(function(produtos) {
      if (produtos && produtos.length) {
        produtos.forEach(function(p) {
          var opt = document.createElement('option');
          opt.value = p.nome;
          opt.textContent = p.nome;
          prodSel.appendChild(opt);
        });
      }
    });
  }

  const equipe = document.getElementById('projetos-equipe').value;
  const produto = document.getElementById('projetos-produto').value;
  const removido = document.getElementById('projetos-removido').value;
  var q = '?removido=' + removido;
  if (equipe) q += '&equipe=' + encodeURIComponent(equipe);
  if (produto) q += '&produto=' + encodeURIComponent(produto);

  try {
    const epicos = await api('/projetos' + q);
    if (!epicos || epicos.length === 0) { el.innerHTML = '<div class="alloc-empty">Nenhum projeto encontrado.</div>'; return; }
    let rows = '';
    epicos.forEach(p => {
```

Note: `loadProjetos` already uses `async/const` style (pre-existing). Keep the same style for the new code added to this function.

- [ ] **Step 3: Update ListProjetos handler**

In `backend/internal/handler/timeline.go`, update `ListProjetos` to accept new query params:

Replace the function:
```go
func (h *TimelineHandler) ListProjetos(w http.ResponseWriter, r *http.Request) {
	var equipeID *uuid.UUID
	if t := r.URL.Query().Get("equipe"); t != "" {
		id, err := uuid.Parse(t)
		if err != nil {
			respondError(w, http.StatusBadRequest, "equipe id inválido")
			return
		}
		equipeID = &id
	}

	var produtoNome *string
	if v := r.URL.Query().Get("produto"); v != "" {
		produtoNome = &v
	}

	removido := r.URL.Query().Get("removido")
	if removido == "" {
		removido = "nao"
	}

	projetoIDs := middleware.ProjetoIDsFromContext(r.Context())
	epicos, err := h.store.ListarEpicos(r.Context(), equipeID, projetoIDs, produtoNome, removido)
	if err != nil {
		h.logger.Error("failed to list epicos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar épicos")
		return
	}

	respondJSON(w, http.StatusOK, epicos)
}
```

- [ ] **Step 4: Update ListarEpicos repository**

In `backend/internal/repository/timeline.go`, update the `ListarEpicos` function signature and all 3 query branches to accept `produtoNome *string` and `removido string` parameters.

New signature:
```go
func (r *TimelineRepository) ListarEpicos(ctx context.Context, equipeID *uuid.UUID, projetoIDs []uuid.UUID, produtoNome *string, removido string) ([]domain.ProjetoListItem, error) {
```

Add dynamic WHERE conditions:

```go
	var extraConditions string
	switch removido {
	case "sim":
		extraConditions += " AND e.removido_em IS NOT NULL"
	case "todos":
		// show all
	default:
		extraConditions += " AND e.removido_em IS NULL"
	}
```

If `produtoNome != nil`, add:
```go
	if produtoNome != nil {
		// This will be added as a condition in each branch
		// Since arg positions differ per branch, build it inline
	}
```

For each of the 3 query branches, append `extraConditions` to the WHERE clause and add the produto filter if needed. The produto filter uses a subquery:
```sql
AND EXISTS (SELECT 1 FROM tarefas c JOIN tarefa_produtos tp ON tp.tarefa_id = c.id JOIN produtos p ON p.id = tp.produto_id WHERE c.parent_id = e.id AND LOWER(p.nome) = LOWER($N))
```

Where `$N` is the next arg position in each branch.

Also add `e.removido_em` to the SELECT list and scan into a new field in `ProjetoListItem`.

- [ ] **Step 5: Add removido_em to ProjetoListItem**

In `backend/internal/domain/models.go`, find the `ProjetoListItem` struct and add:

```go
RemovidoEm *time.Time `json:"removido_em"`
```

- [ ] **Step 6: Update frontend table rendering**

In `frontend/index.html`, in `loadProjetos`, where the table rows are built, add a "Removido" column when the filter is "sim" or "todos". After the existing columns, add:

```javascript
    var showRemovido = removido !== 'nao';
```

In the table header:
```javascript
    if (showRemovido) rows += '<th>Removido</th>';
```

In each row:
```javascript
    if (showRemovido) {
      if (p.removido_em) {
        rows += '<td>' + new Date(p.removido_em).toLocaleDateString('pt-BR') + '</td>';
      } else {
        rows += '<td>--</td>';
      }
    }
```

- [ ] **Step 7: Build and test**

```bash
cd /home/emerson/code/myplanner/backend
go build ./...
```

Test in browser:
1. Open Projetos page
2. Verify new filter dropdowns appear (Produto, Removido)
3. Change Removido to "Sim" — should show removed projects (if any)
4. Filter by Produto — should narrow results
5. Verify dark mode
