# SP3: Sprint + Allocation Service Unit Tests — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Achieve ≥70% unit test coverage for `internal/service/sprint.go` (~1,072 lines) and `internal/service/allocation.go` (~575 lines) using stdlib `testing` + function-field mocks from SP1.

**Architecture:** Each public method gets test coverage for happy path and key error paths. The `mockSprintRepoStore` and `mockAllocationStore` (function-field pattern) from `mocks_test.go` are used. SprintService is tested in a new `sprint_test.go`. AllocationService tests extend existing `allocation_test.go`. Private helpers (e.g., `contarDiasUteisComFeriados`, `computeProjectStatus`) are tested indirectly through public methods or directly if exported to test package.

**Tech Stack:** Go stdlib `testing`, `go.uber.org/zap` (nop logger), function-field mocks from `mocks_test.go`

## Global Constraints

- Framework: stdlib `testing` only — no testify, gomock, or external test deps
- Mock pattern: function-field injection per test (matches existing mocks in `mocks_test.go`)
- Files: `internal/service/sprint_test.go` (new), `internal/service/allocation_test.go` (extend)
- No commits until everything passes 100%
- Coverage target: ≥70% of both `sprint.go` and `allocation.go` lines
- Logger: use `zap.NewNop()` in all tests

---

### Task 1: SprintService — Pass-Through Methods + ListSprints

**Files:**
- Create: `internal/service/sprint_test.go`

**Interfaces:**
- Consumes: `mockSprintRepoStore` from `mocks_test.go`
- Produces: `TestNewSprintService`, `TestListProjetosComSprints`, `TestListByProjeto`, `TestListSprints`

**Context:** `ListProjetosComSprints` and `ListByProjeto` are simple pass-throughs to the repo. `ListSprints` has logic: when equipeID is non-nil, it first calls `GetEquipeBoardID` then passes boardID to `ListSprints`. Test both paths (with and without equipeID) and error from `GetEquipeBoardID`.

- [ ] **Step 1: Create sprint_test.go with imports and TestNewSprintService**

```go
package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestNewSprintService(t *testing.T) {
	svc := NewSprintService(nil, zap.NewNop())
	if svc == nil {
		t.Fatal("expected non-nil")
	}
}
```

- [ ] **Step 2: Write TestListProjetosComSprints**

```go
func TestListProjetosComSprints(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		expected := []repository.ProjetoComSprints{{ProjetoID: uuid.New(), ProjetoNome: "P1"}}
		repo := &mockSprintRepoStore{
			listProjetosComSprintsFn: func(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error) {
				return expected, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.ListProjetosComSprints(ctx, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("expected 1 result, got %d", len(result))
		}
	})

	t.Run("error", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			listProjetosComSprintsFn: func(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.ListProjetosComSprints(ctx, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
```

- [ ] **Step 3: Write TestListByProjeto**

```go
func TestListByProjeto(t *testing.T) {
	ctx := context.Background()
	projetoID := uuid.New()

	t.Run("success", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			listByProjetoFn: func(ctx context.Context, pid uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
				return []repository.SprintListItem{{ID: uuid.New(), Nome: "Sprint 1"}}, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.ListByProjeto(ctx, projetoID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("expected 1, got %d", len(result))
		}
	})

	t.Run("error", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			listByProjetoFn: func(ctx context.Context, pid uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.ListByProjeto(ctx, projetoID, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
```

- [ ] **Step 4: Write TestListSprints with equipeID logic**

```go
func TestListSprints(t *testing.T) {
	ctx := context.Background()

	t.Run("without equipeID", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			listSprintsFn: func(ctx context.Context, eid *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error) {
				if boardID != nil {
					t.Error("expected nil boardID when no equipeID")
				}
				return []repository.SprintListItem{{Nome: "S1"}}, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.ListSprints(ctx, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("expected 1, got %d", len(result))
		}
	})

	t.Run("with equipeID gets boardID", func(t *testing.T) {
		equipeID := uuid.New()
		boardID := 42
		repo := &mockSprintRepoStore{
			getEquipeBoardIDFn: func(ctx context.Context, eid uuid.UUID) (*int, error) {
				return &boardID, nil
			},
			listSprintsFn: func(ctx context.Context, eid *uuid.UUID, estado *string, bid *int) ([]repository.SprintListItem, error) {
				if bid == nil || *bid != 42 {
					t.Errorf("expected boardID 42, got %v", bid)
				}
				return []repository.SprintListItem{{Nome: "S1"}}, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.ListSprints(ctx, &equipeID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("GetEquipeBoardID error", func(t *testing.T) {
		equipeID := uuid.New()
		repo := &mockSprintRepoStore{
			getEquipeBoardIDFn: func(ctx context.Context, eid uuid.UUID) (*int, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.ListSprints(ctx, &equipeID, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/service/ -run "TestNewSprintService|TestListProjetosComSprints|TestListByProjeto|TestListSprints" -v`
Expected: all PASS

---

### Task 2: SprintService — GetCapacity Tests

**Files:**
- Modify: `internal/service/sprint_test.go`

**Interfaces:**
- Consumes: `mockSprintRepoStore` from `mocks_test.go`
- Produces: `TestGetCapacity` covering: GetByID error, sprint without dates (returns empty), happy path with members/tarefas/feriados/ausencias

**Context:** `GetCapacity` (line 123-463) is the largest method. It calls: GetByID → GetProjetoChave → GetFeriadosNoPeriodo → GetMembrosEquipeIDs/Info → GetMembrosFromSprint → GetTarefasDetailBySprint → GetAusenciasNoPeriodo. It calculates capacity, hours, and percentages per member. Key branches: sprint without dates returns early; equipeID nil vs non-nil; members with no tasks; various status category mappings for tasks.

The `domain.Sprint` struct is needed for GetByID return. Check its shape. The `repository.TarefaDetail`, `repository.MembroInfo`, `repository.AusenciaRecord`, `repository.FeriadoRecord` types are needed for mock returns.

- [ ] **Step 1: Write TestGetCapacity_GetByIDError**

```go
func TestGetCapacity(t *testing.T) {
	ctx := context.Background()
	sprintID := uuid.New()

	t.Run("GetByID error", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.GetCapacity(ctx, sprintID, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
```

- [ ] **Step 2: Write TestGetCapacity_NoDates**

Sprint with nil DataInicio/DataFim should return empty result:

```go
	t.Run("sprint without dates returns empty", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
				return &domain.Sprint{ID: sprintID, Nome: "No Dates"}, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.GetCapacity(ctx, sprintID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Sprint.Nome != "No Dates" {
			t.Errorf("expected sprint nome 'No Dates', got %q", result.Sprint.Nome)
		}
		if len(result.Membros) != 0 {
			t.Errorf("expected 0 membros, got %d", len(result.Membros))
		}
	})
```

- [ ] **Step 3: Write TestGetCapacity_HappyPath**

Full happy path with dates, members, tasks, holidays, ausencias. Need to construct all mock returns. This is the most complex test — construct minimal but complete data.

Read `sprint.go` GetCapacity carefully. Need `domain.Sprint` with DataInicio, DataFim, ProjetoID, FonteDadosID. Need `repository.MembroInfo` with ID, Nome. Need `repository.TarefaDetail` with MembroID, Horas, Status, ProjetoID, ProjetoChave, ProjetoNome. Need `repository.AusenciaRecord` with MembroID, DataInicio, DataFim, Tipo.

```go
	t.Run("happy path with members and tasks", func(t *testing.T) {
		inicio := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)  // Monday
		fim := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)     // Friday next week
		projetoID := uuid.New()
		membroID := uuid.New()

		repo := &mockSprintRepoStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
				return &domain.Sprint{
					ID: sprintID, Nome: "Sprint 1",
					DataInicio: &inicio, DataFim: &fim,
					ProjetoID: &projetoID, FonteDadosID: uuid.New(),
				}, nil
			},
			getProjetoChaveFn: func(ctx context.Context, pid uuid.UUID) (string, error) {
				return "PROJ", nil
			},
			getFeriadosNoPeriodoFn: func(ctx context.Context, i, f time.Time) ([]repository.FeriadoRecord, error) {
				return []repository.FeriadoRecord{}, nil
			},
			getMembrosFromSprintFn: func(ctx context.Context, sid uuid.UUID) ([]repository.MembroInfo, error) {
				return []repository.MembroInfo{
					{ID: membroID, Nome: "Dev 1"},
				}, nil
			},
			getTarefasDetailBySprintFn: func(ctx context.Context, sid uuid.UUID) ([]repository.TarefaDetail, error) {
				return []repository.TarefaDetail{
					{
						MembroID: &membroID, Horas: 8, Status: "Desenvolvimento",
						ProjetoID: projetoID, ProjetoChave: "PROJ", ProjetoNome: "Project",
						ID: uuid.New(), NumeroTicket: "PROJ-1", Resumo: "Task 1", Tipo: "Story",
					},
				}, nil
			},
			getAusenciasNoPeriodoFn: func(ctx context.Context, mids []uuid.UUID, i, f time.Time) ([]repository.AusenciaRecord, error) {
				return []repository.AusenciaRecord{}, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.GetCapacity(ctx, sprintID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.DiasUteis <= 0 {
			t.Error("expected positive dias uteis")
		}
		if len(result.Membros) != 1 {
			t.Fatalf("expected 1 membro, got %d", len(result.Membros))
		}
		if result.Membros[0].Nome != "Dev 1" {
			t.Errorf("expected membro nome 'Dev 1', got %q", result.Membros[0].Nome)
		}
	})
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/ -run "TestGetCapacity" -v`
Expected: all PASS

---

### Task 3: SprintService — GetUnplannedAnalysis + GetDisclaimerTasks + GetBurndown

**Files:**
- Modify: `internal/service/sprint_test.go`

**Interfaces:**
- Consumes: `mockSprintRepoStore`
- Produces: `TestGetUnplannedAnalysis`, `TestGetDisclaimerTasks`, `TestGetBurndown`

**Context:** `GetUnplannedAnalysis` (line 465) calls GetUnplannedStats, optionally GetEquipeNome, GetSprintProjetoID, GetHistoricalUnplanned, GetByID for each historical sprint, GetFeriadosNoPeriodo. Test: error from GetUnplannedStats, happy path with stats only (no projetoID), happy path with historical data.

`GetDisclaimerTasks` (line 573) calls GetDisclaimerTasks + GetDisclaimerTarefaProdutos. Test: error, happy path.

`GetBurndown` (line 643) calls GetByID, GetBurndownTarefas, computes burndown lines. Test: error, sprint without dates, happy path.

Read `sprint.go` for these methods before implementing. Each test should cover error path + happy path.

- [ ] **Step 1: Write TestGetUnplannedAnalysis**

Test error from GetUnplannedStats and happy path with basic stats:

```go
func TestGetUnplannedAnalysis(t *testing.T) {
	ctx := context.Background()
	sprintID := uuid.New()

	t.Run("GetUnplannedStats error", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			getUnplannedStatsFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID) (*repository.UnplannedStats, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.GetUnplannedAnalysis(ctx, sprintID, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("happy path no projetoID", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			getUnplannedStatsFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID) (*repository.UnplannedStats, error) {
				return &repository.UnplannedStats{TotalTarefas: 10, TarefasNaoPlanejadas: 3}, nil
			},
			getSprintProjetoIDFn: func(ctx context.Context, sid uuid.UUID) (*uuid.UUID, error) {
				return nil, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.GetUnplannedAnalysis(ctx, sprintID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.SprintAtual.TotalTarefas != 10 {
			t.Errorf("expected 10 total, got %d", result.SprintAtual.TotalTarefas)
		}
	})
}
```

- [ ] **Step 2: Write TestGetDisclaimerTasks**

```go
func TestGetDisclaimerTasks(t *testing.T) {
	ctx := context.Background()
	sprintID := uuid.New()

	t.Run("error", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			getDisclaimerTasksFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID, tt string) ([]repository.DisclaimerTarefaRow, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.GetDisclaimerTasks(ctx, sprintID, nil, "unplanned")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("happy path", func(t *testing.T) {
		tarefaID := uuid.New()
		repo := &mockSprintRepoStore{
			getDisclaimerTasksFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID, tt string) ([]repository.DisclaimerTarefaRow, error) {
				return []repository.DisclaimerTarefaRow{{ID: tarefaID, NumeroTicket: "T-1", Resumo: "Task"}}, nil
			},
			getDisclaimerTarefaProdFn: func(ctx context.Context, tids []uuid.UUID) (map[uuid.UUID][]string, error) {
				return map[uuid.UUID][]string{tarefaID: {"Produto A"}}, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.GetDisclaimerTasks(ctx, sprintID, nil, "unplanned")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Tarefas) != 1 {
			t.Errorf("expected 1 tarefa, got %d", len(result.Tarefas))
		}
	})
}
```

- [ ] **Step 3: Write TestGetBurndown**

Read `sprint.go:643` to understand GetBurndown shape. Test error from GetByID, sprint without dates, happy path.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/ -run "TestGetUnplannedAnalysis|TestGetDisclaimerTasks|TestGetBurndown" -v`
Expected: all PASS

---

### Task 4: SprintService — GetSprintsTimeline + GetTimelineDetail

**Files:**
- Modify: `internal/service/sprint_test.go`

**Interfaces:**
- Consumes: `mockSprintRepoStore`
- Produces: `TestGetSprintsTimeline`, `TestGetTimelineDetail`

**Context:** `GetSprintsTimeline` (line 815) calls ListSprintsIncludeEmpty, GetAllMembrosEquipe, GetHorasAlocadasPorSprint. Returns timeline items with sprint status, capacity, allocation percentages.

`GetTimelineDetail` (line 996) calls GetByID, GetTimelineDetailTarefas, computes per-member allocation detail.

Test error paths + happy path for each.

- [ ] **Step 1: Write TestGetSprintsTimeline**

Test error from ListSprintsIncludeEmpty, empty sprints, happy path with sprints.

- [ ] **Step 2: Write TestGetTimelineDetail**

Test GetByID error, sprint without dates, happy path.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/service/ -run "TestGetSprintsTimeline|TestGetTimelineDetail" -v`
Expected: all PASS

---

### Task 5: AllocationService — ListProjectAllocations + GetProjectDetail + Simple Methods

**Files:**
- Modify: `internal/service/allocation_test.go`

**Interfaces:**
- Consumes: `mockAllocationStore`, `mockSprintRepoStore`, `mockFonteDadosStore` from `mocks_test.go`
- Produces: `TestListProjectAllocations_HappyPath`, `TestGetProjectDetail`, `TestGetAvailableSprints`, `TestCloseProject`, `TestReopenProject`, `TestGetFilteredProducts`

**Context:** `ListProjectAllocations` (line 148) calls GetEpicsByEquipeAndProduto, CheckGDPTCAncestors, GetClosedEpicIDs, and iterates to build ProjectAllocation list. `GetProjectDetail` (line 229) calls GetEpicByID, GetEpicTasks, GetEpicPeople, builds ProjectDetail with categorized tasks. Simple methods: `GetAvailableSprints` (pass-through), `CloseProject`, `ReopenProject`, `GetFilteredProducts`.

- [ ] **Step 1: Write TestListProjectAllocations_HappyPath**

Test with repo returning epic rows, GDPTC ancestors, closed epic check. Verify computed fields (pctEstimado, pctPlanejado, status).

- [ ] **Step 2: Write TestGetProjectDetail**

Test GetEpicByID error, happy path with tasks categorized.

- [ ] **Step 3: Write TestGetAvailableSprints, TestCloseProject, TestReopenProject, TestGetFilteredProducts**

Simple error + happy path for each pass-through.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/ -run "TestListProjectAllocations|TestGetProjectDetail|TestGetAvailableSprints|TestCloseProject|TestReopenProject|TestGetFilteredProducts" -v`
Expected: all PASS

---

### Task 6: AllocationService — AllocateTask + SyncProjectTasks

**Files:**
- Modify: `internal/service/allocation_test.go`

**Interfaces:**
- Consumes: `mockAllocationStore`, `mockSprintRepoStore`, `mockFonteDadosStore`, `mockSyncRepoStore` from `mocks_test.go`
- Produces: `TestAllocateTask`, `TestSyncProjectTasks`

**Context:** `AllocateTask` (line 383) calls GetTaskPreviousState, checks for conflict (member over 100% capacity via sprintSvc.GetCapacity), calls UpdateTaskAllocation, fires `go writeToJira`. Test: previous state error, conflict detection (Force=false), happy path.

`SyncProjectTasks` (line 546) calls repo.GetEpicByID to get epic ticket number, then syncSvc.SyncEpicTasks. Test: GetEpicByID error, happy path.

- [ ] **Step 1: Write TestAllocateTask**

Test GetTaskPreviousState error, happy path with no conflict.

- [ ] **Step 2: Write TestSyncProjectTasks**

Test GetEpicByID error, happy path.

- [ ] **Step 3: Run all tests + coverage**

Run: `go test ./internal/service/ -v -coverprofile=coverage.out && go tool cover -func=coverage.out | grep -E "sprint.go|allocation.go"`
Expected: ≥70% coverage on both files, all tests PASS
