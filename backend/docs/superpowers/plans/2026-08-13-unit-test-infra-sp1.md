# SP1: Infraestrutura de Testabilidade — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract 9 repository interfaces into the service package, create matching mock structs, refactor all service structs/constructors to accept interfaces, and add coverage tooling — enabling unit testing of the service layer without database dependencies.

**Architecture:** Define interfaces at the consumer (service package) following Go idiom "accept interfaces, return structs." Each interface mirrors a repository's methods (union of what all consuming services call). Function-field mock structs for per-test injection. Concrete `*repository.XxxRepository` types satisfy the interfaces implicitly — zero changes to main.go wiring.

**Tech Stack:** Go stdlib `testing`, hand-written mocks, no external test frameworks.

## Global Constraints

- Go stdlib only — `testing` package, no testify/gomock/mockery
- Hand-written mock structs using function-field pattern (consistent with existing `mockJiraClient` in `sync_test.go`)
- Interfaces defined in service package (consumer), not repository package (provider)
- **No changes to repository package** — interfaces mirror existing repo method signatures exactly
- `coverage.out` and `coverage.html` added to `.gitignore`
- Naming collision: `SprintStore` and `SyncStore` already exist in the service package as handler→service interfaces. New repo interfaces use `SprintRepoStore` and `SyncRepoStore` to avoid collision.
- `EqualizerService.sprintRepo` stays concrete `*repository.SprintRepository` because it calls `Pool().QueryRow(...)` — a raw pgxpool access that can't be cleanly interfaced without repo changes. All other EqualizerService repo fields are refactored.

---

## File Structure

| Action | File | Purpose |
|--------|------|---------|
| Create | `internal/service/interfaces.go` | 9 repository interfaces (84 methods total) |
| Create | `internal/service/mocks_test.go` | 9 mock structs matching each interface |
| Modify | `internal/service/sprint.go` | SprintService struct + constructor |
| Modify | `internal/service/sync.go` | SyncService struct + constructor |
| Modify | `internal/service/allocation.go` | AllocationService struct + constructor |
| Modify | `internal/service/equalizer.go` | EqualizerService struct (fdRepo, configRepo only) + constructor |
| Modify | `internal/service/sprint_generation.go` | SprintGenerationService struct + constructor |
| Modify | `internal/service/review.go` | ReviewService struct + constructor |
| Modify | `internal/service/notification.go` | NotificationService struct + constructor |
| Modify | `internal/service/email.go` | EmailProvider struct + constructor |
| Modify | `internal/service/whatsapp.go` | WhatsAppProvider struct + constructor |
| Modify | `internal/service/scheduler.go` | SchedulerService struct + constructor |
| Create | `Makefile` | test, test-cover, test-cover-html targets |
| Create | `scripts/check-coverage.sh` | Coverage threshold enforcement |
| Create | `.gitignore` | coverage.out, coverage.html |

---

### Task 1: Create Repository Interfaces

**Files:**
- Create: `internal/service/interfaces.go`

**Interfaces:**
- Consumes: existing repository method signatures (read-only reference)
- Produces: 9 interfaces consumed by Tasks 2 and 3 — `FonteDadosStore`, `SprintRepoStore`, `SyncRepoStore`, `AllocationStore`, `ReviewStore`, `ConfigStore`, `EquipeStore`, `DestinatarioStore`, `SyncScheduleStore`

- [ ] **Step 1: Create `internal/service/interfaces.go`**

```go
package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
)

// FonteDadosStore abstracts *repository.FonteDadosRepository for testing.
// Consumed by: SyncService, AllocationService, EqualizerService, SprintGenerationService.
type FonteDadosStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error)
	SaveOAuthTokens(ctx context.Context, id uuid.UUID, baseURL, accessToken, refreshToken string, expiry time.Time) error
	UpdateUltimoSync(ctx context.Context, id uuid.UUID, syncTime time.Time) error
}

// SprintRepoStore abstracts *repository.SprintRepository for testing.
// Named "RepoStore" to avoid collision with existing SprintStore (handler→service interface).
// Consumed by: SprintService, AllocationService, NotificationService, SprintGenerationService.
// Note: EqualizerService uses concrete *repository.SprintRepository (needs Pool() for raw query).
type SprintRepoStore interface {
	ListProjetosComSprints(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error)
	ListByProjeto(ctx context.Context, projetoID uuid.UUID, estado *string) ([]repository.SprintListItem, error)
	GetEquipeBoardID(ctx context.Context, equipeID uuid.UUID) (*int, error)
	ListSprints(ctx context.Context, equipeID *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Sprint, error)
	GetProjetoChave(ctx context.Context, projetoID uuid.UUID) (string, error)
	GetFeriadosNoPeriodo(ctx context.Context, inicio, fim time.Time) ([]repository.FeriadoRecord, error)
	GetMembrosEquipeIDs(ctx context.Context, equipeID uuid.UUID, dataFim time.Time) (map[uuid.UUID]bool, error)
	GetMembrosEquipeInfo(ctx context.Context, equipeID uuid.UUID, dataFim time.Time) ([]repository.MembroInfo, error)
	GetMembrosFromSprint(ctx context.Context, sprintID uuid.UUID) ([]repository.MembroInfo, error)
	GetTarefasDetailBySprint(ctx context.Context, sprintID uuid.UUID) ([]repository.TarefaDetail, error)
	GetAusenciasNoPeriodo(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]repository.AusenciaRecord, error)
	GetUnplannedStats(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*repository.UnplannedStats, error)
	GetEquipeNome(ctx context.Context, equipeID uuid.UUID) (string, error)
	GetSprintProjetoID(ctx context.Context, sprintID uuid.UUID) (*uuid.UUID, error)
	GetHistoricalUnplanned(ctx context.Context, projetoID uuid.UUID, equipeID *uuid.UUID, currentSprintID uuid.UUID, limit int) ([]repository.HistoricalUnplannedItem, error)
	GetDisclaimerTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, taskType string) ([]repository.DisclaimerTarefaRow, error)
	GetDisclaimerTarefaProdutos(ctx context.Context, tarefaIDs []uuid.UUID) (map[uuid.UUID][]string, error)
	GetBurndownTarefas(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) ([]repository.BurndownTarefa, error)
	ListSprintsIncludeEmpty(ctx context.Context, equipeID *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error)
	GetAllMembrosEquipe(ctx context.Context, equipeID uuid.UUID) ([]repository.MembroInfo, error)
	GetHorasAlocadasPorSprint(ctx context.Context, sprintIDs []uuid.UUID, membroIDs []uuid.UUID) (map[uuid.UUID]float64, error)
	GetTimelineDetailTarefas(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID) ([]repository.TimelineDetailTarefa, error)
	GetMembroJiraAccountID(ctx context.Context, membroID uuid.UUID) (string, error)
	GetEqualizerTarefas(ctx context.Context, sprintID, membroID uuid.UUID) ([]repository.EqualizerTarefa, error)
	UpdateTarefaResponsavel(ctx context.Context, sprintID, tarefaID, novoResponsavelID uuid.UUID) error
}

// SyncRepoStore abstracts *repository.SyncRepository for testing.
// Named "RepoStore" to avoid collision with existing SyncStore (handler→service interface).
// Consumed by: SyncService, SprintGenerationService.
type SyncRepoStore interface {
	HasRunningSyncForProject(ctx context.Context, fonteDadosID uuid.UUID, projectKey string) (bool, error)
	CreateSyncLog(ctx context.Context, log *domain.SyncLog) error
	UpdateSyncLog(ctx context.Context, id uuid.UUID, status string, finalizadoEm time.Time, totals repository.SyncTotals, erros json.RawMessage, mensagem *string) error
	GetFonteDadosAtivas(ctx context.Context) ([]domain.FonteDados, error)
	GetLatestSyncLog(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error)
	ListSyncLogs(ctx context.Context, fonteDadosID uuid.UUID, limit int) ([]domain.SyncLog, error)
	HasRunningSync(ctx context.Context, fonteDadosID uuid.UUID) (bool, error)
	UpdateSyncLogTotals(ctx context.Context, id uuid.UUID, totals repository.SyncTotals) error
	LookupTarefaIDByJiraID(ctx context.Context, fonteDadosID uuid.UUID, jiraID string) (uuid.UUID, error)
	UpdateTarefaParent(ctx context.Context, tarefaID, parentID uuid.UUID) error
	UpdateCustomFieldMap(ctx context.Context, fonteID uuid.UUID, cfMap json.RawMessage) error
	GetProjectKeysForSync(ctx context.Context, fonteDadosID uuid.UUID) ([]string, error)
	UpsertProduto(ctx context.Context, fonteDadosID uuid.UUID, jiraID, nome string, descricao *string, projetoID *uuid.UUID) (uuid.UUID, error)
	LinkTarefaProduto(ctx context.Context, tarefaID, produtoID uuid.UUID) error
	UndeleteReappearedTarefas(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error)
	SoftDeleteAbsentTarefas(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error)
	UpsertProjeto(ctx context.Context, fonteDadosID uuid.UUID, jiraID, chave, nome string, descricao *string, leadID *uuid.UUID, categoria *string) (uuid.UUID, error)
	UpsertMembro(ctx context.Context, fonteDadosID uuid.UUID, jiraAccountID, nome string, email, avatarURL, team *string) (uuid.UUID, error)
	GetDistinctBoardProjects(ctx context.Context, fonteDadosID uuid.UUID) (map[int]uuid.UUID, error)
	UpsertSprint(ctx context.Context, fonteDadosID uuid.UUID, jiraID int, nome string, estado *string, dataInicio, dataFim, dataConclusao *time.Time, boardID *int, projetoID *uuid.UUID) (uuid.UUID, error)
	UpsertTarefa(ctx context.Context, t *repository.UpsertTarefaParams) (uuid.UUID, error)
	AutoDetectEquipeBoardIDs(ctx context.Context, fonteDadosID uuid.UUID) (int, error)
}

// AllocationStore abstracts *repository.AllocationRepository for testing.
// Consumed by: AllocationService.
type AllocationStore interface {
	GetEpicsByEquipeAndProduto(ctx context.Context, equipeID uuid.UUID, produtoNomes []string, statusFilter string) ([]repository.EpicAllocationRow, error)
	CheckGDPTCAncestors(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	GetClosedEpicIDs(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	GetProjectClosure(ctx context.Context, epicID uuid.UUID) (*repository.ProjectClosureRow, error)
	GetEpicByID(ctx context.Context, epicID uuid.UUID) (*repository.EpicAllocationRow, error)
	GetEpicTasks(ctx context.Context, epicID uuid.UUID) ([]repository.TaskAllocationRow, error)
	GetEpicPeople(ctx context.Context, epicID uuid.UUID) ([]repository.PersonAllocationRow, error)
	GetTaskPreviousState(ctx context.Context, taskID uuid.UUID) (*repository.TaskPreviousState, error)
	UpdateTaskAllocation(ctx context.Context, taskID, sprintID uuid.UUID, assigneeID *uuid.UUID, estimateSeconds int) error
	GetTaskJiraKey(ctx context.Context, taskID uuid.UUID) (string, error)
	GetTaskFonteDadosID(ctx context.Context, taskID uuid.UUID) (uuid.UUID, error)
	GetSprintJiraID(ctx context.Context, sprintID uuid.UUID) (int, error)
	RollbackTaskAllocation(ctx context.Context, taskID uuid.UUID, prev *repository.TaskPreviousState) error
	GetFutureSprintsByEquipe(ctx context.Context, equipeID uuid.UUID) ([]repository.SprintOptionRow, error)
	CloseProject(ctx context.Context, epicID uuid.UUID, descricao string, dataEncerramento time.Time, encerradoPor string) error
	ReopenProject(ctx context.Context, epicID uuid.UUID) error
	GetProdutosComProjetosAtivos(ctx context.Context) ([]repository.ProdutoRow, error)
}

// ReviewStore abstracts *repository.ReviewRepository for testing.
// Consumed by: ReviewService.
type ReviewStore interface {
	GetReviewAnalise(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*repository.ReviewAnalise, error)
	GetSprintEstado(ctx context.Context, sprintID uuid.UUID) (*string, error)
	GetSprintSnapshot(ctx context.Context, sprintID uuid.UUID) ([]repository.ReviewTaskRow, error)
	GetReviewTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, produtoIDs []uuid.UUID) ([]repository.ReviewTaskRow, error)
	GetGDPTCAncestorTaskIDs(ctx context.Context, taskIDs []uuid.UUID) ([]uuid.UUID, error)
	GetReviewPOs(ctx context.Context, equipeID uuid.UUID, produtoIDs []uuid.UUID) ([]repository.ReviewPO, error)
	ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error)
	CreateDestaque(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error)
	UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error)
	DeleteDestaque(ctx context.Context, id uuid.UUID) error
	SaveReviewAnalise(ctx context.Context, a repository.ReviewAnalise) error
}

// ConfigStore abstracts *repository.ConfigRepository for testing.
// Consumed by: ReviewService, EmailProvider, WhatsAppProvider, EqualizerService.
type ConfigStore interface {
	GetConfig(ctx context.Context, chave string) (string, error)
}

// EquipeStore abstracts *repository.EquipeRepository for testing.
// Consumed by: SprintGenerationService.
// Note: ImportService/InvestimentoService also use EquipeRepository but are out of SP1 scope.
type EquipeStore interface {
	GetMembrosEquipe(ctx context.Context, equipeID uuid.UUID) ([]domain.Membro, error)
}

// DestinatarioStore abstracts *repository.DestinatarioRepository for testing.
// Consumed by: NotificationService.
type DestinatarioStore interface {
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]repository.Destinatario, error)
}

// SyncScheduleStore abstracts *repository.SyncScheduleRepository for testing.
// Consumed by: SchedulerService.
type SyncScheduleStore interface {
	GetDueSchedules(ctx context.Context, horaMinuto string) ([]domain.SyncSchedule, error)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/service/...`
Expected: PASS (interfaces exist but nothing uses them yet)

- [ ] **Step 3: Commit**

```bash
git add internal/service/interfaces.go
git commit -m "feat: add 9 repository interfaces for service layer testability"
```

---

### Task 2: Create Mock Structs

**Files:**
- Create: `internal/service/mocks_test.go`

**Interfaces:**
- Consumes: all 9 interfaces from Task 1 (`FonteDadosStore`, `SprintRepoStore`, `SyncRepoStore`, `AllocationStore`, `ReviewStore`, `ConfigStore`, `EquipeStore`, `DestinatarioStore`, `SyncScheduleStore`)
- Produces: 9 mock structs used by future test files (SP2-SP5)

Pattern: each mock struct has one function field per interface method. Each method implementation delegates to its function field. If the function field is nil, the method panics — this catches unintentional calls in tests.

- [ ] **Step 1: Create `internal/service/mocks_test.go`**

```go
package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
)

// --- mockFonteDadosStore ---

type mockFonteDadosStore struct {
	getByIDFn          func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error)
	saveOAuthTokensFn  func(ctx context.Context, id uuid.UUID, baseURL, accessToken, refreshToken string, expiry time.Time) error
	updateUltimoSyncFn func(ctx context.Context, id uuid.UUID, syncTime time.Time) error
}

func (m *mockFonteDadosStore) GetByID(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockFonteDadosStore) SaveOAuthTokens(ctx context.Context, id uuid.UUID, baseURL, accessToken, refreshToken string, expiry time.Time) error {
	return m.saveOAuthTokensFn(ctx, id, baseURL, accessToken, refreshToken, expiry)
}
func (m *mockFonteDadosStore) UpdateUltimoSync(ctx context.Context, id uuid.UUID, syncTime time.Time) error {
	return m.updateUltimoSyncFn(ctx, id, syncTime)
}

// --- mockSprintRepoStore ---

type mockSprintRepoStore struct {
	listProjetosComSprintsFn    func(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error)
	listByProjetoFn             func(ctx context.Context, projetoID uuid.UUID, estado *string) ([]repository.SprintListItem, error)
	getEquipeBoardIDFn          func(ctx context.Context, equipeID uuid.UUID) (*int, error)
	listSprintsFn               func(ctx context.Context, equipeID *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error)
	getByIDFn                   func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error)
	getProjetoChaveFn           func(ctx context.Context, projetoID uuid.UUID) (string, error)
	getFeriadosNoPeriodoFn      func(ctx context.Context, inicio, fim time.Time) ([]repository.FeriadoRecord, error)
	getMembrosEquipeIDsFn       func(ctx context.Context, equipeID uuid.UUID, dataFim time.Time) (map[uuid.UUID]bool, error)
	getMembrosEquipeInfoFn      func(ctx context.Context, equipeID uuid.UUID, dataFim time.Time) ([]repository.MembroInfo, error)
	getMembrosFromSprintFn      func(ctx context.Context, sprintID uuid.UUID) ([]repository.MembroInfo, error)
	getTarefasDetailBySprintFn  func(ctx context.Context, sprintID uuid.UUID) ([]repository.TarefaDetail, error)
	getAusenciasNoPeriodoFn     func(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]repository.AusenciaRecord, error)
	getUnplannedStatsFn         func(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*repository.UnplannedStats, error)
	getEquipeNomeFn             func(ctx context.Context, equipeID uuid.UUID) (string, error)
	getSprintProjetoIDFn        func(ctx context.Context, sprintID uuid.UUID) (*uuid.UUID, error)
	getHistoricalUnplannedFn    func(ctx context.Context, projetoID uuid.UUID, equipeID *uuid.UUID, currentSprintID uuid.UUID, limit int) ([]repository.HistoricalUnplannedItem, error)
	getDisclaimerTasksFn        func(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, taskType string) ([]repository.DisclaimerTarefaRow, error)
	getDisclaimerTarefaProdFn   func(ctx context.Context, tarefaIDs []uuid.UUID) (map[uuid.UUID][]string, error)
	getBurndownTarefasFn        func(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) ([]repository.BurndownTarefa, error)
	listSprintsIncludeEmptyFn   func(ctx context.Context, equipeID *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error)
	getAllMembrosEquipeFn        func(ctx context.Context, equipeID uuid.UUID) ([]repository.MembroInfo, error)
	getHorasAlocadasPorSprintFn func(ctx context.Context, sprintIDs []uuid.UUID, membroIDs []uuid.UUID) (map[uuid.UUID]float64, error)
	getTimelineDetailTarefasFn  func(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID) ([]repository.TimelineDetailTarefa, error)
	getMembroJiraAccountIDFn    func(ctx context.Context, membroID uuid.UUID) (string, error)
	getEqualizerTarefasFn       func(ctx context.Context, sprintID, membroID uuid.UUID) ([]repository.EqualizerTarefa, error)
	updateTarefaResponsavelFn   func(ctx context.Context, sprintID, tarefaID, novoResponsavelID uuid.UUID) error
}

func (m *mockSprintRepoStore) ListProjetosComSprints(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error) {
	return m.listProjetosComSprintsFn(ctx, equipeID)
}
func (m *mockSprintRepoStore) ListByProjeto(ctx context.Context, projetoID uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
	return m.listByProjetoFn(ctx, projetoID, estado)
}
func (m *mockSprintRepoStore) GetEquipeBoardID(ctx context.Context, equipeID uuid.UUID) (*int, error) {
	return m.getEquipeBoardIDFn(ctx, equipeID)
}
func (m *mockSprintRepoStore) ListSprints(ctx context.Context, equipeID *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error) {
	return m.listSprintsFn(ctx, equipeID, estado, boardID)
}
func (m *mockSprintRepoStore) GetByID(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockSprintRepoStore) GetProjetoChave(ctx context.Context, projetoID uuid.UUID) (string, error) {
	return m.getProjetoChaveFn(ctx, projetoID)
}
func (m *mockSprintRepoStore) GetFeriadosNoPeriodo(ctx context.Context, inicio, fim time.Time) ([]repository.FeriadoRecord, error) {
	return m.getFeriadosNoPeriodoFn(ctx, inicio, fim)
}
func (m *mockSprintRepoStore) GetMembrosEquipeIDs(ctx context.Context, equipeID uuid.UUID, dataFim time.Time) (map[uuid.UUID]bool, error) {
	return m.getMembrosEquipeIDsFn(ctx, equipeID, dataFim)
}
func (m *mockSprintRepoStore) GetMembrosEquipeInfo(ctx context.Context, equipeID uuid.UUID, dataFim time.Time) ([]repository.MembroInfo, error) {
	return m.getMembrosEquipeInfoFn(ctx, equipeID, dataFim)
}
func (m *mockSprintRepoStore) GetMembrosFromSprint(ctx context.Context, sprintID uuid.UUID) ([]repository.MembroInfo, error) {
	return m.getMembrosFromSprintFn(ctx, sprintID)
}
func (m *mockSprintRepoStore) GetTarefasDetailBySprint(ctx context.Context, sprintID uuid.UUID) ([]repository.TarefaDetail, error) {
	return m.getTarefasDetailBySprintFn(ctx, sprintID)
}
func (m *mockSprintRepoStore) GetAusenciasNoPeriodo(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]repository.AusenciaRecord, error) {
	return m.getAusenciasNoPeriodoFn(ctx, membroIDs, inicio, fim)
}
func (m *mockSprintRepoStore) GetUnplannedStats(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*repository.UnplannedStats, error) {
	return m.getUnplannedStatsFn(ctx, sprintID, equipeID)
}
func (m *mockSprintRepoStore) GetEquipeNome(ctx context.Context, equipeID uuid.UUID) (string, error) {
	return m.getEquipeNomeFn(ctx, equipeID)
}
func (m *mockSprintRepoStore) GetSprintProjetoID(ctx context.Context, sprintID uuid.UUID) (*uuid.UUID, error) {
	return m.getSprintProjetoIDFn(ctx, sprintID)
}
func (m *mockSprintRepoStore) GetHistoricalUnplanned(ctx context.Context, projetoID uuid.UUID, equipeID *uuid.UUID, currentSprintID uuid.UUID, limit int) ([]repository.HistoricalUnplannedItem, error) {
	return m.getHistoricalUnplannedFn(ctx, projetoID, equipeID, currentSprintID, limit)
}
func (m *mockSprintRepoStore) GetDisclaimerTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, taskType string) ([]repository.DisclaimerTarefaRow, error) {
	return m.getDisclaimerTasksFn(ctx, sprintID, equipeID, taskType)
}
func (m *mockSprintRepoStore) GetDisclaimerTarefaProdutos(ctx context.Context, tarefaIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	return m.getDisclaimerTarefaProdFn(ctx, tarefaIDs)
}
func (m *mockSprintRepoStore) GetBurndownTarefas(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) ([]repository.BurndownTarefa, error) {
	return m.getBurndownTarefasFn(ctx, sprintID, equipeID)
}
func (m *mockSprintRepoStore) ListSprintsIncludeEmpty(ctx context.Context, equipeID *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error) {
	return m.listSprintsIncludeEmptyFn(ctx, equipeID, estado, boardID)
}
func (m *mockSprintRepoStore) GetAllMembrosEquipe(ctx context.Context, equipeID uuid.UUID) ([]repository.MembroInfo, error) {
	return m.getAllMembrosEquipeFn(ctx, equipeID)
}
func (m *mockSprintRepoStore) GetHorasAlocadasPorSprint(ctx context.Context, sprintIDs []uuid.UUID, membroIDs []uuid.UUID) (map[uuid.UUID]float64, error) {
	return m.getHorasAlocadasPorSprintFn(ctx, sprintIDs, membroIDs)
}
func (m *mockSprintRepoStore) GetTimelineDetailTarefas(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID) ([]repository.TimelineDetailTarefa, error) {
	return m.getTimelineDetailTarefasFn(ctx, sprintID, equipeID)
}
func (m *mockSprintRepoStore) GetMembroJiraAccountID(ctx context.Context, membroID uuid.UUID) (string, error) {
	return m.getMembroJiraAccountIDFn(ctx, membroID)
}
func (m *mockSprintRepoStore) GetEqualizerTarefas(ctx context.Context, sprintID, membroID uuid.UUID) ([]repository.EqualizerTarefa, error) {
	return m.getEqualizerTarefasFn(ctx, sprintID, membroID)
}
func (m *mockSprintRepoStore) UpdateTarefaResponsavel(ctx context.Context, sprintID, tarefaID, novoResponsavelID uuid.UUID) error {
	return m.updateTarefaResponsavelFn(ctx, sprintID, tarefaID, novoResponsavelID)
}

// --- mockSyncRepoStore ---

type mockSyncRepoStore struct {
	hasRunningSyncForProjectFn func(ctx context.Context, fonteDadosID uuid.UUID, projectKey string) (bool, error)
	createSyncLogFn            func(ctx context.Context, log *domain.SyncLog) error
	updateSyncLogFn            func(ctx context.Context, id uuid.UUID, status string, finalizadoEm time.Time, totals repository.SyncTotals, erros json.RawMessage, mensagem *string) error
	getFonteDadosAtivasFn      func(ctx context.Context) ([]domain.FonteDados, error)
	getLatestSyncLogFn         func(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error)
	listSyncLogsFn             func(ctx context.Context, fonteDadosID uuid.UUID, limit int) ([]domain.SyncLog, error)
	hasRunningSyncFn            func(ctx context.Context, fonteDadosID uuid.UUID) (bool, error)
	updateSyncLogTotalsFn      func(ctx context.Context, id uuid.UUID, totals repository.SyncTotals) error
	lookupTarefaIDByJiraIDFn   func(ctx context.Context, fonteDadosID uuid.UUID, jiraID string) (uuid.UUID, error)
	updateTarefaParentFn       func(ctx context.Context, tarefaID, parentID uuid.UUID) error
	updateCustomFieldMapFn     func(ctx context.Context, fonteID uuid.UUID, cfMap json.RawMessage) error
	getProjectKeysForSyncFn    func(ctx context.Context, fonteDadosID uuid.UUID) ([]string, error)
	upsertProdutoFn            func(ctx context.Context, fonteDadosID uuid.UUID, jiraID, nome string, descricao *string, projetoID *uuid.UUID) (uuid.UUID, error)
	linkTarefaProdutoFn        func(ctx context.Context, tarefaID, produtoID uuid.UUID) error
	undeleteReappearedFn       func(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error)
	softDeleteAbsentFn         func(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error)
	upsertProjetoFn            func(ctx context.Context, fonteDadosID uuid.UUID, jiraID, chave, nome string, descricao *string, leadID *uuid.UUID, categoria *string) (uuid.UUID, error)
	upsertMembroFn             func(ctx context.Context, fonteDadosID uuid.UUID, jiraAccountID, nome string, email, avatarURL, team *string) (uuid.UUID, error)
	getDistinctBoardProjectsFn func(ctx context.Context, fonteDadosID uuid.UUID) (map[int]uuid.UUID, error)
	upsertSprintFn             func(ctx context.Context, fonteDadosID uuid.UUID, jiraID int, nome string, estado *string, dataInicio, dataFim, dataConclusao *time.Time, boardID *int, projetoID *uuid.UUID) (uuid.UUID, error)
	upsertTarefaFn             func(ctx context.Context, t *repository.UpsertTarefaParams) (uuid.UUID, error)
	autoDetectEquipeBoardIDsFn func(ctx context.Context, fonteDadosID uuid.UUID) (int, error)
}

func (m *mockSyncRepoStore) HasRunningSyncForProject(ctx context.Context, fonteDadosID uuid.UUID, projectKey string) (bool, error) {
	return m.hasRunningSyncForProjectFn(ctx, fonteDadosID, projectKey)
}
func (m *mockSyncRepoStore) CreateSyncLog(ctx context.Context, log *domain.SyncLog) error {
	return m.createSyncLogFn(ctx, log)
}
func (m *mockSyncRepoStore) UpdateSyncLog(ctx context.Context, id uuid.UUID, status string, finalizadoEm time.Time, totals repository.SyncTotals, erros json.RawMessage, mensagem *string) error {
	return m.updateSyncLogFn(ctx, id, status, finalizadoEm, totals, erros, mensagem)
}
func (m *mockSyncRepoStore) GetFonteDadosAtivas(ctx context.Context) ([]domain.FonteDados, error) {
	return m.getFonteDadosAtivasFn(ctx)
}
func (m *mockSyncRepoStore) GetLatestSyncLog(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error) {
	return m.getLatestSyncLogFn(ctx, fonteDadosID)
}
func (m *mockSyncRepoStore) ListSyncLogs(ctx context.Context, fonteDadosID uuid.UUID, limit int) ([]domain.SyncLog, error) {
	return m.listSyncLogsFn(ctx, fonteDadosID, limit)
}
func (m *mockSyncRepoStore) HasRunningSync(ctx context.Context, fonteDadosID uuid.UUID) (bool, error) {
	return m.hasRunningSyncFn(ctx, fonteDadosID)
}
func (m *mockSyncRepoStore) UpdateSyncLogTotals(ctx context.Context, id uuid.UUID, totals repository.SyncTotals) error {
	return m.updateSyncLogTotalsFn(ctx, id, totals)
}
func (m *mockSyncRepoStore) LookupTarefaIDByJiraID(ctx context.Context, fonteDadosID uuid.UUID, jiraID string) (uuid.UUID, error) {
	return m.lookupTarefaIDByJiraIDFn(ctx, fonteDadosID, jiraID)
}
func (m *mockSyncRepoStore) UpdateTarefaParent(ctx context.Context, tarefaID, parentID uuid.UUID) error {
	return m.updateTarefaParentFn(ctx, tarefaID, parentID)
}
func (m *mockSyncRepoStore) UpdateCustomFieldMap(ctx context.Context, fonteID uuid.UUID, cfMap json.RawMessage) error {
	return m.updateCustomFieldMapFn(ctx, fonteID, cfMap)
}
func (m *mockSyncRepoStore) GetProjectKeysForSync(ctx context.Context, fonteDadosID uuid.UUID) ([]string, error) {
	return m.getProjectKeysForSyncFn(ctx, fonteDadosID)
}
func (m *mockSyncRepoStore) UpsertProduto(ctx context.Context, fonteDadosID uuid.UUID, jiraID, nome string, descricao *string, projetoID *uuid.UUID) (uuid.UUID, error) {
	return m.upsertProdutoFn(ctx, fonteDadosID, jiraID, nome, descricao, projetoID)
}
func (m *mockSyncRepoStore) LinkTarefaProduto(ctx context.Context, tarefaID, produtoID uuid.UUID) error {
	return m.linkTarefaProdutoFn(ctx, tarefaID, produtoID)
}
func (m *mockSyncRepoStore) UndeleteReappearedTarefas(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error) {
	return m.undeleteReappearedFn(ctx, fonteDadosID, presentJiraIDs)
}
func (m *mockSyncRepoStore) SoftDeleteAbsentTarefas(ctx context.Context, fonteDadosID uuid.UUID, presentJiraIDs []string) (int64, error) {
	return m.softDeleteAbsentFn(ctx, fonteDadosID, presentJiraIDs)
}
func (m *mockSyncRepoStore) UpsertProjeto(ctx context.Context, fonteDadosID uuid.UUID, jiraID, chave, nome string, descricao *string, leadID *uuid.UUID, categoria *string) (uuid.UUID, error) {
	return m.upsertProjetoFn(ctx, fonteDadosID, jiraID, chave, nome, descricao, leadID, categoria)
}
func (m *mockSyncRepoStore) UpsertMembro(ctx context.Context, fonteDadosID uuid.UUID, jiraAccountID, nome string, email, avatarURL, team *string) (uuid.UUID, error) {
	return m.upsertMembroFn(ctx, fonteDadosID, jiraAccountID, nome, email, avatarURL, team)
}
func (m *mockSyncRepoStore) GetDistinctBoardProjects(ctx context.Context, fonteDadosID uuid.UUID) (map[int]uuid.UUID, error) {
	return m.getDistinctBoardProjectsFn(ctx, fonteDadosID)
}
func (m *mockSyncRepoStore) UpsertSprint(ctx context.Context, fonteDadosID uuid.UUID, jiraID int, nome string, estado *string, dataInicio, dataFim, dataConclusao *time.Time, boardID *int, projetoID *uuid.UUID) (uuid.UUID, error) {
	return m.upsertSprintFn(ctx, fonteDadosID, jiraID, nome, estado, dataInicio, dataFim, dataConclusao, boardID, projetoID)
}
func (m *mockSyncRepoStore) UpsertTarefa(ctx context.Context, t *repository.UpsertTarefaParams) (uuid.UUID, error) {
	return m.upsertTarefaFn(ctx, t)
}
func (m *mockSyncRepoStore) AutoDetectEquipeBoardIDs(ctx context.Context, fonteDadosID uuid.UUID) (int, error) {
	return m.autoDetectEquipeBoardIDsFn(ctx, fonteDadosID)
}

// --- mockAllocationStore ---

type mockAllocationStore struct {
	getEpicsByEquipeAndProdutoFn func(ctx context.Context, equipeID uuid.UUID, produtoNomes []string, statusFilter string) ([]repository.EpicAllocationRow, error)
	checkGDPTCAncestorsFn       func(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	getClosedEpicIDsFn          func(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	getProjectClosureFn         func(ctx context.Context, epicID uuid.UUID) (*repository.ProjectClosureRow, error)
	getEpicByIDFn               func(ctx context.Context, epicID uuid.UUID) (*repository.EpicAllocationRow, error)
	getEpicTasksFn              func(ctx context.Context, epicID uuid.UUID) ([]repository.TaskAllocationRow, error)
	getEpicPeopleFn             func(ctx context.Context, epicID uuid.UUID) ([]repository.PersonAllocationRow, error)
	getTaskPreviousStateFn      func(ctx context.Context, taskID uuid.UUID) (*repository.TaskPreviousState, error)
	updateTaskAllocationFn      func(ctx context.Context, taskID, sprintID uuid.UUID, assigneeID *uuid.UUID, estimateSeconds int) error
	getTaskJiraKeyFn            func(ctx context.Context, taskID uuid.UUID) (string, error)
	getTaskFonteDadosIDFn       func(ctx context.Context, taskID uuid.UUID) (uuid.UUID, error)
	getSprintJiraIDFn           func(ctx context.Context, sprintID uuid.UUID) (int, error)
	rollbackTaskAllocationFn    func(ctx context.Context, taskID uuid.UUID, prev *repository.TaskPreviousState) error
	getFutureSprintsByEquipeFn  func(ctx context.Context, equipeID uuid.UUID) ([]repository.SprintOptionRow, error)
	closeProjectFn              func(ctx context.Context, epicID uuid.UUID, descricao string, dataEncerramento time.Time, encerradoPor string) error
	reopenProjectFn             func(ctx context.Context, epicID uuid.UUID) error
	getProdutosComProjetosAtvFn func(ctx context.Context) ([]repository.ProdutoRow, error)
}

func (m *mockAllocationStore) GetEpicsByEquipeAndProduto(ctx context.Context, equipeID uuid.UUID, produtoNomes []string, statusFilter string) ([]repository.EpicAllocationRow, error) {
	return m.getEpicsByEquipeAndProdutoFn(ctx, equipeID, produtoNomes, statusFilter)
}
func (m *mockAllocationStore) CheckGDPTCAncestors(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	return m.checkGDPTCAncestorsFn(ctx, epicIDs)
}
func (m *mockAllocationStore) GetClosedEpicIDs(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	return m.getClosedEpicIDsFn(ctx, epicIDs)
}
func (m *mockAllocationStore) GetProjectClosure(ctx context.Context, epicID uuid.UUID) (*repository.ProjectClosureRow, error) {
	return m.getProjectClosureFn(ctx, epicID)
}
func (m *mockAllocationStore) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*repository.EpicAllocationRow, error) {
	return m.getEpicByIDFn(ctx, epicID)
}
func (m *mockAllocationStore) GetEpicTasks(ctx context.Context, epicID uuid.UUID) ([]repository.TaskAllocationRow, error) {
	return m.getEpicTasksFn(ctx, epicID)
}
func (m *mockAllocationStore) GetEpicPeople(ctx context.Context, epicID uuid.UUID) ([]repository.PersonAllocationRow, error) {
	return m.getEpicPeopleFn(ctx, epicID)
}
func (m *mockAllocationStore) GetTaskPreviousState(ctx context.Context, taskID uuid.UUID) (*repository.TaskPreviousState, error) {
	return m.getTaskPreviousStateFn(ctx, taskID)
}
func (m *mockAllocationStore) UpdateTaskAllocation(ctx context.Context, taskID, sprintID uuid.UUID, assigneeID *uuid.UUID, estimateSeconds int) error {
	return m.updateTaskAllocationFn(ctx, taskID, sprintID, assigneeID, estimateSeconds)
}
func (m *mockAllocationStore) GetTaskJiraKey(ctx context.Context, taskID uuid.UUID) (string, error) {
	return m.getTaskJiraKeyFn(ctx, taskID)
}
func (m *mockAllocationStore) GetTaskFonteDadosID(ctx context.Context, taskID uuid.UUID) (uuid.UUID, error) {
	return m.getTaskFonteDadosIDFn(ctx, taskID)
}
func (m *mockAllocationStore) GetSprintJiraID(ctx context.Context, sprintID uuid.UUID) (int, error) {
	return m.getSprintJiraIDFn(ctx, sprintID)
}
func (m *mockAllocationStore) RollbackTaskAllocation(ctx context.Context, taskID uuid.UUID, prev *repository.TaskPreviousState) error {
	return m.rollbackTaskAllocationFn(ctx, taskID, prev)
}
func (m *mockAllocationStore) GetFutureSprintsByEquipe(ctx context.Context, equipeID uuid.UUID) ([]repository.SprintOptionRow, error) {
	return m.getFutureSprintsByEquipeFn(ctx, equipeID)
}
func (m *mockAllocationStore) CloseProject(ctx context.Context, epicID uuid.UUID, descricao string, dataEncerramento time.Time, encerradoPor string) error {
	return m.closeProjectFn(ctx, epicID, descricao, dataEncerramento, encerradoPor)
}
func (m *mockAllocationStore) ReopenProject(ctx context.Context, epicID uuid.UUID) error {
	return m.reopenProjectFn(ctx, epicID)
}
func (m *mockAllocationStore) GetProdutosComProjetosAtivos(ctx context.Context) ([]repository.ProdutoRow, error) {
	return m.getProdutosComProjetosAtvFn(ctx)
}

// --- mockReviewStore ---

type mockReviewStore struct {
	getReviewAnaliseFn      func(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*repository.ReviewAnalise, error)
	getSprintEstadoFn       func(ctx context.Context, sprintID uuid.UUID) (*string, error)
	getSprintSnapshotFn     func(ctx context.Context, sprintID uuid.UUID) ([]repository.ReviewTaskRow, error)
	getReviewTasksFn        func(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, produtoIDs []uuid.UUID) ([]repository.ReviewTaskRow, error)
	getGDPTCAncestorIDsFn   func(ctx context.Context, taskIDs []uuid.UUID) ([]uuid.UUID, error)
	getReviewPOsFn          func(ctx context.Context, equipeID uuid.UUID, produtoIDs []uuid.UUID) ([]repository.ReviewPO, error)
	listDestaquesFn         func(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error)
	createDestaqueFn        func(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error)
	updateDestaqueFn        func(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error)
	deleteDestaqueFn        func(ctx context.Context, id uuid.UUID) error
	saveReviewAnaliseFn     func(ctx context.Context, a repository.ReviewAnalise) error
}

func (m *mockReviewStore) GetReviewAnalise(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*repository.ReviewAnalise, error) {
	return m.getReviewAnaliseFn(ctx, sprintID, equipeID, produtoIDs)
}
func (m *mockReviewStore) GetSprintEstado(ctx context.Context, sprintID uuid.UUID) (*string, error) {
	return m.getSprintEstadoFn(ctx, sprintID)
}
func (m *mockReviewStore) GetSprintSnapshot(ctx context.Context, sprintID uuid.UUID) ([]repository.ReviewTaskRow, error) {
	return m.getSprintSnapshotFn(ctx, sprintID)
}
func (m *mockReviewStore) GetReviewTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, produtoIDs []uuid.UUID) ([]repository.ReviewTaskRow, error) {
	return m.getReviewTasksFn(ctx, sprintID, equipeID, produtoIDs)
}
func (m *mockReviewStore) GetGDPTCAncestorTaskIDs(ctx context.Context, taskIDs []uuid.UUID) ([]uuid.UUID, error) {
	return m.getGDPTCAncestorIDsFn(ctx, taskIDs)
}
func (m *mockReviewStore) GetReviewPOs(ctx context.Context, equipeID uuid.UUID, produtoIDs []uuid.UUID) ([]repository.ReviewPO, error) {
	return m.getReviewPOsFn(ctx, equipeID, produtoIDs)
}
func (m *mockReviewStore) ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error) {
	return m.listDestaquesFn(ctx, sprintID, equipeID)
}
func (m *mockReviewStore) CreateDestaque(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error) {
	return m.createDestaqueFn(ctx, d)
}
func (m *mockReviewStore) UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error) {
	return m.updateDestaqueFn(ctx, id, titulo, descricao, link)
}
func (m *mockReviewStore) DeleteDestaque(ctx context.Context, id uuid.UUID) error {
	return m.deleteDestaqueFn(ctx, id)
}
func (m *mockReviewStore) SaveReviewAnalise(ctx context.Context, a repository.ReviewAnalise) error {
	return m.saveReviewAnaliseFn(ctx, a)
}

// --- mockConfigStore ---

type mockConfigStore struct {
	getConfigFn func(ctx context.Context, chave string) (string, error)
}

func (m *mockConfigStore) GetConfig(ctx context.Context, chave string) (string, error) {
	return m.getConfigFn(ctx, chave)
}

// --- mockEquipeStore ---

type mockEquipeStore struct {
	getMembrosEquipeFn func(ctx context.Context, equipeID uuid.UUID) ([]domain.Membro, error)
}

func (m *mockEquipeStore) GetMembrosEquipe(ctx context.Context, equipeID uuid.UUID) ([]domain.Membro, error) {
	return m.getMembrosEquipeFn(ctx, equipeID)
}

// --- mockDestinatarioStore ---

type mockDestinatarioStore struct {
	getByIDsFn func(ctx context.Context, ids []uuid.UUID) ([]repository.Destinatario, error)
}

func (m *mockDestinatarioStore) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]repository.Destinatario, error) {
	return m.getByIDsFn(ctx, ids)
}

// --- mockSyncScheduleStore ---

type mockSyncScheduleStore struct {
	getDueSchedulesFn func(ctx context.Context, horaMinuto string) ([]domain.SyncSchedule, error)
}

func (m *mockSyncScheduleStore) GetDueSchedules(ctx context.Context, horaMinuto string) ([]domain.SyncSchedule, error) {
	return m.getDueSchedulesFn(ctx, horaMinuto)
}
```

- [ ] **Step 2: Verify test files compile**

Run: `go test ./internal/service/ -run NONE -count=1`
Expected: `ok` (compiles, runs zero tests)

- [ ] **Step 3: Commit**

```bash
git add internal/service/mocks_test.go
git commit -m "feat: add mock structs for all 9 repository interfaces"
```

---

### Task 3: Refactor Service Structs and Constructors

**Files:**
- Modify: `internal/service/sprint.go:94-100`
- Modify: `internal/service/sync.go:34-54`
- Modify: `internal/service/allocation.go:109-135`
- Modify: `internal/service/equalizer.go:111-140`
- Modify: `internal/service/sprint_generation.go:16-44`
- Modify: `internal/service/review.go:16-27`
- Modify: `internal/service/notification.go:14-30`
- Modify: `internal/service/email.go:16-25`
- Modify: `internal/service/whatsapp.go:16-27`
- Modify: `internal/service/scheduler.go:14-30`

**Interfaces:**
- Consumes: all 9 interfaces from Task 1
- Produces: refactored service structs that accept interfaces — enabling mock injection in tests. `cmd/api/main.go` requires zero changes (concrete repos satisfy interfaces implicitly).

**Important:** Do NOT modify `cmd/api/main.go`. Do NOT modify any file in `internal/repository/`. Only change the struct field types and constructor parameter types in service files.

For each service below, apply two changes: (1) change struct field types from concrete to interface, (2) change constructor parameter types to match.

- [ ] **Step 1: Refactor SprintService** (`sprint.go`)

BEFORE (line 94):
```go
type SprintService struct {
	repo   *repository.SprintRepository
	logger *zap.Logger
}

func NewSprintService(repo *repository.SprintRepository, logger *zap.Logger) *SprintService {
	return &SprintService{repo: repo, logger: logger}
}
```

AFTER:
```go
type SprintService struct {
	repo   SprintRepoStore
	logger *zap.Logger
}

func NewSprintService(repo SprintRepoStore, logger *zap.Logger) *SprintService {
	return &SprintService{repo: repo, logger: logger}
}
```

Also remove `"github.com/emersonpaula83/myplanner/backend/internal/repository"` from imports ONLY IF no other code in `sprint.go` references the `repository` package. (It does — many types like `repository.SprintListItem` are used, so the import stays.)

- [ ] **Step 2: Refactor SyncService** (`sync.go`)

BEFORE (line 34):
```go
type SyncService struct {
	repo               *repository.SyncRepository
	fdRepo             *repository.FonteDadosRepository
	clientFactory      ClientFactory
	oauthClientFactory OAuthClientFactory
	oauthSvc           *jira.OAuthService
	rateLimit          int
	logger             *zap.Logger
}

func NewSyncService(repo *repository.SyncRepository, fdRepo *repository.FonteDadosRepository, clientFactory ClientFactory, oauthClientFactory OAuthClientFactory, oauthSvc *jira.OAuthService, rateLimit int, logger *zap.Logger) *SyncService {
```

AFTER:
```go
type SyncService struct {
	repo               SyncRepoStore
	fdRepo             FonteDadosStore
	clientFactory      ClientFactory
	oauthClientFactory OAuthClientFactory
	oauthSvc           *jira.OAuthService
	rateLimit          int
	logger             *zap.Logger
}

func NewSyncService(repo SyncRepoStore, fdRepo FonteDadosStore, clientFactory ClientFactory, oauthClientFactory OAuthClientFactory, oauthSvc *jira.OAuthService, rateLimit int, logger *zap.Logger) *SyncService {
```

- [ ] **Step 3: Refactor AllocationService** (`allocation.go`)

BEFORE (line 109):
```go
type AllocationService struct {
	repo           *repository.AllocationRepository
	sprintSvc      *SprintService
	sprintRepo     *repository.SprintRepository
	fdRepo         *repository.FonteDadosRepository
	syncSvc        *SyncService
	clientFactory  ClientFactory
	oauthFactory   OAuthClientFactory
	oauthSvc       *jira.OAuthService
	rateLimit      int
	logger         *zap.Logger
}
```

AFTER — change `repo`, `sprintRepo`, `fdRepo` to interfaces; keep `sprintSvc` and `syncSvc` as concrete (inter-service deps):
```go
type AllocationService struct {
	repo           AllocationStore
	sprintSvc      *SprintService
	sprintRepo     SprintRepoStore
	fdRepo         FonteDadosStore
	syncSvc        *SyncService
	clientFactory  ClientFactory
	oauthFactory   OAuthClientFactory
	oauthSvc       *jira.OAuthService
	rateLimit      int
	logger         *zap.Logger
}
```

Update `NewAllocationService` parameter types correspondingly: `repo AllocationStore`, `sprintRepo SprintRepoStore`, `fdRepo FonteDadosStore`.

- [ ] **Step 4: Refactor EqualizerService** (`equalizer.go`)

BEFORE (line 111):
```go
type EqualizerService struct {
	sprintSvc          *SprintService
	sprintRepo         *repository.SprintRepository
	fdRepo             *repository.FonteDadosRepository
	configRepo         *repository.ConfigRepository
	clientFactory      ClientFactory
	oauthClientFactory OAuthClientFactory
	oauthSvc           *jira.OAuthService
	rateLimit          int
	logger             *zap.Logger
}
```

AFTER — change `fdRepo` and `configRepo` to interfaces. **Keep `sprintRepo` as concrete** `*repository.SprintRepository` (it calls `Pool().QueryRow(...)` which can't be interfaced without repo changes):
```go
type EqualizerService struct {
	sprintSvc          *SprintService
	sprintRepo         *repository.SprintRepository
	fdRepo             FonteDadosStore
	configRepo         ConfigStore
	clientFactory      ClientFactory
	oauthClientFactory OAuthClientFactory
	oauthSvc           *jira.OAuthService
	rateLimit          int
	logger             *zap.Logger
}
```

Update `NewEqualizerService` parameter types: `fdRepo FonteDadosStore`, `configRepo ConfigStore`. Keep `sprintRepo *repository.SprintRepository`.

- [ ] **Step 5: Refactor SprintGenerationService** (`sprint_generation.go`)

BEFORE (line 16):
```go
type SprintGenerationService struct {
	fdRepo             *repository.FonteDadosRepository
	equipeRepo         *repository.EquipeRepository
	syncRepo           *repository.SyncRepository
	sprintRepo         *repository.SprintRepository
	clientFactory      ClientFactory
	oauthClientFactory OAuthClientFactory
	oauthSvc           *jira.OAuthService
	rateLimit          int
	logger             *zap.Logger
}
```

AFTER:
```go
type SprintGenerationService struct {
	fdRepo             FonteDadosStore
	equipeRepo         EquipeStore
	syncRepo           SyncRepoStore
	sprintRepo         SprintRepoStore
	clientFactory      ClientFactory
	oauthClientFactory OAuthClientFactory
	oauthSvc           *jira.OAuthService
	rateLimit          int
	logger             *zap.Logger
}
```

Update `NewSprintGenerationService` parameter types correspondingly.

- [ ] **Step 6: Refactor ReviewService** (`review.go`)

BEFORE (line 16):
```go
type ReviewService struct {
	repo       *repository.ReviewRepository
	configRepo *repository.ConfigRepository
	logger     *zap.Logger
}

func NewReviewService(repo *repository.ReviewRepository, configRepo *repository.ConfigRepository, logger *zap.Logger) *ReviewService {
```

AFTER:
```go
type ReviewService struct {
	repo       ReviewStore
	configRepo ConfigStore
	logger     *zap.Logger
}

func NewReviewService(repo ReviewStore, configRepo ConfigStore, logger *zap.Logger) *ReviewService {
```

- [ ] **Step 7: Refactor NotificationService** (`notification.go`)

BEFORE (line 14):
```go
type NotificationService struct {
	reviewSvc    *ReviewService
	destRepo     *repository.DestinatarioRepository
	sprintRepo   *repository.SprintRepository
	emailProv    *EmailProvider
	whatsappProv *WhatsAppProvider
	logger       *zap.Logger
}
```

AFTER — change `destRepo` and `sprintRepo` to interfaces; keep `reviewSvc`, `emailProv`, `whatsappProv` as concrete:
```go
type NotificationService struct {
	reviewSvc    *ReviewService
	destRepo     DestinatarioStore
	sprintRepo   SprintRepoStore
	emailProv    *EmailProvider
	whatsappProv *WhatsAppProvider
	logger       *zap.Logger
}
```

Update `NewNotificationService` parameter types: `destRepo DestinatarioStore`, `sprintRepo SprintRepoStore`.

- [ ] **Step 8: Refactor EmailProvider** (`email.go`)

BEFORE (line 16):
```go
type EmailProvider struct {
	configRepo *repository.ConfigRepository
	logger     *zap.Logger
}

func NewEmailProvider(configRepo *repository.ConfigRepository, logger *zap.Logger) *EmailProvider {
```

AFTER:
```go
type EmailProvider struct {
	configRepo ConfigStore
	logger     *zap.Logger
}

func NewEmailProvider(configRepo ConfigStore, logger *zap.Logger) *EmailProvider {
```

- [ ] **Step 9: Refactor WhatsAppProvider** (`whatsapp.go`)

BEFORE (line 16):
```go
type WhatsAppProvider struct {
	configRepo *repository.ConfigRepository
	httpClient *http.Client
	logger     *zap.Logger
}

func NewWhatsAppProvider(configRepo *repository.ConfigRepository, logger *zap.Logger) *WhatsAppProvider {
```

AFTER:
```go
type WhatsAppProvider struct {
	configRepo ConfigStore
	httpClient *http.Client
	logger     *zap.Logger
}

func NewWhatsAppProvider(configRepo ConfigStore, logger *zap.Logger) *WhatsAppProvider {
```

- [ ] **Step 10: Refactor SchedulerService** (`scheduler.go`)

BEFORE (line 14):
```go
type SchedulerService struct {
	syncSvc      *SyncService
	scheduleRepo *repository.SyncScheduleRepository
	logger       *zap.Logger
	mu           sync.Mutex
	lastFired    map[uuid.UUID]string
}

func NewSchedulerService(syncSvc *SyncService, scheduleRepo *repository.SyncScheduleRepository, logger *zap.Logger) *SchedulerService {
```

AFTER — change `scheduleRepo` to interface; keep `syncSvc` as concrete:
```go
type SchedulerService struct {
	syncSvc      *SyncService
	scheduleRepo SyncScheduleStore
	logger       *zap.Logger
	mu           sync.Mutex
	lastFired    map[uuid.UUID]string
}

func NewSchedulerService(syncSvc *SyncService, scheduleRepo SyncScheduleStore, logger *zap.Logger) *SchedulerService {
```

- [ ] **Step 11: Verify build**

Run: `go build ./...`
Expected: PASS — concrete repo types satisfy all new interfaces implicitly. `main.go` passes concrete repos to constructors that now accept interfaces.

- [ ] **Step 12: Verify all tests pass**

Run: `go test ./... -count=1`
Expected: all packages PASS (existing tests unchanged, mocks compile)

- [ ] **Step 13: Commit**

```bash
git add internal/service/sprint.go internal/service/sync.go internal/service/allocation.go internal/service/equalizer.go internal/service/sprint_generation.go internal/service/review.go internal/service/notification.go internal/service/email.go internal/service/whatsapp.go internal/service/scheduler.go
git commit -m "refactor: service structs accept interfaces instead of concrete repository types"
```

---

### Task 4: Coverage Tooling and .gitignore

**Files:**
- Create: `Makefile`
- Create: `scripts/check-coverage.sh`
- Create: `.gitignore`

**Interfaces:**
- Consumes: nothing from prior tasks
- Produces: `make test`, `make test-cover`, `make test-cover-html` targets; `scripts/check-coverage.sh` for CI enforcement

- [ ] **Step 1: Create `.gitignore`**

```
coverage.out
coverage.html
```

- [ ] **Step 2: Create `Makefile`**

```makefile
.PHONY: test test-cover test-cover-html

test:
	go test ./internal/...

test-cover:
	go test ./internal/... -coverprofile=coverage.out
	@go tool cover -func=coverage.out | grep total | awk '{print $$3}'
	@echo "---"
	@go tool cover -func=coverage.out | grep -E "^github" | awk '{split($$1,a,"/"); pkg=a[length(a)-1]"/"a[length(a)]; print pkg, $$NF}' | sort -t'.' -k1 -rn

test-cover-html: test-cover
	go tool cover -html=coverage.out -o coverage.html
	@echo "Open coverage.html in browser"
```

- [ ] **Step 3: Create `scripts/check-coverage.sh`**

```bash
#!/bin/bash
MIN_TOTAL=70.0
EXCLUDE="repository"

TOTAL=$(go test ./internal/... -coverprofile=coverage.out 2>/dev/null \
  | grep -v "/$EXCLUDE" \
  | grep "coverage:" \
  | awk '{sum+=$2; n++} END {printf "%.1f", sum/n}')

echo "Coverage: ${TOTAL}%"
if (( $(echo "$TOTAL < $MIN_TOTAL" | bc -l) )); then
  echo "FAIL: coverage ${TOTAL}% < minimum ${MIN_TOTAL}%"
  exit 1
fi
echo "PASS"
```

- [ ] **Step 4: Make script executable**

Run: `chmod +x scripts/check-coverage.sh`

- [ ] **Step 5: Verify make test**

Run: `make test`
Expected: all packages PASS

- [ ] **Step 6: Verify make test-cover**

Run: `make test-cover`
Expected: prints per-package coverage percentages

- [ ] **Step 7: Commit**

```bash
git add .gitignore Makefile scripts/check-coverage.sh
git commit -m "feat: add Makefile with coverage targets and threshold script"
```
