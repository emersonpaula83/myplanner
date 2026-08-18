# Sprint Planning Tab Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Planning" tab to sprint detail that lets coordinators plan the next sprint — drag-and-drop task reallocation, required field validation, bulk actions, and batch Jira push with progress tracking.

**Architecture:** Full client cache approach. Frontend loads capacity data from the backend once, stores all changes in a JS `planningState` object, recalculates capacity instantly on the client, and submits the complete changeset on "Finalizar Planning". Backend processes operations sequentially (DB + Jira) with progress polling.

**Tech Stack:** Go backend (chi router, pgx), vanilla JS SPA (single `frontend/index.html`), SortableJS (inline, no CDN), Jira REST API v3 + Agile API.

## Global Constraints

- Frontend is a single monolithic HTML file (`frontend/index.html`, ~9,140 lines) — no framework
- Sprint states from Jira: `"active"`, `"closed"`, `"future"`
- Capacity formula: `horasPorDia = 6.0`, business days exclude weekends + holidays
- Status buckets (must match backend `GetCapacity` in `internal/service/sprint.go:224-232` exactly):
  - `statusExecutado`: `"Code Review"`, `"Teste"`, `"Validação do Solicitante"`, `"Deploy"`, `"Concluído"`
  - `statusAmbos`: `"Teste"`, `"Validação do Solicitante"`, `"Deploy"`
  - `statusPendente`: `"Backlog"`, `"Desenvolvimento"`, `"Em Desenvolvimento"`, `"A Fazer"`
  - Skip entirely: `"Cancelado"`, `"Rejeitada"`
- Only active sprints show the Planning tab; visibility = `data_fim - now <= 3 days`
- Never auto-commit/push without explicit user consent
- Handler pattern: parse ID from chi URL param, optional equipe query param with `middleware.ValidateEquipeAccess`, delegate to service, `respondJSON`/`respondError`
- Jira client factory pattern: `buildClient(ctx, fonteDadosID)` resolves auth type (basic vs oauth2) — see `internal/service/equalizer.go:151-188`
- Module path: `github.com/emersonpaula83/myplanner/backend`

---

### Task 1: Jira Client — RemoveFromSprint Method

**Files:**
- Modify: `internal/jira/client.go:34-50` (Client interface) and `internal/jira/client.go:642-656` (after UpdateTimeEstimate)

**Interfaces:**
- Consumes: existing `doPut(ctx, path, payload)` helper on HTTPClient
- Produces: `RemoveFromSprint(ctx context.Context, issueKey string) error` — used by Task 3 (PlanningService.processBacklogMoves)

- [ ] **Step 1: Add `RemoveFromSprint` to the Client interface**

In `internal/jira/client.go`, add to the `Client` interface (after line 49, the `UpdateTimeEstimate` method):

```go
RemoveFromSprint(ctx context.Context, issueKey string) error
```

The full interface becomes (showing last two lines + new line):
```go
	UpdateTimeEstimate(ctx context.Context, issueKey string, seconds int) error
	RemoveFromSprint(ctx context.Context, issueKey string) error
}
```

- [ ] **Step 2: Implement `RemoveFromSprint` on HTTPClient**

Add after the `UpdateTimeEstimate` method (after line 656):

```go
func (c *HTTPClient) RemoveFromSprint(ctx context.Context, issueKey string) error {
	payload := map[string]any{
		"fields": map[string]any{
			"sprint": nil,
		},
	}
	_, err := c.doPut(ctx, "/rest/api/3/issue/"+issueKey, payload)
	if err != nil {
		return fmt.Errorf("removing issue %q from sprint: %w", issueKey, err)
	}
	return nil
}
```

- [ ] **Step 3: Add `RemoveFromSprint` to mock in test file**

In `internal/service/mocks_test.go`, find `mockJiraClient` (search for `type mockJiraClient struct`). Add the field and method:

Field in struct:
```go
removeFromSprintFn func(ctx context.Context, issueKey string) error
```

Method on struct:
```go
func (m *mockJiraClient) RemoveFromSprint(ctx context.Context, issueKey string) error {
	if m.removeFromSprintFn != nil {
		return m.removeFromSprintFn(ctx, issueKey)
	}
	return nil
}
```

- [ ] **Step 4: Verify build**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: BUILD SUCCESS — no compile errors.

- [ ] **Step 5: Run existing tests**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/jira/... ./internal/service/... -count=1 -timeout 30s`
Expected: All existing tests PASS (no regressions).

- [ ] **Step 6: Commit**

```bash
git add internal/jira/client.go internal/service/mocks_test.go
git commit -m "feat(jira): add RemoveFromSprint client method"
```

---

### Task 2: Planning Repository

**Files:**
- Create: `internal/repository/planning.go`

**Interfaces:**
- Consumes: `*pgxpool.Pool` (standard pgx pool)
- Produces: `PlanningRepository` struct with methods:
  - `GetNextSprint(ctx, boardID int, currentDataInicio time.Time) (*domain.Sprint, error)`
  - `GetAllTarefasBySprint(ctx, sprintID uuid.UUID) ([]PlanningTarefa, error)`
  - `UpdateTarefaEstimativa(ctx, tarefaID uuid.UUID, segundos int) error`
  - `UpdateTarefaTipoDemanda(ctx, tarefaID uuid.UUID, valor string) error`
  - `UpdateTarefaResponsavel(ctx, tarefaID uuid.UUID, responsavelID *uuid.UUID) error`
  - `MoveTarefaToSprint(ctx, tarefaID uuid.UUID, sprintID uuid.UUID) error`
  - `RemoveTarefaFromSprint(ctx, tarefaID uuid.UUID) error`
  - `GetSprintJiraID(ctx, sprintID uuid.UUID) (int, error)`

All consumed by Task 3 (PlanningService). `PlanningTarefa` used by Task 6 (frontend) via the API response.

- [ ] **Step 1: Create `internal/repository/planning.go`**

```go
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PlanningRepository struct {
	pool *pgxpool.Pool
}

func NewPlanningRepository(pool *pgxpool.Pool) *PlanningRepository {
	return &PlanningRepository{pool: pool}
}

func (r *PlanningRepository) GetNextSprint(ctx context.Context, boardID int, currentDataInicio time.Time) (*domain.Sprint, error) {
	var s domain.Sprint
	err := r.pool.QueryRow(ctx, `
		SELECT id, fonte_dados_id, projeto_id, jira_id, nome, estado,
		       data_inicio, data_fim, data_conclusao, board_id, created_at, updated_at
		FROM sprints
		WHERE board_id = $1 AND estado = 'future' AND data_inicio > $2
		ORDER BY data_inicio ASC
		LIMIT 1
	`, boardID, currentDataInicio).Scan(
		&s.ID, &s.FonteDadosID, &s.ProjetoID, &s.JiraID, &s.Nome, &s.Estado,
		&s.DataInicio, &s.DataFim, &s.DataConclusao, &s.BoardID, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting next sprint for board %d: %w", boardID, err)
	}
	return &s, nil
}

type PlanningTarefa struct {
	ID              uuid.UUID  `json:"id"`
	NumeroTicket    string     `json:"numero_ticket"`
	Resumo          string     `json:"resumo"`
	Tipo            string     `json:"tipo"`
	Status          string     `json:"status"`
	Prioridade      *string    `json:"prioridade"`
	EstimativaTempo *int       `json:"estimativa_tempo"`
	TipoDemanda     *string    `json:"tipo_demanda"`
	ResponsavelID   *uuid.UUID `json:"responsavel_id"`
	ProjetoID       uuid.UUID  `json:"projeto_id"`
	ProjetoChave    string     `json:"projeto_chave"`
}

func (r *PlanningRepository) GetAllTarefasBySprint(ctx context.Context, sprintID uuid.UUID) ([]PlanningTarefa, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.status, t.prioridade,
		       t.estimativa_tempo, t.tipo_demanda, t.responsavel_id,
		       p.id, p.chave
		FROM tarefas t
		INNER JOIN projetos p ON p.id = t.projeto_id
		WHERE t.sprint_id = $1 AND t.deleted_at IS NULL
		ORDER BY t.numero_ticket
	`, sprintID)
	if err != nil {
		return nil, fmt.Errorf("getting planning tarefas: %w", err)
	}
	defer rows.Close()

	var result []PlanningTarefa
	for rows.Next() {
		var t PlanningTarefa
		if err := rows.Scan(&t.ID, &t.NumeroTicket, &t.Resumo, &t.Tipo, &t.Status,
			&t.Prioridade, &t.EstimativaTempo, &t.TipoDemanda, &t.ResponsavelID,
			&t.ProjetoID, &t.ProjetoChave); err != nil {
			return nil, fmt.Errorf("scanning planning tarefa: %w", err)
		}
		result = append(result, t)
	}
	return result, nil
}

func (r *PlanningRepository) UpdateTarefaEstimativa(ctx context.Context, tarefaID uuid.UUID, segundos int) error {
	_, err := r.pool.Exec(ctx, `UPDATE tarefas SET estimativa_tempo = $2, updated_at = NOW() WHERE id = $1`, tarefaID, segundos)
	if err != nil {
		return fmt.Errorf("updating estimativa for %s: %w", tarefaID, err)
	}
	return nil
}

func (r *PlanningRepository) UpdateTarefaTipoDemanda(ctx context.Context, tarefaID uuid.UUID, valor string) error {
	_, err := r.pool.Exec(ctx, `UPDATE tarefas SET tipo_demanda = $2, updated_at = NOW() WHERE id = $1`, tarefaID, valor)
	if err != nil {
		return fmt.Errorf("updating tipo_demanda for %s: %w", tarefaID, err)
	}
	return nil
}

func (r *PlanningRepository) UpdateTarefaResponsavel(ctx context.Context, tarefaID uuid.UUID, responsavelID *uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE tarefas SET responsavel_id = $2, updated_at = NOW() WHERE id = $1`, tarefaID, responsavelID)
	if err != nil {
		return fmt.Errorf("updating responsavel for %s: %w", tarefaID, err)
	}
	return nil
}

func (r *PlanningRepository) MoveTarefaToSprint(ctx context.Context, tarefaID uuid.UUID, sprintID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE tarefas SET sprint_id = $2, updated_at = NOW() WHERE id = $1`, tarefaID, sprintID)
	if err != nil {
		return fmt.Errorf("moving tarefa %s to sprint %s: %w", tarefaID, sprintID, err)
	}
	return nil
}

func (r *PlanningRepository) RemoveTarefaFromSprint(ctx context.Context, tarefaID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE tarefas SET sprint_id = NULL, updated_at = NOW() WHERE id = $1`, tarefaID)
	if err != nil {
		return fmt.Errorf("removing tarefa %s from sprint: %w", tarefaID, err)
	}
	return nil
}

func (r *PlanningRepository) GetSprintJiraID(ctx context.Context, sprintID uuid.UUID) (int, error) {
	var jiraID int
	err := r.pool.QueryRow(ctx, `SELECT jira_id FROM sprints WHERE id = $1`, sprintID).Scan(&jiraID)
	if err != nil {
		return 0, fmt.Errorf("getting sprint jira_id for %s: %w", sprintID, err)
	}
	return jiraID, nil
}
```

- [ ] **Step 2: Verify build**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: BUILD SUCCESS.

- [ ] **Step 3: Commit**

```bash
git add internal/repository/planning.go
git commit -m "feat(planning): add planning repository with task mutation queries"
```

---

### Task 3: Planning Service

**Files:**
- Create: `internal/service/planning.go`

**Interfaces:**
- Consumes:
  - `PlanningRepoStore` (new interface — see below)
  - `SprintRepoStore` (existing, from `internal/service/interfaces.go:25-52`) — for `GetByID`, `GetEquipeBoardID`, `GetMembroJiraAccountID`
  - `FonteDadosStore` (existing, from `internal/service/interfaces.go:15-19`) — for `buildClient`
  - `ClientFactory`, `OAuthClientFactory` (from `internal/service/sync.go:21-22`)
  - `jira.OAuthService` (from `internal/jira/`)
  - `jira.Client` interface (from Task 1, with `RemoveFromSprint`)
- Produces:
  - `PlanningService` struct
  - `GetNextSprint(ctx, currentSprintID uuid.UUID, equipeID *uuid.UUID) (*NextSprintResult, error)`
  - `Apply(ctx, req PlanningApplyRequest) (string, error)` — returns jobID
  - `GetProgress(jobID string) *PlanningJobProgress`
  - Types: `NextSprintResult`, `PlanningApplyRequest`, `PlanningChanges`, `PlanningJobProgress`, `PlanningOperation`

All consumed by Task 4 (PlanningHandler).

- [ ] **Step 1: Add `PlanningRepoStore` interface to `internal/service/interfaces.go`**

Add after the `SyncScheduleStore` interface (end of file, before the closing):

```go
// PlanningRepoStore abstracts *repository.PlanningRepository for testing.
// Consumed by: PlanningService.
type PlanningRepoStore interface {
	GetNextSprint(ctx context.Context, boardID int, currentDataInicio time.Time) (*domain.Sprint, error)
	GetAllTarefasBySprint(ctx context.Context, sprintID uuid.UUID) ([]repository.PlanningTarefa, error)
	UpdateTarefaEstimativa(ctx context.Context, tarefaID uuid.UUID, segundos int) error
	UpdateTarefaTipoDemanda(ctx context.Context, tarefaID uuid.UUID, valor string) error
	UpdateTarefaResponsavel(ctx context.Context, tarefaID uuid.UUID, responsavelID *uuid.UUID) error
	MoveTarefaToSprint(ctx context.Context, tarefaID uuid.UUID, sprintID uuid.UUID) error
	RemoveTarefaFromSprint(ctx context.Context, tarefaID uuid.UUID) error
	GetSprintJiraID(ctx context.Context, sprintID uuid.UUID) (int, error)
}
```

- [ ] **Step 2: Create `internal/service/planning.go`**

```go
package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type NextSprintResult struct {
	Sprint domain.Sprint                `json:"sprint"`
	Tarefas []repository.PlanningTarefa `json:"tarefas"`
}

type ReassignChange struct {
	TarefaID          uuid.UUID  `json:"tarefa_id"`
	TarefaKey         string     `json:"tarefa_key"`
	NovoResponsavelID *uuid.UUID `json:"novo_responsavel_id"`
}

type EstimateChange struct {
	TarefaID  uuid.UUID `json:"tarefa_id"`
	TarefaKey string    `json:"tarefa_key"`
	Segundos  int       `json:"segundos"`
}

type TipoDemandaChange struct {
	TarefaID  uuid.UUID `json:"tarefa_id"`
	TarefaKey string    `json:"tarefa_key"`
	Valor     string    `json:"valor"`
}

type MoveSprintChange struct {
	TarefaID           uuid.UUID `json:"tarefa_id"`
	TarefaKey          string    `json:"tarefa_key"`
	DestinoSprintJiraID int      `json:"destino_sprint_jira_id"`
}

type MoveBacklogChange struct {
	TarefaID  uuid.UUID `json:"tarefa_id"`
	TarefaKey string    `json:"tarefa_key"`
}

type PlanningChanges struct {
	Reassigned      []ReassignChange    `json:"reassigned"`
	Estimated       []EstimateChange    `json:"estimated"`
	TipoDemanda     []TipoDemandaChange `json:"tipo_demanda"`
	MovedNextSprint []MoveSprintChange  `json:"moved_next_sprint"`
	MovedBacklog    []MoveBacklogChange `json:"moved_backlog"`
}

type PlanningApplyRequest struct {
	SprintID     uuid.UUID       `json:"-"`
	FonteDadosID uuid.UUID       `json:"fonte_dados_id"`
	Changes      PlanningChanges `json:"changes"`
}

type PlanningOperation struct {
	Key    string `json:"key"`
	Action string `json:"action"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type PlanningJobProgress struct {
	Total      int                  `json:"total"`
	Completed  int                  `json:"completed"`
	Current    string               `json:"current"`
	Operations []PlanningOperation  `json:"operations"`
	Finished   bool                 `json:"finished"`
	Errors     []PlanningOperation  `json:"errors"`
}

type PlanningService struct {
	planRepo           PlanningRepoStore
	sprintRepo         SprintRepoStore
	fdRepo             FonteDadosStore
	clientFactory      ClientFactory
	oauthClientFactory OAuthClientFactory
	oauthSvc           *jira.OAuthService
	rateLimit          int
	logger             *zap.Logger
	jobs               sync.Map
}

func NewPlanningService(
	planRepo PlanningRepoStore,
	sprintRepo SprintRepoStore,
	fdRepo FonteDadosStore,
	clientFactory ClientFactory,
	oauthClientFactory OAuthClientFactory,
	oauthSvc *jira.OAuthService,
	rateLimit int,
	logger *zap.Logger,
) *PlanningService {
	return &PlanningService{
		planRepo:           planRepo,
		sprintRepo:         sprintRepo,
		fdRepo:             fdRepo,
		clientFactory:      clientFactory,
		oauthClientFactory: oauthClientFactory,
		oauthSvc:           oauthSvc,
		rateLimit:          rateLimit,
		logger:             logger,
	}
}

func (s *PlanningService) GetNextSprint(ctx context.Context, currentSprintID uuid.UUID, equipeID *uuid.UUID) (*NextSprintResult, error) {
	currentSprint, err := s.sprintRepo.GetByID(ctx, currentSprintID)
	if err != nil {
		return nil, fmt.Errorf("getting current sprint: %w", err)
	}
	if currentSprint.BoardID == nil {
		return nil, fmt.Errorf("current sprint has no board_id")
	}
	if currentSprint.DataInicio == nil {
		return nil, fmt.Errorf("current sprint has no data_inicio")
	}

	nextSprint, err := s.planRepo.GetNextSprint(ctx, *currentSprint.BoardID, *currentSprint.DataInicio)
	if err != nil {
		return nil, fmt.Errorf("finding next sprint: %w", err)
	}
	if nextSprint == nil {
		return nil, nil
	}

	tarefas, err := s.planRepo.GetAllTarefasBySprint(ctx, nextSprint.ID)
	if err != nil {
		return nil, fmt.Errorf("getting tarefas for next sprint: %w", err)
	}
	if tarefas == nil {
		tarefas = []repository.PlanningTarefa{}
	}

	return &NextSprintResult{
		Sprint:  *nextSprint,
		Tarefas: tarefas,
	}, nil
}

func (s *PlanningService) Apply(ctx context.Context, req PlanningApplyRequest) (string, error) {
	client, err := s.buildClient(ctx, req.FonteDadosID)
	if err != nil {
		return "", fmt.Errorf("building jira client: %w", err)
	}

	ops := s.buildOperations(req.Changes)
	if len(ops) == 0 {
		return "", fmt.Errorf("no operations to apply")
	}

	jobID := uuid.New().String()
	progress := &PlanningJobProgress{
		Total:      len(ops),
		Operations: ops,
		Errors:     []PlanningOperation{},
	}
	s.jobs.Store(jobID, progress)

	go s.processJob(context.Background(), jobID, req, client, progress)

	return jobID, nil
}

func (s *PlanningService) GetProgress(jobID string) *PlanningJobProgress {
	val, ok := s.jobs.Load(jobID)
	if !ok {
		return nil
	}
	return val.(*PlanningJobProgress)
}

func (s *PlanningService) buildOperations(changes PlanningChanges) []PlanningOperation {
	var ops []PlanningOperation
	for _, c := range changes.Reassigned {
		ops = append(ops, PlanningOperation{Key: c.TarefaKey, Action: "assign", Status: "pending"})
	}
	for _, c := range changes.Estimated {
		ops = append(ops, PlanningOperation{Key: c.TarefaKey, Action: "estimate", Status: "pending"})
	}
	for _, c := range changes.TipoDemanda {
		ops = append(ops, PlanningOperation{Key: c.TarefaKey, Action: "tipo_demanda", Status: "pending"})
	}
	for _, c := range changes.MovedNextSprint {
		ops = append(ops, PlanningOperation{Key: c.TarefaKey, Action: "move_sprint", Status: "pending"})
	}
	for _, c := range changes.MovedBacklog {
		ops = append(ops, PlanningOperation{Key: c.TarefaKey, Action: "move_backlog", Status: "pending"})
	}
	return ops
}

func (s *PlanningService) processJob(ctx context.Context, jobID string, req PlanningApplyRequest, client jira.Client, progress *PlanningJobProgress) {
	idx := 0

	for i, c := range req.Changes.Reassigned {
		_ = i
		progress.Operations[idx].Status = "running"
		progress.Current = c.TarefaKey + " — Atualizando responsável"

		var jiraErr error
		if c.NovoResponsavelID != nil {
			jiraAccountID, err := s.sprintRepo.GetMembroJiraAccountID(ctx, *c.NovoResponsavelID)
			if err != nil {
				jiraErr = fmt.Errorf("membro não encontrado: %w", err)
			} else {
				if err := s.planRepo.UpdateTarefaResponsavel(ctx, c.TarefaID, c.NovoResponsavelID); err != nil {
					jiraErr = err
				} else if err := client.AssignIssue(ctx, c.TarefaKey, jiraAccountID); err != nil {
					jiraErr = err
				}
			}
		} else {
			if err := s.planRepo.UpdateTarefaResponsavel(ctx, c.TarefaID, nil); err != nil {
				jiraErr = err
			}
		}

		if jiraErr != nil {
			s.logger.Warn("planning assign failed", zap.String("key", c.TarefaKey), zap.Error(jiraErr))
			progress.Operations[idx].Status = "error"
			progress.Operations[idx].Error = jiraErr.Error()
			progress.Errors = append(progress.Errors, progress.Operations[idx])
		} else {
			progress.Operations[idx].Status = "done"
		}
		progress.Completed++
		idx++
	}

	for _, c := range req.Changes.Estimated {
		progress.Operations[idx].Status = "running"
		progress.Current = c.TarefaKey + " — Atualizando estimativa"

		var jiraErr error
		if err := s.planRepo.UpdateTarefaEstimativa(ctx, c.TarefaID, c.Segundos); err != nil {
			jiraErr = err
		} else if err := client.UpdateTimeEstimate(ctx, c.TarefaKey, c.Segundos); err != nil {
			jiraErr = err
		}

		if jiraErr != nil {
			s.logger.Warn("planning estimate failed", zap.String("key", c.TarefaKey), zap.Error(jiraErr))
			progress.Operations[idx].Status = "error"
			progress.Operations[idx].Error = jiraErr.Error()
			progress.Errors = append(progress.Errors, progress.Operations[idx])
		} else {
			progress.Operations[idx].Status = "done"
		}
		progress.Completed++
		idx++
	}

	for _, c := range req.Changes.TipoDemanda {
		progress.Operations[idx].Status = "running"
		progress.Current = c.TarefaKey + " — Atualizando tipo de demanda"

		if err := s.planRepo.UpdateTarefaTipoDemanda(ctx, c.TarefaID, c.Valor); err != nil {
			s.logger.Warn("planning tipo_demanda failed", zap.String("key", c.TarefaKey), zap.Error(err))
			progress.Operations[idx].Status = "error"
			progress.Operations[idx].Error = err.Error()
			progress.Errors = append(progress.Errors, progress.Operations[idx])
		} else {
			progress.Operations[idx].Status = "done"
		}
		progress.Completed++
		idx++
	}

	for _, c := range req.Changes.MovedNextSprint {
		progress.Operations[idx].Status = "running"
		progress.Current = c.TarefaKey + " — Movendo para próxima sprint"

		var jiraErr error
		sprintID, err := s.findSprintIDByJiraID(ctx, req.SprintID, c.DestinoSprintJiraID)
		if err != nil {
			jiraErr = err
		} else {
			if err := s.planRepo.MoveTarefaToSprint(ctx, c.TarefaID, sprintID); err != nil {
				jiraErr = err
			} else if err := client.MoveToSprint(ctx, c.DestinoSprintJiraID, c.TarefaKey); err != nil {
				jiraErr = err
			}
		}

		if jiraErr != nil {
			s.logger.Warn("planning move_sprint failed", zap.String("key", c.TarefaKey), zap.Error(jiraErr))
			progress.Operations[idx].Status = "error"
			progress.Operations[idx].Error = jiraErr.Error()
			progress.Errors = append(progress.Errors, progress.Operations[idx])
		} else {
			progress.Operations[idx].Status = "done"
		}
		progress.Completed++
		idx++
	}

	for _, c := range req.Changes.MovedBacklog {
		progress.Operations[idx].Status = "running"
		progress.Current = c.TarefaKey + " — Movendo para backlog"

		var jiraErr error
		if err := s.planRepo.RemoveTarefaFromSprint(ctx, c.TarefaID); err != nil {
			jiraErr = err
		} else if err := client.RemoveFromSprint(ctx, c.TarefaKey); err != nil {
			jiraErr = err
		}

		if jiraErr != nil {
			s.logger.Warn("planning move_backlog failed", zap.String("key", c.TarefaKey), zap.Error(jiraErr))
			progress.Operations[idx].Status = "error"
			progress.Operations[idx].Error = jiraErr.Error()
			progress.Errors = append(progress.Errors, progress.Operations[idx])
		} else {
			progress.Operations[idx].Status = "done"
		}
		progress.Completed++
		idx++
	}

	progress.Current = ""
	progress.Finished = true
}

func (s *PlanningService) findSprintIDByJiraID(ctx context.Context, currentSprintID uuid.UUID, jiraID int) (uuid.UUID, error) {
	currentSprint, err := s.sprintRepo.GetByID(ctx, currentSprintID)
	if err != nil {
		return uuid.Nil, err
	}
	if currentSprint.BoardID == nil || currentSprint.DataInicio == nil {
		return uuid.Nil, fmt.Errorf("current sprint missing board_id or data_inicio")
	}
	nextSprint, err := s.planRepo.GetNextSprint(ctx, *currentSprint.BoardID, *currentSprint.DataInicio)
	if err != nil {
		return uuid.Nil, err
	}
	if nextSprint == nil {
		return uuid.Nil, fmt.Errorf("next sprint not found")
	}
	// the next-next sprint (for "mover próxima sprint" from the planning sprint)
	nextNextSprint, err := s.planRepo.GetNextSprint(ctx, *currentSprint.BoardID, *nextSprint.DataInicio)
	if err != nil {
		return uuid.Nil, err
	}
	if nextNextSprint != nil && nextNextSprint.JiraID == jiraID {
		return nextNextSprint.ID, nil
	}
	// fallback: find any sprint with matching jira_id on the same board
	return uuid.Nil, fmt.Errorf("no local sprint found with jira_id %d", jiraID)
}

func (s *PlanningService) buildClient(ctx context.Context, fonteDadosID uuid.UUID) (jira.Client, error) {
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
				return nil, fmt.Errorf("fonte %s: oauth token expired and no oauth service configured", fonte.Nome)
			}
			newTokens, err := s.oauthSvc.RefreshAccessToken(ctx, *fonte.OAuth2RefreshToken)
			if err != nil {
				return nil, fmt.Errorf("refreshing oauth token for %s: %w", fonte.Nome, err)
			}
			expiry := newTokens.Expiry()
			if err := s.fdRepo.SaveOAuthTokens(ctx, fonte.ID, fonte.BaseURL, newTokens.AccessToken, newTokens.RefreshToken, expiry); err != nil {
				return nil, fmt.Errorf("saving refreshed tokens: %w", err)
			}
			accessToken = newTokens.AccessToken
		}
		return s.oauthClientFactory(fonte.BaseURL, accessToken, s.rateLimit, s.logger), nil
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

Note: Add the missing `"time"` import and `domain` import. The full import block should be:

```go
import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)
```

- [ ] **Step 3: Verify build**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: BUILD SUCCESS.

- [ ] **Step 4: Write unit test for GetNextSprint**

Create `internal/service/planning_test.go`:

```go
package service

import (
	"context"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockPlanningRepoStore struct {
	getNextSprintFn          func(ctx context.Context, boardID int, currentDataInicio time.Time) (*domain.Sprint, error)
	getAllTarefasBySprintFn   func(ctx context.Context, sprintID uuid.UUID) ([]repository.PlanningTarefa, error)
	updateTarefaEstimativaFn func(ctx context.Context, tarefaID uuid.UUID, segundos int) error
	updateTarefaTipoDemandaFn func(ctx context.Context, tarefaID uuid.UUID, valor string) error
	updateTarefaResponsavelFn func(ctx context.Context, tarefaID uuid.UUID, responsavelID *uuid.UUID) error
	moveTarefaToSprintFn     func(ctx context.Context, tarefaID uuid.UUID, sprintID uuid.UUID) error
	removeTarefaFromSprintFn func(ctx context.Context, tarefaID uuid.UUID) error
	getSprintJiraIDFn        func(ctx context.Context, sprintID uuid.UUID) (int, error)
}

func (m *mockPlanningRepoStore) GetNextSprint(ctx context.Context, boardID int, currentDataInicio time.Time) (*domain.Sprint, error) {
	return m.getNextSprintFn(ctx, boardID, currentDataInicio)
}
func (m *mockPlanningRepoStore) GetAllTarefasBySprint(ctx context.Context, sprintID uuid.UUID) ([]repository.PlanningTarefa, error) {
	return m.getAllTarefasBySprintFn(ctx, sprintID)
}
func (m *mockPlanningRepoStore) UpdateTarefaEstimativa(ctx context.Context, tarefaID uuid.UUID, segundos int) error {
	if m.updateTarefaEstimativaFn != nil {
		return m.updateTarefaEstimativaFn(ctx, tarefaID, segundos)
	}
	return nil
}
func (m *mockPlanningRepoStore) UpdateTarefaTipoDemanda(ctx context.Context, tarefaID uuid.UUID, valor string) error {
	if m.updateTarefaTipoDemandaFn != nil {
		return m.updateTarefaTipoDemandaFn(ctx, tarefaID, valor)
	}
	return nil
}
func (m *mockPlanningRepoStore) UpdateTarefaResponsavel(ctx context.Context, tarefaID uuid.UUID, responsavelID *uuid.UUID) error {
	if m.updateTarefaResponsavelFn != nil {
		return m.updateTarefaResponsavelFn(ctx, tarefaID, responsavelID)
	}
	return nil
}
func (m *mockPlanningRepoStore) MoveTarefaToSprint(ctx context.Context, tarefaID uuid.UUID, sprintID uuid.UUID) error {
	if m.moveTarefaToSprintFn != nil {
		return m.moveTarefaToSprintFn(ctx, tarefaID, sprintID)
	}
	return nil
}
func (m *mockPlanningRepoStore) RemoveTarefaFromSprint(ctx context.Context, tarefaID uuid.UUID) error {
	if m.removeTarefaFromSprintFn != nil {
		return m.removeTarefaFromSprintFn(ctx, tarefaID)
	}
	return nil
}
func (m *mockPlanningRepoStore) GetSprintJiraID(ctx context.Context, sprintID uuid.UUID) (int, error) {
	if m.getSprintJiraIDFn != nil {
		return m.getSprintJiraIDFn(ctx, sprintID)
	}
	return 0, nil
}

func TestPlanningService_GetNextSprint(t *testing.T) {
	boardID := 42
	now := time.Now()
	currentSprintID := uuid.New()
	nextSprintID := uuid.New()
	estado := "active"
	estadoFuture := "future"
	tarefaID := uuid.New()

	sprintRepo := &mockSprintRepoStore{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
			return &domain.Sprint{
				ID:         currentSprintID,
				BoardID:    &boardID,
				Estado:     &estado,
				DataInicio: &now,
				DataFim:    func() *time.Time { t := now.Add(14 * 24 * time.Hour); return &t }(),
			}, nil
		},
	}

	planRepo := &mockPlanningRepoStore{
		getNextSprintFn: func(ctx context.Context, bID int, di time.Time) (*domain.Sprint, error) {
			if bID != boardID {
				t.Errorf("expected boardID %d, got %d", boardID, bID)
			}
			nextStart := now.Add(15 * 24 * time.Hour)
			nextEnd := now.Add(29 * 24 * time.Hour)
			return &domain.Sprint{
				ID:         nextSprintID,
				BoardID:    &boardID,
				Estado:     &estadoFuture,
				Nome:       "Sprint 11",
				DataInicio: &nextStart,
				DataFim:    &nextEnd,
			}, nil
		},
		getAllTarefasBySprintFn: func(ctx context.Context, sprintID uuid.UUID) ([]repository.PlanningTarefa, error) {
			if sprintID != nextSprintID {
				t.Errorf("expected nextSprintID, got %s", sprintID)
			}
			return []repository.PlanningTarefa{
				{ID: tarefaID, NumeroTicket: "PROJ-101", Resumo: "Test task", Tipo: "Story", Status: "Backlog"},
			}, nil
		},
	}

	svc := NewPlanningService(planRepo, sprintRepo, nil, nil, nil, nil, 5, zap.NewNop())
	result, err := svc.GetNextSprint(context.Background(), currentSprintID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Sprint.ID != nextSprintID {
		t.Errorf("expected sprint ID %s, got %s", nextSprintID, result.Sprint.ID)
	}
	if len(result.Tarefas) != 1 {
		t.Fatalf("expected 1 tarefa, got %d", len(result.Tarefas))
	}
	if result.Tarefas[0].NumeroTicket != "PROJ-101" {
		t.Errorf("expected PROJ-101, got %s", result.Tarefas[0].NumeroTicket)
	}
}

func TestPlanningService_GetNextSprint_NoNext(t *testing.T) {
	boardID := 42
	now := time.Now()
	currentSprintID := uuid.New()
	estado := "active"

	sprintRepo := &mockSprintRepoStore{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
			return &domain.Sprint{
				ID:         currentSprintID,
				BoardID:    &boardID,
				Estado:     &estado,
				DataInicio: &now,
			}, nil
		},
	}

	planRepo := &mockPlanningRepoStore{
		getNextSprintFn: func(ctx context.Context, bID int, di time.Time) (*domain.Sprint, error) {
			return nil, nil
		},
	}

	svc := NewPlanningService(planRepo, sprintRepo, nil, nil, nil, nil, 5, zap.NewNop())
	result, err := svc.GetNextSprint(context.Background(), currentSprintID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result when no next sprint, got %+v", result)
	}
}

func TestPlanningService_BuildOperations(t *testing.T) {
	svc := &PlanningService{}
	changes := PlanningChanges{
		Reassigned:      []ReassignChange{{TarefaKey: "P-1"}},
		Estimated:       []EstimateChange{{TarefaKey: "P-2"}},
		TipoDemanda:     []TipoDemandaChange{{TarefaKey: "P-3"}},
		MovedNextSprint: []MoveSprintChange{{TarefaKey: "P-4"}},
		MovedBacklog:    []MoveBacklogChange{{TarefaKey: "P-5"}},
	}
	ops := svc.buildOperations(changes)
	if len(ops) != 5 {
		t.Fatalf("expected 5 operations, got %d", len(ops))
	}
	expected := []struct{ key, action string }{
		{"P-1", "assign"}, {"P-2", "estimate"}, {"P-3", "tipo_demanda"},
		{"P-4", "move_sprint"}, {"P-5", "move_backlog"},
	}
	for i, e := range expected {
		if ops[i].Key != e.key || ops[i].Action != e.action {
			t.Errorf("op[%d]: expected %s/%s, got %s/%s", i, e.key, e.action, ops[i].Key, ops[i].Action)
		}
		if ops[i].Status != "pending" {
			t.Errorf("op[%d]: expected pending, got %s", i, ops[i].Status)
		}
	}
}
```

- [ ] **Step 5: Run tests**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/service/ -run TestPlanning -v -count=1`
Expected: All 3 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/service/interfaces.go internal/service/planning.go internal/service/planning_test.go
git commit -m "feat(planning): add PlanningService with GetNextSprint, Apply, and progress tracking"
```

---

### Task 4: Planning Handler + Route Registration

**Files:**
- Create: `internal/handler/planning.go`
- Modify: `cmd/api/main.go:113-114` (instantiation) and `cmd/api/main.go:330` (route registration)

**Interfaces:**
- Consumes:
  - `PlanningServiceInterface` (new handler-level interface, wrapping `PlanningService` methods)
  - `middleware.ValidateEquipeAccess` (existing)
  - `respondJSON`, `respondError` (existing in `internal/handler/response.go`)
- Produces:
  - `PlanningHandler` struct with `GetNextSprint`, `Apply`, `GetProgress` HTTP handlers
  - Routes: `GET /sprints/{id}/next`, `POST /sprints/{id}/planning/apply`, `GET /sprints/{id}/planning/progress`

Consumed by frontend (Task 5 and Task 6).

- [ ] **Step 1: Create `internal/handler/planning.go`**

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type PlanningServiceInterface interface {
	GetNextSprint(ctx context.Context, currentSprintID uuid.UUID, equipeID *uuid.UUID) (*service.NextSprintResult, error)
	Apply(ctx context.Context, req service.PlanningApplyRequest) (string, error)
	GetProgress(jobID string) *service.PlanningJobProgress
}

type PlanningHandler struct {
	svc    PlanningServiceInterface
	logger *zap.Logger
}

func NewPlanningHandler(svc PlanningServiceInterface, logger *zap.Logger) *PlanningHandler {
	return &PlanningHandler{svc: svc, logger: logger}
}

func (h *PlanningHandler) GetNextSprint(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}

	var equipeID *uuid.UUID
	if e := r.URL.Query().Get("equipe_id"); e != "" {
		id, err := uuid.Parse(e)
		if err == nil {
			if err := middleware.ValidateEquipeAccess(r.Context(), []uuid.UUID{id}); err != nil {
				respondError(w, http.StatusForbidden, err.Error())
				return
			}
			equipeID = &id
		}
	}

	result, err := h.svc.GetNextSprint(r.Context(), sprintID, equipeID)
	if err != nil {
		h.logger.Error("getting next sprint", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar próxima sprint")
		return
	}
	if result == nil {
		respondError(w, http.StatusNotFound, "nenhuma sprint futura encontrada")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (h *PlanningHandler) Apply(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}

	var req service.PlanningApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	req.SprintID = sprintID

	if req.FonteDadosID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "fonte_dados_id obrigatório")
		return
	}

	jobID, err := h.svc.Apply(r.Context(), req)
	if err != nil {
		h.logger.Error("applying planning", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao aplicar planning")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"job_id": jobID})
}

func (h *PlanningHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		respondError(w, http.StatusBadRequest, "job_id obrigatório")
		return
	}

	progress := h.svc.GetProgress(jobID)
	if progress == nil {
		respondError(w, http.StatusNotFound, "job não encontrado")
		return
	}

	respondJSON(w, http.StatusOK, progress)
}
```

- [ ] **Step 2: Register service and handler in `cmd/api/main.go`**

After the `allocHandler` instantiation block (around line 186 — after `allocHandler := handler.NewAllocationHandler(allocSvc, logger)`), add:

```go
	planningRepo := repository.NewPlanningRepository(pool)
	planningSvc := service.NewPlanningService(planningRepo, sprintRepo, fonteDadosRepo, clientFactory, oauthClientFactory, oauthSvc, cfg.Sync.RateLimitPerSec, logger)
	planningHandler := handler.NewPlanningHandler(planningSvc, logger)
```

- [ ] **Step 3: Register routes in `cmd/api/main.go`**

After the equalizer routes (after line 330 — `r.Post("/sprints/{id}/equalizer/apply", equalizerHandler.ApplyTransfers)`), add:

```go
			r.Get("/sprints/{id}/next", planningHandler.GetNextSprint)
			r.Post("/sprints/{id}/planning/apply", planningHandler.Apply)
			r.Get("/sprints/{id}/planning/progress", planningHandler.GetProgress)
```

- [ ] **Step 4: Add `repository` import to `cmd/api/main.go`**

Verify the `repository` import is already present in `cmd/api/main.go` (it should be — `repository.NewSprintRepository` is already used). No action needed if already imported.

- [ ] **Step 5: Verify build**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: BUILD SUCCESS.

- [ ] **Step 6: Run all tests**

Run: `cd /home/emerson/code/myplanner/backend && go test ./... -count=1 -timeout 60s`
Expected: All tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/handler/planning.go cmd/api/main.go
git commit -m "feat(planning): add planning handler with next-sprint, apply, and progress endpoints"
```

---

### Task 5: Frontend — Jira Task Type Icons (Acompanhamento + Planning)

**Files:**
- Modify: `frontend/index.html` — add `getTaskTypeIcon()` function and update `renderCapacityCard()` task rows

**Interfaces:**
- Consumes: nothing new — modifies existing `renderCapacityCard()` function at line ~5074
- Produces: `getTaskTypeIcon(tipo)` — returns inline SVG string, used here and in Task 6 (Planning tab)

- [ ] **Step 1: Add `getTaskTypeIcon` function**

In `frontend/index.html`, add this function **before** the `renderCapacityCard` function (before line 5023). Find a suitable location — right after the `formatSprintDates` helper block (around line 3579) or right before `renderCapacityCard`:

```js
function getTaskTypeIcon(tipo) {
  var t = (tipo || '').toLowerCase();
  if (t === 'story' || t === 'história' || t === 'historia') {
    return '<svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M3 1h8l4 4v10H3V1z" fill="#63BA3C" opacity="0.2"/><path d="M5 4h4M5 7h6M5 10h3" stroke="#63BA3C" stroke-width="1.5" stroke-linecap="round"/></svg>';
  }
  if (t === 'bug') {
    return '<svg width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="7" fill="#E5493A"/><circle cx="8" cy="8" r="3" fill="white"/></svg>';
  }
  if (t === 'task' || t === 'tarefa') {
    return '<svg width="16" height="16" viewBox="0 0 16 16"><rect x="1" y="1" width="14" height="14" rx="2" fill="#4BADE8"/><path d="M4 8l3 3 5-6" stroke="white" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>';
  }
  if (t === 'sub-task' || t === 'sub-tarefa' || t === 'subtask') {
    return '<svg width="16" height="16" viewBox="0 0 16 16"><rect x="2" y="2" width="12" height="12" rx="2" fill="#4BADE8" opacity="0.7"/><path d="M5 8l2 2 4-4" stroke="white" stroke-width="1.5" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>';
  }
  if (t === 'epic' || t === 'épico' || t === 'epico') {
    return '<svg width="16" height="16" viewBox="0 0 16 16"><path d="M9 1L4 9h4l-1 6 5-8H8l1-6z" fill="#904EE2"/></svg>';
  }
  if (t === 'improvement' || t === 'melhoria') {
    return '<svg width="16" height="16" viewBox="0 0 16 16"><path d="M8 2l5 5H9v7H7V7H3l5-5z" fill="#63BA3C"/></svg>';
  }
  if (t.includes('incidente') || t.includes('incident')) {
    return '<svg width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="7" fill="#E5493A"/><path d="M8 4v5M8 11v1" stroke="white" stroke-width="2" stroke-linecap="round"/></svg>';
  }
  return '<svg width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="7" fill="#6B778C" opacity="0.3"/><circle cx="8" cy="8" r="3" fill="#6B778C"/></svg>';
}
```

- [ ] **Step 2: Update `renderCapacityCard` task rows to include icons**

In `renderCapacityCard` (line ~5074-5082), find this line:

```js
        html += '<div class="capacity-tarefa-row">';
```

And the block through line 5082. Replace the task row rendering within the `g.tarefas.forEach(t => {` block:

Change from:
```js
      g.tarefas.forEach(t => {
        const statusClass = (t.status || '').toLowerCase().includes('done') || (t.status || '').toLowerCase().includes('conclu') ? 'done' : (t.status || '').toLowerCase().includes('progress') || (t.status || '').toLowerCase().includes('andamento') ? 'inprogress' : 'todo';
        html += '<div class="capacity-tarefa-row">';
        html += '<span class="capacity-tarefa-ticket">' + jiraTicketLink(t.numero_ticket) + '</span>';
        html += '<span class="capacity-tarefa-resumo" title="' + esc(t.resumo) + '">' + esc(t.resumo) + '</span>';
        html += '<span class="capacity-tarefa-tipo">' + esc(t.tipo) + '</span>';
        html += '<span class="capacity-tarefa-status ' + statusClass + '">' + esc(t.status) + '</span>';
        if (t.horas > 0) html += '<span class="capacity-tarefa-horas">' + t.horas.toFixed(1) + 'h</span>';
        html += '</div>';
      });
```

Change to:
```js
      g.tarefas.forEach(t => {
        const statusClass = (t.status || '').toLowerCase().includes('done') || (t.status || '').toLowerCase().includes('conclu') ? 'done' : (t.status || '').toLowerCase().includes('progress') || (t.status || '').toLowerCase().includes('andamento') ? 'inprogress' : 'todo';
        html += '<div class="capacity-tarefa-row">';
        html += '<span class="capacity-tarefa-icon">' + getTaskTypeIcon(t.tipo) + '</span>';
        html += '<span class="capacity-tarefa-ticket">' + jiraTicketLink(t.numero_ticket) + '</span>';
        html += '<span class="capacity-tarefa-resumo" title="' + esc(t.resumo) + '">' + esc(t.resumo) + '</span>';
        html += '<span class="capacity-tarefa-tipo">' + esc(t.tipo) + '</span>';
        html += '<span class="capacity-tarefa-status ' + statusClass + '">' + esc(t.status) + '</span>';
        if (t.horas > 0) html += '<span class="capacity-tarefa-horas">' + t.horas.toFixed(1) + 'h</span>';
        html += '</div>';
      });
```

- [ ] **Step 3: Add CSS for the icon span**

Find the `.capacity-tarefa-row` CSS block in the `<style>` section (search for `.capacity-tarefa-row`). Add this rule near it:

```css
.capacity-tarefa-icon { display:inline-flex; align-items:center; flex-shrink:0; width:16px; height:16px; margin-right:4px; }
.capacity-tarefa-icon svg { display:block; }
```

- [ ] **Step 4: Manual test**

Start the dev server. Open a sprint in Acompanhamento tab, expand a member's task list. Verify:
- Each task row now shows an SVG icon to the left of the ticket number
- Story = green bookmark, Bug = red circle, Task = blue checkbox, Epic = purple lightning
- Layout not broken — icon inline with the rest of the row

- [ ] **Step 5: Commit**

```bash
git add frontend/index.html
git commit -m "feat(sprints): add Jira task type SVG icons to capacity card task rows"
```

---

### Task 6: Frontend — Planning Tab (Full Implementation)

**Files:**
- Modify: `frontend/index.html` — add Planning tab rendering, SortableJS library (inline), planningState engine, modals, stats, bulk actions, Finalizar flow

**Interfaces:**
- Consumes:
  - `getTaskTypeIcon(tipo)` (from Task 5)
  - `GET /sprints/{id}/next?equipe_id=xxx` (from Task 4)
  - `GET /sprints/{id}/capacity?equipe=xxx` (existing)
  - `POST /sprints/{id}/planning/apply` (from Task 4)
  - `GET /sprints/{id}/planning/progress?job_id=xxx` (from Task 4)
  - `api()` helper (existing, line 2182)
  - `esc()`, `jiraTicketLink()`, `formatSprintDates()` (existing helpers)
- Produces: Complete Planning tab UI with all interactions

This is the largest task. It modifies `frontend/index.html` in several places:

1. Add SortableJS library inline (minified)
2. Modify `openSprintCapacity` to add Planning tab
3. Modify `showSprintTab` to handle 'planning' tab
4. Add all Planning JS functions
5. Add CSS for Planning tab components

- [ ] **Step 1: Add SortableJS library inline**

Download SortableJS minified and add it as an inline `<script>` in the `<head>` section of `frontend/index.html`. Find the closing `</style>` tag and add right after it (before `</head>`):

```html
<script>
/* SortableJS v1.15.3 - MIT License - https://sortablejs.github.io/Sortable/ */
/* PASTE MINIFIED SORTABLE.JS CONTENT HERE */
</script>
```

To get the content, run:
```bash
curl -sL https://cdn.jsdelivr.net/npm/sortablejs@1.15.3/Sortable.min.js > /tmp/sortable.min.js
```

Then inline the content between `<script>` tags.

- [ ] **Step 2: Add Planning tab CSS**

Add these styles in the `<style>` section, near the existing `.sprint-tab` and `.capacity-*` styles:

```css
.sprint-tab.planning-highlight { background:var(--accent); color:white; font-weight:600; animation: pulse-planning 2s ease-in-out infinite; }
@keyframes pulse-planning { 0%,100% { box-shadow: 0 0 0 0 rgba(var(--accent-rgb,99,102,241), 0.4); } 50% { box-shadow: 0 0 0 8px rgba(var(--accent-rgb,99,102,241), 0); } }

.planning-header-stats { display:flex; gap:8px; flex-wrap:wrap; margin-bottom:16px; padding:12px 16px; background:var(--surface-secondary); border-radius:10px; }
.planning-stat { padding:8px 14px; border-radius:8px; text-align:center; min-width:100px; }
.planning-stat-label { font-size:11px; font-weight:500; opacity:0.7; text-transform:uppercase; letter-spacing:0.5px; }
.planning-stat-value { font-size:20px; font-weight:700; margin-top:2px; }
.planning-stat.tasks { background:rgba(59,130,246,0.1); color:var(--blue,#3B82F6); }
.planning-stat.hours { background:rgba(16,185,129,0.1); color:var(--green,#10B981); }

.planning-bulk-bar { display:flex; align-items:center; gap:8px; padding:10px 16px; background:var(--surface-secondary); border-radius:8px; margin-bottom:12px; border:1px solid var(--accent); }
.planning-bulk-bar .btn-bulk { padding:6px 14px; border-radius:6px; border:1px solid var(--border); background:var(--surface); cursor:pointer; font-size:13px; font-weight:500; }
.planning-bulk-bar .btn-bulk:hover { background:var(--accent); color:white; border-color:var(--accent); }
.planning-bulk-count { font-size:13px; font-weight:600; color:var(--accent); }

.planning-card { border:1px solid var(--border); border-radius:10px; padding:14px; margin-bottom:12px; background:var(--surface); }
.planning-card.unassigned { border-style:dashed; border-color:var(--text-muted); background:var(--surface-secondary); }
.planning-card-header { display:flex; align-items:center; gap:10px; margin-bottom:10px; }
.planning-card-name { font-weight:600; font-size:14px; flex:1; }
.planning-card-pct { font-weight:700; font-size:14px; }
.planning-card-pct.green { color:var(--green,#10B981); }
.planning-card-pct.amber { color:var(--amber,#F59E0B); }
.planning-card-pct.red { color:var(--red,#EF4444); }
.planning-card-detail { font-size:12px; color:var(--text-muted); margin-bottom:8px; }

.planning-task-list { min-height:40px; }
.planning-task-row { display:flex; align-items:center; gap:6px; padding:6px 8px; border-radius:6px; margin-bottom:4px; background:var(--surface); border:1px solid var(--border); cursor:grab; font-size:13px; transition: background 0.15s; }
.planning-task-row:hover { background:var(--surface-secondary); }
.planning-task-row.sortable-ghost { opacity:0.4; }
.planning-task-row.sortable-chosen { background:var(--accent-subtle,rgba(99,102,241,0.1)); }
.planning-task-row .task-checkbox { width:16px; height:16px; cursor:pointer; flex-shrink:0; }
.planning-task-row .task-icon { flex-shrink:0; width:16px; height:16px; display:inline-flex; align-items:center; }
.planning-task-row .task-key { font-weight:500; color:var(--accent); white-space:nowrap; }
.planning-task-row .task-resumo { flex:1; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.planning-task-row .task-horas { font-weight:500; white-space:nowrap; min-width:40px; text-align:right; }

.planning-btn-finalizar { padding:10px 24px; background:var(--accent); color:white; border:none; border-radius:8px; font-size:14px; font-weight:600; cursor:pointer; }
.planning-btn-finalizar:hover { opacity:0.9; }
.planning-btn-finalizar:disabled { opacity:0.5; cursor:not-allowed; }

.planning-modal-overlay { position:fixed; top:0; left:0; width:100%; height:100%; background:rgba(0,0,0,0.5); z-index:1000; display:flex; align-items:center; justify-content:center; }
.planning-modal { background:var(--surface); border-radius:12px; padding:24px; max-width:520px; width:90%; max-height:80vh; overflow-y:auto; box-shadow:0 20px 60px rgba(0,0,0,0.3); }
.planning-modal h3 { margin:0 0 16px; font-size:16px; }
.planning-modal-actions { display:flex; justify-content:flex-end; gap:8px; margin-top:20px; }
.planning-modal .btn-cancel { padding:8px 18px; border:1px solid var(--border); background:var(--surface); border-radius:6px; cursor:pointer; }
.planning-modal .btn-confirm { padding:8px 18px; background:var(--accent); color:white; border:none; border-radius:6px; cursor:pointer; font-weight:500; }

.planning-progress-bar { width:100%; height:8px; background:var(--surface-secondary); border-radius:4px; overflow:hidden; margin:12px 0; }
.planning-progress-fill { height:100%; background:var(--accent); transition: width 0.3s; border-radius:4px; }
.planning-op-list { max-height:300px; overflow-y:auto; }
.planning-op-item { display:flex; align-items:center; gap:8px; padding:4px 0; font-size:13px; }
.planning-op-icon { width:20px; text-align:center; }

.planning-compare-table { width:100%; border-collapse:collapse; margin:12px 0; font-size:13px; }
.planning-compare-table th, .planning-compare-table td { padding:8px 12px; border-bottom:1px solid var(--border); text-align:left; }
.planning-compare-table th { font-weight:500; color:var(--text-muted); }

.planning-member-select { list-style:none; padding:0; margin:0; }
.planning-member-select li { display:flex; align-items:center; gap:10px; padding:10px 12px; border-radius:8px; cursor:pointer; margin-bottom:4px; }
.planning-member-select li:hover { background:var(--surface-secondary); }
.planning-member-select li.selected { background:var(--accent-subtle,rgba(99,102,241,0.1)); border:1px solid var(--accent); }
```

- [ ] **Step 3: Modify `openSprintCapacity` to add Planning tab**

In the `openSprintCapacity` function (line ~3618-3621), find where the tabs are rendered:

```js
    html += '<div class="sprint-tabs">';
    html += '<div class="sprint-tab active" onclick="showSprintTab(\'acompanhamento\')">Acompanhamento</div>';
    html += '<div class="sprint-tab" onclick="showSprintTab(\'review\')">Review</div>';
    html += '</div>';
```

Replace with:

```js
    html += '<div class="sprint-tabs">';
    html += '<div class="sprint-tab active" onclick="showSprintTab(\'acompanhamento\')">Acompanhamento</div>';
    html += '<div class="sprint-tab" onclick="showSprintTab(\'review\')">Review</div>';
    var sprintEstado = data.sprint.estado || 'future';
    if (sprintEstado === 'active' && data.sprint.data_fim) {
      var daysLeft = (new Date(data.sprint.data_fim) - new Date()) / (1000*60*60*24);
      if (daysLeft <= 3) {
        html += '<div class="sprint-tab planning-highlight" onclick="showSprintTab(\'planning\')">Planning</div>';
      }
    }
    html += '</div>';
```

Then, after the review tab div closing (find `<div id="sprint-tab-review"` and its creation), add the planning tab container. After the line that creates the review div (around line 3622 area), look for where `sprint-tab-review` div is created and after that block, add:

```js
    html += '<div id="sprint-tab-planning" style="display:none"></div>';
```

- [ ] **Step 4: Modify `showSprintTab` function**

Replace the entire `showSprintTab` function (line 3824-3836):

```js
function showSprintTab(tab) {
  var tabs = document.querySelectorAll('.sprint-tab');
  tabs.forEach(function(t) { t.classList.remove('active'); });
  event.target.classList.add('active');

  document.getElementById('sprint-tab-acompanhamento').style.display = tab === 'acompanhamento' ? '' : 'none';
  var reviewEl = document.getElementById('sprint-tab-review');
  reviewEl.style.display = tab === 'review' ? '' : 'none';

  var planningEl = document.getElementById('sprint-tab-planning');
  if (planningEl) {
    planningEl.style.display = tab === 'planning' ? '' : 'none';
    if (tab === 'planning' && !planningEl.dataset.loaded) {
      loadPlanningTab();
    }
  }

  if (tab === 'review' && !reviewEl.dataset.loaded) {
    loadSprintReview();
  }
}
```

- [ ] **Step 5: Add the Planning tab JS engine**

Add the following block of JS at the end of the `<script>` section in `frontend/index.html` (before the closing `</script>` tag), or after the `showSprintTab` function. This is the complete Planning tab engine:

```js
// === PLANNING TAB ===

window.planningState = null;

async function loadPlanningTab() {
  var container = document.getElementById('sprint-tab-planning');
  container.dataset.loaded = '1';
  container.innerHTML = '<div class="loading"><div class="spinner"></div></div>';

  var equipeID = document.getElementById('sprints-equipe').value;
  if (!equipeID) {
    container.innerHTML = '<div style="padding:20px;color:#888">Selecione uma equipe para usar o Planning.</div>';
    return;
  }

  try {
    var currentSprintID = window._currentSprintID;
    var nextData = await api('/sprints/' + currentSprintID + '/next?equipe_id=' + equipeID);
    var capacityData = await api('/sprints/' + nextData.sprint.id + '/capacity?equipe=' + equipeID);

    initPlanningState(nextData, capacityData);
    renderPlanningTab();
  } catch (e) {
    container.innerHTML = '<div style="padding:20px;color:var(--red)">Erro ao carregar planning: ' + esc(e.message || String(e)) + ' <button onclick="loadPlanningTab()" class="btn-sm" style="margin-left:8px">Tentar novamente</button></div>';
  }
}

function initPlanningState(nextData, capacityData) {
  var state = {
    sprint: {
      id: nextData.sprint.id,
      nome: nextData.sprint.nome,
      dataInicio: nextData.sprint.data_inicio,
      dataFim: nextData.sprint.data_fim,
      jiraId: nextData.sprint.jira_id,
      fonteDadosId: capacityData.fonte_dados_id
    },
    diasUteis: capacityData.dias_uteis,
    feriados: capacityData.feriados || [],
    tasks: {},
    members: {},
    unassigned: [],
    movedBacklog: [],
    movedNextSprint: [],
    selected: new Set()
  };

  // index members from capacity data
  (capacityData.membros || []).forEach(function(m) {
    if (!m.da_equipe) return;
    state.members[m.membro_id] = {
      id: m.membro_id,
      nome: m.nome,
      avatarUrl: m.avatar_url,
      horasDisponiveis: m.horas_disponiveis,
      ausencias: m.ausencias || [],
      tarefaIds: []
    };
  });

  // index tasks
  (nextData.tarefas || []).forEach(function(t) {
    var estSec = t.estimativa_tempo || 0;
    state.tasks[t.id] = {
      id: t.id,
      key: t.numero_ticket,
      resumo: t.resumo,
      tipo: t.tipo,
      status: t.status,
      prioridade: t.prioridade,
      responsavelId: t.responsavel_id || null,
      estimativaTempo: estSec,
      tipoDemanda: t.tipo_demanda || null,
      projetoChave: t.projeto_chave,
      originalResponsavelId: t.responsavel_id || null,
      originalEstimativa: estSec,
      originalTipoDemanda: t.tipo_demanda || null,
      originalSprintId: state.sprint.id
    };

    if (t.responsavel_id && state.members[t.responsavel_id]) {
      state.members[t.responsavel_id].tarefaIds.push(t.id);
    } else if (t.responsavel_id && !state.members[t.responsavel_id]) {
      // member not in equipe — create a placeholder
      state.members[t.responsavel_id] = {
        id: t.responsavel_id,
        nome: '(Externo)',
        avatarUrl: null,
        horasDisponiveis: 0,
        ausencias: [],
        tarefaIds: [t.id]
      };
    } else {
      state.unassigned.push(t.id);
    }
  });

  window.planningState = state;
}

var PLANNING_STATUS_EXECUTADO = {'Code Review':1,'Teste':1,'Validação do Solicitante':1,'Deploy':1,'Concluído':1};
var PLANNING_STATUS_AMBOS = {'Teste':1,'Validação do Solicitante':1,'Deploy':1};
var PLANNING_STATUS_PENDENTE = {'Backlog':1,'Desenvolvimento':1,'Em Desenvolvimento':1,'A Fazer':1};

function recalcMemberCapacity(memberId) {
  var s = window.planningState;
  var member = s.members[memberId];
  if (!member) return { horasAlocadas:0, horasExecutadas:0, percentual:0, overcapacity:false };

  var horasAlocadas = 0, horasExecutadas = 0, horasAmbos = 0;
  member.tarefaIds.forEach(function(tid) {
    var t = s.tasks[tid];
    if (!t || t.status === 'Cancelado' || t.status === 'Rejeitada') return;
    if (s.movedBacklog.indexOf(tid) >= 0 || s.movedNextSprint.indexOf(tid) >= 0) return;
    var horas = (t.estimativaTempo || 0) / 3600;
    if (PLANNING_STATUS_AMBOS[t.status]) {
      horasAlocadas += horas;
      horasExecutadas += horas;
      horasAmbos += horas;
    } else if (PLANNING_STATUS_EXECUTADO[t.status]) {
      horasExecutadas += horas;
    } else {
      horasAlocadas += horas;
    }
  });

  var horasAlocPura = horasAlocadas - horasAmbos;
  var pct = member.horasDisponiveis > 0 ? Math.round((horasAlocPura / member.horasDisponiveis) * 1000) / 10 : (horasAlocPura > 0 ? 999.9 : 0);
  return { horasAlocadas: Math.round(horasAlocPura * 10) / 10, horasExecutadas: Math.round(horasExecutadas * 10) / 10, percentual: pct, overcapacity: pct > 100 };
}

function calcPlanningStats() {
  var s = window.planningState;
  var totalTarefas = 0, alocadas = 0, naoAlocadas = 0;
  var horasDisponiveis = 0, horasAlocadas = 0, horasPendentes = 0;

  Object.keys(s.members).forEach(function(mid) {
    horasDisponiveis += s.members[mid].horasDisponiveis;
  });

  Object.keys(s.tasks).forEach(function(tid) {
    var t = s.tasks[tid];
    if (s.movedBacklog.indexOf(tid) >= 0 || s.movedNextSprint.indexOf(tid) >= 0) return;
    if (t.status === 'Cancelado' || t.status === 'Rejeitada') return;
    totalTarefas++;
    if (t.responsavelId) { alocadas++; } else { naoAlocadas++; }
    var h = (t.estimativaTempo || 0) / 3600;
    if (!t.responsavelId) horasPendentes += h;
  });

  Object.keys(s.members).forEach(function(mid) {
    var cap = recalcMemberCapacity(mid);
    horasAlocadas += cap.horasAlocadas;
  });

  return { totalTarefas: totalTarefas, alocadas: alocadas, naoAlocadas: naoAlocadas, horasDisponiveis: Math.round(horasDisponiveis*10)/10, horasAlocadas: Math.round(horasAlocadas*10)/10, horasPendentes: Math.round(horasPendentes*10)/10 };
}

function renderPlanningTab() {
  var container = document.getElementById('sprint-tab-planning');
  var s = window.planningState;
  var stats = calcPlanningStats();

  var html = '<div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:16px">';
  html += '<div><div style="font-size:16px;font-weight:700">Planning: ' + esc(s.sprint.nome) + '</div>';
  html += '<div style="font-size:13px;color:var(--text-muted)">' + formatSprintDates(s.sprint.dataInicio, s.sprint.dataFim) + ' • ' + s.diasUteis + ' dias úteis</div></div>';
  html += '<button class="planning-btn-finalizar" onclick="finalizarPlanning()" id="btn-finalizar-planning">Finalizar Planning</button>';
  html += '</div>';

  // stats bar
  html += '<div class="planning-header-stats">';
  html += '<div class="planning-stat tasks"><div class="planning-stat-label">Total Tarefas</div><div class="planning-stat-value" id="ps-total">' + stats.totalTarefas + '</div></div>';
  html += '<div class="planning-stat tasks"><div class="planning-stat-label">Alocadas</div><div class="planning-stat-value" id="ps-alocadas">' + stats.alocadas + '</div></div>';
  html += '<div class="planning-stat tasks"><div class="planning-stat-label">Não Alocadas</div><div class="planning-stat-value" id="ps-nao-alocadas">' + stats.naoAlocadas + '</div></div>';
  html += '<div class="planning-stat hours"><div class="planning-stat-label">Horas Disponíveis</div><div class="planning-stat-value" id="ps-horas-disp">' + stats.horasDisponiveis.toFixed(1) + 'h</div></div>';
  html += '<div class="planning-stat hours"><div class="planning-stat-label">Horas Pendentes</div><div class="planning-stat-value" id="ps-horas-pend">' + stats.horasPendentes.toFixed(1) + 'h</div></div>';
  html += '<div class="planning-stat hours"><div class="planning-stat-label">Horas Alocadas</div><div class="planning-stat-value" id="ps-horas-aloc">' + stats.horasAlocadas.toFixed(1) + 'h</div></div>';
  html += '</div>';

  // bulk bar
  html += '<div class="planning-bulk-bar" id="planning-bulk-bar" style="display:none">';
  html += '<span class="planning-bulk-count" id="planning-bulk-count">0 selecionadas</span>';
  html += '<button class="btn-bulk" onclick="bulkAlocar()">Alocar</button>';
  html += '<button class="btn-bulk" onclick="bulkMoverProximaSprint()">Mover próxima Sprint</button>';
  html += '<button class="btn-bulk" onclick="bulkVoltarBacklog()">Voltar para Backlog</button>';
  html += '</div>';

  // unassigned card
  html += '<div class="planning-card unassigned">';
  html += '<div class="planning-card-header"><div class="planning-card-name">Não Alocadas (' + s.unassigned.length + ' tarefas)</div></div>';
  html += '<div class="planning-task-list" id="planning-unassigned" data-member-id="">';
  s.unassigned.forEach(function(tid) {
    html += renderPlanningTaskRow(tid);
  });
  html += '</div></div>';

  // member cards sorted by allocation desc
  var memberList = Object.keys(s.members).map(function(mid) {
    var cap = recalcMemberCapacity(mid);
    return { id: mid, member: s.members[mid], cap: cap };
  }).sort(function(a,b) { return b.cap.percentual - a.cap.percentual; });

  memberList.forEach(function(item) {
    var m = item.member;
    var cap = item.cap;
    var pctClass = cap.percentual > 100 ? 'red' : cap.percentual > 80 ? 'amber' : 'green';
    var initials = m.nome.split(' ').map(function(w){return w[0]}).join('').substring(0,2).toUpperCase();
    var avatar = m.avatarUrl ? '<img src="' + esc(m.avatarUrl) + '" style="width:32px;height:32px;border-radius:50%" alt="">' : '<div style="width:32px;height:32px;border-radius:50%;background:var(--accent);color:white;display:flex;align-items:center;justify-content:center;font-size:12px;font-weight:600">' + initials + '</div>';
    var barW = Math.min(cap.percentual, 150);

    html += '<div class="planning-card" data-member-id="' + m.id + '">';
    html += '<div class="planning-card-header">' + avatar;
    html += '<div class="planning-card-name">' + esc(m.nome) + ' <span style="font-weight:400;font-size:12px;color:var(--text-muted)">(' + m.tarefaIds.length + ' tarefas)</span></div>';
    html += '<div class="planning-card-pct ' + pctClass + '">' + cap.percentual.toFixed(1) + '%</div>';
    if (cap.overcapacity) html += '<span class="capacity-badge overcapacity" style="margin-left:6px">Overcapacity</span>';
    html += '</div>';
    html += '<div class="capacity-bar"><div class="capacity-bar-fill ' + pctClass + '" style="width:' + barW + '%"></div></div>';
    html += '<div class="planning-card-detail">' + cap.horasAlocadas.toFixed(1) + 'h alocadas / ' + m.horasDisponiveis.toFixed(1) + 'h disponíveis</div>';
    html += '<div class="planning-task-list" id="planning-member-' + m.id + '" data-member-id="' + m.id + '">';
    m.tarefaIds.forEach(function(tid) {
      if (window.planningState.movedBacklog.indexOf(tid) < 0 && window.planningState.movedNextSprint.indexOf(tid) < 0) {
        html += renderPlanningTaskRow(tid);
      }
    });
    html += '</div></div>';
  });

  container.innerHTML = html;
  initSortables();
}

function renderPlanningTaskRow(tid) {
  var t = window.planningState.tasks[tid];
  if (!t) return '';
  var horas = t.estimativaTempo ? (t.estimativaTempo / 3600).toFixed(1) + 'h' : '—';
  var checked = window.planningState.selected.has(tid) ? ' checked' : '';
  return '<div class="planning-task-row" data-task-id="' + tid + '">' +
    '<input type="checkbox" class="task-checkbox" onclick="event.stopPropagation();togglePlanningSelect(\'' + tid + '\')"' + checked + '>' +
    '<span class="task-icon">' + getTaskTypeIcon(t.tipo) + '</span>' +
    '<span class="task-key">' + esc(t.key) + '</span>' +
    '<span class="task-resumo" title="' + esc(t.resumo) + '">' + esc(t.resumo) + '</span>' +
    '<span class="task-horas">' + horas + '</span>' +
    '</div>';
}

function initSortables() {
  var containers = document.querySelectorAll('.planning-task-list');
  containers.forEach(function(el) {
    new Sortable(el, {
      group: 'planning',
      animation: 150,
      ghostClass: 'sortable-ghost',
      chosenClass: 'sortable-chosen',
      handle: '.planning-task-row',
      onEnd: function(evt) {
        handlePlanningDrop(evt);
      }
    });
  });
}

function handlePlanningDrop(evt) {
  var taskEl = evt.item;
  var taskId = taskEl.dataset.taskId;
  var fromContainer = evt.from;
  var toContainer = evt.to;
  var fromMemberId = fromContainer.dataset.memberId || null;
  var toMemberId = toContainer.dataset.memberId || null;

  if (fromMemberId === toMemberId) return;

  var task = window.planningState.tasks[taskId];
  if (!task) return;

  // check required fields when assigning to a member
  if (toMemberId && (!task.tipoDemanda || !task.estimativaTempo)) {
    showRequiredFieldsModal([taskId], function() {
      checkOvercapacityAndApply(taskId, fromMemberId, toMemberId);
    }, function() {
      revertDrop(evt);
    });
    return;
  }

  if (toMemberId) {
    checkOvercapacityAndApply(taskId, fromMemberId, toMemberId);
  } else {
    applyTaskMove(taskId, fromMemberId, toMemberId);
  }
}

function checkOvercapacityAndApply(taskId, fromMemberId, toMemberId) {
  var s = window.planningState;
  var task = s.tasks[taskId];
  var member = s.members[toMemberId];
  if (!member) { applyTaskMove(taskId, fromMemberId, toMemberId); return; }

  // simulate: temporarily add task
  var capBefore = recalcMemberCapacity(toMemberId);
  member.tarefaIds.push(taskId);
  task.responsavelId = toMemberId;
  var capAfter = recalcMemberCapacity(toMemberId);

  // revert simulation
  member.tarefaIds.pop();
  task.responsavelId = fromMemberId;

  if (capAfter.overcapacity) {
    showOvercapacityModal(member, capBefore, capAfter, function() {
      applyTaskMove(taskId, fromMemberId, toMemberId);
    }, function() {
      // revert: need to move DOM element back
      renderPlanningTab();
    });
  } else {
    applyTaskMove(taskId, fromMemberId, toMemberId);
  }
}

function applyTaskMove(taskId, fromMemberId, toMemberId) {
  var s = window.planningState;
  var task = s.tasks[taskId];

  // remove from source
  if (fromMemberId && s.members[fromMemberId]) {
    var idx = s.members[fromMemberId].tarefaIds.indexOf(taskId);
    if (idx >= 0) s.members[fromMemberId].tarefaIds.splice(idx, 1);
  } else {
    var idx = s.unassigned.indexOf(taskId);
    if (idx >= 0) s.unassigned.splice(idx, 1);
  }

  // add to destination
  if (toMemberId && s.members[toMemberId]) {
    s.members[toMemberId].tarefaIds.push(taskId);
    task.responsavelId = toMemberId;
  } else {
    s.unassigned.push(taskId);
    task.responsavelId = null;
  }

  updatePlanningStats();
  renderPlanningTab();
}

function revertDrop(evt) {
  renderPlanningTab();
}

function updatePlanningStats() {
  var stats = calcPlanningStats();
  var el;
  el = document.getElementById('ps-total'); if (el) el.textContent = stats.totalTarefas;
  el = document.getElementById('ps-alocadas'); if (el) el.textContent = stats.alocadas;
  el = document.getElementById('ps-nao-alocadas'); if (el) el.textContent = stats.naoAlocadas;
  el = document.getElementById('ps-horas-disp'); if (el) el.textContent = stats.horasDisponiveis.toFixed(1) + 'h';
  el = document.getElementById('ps-horas-pend'); if (el) el.textContent = stats.horasPendentes.toFixed(1) + 'h';
  el = document.getElementById('ps-horas-aloc'); if (el) el.textContent = stats.horasAlocadas.toFixed(1) + 'h';
}

function togglePlanningSelect(taskId) {
  var s = window.planningState;
  if (s.selected.has(taskId)) { s.selected.delete(taskId); } else { s.selected.add(taskId); }
  var bar = document.getElementById('planning-bulk-bar');
  var count = document.getElementById('planning-bulk-count');
  if (s.selected.size > 0) {
    bar.style.display = '';
    count.textContent = s.selected.size + ' selecionada' + (s.selected.size > 1 ? 's' : '');
  } else {
    bar.style.display = 'none';
  }
}

// --- Modals ---

function showOvercapacityModal(member, before, after, onConfirm, onCancel) {
  var html = '<div class="planning-modal-overlay" id="planning-modal">';
  html += '<div class="planning-modal">';
  html += '<h3>⚠️ Alerta de Overcapacity</h3>';
  html += '<p><b>' + esc(member.nome) + '</b> ficará <b>' + after.percentual.toFixed(1) + '%</b> overcapacity. Continuar?</p>';
  html += '<table class="planning-compare-table"><tr><th></th><th>Antes</th><th>Depois</th></tr>';
  html += '<tr><td>Horas Alocadas</td><td>' + before.horasAlocadas.toFixed(1) + 'h</td><td>' + after.horasAlocadas.toFixed(1) + 'h</td></tr>';
  html += '<tr><td>Horas Disponíveis</td><td>' + member.horasDisponiveis.toFixed(1) + 'h</td><td>' + member.horasDisponiveis.toFixed(1) + 'h</td></tr>';
  html += '<tr><td>Alocação %</td><td>' + before.percentual.toFixed(1) + '%</td><td style="color:var(--red);font-weight:600">' + after.percentual.toFixed(1) + '%</td></tr>';
  html += '</table>';
  html += '<div class="planning-modal-actions">';
  html += '<button class="btn-cancel" onclick="closePlanningModal();(' + onCancel.toString() + ')()">Cancelar</button>';
  html += '<button class="btn-confirm" onclick="closePlanningModal();(' + onConfirm.toString() + ')()">Continuar</button>';
  html += '</div></div></div>';
  document.body.insertAdjacentHTML('beforeend', html);
}

function showRequiredFieldsModal(taskIds, onConfirm, onCancel) {
  var s = window.planningState;
  var html = '<div class="planning-modal-overlay" id="planning-modal">';
  html += '<div class="planning-modal">';
  html += '<h3>Preencher campos obrigatórios</h3>';

  taskIds.forEach(function(tid, i) {
    var t = s.tasks[tid];
    html += '<div style="margin-bottom:16px;padding:12px;background:var(--surface-secondary);border-radius:8px">';
    html += '<div style="font-weight:600;margin-bottom:8px">' + esc(t.key) + ': ' + esc(t.resumo) + '</div>';
    if (!t.tipoDemanda) {
      html += '<div style="margin-bottom:8px"><label style="font-size:12px;font-weight:500">Tipo de Demanda *</label>';
      html += '<select id="pf-tipo-' + i + '" data-task-id="' + tid + '" style="width:100%;padding:6px 8px;border:1px solid var(--border);border-radius:6px;margin-top:4px">';
      html += '<option value="">Selecione...</option><option value="Compromisso">Compromisso</option><option value="Meta">Meta</option><option value="Iniciativa">Iniciativa</option>';
      html += '</select></div>';
    }
    if (!t.estimativaTempo) {
      html += '<div><label style="font-size:12px;font-weight:500">Estimativa (horas) *</label>';
      html += '<input type="number" id="pf-est-' + i + '" data-task-id="' + tid + '" min="0.5" step="0.5" style="width:100%;padding:6px 8px;border:1px solid var(--border);border-radius:6px;margin-top:4px" placeholder="Ex: 4"></div>';
    }
    html += '</div>';
  });

  html += '<div class="planning-modal-actions">';
  html += '<button class="btn-cancel" id="pf-cancel">Cancelar</button>';
  html += '<button class="btn-confirm" id="pf-confirm">Confirmar</button>';
  html += '</div></div></div>';

  document.body.insertAdjacentHTML('beforeend', html);

  document.getElementById('pf-cancel').onclick = function() {
    closePlanningModal();
    onCancel();
  };
  document.getElementById('pf-confirm').onclick = function() {
    var valid = true;
    taskIds.forEach(function(tid, i) {
      var t = s.tasks[tid];
      var tipoEl = document.getElementById('pf-tipo-' + i);
      var estEl = document.getElementById('pf-est-' + i);
      if (tipoEl && !tipoEl.value) { valid = false; tipoEl.style.borderColor = 'var(--red)'; }
      if (estEl && (!estEl.value || parseFloat(estEl.value) <= 0)) { valid = false; estEl.style.borderColor = 'var(--red)'; }
      if (valid) {
        if (tipoEl && tipoEl.value) t.tipoDemanda = tipoEl.value;
        if (estEl && parseFloat(estEl.value) > 0) t.estimativaTempo = Math.round(parseFloat(estEl.value) * 3600);
      }
    });
    if (!valid) return;
    closePlanningModal();
    onConfirm();
  };
}

function closePlanningModal() {
  var el = document.getElementById('planning-modal');
  if (el) el.remove();
}

// --- Bulk Actions ---

function bulkAlocar() {
  var s = window.planningState;
  var taskIds = Array.from(s.selected);
  if (taskIds.length === 0) return;

  var memberList = Object.keys(s.members).map(function(mid) {
    var cap = recalcMemberCapacity(mid);
    return { id: mid, nome: s.members[mid].nome, avatarUrl: s.members[mid].avatarUrl, cap: cap, horasDisp: s.members[mid].horasDisponiveis };
  }).sort(function(a,b) { return a.cap.percentual - b.cap.percentual; });

  var html = '<div class="planning-modal-overlay" id="planning-modal">';
  html += '<div class="planning-modal">';
  html += '<h3>Alocar ' + taskIds.length + ' tarefa' + (taskIds.length > 1 ? 's' : '') + '</h3>';
  html += '<p style="font-size:13px;color:var(--text-muted)">Selecione o responsável:</p>';
  html += '<ul class="planning-member-select" id="bulk-member-list">';
  memberList.forEach(function(m) {
    var pctClass = m.cap.percentual > 100 ? 'red' : m.cap.percentual > 80 ? 'amber' : 'green';
    var initials = m.nome.split(' ').map(function(w){return w[0]}).join('').substring(0,2).toUpperCase();
    html += '<li data-member-id="' + m.id + '" onclick="selectBulkMember(this)">';
    html += '<div style="width:28px;height:28px;border-radius:50%;background:var(--accent);color:white;display:flex;align-items:center;justify-content:center;font-size:11px;font-weight:600">' + initials + '</div>';
    html += '<span style="flex:1;font-weight:500">' + esc(m.nome) + '</span>';
    html += '<span class="planning-card-pct ' + pctClass + '">' + m.cap.percentual.toFixed(1) + '% (' + m.cap.horasAlocadas.toFixed(1) + 'h/' + m.horasDisp.toFixed(1) + 'h)</span>';
    html += '</li>';
  });
  html += '</ul>';
  html += '<div class="planning-modal-actions">';
  html += '<button class="btn-cancel" onclick="closePlanningModal()">Cancelar</button>';
  html += '<button class="btn-confirm" id="bulk-alocar-btn" disabled onclick="confirmBulkAlocar()">Alocar</button>';
  html += '</div></div></div>';
  document.body.insertAdjacentHTML('beforeend', html);
}

var _bulkSelectedMember = null;
function selectBulkMember(el) {
  document.querySelectorAll('#bulk-member-list li').forEach(function(li) { li.classList.remove('selected'); });
  el.classList.add('selected');
  _bulkSelectedMember = el.dataset.memberId;
  document.getElementById('bulk-alocar-btn').disabled = false;
}

function confirmBulkAlocar() {
  if (!_bulkSelectedMember) return;
  closePlanningModal();
  var s = window.planningState;
  var taskIds = Array.from(s.selected);

  // check required fields
  var needFields = taskIds.filter(function(tid) {
    var t = s.tasks[tid];
    return !t.tipoDemanda || !t.estimativaTempo;
  });

  if (needFields.length > 0) {
    showRequiredFieldsModal(needFields, function() {
      applyBulkAlocar(taskIds, _bulkSelectedMember);
    }, function() {});
  } else {
    applyBulkAlocar(taskIds, _bulkSelectedMember);
  }
}

function applyBulkAlocar(taskIds, memberId) {
  taskIds.forEach(function(tid) {
    var t = window.planningState.tasks[tid];
    var fromMemberId = t.responsavelId;
    applyTaskMove(tid, fromMemberId, memberId);
  });
  window.planningState.selected.clear();
  renderPlanningTab();
}

function bulkMoverProximaSprint() {
  var s = window.planningState;
  var taskIds = Array.from(s.selected);
  if (taskIds.length === 0) return;

  var equipeID = document.getElementById('sprints-equipe').value;
  api('/sprints/' + s.sprint.id + '/next?equipe_id=' + equipeID).then(function(data) {
    var msg = 'Mover ' + taskIds.length + ' tarefa' + (taskIds.length > 1 ? 's' : '') + ' para ' + data.sprint.nome + '?';
    if (confirm(msg)) {
      taskIds.forEach(function(tid) {
        s.movedNextSprint.push(tid);
        var t = s.tasks[tid];
        if (t.responsavelId && s.members[t.responsavelId]) {
          var idx = s.members[t.responsavelId].tarefaIds.indexOf(tid);
          if (idx >= 0) s.members[t.responsavelId].tarefaIds.splice(idx, 1);
        } else {
          var idx = s.unassigned.indexOf(tid);
          if (idx >= 0) s.unassigned.splice(idx, 1);
        }
      });
      s.selected.clear();
      renderPlanningTab();
    }
  }).catch(function() {
    if (confirm('Não há próxima Sprint para o projeto. Deseja mover para o backlog?')) {
      bulkVoltarBacklog();
    }
  });
}

function bulkVoltarBacklog() {
  var s = window.planningState;
  var taskIds = Array.from(s.selected);
  if (taskIds.length === 0) return;

  if (!confirm('Remover ' + taskIds.length + ' tarefa' + (taskIds.length > 1 ? 's' : '') + ' da sprint (voltar para backlog)?')) return;

  taskIds.forEach(function(tid) {
    s.movedBacklog.push(tid);
    var t = s.tasks[tid];
    if (t.responsavelId && s.members[t.responsavelId]) {
      var idx = s.members[t.responsavelId].tarefaIds.indexOf(tid);
      if (idx >= 0) s.members[t.responsavelId].tarefaIds.splice(idx, 1);
    } else {
      var idx = s.unassigned.indexOf(tid);
      if (idx >= 0) s.unassigned.splice(idx, 1);
    }
  });
  s.selected.clear();
  renderPlanningTab();
}

// --- Finalizar ---

function buildChangeset() {
  var s = window.planningState;
  var changes = { reassigned:[], estimated:[], tipo_demanda:[], moved_next_sprint:[], moved_backlog:[] };

  Object.keys(s.tasks).forEach(function(tid) {
    var t = s.tasks[tid];
    if (s.movedBacklog.indexOf(tid) >= 0 || s.movedNextSprint.indexOf(tid) >= 0) return;
    if (t.responsavelId !== t.originalResponsavelId) {
      changes.reassigned.push({ tarefa_id: t.id, tarefa_key: t.key, novo_responsavel_id: t.responsavelId });
    }
    if (t.estimativaTempo !== t.originalEstimativa) {
      changes.estimated.push({ tarefa_id: t.id, tarefa_key: t.key, segundos: t.estimativaTempo });
    }
    if (t.tipoDemanda !== t.originalTipoDemanda) {
      changes.tipo_demanda.push({ tarefa_id: t.id, tarefa_key: t.key, valor: t.tipoDemanda });
    }
  });

  s.movedBacklog.forEach(function(tid) {
    var t = s.tasks[tid];
    changes.moved_backlog.push({ tarefa_id: t.id, tarefa_key: t.key });
  });

  s.movedNextSprint.forEach(function(tid) {
    var t = s.tasks[tid];
    // need next-next sprint jira_id — stored from the API call
    changes.moved_next_sprint.push({ tarefa_id: t.id, tarefa_key: t.key, destino_sprint_jira_id: window._planningNextNextSprintJiraId || 0 });
  });

  return changes;
}

async function finalizarPlanning() {
  var changes = buildChangeset();
  var totalOps = changes.reassigned.length + changes.estimated.length + changes.tipo_demanda.length + changes.moved_next_sprint.length + changes.moved_backlog.length;

  if (totalOps === 0) {
    alert('Nenhuma alteração para aplicar.');
    return;
  }

  // build ops list for display
  var opsList = [];
  changes.reassigned.forEach(function(c) { opsList.push({ key: c.tarefa_key, action: 'Responsável atualizado' }); });
  changes.estimated.forEach(function(c) { opsList.push({ key: c.tarefa_key, action: 'Estimativa atualizada' }); });
  changes.tipo_demanda.forEach(function(c) { opsList.push({ key: c.tarefa_key, action: 'Tipo de demanda atualizado' }); });
  changes.moved_next_sprint.forEach(function(c) { opsList.push({ key: c.tarefa_key, action: 'Movido para próxima sprint' }); });
  changes.moved_backlog.forEach(function(c) { opsList.push({ key: c.tarefa_key, action: 'Movido para backlog' }); });

  var html = '<div class="planning-modal-overlay" id="planning-modal">';
  html += '<div class="planning-modal" style="max-width:600px">';
  html += '<h3>Finalizando Planning — ' + esc(window.planningState.sprint.nome) + '</h3>';
  html += '<div class="planning-progress-bar"><div class="planning-progress-fill" id="fp-bar" style="width:0%"></div></div>';
  html += '<div style="font-size:13px;margin-bottom:12px" id="fp-counter">0 de ' + totalOps + ' operações concluídas</div>';
  html += '<div class="planning-op-list" id="fp-ops">';
  opsList.forEach(function(op, i) {
    html += '<div class="planning-op-item" id="fp-op-' + i + '"><span class="planning-op-icon">⬜</span><span><b>' + esc(op.key) + '</b> — ' + esc(op.action) + '</span></div>';
  });
  html += '</div>';
  html += '<div class="planning-modal-actions"><button class="btn-cancel" id="fp-close" onclick="closePlanningModal()">Cancelar</button></div>';
  html += '</div></div>';
  document.body.insertAdjacentHTML('beforeend', html);

  try {
    var resp = await api('/sprints/' + window._currentSprintID + '/planning/apply', {
      method: 'POST',
      body: JSON.stringify({ fonte_dados_id: window.planningState.sprint.fonteDadosId, changes: changes })
    });

    var jobId = resp.job_id;
    pollPlanningProgress(jobId, opsList.length);
  } catch (e) {
    document.getElementById('fp-counter').textContent = 'Erro: ' + (e.message || String(e));
    document.getElementById('fp-close').textContent = 'Fechar';
  }
}

function pollPlanningProgress(jobId, totalDisplay) {
  var interval = setInterval(async function() {
    try {
      var data = await api('/sprints/' + window._currentSprintID + '/planning/progress?job_id=' + jobId);
      var pct = data.total > 0 ? Math.round(data.completed / data.total * 100) : 0;
      var bar = document.getElementById('fp-bar');
      if (bar) bar.style.width = pct + '%';
      var counter = document.getElementById('fp-counter');
      if (counter) counter.textContent = data.completed + ' de ' + data.total + ' operações concluídas';

      data.operations.forEach(function(op, i) {
        var el = document.getElementById('fp-op-' + i);
        if (!el) return;
        var iconEl = el.querySelector('.planning-op-icon');
        if (op.status === 'done') iconEl.textContent = '✅';
        else if (op.status === 'running') iconEl.textContent = '🔄';
        else if (op.status === 'error') iconEl.textContent = '❌';
        else iconEl.textContent = '⬜';
      });

      if (data.finished) {
        clearInterval(interval);
        var closeBtn = document.getElementById('fp-close');
        if (closeBtn) {
          closeBtn.textContent = 'Fechar';
          closeBtn.onclick = function() { closePlanningModal(); loadPlanningTab(); };
        }
        if (data.errors && data.errors.length > 0) {
          counter.textContent = data.completed + ' de ' + data.total + ' concluídas (' + data.errors.length + ' erro' + (data.errors.length > 1 ? 's' : '') + ')';
        }
      }
    } catch (e) {
      clearInterval(interval);
      var counter = document.getElementById('fp-counter');
      if (counter) counter.textContent = 'Erro ao verificar progresso. Recarregue a página.';
    }
  }, 1000);
}
```

- [ ] **Step 6: Start dev server and manually test the complete Planning tab flow**

Run the backend: `cd /home/emerson/code/myplanner/backend && go run cmd/api/main.go`
Open the frontend in a browser.

Test checklist:
1. Open an active sprint that is within 3 days of ending — verify "Planning" tab appears with highlight styling
2. Open a sprint NOT within 3 days — verify no Planning tab
3. Click Planning tab — verify it loads next sprint data and shows cards
4. Verify stats bar shows correct numbers (Total, Alocadas, Não Alocadas, Horas)
5. Drag a task from one member to another — verify capacity recalculates
6. Drag a task to a member pushing them over 100% — verify overcapacity modal appears
7. Drag a task with missing fields — verify required fields modal appears
8. Select multiple tasks with checkboxes — verify bulk action bar appears
9. Click "Alocar" — verify member selection modal
10. Click "Voltar para Backlog" — verify tasks removed from view
11. Click "Finalizar Planning" — verify progress modal with operation tracking
12. Verify task type SVG icons show on all task rows

- [ ] **Step 7: Commit**

```bash
git add frontend/index.html
git commit -m "feat(planning): add full Planning tab with drag-and-drop, bulk actions, and Jira push"
```

---

## Self-Review Checklist

**Spec coverage:**
- ✅ Tab visibility (3 days, active only) — Task 6, Step 3
- ✅ Next sprint lookup — Task 2 (repo), Task 3 (service), Task 4 (handler)
- ✅ Person cards with capacity — Task 6, `renderPlanningTab()`
- ✅ Unassigned card — Task 6, `renderPlanningTab()`
- ✅ Drag-and-drop (SortableJS) — Task 6, Steps 1 + 5
- ✅ Overcapacity modal — Task 6, `showOvercapacityModal()`
- ✅ Required fields modal — Task 6, `showRequiredFieldsModal()`
- ✅ Header stats — Task 6, `calcPlanningStats()` + `renderPlanningTab()`
- ✅ Multi-select + bulk Alocar — Task 6, `bulkAlocar()` + `confirmBulkAlocar()`
- ✅ Bulk Mover próxima Sprint — Task 6, `bulkMoverProximaSprint()`
- ✅ Bulk Voltar Backlog — Task 6, `bulkVoltarBacklog()`
- ✅ Task type SVG icons — Task 5
- ✅ Client-side capacity engine — Task 6, `recalcMemberCapacity()`
- ✅ Changeset builder — Task 6, `buildChangeset()`
- ✅ Finalizar with progress modal — Task 6, `finalizarPlanning()` + `pollPlanningProgress()`
- ✅ Backend Apply endpoint — Task 3 (service) + Task 4 (handler)
- ✅ Backend Progress endpoint — Task 3 + Task 4
- ✅ RemoveFromSprint Jira method — Task 1
- ✅ Fallback to backlog if no next sprint — Task 6, `bulkMoverProximaSprint()`

**Placeholder scan:** No TBD/TODO. SortableJS minified content placeholder noted in Step 1 — implementer must download and inline the actual content.

**Type consistency:**
- `PlanningTarefa` (repo) used consistently in service and handler
- `PlanningApplyRequest` / `PlanningChanges` used consistently between handler and service
- `PlanningJobProgress` / `PlanningOperation` used consistently between service and handler
- `NextSprintResult` used consistently
- `RemoveFromSprint` signature matches across Client interface, HTTPClient impl, and mock
