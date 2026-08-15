# SP4: Remaining Services Unit Tests — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Achieve ≥70% unit test coverage for `review.go`, `equalizer.go`, `sprint_generation.go`, `notification.go`, `scheduler.go` (~1,784 lines total).

**Architecture:** Focus on testable code — pure functions, pass-through methods, and logic paths that don't require external APIs (OpenRouter, Jira). Services that use concrete repo types (EqualizerService uses `*repository.SprintRepository`) get pure-function-only tests. Notification/Scheduler get limited coverage due to concrete provider dependencies.

**Tech Stack:** Go stdlib `testing`, `go.uber.org/zap` (nop logger), function-field mocks from `mocks_test.go`

## Global Constraints

- Framework: stdlib `testing` only — no testify, gomock, or external test deps
- Mock pattern: function-field injection per test (matches existing mocks in `mocks_test.go`)
- No commits until everything passes 100%
- Coverage target: ≥70% where mockable, best-effort for concrete-dependency services
- Logger: use `zap.NewNop()` in all tests

---

### Task 1: ReviewService — GetAnalise + Pass-Throughs + GetReviewData

**Files:**
- Create: `internal/service/review_test.go`

**Interfaces:**
- Consumes: `mockReviewStore`, `mockConfigStore` from `mocks_test.go`
- Produces: `TestNewReviewService`, `TestGetAnalise`, `TestListDestaques`, `TestCreateDestaque`, `TestUpdateDestaque`, `TestDeleteDestaque`, `TestGetReviewData`, `TestFilterSnapshotTasks`, `TestBuildReviewAnalisePrompt`

**Context:** ReviewService uses `ReviewStore` and `ConfigStore` interfaces — both mockable. GetReviewData (line 97-288) is the complex method: calls GetSprintEstado, conditionally GetSprintSnapshot or GetReviewTasks, collects GDPTC ancestors, computes stats/charts. Test with varied task statuses and types to exercise classification branches.

- [ ] **Step 1: Create review_test.go with imports and pass-through tests**

Test NewReviewService, GetAnalise (success + error), ListDestaques, CreateDestaque, UpdateDestaque, DeleteDestaque — all pass-throughs to mockReviewStore.

- [ ] **Step 2: Write TestGetReviewData**

Subtests:
- `GetReviewTasks error` — returns error
- `happy path with varied tasks` — tasks with different statuses (Concluído, Desenvolvimento, Backlog), types (Bug, Melhoria, História), planejada vs nao_planejada, with/without ParentID, with/without produtos. Verify stats counts, graficoCategorias, graficoPlanejamento, tarefaList classification.
- `closed sprint uses snapshot` — GetSprintEstado returns "closed", GetSprintSnapshot returns tasks → verify uses snapshot tasks

Mock setup for GetReviewData:
```go
repo := &mockReviewStore{
    getSprintEstadoFn: func(...) (*string, error) { return nil, nil },
    getReviewTasksFn: func(...) ([]repository.ReviewTaskRow, error) { return tasks, nil },
    getGDPTCAncestorIDsFn: func(...) ([]uuid.UUID, error) { return nil, nil },
    getReviewPOsFn: func(...) ([]repository.ReviewPO, error) { return pos, nil },
}
```

- [ ] **Step 3: Write TestFilterSnapshotTasks**

Pure function — test with empty produtoIDs (returns all), with produtoIDs (filters matching).

- [ ] **Step 4: Write TestBuildReviewAnalisePrompt**

Pure function — verify returns non-empty system and user prompts, user prompt contains JSON.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/service/ -run "TestNewReviewService|TestGetAnalise|TestListDestaques|TestCreateDestaque|TestUpdateDestaque|TestDeleteDestaque|TestGetReviewData|TestFilterSnapshotTasks|TestBuildReviewAnalisePrompt" -v`
Expected: all PASS

---

### Task 2: SprintGeneration — Pure Functions + getFonteDadosForEquipe

**Files:**
- Modify: `internal/service/sprint_generation_test.go`

**Interfaces:**
- Consumes: `mockEquipeStore`, `mockFonteDadosStore` from `mocks_test.go`
- Produces: `TestNextMonday`, `TestFormatSprintName`, `TestFilterExistingSlots`, `TestParseJiraDate`, `TestModeString`, `TestModeInt`, `TestGetFonteDadosForEquipe`

**Context:** sprint_generation_test.go already has 10 tests covering detectSprintPattern, generateSprintSlots, previewSprintsWithClient. Add tests for remaining pure functions + getFonteDadosForEquipe which uses mockable EquipeStore.

- [ ] **Step 1: Write TestNextMonday**

```go
func TestNextMonday(t *testing.T) {
    tests := []struct{ input, want time.Time }{
        {time.Date(2026, 8, 3, 0, 0, 0, 0, saoPaulo), time.Date(2026, 8, 3, 0, 0, 0, 0, saoPaulo)}, // already Monday
        {time.Date(2026, 8, 5, 0, 0, 0, 0, saoPaulo), time.Date(2026, 8, 10, 0, 0, 0, 0, saoPaulo)}, // Wednesday → next Monday
        {time.Date(2026, 8, 2, 0, 0, 0, 0, saoPaulo), time.Date(2026, 8, 3, 0, 0, 0, 0, saoPaulo)},  // Sunday → next Monday
    }
    for _, tc := range tests { ... }
}
```

- [ ] **Step 2: Write TestFormatSprintName, TestParseJiraDate, TestModeString, TestModeInt**

All pure functions, simple table-driven tests.

- [ ] **Step 3: Write TestFilterExistingSlots**

Test with overlapping and non-overlapping sprints.

- [ ] **Step 4: Write TestGetFonteDadosForEquipe**

Test error from GetMembrosEquipe, empty members (error), happy path returns first member's FonteDadosID.

SprintGenerationService constructor: `svc := &SprintGenerationService{equipeRepo: mockEquipeStore, logger: zap.NewNop()}`

- [ ] **Step 5: Run tests**

Run: `go test ./internal/service/ -run "TestNextMonday|TestFormatSprintName|TestFilterExistingSlots|TestParseJiraDate|TestModeString|TestModeInt|TestGetFonteDadosForEquipe" -v`
Expected: all PASS

---

### Task 3: Equalizer — Pure Functions + extractAnaliseText + Scheduler

**Files:**
- Create: `internal/service/equalizer_test.go`
- Create: `internal/service/notification_test.go`
- Create: `internal/service/scheduler_test.go`

**Interfaces:**
- Consumes: `mockSyncScheduleStore` from `mocks_test.go`
- Produces: `TestCalcStdDev`, `TestBuildEqualizerPrompt`, `TestFirstNonEmpty`, `TestResetHorasMov`, `TestExtractAnaliseText`, `TestSchedulerTick`, `TestCleanLastFired`

**Context:**
- EqualizerService uses concrete `*repository.SprintRepository` — can't mock Calculate/Apply end-to-end. Test pure functions only: calcStdDev, buildEqualizerPrompt, firstNonEmpty, resetHorasMov.
- NotificationService depends on concrete ReviewService + EmailProvider + WhatsAppProvider — test extractAnaliseText (pure function) only.
- SchedulerService: tick uses time.Now() and syncSvc — test cleanLastFired (resets map). tick can be tested with mockSyncScheduleStore returning empty schedules (early return) and schedules with project keys (fires goroutines). Need concrete SyncService for full tick test — test only the GetDueSchedules-error and empty-schedules paths.

- [ ] **Step 1: Write equalizer_test.go**

```go
func TestCalcStdDev(t *testing.T) {
    tests := []struct{values []float64; want float64}{
        {nil, 0},
        {[]float64{5, 5, 5}, 0},
        {[]float64{2, 4, 4, 4, 5, 5, 7, 9}, ≈2.0}, // known stddev
    }
}

func TestFirstNonEmpty(t *testing.T) { ... }

func TestResetHorasMov(t *testing.T) {
    // verify horasMov zeroed, role preserved
}

func TestBuildEqualizerPrompt(t *testing.T) {
    // verify returns non-empty system+user prompts, user contains JSON
}
```

- [ ] **Step 2: Write notification_test.go**

```go
func TestExtractAnaliseText(t *testing.T) {
    t.Run("valid JSON", func(t *testing.T) {
        raw := json.RawMessage(`{"analises_por_produto":[{"produto":"Prod A","foco_sprint":{"descricao":"Focus"},"top3_entregas":[{"ticket":"T-1","resumo":"Task"}]}]}`)
        result := extractAnaliseText(raw)
        if !strings.Contains(result, "Prod A") { t.Error("missing produto") }
        if !strings.Contains(result, "T-1") { t.Error("missing ticket") }
    })
    t.Run("invalid JSON returns raw", func(t *testing.T) {
        raw := json.RawMessage(`not json`)
        result := extractAnaliseText(raw)
        if result != "not json" { t.Errorf("expected raw string") }
    })
}
```

- [ ] **Step 3: Write scheduler_test.go**

```go
func TestNewSchedulerService(t *testing.T) { ... }

func TestCleanLastFired(t *testing.T) {
    svc := &SchedulerService{lastFired: map[uuid.UUID]string{uuid.New(): "10:00"}}
    svc.cleanLastFired()
    if len(svc.lastFired) != 0 { t.Error("expected empty") }
}

func TestTick_EmptySchedules(t *testing.T) {
    repo := &mockSyncScheduleStore{
        getDueSchedulesFn: func(ctx context.Context, hora string) ([]domain.SyncSchedule, error) {
            return nil, nil
        },
    }
    svc := &SchedulerService{scheduleRepo: repo, logger: zap.NewNop(), lastFired: make(map[uuid.UUID]string)}
    svc.tick(context.Background()) // should not panic
}

func TestTick_RepoError(t *testing.T) {
    repo := &mockSyncScheduleStore{
        getDueSchedulesFn: func(ctx context.Context, hora string) ([]domain.SyncSchedule, error) {
            return nil, fmt.Errorf("db error")
        },
    }
    svc := &SchedulerService{scheduleRepo: repo, logger: zap.NewNop(), lastFired: make(map[uuid.UUID]string)}
    svc.tick(context.Background()) // should not panic, logs error
}
```

- [ ] **Step 4: Run all tests**

Run: `go test ./internal/service/ -run "TestCalcStdDev|TestBuildEqualizerPrompt|TestFirstNonEmpty|TestResetHorasMov|TestExtractAnaliseText|TestCleanLastFired|TestTick|TestNewSchedulerService" -v`
Expected: all PASS

---

### Task 4: Coverage Check + Boosters

**Files:**
- Modify: any of the test files from Tasks 1-3

**Context:** After Tasks 1-3, check coverage. If any file is below 70% (where mockable), add targeted tests for uncovered branches. Focus on:
- review.go: GetReviewData edge cases (empty tasks, closed sprint with produtoID filter)
- sprint_generation.go: should already be high with existing + new tests
- equalizer.go/notification.go/scheduler.go: accept lower coverage due to concrete deps

- [ ] **Step 1: Run coverage**

```bash
go test ./internal/service/ -coverprofile=coverage.out && go tool cover -func=coverage.out | grep -E "review\.go|equalizer\.go|sprint_generation\.go|notification\.go|scheduler\.go"
```

- [ ] **Step 2: Add boosters if needed**

Target uncovered branches in mockable methods.

- [ ] **Step 3: Final verification**

Run: `go test ./internal/service/ -v -count=1`
Expected: all PASS, no regressions
