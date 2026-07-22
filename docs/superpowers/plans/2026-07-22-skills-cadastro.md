# Skills Cadastro Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a global skills catalog with N:N member association, managed inline on the member detail page.

**Architecture:** New `skills` + `membro_skills` tables following the `equipes`/`equipe_membros` pattern. Standalone `SkillHandler` with `SkillStore` interface + `SkillRepository` using pgxpool. Frontend autocomplete in the member detail page via separate API calls.

**Tech Stack:** Go 1.22, chi router, pgx/v5, zap logger, vanilla JS SPA

## Global Constraints

- Follow existing handler→store-interface→repository pattern (no service layer for this feature)
- Repository constructors take `*pgxpool.Pool`, handlers take `(store Store, logger *zap.Logger)`
- All routes under `/api/v1` authenticated group with `middleware.AuthJWT` + `middleware.ProjetoFilter`
- Frontend is single-file SPA at `frontend/index.html` — all changes in that one file
- Responses use existing `respondJSON` / `respondError` helpers from `handler/response.go`
- Domain structs use `json` tags with snake_case
- Error messages in Portuguese where user-facing
- UUID primary keys with `gen_random_uuid()` default
- Tests use mock store + httptest pattern (see `auth_test.go`)

---

### Task 1: Migration + Domain Struct

**Files:**
- Create: `backend/migrations/000013_skills.up.sql`
- Create: `backend/migrations/000013_skills.down.sql`
- Create: `backend/internal/domain/skill.go`

**Interfaces:**
- Consumes: nothing
- Produces: `domain.Skill` struct used by Tasks 2, 3, 4, 5

- [ ] **Step 1: Create the up migration**

Create `backend/migrations/000013_skills.up.sql`:

```sql
CREATE TABLE skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nome VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX skills_nome_lower_unique ON skills (LOWER(nome));

CREATE TABLE membro_skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    membro_id UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
    skill_id UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT membro_skills_unique UNIQUE (membro_id, skill_id)
);
```

Note: Uses a functional unique index on `LOWER(nome)` instead of a plain UNIQUE constraint, so "golang" and "Golang" are treated as the same skill. The first insertion wins and preserves its original casing.

- [ ] **Step 2: Create the down migration**

Create `backend/migrations/000013_skills.down.sql`:

```sql
DROP TABLE IF EXISTS membro_skills;
DROP TABLE IF EXISTS skills;
```

- [ ] **Step 3: Create the domain struct**

Create `backend/internal/domain/skill.go`:

```go
package domain

import (
	"time"

	"github.com/google/uuid"
)

type Skill struct {
	ID        uuid.UUID `json:"id"`
	Nome      string    `json:"nome"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
```

- [ ] **Step 4: Verify build**

Run: `cd backend && go build ./...`
Expected: clean build, no errors

- [ ] **Step 5: Commit**

```bash
git add backend/migrations/000013_skills.up.sql backend/migrations/000013_skills.down.sql backend/internal/domain/skill.go
git commit -m "feat: add skills migration and domain struct"
```

---

### Task 2: Repository

**Files:**
- Create: `backend/internal/repository/skill.go`

**Interfaces:**
- Consumes: `domain.Skill` from Task 1
- Produces: `SkillRepository` with methods: `List(ctx, query string) ([]domain.Skill, error)`, `GetByID(ctx, id uuid.UUID) (*domain.Skill, error)`, `Create(ctx, nome string) (*domain.Skill, error)`, `Delete(ctx, id uuid.UUID) error`, `GetMembroSkills(ctx, membroID uuid.UUID) ([]domain.Skill, error)`, `AddMembroSkill(ctx, membroID, skillID uuid.UUID) error`, `RemoveMembroSkill(ctx, membroID, skillID uuid.UUID) error`

- [ ] **Step 1: Create repository file**

Create `backend/internal/repository/skill.go`:

```go
package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

type SkillRepository struct {
	pool *pgxpool.Pool
}

func NewSkillRepository(pool *pgxpool.Pool) *SkillRepository {
	return &SkillRepository{pool: pool}
}

func (r *SkillRepository) List(ctx context.Context, query string) ([]domain.Skill, error) {
	var rows pgx.Rows
	var err error
	if query == "" {
		rows, err = r.pool.Query(ctx, `
			SELECT id, nome, created_at, updated_at FROM skills ORDER BY nome
		`)
	} else {
		rows, err = r.pool.Query(ctx, `
			SELECT id, nome, created_at, updated_at FROM skills
			WHERE nome ILIKE '%' || $1 || '%'
			ORDER BY nome
		`, query)
	}
	if err != nil {
		return nil, fmt.Errorf("listing skills: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Skill, 0)
	for rows.Next() {
		var s domain.Skill
		if err := rows.Scan(&s.ID, &s.Nome, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning skill: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *SkillRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Skill, error) {
	var s domain.Skill
	err := r.pool.QueryRow(ctx, `
		SELECT id, nome, created_at, updated_at FROM skills WHERE id = $1
	`, id).Scan(&s.ID, &s.Nome, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting skill: %w", err)
	}
	return &s, nil
}

func (r *SkillRepository) Create(ctx context.Context, nome string) (*domain.Skill, error) {
	var s domain.Skill
	err := r.pool.QueryRow(ctx, `
		INSERT INTO skills (nome) VALUES ($1)
		ON CONFLICT ((LOWER(nome))) DO UPDATE SET updated_at = skills.updated_at
		RETURNING id, nome, created_at, updated_at
	`, nome).Scan(&s.ID, &s.Nome, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating skill: %w", err)
	}
	return &s, nil
}

func (r *SkillRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM skills WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting skill: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("skill %s not found", id)
	}
	return nil
}

func (r *SkillRepository) GetMembroSkills(ctx context.Context, membroID uuid.UUID) ([]domain.Skill, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.nome, s.created_at, s.updated_at
		FROM skills s
		INNER JOIN membro_skills ms ON ms.skill_id = s.id
		WHERE ms.membro_id = $1
		ORDER BY s.nome
	`, membroID)
	if err != nil {
		return nil, fmt.Errorf("getting membro skills: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Skill, 0)
	for rows.Next() {
		var s domain.Skill
		if err := rows.Scan(&s.ID, &s.Nome, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning membro skill: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *SkillRepository) AddMembroSkill(ctx context.Context, membroID, skillID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO membro_skills (membro_id, skill_id) VALUES ($1, $2)
		ON CONFLICT (membro_id, skill_id) DO NOTHING
	`, membroID, skillID)
	if err != nil {
		return fmt.Errorf("adding skill to membro: %w", err)
	}
	return nil
}

func (r *SkillRepository) RemoveMembroSkill(ctx context.Context, membroID, skillID uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `
		DELETE FROM membro_skills WHERE membro_id = $1 AND skill_id = $2
	`, membroID, skillID)
	if err != nil {
		return fmt.Errorf("removing skill from membro: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("skill %s not associated with membro %s", skillID, membroID)
	}
	return nil
}
```

- [ ] **Step 2: Verify build**

Run: `cd backend && go build ./...`
Expected: clean build, no errors

- [ ] **Step 3: Commit**

```bash
git add backend/internal/repository/skill.go
git commit -m "feat: add skill repository with catalog and member association queries"
```

---

### Task 3: Handler + Tests

**Files:**
- Create: `backend/internal/handler/skill.go`
- Create: `backend/internal/handler/skill_test.go`

**Interfaces:**
- Consumes: `domain.Skill` from Task 1; repository method signatures from Task 2 (via `SkillStore` interface)
- Produces: `SkillHandler` with methods: `List`, `Create`, `Delete`, `GetMembroSkills`, `AddMembroSkill`, `RemoveMembroSkill`; `NewSkillHandler(store SkillStore, logger *zap.Logger) *SkillHandler`

- [ ] **Step 1: Write the handler test file with mock store**

Create `backend/internal/handler/skill_test.go`:

```go
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"go.uber.org/zap"
)

type mockSkillStore struct {
	listFn             func(ctx context.Context, query string) ([]domain.Skill, error)
	getByIDFn          func(ctx context.Context, id uuid.UUID) (*domain.Skill, error)
	createFn           func(ctx context.Context, nome string) (*domain.Skill, error)
	deleteFn           func(ctx context.Context, id uuid.UUID) error
	getMembroSkillsFn  func(ctx context.Context, membroID uuid.UUID) ([]domain.Skill, error)
	addMembroSkillFn   func(ctx context.Context, membroID, skillID uuid.UUID) error
	removeMembroSkillFn func(ctx context.Context, membroID, skillID uuid.UUID) error
}

func (m *mockSkillStore) List(ctx context.Context, query string) ([]domain.Skill, error) {
	return m.listFn(ctx, query)
}
func (m *mockSkillStore) GetByID(ctx context.Context, id uuid.UUID) (*domain.Skill, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockSkillStore) Create(ctx context.Context, nome string) (*domain.Skill, error) {
	return m.createFn(ctx, nome)
}
func (m *mockSkillStore) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteFn(ctx, id)
}
func (m *mockSkillStore) GetMembroSkills(ctx context.Context, membroID uuid.UUID) ([]domain.Skill, error) {
	return m.getMembroSkillsFn(ctx, membroID)
}
func (m *mockSkillStore) AddMembroSkill(ctx context.Context, membroID, skillID uuid.UUID) error {
	return m.addMembroSkillFn(ctx, membroID, skillID)
}
func (m *mockSkillStore) RemoveMembroSkill(ctx context.Context, membroID, skillID uuid.UUID) error {
	return m.removeMembroSkillFn(ctx, membroID, skillID)
}

func newTestSkillHandler(store *mockSkillStore) *SkillHandler {
	return NewSkillHandler(store, zap.NewNop())
}

func TestSkillList_NoQuery(t *testing.T) {
	skills := []domain.Skill{{ID: uuid.New(), Nome: "golang"}, {ID: uuid.New(), Nome: "python"}}
	store := &mockSkillStore{
		listFn: func(_ context.Context, q string) ([]domain.Skill, error) {
			if q != "" {
				t.Errorf("expected empty query, got %q", q)
			}
			return skills, nil
		},
	}
	h := newTestSkillHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/skills", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var got []domain.Skill
	json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 2 {
		t.Errorf("got %d skills, want 2", len(got))
	}
}

func TestSkillList_WithQuery(t *testing.T) {
	store := &mockSkillStore{
		listFn: func(_ context.Context, q string) ([]domain.Skill, error) {
			if q != "go" {
				t.Errorf("expected query 'go', got %q", q)
			}
			return []domain.Skill{{ID: uuid.New(), Nome: "golang"}}, nil
		},
	}
	h := newTestSkillHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/skills?q=go", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var got []domain.Skill
	json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 1 {
		t.Errorf("got %d skills, want 1", len(got))
	}
}

func TestSkillCreate_Valid(t *testing.T) {
	created := domain.Skill{ID: uuid.New(), Nome: "golang"}
	store := &mockSkillStore{
		createFn: func(_ context.Context, nome string) (*domain.Skill, error) {
			if nome != "golang" {
				t.Errorf("expected nome 'golang', got %q", nome)
			}
			return &created, nil
		},
	}
	h := newTestSkillHandler(store)

	body := `{"nome":"  golang  "}`
	req := httptest.NewRequest("POST", "/api/v1/skills", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestSkillCreate_EmptyName(t *testing.T) {
	store := &mockSkillStore{}
	h := newTestSkillHandler(store)

	body := `{"nome":"   "}`
	req := httptest.NewRequest("POST", "/api/v1/skills", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestSkillCreate_TooLong(t *testing.T) {
	store := &mockSkillStore{}
	h := newTestSkillHandler(store)

	longName := make([]byte, 101)
	for i := range longName {
		longName[i] = 'a'
	}
	body := `{"nome":"` + string(longName) + `"}`
	req := httptest.NewRequest("POST", "/api/v1/skills", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestSkillDelete_Valid(t *testing.T) {
	skillID := uuid.New()
	store := &mockSkillStore{
		deleteFn: func(_ context.Context, id uuid.UUID) error {
			if id != skillID {
				t.Errorf("expected id %s, got %s", skillID, id)
			}
			return nil
		},
	}
	h := newTestSkillHandler(store)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", skillID.String())
	req := httptest.NewRequest("DELETE", "/api/v1/skills/"+skillID.String(), nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestSkillDelete_InvalidID(t *testing.T) {
	store := &mockSkillStore{}
	h := newTestSkillHandler(store)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "not-a-uuid")
	req := httptest.NewRequest("DELETE", "/api/v1/skills/not-a-uuid", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestSkillDelete_NotFound(t *testing.T) {
	store := &mockSkillStore{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return fmt.Errorf("skill not found")
		},
	}
	h := newTestSkillHandler(store)

	skillID := uuid.New()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", skillID.String())
	req := httptest.NewRequest("DELETE", "/api/v1/skills/"+skillID.String(), nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestGetMembroSkills(t *testing.T) {
	membroID := uuid.New()
	skills := []domain.Skill{{ID: uuid.New(), Nome: "golang"}}
	store := &mockSkillStore{
		getMembroSkillsFn: func(_ context.Context, id uuid.UUID) ([]domain.Skill, error) {
			if id != membroID {
				t.Errorf("expected membroID %s, got %s", membroID, id)
			}
			return skills, nil
		},
	}
	h := newTestSkillHandler(store)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", membroID.String())
	req := httptest.NewRequest("GET", "/api/v1/membros/"+membroID.String()+"/skills", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.GetMembroSkills(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var got []domain.Skill
	json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 1 || got[0].Nome != "golang" {
		t.Errorf("unexpected skills: %+v", got)
	}
}

func TestAddMembroSkill_Valid(t *testing.T) {
	membroID := uuid.New()
	skillID := uuid.New()
	store := &mockSkillStore{
		addMembroSkillFn: func(_ context.Context, mID, sID uuid.UUID) error {
			if mID != membroID || sID != skillID {
				t.Errorf("unexpected IDs: membro=%s skill=%s", mID, sID)
			}
			return nil
		},
	}
	h := newTestSkillHandler(store)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", membroID.String())
	body := `{"skill_id":"` + skillID.String() + `"}`
	req := httptest.NewRequest("POST", "/api/v1/membros/"+membroID.String()+"/skills", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.AddMembroSkill(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestAddMembroSkill_EmptySkillID(t *testing.T) {
	store := &mockSkillStore{}
	h := newTestSkillHandler(store)

	membroID := uuid.New()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", membroID.String())
	body := `{"skill_id":""}`
	req := httptest.NewRequest("POST", "/api/v1/membros/"+membroID.String()+"/skills", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.AddMembroSkill(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestRemoveMembroSkill_Valid(t *testing.T) {
	membroID := uuid.New()
	skillID := uuid.New()
	store := &mockSkillStore{
		removeMembroSkillFn: func(_ context.Context, mID, sID uuid.UUID) error {
			if mID != membroID || sID != skillID {
				t.Errorf("unexpected IDs")
			}
			return nil
		},
	}
	h := newTestSkillHandler(store)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", membroID.String())
	rctx.URLParams.Add("skillId", skillID.String())
	req := httptest.NewRequest("DELETE", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.RemoveMembroSkill(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/handler/ -run TestSkill -v`
Expected: compilation errors — `SkillHandler`, `NewSkillHandler` not defined yet

- [ ] **Step 3: Write the handler**

Create `backend/internal/handler/skill.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"go.uber.org/zap"
)

type SkillStore interface {
	List(ctx context.Context, query string) ([]domain.Skill, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Skill, error)
	Create(ctx context.Context, nome string) (*domain.Skill, error)
	Delete(ctx context.Context, id uuid.UUID) error
	GetMembroSkills(ctx context.Context, membroID uuid.UUID) ([]domain.Skill, error)
	AddMembroSkill(ctx context.Context, membroID, skillID uuid.UUID) error
	RemoveMembroSkill(ctx context.Context, membroID, skillID uuid.UUID) error
}

type SkillHandler struct {
	store  SkillStore
	logger *zap.Logger
}

func NewSkillHandler(store SkillStore, logger *zap.Logger) *SkillHandler {
	return &SkillHandler{store: store, logger: logger}
}

func (h *SkillHandler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	skills, err := h.store.List(r.Context(), query)
	if err != nil {
		h.logger.Error("failed to list skills", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar skills")
		return
	}
	respondJSON(w, http.StatusOK, skills)
}

func (h *SkillHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nome string `json:"nome"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo invalido")
		return
	}
	req.Nome = strings.TrimSpace(req.Nome)
	if req.Nome == "" {
		respondError(w, http.StatusBadRequest, "nome e obrigatorio")
		return
	}
	if len(req.Nome) > 100 {
		respondError(w, http.StatusBadRequest, "nome deve ter no maximo 100 caracteres")
		return
	}
	skill, err := h.store.Create(r.Context(), req.Nome)
	if err != nil {
		h.logger.Error("failed to create skill", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao criar skill")
		return
	}
	respondJSON(w, http.StatusOK, skill)
}

func (h *SkillHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id invalido")
		return
	}
	if err := h.store.Delete(r.Context(), id); err != nil {
		h.logger.Error("failed to delete skill", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao excluir skill")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "skill excluida"})
}

func (h *SkillHandler) GetMembroSkills(w http.ResponseWriter, r *http.Request) {
	membroID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id invalido")
		return
	}
	skills, err := h.store.GetMembroSkills(r.Context(), membroID)
	if err != nil {
		h.logger.Error("failed to get membro skills", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar skills do membro")
		return
	}
	respondJSON(w, http.StatusOK, skills)
}

func (h *SkillHandler) AddMembroSkill(w http.ResponseWriter, r *http.Request) {
	membroID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id invalido")
		return
	}
	var req struct {
		SkillID string `json:"skill_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo invalido")
		return
	}
	skillID, err := uuid.Parse(req.SkillID)
	if err != nil || skillID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "skill_id invalido")
		return
	}
	if err := h.store.AddMembroSkill(r.Context(), membroID, skillID); err != nil {
		h.logger.Error("failed to add skill to membro", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao associar skill")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "skill associada"})
}

func (h *SkillHandler) RemoveMembroSkill(w http.ResponseWriter, r *http.Request) {
	membroID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id invalido")
		return
	}
	skillID, err := uuid.Parse(chi.URLParam(r, "skillId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "skill_id invalido")
		return
	}
	if err := h.store.RemoveMembroSkill(r.Context(), membroID, skillID); err != nil {
		h.logger.Error("failed to remove skill from membro", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao desassociar skill")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "skill desassociada"})
}
```

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./internal/handler/ -run TestSkill -v`
Expected: all `TestSkill*` tests PASS

- [ ] **Step 5: Run full test suite**

Run: `cd backend && go test ./...`
Expected: all tests pass

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handler/skill.go backend/internal/handler/skill_test.go
git commit -m "feat: add skill handler with catalog and member association endpoints"
```

---

### Task 4: Wiring (main.go routes)

**Files:**
- Modify: `backend/cmd/api/main.go`

**Interfaces:**
- Consumes: `repository.NewSkillRepository(pool)` from Task 2, `handler.NewSkillHandler(store, logger)` from Task 3
- Produces: Registered routes accessible via HTTP

- [ ] **Step 1: Add skill repository + handler instantiation**

In `backend/cmd/api/main.go`, find the section where repositories and handlers are instantiated (look for `equipeRepo :=` and `equipeHandler :=`). Add after the existing handler instantiations:

```go
skillRepo := repository.NewSkillRepository(pool)
skillHandler := handler.NewSkillHandler(skillRepo, logger)
```

- [ ] **Step 2: Register skill catalog routes**

In the authenticated route group, after the membro routes block (after the `r.Put("/membros/{id}/desligamento", membroHandler.UpdateDataDesligamento)` line), add:

```go
r.Get("/skills", skillHandler.List)
r.Post("/skills", skillHandler.Create)
r.Delete("/skills/{id}", skillHandler.Delete)

r.Get("/membros/{id}/skills", skillHandler.GetMembroSkills)
r.Post("/membros/{id}/skills", skillHandler.AddMembroSkill)
r.Delete("/membros/{id}/skills/{skillId}", skillHandler.RemoveMembroSkill)
```

- [ ] **Step 3: Verify build**

Run: `cd backend && go build ./...`
Expected: clean build, no errors

- [ ] **Step 4: Run full test suite**

Run: `cd backend && go test ./...`
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add backend/cmd/api/main.go
git commit -m "feat: wire skill handler and register skill routes"
```

---

### Task 5: Frontend — Skills Section in Member Detail

**Files:**
- Modify: `frontend/index.html`

**Interfaces:**
- Consumes: API endpoints from Tasks 3-4: `GET /api/v1/skills?q=`, `POST /api/v1/skills`, `GET /api/v1/membros/{id}/skills`, `POST /api/v1/membros/{id}/skills`, `DELETE /api/v1/membros/{id}/skills/{skillId}`
- Produces: Skills section in member detail page with badges, autocomplete input, add/remove functionality

- [ ] **Step 1: Add CSS styles**

In the `<style>` section of `frontend/index.html`, after the `.membro-detail-info .team-badge` rule (around line 516), add:

```css
.skill-section { margin-bottom: 16px; }
.skill-badges { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
.skill-badge { display: inline-flex; align-items: center; gap: 4px; background: var(--accent-soft); color: var(--accent); border-radius: 12px; padding: 2px 10px; font-size: 12px; font-weight: 600; }
.skill-badge .skill-remove { cursor: pointer; opacity: .6; font-size: 14px; line-height: 1; }
.skill-badge .skill-remove:hover { opacity: 1; }
.skill-add-btn { cursor: pointer; width: 22px; height: 22px; border-radius: 50%; border: 1.5px dashed var(--border); background: none; color: var(--text-secondary); font-size: 14px; display: inline-flex; align-items: center; justify-content: center; transition: border-color .15s, color .15s; }
.skill-add-btn:hover { border-color: var(--accent); color: var(--accent); }
.skill-input-wrap { position: relative; display: inline-block; margin-top: 6px; }
.skill-input-wrap input { padding: 4px 10px; font-size: 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--card-bg); color: var(--text-primary); width: 200px; }
.skill-dropdown { position: absolute; top: 100%; left: 0; width: 200px; max-height: 160px; overflow-y: auto; background: var(--card-bg); border: 1px solid var(--border); border-radius: 6px; margin-top: 2px; z-index: 100; display: none; }
.skill-dropdown.open { display: block; }
.skill-dropdown-item { padding: 6px 10px; font-size: 12px; cursor: pointer; color: var(--text-primary); }
.skill-dropdown-item:hover { background: var(--accent-soft); }
.skill-dropdown-item.create { color: var(--accent); font-style: italic; }
```

- [ ] **Step 2: Insert skills section in loadMembroDetail**

In the `loadMembroDetail` function (around line 2069), find the string that builds the desligamento section. The skills section goes between the desligamento `</div>` and the period chips `<div class="membro-detail-period-chips"`. Insert after the desligamento closing `</div>` (the line ending with `') +`):

```javascript
      '<div class="skill-section" id="membro-skills-section"></div>' +
```

This inserts an empty container that `loadMembroSkills` will populate after the main page renders.

Then, at the end of `loadMembroDetail`'s try block, after the period chips event listener setup (the `document.querySelectorAll('#membro-period-chips .chip').forEach(...)` block), add:

```javascript
    loadMembroSkills(membroId);
```

- [ ] **Step 3: Add JavaScript functions**

At the end of the `<script>` section (before the closing `</script>` tag), add:

```javascript
let skillSearchTimer = null;

async function loadMembroSkills(membroId) {
  const section = document.getElementById('membro-skills-section');
  if (!section) return;
  try {
    const skills = await api('/membros/' + membroId + '/skills');
    renderMembroSkills(skills, membroId, section);
  } catch (err) {
    section.innerHTML = '';
  }
}

function renderMembroSkills(skills, membroId, container) {
  let html = '<div class="skill-badges">';
  skills.forEach(function(s) {
    html += '<span class="skill-badge">' + esc(s.nome) +
      '<span class="skill-remove" onclick="removeSkillFromMembro(\'' + membroId + '\',\'' + s.id + '\')">&times;</span></span>';
  });
  if (skills.length === 0) {
    html += '<span style="font-size:12px;color:var(--text-secondary)">Nenhuma skill cadastrada</span>';
  }
  html += '<button class="skill-add-btn" onclick="showSkillInput(\'' + membroId + '\')" title="Adicionar skill">+</button>';
  html += '</div>';
  html += '<div class="skill-input-wrap" id="skill-input-wrap-' + membroId + '" style="display:none">' +
    '<input type="text" id="skill-input-' + membroId + '" placeholder="Adicionar skill..." ' +
    'oninput="searchSkills(\'' + membroId + '\')" onkeydown="skillInputKeydown(event,\'' + membroId + '\')">' +
    '<div class="skill-dropdown" id="skill-dropdown-' + membroId + '"></div></div>';
  container.innerHTML = html;
}

function showSkillInput(membroId) {
  var wrap = document.getElementById('skill-input-wrap-' + membroId);
  wrap.style.display = 'inline-block';
  var inp = document.getElementById('skill-input-' + membroId);
  inp.value = '';
  inp.focus();
}

function searchSkills(membroId) {
  clearTimeout(skillSearchTimer);
  var inp = document.getElementById('skill-input-' + membroId);
  var q = inp.value.trim();
  var dd = document.getElementById('skill-dropdown-' + membroId);
  if (q.length === 0) { dd.classList.remove('open'); return; }
  skillSearchTimer = setTimeout(async function() {
    try {
      var skills = await api('/skills?q=' + encodeURIComponent(q));
      var html = '';
      skills.forEach(function(s) {
        html += '<div class="skill-dropdown-item" onclick="addSkillToMembro(\'' + membroId + '\',\'' + s.id + '\')">' + esc(s.nome) + '</div>';
      });
      var exactMatch = skills.some(function(s) { return s.nome.toLowerCase() === q.toLowerCase(); });
      if (!exactMatch && q.length > 0) {
        html += '<div class="skill-dropdown-item create" onclick="createAndAddSkill(\'' + membroId + '\',\'' + esc(q) + '\')">+ Criar "' + esc(q) + '"</div>';
      }
      dd.innerHTML = html;
      dd.classList.add('open');
    } catch (err) {
      dd.classList.remove('open');
    }
  }, 300);
}

function skillInputKeydown(e, membroId) {
  if (e.key === 'Escape') {
    var wrap = document.getElementById('skill-input-wrap-' + membroId);
    wrap.style.display = 'none';
    document.getElementById('skill-dropdown-' + membroId).classList.remove('open');
  }
  if (e.key === 'Enter') {
    var dd = document.getElementById('skill-dropdown-' + membroId);
    var createItem = dd.querySelector('.skill-dropdown-item.create');
    if (createItem) createItem.click();
  }
}

async function addSkillToMembro(membroId, skillId) {
  try {
    await api('/membros/' + membroId + '/skills', { method: 'POST', body: JSON.stringify({ skill_id: skillId }) });
    var wrap = document.getElementById('skill-input-wrap-' + membroId);
    wrap.style.display = 'none';
    document.getElementById('skill-dropdown-' + membroId).classList.remove('open');
    loadMembroSkills(membroId);
  } catch (err) {
    alert('Falha ao associar skill');
  }
}

async function createAndAddSkill(membroId, nome) {
  try {
    var skill = await api('/skills', { method: 'POST', body: JSON.stringify({ nome: nome }) });
    await api('/membros/' + membroId + '/skills', { method: 'POST', body: JSON.stringify({ skill_id: skill.id }) });
    var wrap = document.getElementById('skill-input-wrap-' + membroId);
    wrap.style.display = 'none';
    document.getElementById('skill-dropdown-' + membroId).classList.remove('open');
    loadMembroSkills(membroId);
  } catch (err) {
    alert('Falha ao criar skill');
  }
}

async function removeSkillFromMembro(membroId, skillId) {
  try {
    await api('/membros/' + membroId + '/skills/' + skillId, { method: 'DELETE' });
    loadMembroSkills(membroId);
  } catch (err) {
    alert('Falha ao remover skill');
  }
}
```

- [ ] **Step 4: Test in browser**

1. Open the app, navigate to Equipes, click a member to open detail
2. Verify the skills section appears between desligamento and period chips
3. Verify "Nenhuma skill cadastrada" text and "+" button appear
4. Click "+", type a skill name, verify dropdown appears with "Criar" option
5. Click "Criar", verify badge appears
6. Type partial name, verify autocomplete shows existing skill
7. Click autocomplete suggestion, verify badge appears (no duplicate)
8. Click "×" on a badge, verify it's removed
9. Press Escape while input is open, verify it closes

- [ ] **Step 5: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add skills section with autocomplete in member detail page"
```
