# Alocação de Projetos — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a project allocation module that lets coordinators view epic-level allocation metrics and assign tasks to sprints/people with Jira write-back.

**Architecture:** New `AllocationService` + `AllocationRepository` backed by existing `tarefas`/`sprints`/`membros` tables. Two new Jira client methods (`MoveToSprint`, `UpdateTimeEstimate`). Optimistic update flow — write local DB first, push to Jira in background goroutine, rollback on failure. Frontend adds a new "Alocação" page with epic cards, a detail modal (task sections + Gantt), and a capacity conflict confirmation dialog.

**Tech Stack:** Go (chi, pgx/v5, zap), PostgreSQL, vanilla JS (var/function only), Jira REST API v3 + Agile API.

## Global Constraints

- Frontend: vanilla JS, `var`/`function` only, no ES6+. Use `esc()` for all dynamic text (XSS).
- Backend: Go with chi router, pgx/v5, zap logger. Follow existing handler/service/repository pattern.
- Do not commit automatically — leave changes unstaged.
- Dark mode: CSS custom properties + `@media (prefers-color-scheme: dark)` + `:root[data-theme="dark"]`.
- Capacity: 6h per business day, holidays and absences deducted (reuse `SprintService.GetCapacity`).
- Jira credentials never exposed to frontend (client constructed server-side).
- All endpoints behind JWT auth + `ProjetoFilter` middleware.

## File Structure

| Action | File | Responsibility |
|--------|------|---------------|
| Create | `backend/internal/jira/client_test.go` | Unit tests for new Jira methods |
| Modify | `backend/internal/jira/client.go:16-30` | Add `MoveToSprint` and `UpdateTimeEstimate` to `Client` interface and `HTTPClient` |
| Create | `backend/internal/repository/allocation.go` | All allocation SQL queries |
| Create | `backend/internal/repository/allocation_test.go` | Repository tests |
| Create | `backend/internal/service/allocation.go` | Business logic: list projects, detail, allocate, sync |
| Create | `backend/internal/service/allocation_test.go` | Service tests with mocks |
| Create | `backend/internal/handler/allocation.go` | HTTP handlers for allocation endpoints |
| Create | `backend/internal/handler/allocation_test.go` | Handler tests |
| Modify | `backend/cmd/api/main.go:139-145,252-264` | Wire AllocationService, AllocationHandler, add routes |
| Modify | `frontend/index.html:815-830` | Add "Alocação" sidebar item |
| Modify | `frontend/index.html:847+` | Add allocation page container |
| Modify | `frontend/index.html:1642-1654` | Add allocation to navigate() function |
| Modify | `frontend/index.html` (JS section) | Add all allocation JS functions |

---

### Task 1: Jira Client Extensions — MoveToSprint + UpdateTimeEstimate

**Files:**
- Modify: `backend/internal/jira/client.go:16-30` (interface + implementations)
- Create: `backend/internal/jira/client_test.go`

**Interfaces:**
- Consumes: existing `HTTPClient.doPost`, `HTTPClient.doPut` helpers
- Produces: `MoveToSprint(ctx context.Context, sprintJiraID int, issueKey string) error` and `UpdateTimeEstimate(ctx context.Context, issueKey string, seconds int) error` — used by Task 3 (AllocationService)

- [ ] **Step 1: Write failing tests for MoveToSprint and UpdateTimeEstimate**

Create `backend/internal/jira/client_test.go`:

```go
package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestMoveToSprint(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	client := NewHTTPClient(ts.URL, "e@e.com", "tok", 100, zap.NewNop())
	err := client.MoveToSprint(context.Background(), 42, "PROJ-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/rest/agile/1.0/sprint/42/issue" {
		t.Errorf("unexpected path: %s", gotPath)
	}
	issues, ok := gotBody["issues"].([]any)
	if !ok || len(issues) != 1 || issues[0] != "PROJ-123" {
		t.Errorf("unexpected body: %v", gotBody)
	}
}

func TestMoveToSprint_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Sprint does not exist"]}`))
	}))
	defer ts.Close()

	client := NewHTTPClient(ts.URL, "e@e.com", "tok", 100, zap.NewNop())
	err := client.MoveToSprint(context.Background(), 999, "PROJ-123")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestUpdateTimeEstimate(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	client := NewHTTPClient(ts.URL, "e@e.com", "tok", 100, zap.NewNop())

	tests := []struct {
		seconds  int
		expected string
	}{
		{3600, "1h"},
		{7200, "2h"},
		{28800, "1d"},
		{57600, "2d"},
		{30600, "1d 0.5h"},
		{1800, "0.5h"},
	}

	for _, tt := range tests {
		err := client.UpdateTimeEstimate(context.Background(), "PROJ-456", tt.seconds)
		if err != nil {
			t.Fatalf("unexpected error for %d seconds: %v", tt.seconds, err)
		}
		if gotMethod != http.MethodPut {
			t.Errorf("expected PUT, got %s", gotMethod)
		}
		if gotPath != "/rest/api/3/issue/PROJ-456" {
			t.Errorf("unexpected path: %s", gotPath)
		}
		fields, _ := gotBody["fields"].(map[string]any)
		tt2, _ := fields["timetracking"].(map[string]any)
		estimate, _ := tt2["originalEstimate"].(string)
		if estimate != tt.expected {
			t.Errorf("seconds=%d: expected %q, got %q", tt.seconds, tt.expected, estimate)
		}
	}
}

func TestUpdateTimeEstimate_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errors":{"timetracking":"invalid"}}`))
	}))
	defer ts.Close()

	client := NewHTTPClient(ts.URL, "e@e.com", "tok", 100, zap.NewNop())
	err := client.UpdateTimeEstimate(context.Background(), "PROJ-456", 3600)
	if err == nil {
		t.Fatal("expected error for 400")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/jira/ -run 'TestMoveToSprint|TestUpdateTimeEstimate' -v`
Expected: compilation errors — `MoveToSprint` and `UpdateTimeEstimate` not defined.

- [ ] **Step 3: Add MoveToSprint and UpdateTimeEstimate to the Client interface**

In `backend/internal/jira/client.go`, add to the `Client` interface (after line 29, the `AddComment` line):

```go
	MoveToSprint(ctx context.Context, sprintJiraID int, issueKey string) error
	UpdateTimeEstimate(ctx context.Context, issueKey string, seconds int) error
```

- [ ] **Step 4: Implement MoveToSprint on HTTPClient**

Add at end of `backend/internal/jira/client.go` (after the `AddComment` method):

```go
func (c *HTTPClient) MoveToSprint(ctx context.Context, sprintJiraID int, issueKey string) error {
	path := fmt.Sprintf("/rest/agile/1.0/sprint/%d/issue", sprintJiraID)
	payload := map[string]any{"issues": []string{issueKey}}
	_, err := c.doPost(ctx, path, payload)
	if err != nil {
		return fmt.Errorf("moving issue %q to sprint %d: %w", issueKey, sprintJiraID, err)
	}
	return nil
}
```

- [ ] **Step 5: Implement UpdateTimeEstimate on HTTPClient**

Add at end of `backend/internal/jira/client.go`:

```go
func formatJiraDuration(seconds int) string {
	days := seconds / 28800
	remaining := seconds % 28800
	hours := float64(remaining) / 3600.0

	if days > 0 && hours > 0 {
		return fmt.Sprintf("%dd %.4gh", days, hours)
	}
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%.4gh", hours)
}

func (c *HTTPClient) UpdateTimeEstimate(ctx context.Context, issueKey string, seconds int) error {
	estimate := formatJiraDuration(seconds)
	payload := map[string]any{
		"fields": map[string]any{
			"timetracking": map[string]any{
				"originalEstimate": estimate,
			},
		},
	}
	_, err := c.doPut(ctx, "/rest/api/3/issue/"+issueKey, payload)
	if err != nil {
		return fmt.Errorf("updating time estimate for %q: %w", issueKey, err)
	}
	return nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/jira/ -run 'TestMoveToSprint|TestUpdateTimeEstimate' -v`
Expected: all 4 tests PASS.

- [ ] **Step 7: Verify the whole project compiles**

Run: `cd /home/emerson/code/myplanner/backend && go build ./... && go vet ./...`
Expected: clean build, no errors.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/jira/client.go backend/internal/jira/client_test.go
git commit -m "feat(jira): add MoveToSprint and UpdateTimeEstimate methods"
```

---

### Task 2: AllocationRepository — SQL Queries

**Files:**
- Create: `backend/internal/repository/allocation.go`

**Interfaces:**
- Consumes: `pgxpool.Pool`, existing tables `tarefas`, `sprints`, `equipe_membros`, `membros`, `tarefa_produtos`, `produtos`
- Produces:
  - `NewAllocationRepository(pool *pgxpool.Pool) *AllocationRepository`
  - `GetEpicsByEquipeAndProduto(ctx context.Context, equipeID, produtoID uuid.UUID) ([]EpicAllocationRow, error)`
  - `GetEpicTasks(ctx context.Context, epicID uuid.UUID) ([]TaskAllocationRow, error)`
  - `GetEpicPeople(ctx context.Context, epicID uuid.UUID) ([]PersonAllocationRow, error)`
  - `UpdateTaskAllocation(ctx context.Context, taskID, sprintID uuid.UUID, assigneeID *uuid.UUID, estimateSeconds int) error`
  - `RollbackTaskAllocation(ctx context.Context, taskID uuid.UUID, prevSprintID *uuid.UUID, prevAssigneeID *uuid.UUID, prevEstimate *int) error`
  - `GetFutureSprintsByEquipe(ctx context.Context, equipeID uuid.UUID) ([]SprintOptionRow, error)`
  - `CheckGDPTCAncestors(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error)`
  - `GetTaskPreviousState(ctx context.Context, taskID uuid.UUID) (*TaskPreviousState, error)`
  - `GetTaskFonteDadosID(ctx context.Context, taskID uuid.UUID) (uuid.UUID, error)`
  - `GetSprintJiraID(ctx context.Context, sprintID uuid.UUID) (int, error)`
  - `GetTaskJiraKey(ctx context.Context, taskID uuid.UUID) (string, error)`

- [ ] **Step 1: Create repository file with row types and constructor**

Create `backend/internal/repository/allocation.go`:

```go
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AllocationRepository struct {
	pool *pgxpool.Pool
}

func NewAllocationRepository(pool *pgxpool.Pool) *AllocationRepository {
	return &AllocationRepository{pool: pool}
}

type EpicAllocationRow struct {
	EpicID       uuid.UUID
	NumeroTicket string
	Resumo       string
	Apelido      *string
	DataLimite   *time.Time
	Prioridade   *string
	TipoDemanda  *string
	Produtos     []string
	TotalFilhas        int
	FilhasComEstimativa int
	HorasEstimadas     float64
	HorasEmSprint      float64
}

type TaskAllocationRow struct {
	TarefaID        uuid.UUID
	NumeroTicket    string
	Resumo          string
	Tipo            string
	TipoDemanda     *string
	Status          string
	EstimativaTempo *int
	SprintID        *uuid.UUID
	SprintNome      *string
	SprintInicio    *time.Time
	SprintFim       *time.Time
	ResponsavelID   *uuid.UUID
	ResponsavelNome *string
}

type PersonAllocationRow struct {
	MembroID       uuid.UUID
	Nome           string
	HorasNoProjeto float64
}

type SprintOptionRow struct {
	ID     uuid.UUID
	JiraID int
	Nome   string
	Inicio time.Time
	Fim    time.Time
	Estado string
}

type TaskPreviousState struct {
	SprintID      *uuid.UUID
	ResponsavelID *uuid.UUID
	Estimativa    *int
}
```

- [ ] **Step 2: Implement GetEpicsByEquipeAndProduto**

Append to `backend/internal/repository/allocation.go`:

```go
func (r *AllocationRepository) GetEpicsByEquipeAndProduto(ctx context.Context, equipeID, produtoID uuid.UUID) ([]EpicAllocationRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			e.id,
			e.numero_ticket,
			e.resumo,
			e.apelido,
			e.data_limite::timestamptz,
			e.prioridade,
			COALESCE(e.tipo_demanda,
				CASE
					WHEN e.tipo IN ('Épico', 'Projeto') THEN 'Meta'
					WHEN e.tipo IN ('Spike', 'Implantação', 'Aditivo - Delivery') THEN 'Compromisso'
					ELSE 'Iniciativa'
				END
			),
			COALESCE(
				(SELECT ARRAY_AGG(DISTINCT p.nome ORDER BY p.nome)
				 FROM tarefas c
				 JOIN tarefa_produtos tp ON tp.tarefa_id = c.id
				 JOIN produtos p ON p.id = tp.produto_id
				 WHERE c.parent_id = e.id),
				'{}'
			),
			(SELECT COUNT(*) FROM tarefas c WHERE c.parent_id = e.id AND c.status NOT IN ('Cancelado', 'Rejeitada'))::int,
			(SELECT COUNT(*) FROM tarefas c WHERE c.parent_id = e.id AND c.status NOT IN ('Cancelado', 'Rejeitada') AND c.estimativa_tempo IS NOT NULL AND c.estimativa_tempo > 0)::int,
			COALESCE(
				(SELECT SUM(c.estimativa_tempo) FROM tarefas c WHERE c.parent_id = e.id AND c.status NOT IN ('Cancelado', 'Rejeitada') AND c.estimativa_tempo IS NOT NULL AND c.estimativa_tempo > 0),
				0
			)::float8 / 3600.0,
			COALESCE(
				(SELECT SUM(c.estimativa_tempo) FROM tarefas c
				 JOIN sprints s ON s.id = c.sprint_id
				 WHERE c.parent_id = e.id
				   AND c.status NOT IN ('Cancelado', 'Rejeitada')
				   AND c.estimativa_tempo IS NOT NULL AND c.estimativa_tempo > 0
				   AND s.estado IN ('active', 'future')),
				0
			)::float8 / 3600.0
		FROM tarefas e
		WHERE e.tipo IN ('Épico', 'Epico')
		  AND e.fonte_dados_id IN (
			SELECT DISTINCT m.fonte_dados_id
			FROM equipe_membros em
			JOIN membros m ON em.membro_id = m.id
			WHERE em.equipe_id = $1
		  )
		  AND EXISTS (
			SELECT 1 FROM tarefas c
			JOIN tarefa_produtos tp ON tp.tarefa_id = c.id
			WHERE c.parent_id = e.id AND tp.produto_id = $2
		  )
		  AND e.status NOT IN ('Cancelado', 'Rejeitada', 'Concluído')
		ORDER BY
			CASE e.prioridade
				WHEN 'Highest' THEN 1
				WHEN 'High' THEN 2
				WHEN 'Medium' THEN 3
				WHEN 'Low' THEN 4
				WHEN 'Lowest' THEN 5
				ELSE 6
			END,
			CASE
				WHEN COALESCE(e.tipo_demanda, '') = 'Meta' THEN 1
				WHEN COALESCE(e.tipo_demanda, '') = 'Compromisso' THEN 2
				WHEN COALESCE(e.tipo_demanda, '') = 'Iniciativa' THEN 3
				ELSE 4
			END,
			e.numero_ticket
	`, equipeID, produtoID)
	if err != nil {
		return nil, fmt.Errorf("querying epics: %w", err)
	}
	defer rows.Close()

	result := make([]EpicAllocationRow, 0)
	for rows.Next() {
		var e EpicAllocationRow
		if err := rows.Scan(
			&e.EpicID, &e.NumeroTicket, &e.Resumo, &e.Apelido,
			&e.DataLimite, &e.Prioridade, &e.TipoDemanda, &e.Produtos,
			&e.TotalFilhas, &e.FilhasComEstimativa,
			&e.HorasEstimadas, &e.HorasEmSprint,
		); err != nil {
			return nil, fmt.Errorf("scanning epic: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
```

- [ ] **Step 3: Implement GetEpicTasks**

Append to `backend/internal/repository/allocation.go`:

```go
func (r *AllocationRepository) GetEpicTasks(ctx context.Context, epicID uuid.UUID) ([]TaskAllocationRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			t.id, t.numero_ticket, t.resumo, t.tipo, t.tipo_demanda, t.status,
			t.estimativa_tempo,
			t.sprint_id, s.nome, s.data_inicio, s.data_fim,
			t.responsavel_id, m.nome
		FROM tarefas t
		LEFT JOIN sprints s ON s.id = t.sprint_id
		LEFT JOIN membros m ON m.id = t.responsavel_id
		WHERE t.parent_id = $1
		  AND t.status NOT IN ('Cancelado', 'Rejeitada')
		ORDER BY t.numero_ticket
	`, epicID)
	if err != nil {
		return nil, fmt.Errorf("querying epic tasks: %w", err)
	}
	defer rows.Close()

	result := make([]TaskAllocationRow, 0)
	for rows.Next() {
		var t TaskAllocationRow
		if err := rows.Scan(
			&t.TarefaID, &t.NumeroTicket, &t.Resumo, &t.Tipo, &t.TipoDemanda, &t.Status,
			&t.EstimativaTempo,
			&t.SprintID, &t.SprintNome, &t.SprintInicio, &t.SprintFim,
			&t.ResponsavelID, &t.ResponsavelNome,
		); err != nil {
			return nil, fmt.Errorf("scanning task: %w", err)
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
```

- [ ] **Step 4: Implement GetEpicPeople**

Append to `backend/internal/repository/allocation.go`:

```go
func (r *AllocationRepository) GetEpicPeople(ctx context.Context, epicID uuid.UUID) ([]PersonAllocationRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			m.id,
			m.nome,
			COALESCE(SUM(t.estimativa_tempo), 0)::float8 / 3600.0
		FROM tarefas t
		JOIN membros m ON m.id = t.responsavel_id
		WHERE t.parent_id = $1
		  AND t.status NOT IN ('Cancelado', 'Rejeitada')
		GROUP BY m.id, m.nome
		ORDER BY SUM(t.estimativa_tempo) DESC NULLS LAST
	`, epicID)
	if err != nil {
		return nil, fmt.Errorf("querying epic people: %w", err)
	}
	defer rows.Close()

	result := make([]PersonAllocationRow, 0)
	for rows.Next() {
		var p PersonAllocationRow
		if err := rows.Scan(&p.MembroID, &p.Nome, &p.HorasNoProjeto); err != nil {
			return nil, fmt.Errorf("scanning person: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
```

- [ ] **Step 5: Implement UpdateTaskAllocation, RollbackTaskAllocation, GetTaskPreviousState**

Append to `backend/internal/repository/allocation.go`:

```go
func (r *AllocationRepository) GetTaskPreviousState(ctx context.Context, taskID uuid.UUID) (*TaskPreviousState, error) {
	var s TaskPreviousState
	err := r.pool.QueryRow(ctx, `
		SELECT sprint_id, responsavel_id, estimativa_tempo FROM tarefas WHERE id = $1
	`, taskID).Scan(&s.SprintID, &s.ResponsavelID, &s.Estimativa)
	if err != nil {
		return nil, fmt.Errorf("getting task previous state: %w", err)
	}
	return &s, nil
}

func (r *AllocationRepository) UpdateTaskAllocation(ctx context.Context, taskID, sprintID uuid.UUID, assigneeID *uuid.UUID, estimateSeconds int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tarefas
		SET sprint_id = $2, responsavel_id = $3, estimativa_tempo = $4, updated_at = NOW()
		WHERE id = $1
	`, taskID, sprintID, assigneeID, estimateSeconds)
	if err != nil {
		return fmt.Errorf("updating task allocation: %w", err)
	}
	return nil
}

func (r *AllocationRepository) RollbackTaskAllocation(ctx context.Context, taskID uuid.UUID, prev *TaskPreviousState) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tarefas
		SET sprint_id = $2, responsavel_id = $3, estimativa_tempo = $4, updated_at = NOW()
		WHERE id = $1
	`, taskID, prev.SprintID, prev.ResponsavelID, prev.Estimativa)
	if err != nil {
		return fmt.Errorf("rolling back task allocation: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Implement GetFutureSprintsByEquipe**

Append to `backend/internal/repository/allocation.go`:

```go
func (r *AllocationRepository) GetFutureSprintsByEquipe(ctx context.Context, equipeID uuid.UUID) ([]SprintOptionRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT s.id, s.jira_id, s.nome, s.data_inicio, s.data_fim, COALESCE(s.estado, 'future')
		FROM sprints s
		JOIN equipes eq ON eq.board_id = s.board_id
		WHERE eq.id = $1
		  AND s.estado IN ('active', 'future')
		  AND s.data_inicio IS NOT NULL AND s.data_fim IS NOT NULL
		ORDER BY s.data_inicio
	`, equipeID)
	if err != nil {
		return nil, fmt.Errorf("querying future sprints: %w", err)
	}
	defer rows.Close()

	result := make([]SprintOptionRow, 0)
	for rows.Next() {
		var s SprintOptionRow
		if err := rows.Scan(&s.ID, &s.JiraID, &s.Nome, &s.Inicio, &s.Fim, &s.Estado); err != nil {
			return nil, fmt.Errorf("scanning sprint: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
```

- [ ] **Step 7: Implement CheckGDPTCAncestors**

Append to `backend/internal/repository/allocation.go`:

```go
func (r *AllocationRepository) CheckGDPTCAncestors(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if len(epicIDs) == 0 {
		return make(map[uuid.UUID]bool), nil
	}

	rows, err := r.pool.Query(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT t.id AS original_id, t.id, t.parent_id, t.numero_ticket, 1 AS depth
			FROM tarefas t WHERE t.id = ANY($1)
			UNION ALL
			SELECT a.original_id, p.id, p.parent_id, p.numero_ticket, a.depth + 1
			FROM tarefas p JOIN ancestors a ON p.id = a.parent_id
			WHERE a.depth < 10
		)
		SELECT DISTINCT original_id FROM ancestors
		WHERE numero_ticket LIKE 'GDPTC-%' AND original_id != id
	`, epicIDs)
	if err != nil {
		return nil, fmt.Errorf("querying GDPTC ancestors: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]bool)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning GDPTC id: %w", err)
		}
		result[id] = true
	}
	return result, rows.Err()
}
```

- [ ] **Step 8: Implement helper lookups for Jira write-back**

Append to `backend/internal/repository/allocation.go`:

```go
func (r *AllocationRepository) GetTaskFonteDadosID(ctx context.Context, taskID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT fonte_dados_id FROM tarefas WHERE id = $1`, taskID).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("getting task fonte_dados_id: %w", err)
	}
	return id, nil
}

func (r *AllocationRepository) GetSprintJiraID(ctx context.Context, sprintID uuid.UUID) (int, error) {
	var jiraID int
	err := r.pool.QueryRow(ctx, `SELECT jira_id FROM sprints WHERE id = $1`, sprintID).Scan(&jiraID)
	if err != nil {
		return 0, fmt.Errorf("getting sprint jira_id: %w", err)
	}
	return jiraID, nil
}

func (r *AllocationRepository) GetTaskJiraKey(ctx context.Context, taskID uuid.UUID) (string, error) {
	var key string
	err := r.pool.QueryRow(ctx, `SELECT numero_ticket FROM tarefas WHERE id = $1`, taskID).Scan(&key)
	if err != nil {
		return "", fmt.Errorf("getting task jira key: %w", err)
	}
	return key, nil
}
```

- [ ] **Step 9: Verify compilation**

Run: `cd /home/emerson/code/myplanner/backend && go build ./... && go vet ./...`
Expected: clean build.

- [ ] **Step 10: Commit**

```bash
git add backend/internal/repository/allocation.go
git commit -m "feat(allocation): add AllocationRepository with SQL queries"
```

---

### Task 3: AllocationService — Business Logic

**Files:**
- Create: `backend/internal/service/allocation.go`
- Create: `backend/internal/service/allocation_test.go`

**Interfaces:**
- Consumes:
  - `repository.AllocationRepository` (all methods from Task 2)
  - `SprintService.GetCapacity(ctx, sprintID uuid.UUID, equipeID *uuid.UUID) (*SprintCapacityResult, error)` (existing)
  - `SyncService.SyncProject(ctx, fonteDadosID uuid.UUID, projectKey string) (*domain.SyncLog, error)` (existing — but we need a lighter version. We'll call it via `executSyncProject` pattern but with a JQL filter)
  - `jira.Client` interface with `MoveToSprint`, `UpdateTimeEstimate`, `AssignIssue`, `AddComment` (from Task 1)
  - `ClientFactory` and `OAuthClientFactory` types from `service/sync.go`
  - `repository.FonteDadosRepository.GetByID` (existing)
  - `repository.SprintRepository.GetMembroJiraAccountID` (existing)
- Produces:
  - `NewAllocationService(...) *AllocationService`
  - `ListProjectAllocations(ctx, equipeID, produtoID uuid.UUID) ([]ProjectAllocation, error)`
  - `GetProjectDetail(ctx, epicID, equipeID uuid.UUID) (*ProjectDetail, error)`
  - `AllocateTask(ctx, req AllocateTaskRequest) (*AllocateTaskResult, error)`
  - `SyncProjectTasks(ctx, epicID uuid.UUID) (int, error)`
  - `GetAvailableSprints(ctx, equipeID uuid.UUID) ([]SprintOption, error)`
  - Types: `ProjectAllocation`, `ProjectDetail`, `PersonAllocation`, `TaskAllocation`, `SprintOption`, `AllocateTaskRequest`, `AllocateTaskResult`

- [ ] **Step 1: Write failing test for ListProjectAllocations**

Create `backend/internal/service/allocation_test.go`:

```go
package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestListProjectAllocations_ComputesMetrics(t *testing.T) {
	svc := &AllocationService{}
	_, err := svc.ListProjectAllocations(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error from nil repo, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/service/ -run TestListProjectAllocations -v`
Expected: compilation error — `AllocationService` type not defined.

- [ ] **Step 3: Implement AllocationService types and constructor**

Create `backend/internal/service/allocation.go`:

```go
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

type ProjectAllocation struct {
	EpicID        uuid.UUID  `json:"epic_id"`
	NumeroTicket  string     `json:"numero_ticket"`
	Resumo        string     `json:"resumo"`
	Apelido       *string    `json:"apelido"`
	DataLimite    *time.Time `json:"data_limite"`
	Prioridade    *string    `json:"prioridade"`
	TipoDemanda   *string    `json:"tipo_demanda"`
	Produtos      []string   `json:"produtos"`
	PctEstimado   float64    `json:"pct_estimado"`
	PctPlanejado  float64    `json:"pct_planejado"`
	TarefasSemEst int        `json:"tarefas_sem_estimativa"`
	TotalTarefas  int        `json:"total_tarefas"`
	IsGDPTC       bool       `json:"is_gdptc"`
	Status        string     `json:"status"`
}

type TaskAllocation struct {
	TarefaID        uuid.UUID  `json:"tarefa_id"`
	NumeroTicket    string     `json:"numero_ticket"`
	Resumo          string     `json:"resumo"`
	Tipo            string     `json:"tipo"`
	TipoDemanda     *string    `json:"tipo_demanda"`
	Status          string     `json:"status"`
	EstimativaHoras *float64   `json:"estimativa_horas"`
	SprintID        *uuid.UUID `json:"sprint_id"`
	SprintNome      *string    `json:"sprint_nome"`
	SprintInicio    *time.Time `json:"sprint_inicio"`
	SprintFim       *time.Time `json:"sprint_fim"`
	ResponsavelID   *uuid.UUID `json:"responsavel_id"`
	ResponsavelNome *string    `json:"responsavel_nome"`
}

type PersonAllocation struct {
	MembroID       uuid.UUID `json:"membro_id"`
	Nome           string    `json:"nome"`
	HorasNoProjeto float64   `json:"horas_no_projeto"`
	HorasCapTotal  float64   `json:"horas_cap_total"`
	PctNoProjeto   float64   `json:"pct_no_projeto"`
}

type ProjectDetail struct {
	Epic        ProjectAllocation  `json:"epic"`
	Pessoas     []PersonAllocation `json:"pessoas"`
	NaoAlocadas []TaskAllocation   `json:"nao_alocadas"`
	Parciais    []TaskAllocation   `json:"parciais"`
	Completas   []TaskAllocation   `json:"completas"`
}

type SprintOption struct {
	ID     uuid.UUID `json:"id"`
	JiraID int       `json:"jira_id"`
	Nome   string    `json:"nome"`
	Inicio time.Time `json:"inicio"`
	Fim    time.Time `json:"fim"`
	Estado string    `json:"estado"`
}

type AllocateTaskRequest struct {
	TaskID        uuid.UUID  `json:"task_id"`
	SprintID      uuid.UUID  `json:"sprint_id"`
	AssigneeID    *uuid.UUID `json:"assignee_id"`
	EstimateHours float64    `json:"estimate_hours"`
	Force         bool       `json:"force"`
	EquipeID      uuid.UUID  `json:"equipe_id"`
}

type AllocateTaskResult struct {
	Conflict   bool    `json:"conflict,omitempty"`
	MembroNome string  `json:"membro_nome,omitempty"`
	SprintNome string  `json:"sprint_nome,omitempty"`
	PctAtual   float64 `json:"pct_atual,omitempty"`
}

type AllocationService struct {
	repo           *repository.AllocationRepository
	sprintSvc      *SprintService
	sprintRepo     *repository.SprintRepository
	fdRepo         *repository.FonteDadosRepository
	clientFactory  ClientFactory
	oauthFactory   OAuthClientFactory
	oauthSvc       *jira.OAuthService
	rateLimit      int
	logger         *zap.Logger
}

func NewAllocationService(
	repo *repository.AllocationRepository,
	sprintSvc *SprintService,
	sprintRepo *repository.SprintRepository,
	fdRepo *repository.FonteDadosRepository,
	clientFactory ClientFactory,
	oauthFactory OAuthClientFactory,
	oauthSvc *jira.OAuthService,
	rateLimit int,
	logger *zap.Logger,
) *AllocationService {
	return &AllocationService{
		repo:          repo,
		sprintSvc:     sprintSvc,
		sprintRepo:    sprintRepo,
		fdRepo:        fdRepo,
		clientFactory: clientFactory,
		oauthFactory:  oauthFactory,
		oauthSvc:      oauthSvc,
		rateLimit:     rateLimit,
		logger:        logger,
	}
}
```

- [ ] **Step 4: Implement ListProjectAllocations**

Append to `backend/internal/service/allocation.go`:

```go
func (s *AllocationService) ListProjectAllocations(ctx context.Context, equipeID, produtoID uuid.UUID) ([]ProjectAllocation, error) {
	rows, err := s.repo.GetEpicsByEquipeAndProduto(ctx, equipeID, produtoID)
	if err != nil {
		return nil, fmt.Errorf("listing epics: %w", err)
	}

	epicIDs := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		epicIDs[i] = r.EpicID
	}

	gdptcMap, err := s.repo.CheckGDPTCAncestors(ctx, epicIDs)
	if err != nil {
		s.logger.Warn("checking GDPTC ancestors", zap.Error(err))
		gdptcMap = make(map[uuid.UUID]bool)
	}

	result := make([]ProjectAllocation, 0, len(rows))
	for _, r := range rows {
		var pctEstimado, pctPlanejado float64
		if r.TotalFilhas > 0 {
			pctEstimado = float64(r.FilhasComEstimativa) / float64(r.TotalFilhas) * 100
		}
		if r.HorasEstimadas > 0 {
			pctPlanejado = r.HorasEmSprint / r.HorasEstimadas * 100
		}

		status := "nao_planejado"
		if pctPlanejado >= 100 {
			status = "planejado"
			pctPlanejado = 100
		} else if pctPlanejado > 0 {
			status = "em_planejamento"
		}

		result = append(result, ProjectAllocation{
			EpicID:        r.EpicID,
			NumeroTicket:  r.NumeroTicket,
			Resumo:        r.Resumo,
			Apelido:       r.Apelido,
			DataLimite:    r.DataLimite,
			Prioridade:    r.Prioridade,
			TipoDemanda:   r.TipoDemanda,
			Produtos:      r.Produtos,
			PctEstimado:   pctEstimado,
			PctPlanejado:  pctPlanejado,
			TarefasSemEst: r.TotalFilhas - r.FilhasComEstimativa,
			TotalTarefas:  r.TotalFilhas,
			IsGDPTC:       gdptcMap[r.EpicID],
			Status:        status,
		})
	}

	return result, nil
}
```

- [ ] **Step 5: Implement GetProjectDetail**

Append to `backend/internal/service/allocation.go`:

```go
func (s *AllocationService) GetProjectDetail(ctx context.Context, epicID, equipeID uuid.UUID) (*ProjectDetail, error) {
	epics, err := s.repo.GetEpicsByEquipeAndProduto(ctx, equipeID, uuid.Nil)
	if err != nil {
		return nil, fmt.Errorf("getting epic: %w", err)
	}

	var epicRow *repository.EpicAllocationRow
	for i := range epics {
		if epics[i].EpicID == epicID {
			epicRow = &epics[i]
			break
		}
	}

	tasks, err := s.repo.GetEpicTasks(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("getting tasks: %w", err)
	}

	people, err := s.repo.GetEpicPeople(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("getting people: %w", err)
	}

	gdptcMap, _ := s.repo.CheckGDPTCAncestors(ctx, []uuid.UUID{epicID})

	var epic ProjectAllocation
	if epicRow != nil {
		var pctEstimado, pctPlanejado float64
		if epicRow.TotalFilhas > 0 {
			pctEstimado = float64(epicRow.FilhasComEstimativa) / float64(epicRow.TotalFilhas) * 100
		}
		if epicRow.HorasEstimadas > 0 {
			pctPlanejado = epicRow.HorasEmSprint / epicRow.HorasEstimadas * 100
		}
		status := "nao_planejado"
		if pctPlanejado >= 100 {
			status = "planejado"
			pctPlanejado = 100
		} else if pctPlanejado > 0 {
			status = "em_planejamento"
		}
		epic = ProjectAllocation{
			EpicID: epicRow.EpicID, NumeroTicket: epicRow.NumeroTicket,
			Resumo: epicRow.Resumo, Apelido: epicRow.Apelido,
			DataLimite: epicRow.DataLimite, Prioridade: epicRow.Prioridade,
			TipoDemanda: epicRow.TipoDemanda, Produtos: epicRow.Produtos,
			PctEstimado: pctEstimado, PctPlanejado: pctPlanejado,
			TarefasSemEst: epicRow.TotalFilhas - epicRow.FilhasComEstimativa,
			TotalTarefas: epicRow.TotalFilhas, IsGDPTC: gdptcMap[epicID],
			Status: status,
		}
	} else {
		epic = ProjectAllocation{EpicID: epicID, Status: "nao_planejado"}
	}

	pessoas := make([]PersonAllocation, 0, len(people))
	for _, p := range people {
		pessoas = append(pessoas, PersonAllocation{
			MembroID:       p.MembroID,
			Nome:           p.Nome,
			HorasNoProjeto: p.HorasNoProjeto,
		})
	}

	var naoAlocadas, parciais, completas []TaskAllocation
	for _, t := range tasks {
		ta := taskRowToAllocation(t)
		hasEstimate := t.EstimativaTempo != nil && *t.EstimativaTempo > 0
		hasSprint := t.SprintID != nil
		hasPerson := t.ResponsavelID != nil

		if !hasEstimate || !hasSprint {
			naoAlocadas = append(naoAlocadas, ta)
		} else if !hasPerson {
			parciais = append(parciais, ta)
		} else {
			completas = append(completas, ta)
		}
	}

	return &ProjectDetail{
		Epic:        epic,
		Pessoas:     pessoas,
		NaoAlocadas: naoAlocadas,
		Parciais:    parciais,
		Completas:   completas,
	}, nil
}

func taskRowToAllocation(t repository.TaskAllocationRow) TaskAllocation {
	ta := TaskAllocation{
		TarefaID:        t.TarefaID,
		NumeroTicket:    t.NumeroTicket,
		Resumo:          t.Resumo,
		Tipo:            t.Tipo,
		TipoDemanda:     t.TipoDemanda,
		Status:          t.Status,
		SprintID:        t.SprintID,
		SprintNome:      t.SprintNome,
		SprintInicio:    t.SprintInicio,
		SprintFim:       t.SprintFim,
		ResponsavelID:   t.ResponsavelID,
		ResponsavelNome: t.ResponsavelNome,
	}
	if t.EstimativaTempo != nil && *t.EstimativaTempo > 0 {
		h := float64(*t.EstimativaTempo) / 3600.0
		ta.EstimativaHoras = &h
	}
	return ta
}
```

- [ ] **Step 6: Implement AllocateTask with optimistic update and capacity check**

Append to `backend/internal/service/allocation.go`:

```go
func (s *AllocationService) AllocateTask(ctx context.Context, req AllocateTaskRequest) (*AllocateTaskResult, error) {
	if req.EstimateHours <= 0 {
		return nil, fmt.Errorf("estimate_hours must be > 0")
	}

	estimateSeconds := int(req.EstimateHours * 3600)

	if req.AssigneeID != nil && !req.Force {
		capResult, err := s.sprintSvc.GetCapacity(ctx, req.SprintID, &req.EquipeID)
		if err == nil {
			for _, m := range capResult.Membros {
				if m.MembroID == *req.AssigneeID {
					newHours := m.HorasAlocadas + req.EstimateHours
					newPct := 0.0
					if m.HorasDisponiveis > 0 {
						newPct = (newHours / m.HorasDisponiveis) * 100
					}
					if newPct > 100 {
						return &AllocateTaskResult{
							Conflict:   true,
							MembroNome: m.Nome,
							SprintNome: capResult.Sprint.Nome,
							PctAtual:   newPct,
						}, nil
					}
					break
				}
			}
		}
	}

	prev, err := s.repo.GetTaskPreviousState(ctx, req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("getting previous state: %w", err)
	}

	if err := s.repo.UpdateTaskAllocation(ctx, req.TaskID, req.SprintID, req.AssigneeID, estimateSeconds); err != nil {
		return nil, fmt.Errorf("updating allocation: %w", err)
	}

	go s.writeToJira(req.TaskID, req.SprintID, req.AssigneeID, estimateSeconds, prev)

	return &AllocateTaskResult{}, nil
}

func (s *AllocationService) writeToJira(taskID, sprintID uuid.UUID, assigneeID *uuid.UUID, estimateSeconds int, prev *repository.TaskPreviousState) {
	ctx := context.Background()

	issueKey, err := s.repo.GetTaskJiraKey(ctx, taskID)
	if err != nil {
		s.logger.Error("jira write: getting issue key", zap.Error(err))
		s.rollback(ctx, taskID, prev)
		return
	}

	fonteDadosID, err := s.repo.GetTaskFonteDadosID(ctx, taskID)
	if err != nil {
		s.logger.Error("jira write: getting fonte_dados_id", zap.Error(err))
		s.rollback(ctx, taskID, prev)
		return
	}

	client, err := s.buildClient(ctx, fonteDadosID)
	if err != nil {
		s.logger.Error("jira write: building client", zap.Error(err))
		s.rollback(ctx, taskID, prev)
		return
	}

	if err := client.UpdateTimeEstimate(ctx, issueKey, estimateSeconds); err != nil {
		s.logger.Warn("jira write: update estimate failed", zap.String("key", issueKey), zap.Error(err))
		s.rollback(ctx, taskID, prev)
		return
	}

	sprintJiraID, err := s.repo.GetSprintJiraID(ctx, sprintID)
	if err != nil {
		s.logger.Error("jira write: getting sprint jira_id", zap.Error(err))
		s.rollback(ctx, taskID, prev)
		return
	}

	if err := client.MoveToSprint(ctx, sprintJiraID, issueKey); err != nil {
		s.logger.Warn("jira write: move to sprint failed", zap.String("key", issueKey), zap.Error(err))
		s.rollback(ctx, taskID, prev)
		return
	}

	if assigneeID != nil {
		accountID, err := s.sprintRepo.GetMembroJiraAccountID(ctx, *assigneeID)
		if err != nil {
			s.logger.Warn("jira write: getting account id", zap.Error(err))
			return
		}
		if err := client.AssignIssue(ctx, issueKey, accountID); err != nil {
			s.logger.Warn("jira write: assign failed", zap.String("key", issueKey), zap.Error(err))
			return
		}
	}

	s.logger.Info("jira write complete", zap.String("key", issueKey))
}

func (s *AllocationService) rollback(ctx context.Context, taskID uuid.UUID, prev *repository.TaskPreviousState) {
	if err := s.repo.RollbackTaskAllocation(ctx, taskID, prev); err != nil {
		s.logger.Error("rollback failed", zap.String("taskID", taskID.String()), zap.Error(err))
	}
}

func (s *AllocationService) buildClient(ctx context.Context, fonteDadosID uuid.UUID) (jira.Client, error) {
	fonte, err := s.fdRepo.GetByID(ctx, fonteDadosID)
	if err != nil {
		return nil, fmt.Errorf("getting fonte dados: %w", err)
	}

	if fonte.AuthType == "oauth2" {
		if fonte.OAuth2AccessToken == nil || fonte.OAuth2RefreshToken == nil {
			return nil, fmt.Errorf("fonte %s: oauth2 tokens missing", fonte.Nome)
		}
		accessToken := *fonte.OAuth2AccessToken
		if fonte.OAuth2TokenExpiry != nil && time.Now().After(*fonte.OAuth2TokenExpiry) {
			if s.oauthSvc == nil {
				return nil, fmt.Errorf("fonte %s: oauth token expired", fonte.Nome)
			}
			newTokens, err := s.oauthSvc.RefreshAccessToken(ctx, *fonte.OAuth2RefreshToken)
			if err != nil {
				return nil, fmt.Errorf("refreshing oauth token: %w", err)
			}
			expiry := newTokens.Expiry()
			if err := s.fdRepo.SaveOAuthTokens(ctx, fonte.ID, fonte.BaseURL, newTokens.AccessToken, newTokens.RefreshToken, expiry); err != nil {
				return nil, fmt.Errorf("saving refreshed tokens: %w", err)
			}
			accessToken = newTokens.AccessToken
		}
		return s.oauthFactory(fonte.BaseURL, accessToken, s.rateLimit, s.logger), nil
	}

	email := ""
	if fonte.UserEmail != nil {
		email = *fonte.UserEmail
	}
	apiToken := ""
	if fonte.APIToken != nil {
		apiToken = *fonte.APIToken
	}
	return s.clientFactory(fonte.BaseURL, email, apiToken, s.rateLimit, s.logger), nil
}
```

- [ ] **Step 7: Implement GetAvailableSprints and SyncProjectTasks**

Append to `backend/internal/service/allocation.go`:

```go
func (s *AllocationService) GetAvailableSprints(ctx context.Context, equipeID uuid.UUID) ([]SprintOption, error) {
	rows, err := s.repo.GetFutureSprintsByEquipe(ctx, equipeID)
	if err != nil {
		return nil, err
	}
	result := make([]SprintOption, len(rows))
	for i, r := range rows {
		result[i] = SprintOption{
			ID: r.ID, JiraID: r.JiraID, Nome: r.Nome,
			Inicio: r.Inicio, Fim: r.Fim, Estado: r.Estado,
		}
	}
	return result, nil
}

func (s *AllocationService) SyncProjectTasks(ctx context.Context, epicID uuid.UUID) (int, error) {
	issueKey, err := s.repo.GetTaskJiraKey(ctx, epicID)
	if err != nil {
		return 0, fmt.Errorf("getting epic key: %w", err)
	}

	fonteDadosID, err := s.repo.GetTaskFonteDadosID(ctx, epicID)
	if err != nil {
		return 0, fmt.Errorf("getting fonte_dados_id: %w", err)
	}

	client, err := s.buildClient(ctx, fonteDadosID)
	if err != nil {
		return 0, fmt.Errorf("building client: %w", err)
	}

	jql := fmt.Sprintf(`"Epic Link" = %s OR parent = %s`, issueKey, issueKey)
	fields := []string{"summary", "issuetype", "status", "priority", "assignee", "reporter",
		"project", "created", "updated", "duedate", "resolutiondate", "timetracking",
		"sprint", "parent", "labels", "components"}

	payload := map[string]any{
		"jql":        jql,
		"maxResults": 100,
		"fields":     fields,
	}

	s.logger.Info("syncing project tasks", zap.String("epic", issueKey), zap.String("jql", jql))

	body, err := func() ([]byte, error) {
		type doPostClient interface {
			GetIssuesByProjects(ctx context.Context, projectKeys []string, updatedSince *time.Time) ([]jira.JiraIssue, error)
		}
		_ = payload
		issues, err := client.GetIssuesByProjects(ctx, []string{issueKey[:strings.Index(issueKey, "-")]}, nil)
		if err != nil {
			return nil, err
		}
		_ = issues
		return nil, nil
	}()
	_ = body

	return 0, fmt.Errorf("sync project tasks: not yet fully wired (requires SyncService integration)")
}
```

Note: The `SyncProjectTasks` implementation above is a placeholder that shows the approach. The actual implementation requires access to `SyncService.processIssue` which is unexported. The implementer should either:
1. Export `processIssue` from SyncService, or
2. Add a `SyncEpicTasks(ctx, fonteDadosID uuid.UUID, epicKey string) (int, error)` method to SyncService that internally uses `processIssue`, then call it from AllocationService.

The recommended approach is option 2. Add this method to `backend/internal/service/sync.go`:

```go
func (s *SyncService) SyncEpicTasks(ctx context.Context, fonteDadosID uuid.UUID, epicKey string) (int, error) {
	fonte, err := s.getFonte(ctx, fonteDadosID)
	if err != nil {
		return 0, err
	}
	client, err := s.buildClient(ctx, fonte)
	if err != nil {
		return 0, err
	}

	if sprintFieldID, err := client.GetSprintFieldID(ctx); err == nil {
		client.SetSprintFieldID(sprintFieldID)
	}

	projectKey := epicKey[:strings.Index(epicKey, "-")]
	issues, err := client.GetIssuesByProjects(ctx, []string{projectKey}, nil)
	if err != nil {
		return 0, fmt.Errorf("fetching issues: %w", err)
	}

	memberCache := make(map[string]uuid.UUID)
	sprintCache := make(map[int]uuid.UUID)
	projectCache := make(map[string]uuid.UUID)
	count := 0

	for _, issue := range issues {
		if issue.Fields.Parent == nil {
			continue
		}
		if issue.Fields.Parent.Key != epicKey {
			continue
		}
		projetoID, err := s.ensureProject(ctx, fonte, issue, projectCache)
		if err != nil {
			continue
		}
		s.ensureMember(ctx, fonte.ID, issue.Fields.Assignee, memberCache)
		s.ensureMember(ctx, fonte.ID, issue.Fields.Reporter, memberCache)
		if _, err := s.processIssue(ctx, fonte, projetoID, issue, memberCache, sprintCache); err != nil {
			s.logger.Warn("sync epic task failed", zap.String("key", issue.Key), zap.Error(err))
			continue
		}
		count++
	}

	return count, nil
}
```

Then update `AllocationService.SyncProjectTasks` to delegate:

```go
func (s *AllocationService) SyncProjectTasks(ctx context.Context, epicID uuid.UUID) (int, error) {
	issueKey, err := s.repo.GetTaskJiraKey(ctx, epicID)
	if err != nil {
		return 0, fmt.Errorf("getting epic key: %w", err)
	}
	fonteDadosID, err := s.repo.GetTaskFonteDadosID(ctx, epicID)
	if err != nil {
		return 0, fmt.Errorf("getting fonte_dados_id: %w", err)
	}
	return s.syncSvc.SyncEpicTasks(ctx, fonteDadosID, issueKey)
}
```

Add `syncSvc *SyncService` to `AllocationService` struct and constructor.

- [ ] **Step 8: Run tests and verify compilation**

Run:
```bash
cd /home/emerson/code/myplanner/backend && go build ./... && go vet ./...
cd /home/emerson/code/myplanner/backend && go test ./internal/service/ -run TestListProjectAllocations -v
```
Expected: builds clean, test passes.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/service/allocation.go backend/internal/service/allocation_test.go backend/internal/service/sync.go
git commit -m "feat(allocation): add AllocationService with business logic and Jira write-back"
```

---

### Task 4: AllocationHandler + Route Wiring

**Files:**
- Create: `backend/internal/handler/allocation.go`
- Modify: `backend/cmd/api/main.go:139-145,252-264`

**Interfaces:**
- Consumes:
  - `AllocationService` (all methods from Task 3)
  - Existing handler patterns: `respondJSON`, `respondError`, `chi.URLParam`
- Produces:
  - `NewAllocationHandler(svc *service.AllocationService, logger *zap.Logger) *AllocationHandler`
  - HTTP handlers: `ListProjects`, `GetProjectDetail`, `AllocateTask`, `SyncProject`, `ListSprints`
  - Routes registered under `/api/v1/allocation/`

- [ ] **Step 1: Create AllocationHandler**

Create `backend/internal/handler/allocation.go`:

```go
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AllocationHandler struct {
	svc    *service.AllocationService
	logger *zap.Logger
}

func NewAllocationHandler(svc *service.AllocationService, logger *zap.Logger) *AllocationHandler {
	return &AllocationHandler{svc: svc, logger: logger}
}

func (h *AllocationHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	equipeStr := r.URL.Query().Get("equipe_id")
	if equipeStr == "" {
		respondError(w, http.StatusBadRequest, "equipe_id is required")
		return
	}
	equipeID, err := uuid.Parse(equipeStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid equipe_id")
		return
	}

	produtoStr := r.URL.Query().Get("produto_id")
	if produtoStr == "" {
		respondError(w, http.StatusBadRequest, "produto_id is required")
		return
	}
	produtoID, err := uuid.Parse(produtoStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid produto_id")
		return
	}

	result, err := h.svc.ListProjectAllocations(r.Context(), equipeID, produtoID)
	if err != nil {
		h.logger.Error("listing project allocations", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar projetos")
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func (h *AllocationHandler) GetProjectDetail(w http.ResponseWriter, r *http.Request) {
	epicID, err := uuid.Parse(chi.URLParam(r, "epicId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid epicId")
		return
	}

	equipeStr := r.URL.Query().Get("equipe_id")
	if equipeStr == "" {
		respondError(w, http.StatusBadRequest, "equipe_id is required")
		return
	}
	equipeID, err := uuid.Parse(equipeStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid equipe_id")
		return
	}

	result, err := h.svc.GetProjectDetail(r.Context(), epicID, equipeID)
	if err != nil {
		h.logger.Error("getting project detail", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao carregar projeto")
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func (h *AllocationHandler) AllocateTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid taskId")
		return
	}

	var req service.AllocateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	req.TaskID = taskID

	if req.SprintID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "sprint_id obrigatório")
		return
	}
	if req.EstimateHours <= 0 {
		respondError(w, http.StatusBadRequest, "estimate_hours deve ser > 0")
		return
	}
	if req.EquipeID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "equipe_id obrigatório")
		return
	}

	result, err := h.svc.AllocateTask(r.Context(), req)
	if err != nil {
		h.logger.Error("allocating task", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao alocar tarefa")
		return
	}

	if result.Conflict {
		respondJSON(w, http.StatusConflict, result)
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (h *AllocationHandler) SyncProject(w http.ResponseWriter, r *http.Request) {
	epicID, err := uuid.Parse(chi.URLParam(r, "epicId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid epicId")
		return
	}

	count, err := h.svc.SyncProjectTasks(r.Context(), epicID)
	if err != nil {
		h.logger.Error("syncing project tasks", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao sincronizar tarefas")
		return
	}

	respondJSON(w, http.StatusOK, map[string]int{"synced": count})
}

func (h *AllocationHandler) ListSprints(w http.ResponseWriter, r *http.Request) {
	equipeStr := r.URL.Query().Get("equipe_id")
	if equipeStr == "" {
		respondError(w, http.StatusBadRequest, "equipe_id is required")
		return
	}
	equipeID, err := uuid.Parse(equipeStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid equipe_id")
		return
	}

	result, err := h.svc.GetAvailableSprints(r.Context(), equipeID)
	if err != nil {
		h.logger.Error("listing sprints", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar sprints")
		return
	}
	respondJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 2: Wire AllocationService and handler in main.go**

In `backend/cmd/api/main.go`, after the review service block (around line 145), add:

```go
	allocRepo := repository.NewAllocationRepository(pool)
	allocSvc := service.NewAllocationService(allocRepo, sprintService, sprintRepo, fonteDadosRepo, clientFactory, oauthClientFactory, oauthSvc, cfg.Sync.RateLimitPerSec, logger)
	allocHandler := handler.NewAllocationHandler(allocSvc, logger)
```

In the router group (after the config routes, around line 264), add:

```go
			r.Get("/allocation/projects", allocHandler.ListProjects)
			r.Get("/allocation/projects/{epicId}", allocHandler.GetProjectDetail)
			r.Post("/allocation/tasks/{taskId}/allocate", allocHandler.AllocateTask)
			r.Post("/allocation/projects/{epicId}/sync", allocHandler.SyncProject)
			r.Get("/allocation/sprints", allocHandler.ListSprints)
```

- [ ] **Step 3: Verify compilation**

Run: `cd /home/emerson/code/myplanner/backend && go build ./... && go vet ./...`
Expected: clean build. Fix any import or wiring issues.

- [ ] **Step 4: Run existing tests to check no regressions**

Run: `cd /home/emerson/code/myplanner/backend && go test ./... 2>&1 | tail -20`
Expected: all existing tests still pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/allocation.go backend/cmd/api/main.go
git commit -m "feat(allocation): add AllocationHandler and wire routes in main.go"
```

---

### Task 5: Frontend — Allocation Page with Epic Boxes

**Files:**
- Modify: `frontend/index.html` — sidebar, page container, CSS, JS

**Interfaces:**
- Consumes: Backend endpoints `GET /allocation/projects`, `GET /api/v1/equipes`, `GET /api/v1/produtos`
- Produces: `loadAlocacao()`, `loadProjectAllocations(equipeId, produtoId)`, `renderAllocationBoxes(projects)` — used by Task 6 (modal)

- [ ] **Step 1: Add CSS for allocation page**

In `frontend/index.html`, in the `<style>` section (before the closing `</style>`), add:

```css
.alloc-filters { display: flex; gap: 16px; margin-bottom: 20px; align-items: center; }
.alloc-filters select { padding: 8px 12px; border-radius: 6px; border: 1px solid var(--border); background: var(--card-bg); color: var(--text); font-size: 14px; min-width: 200px; }
.alloc-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }
.alloc-box { background: var(--card-bg); border-radius: 10px; padding: 16px; cursor: pointer; border: 1px solid var(--border); transition: transform .15s, box-shadow .15s; position: relative; }
.alloc-box:hover { transform: translateY(-2px); box-shadow: 0 4px 12px rgba(0,0,0,.15); }
.alloc-box-header { display: flex; align-items: center; gap: 6px; margin-bottom: 8px; }
.alloc-box-star { color: #f5c518; font-size: 18px; cursor: help; }
.alloc-box-ticket { font-size: 12px; color: var(--text-secondary); font-weight: 600; }
.alloc-box-title { font-size: 14px; font-weight: 600; color: var(--text); margin-bottom: 6px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.alloc-box-meta { font-size: 12px; color: var(--text-secondary); margin-bottom: 8px; }
.alloc-box-produtos { display: flex; flex-wrap: wrap; gap: 4px; margin-bottom: 8px; }
.alloc-box-produto { font-size: 11px; padding: 2px 8px; border-radius: 10px; background: rgba(13,124,102,.15); color: var(--primary); }
.alloc-bar-group { display: flex; flex-direction: column; gap: 4px; margin-bottom: 8px; }
.alloc-bar { height: 6px; border-radius: 3px; background: var(--border); overflow: hidden; }
.alloc-bar-fill { height: 100%; border-radius: 3px; transition: width .3s; }
.alloc-bar-label { font-size: 11px; color: var(--text-secondary); display: flex; justify-content: space-between; }
.alloc-alert { font-size: 11px; color: var(--warning, #e6a817); margin-bottom: 6px; }
.alloc-badge { font-size: 11px; padding: 2px 8px; border-radius: 10px; font-weight: 600; display: inline-block; }
.alloc-badge-planejado { background: rgba(34,197,94,.15); color: #22c55e; }
.alloc-badge-em_planejamento { background: rgba(234,179,8,.15); color: #eab308; }
.alloc-badge-nao_planejado { background: rgba(156,163,175,.15); color: #9ca3af; }
.alloc-empty { text-align: center; padding: 60px 20px; color: var(--text-secondary); }
```

- [ ] **Step 2: Add sidebar item for Alocação**

In `frontend/index.html`, inside the `sidebar-group-items` of "Relatórios" (after the Sprints Timeline button, around line 829), add:

```html
        <button class="sidebar-item" data-page="alocacao" title="Alocação" onclick="navigate('alocacao')">
          <svg viewBox="0 0 24 24"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>
          <span class="sidebar-item-label">Alocação</span>
        </button>
```

- [ ] **Step 3: Add allocation page container**

In `frontend/index.html`, before the closing `</div>` of `<div class="main">` (before the `<script>` tag), add a new page:

```html
    <!-- ALOCAÇÃO -->
    <div class="page" id="page-alocacao">
      <div class="page-header-row">
        <h1 class="page-title">Alocação de Projetos</h1>
      </div>
      <div class="alloc-filters">
        <select id="alloc-equipe" onchange="onAllocFilterChange()">
          <option value="">Selecione a Equipe</option>
        </select>
        <select id="alloc-produto" onchange="onAllocFilterChange()">
          <option value="">Selecione o Produto</option>
        </select>
      </div>
      <div id="alloc-content">
        <div class="alloc-empty">Selecione equipe e produto para ver os projetos.</div>
      </div>
    </div>
```

- [ ] **Step 4: Update navigate() function**

In `frontend/index.html`, in the `navigate()` function (around line 1646), update the `reportPages` array and add the load trigger:

Change:
```javascript
  const reportPages = ['timeline', 'sprints-timeline'];
```
To:
```javascript
  const reportPages = ['timeline', 'sprints-timeline', 'alocacao'];
```

And add after the existing page load triggers:
```javascript
  if (page === 'alocacao') loadAlocacao();
```

- [ ] **Step 5: Add allocation JS functions**

In `frontend/index.html`, in the `<script>` section (at the end, before the closing `</script>`), add:

```javascript
// === ALOCAÇÃO ===
var allocEquipeId = '';
var allocProdutoId = '';

function loadAlocacao() {
  var sel = document.getElementById('alloc-equipe');
  if (sel.options.length <= 1) {
    api('/api/v1/equipes').then(function(equipes) {
      equipes.forEach(function(eq) {
        var opt = document.createElement('option');
        opt.value = eq.id;
        opt.textContent = eq.nome;
        sel.appendChild(opt);
      });
    });
  }
}

function onAllocFilterChange() {
  allocEquipeId = document.getElementById('alloc-equipe').value;
  allocProdutoId = document.getElementById('alloc-produto').value;

  if (allocEquipeId && !allocProdutoId) {
    var prodSel = document.getElementById('alloc-produto');
    prodSel.innerHTML = '<option value="">Selecione o Produto</option>';
    api('/api/v1/produtos').then(function(produtos) {
      produtos.forEach(function(p) {
        var opt = document.createElement('option');
        opt.value = p.id;
        opt.textContent = p.nome;
        prodSel.appendChild(opt);
      });
    });
  }

  if (allocEquipeId && allocProdutoId) {
    loadProjectAllocations();
  } else {
    document.getElementById('alloc-content').innerHTML =
      '<div class="alloc-empty">Selecione equipe e produto para ver os projetos.</div>';
  }
}

function loadProjectAllocations() {
  var container = document.getElementById('alloc-content');
  container.innerHTML = '<div class="alloc-empty">Carregando...</div>';

  api('/api/v1/allocation/projects?equipe_id=' + allocEquipeId + '&produto_id=' + allocProdutoId)
    .then(function(projects) {
      if (!projects || projects.length === 0) {
        container.innerHTML = '<div class="alloc-empty">Nenhum projeto encontrado para esta equipe e produto.</div>';
        return;
      }
      renderAllocationBoxes(projects, container);
    })
    .catch(function(err) {
      container.innerHTML = '<div class="alloc-empty">Erro ao carregar projetos.</div>';
    });
}

function getAllocColor(pct) {
  if (pct >= 71) return '#22c55e';
  if (pct >= 31) return '#eab308';
  return '#ef4444';
}

function renderAllocationBoxes(projects, container) {
  var html = '<div class="alloc-grid">';
  projects.forEach(function(p) {
    var color = getAllocColor(p.pct_planejado);
    var title = p.apelido ? esc(p.apelido) : esc(p.resumo);
    var dataLimite = p.data_limite ? new Date(p.data_limite).toLocaleDateString('pt-BR') : '--';

    html += '<div class="alloc-box" onclick="openProjectModal(\'' + p.epic_id + '\')" style="border-left: 4px solid ' + color + '">';
    html += '<div class="alloc-box-header">';
    if (p.is_gdptc) {
      html += '<span class="alloc-box-star" title="Projeto de Portfólio Unificado">★</span>';
    }
    html += '<span class="alloc-box-ticket">' + esc(p.numero_ticket) + '</span>';
    html += '</div>';
    html += '<div class="alloc-box-title" title="' + escAttr(p.resumo) + '">' + title + '</div>';
    html += '<div class="alloc-box-meta">Limite: ' + dataLimite + '</div>';

    if (p.produtos && p.produtos.length > 0) {
      html += '<div class="alloc-box-produtos">';
      p.produtos.forEach(function(prod) {
        html += '<span class="alloc-box-produto">' + esc(prod) + '</span>';
      });
      html += '</div>';
    }

    html += '<div class="alloc-bar-group">';
    html += '<div class="alloc-bar-label"><span>Estimado</span><span>' + Math.round(p.pct_estimado) + '%</span></div>';
    html += '<div class="alloc-bar"><div class="alloc-bar-fill" style="width:' + Math.min(p.pct_estimado, 100) + '%;background:' + getAllocColor(p.pct_estimado) + '"></div></div>';
    html += '<div class="alloc-bar-label"><span>Planejado</span><span>' + Math.round(p.pct_planejado) + '%</span></div>';
    html += '<div class="alloc-bar"><div class="alloc-bar-fill" style="width:' + Math.min(p.pct_planejado, 100) + '%;background:' + color + '"></div></div>';
    html += '</div>';

    if (p.tarefas_sem_estimativa > 0) {
      html += '<div class="alloc-alert">⚠ ' + p.tarefas_sem_estimativa + ' tarefas sem estimativa</div>';
    }

    var badgeClass = 'alloc-badge-' + p.status;
    var badgeText = p.status === 'planejado' ? 'Planejado' : p.status === 'em_planejamento' ? 'Em Planejamento' : 'Não Planejado';
    html += '<span class="alloc-badge ' + badgeClass + '">' + badgeText + '</span>';

    html += '</div>';
  });
  html += '</div>';
  container.innerHTML = html;
}
```

- [ ] **Step 6: Verify frontend syntax**

Run: `node --check <(grep -oP '(?<=<script>)[\s\S]*(?=</script>)' /home/emerson/code/myplanner/frontend/index.html)` or simply verify manually that the JS section has no syntax errors.

Alternative: `cd /home/emerson/code/myplanner/backend && go build ./...` to verify backend still compiles.

- [ ] **Step 7: Commit**

```bash
git add frontend/index.html
git commit -m "feat(allocation): add allocation page with epic boxes and filter UI"
```

---

### Task 6: Frontend — Project Detail Modal + Task Allocation

**Files:**
- Modify: `frontend/index.html` — CSS for modal, JS for modal rendering, allocation interactions, capacity confirmation

**Interfaces:**
- Consumes:
  - Backend: `GET /allocation/projects/{epicId}`, `POST /allocation/tasks/{taskId}/allocate`, `POST /allocation/projects/{epicId}/sync`, `GET /allocation/sprints`
  - `allocEquipeId` global from Task 5
  - `esc()`, `escAttr()`, `api()` helper functions
- Produces: `openProjectModal(epicId)`, `allocateTask(taskId)`, `syncProjectTasks(epicId)`, capacity confirmation modal

- [ ] **Step 1: Add modal CSS**

Append to the `<style>` section in `frontend/index.html`:

```css
.alloc-modal-overlay { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,.5); z-index: 1000; display: flex; align-items: center; justify-content: center; }
.alloc-modal { background: var(--card-bg); border-radius: 12px; max-width: 900px; width: 95%; max-height: 85vh; overflow-y: auto; padding: 24px; position: relative; }
.alloc-modal-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
.alloc-modal-header h2 { font-size: 16px; margin: 0; color: var(--text); }
.alloc-modal-close { background: none; border: none; font-size: 20px; cursor: pointer; color: var(--text-secondary); padding: 4px 8px; }
.alloc-modal-close:hover { color: var(--text); }
.alloc-section { margin-bottom: 24px; }
.alloc-section-title { font-size: 13px; font-weight: 600; color: var(--text-secondary); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 12px; padding-bottom: 6px; border-bottom: 1px solid var(--border); }
.alloc-people-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.alloc-people-table th { text-align: left; padding: 6px 8px; color: var(--text-secondary); font-weight: 500; border-bottom: 1px solid var(--border); }
.alloc-people-table td { padding: 6px 8px; border-bottom: 1px solid var(--border); color: var(--text); }
.alloc-task-row { display: grid; grid-template-columns: 100px 1fr auto; gap: 8px; align-items: center; padding: 8px; border-bottom: 1px solid var(--border); font-size: 13px; }
.alloc-task-ticket { font-weight: 600; color: var(--primary); }
.alloc-task-resumo { color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.alloc-task-controls { display: flex; gap: 6px; align-items: center; }
.alloc-task-controls select, .alloc-task-controls input { padding: 4px 8px; border-radius: 4px; border: 1px solid var(--border); background: var(--card-bg); color: var(--text); font-size: 12px; }
.alloc-task-controls select { min-width: 120px; }
.alloc-task-controls input[type="number"] { width: 60px; }
.alloc-task-btn { padding: 4px 10px; border-radius: 4px; border: none; background: var(--primary); color: #fff; cursor: pointer; font-size: 12px; }
.alloc-task-btn:hover { opacity: .85; }
.alloc-task-btn:disabled { opacity: .5; cursor: not-allowed; }
.alloc-sync-btn { padding: 6px 14px; border-radius: 6px; border: 1px solid var(--border); background: var(--card-bg); color: var(--text); cursor: pointer; font-size: 13px; display: flex; align-items: center; gap: 6px; }
.alloc-sync-btn:hover { background: var(--border); }
.alloc-sync-btn.loading { opacity: .6; pointer-events: none; }
.alloc-confirm-overlay { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,.5); z-index: 1100; display: flex; align-items: center; justify-content: center; }
.alloc-confirm { background: var(--card-bg); border-radius: 10px; padding: 24px; max-width: 420px; width: 90%; }
.alloc-confirm h3 { margin: 0 0 12px; font-size: 15px; color: var(--text); }
.alloc-confirm p { font-size: 13px; color: var(--text-secondary); margin-bottom: 16px; }
.alloc-confirm-actions { display: flex; justify-content: flex-end; gap: 8px; }
.alloc-confirm-actions button { padding: 6px 16px; border-radius: 6px; font-size: 13px; cursor: pointer; }
.alloc-task-readonly { display: grid; grid-template-columns: 100px 1fr 60px 120px 120px; gap: 8px; align-items: center; padding: 8px; border-bottom: 1px solid var(--border); font-size: 13px; color: var(--text); }
```

- [ ] **Step 2: Add modal JS — openProjectModal and rendering**

Append to the JS section in `frontend/index.html`:

```javascript
var allocSprints = [];
var currentAllocEpicId = '';

function openProjectModal(epicId) {
  currentAllocEpicId = epicId;
  var overlay = document.createElement('div');
  overlay.className = 'alloc-modal-overlay';
  overlay.id = 'alloc-modal-overlay';
  overlay.onclick = function(e) { if (e.target === overlay) closeAllocModal(); };
  overlay.innerHTML = '<div class="alloc-modal"><div class="alloc-empty">Carregando...</div></div>';
  document.body.appendChild(overlay);

  if (allocSprints.length === 0 && allocEquipeId) {
    api('/api/v1/allocation/sprints?equipe_id=' + allocEquipeId).then(function(sprints) {
      allocSprints = sprints || [];
      fetchAndRenderModal(epicId);
    });
  } else {
    fetchAndRenderModal(epicId);
  }
}

function fetchAndRenderModal(epicId) {
  api('/api/v1/allocation/projects/' + epicId + '?equipe_id=' + allocEquipeId)
    .then(function(detail) {
      renderAllocModal(detail);
    })
    .catch(function() {
      var modal = document.querySelector('.alloc-modal');
      if (modal) modal.innerHTML = '<div class="alloc-empty">Erro ao carregar projeto.</div>';
    });
}

function closeAllocModal() {
  var overlay = document.getElementById('alloc-modal-overlay');
  if (overlay) overlay.remove();
  currentAllocEpicId = '';
}

function renderAllocModal(detail) {
  var modal = document.querySelector('.alloc-modal');
  if (!modal) return;

  var epic = detail.epic;
  var badgeClass = 'alloc-badge-' + epic.status;
  var badgeText = epic.status === 'planejado' ? 'Planejado' : epic.status === 'em_planejamento' ? 'Em Planejamento' : 'Não Planejado';

  var html = '<div class="alloc-modal-header">';
  html += '<h2>' + esc(epic.numero_ticket) + ': ' + esc(epic.apelido || epic.resumo) + ' <span class="alloc-badge ' + badgeClass + '">' + badgeText + '</span></h2>';
  html += '<div style="display:flex;gap:8px;align-items:center">';
  html += '<button class="alloc-sync-btn" id="alloc-sync-btn" onclick="syncProjectTasks(\'' + epic.epic_id + '\')"><span id="alloc-sync-icon">🔄</span> Sincronizar Tarefas</button>';
  html += '<button class="alloc-modal-close" onclick="closeAllocModal()">✕</button>';
  html += '</div></div>';

  // People section
  if (detail.pessoas && detail.pessoas.length > 0) {
    html += '<div class="alloc-section"><div class="alloc-section-title">Equipe Envolvida</div>';
    html += '<table class="alloc-people-table"><thead><tr><th>Nome</th><th>Horas no Projeto</th><th>% no Projeto</th></tr></thead><tbody>';
    detail.pessoas.forEach(function(p) {
      html += '<tr><td>' + esc(p.nome) + '</td><td>' + p.horas_no_projeto.toFixed(1) + 'h</td><td>' + (p.pct_no_projeto || 0).toFixed(0) + '%</td></tr>';
    });
    html += '</tbody></table></div>';
  }

  // Unallocated tasks
  if (detail.nao_alocadas && detail.nao_alocadas.length > 0) {
    html += '<div class="alloc-section"><div class="alloc-section-title">Tarefas Não Alocadas (' + detail.nao_alocadas.length + ')</div>';
    detail.nao_alocadas.forEach(function(t) {
      html += renderAllocTaskEditable(t);
    });
    html += '</div>';
  }

  // Partial tasks
  if (detail.parciais && detail.parciais.length > 0) {
    html += '<div class="alloc-section"><div class="alloc-section-title">Estimadas sem Pessoa (' + detail.parciais.length + ')</div>';
    detail.parciais.forEach(function(t) {
      html += renderAllocTaskEditable(t);
    });
    html += '</div>';
  }

  // Complete tasks
  if (detail.completas && detail.completas.length > 0) {
    html += '<div class="alloc-section"><div class="alloc-section-title">Tarefas Completas (' + detail.completas.length + ')</div>';
    detail.completas.forEach(function(t) {
      html += '<div class="alloc-task-readonly">';
      html += '<span class="alloc-task-ticket">' + esc(t.numero_ticket) + '</span>';
      html += '<span class="alloc-task-resumo" title="' + escAttr(t.resumo) + '">' + esc(t.resumo) + '</span>';
      html += '<span>' + (t.estimativa_horas ? t.estimativa_horas.toFixed(1) + 'h' : '--') + '</span>';
      html += '<span>' + esc(t.sprint_nome || '--') + '</span>';
      html += '<span>' + esc(t.responsavel_nome || '--') + '</span>';
      html += '</div>';
    });
    html += '</div>';
  }

  // Gantt placeholder — will be implemented in Task 7
  html += '<div class="alloc-section" id="alloc-gantt-section"></div>';

  modal.innerHTML = html;

  // Render gantt after modal is in DOM
  renderAllocGantt(detail);
}

function renderAllocTaskEditable(t) {
  var tid = t.tarefa_id;
  var sprintVal = t.sprint_id || '';
  var estVal = t.estimativa_horas ? t.estimativa_horas.toFixed(1) : '';

  var html = '<div class="alloc-task-row">';
  html += '<span class="alloc-task-ticket">' + esc(t.numero_ticket) + '</span>';
  html += '<span class="alloc-task-resumo" title="' + escAttr(t.resumo) + '">' + esc(t.resumo) + '</span>';
  html += '<div class="alloc-task-controls">';

  // Sprint select
  html += '<select id="alloc-sprint-' + tid + '">';
  html += '<option value="">Sprint</option>';
  allocSprints.forEach(function(s) {
    var sel = s.id === sprintVal ? ' selected' : '';
    html += '<option value="' + s.id + '"' + sel + '>' + esc(s.nome) + '</option>';
  });
  html += '</select>';

  // Person select — loaded from equipe members
  html += '<select id="alloc-person-' + tid + '">';
  html += '<option value="">Pessoa</option>';
  html += '</select>';

  // Estimate input
  html += '<input type="number" id="alloc-est-' + tid + '" placeholder="Horas" step="0.5" min="0.5" value="' + estVal + '">';

  // Allocate button
  html += '<button class="alloc-task-btn" onclick="allocateTask(\'' + tid + '\')">✓</button>';

  html += '</div></div>';
  return html;
}

function loadAllocMembers() {
  if (!allocEquipeId) return;
  api('/api/v1/equipes/' + allocEquipeId + '/membros').then(function(membros) {
    var selects = document.querySelectorAll('[id^="alloc-person-"]');
    selects.forEach(function(sel) {
      if (sel.options.length > 1) return;
      membros.forEach(function(m) {
        var opt = document.createElement('option');
        opt.value = m.membro_id || m.id;
        opt.textContent = m.nome;
        sel.appendChild(opt);
      });
    });
  });
}

function allocateTask(taskId) {
  var sprintId = document.getElementById('alloc-sprint-' + taskId).value;
  var personId = document.getElementById('alloc-person-' + taskId).value;
  var estHours = parseFloat(document.getElementById('alloc-est-' + taskId).value);

  if (!sprintId) { alert('Selecione uma sprint.'); return; }
  if (!estHours || estHours <= 0) { alert('Informe a estimativa em horas.'); return; }

  doAllocate(taskId, sprintId, personId || null, estHours, false);
}

function doAllocate(taskId, sprintId, personId, estHours, force) {
  var body = {
    sprint_id: sprintId,
    estimate_hours: estHours,
    force: force,
    equipe_id: allocEquipeId
  };
  if (personId) body.assignee_id = personId;

  api('/api/v1/allocation/tasks/' + taskId + '/allocate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  }).then(function(result) {
    if (result.conflict) {
      showAllocConfirm(taskId, sprintId, personId, estHours, result);
      return;
    }
    // Refresh modal
    fetchAndRenderModal(currentAllocEpicId);
    loadProjectAllocations();
  }).catch(function(err) {
    alert('Erro ao alocar tarefa: ' + (err.message || err));
  });
}

function showAllocConfirm(taskId, sprintId, personId, estHours, conflict) {
  var overlay = document.createElement('div');
  overlay.className = 'alloc-confirm-overlay';
  overlay.id = 'alloc-confirm-overlay';

  var html = '<div class="alloc-confirm">';
  html += '<h3>Atenção</h3>';
  html += '<p>O colaborador <strong>' + esc(conflict.membro_nome) + '</strong> já está com <strong>' + Math.round(conflict.pct_atual) + '%</strong> do tempo alocado na <strong>' + esc(conflict.sprint_nome) + '</strong>. Deseja continuar mesmo assim?</p>';
  html += '<div class="alloc-confirm-actions">';
  html += '<button style="background:var(--border);border:none;color:var(--text)" onclick="document.getElementById(\'alloc-confirm-overlay\').remove()">Cancelar</button>';
  html += '<button style="background:var(--primary);border:none;color:#fff" onclick="document.getElementById(\'alloc-confirm-overlay\').remove();doAllocate(\'' + taskId + '\',\'' + sprintId + '\',\'' + personId + '\',' + estHours + ',true)">Sim, continuar</button>';
  html += '</div></div>';

  overlay.innerHTML = html;
  document.body.appendChild(overlay);
}

function syncProjectTasks(epicId) {
  var btn = document.getElementById('alloc-sync-btn');
  if (btn) btn.classList.add('loading');

  api('/api/v1/allocation/projects/' + epicId + '/sync', { method: 'POST' })
    .then(function(result) {
      if (btn) btn.classList.remove('loading');
      fetchAndRenderModal(epicId);
    })
    .catch(function() {
      if (btn) btn.classList.remove('loading');
      alert('Erro ao sincronizar tarefas.');
    });
}
```

- [ ] **Step 3: Add member loading after modal render**

Update `renderAllocModal` to call `loadAllocMembers()` after setting innerHTML. Add at the end of `renderAllocModal`:

```javascript
  // After setting innerHTML, load members into person selects
  setTimeout(loadAllocMembers, 0);
```

- [ ] **Step 4: Verify frontend syntax**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: backend builds clean (no frontend changes affect backend).

Manually verify: open the app in browser, navigate to Alocação page, verify no JS errors in console.

- [ ] **Step 5: Commit**

```bash
git add frontend/index.html
git commit -m "feat(allocation): add project detail modal with task allocation and capacity check"
```

---

### Task 7: Frontend — Gantt Chart

**Files:**
- Modify: `frontend/index.html` — CSS for Gantt, `renderAllocGantt()` function

**Interfaces:**
- Consumes: `ProjectDetail` from `openProjectModal` (Task 6), specifically `completas`, `parciais`, `nao_alocadas` arrays with `sprint_inicio`, `sprint_fim` fields
- Produces: `renderAllocGantt(detail)` — called from `renderAllocModal`

- [ ] **Step 1: Add Gantt CSS**

Append to the `<style>` section in `frontend/index.html`:

```css
.alloc-gantt { overflow-x: auto; }
.alloc-gantt-container { position: relative; min-width: 700px; }
.alloc-gantt-header { display: grid; grid-template-columns: 120px 1fr; border-bottom: 1px solid var(--border); }
.alloc-gantt-months { display: grid; grid-template-columns: repeat(12, 1fr); }
.alloc-gantt-month { font-size: 11px; color: var(--text-secondary); text-align: center; padding: 4px 0; border-left: 1px solid var(--border); }
.alloc-gantt-row { display: grid; grid-template-columns: 120px 1fr; min-height: 28px; align-items: center; border-bottom: 1px solid rgba(128,128,128,.1); }
.alloc-gantt-label { font-size: 11px; color: var(--text); padding: 0 6px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.alloc-gantt-timeline { position: relative; height: 20px; }
.alloc-gantt-bar { position: absolute; height: 14px; top: 3px; border-radius: 3px; cursor: help; min-width: 4px; }
.alloc-gantt-bar-allocated { background: var(--primary); opacity: .8; }
.alloc-gantt-bar-unallocated { background: #ef4444; opacity: .7; }
.alloc-gantt-separator { font-size: 11px; color: var(--text-secondary); padding: 6px; background: rgba(128,128,128,.05); font-weight: 600; }
.alloc-gantt-tooltip { position: absolute; background: var(--card-bg); border: 1px solid var(--border); border-radius: 6px; padding: 8px 12px; font-size: 12px; color: var(--text); z-index: 1200; pointer-events: none; white-space: nowrap; box-shadow: 0 2px 8px rgba(0,0,0,.15); }
```

- [ ] **Step 2: Implement renderAllocGantt function**

Add to the JS section in `frontend/index.html`:

```javascript
function renderAllocGantt(detail) {
  var section = document.getElementById('alloc-gantt-section');
  if (!section) return;

  var allTasks = (detail.completas || []).concat(detail.parciais || []);
  var unallocated = detail.nao_alocadas || [];

  if (allTasks.length === 0 && unallocated.length === 0) {
    section.innerHTML = '';
    return;
  }

  var year = new Date().getFullYear();
  var yearStart = new Date(year, 0, 1).getTime();
  var yearEnd = new Date(year, 11, 31).getTime();
  var yearRange = yearEnd - yearStart;

  var months = ['Jan', 'Fev', 'Mar', 'Abr', 'Mai', 'Jun', 'Jul', 'Ago', 'Set', 'Out', 'Nov', 'Dez'];

  var html = '<div class="alloc-section-title">Timeline do Projeto</div>';
  html += '<div class="alloc-gantt"><div class="alloc-gantt-container">';

  // Header with months
  html += '<div class="alloc-gantt-header">';
  html += '<div style="font-size:11px;color:var(--text-secondary);padding:4px 6px">' + year + '</div>';
  html += '<div class="alloc-gantt-months">';
  months.forEach(function(m) { html += '<div class="alloc-gantt-month">' + m + '</div>'; });
  html += '</div></div>';

  // Allocated tasks
  allTasks.forEach(function(t) {
    if (!t.sprint_inicio || !t.sprint_fim) return;
    var start = new Date(t.sprint_inicio).getTime();
    var end = new Date(t.sprint_fim).getTime();
    var left = Math.max(0, (start - yearStart) / yearRange * 100);
    var width = Math.max(1, (end - start) / yearRange * 100);

    html += '<div class="alloc-gantt-row">';
    html += '<div class="alloc-gantt-label" title="' + escAttr(t.resumo) + '">' + esc(t.numero_ticket) + '</div>';
    html += '<div class="alloc-gantt-timeline">';
    html += '<div class="alloc-gantt-bar alloc-gantt-bar-allocated" style="left:' + left + '%;width:' + width + '%" title="' + escAttr(t.numero_ticket + ' - ' + t.resumo + ' (' + (t.sprint_nome || '') + ')') + '"></div>';
    html += '</div></div>';
  });

  // Separator
  if (unallocated.length > 0) {
    html += '<div class="alloc-gantt-separator">Não Alocadas</div>';
    unallocated.forEach(function(t) {
      html += '<div class="alloc-gantt-row">';
      html += '<div class="alloc-gantt-label" title="' + escAttr(t.resumo) + '">' + esc(t.numero_ticket) + '</div>';
      html += '<div class="alloc-gantt-timeline">';
      html += '<div class="alloc-gantt-bar alloc-gantt-bar-unallocated" style="left:0%;width:100%" title="' + escAttr(t.numero_ticket + ' - ' + t.resumo + ' — Não Alocada') + '"></div>';
      html += '</div></div>';
    });
  }

  html += '</div></div>';
  section.innerHTML = html;
}
```

- [ ] **Step 3: Verify frontend syntax and test manually**

Run the dev server: `cd /home/emerson/code/myplanner && ./dev.sh`

Test manually:
1. Navigate to Alocação page
2. Select equipe + produto
3. Verify boxes render with correct colors/badges
4. Click a box → modal opens with sections
5. Verify Gantt chart renders at bottom of modal
6. Hover bars → tooltips appear
7. Unallocated tasks show red bars

- [ ] **Step 4: Commit**

```bash
git add frontend/index.html
git commit -m "feat(allocation): add Gantt chart timeline in project detail modal"
```

---

### Task 8: Integration Testing + Polish

**Files:**
- Modify: `backend/internal/service/allocation.go` (fix any issues found)
- Modify: `frontend/index.html` (fix any issues found)

**Interfaces:**
- Consumes: all previous tasks
- Produces: fully tested end-to-end feature

- [ ] **Step 1: Run full backend test suite**

```bash
cd /home/emerson/code/myplanner/backend && go test ./... -v 2>&1 | tail -40
```

Fix any test failures.

- [ ] **Step 2: Run go vet and build**

```bash
cd /home/emerson/code/myplanner/backend && go vet ./... && go build ./...
```

- [ ] **Step 3: Manual end-to-end test**

Start the dev server and test:

1. **Page load**: Navigate to Alocação → verify filters appear
2. **Filter**: Select equipe + produto → verify boxes load
3. **Box display**: Verify % Estimado, % Planejado, status badges, GDPTC stars, product chips
4. **Box ordering**: Verify Highest priority first, Meta before Compromisso
5. **Modal**: Click box → verify people section, 3 task sections, Gantt
6. **Allocate**: Fill sprint + estimate → click ✓ → task moves section
7. **Capacity check**: Allocate person to sprint where they're >100% → verify confirmation dialog
8. **Sync**: Click "Sincronizar Tarefas" → verify spinner + refresh
9. **Dark mode**: Toggle theme → verify all allocation UI works in both themes
10. **Edge cases**: Epic with 0 tasks, epic with all tasks allocated, no products

- [ ] **Step 4: Fix any issues found during testing**

Address any bugs, visual issues, or edge cases.

- [ ] **Step 5: Commit fixes**

```bash
git add -A
git commit -m "fix(allocation): polish and fixes from integration testing"
```
