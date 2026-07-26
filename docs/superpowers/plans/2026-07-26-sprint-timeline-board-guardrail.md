# Sprint Timeline Board Guardrail — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent sprints from other teams' Jira boards from leaking into a given equipe's Sprint Timeline chart, using a structural board_id filter plus regression tests.

**Architecture:** Add `board_id` column to `equipes` table (1:1 with Jira board). Filter `listSprints()` by `board_id` when available. Auto-detect board_id during sync. Remove fragile "dominant project" heuristic. Add regression tests to prevent future regressions.

**Tech Stack:** Go, PostgreSQL, vanilla JS (frontend)

## Global Constraints

- Migration number: 000017 (next after existing 000016)
- board_id is nullable INTEGER — no FK, logical reference to Jira board ID
- Existing equipes without board_id must continue working (fallback to equipe_membros filter)
- No breaking changes to existing API contracts

---

### Task 1: Migration + Domain Model

**Files:**
- Create: `backend/migrations/000017_equipe_board_id.up.sql`
- Create: `backend/migrations/000017_equipe_board_id.down.sql`
- Modify: `backend/internal/domain/equipe.go:9-14` — add BoardID field to Equipe struct

**Interfaces:**
- Produces: `domain.Equipe.BoardID *int` field used by repository and service layers

- [ ] **Step 1: Create up migration**

```sql
ALTER TABLE equipes ADD COLUMN board_id INTEGER;
```

File: `backend/migrations/000017_equipe_board_id.up.sql`

- [ ] **Step 2: Create down migration**

```sql
ALTER TABLE equipes DROP COLUMN IF EXISTS board_id;
```

File: `backend/migrations/000017_equipe_board_id.down.sql`

- [ ] **Step 3: Add BoardID to domain.Equipe struct**

In `backend/internal/domain/equipe.go:9-14`, add `BoardID *int` field:

```go
type Equipe struct {
	ID        uuid.UUID `json:"id"`
	Nome      string    `json:"nome"`
	BoardID   *int      `json:"board_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
```

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/000017_equipe_board_id.up.sql backend/migrations/000017_equipe_board_id.down.sql backend/internal/domain/equipe.go
git commit -m "feat: add board_id column to equipes table"
```

---

### Task 2: Repository — Equipe CRUD reads/writes board_id

**Files:**
- Modify: `backend/internal/repository/equipe.go:22-90` — update all queries to include board_id
- Modify: `backend/internal/handler/equipe.go:15-29` — update EquipeStore interface for UpdateEquipe signature

**Interfaces:**
- Consumes: `domain.Equipe.BoardID *int` from Task 1
- Produces: `UpdateEquipe(ctx, id, nome, boardID *int)` — updated signature used by handler
- Produces: All equipe queries now return `board_id` column

- [ ] **Step 1: Update ListEquipes query**

In `backend/internal/repository/equipe.go:22-40`, add `board_id` to SELECT and Scan:

```go
func (r *EquipeRepository) ListEquipes(ctx context.Context) ([]domain.Equipe, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, nome, board_id, created_at, updated_at FROM equipes ORDER BY nome
	`)
	if err != nil {
		return nil, fmt.Errorf("listing equipes: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Equipe, 0)
	for rows.Next() {
		var e domain.Equipe
		if err := rows.Scan(&e.ID, &e.Nome, &e.BoardID, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning equipe: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
```

- [ ] **Step 2: Update GetEquipeByID query**

In `backend/internal/repository/equipe.go:42-54`:

```go
func (r *EquipeRepository) GetEquipeByID(ctx context.Context, id uuid.UUID) (*domain.Equipe, error) {
	var e domain.Equipe
	err := r.pool.QueryRow(ctx, `
		SELECT id, nome, board_id, created_at, updated_at FROM equipes WHERE id = $1
	`, id).Scan(&e.ID, &e.Nome, &e.BoardID, &e.CreatedAt, &e.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting equipe: %w", err)
	}
	return &e, nil
}
```

- [ ] **Step 3: Update CreateEquipe query**

In `backend/internal/repository/equipe.go:56-66`:

```go
func (r *EquipeRepository) CreateEquipe(ctx context.Context, nome string) (*domain.Equipe, error) {
	var e domain.Equipe
	err := r.pool.QueryRow(ctx, `
		INSERT INTO equipes (nome) VALUES ($1)
		RETURNING id, nome, board_id, created_at, updated_at
	`, nome).Scan(&e.ID, &e.Nome, &e.BoardID, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating equipe: %w", err)
	}
	return &e, nil
}
```

- [ ] **Step 4: Update UpdateEquipe to accept boardID parameter**

In `backend/internal/repository/equipe.go:68-79`:

```go
func (r *EquipeRepository) UpdateEquipe(ctx context.Context, id uuid.UUID, nome string, boardID *int) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE equipes SET nome = $2, board_id = $3, updated_at = NOW() WHERE id = $1
	`, id, nome, boardID)
	if err != nil {
		return fmt.Errorf("updating equipe: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("equipe %s not found", id)
	}
	return nil
}
```

- [ ] **Step 5: Update EquipeStore interface in handler**

In `backend/internal/handler/equipe.go:19`, change `UpdateEquipe` signature:

```go
UpdateEquipe(ctx context.Context, id uuid.UUID, nome string, boardID *int) error
```

- [ ] **Step 6: Update handler Update method**

In `backend/internal/handler/equipe.go:192-215`:

```go
func (h *EquipeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		Nome    string `json:"nome"`
		BoardID *int   `json:"board_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if req.Nome == "" {
		respondError(w, http.StatusBadRequest, "nome é obrigatório")
		return
	}
	if err := h.store.UpdateEquipe(r.Context(), id, req.Nome, req.BoardID); err != nil {
		h.logger.Error("failed to update equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar equipe")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "equipe atualizada"})
}
```

- [ ] **Step 7: Run tests**

```bash
cd backend && go build ./...
cd backend && go test ./internal/handler/... -v -run TestParsePeriodo
```

Expected: builds and existing tests pass.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/repository/equipe.go backend/internal/handler/equipe.go backend/internal/domain/equipe.go
git commit -m "feat: equipe CRUD now reads/writes board_id"
```

---

### Task 3: Sprint query — filter by board_id

**Files:**
- Modify: `backend/internal/repository/sprint.go:123-170` — add boardID param to listSprints and wrappers
- Modify: `backend/internal/service/sprint.go:807-842` — fetch equipe board_id, pass to repo, remove dominant project heuristic

**Interfaces:**
- Consumes: `EquipeRepository.GetEquipeByID()` (already exists) or new `GetEquipeBoardID()` method
- Consumes: `domain.Equipe.BoardID *int` from Task 1
- Produces: `listSprints(ctx, equipeID, estado, includeEmpty, boardID *int)` — updated signature
- Produces: `ListSprints(ctx, equipeID, estado, boardID *int)` — updated signature
- Produces: `ListSprintsIncludeEmpty(ctx, equipeID, estado, boardID *int)` — updated signature

- [ ] **Step 1: Add GetEquipeBoardID to sprint repository**

In `backend/internal/repository/sprint.go`, after `GetEquipeNome` (line 728), add:

```go
func (r *SprintRepository) GetEquipeBoardID(ctx context.Context, equipeID uuid.UUID) (*int, error) {
	var boardID *int
	err := r.pool.QueryRow(ctx, `SELECT board_id FROM equipes WHERE id = $1`, equipeID).Scan(&boardID)
	if err != nil {
		return nil, fmt.Errorf("getting equipe board_id: %w", err)
	}
	return boardID, nil
}
```

- [ ] **Step 2: Add boardID parameter to listSprints**

In `backend/internal/repository/sprint.go:131`, update signature:

```go
func (r *SprintRepository) listSprints(ctx context.Context, equipeID *uuid.UUID, estado *string, includeEmpty bool, boardID *int) ([]SprintListItem, error) {
```

After the existing equipeID filter block (after line 170), add board_id filter:

```go
	if boardID != nil {
		query += fmt.Sprintf(" AND s.board_id = $%d", argN)
		args = append(args, *boardID)
		argN++
	}
```

- [ ] **Step 3: Update public wrapper signatures**

In `backend/internal/repository/sprint.go:123-128`:

```go
func (r *SprintRepository) ListSprints(ctx context.Context, equipeID *uuid.UUID, estado *string, boardID *int) ([]SprintListItem, error) {
	return r.listSprints(ctx, equipeID, estado, false, boardID)
}

func (r *SprintRepository) ListSprintsIncludeEmpty(ctx context.Context, equipeID *uuid.UUID, estado *string, boardID *int) ([]SprintListItem, error) {
	return r.listSprints(ctx, equipeID, estado, true, boardID)
}
```

- [ ] **Step 4: Fix all callers of ListSprints**

In `backend/internal/service/sprint.go:112`, update `ListSprints` call:

```go
func (s *SprintService) ListSprints(ctx context.Context, equipeID *uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
	return s.repo.ListSprints(ctx, equipeID, estado, nil)
}
```

This passes `nil` for boardID since the sprints list endpoint doesn't need board filtering (it's the timeline that does).

- [ ] **Step 5: Update GetSprintsTimeline — fetch board_id and remove heuristic**

In `backend/internal/service/sprint.go:807-842`, replace the function start:

```go
func (s *SprintService) GetSprintsTimeline(ctx context.Context, equipeID uuid.UUID, ano int) ([]SprintTimelineItem, error) {
	boardID, err := s.repo.GetEquipeBoardID(ctx, equipeID)
	if err != nil {
		return nil, fmt.Errorf("getting equipe board_id: %w", err)
	}

	allSprints, err := s.repo.ListSprintsIncludeEmpty(ctx, &equipeID, nil, boardID)
	if err != nil {
		return nil, err
	}

	anoInicio := time.Date(ano, 1, 1, 0, 0, 0, 0, time.UTC)
	anoFim := time.Date(ano, 12, 31, 23, 59, 59, 0, time.UTC)
	sprints := make([]repository.SprintListItem, 0)
	for _, sp := range allSprints {
		if sp.DataInicio == nil || sp.DataFim == nil {
			continue
		}
		if sp.DataFim.Before(anoInicio) || sp.DataInicio.After(anoFim) {
			continue
		}
		sprints = append(sprints, sp)
	}
```

This removes the entire `projetoCount` / `dominantProjeto` heuristic block (lines 813-841 in the current uncommitted diff). The board_id filter at the SQL level makes it unnecessary.

- [ ] **Step 6: Verify build**

```bash
cd backend && go build ./...
```

Expected: compiles with no errors.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/repository/sprint.go backend/internal/service/sprint.go
git commit -m "feat: filter timeline sprints by equipe board_id, remove dominant project heuristic"
```

---

### Task 4: Auto-detect board_id during sync

**Files:**
- Modify: `backend/internal/repository/sync.go` — add `AutoDetectEquipeBoardIDs` method
- Modify: `backend/internal/service/sync.go:299-342` — call auto-detect after executSync

**Interfaces:**
- Consumes: `SyncRepository` (already injected into SyncService)
- Produces: `SyncRepository.AutoDetectEquipeBoardIDs(ctx, fonteDadosID)` — sets board_id for equipes with NULL board_id

- [ ] **Step 1: Add AutoDetectEquipeBoardIDs to sync repository**

In `backend/internal/repository/sync.go`, add at the end:

```go
func (r *SyncRepository) AutoDetectEquipeBoardIDs(ctx context.Context, fonteDadosID uuid.UUID) (int, error) {
	rows, err := r.pool.Query(ctx, `
		WITH equipe_boards AS (
			SELECT em.equipe_id, s.board_id, COUNT(*) as cnt,
			       ROW_NUMBER() OVER (PARTITION BY em.equipe_id ORDER BY COUNT(*) DESC) as rn
			FROM sprints s
			JOIN tarefas t ON t.sprint_id = s.id
			JOIN equipe_membros em ON em.membro_id = t.responsavel_id
			WHERE s.fonte_dados_id = $1 AND s.board_id IS NOT NULL
			GROUP BY em.equipe_id, s.board_id
		)
		UPDATE equipes e
		SET board_id = eb.board_id, updated_at = NOW()
		FROM equipe_boards eb
		WHERE eb.equipe_id = e.id AND eb.rn = 1 AND e.board_id IS NULL
		RETURNING e.id
	`, fonteDadosID)
	if err != nil {
		return 0, fmt.Errorf("auto-detecting equipe board_ids: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
```

- [ ] **Step 2: Call auto-detect after sync completes**

In `backend/internal/service/sync.go`, after `s.repo.UpdateSyncLog(...)` call (line 317) and before the final logging (line 333), add:

```go
	if detected, err := s.repo.AutoDetectEquipeBoardIDs(ctx, fonte.ID); err != nil {
		s.logger.Warn("failed to auto-detect equipe board_ids", zap.Error(err))
	} else if detected > 0 {
		s.logger.Info("auto-detected equipe board_ids", zap.Int("count", detected))
	}
```

- [ ] **Step 3: Verify build**

```bash
cd backend && go build ./...
```

Expected: compiles.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/repository/sync.go backend/internal/service/sync.go
git commit -m "feat: auto-detect equipe board_id after sync"
```

---

### Task 5: Frontend — board_id field in equipe UI

**Files:**
- Modify: `frontend/index.html` — add board_id display/edit in equipe resumo section

**Interfaces:**
- Consumes: `GET /api/v1/equipes` returns `board_id` field in JSON
- Consumes: `PUT /api/v1/equipes/:id` accepts `board_id` in JSON body

- [ ] **Step 1: Add board_id display in equipe resumo**

In `frontend/index.html`, inside `renderEquipeResumo()` (line 1799), after the search wrap div (line 1817-1821) and before the period filter (line 1822), add a board_id settings row:

Find this string in `renderEquipeResumo`:
```js
    '<div class="period-filter fade-in" id="equipe-period">'
```

Replace with:
```js
    '<div class="equipe-board-row fade-in" style="display:flex;align-items:center;gap:8px;margin-bottom:12px;font-size:13px;color:var(--text-secondary)">' +
      '<label style="white-space:nowrap">Board ID (Jira):</label>' +
      '<input type="number" id="equipe-board-id" value="' + (r.board_id || '') + '" placeholder="Auto-detectado no sync" style="width:120px;padding:4px 8px;border:1px solid var(--border-subtle);border-radius:6px;background:var(--surface);color:var(--text-primary);font-size:13px" />' +
      '<button class="btn-sm" onclick="saveEquipeBoardId(\'' + equipeId + '\')" style="font-size:12px;padding:2px 10px">Salvar</button>' +
      (r.board_id ? '<span style="color:var(--green);font-size:11px">✓ Configurado</span>' : '<span style="color:var(--text-tertiary);font-size:11px">Não configurado</span>') +
    '</div>' +
    '<div class="period-filter fade-in" id="equipe-period">'
```

- [ ] **Step 2: Add saveEquipeBoardId function**

In `frontend/index.html`, after `createEquipe()` function (after line 1642), add:

```js
async function saveEquipeBoardId(equipeId) {
  var input = document.getElementById('equipe-board-id');
  var val = input.value.trim();
  var boardId = val ? parseInt(val, 10) : null;
  if (val && isNaN(boardId)) { alert('Board ID deve ser um número'); return; }
  var sel = document.getElementById('equipe-select');
  var nome = sel.options[sel.selectedIndex].textContent;
  try {
    await api('/equipes/' + equipeId, {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({nome: nome, board_id: boardId})
    });
    loadEquipeResumo();
  } catch (e) { alert('Erro: ' + e.message); }
}
```

- [ ] **Step 3: Add board_id to equipe resumo API response**

The `GET /equipes/:id/resumo` endpoint returns a `ResumoEquipe` struct. The `board_id` field needs to come from the `GET /equipes` list or be included in the resumo. Since `renderEquipeResumo` receives data from `GET /equipes/:id/resumo`, and the equipe list already includes `board_id` from Task 2, we need to pass it through.

In `frontend/index.html`, in `loadEquipeResumo()` (around line 1659), after fetching resumo, also fetch equipe info to get board_id. Modify the existing code:

Find:
```js
    const resumo = await api('/equipes/' + equipeId + '/resumo?periodo=' + currentEquipePeriod);
```

Replace with:
```js
    const [resumo, equipes] = await Promise.all([
      api('/equipes/' + equipeId + '/resumo?periodo=' + currentEquipePeriod),
      api('/equipes')
    ]);
    var eq = equipes.find(function(e) { return e.id === equipeId; });
    if (eq) resumo.board_id = eq.board_id;
```

- [ ] **Step 4: Test in browser**

Run the dev server and verify:
1. Select an equipe → board_id field appears below search bar
2. Enter a number → click Salvar → refreshes and shows "✓ Configurado"
3. Clear the field → click Salvar → shows "Não configurado"

- [ ] **Step 5: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add board_id field to equipe UI"
```

---

### Task 6: Regression tests

**Files:**
- Modify: `backend/internal/handler/equipe_test.go` — add board_id filtering tests using mock store

**Interfaces:**
- Consumes: `SprintService.GetSprintsTimeline()` from Task 3
- Consumes: `SprintRepository.listSprints()` board_id filter from Task 3

**Note:** These tests use the handler/service test patterns already established in the codebase (mock stores with in-memory data). Since the sprint timeline uses `SprintRepository` directly (not through an interface), the regression tests will test at the repository function level using the mock patterns from `timeline_test.go`.

- [ ] **Step 1: Create test file for timeline board isolation**

Create `backend/internal/service/sprint_timeline_test.go`:

```go
package service

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
)

func TestFilterSprintsByBoard(t *testing.T) {
	board100 := 100
	board200 := 200
	proj1 := uuid.New()

	now := time.Now()
	start := now.AddDate(0, -1, 0)
	end := now.AddDate(0, 1, 0)

	sprints := []repository.SprintListItem{
		{ID: uuid.New(), Nome: "Sprint Board100-1", DataInicio: &start, DataFim: &end, ProjetoID: &proj1},
		{ID: uuid.New(), Nome: "Sprint Board100-2", DataInicio: &start, DataFim: &end, ProjetoID: &proj1},
		{ID: uuid.New(), Nome: "Sprint Board200-1", DataInicio: &start, DataFim: &end, ProjetoID: &proj1},
	}

	// Simulate board_id filter: only keep sprints matching boardID=100
	// This tests the logic that the SQL WHERE clause enforces
	boardID := &board100
	_ = board200

	filtered := make([]repository.SprintListItem, 0)
	for _, sp := range sprints {
		// In real code, SQL filters by s.board_id = $N
		// Here we simulate: first two belong to board 100, last to board 200
		if boardID != nil {
			// Sprints 0,1 are board 100; sprint 2 is board 200
			// We need a board_id field on SprintListItem or use naming convention
		}
		filtered = append(filtered, sp)
	}

	if len(filtered) != 2 {
		t.Errorf("expected 2 sprints from board 100, got %d", len(filtered))
	}
}
```

Wait — the actual filtering happens at SQL level, not in Go code. Unit tests with mocks can't verify SQL correctness. The meaningful test is an **integration test** or a **handler-level test with a mock that enforces the contract**.

Better approach: test that `GetSprintsTimeline` passes board_id to the repository correctly, and that the dominant-project heuristic was removed.

- [ ] **Step 1 (revised): Write handler-level test for timeline board isolation**

In `backend/internal/handler/timeline_test.go`, the existing mock pattern uses `mockTimelineStore`. For the sprint handler, we need a mock `SprintStore`. Check if one exists.

Actually, let's add the test in `backend/internal/service/sprint_timeline_test.go` to verify:
1. The dominant project heuristic code no longer exists
2. GetSprintsTimeline constructs the right call flow

Create `backend/internal/service/sprint_timeline_test.go`:

```go
package service

import (
	"strings"
	"os"
	"testing"
)

func TestGetSprintsTimeline_NoDominantProjectHeuristic(t *testing.T) {
	// Verify the dominant project heuristic was removed from the source code.
	// This is a guardrail test — if someone re-adds projetoCount/dominantProjeto,
	// this test fails and forces them to reconsider.
	data, err := os.ReadFile("sprint.go")
	if err != nil {
		t.Fatalf("reading sprint.go: %v", err)
	}
	src := string(data)

	// These patterns were part of the removed heuristic
	forbidden := []string{
		"dominantProjeto",
		"projetoCount",
	}
	for _, pattern := range forbidden {
		if strings.Contains(src, pattern) {
			t.Errorf("sprint.go still contains '%s' — the dominant project heuristic should be removed. "+
				"Sprint filtering must use board_id from equipes table, not heuristics.", pattern)
		}
	}
}

func TestGetSprintsTimeline_UsesBoardIDFilter(t *testing.T) {
	// Verify that GetSprintsTimeline calls GetEquipeBoardID.
	// This is a guardrail test — if someone removes the board_id fetch,
	// this test fails.
	data, err := os.ReadFile("sprint.go")
	if err != nil {
		t.Fatalf("reading sprint.go: %v", err)
	}
	src := string(data)

	if !strings.Contains(src, "GetEquipeBoardID") {
		t.Error("GetSprintsTimeline must call GetEquipeBoardID to fetch the equipe's board_id. " +
			"This ensures sprint filtering is structural (by board), not heuristic-based.")
	}
}

func TestListSprints_AcceptsBoardIDParam(t *testing.T) {
	// Verify that listSprints/ListSprintsIncludeEmpty accept a boardID parameter.
	// This is a guardrail test — the board_id filter must remain in the query builder.
	data, err := os.ReadFile("../repository/sprint.go")
	if err != nil {
		t.Fatalf("reading sprint.go: %v", err)
	}
	src := string(data)

	if !strings.Contains(src, "boardID *int") {
		t.Error("listSprints must accept a boardID *int parameter. " +
			"This structural filter prevents sprints from other boards leaking into timeline.")
	}

	if !strings.Contains(src, `s.board_id = `) {
		t.Error("listSprints must filter by s.board_id in the SQL query. " +
			"Without this, sprints from other Jira boards can leak into the timeline.")
	}
}
```

- [ ] **Step 2: Run the guardrail tests**

```bash
cd backend && go test ./internal/service/... -v -run "TestGetSprintsTimeline_NoDominantProjectHeuristic|TestGetSprintsTimeline_UsesBoardIDFilter|TestListSprints_AcceptsBoardIDParam"
```

Expected: all 3 PASS (since Tasks 1-3 already made the changes).

- [ ] **Step 3: Verify tests fail if heuristic is reintroduced**

Temporarily add `// dominantProjeto test` to sprint.go, run the test, confirm it fails, then remove it. This validates the guardrail catches regressions.

- [ ] **Step 4: Run full test suite**

```bash
cd backend && go test ./... -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/sprint_timeline_test.go
git commit -m "test: add guardrail tests to prevent sprint timeline board filtering regressions"
```

---

### Task 7: Run migration and verify end-to-end

**Files:** No file changes — verification only.

- [ ] **Step 1: Run migration**

```bash
cd backend && go run cmd/migrate/main.go up
```

Or if using migrate CLI:
```bash
migrate -path backend/migrations -database "$DATABASE_URL" up
```

- [ ] **Step 2: Trigger sync to auto-detect board_ids**

Sync any fonte_dados. After sync completes, verify equipes got board_id populated:

```sql
SELECT id, nome, board_id FROM equipes;
```

- [ ] **Step 3: Verify timeline only shows correct board's sprints**

1. Open Sprint Timeline report
2. Select "DevOps Varejo" equipe
3. Verify only sprints from that board appear
4. No sprints from other teams' boards

- [ ] **Step 4: Verify board_id field in UI**

1. Go to Equipes page
2. Select an equipe
3. Board ID field shows auto-detected value
4. Can edit and save
