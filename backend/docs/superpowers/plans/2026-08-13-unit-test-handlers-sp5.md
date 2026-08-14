# SP5: Handler Unit Tests — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Achieve ~70% unit test coverage for `internal/handler/` package (currently 21.3%).

**Architecture:** Handler tests follow a uniform pattern: create function-field mock, build `httptest` request with chi route context, call handler method, assert response status + JSON body. Two phases: (1) test handlers already using interfaces, (2) extract interfaces for concrete-dep handlers + test.

**Tech Stack:** Go stdlib `testing`, `net/http/httptest`, `github.com/go-chi/chi/v5`, `go.uber.org/zap` (nop logger), function-field mocks

## Global Constraints

- Framework: stdlib `testing` only — no testify, gomock, or external test deps
- Mock pattern: function-field injection (dominant pattern in handler tests)
- Chi route context: use `chi.NewRouteContext()` + `chi.WithRouteContext()` for URL params
- Logger: `zap.NewNop()` in all tests
- No commits until everything passes 100%
- Coverage target: ≥70% for handler package overall
- Each handler method needs at minimum: happy path + error path tests

---

### Task 1: Sprint + Checkpoint + Feriado + FonteDados handler tests

**Files:**
- Create: `internal/handler/sprint_test.go`
- Create: `internal/handler/checkpoint_test.go`
- Create: `internal/handler/feriado_test.go`
- Create: `internal/handler/fonte_dados_test.go`

**Interfaces:** All already defined in respective handler files (SprintStore, CheckpointStore, FeriadoStore, FonteDadosStore).

**Context:** These 4 handlers are CRUD-like with interfaces already defined. Total ~361 stmts. Sprint is biggest (169 stmts, 9 methods). The others are simpler CRUD.

- [ ] **Step 1: Create sprint_test.go**

Mock struct for SprintStore (9 methods). Tests for all 9 handler methods:
- ListProjetos: happy path (returns projects) + store error
- ListSprints: happy path + error
- ListByProjeto: happy path + error
- GetCapacity: happy path + error + missing sprintID param
- GetUnplanned: happy path + error
- GetDisclaimerTasks: happy path + error
- GetBurndown: happy path + error
- GetSprintsTimeline: happy path + error + missing equipeID
- GetTimelineDetail: happy path + error

Chi route params: use `chi.NewRouteContext()`, set `rctx.URLParams.Add("sprintID", id.String())`, wrap request context with `chi.WithRouteContext(r.Context(), rctx)`.

- [ ] **Step 2: Create checkpoint_test.go**

Mock struct for CheckpointStore (3 methods: List, Create, Delete). Tests for List, Create, Delete — happy + error paths.

- [ ] **Step 3: Create feriado_test.go**

Mock struct for FeriadoStore (3 methods). Tests for List, Create, Delete — happy + error paths.

- [ ] **Step 4: Create fonte_dados_test.go**

Mock struct for FonteDadosStore (5 methods). Tests for List, GetByID, Create, Update, Delete — happy + error paths.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/handler/ -run "TestSprint|TestCheckpoint|TestFeriado|TestFonteDados" -v`
Expected: all PASS

---

### Task 2: Membro + Investimento handler tests

**Files:**
- Create: `internal/handler/membro_test.go`
- Create: `internal/handler/investimento_test.go`

**Interfaces:** MembroStore (9 methods), InvestimentoStore (3 methods), MembroFinanceiroStore (5 methods) — all in handler files.

- [ ] **Step 1: Create membro_test.go**

Mock struct for MembroStore. Tests for: List, Search, GetByID, CreateDisponibilidade, UpdateDisponibilidade, DeleteDisponibilidade, UpdateDataDesligamento. Note: MembroStore uses `pgtype.Date` — import `github.com/jackc/pgx/v5/pgtype`.

- [ ] **Step 2: Create investimento_test.go**

Mock structs for InvestimentoStore + MembroFinanceiroStore. Tests for: GetDashboard, GetGastosMensais, UpdateSalario, UpdateBancoHoras, UpdateDataAdmissao, GetHistoricoSalario, GetHistoricoBancoHoras, GetAlocacoesProjetos.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/handler/ -run "TestMembro|TestInvestimento" -v`
Expected: all PASS

---

### Task 3: Extract interfaces for concrete-dep handlers + tests

**Files:**
- Modify: `internal/handler/allocation.go` (add interface)
- Create: `internal/handler/allocation_test.go`
- Modify: `internal/handler/sync_schedule.go` (add interface)
- Create: `internal/handler/sync_schedule_test.go`
- Modify: `internal/handler/tarefa.go` (add interface)
- Create: `internal/handler/tarefa_test.go`
- Modify: `internal/handler/sprint_generation.go` (add interface)
- Create: `internal/handler/sprint_generation_test.go`
- Modify: `internal/handler/equalizer.go` (add interface)
- Create: `internal/handler/equalizer_test.go`
- Modify: `internal/handler/notification.go` (add interface)
- Create: `internal/handler/notification_test.go`

**Context:** Each handler uses concrete `*service.XxxService` or `*repository.XxxRepository`. Extract the methods actually called into a local interface, change the struct field type, test with function-field mocks. The concrete types already implement the interfaces — zero change in main.go/routes.

Pattern for each handler:
1. Find all methods called on the concrete dep in handler code
2. Define interface with those methods
3. Change struct field type to interface
4. Update constructor signature to accept interface
5. Create mock struct + tests

- [ ] **Step 1: allocation.go — extract AllocationServiceInterface + test**

Methods called on svc: ListProjectAllocations, GetProjectDetail, AllocateTask, SyncProjectTasks, GetAvailableSprints, CloseProject, ReopenProject, GetFilteredProducts. Define interface, change svc field, create allocation_test.go.

- [ ] **Step 2: sync_schedule.go — extract SyncScheduleStore + test**

Methods called on repo: GetByFonteDadosID, Upsert, Delete, ToggleActive. Define interface, change field, create sync_schedule_test.go.

- [ ] **Step 3: tarefa.go — extract TarefaStore + test**

Methods called on repo: ListBySprintAndEquipe, Delete. Define interface, change field, create tarefa_test.go.

- [ ] **Step 4: sprint_generation.go — extract SprintGenService + test**

Methods called on svc: GetBoardsForEquipe, PreviewSprints, GenerateSprints. Define interface, change svc field, create sprint_generation_test.go. Also test unexported validateRequest (pure function).

- [ ] **Step 5: equalizer.go — extract EqualizerService interface + test**

Methods called on svc: Calculate, Apply. Define interface, change field, create equalizer_test.go.

- [ ] **Step 6: notification.go — extract DestinatarioStore + NotifService + test**

Methods called: destRepo.ListByFonteDados, destRepo.Create, destRepo.Delete + notifSvc.EnviarReview. Define 2 interfaces, change fields, create notification_test.go.

- [ ] **Step 7: Run all tests**

Run: `go test ./internal/handler/ -count=1 -v`
Expected: all PASS, no regressions

---

### Task 4: Boost existing handler tests + coverage check

**Files:**
- Modify: existing test files as needed

**Context:** Existing tests have gaps. Check coverage and add targeted tests to boost:
- equipe.go: 26.8% → boost
- review.go: 31.1% → boost
- usuario.go: 34.8% → boost
- timeline.go: 46.3% → boost
- auth.go: 61.3% → minor boost
- sync.go: 60.5% → minor boost

- [ ] **Step 1: Run coverage**

```bash
go test ./internal/handler/ -coverprofile=handler-cov.out && go tool cover -func=handler-cov.out | grep -E "\.go:" | sort -t: -k1
```

- [ ] **Step 2: Add boosters for uncovered handler methods**

- [ ] **Step 3: Final verification**

Run: `go test ./internal/handler/ -v -count=1`
Expected: all PASS, handler coverage ≥ 70%
