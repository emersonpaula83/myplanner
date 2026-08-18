# Projeto Favoritos Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users mark Jira projects as favorites within a Fonte de Dados so favorites appear at the top of the projects modal and can be batch-synced in one click.

**Architecture:** New `usuario_projeto_favoritos` table stores per-user favorites keyed by `project_key`. A `FavoritosRepository` handles persistence with a delete-then-insert replace pattern. A `FavoritosHandler` exposes GET/PUT for favorites and POST for batch sync. The frontend projects modal gets star toggles, favorites-first sorting, and a "Sincronizar Favoritos" button.

**Tech Stack:** Go 1.22+, PostgreSQL (pgx/v5), chi router, vanilla JS frontend (monolithic `index.html`)

## Global Constraints

- Monolithic frontend: all frontend changes in `frontend/index.html`
- Go backend: follow existing patterns (handler defines local interface, struct holds store + logger, constructor injects)
- Auth: reuse `middleware.UserIDFromContext(r.Context())` for user identification
- Routes: register flat under the existing `r.Group` that applies `AuthJWT` + `EquipeFilter` middleware
- CSS: use existing theme variables (`--text-primary`, `--text-tertiary`, `--border-subtle`, `--accent-soft`, etc.)
- No external dependencies

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `backend/migrations/000036_usuario_projeto_favoritos.up.sql` | Create | Migration up |
| `backend/migrations/000036_usuario_projeto_favoritos.down.sql` | Create | Migration down |
| `backend/internal/repository/favoritos.go` | Create | `FavoritosRepository` — List, Replace |
| `backend/internal/repository/favoritos_test.go` | Create | Repository integration tests |
| `backend/internal/handler/favoritos.go` | Create | `FavoritosHandler` — List, Replace, TriggerBatch |
| `backend/cmd/api/main.go` | Modify | Instantiate repo + handler, register 3 routes |
| `frontend/index.html` | Modify | CSS, modal HTML, JS functions |

---

### Task 1: Database Migration

**Files:**
- Create: `backend/migrations/000036_usuario_projeto_favoritos.up.sql`
- Create: `backend/migrations/000036_usuario_projeto_favoritos.down.sql`

**Interfaces:**
- Consumes: nothing
- Produces: `usuario_projeto_favoritos` table used by Task 2's repository

- [ ] **Step 1: Create up migration**

```sql
-- backend/migrations/000036_usuario_projeto_favoritos.up.sql
CREATE TABLE usuario_projeto_favoritos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id UUID NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    fonte_dados_id UUID NOT NULL REFERENCES fonte_dados(id) ON DELETE CASCADE,
    project_key VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(usuario_id, fonte_dados_id, project_key)
);
CREATE INDEX idx_upf_usuario_fonte ON usuario_projeto_favoritos(usuario_id, fonte_dados_id);
```

- [ ] **Step 2: Create down migration**

```sql
-- backend/migrations/000036_usuario_projeto_favoritos.down.sql
DROP TABLE IF EXISTS usuario_projeto_favoritos;
```

- [ ] **Step 3: Run migration**

Run: `cd backend && go run cmd/migrate/main.go up` (or however the project runs migrations — check `backend/cmd/migrate/` for the entry point; if using `golang-migrate` CLI: `migrate -path migrations -database "$DATABASE_URL" up`)

Verify: connect to the DB and confirm the table exists:
```sql
\d usuario_projeto_favoritos
```

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/000036_usuario_projeto_favoritos.up.sql backend/migrations/000036_usuario_projeto_favoritos.down.sql
git commit -m "feat: add usuario_projeto_favoritos migration"
```

---

### Task 2: FavoritosRepository

**Files:**
- Create: `backend/internal/repository/favoritos.go`
- Create: `backend/internal/repository/favoritos_test.go`

**Interfaces:**
- Consumes: `usuario_projeto_favoritos` table (Task 1)
- Produces:
  - `FavoritosRepository` struct with `pool *pgxpool.Pool`
  - `NewFavoritosRepository(pool *pgxpool.Pool) *FavoritosRepository`
  - `List(ctx context.Context, usuarioID uuid.UUID, fonteDadosID uuid.UUID) ([]string, error)` — returns sorted project_keys
  - `Replace(ctx context.Context, usuarioID uuid.UUID, fonteDadosID uuid.UUID, projectKeys []string) error` — delete + insert in tx

- [ ] **Step 1: Write the failing test for List**

```go
// backend/internal/repository/favoritos_test.go
package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestFavoritosRepository_List_Empty(t *testing.T) {
	pool := getTestPool(t)
	repo := NewFavoritosRepository(pool)
	ctx := context.Background()

	keys, err := repo.List(ctx, uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if keys == nil {
		t.Fatal("List returned nil, expected empty slice")
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(keys))
	}
}
```

Note: check how existing repo tests get a DB pool. Look at `backend/internal/repository/` for test helpers — there may be a `getTestPool` or `TestMain` setup. If tests use a real DB, ensure `DATABASE_URL` is set. If no test helper exists, use:
```go
func getTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/repository/ -run TestFavoritosRepository_List_Empty -v`
Expected: compilation error — `NewFavoritosRepository` not defined

- [ ] **Step 3: Implement FavoritosRepository with List**

```go
// backend/internal/repository/favoritos.go
package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FavoritosRepository struct {
	pool *pgxpool.Pool
}

func NewFavoritosRepository(pool *pgxpool.Pool) *FavoritosRepository {
	return &FavoritosRepository{pool: pool}
}

func (r *FavoritosRepository) List(ctx context.Context, usuarioID uuid.UUID, fonteDadosID uuid.UUID) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT project_key FROM usuario_projeto_favoritos
		WHERE usuario_id = $1 AND fonte_dados_id = $2
		ORDER BY project_key
	`, usuarioID, fonteDadosID)
	if err != nil {
		return nil, fmt.Errorf("listing favoritos: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scanning favorito key: %w", err)
		}
		keys = append(keys, key)
	}
	if keys == nil {
		keys = []string{}
	}
	return keys, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/repository/ -run TestFavoritosRepository_List_Empty -v`
Expected: PASS

- [ ] **Step 5: Write the failing test for Replace**

Add to `favoritos_test.go`:
```go
func TestFavoritosRepository_Replace(t *testing.T) {
	pool := getTestPool(t)
	repo := NewFavoritosRepository(pool)
	ctx := context.Background()

	userID := createTestUsuario(t, pool)
	fonteID := createTestFonteDados(t, pool)
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM usuario_projeto_favoritos WHERE usuario_id = $1", userID)
		pool.Exec(ctx, "DELETE FROM usuarios WHERE id = $1", userID)
		pool.Exec(ctx, "DELETE FROM fonte_dados WHERE id = $1", fonteID)
	})

	// Replace with 2 keys
	err := repo.Replace(ctx, userID, fonteID, []string{"TCDV", "PLAT"})
	if err != nil {
		t.Fatalf("Replace returned error: %v", err)
	}

	keys, _ := repo.List(ctx, userID, fonteID)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	// Replace with different keys (old ones should be gone)
	err = repo.Replace(ctx, userID, fonteID, []string{"DATA"})
	if err != nil {
		t.Fatalf("second Replace returned error: %v", err)
	}

	keys, _ = repo.List(ctx, userID, fonteID)
	if len(keys) != 1 || keys[0] != "DATA" {
		t.Fatalf("expected [DATA], got %v", keys)
	}

	// Replace with empty clears all
	err = repo.Replace(ctx, userID, fonteID, []string{})
	if err != nil {
		t.Fatalf("empty Replace returned error: %v", err)
	}

	keys, _ = repo.List(ctx, userID, fonteID)
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys after clear, got %d", len(keys))
	}
}

func createTestUsuario(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO usuarios (id, nome_completo, apelido, email, senha_hash, cargo)
		VALUES ($1, 'Test User', $2, $3, 'hash', 'coordenador')
	`, id, "test_"+id.String()[:8], "test_"+id.String()[:8]+"@test.com")
	if err != nil {
		t.Fatalf("creating test usuario: %v", err)
	}
	return id
}

func createTestFonteDados(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO fonte_dados (id, nome, tipo, base_url, auth_type)
		VALUES ($1, 'Test Fonte', 'jira', 'https://test.atlassian.net', 'basic')
	`, id)
	if err != nil {
		t.Fatalf("creating test fonte_dados: %v", err)
	}
	return id
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd backend && go test ./internal/repository/ -run TestFavoritosRepository_Replace -v`
Expected: compilation error — `Replace` not defined

- [ ] **Step 7: Implement Replace**

Add to `favoritos.go`:
```go
func (r *FavoritosRepository) Replace(ctx context.Context, usuarioID uuid.UUID, fonteDadosID uuid.UUID, projectKeys []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `DELETE FROM usuario_projeto_favoritos WHERE usuario_id = $1 AND fonte_dados_id = $2`, usuarioID, fonteDadosID)
	if err != nil {
		return fmt.Errorf("deleting existing favoritos: %w", err)
	}

	for _, key := range projectKeys {
		_, err = tx.Exec(ctx, `
			INSERT INTO usuario_projeto_favoritos (usuario_id, fonte_dados_id, project_key)
			VALUES ($1, $2, $3)
		`, usuarioID, fonteDadosID, key)
		if err != nil {
			return fmt.Errorf("inserting favorito %s: %w", key, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}
```

- [ ] **Step 8: Run all repository tests**

Run: `cd backend && go test ./internal/repository/ -run TestFavoritosRepository -v`
Expected: all PASS

- [ ] **Step 9: Commit**

```bash
git add backend/internal/repository/favoritos.go backend/internal/repository/favoritos_test.go
git commit -m "feat: add FavoritosRepository with List and Replace"
```

---

### Task 3: FavoritosHandler + Routes

**Files:**
- Create: `backend/internal/handler/favoritos.go`
- Modify: `backend/cmd/api/main.go` (~lines 70, 148, 242)

**Interfaces:**
- Consumes:
  - `FavoritosRepository.List(ctx, usuarioID, fonteDadosID) ([]string, error)` (Task 2)
  - `FavoritosRepository.Replace(ctx, usuarioID, fonteDadosID, projectKeys) error` (Task 2)
  - `middleware.UserIDFromContext(ctx) uuid.UUID` (existing)
  - `SyncService.SyncProject(ctx, fonteDadosID, projectKey) (*domain.SyncLog, error)` (existing)
  - `respondJSON(w, status, data)` and `respondError(w, status, msg)` (existing in `handler/response.go`)
- Produces:
  - `GET /fontes/{id}/favoritos` → `[]string`
  - `PUT /fontes/{id}/favoritos` with body `{"project_keys": [...]}` → `{"project_keys": [...]}`
  - `POST /sync/trigger-batch` with body `{"fonte_dados_id": "..."}` → `{"triggered": [...], "count": N}`

- [ ] **Step 1: Create FavoritosHandler**

```go
// backend/internal/handler/favoritos.go
package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type FavoritosStore interface {
	List(ctx context.Context, usuarioID uuid.UUID, fonteDadosID uuid.UUID) ([]string, error)
	Replace(ctx context.Context, usuarioID uuid.UUID, fonteDadosID uuid.UUID, projectKeys []string) error
}

type FavoritosSyncService interface {
	SyncProject(ctx context.Context, fonteDadosID uuid.UUID, projectKey string) (*domain.SyncLog, error)
}

type FavoritosHandler struct {
	store   FavoritosStore
	syncSvc FavoritosSyncService
	logger  *zap.Logger
}

func NewFavoritosHandler(store FavoritosStore, syncSvc FavoritosSyncService, logger *zap.Logger) *FavoritosHandler {
	return &FavoritosHandler{store: store, syncSvc: syncSvc, logger: logger}
}

func (h *FavoritosHandler) List(w http.ResponseWriter, r *http.Request) {
	fonteID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "ID inválido")
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		respondError(w, http.StatusUnauthorized, "usuário não autenticado")
		return
	}

	keys, err := h.store.List(r.Context(), userID, fonteID)
	if err != nil {
		h.logger.Error("failed to list favoritos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro ao listar favoritos")
		return
	}

	respondJSON(w, http.StatusOK, keys)
}

func (h *FavoritosHandler) Replace(w http.ResponseWriter, r *http.Request) {
	fonteID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "ID inválido")
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		respondError(w, http.StatusUnauthorized, "usuário não autenticado")
		return
	}

	var input struct {
		ProjectKeys []string `json:"project_keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if input.ProjectKeys == nil {
		input.ProjectKeys = []string{}
	}

	if err := h.store.Replace(r.Context(), userID, fonteID, input.ProjectKeys); err != nil {
		h.logger.Error("failed to replace favoritos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro ao salvar favoritos")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"project_keys": input.ProjectKeys})
}

func (h *FavoritosHandler) TriggerBatch(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		respondError(w, http.StatusUnauthorized, "usuário não autenticado")
		return
	}

	var input struct {
		FonteDadosID uuid.UUID `json:"fonte_dados_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if input.FonteDadosID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "fonte_dados_id obrigatório")
		return
	}

	keys, err := h.store.List(r.Context(), userID, input.FonteDadosID)
	if err != nil {
		h.logger.Error("failed to list favoritos for batch", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro ao buscar favoritos")
		return
	}
	if len(keys) == 0 {
		respondError(w, http.StatusBadRequest, "nenhum projeto favorito para sincronizar")
		return
	}

	for _, key := range keys {
		if _, err := h.syncSvc.SyncProject(r.Context(), input.FonteDadosID, key); err != nil {
			h.logger.Warn("batch sync failed for project", zap.String("key", key), zap.Error(err))
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{"triggered": keys, "count": len(keys)})
}
```

- [ ] **Step 2: Register routes and instantiate in main.go**

In `backend/cmd/api/main.go`, add three changes:

**A. Instantiate repo** — add after `fonteDadosRepo := repository.NewFonteDadosRepository(pool)` (line 70):
```go
favoritosRepo := repository.NewFavoritosRepository(pool)
```

**B. Instantiate handler** — add after `syncHandler := handler.NewSyncHandler(syncService, logger)` (line 148):
```go
favoritosHandler := handler.NewFavoritosHandler(favoritosRepo, syncService, logger)
```

**C. Register routes** — add after the existing fontes routes block (after line 242):
```go
r.Get("/fontes/{id}/favoritos", favoritosHandler.List)
r.Put("/fontes/{id}/favoritos", favoritosHandler.Replace)
r.Post("/sync/trigger-batch", favoritosHandler.TriggerBatch)
```

- [ ] **Step 3: Verify build**

Run: `cd backend && go build ./...`
Expected: compiles without errors

- [ ] **Step 4: Manual test with curl**

```bash
# Get favoritos (should return [])
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:9091/api/v1/fontes/$FONTE_ID/favoritos | jq

# Set favoritos
curl -s -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"project_keys":["TCDV","PLAT"]}' \
  http://localhost:9091/api/v1/fontes/$FONTE_ID/favoritos | jq

# Get again (should return ["PLAT","TCDV"])
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:9091/api/v1/fontes/$FONTE_ID/favoritos | jq

# Trigger batch (should return triggered list)
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"fonte_dados_id":"'$FONTE_ID'"}' \
  http://localhost:9091/api/v1/sync/trigger-batch | jq
```

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/favoritos.go backend/cmd/api/main.go
git commit -m "feat: add FavoritosHandler with List, Replace, and TriggerBatch endpoints"
```

---

### Task 4: Frontend — Star Toggle and Favorites Sorting

**Files:**
- Modify: `frontend/index.html` (CSS ~line 773, JS ~lines 5657-5681)

**Interfaces:**
- Consumes:
  - `GET /fontes/{id}/favoritos` → `string[]` (Task 3)
  - `PUT /fontes/{id}/favoritos` with `{"project_keys": [...]}` → `{"project_keys": [...]}` (Task 3)
  - Existing: `api(path, opts)` function, `openProjectsModal(fonteId, nome)`, `loadJiraProjects(fonteId)`, `esc(str)` HTML escaper
- Produces:
  - Modified `openProjectsModal` that stores `fonteId` in `_projectsModalFonteId` global for star toggles
  - Modified `loadJiraProjects` that fetches favorites in parallel and renders favorites-first list with stars
  - New `renderProjectsList(fonteId, projects)` function
  - New `toggleFavorito(fonteId, projectKey)` function
  - New global `_projectFavoritos` (Set of project keys) and `_projectsModalFonteId` (string)
  - New CSS classes: `.project-star`, `.project-star.active`, `.projects-divider`

- [ ] **Step 1: Add CSS for star and divider**

Insert after `.project-sync-btn { flex-shrink: 0; }` (line 773):

```css
.project-star { background:none; border:none; cursor:pointer; padding:2px; flex-shrink:0; transition:color .15s; color:var(--text-tertiary); display:inline-flex; align-items:center; }
.project-star.active { color:#F59E0B; }
.project-star:hover { color:#F59E0B; }
.projects-divider { padding:6px 14px; font-size:11px; font-weight:600; color:var(--text-tertiary); text-transform:uppercase; letter-spacing:.5px; border-bottom:1px solid var(--border-subtle); }
```

- [ ] **Step 2: Modify openProjectsModal to store fonteId**

Replace the `openProjectsModal` function (lines 5657-5662):

```javascript
var _projectsModalFonteId = '';
function openProjectsModal(fonteId, nome) {
  _projectsModalFonteId = fonteId;
  document.getElementById('projects-modal-title').textContent = 'Projetos — ' + nome;
  document.getElementById('projects-modal-content').innerHTML = '<div class="loading"><div class="spinner"></div></div>';
  document.getElementById('projects-modal').classList.add('open');
  loadJiraProjects(fonteId);
}
```

- [ ] **Step 3: Rewrite loadJiraProjects with favorites support**

Replace the `loadJiraProjects` function (lines 5665-5681):

```javascript
var _projectFavoritos = new Set();

async function loadJiraProjects(fonteId) {
  var el = document.getElementById('projects-modal-content');
  try {
    var results = await Promise.all([
      api('/sync/projects?fonte_dados_id=' + fonteId),
      api('/fontes/' + fonteId + '/favoritos')
    ]);
    var projects = results[0];
    var favKeys = results[1];
    _projectFavoritos = new Set(favKeys || []);

    if (!projects || projects.length === 0) {
      el.innerHTML = '<div class="empty-state" style="padding:30px"><div class="empty-state-text">Nenhum projeto encontrado</div></div>';
      return;
    }
    renderProjectsList(fonteId, projects);
  } catch (err) {
    el.innerHTML = '<div class="empty-state" style="padding:30px"><div class="empty-state-text">' + esc(err.message) + '</div></div>';
  }
}

function renderProjectsList(fonteId, projects) {
  var el = document.getElementById('projects-modal-content');
  var favs = projects.filter(function(p) { return _projectFavoritos.has(p.key); });
  var others = projects.filter(function(p) { return !_projectFavoritos.has(p.key); });

  var starFilled = '<svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" stroke="none"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>';
  var starEmpty = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>';

  var html = '<div class="projects-list">';

  favs.forEach(function(p) {
    html += '<div class="project-row" data-project-key="' + esc(p.key) + '">';
    html += '<button class="project-star active" onclick="toggleFavorito(\'' + fonteId + '\',\'' + esc(p.key) + '\')" title="Remover dos favoritos">' + starFilled + '</button>';
    html += '<div class="project-info"><span class="project-key">' + esc(p.key) + '</span><span class="project-name">' + esc(p.name) + '</span></div>';
    html += '<div class="project-sync-btn"><button class="btn-sm primary" onclick="syncProject(\'' + fonteId + '\',\'' + esc(p.key) + '\',this)">Sincronizar</button></div>';
    html += '</div>';
  });

  if (favs.length > 0 && others.length > 0) {
    html += '<div class="projects-divider">Outros projetos</div>';
  }

  others.forEach(function(p) {
    html += '<div class="project-row" data-project-key="' + esc(p.key) + '">';
    html += '<button class="project-star" onclick="toggleFavorito(\'' + fonteId + '\',\'' + esc(p.key) + '\')" title="Adicionar aos favoritos">' + starEmpty + '</button>';
    html += '<div class="project-info"><span class="project-key">' + esc(p.key) + '</span><span class="project-name">' + esc(p.name) + '</span></div>';
    html += '<div class="project-sync-btn"><button class="btn-sm primary" onclick="syncProject(\'' + fonteId + '\',\'' + esc(p.key) + '\',this)">Sincronizar</button></div>';
    html += '</div>';
  });

  html += '</div>';
  el.innerHTML = html;
  updateSyncFavoritosBtn();
}
```

- [ ] **Step 4: Add toggleFavorito function**

Insert after `renderProjectsList`:

```javascript
async function toggleFavorito(fonteId, projectKey) {
  if (_projectFavoritos.has(projectKey)) {
    _projectFavoritos.delete(projectKey);
  } else {
    _projectFavoritos.add(projectKey);
  }

  try {
    await api('/fontes/' + fonteId + '/favoritos', {
      method: 'PUT',
      body: JSON.stringify({ project_keys: Array.from(_projectFavoritos) })
    });
  } catch (err) {
    if (_projectFavoritos.has(projectKey)) {
      _projectFavoritos.delete(projectKey);
    } else {
      _projectFavoritos.add(projectKey);
    }
    alert('Erro ao salvar favorito: ' + err.message);
    return;
  }

  loadJiraProjects(fonteId);
}
```

- [ ] **Step 5: Manual test in browser**

1. Open Fontes de Dados page
2. Click "Sincronizar Projetos" on a fonte card
3. Verify projects list loads with empty stars
4. Click a star — verify it fills amber and project moves to top
5. Close modal, reopen — verify favorite persists at top
6. Click filled star — verify it empties and project drops to "Outros" section
7. Switch theme light/dark — verify star colors adapt

- [ ] **Step 6: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add star toggle and favorites sorting to projects modal"
```

---

### Task 5: Frontend — "Sincronizar Favoritos" Button

**Files:**
- Modify: `frontend/index.html` (modal HTML ~line 1533, JS)

**Interfaces:**
- Consumes:
  - `POST /sync/trigger-batch` with `{"fonte_dados_id": "..."}` → `{"triggered": [...], "count": N}` (Task 3)
  - `_projectFavoritos` Set and `_projectsModalFonteId` string (Task 4)
  - `updateSyncFavoritosBtn()` called from `renderProjectsList` (Task 4)
  - Existing: `startSyncPolling(fonteId, projectKey, btnEl)`, `api(path, opts)`
- Produces:
  - "Sincronizar Favoritos" button in modal header
  - `syncAllFavoritos()` function
  - `updateSyncFavoritosBtn()` function

- [ ] **Step 1: Add button to modal HTML**

Replace the projects modal HTML (lines 1531-1537):

```html
<div class="modal-overlay" id="projects-modal" onclick="if(event.target===this)closeProjectsModal()">
  <div class="modal" style="max-width:560px">
    <div style="display:flex;align-items:center;justify-content:space-between">
      <div class="modal-title" id="projects-modal-title">Projetos JIRA</div>
      <button class="btn-sm primary" id="btn-sync-favoritos" style="display:none;font-size:12px" onclick="syncAllFavoritos()">&#9733; Sincronizar Favoritos</button>
    </div>
    <div id="projects-modal-content"><div class="loading"><div class="spinner"></div></div></div>
    <div class="modal-actions"><button class="btn-cancel" type="button" onclick="closeProjectsModal()">Fechar</button></div>
  </div>
</div>
```

- [ ] **Step 2: Add updateSyncFavoritosBtn and syncAllFavoritos**

Insert after `toggleFavorito`:

```javascript
function updateSyncFavoritosBtn() {
  var btn = document.getElementById('btn-sync-favoritos');
  if (btn) {
    btn.style.display = _projectFavoritos.size > 0 ? '' : 'none';
    btn.textContent = '★ Sincronizar Favoritos (' + _projectFavoritos.size + ')';
  }
}

async function syncAllFavoritos() {
  var btn = document.getElementById('btn-sync-favoritos');
  var fonteId = _projectsModalFonteId;
  if (!fonteId || _projectFavoritos.size === 0) return;

  btn.disabled = true;
  btn.textContent = 'Sincronizando...';

  try {
    var result = await api('/sync/trigger-batch', {
      method: 'POST',
      body: JSON.stringify({ fonte_dados_id: fonteId })
    });

    if (result.triggered) {
      result.triggered.forEach(function(key) {
        var row = document.querySelector('.project-row[data-project-key="' + key + '"]');
        if (row) {
          var syncBtn = row.querySelector('.btn-sm.primary');
          if (syncBtn) {
            syncBtn.disabled = true;
            syncBtn.textContent = 'Sincronizando...';
            startSyncPolling(fonteId, key, syncBtn);
          }
        }
      });
    }
  } catch (err) {
    alert('Erro ao sincronizar favoritos: ' + err.message);
  } finally {
    btn.disabled = false;
    updateSyncFavoritosBtn();
  }
}
```

- [ ] **Step 3: Manual test**

1. Open projects modal on a fonte with 2+ favorited projects
2. Verify "★ Sincronizar Favoritos (N)" button appears in header
3. Click it — verify all favorited projects show "Sincronizando..." and fonte card stats update
4. Remove all favorites — verify button disappears
5. Add one favorite — verify button reappears with count (1)

- [ ] **Step 4: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add batch sync button for favorite projects"
```
