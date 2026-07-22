# Scheduled Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow users to configure per-fonte-de-dados scheduled sync of selected JIRA projects at specific times of day (up to 4 time slots, HH:MM granularity).

**Architecture:** New `sync_schedules` table stores per-project time slots. In-process `time.Ticker` (1-min interval) goroutine checks for due schedules and fires `SyncProject`. New CRUD handler exposes schedule config via REST API. Frontend adds inline schedule UI to fonte cards.

**Tech Stack:** Go (chi router, pgx, zap), PostgreSQL (JSONB for horarios), vanilla JS frontend (single-file SPA).

## Global Constraints

- Max 4 horários per schedule
- Horário format: `HH:MM` validated by regex `^([01]\d|2[0-3]):[0-5]\d$`
- Timezone: server-local, no timezone in stored values
- Follows existing patterns: repository struct + methods, handler with `respondJSON`/`respondError`, chi routing

---

### Task 1: Database Migration

**Files:**
- Create: `backend/migrations/000011_sync_schedules.up.sql`
- Create: `backend/migrations/000011_sync_schedules.down.sql`

**Interfaces:**
- Consumes: nothing
- Produces: `sync_schedules` table, `origem` column on `sync_logs`

- [ ] **Step 1: Write up migration**

```sql
-- backend/migrations/000011_sync_schedules.up.sql

CREATE TABLE sync_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fonte_dados_id UUID NOT NULL REFERENCES fonte_dados(id) ON DELETE CASCADE,
    project_key VARCHAR(50) NOT NULL,
    horarios JSONB NOT NULL DEFAULT '[]',
    ativo BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(fonte_dados_id, project_key)
);

ALTER TABLE sync_logs ADD COLUMN origem VARCHAR(20) NOT NULL DEFAULT 'manual';
```

- [ ] **Step 2: Write down migration**

```sql
-- backend/migrations/000011_sync_schedules.down.sql

DROP TABLE IF EXISTS sync_schedules;
ALTER TABLE sync_logs DROP COLUMN IF EXISTS origem;
```

- [ ] **Step 3: Run migration**

```bash
cd /home/emerson/code/myplanner/backend
go run cmd/migrate/main.go up
```

Expected: migration 000011 applied successfully.

- [ ] **Step 4: Verify table exists**

```bash
psql -c "\d sync_schedules"
psql -c "\d sync_logs" | grep origem
```

Expected: `sync_schedules` table with columns listed, `origem` column in `sync_logs`.

- [ ] **Step 5: Commit**

```bash
git add backend/migrations/000011_sync_schedules.up.sql backend/migrations/000011_sync_schedules.down.sql
git commit -m "feat: add sync_schedules table and origem column to sync_logs"
```

---

### Task 2: Domain Model & Repository

**Files:**
- Create: `backend/internal/domain/sync_schedule.go`
- Create: `backend/internal/repository/sync_schedule.go`
- Modify: `backend/internal/domain/models.go` (add `Origem` field to `SyncLog`)
- Modify: `backend/internal/repository/sync.go` (add `origem` to `CreateSyncLog`)

**Interfaces:**
- Consumes: `sync_schedules` table, `sync_logs.origem` column
- Produces:
  - `domain.SyncSchedule` struct
  - `repository.SyncScheduleRepository` with methods:
    - `Upsert(ctx context.Context, fonteID uuid.UUID, projectKey string, horarios []string) (*domain.SyncSchedule, error)`
    - `Delete(ctx context.Context, fonteID uuid.UUID, projectKey string) error`
    - `ListByFonte(ctx context.Context, fonteID uuid.UUID) ([]domain.SyncSchedule, error)`
    - `GetDueSchedules(ctx context.Context, horaMinuto string) ([]domain.SyncSchedule, error)`
    - `SetAtivo(ctx context.Context, id uuid.UUID, ativo bool) error`
  - Updated `SyncLog.Origem` field
  - Updated `CreateSyncLog` accepting `origem`

- [ ] **Step 1: Create domain struct**

```go
// backend/internal/domain/sync_schedule.go
package domain

import (
	"time"

	"github.com/google/uuid"
)

type SyncSchedule struct {
	ID           uuid.UUID `json:"id" db:"id"`
	FonteDadosID uuid.UUID `json:"fonte_dados_id" db:"fonte_dados_id"`
	ProjectKey   string    `json:"project_key" db:"project_key"`
	Horarios     []string  `json:"horarios" db:"horarios"`
	Ativo        bool      `json:"ativo" db:"ativo"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}
```

- [ ] **Step 2: Add `Origem` field to `SyncLog` in `models.go`**

In `backend/internal/domain/models.go`, add `Origem` field to the `SyncLog` struct, after the `CreatedAt` field:

```go
type SyncLog struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	FonteDadosID   uuid.UUID       `json:"fonte_dados_id" db:"fonte_dados_id"`
	Tipo           string          `json:"tipo" db:"tipo"`
	Status         string          `json:"status" db:"status"`
	IniciadoEm     time.Time       `json:"iniciado_em" db:"iniciado_em"`
	FinalizadoEm   *time.Time      `json:"finalizado_em" db:"finalizado_em"`
	TotalProjetos  int             `json:"total_projetos" db:"total_projetos"`
	TotalTarefas   int             `json:"total_tarefas" db:"total_tarefas"`
	TotalMembros   int             `json:"total_membros" db:"total_membros"`
	TotalSprints   int             `json:"total_sprints" db:"total_sprints"`
	Erros          json.RawMessage `json:"erros" db:"erros"`
	Mensagem       *string         `json:"mensagem" db:"mensagem"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
	Origem         string          `json:"origem" db:"origem"`
}
```

- [ ] **Step 3: Update `CreateSyncLog` in `repository/sync.go`**

Change the `CreateSyncLog` method to include `origem`:

```go
func (r *SyncRepository) CreateSyncLog(ctx context.Context, log *domain.SyncLog) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sync_logs (id, fonte_dados_id, tipo, status, iniciado_em, mensagem, origem)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, log.ID, log.FonteDadosID, log.Tipo, log.Status, log.IniciadoEm, log.Mensagem, log.Origem)
	if err != nil {
		return fmt.Errorf("creating sync log: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Update `GetLatestSyncLog` and `ListSyncLogs` scan in `repository/sync.go`**

Add `&log.Origem` to the `Scan` calls wherever `SyncLog` is scanned from DB. Find all query/scan patterns for `sync_logs` and add `origem` to the SELECT and Scan.

- [ ] **Step 5: Set default `Origem` in `SyncService.SyncProject` and `syncOne`**

In `backend/internal/service/sync.go`, where `SyncLog` is constructed (in `SyncProject` at line ~140 and `syncOne`), set `Origem: "manual"`:

```go
syncLog := &domain.SyncLog{
	ID:           uuid.New(),
	FonteDadosID: fonte.ID,
	Tipo:         "project",
	Status:       "running",
	IniciadoEm:   time.Now(),
	Origem:       "manual",
}
```

Do the same in `syncOne` where the full-sync log is created.

- [ ] **Step 6: Create repository file**

```go
// backend/internal/repository/sync_schedule.go
package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

type SyncScheduleRepository struct {
	pool *pgxpool.Pool
}

func NewSyncScheduleRepository(pool *pgxpool.Pool) *SyncScheduleRepository {
	return &SyncScheduleRepository{pool: pool}
}

func (r *SyncScheduleRepository) Upsert(ctx context.Context, fonteID uuid.UUID, projectKey string, horarios []string) (*domain.SyncSchedule, error) {
	horariosJSON, err := json.Marshal(horarios)
	if err != nil {
		return nil, fmt.Errorf("marshaling horarios: %w", err)
	}

	var s domain.SyncSchedule
	var horariosRaw []byte
	err = r.pool.QueryRow(ctx, `
		INSERT INTO sync_schedules (fonte_dados_id, project_key, horarios)
		VALUES ($1, $2, $3)
		ON CONFLICT (fonte_dados_id, project_key)
		DO UPDATE SET horarios = EXCLUDED.horarios, updated_at = NOW()
		RETURNING id, fonte_dados_id, project_key, horarios, ativo, created_at, updated_at
	`, fonteID, projectKey, horariosJSON).Scan(&s.ID, &s.FonteDadosID, &s.ProjectKey, &horariosRaw, &s.Ativo, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upserting sync schedule: %w", err)
	}
	json.Unmarshal(horariosRaw, &s.Horarios)
	return &s, nil
}

func (r *SyncScheduleRepository) Delete(ctx context.Context, fonteID uuid.UUID, projectKey string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM sync_schedules WHERE fonte_dados_id = $1 AND project_key = $2
	`, fonteID, projectKey)
	if err != nil {
		return fmt.Errorf("deleting sync schedule: %w", err)
	}
	return nil
}

func (r *SyncScheduleRepository) ListByFonte(ctx context.Context, fonteID uuid.UUID) ([]domain.SyncSchedule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, project_key, horarios, ativo, created_at, updated_at
		FROM sync_schedules WHERE fonte_dados_id = $1
		ORDER BY project_key
	`, fonteID)
	if err != nil {
		return nil, fmt.Errorf("listing sync schedules: %w", err)
	}
	defer rows.Close()

	var schedules []domain.SyncSchedule
	for rows.Next() {
		var s domain.SyncSchedule
		var horariosRaw []byte
		if err := rows.Scan(&s.ID, &s.FonteDadosID, &s.ProjectKey, &horariosRaw, &s.Ativo, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning sync schedule: %w", err)
		}
		json.Unmarshal(horariosRaw, &s.Horarios)
		schedules = append(schedules, s)
	}
	if schedules == nil {
		schedules = []domain.SyncSchedule{}
	}
	return schedules, nil
}

func (r *SyncScheduleRepository) GetDueSchedules(ctx context.Context, horaMinuto string) ([]domain.SyncSchedule, error) {
	quoted, _ := json.Marshal(horaMinuto)
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, project_key, horarios, ativo, created_at, updated_at
		FROM sync_schedules
		WHERE ativo = true AND horarios @> $1::jsonb
	`, string(quoted))
	if err != nil {
		return nil, fmt.Errorf("getting due schedules: %w", err)
	}
	defer rows.Close()

	var schedules []domain.SyncSchedule
	for rows.Next() {
		var s domain.SyncSchedule
		var horariosRaw []byte
		if err := rows.Scan(&s.ID, &s.FonteDadosID, &s.ProjectKey, &horariosRaw, &s.Ativo, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning due schedule: %w", err)
		}
		json.Unmarshal(horariosRaw, &s.Horarios)
		schedules = append(schedules, s)
	}
	return schedules, nil
}

func (r *SyncScheduleRepository) SetAtivo(ctx context.Context, id uuid.UUID, ativo bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE sync_schedules SET ativo = $2, updated_at = NOW() WHERE id = $1
	`, id, ativo)
	if err != nil {
		return fmt.Errorf("toggling sync schedule: %w", err)
	}
	return nil
}
```

- [ ] **Step 7: Build and verify**

```bash
cd /home/emerson/code/myplanner/backend
go build ./...
```

Expected: no compilation errors.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/domain/sync_schedule.go backend/internal/domain/models.go backend/internal/repository/sync_schedule.go backend/internal/repository/sync.go backend/internal/service/sync.go
git commit -m "feat: add SyncSchedule domain, repository, and origem field to sync_logs"
```

---

### Task 3: Scheduler Service

**Files:**
- Create: `backend/internal/service/scheduler.go`

**Interfaces:**
- Consumes:
  - `repository.SyncScheduleRepository.GetDueSchedules(ctx, horaMinuto string) ([]domain.SyncSchedule, error)`
  - `service.SyncService.SyncProject(ctx, fonteID uuid.UUID, projectKey string) (*domain.SyncLog, error)`
  - `service.ErrSyncAlreadyRunning`
- Produces:
  - `service.SchedulerService` with methods:
    - `NewSchedulerService(syncSvc *SyncService, scheduleRepo *repository.SyncScheduleRepository, logger *zap.Logger) *SchedulerService`
    - `Start(ctx context.Context)`

- [ ] **Step 1: Create scheduler service**

```go
// backend/internal/service/scheduler.go
package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

type SchedulerService struct {
	syncSvc      *SyncService
	scheduleRepo *repository.SyncScheduleRepository
	logger       *zap.Logger
	mu           sync.Mutex
	lastFired    map[uuid.UUID]string
}

func NewSchedulerService(syncSvc *SyncService, scheduleRepo *repository.SyncScheduleRepository, logger *zap.Logger) *SchedulerService {
	return &SchedulerService{
		syncSvc:      syncSvc,
		scheduleRepo: scheduleRepo,
		logger:       logger,
		lastFired:    make(map[uuid.UUID]string),
	}
}

func (s *SchedulerService) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	s.logger.Info("scheduler started")

	cleanTick := 0
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler stopped")
			return
		case <-ticker.C:
			s.tick(ctx)
			cleanTick++
			if cleanTick >= 60 {
				s.cleanLastFired()
				cleanTick = 0
			}
		}
	}
}

func (s *SchedulerService) tick(ctx context.Context) {
	hora := time.Now().Format("15:04")

	schedules, err := s.scheduleRepo.GetDueSchedules(ctx, hora)
	if err != nil {
		s.logger.Error("scheduler: failed to get due schedules", zap.Error(err))
		return
	}

	if len(schedules) == 0 {
		return
	}

	s.logger.Info("scheduler: found due schedules", zap.String("hora", hora), zap.Int("count", len(schedules)))

	for _, sched := range schedules {
		s.mu.Lock()
		if s.lastFired[sched.ID] == hora {
			s.mu.Unlock()
			continue
		}
		s.lastFired[sched.ID] = hora
		s.mu.Unlock()

		go func(fonteID uuid.UUID, projectKey string, schedID uuid.UUID) {
			s.logger.Info("scheduler: triggering sync",
				zap.String("project", projectKey),
				zap.String("hora", hora),
			)
			_, err := s.syncSvc.SyncProjectScheduled(ctx, fonteID, projectKey)
			if err != nil {
				if errors.Is(err, ErrSyncAlreadyRunning) {
					s.logger.Warn("scheduler: sync already running, skipping",
						zap.String("project", projectKey),
					)
					return
				}
				s.logger.Error("scheduler: sync failed",
					zap.String("project", projectKey),
					zap.Error(err),
				)
			}
		}(sched.FonteDadosID, sched.ProjectKey, sched.ID)
	}
}

func (s *SchedulerService) cleanLastFired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastFired = make(map[uuid.UUID]string)
}
```

- [ ] **Step 2: Add `SyncProjectScheduled` method to `SyncService`**

In `backend/internal/service/sync.go`, add a method that calls the same logic as `SyncProject` but sets `Origem: "scheduled"`:

```go
func (s *SyncService) SyncProjectScheduled(ctx context.Context, fonteDadosID uuid.UUID, projectKey string) (*domain.SyncLog, error) {
	running, err := s.repo.HasRunningSync(ctx, fonteDadosID)
	if err != nil {
		return nil, err
	}
	if running {
		return nil, ErrSyncAlreadyRunning
	}

	fonte, err := s.getFonte(ctx, fonteDadosID)
	if err != nil {
		return nil, err
	}
	client, err := s.buildClient(ctx, fonte)
	if err != nil {
		return nil, err
	}

	syncLog := &domain.SyncLog{
		ID:           uuid.New(),
		FonteDadosID: fonte.ID,
		Tipo:         "project",
		Status:       "running",
		IniciadoEm:   time.Now(),
		Origem:       "scheduled",
	}
	if err := s.repo.CreateSyncLog(ctx, syncLog); err != nil {
		return nil, fmt.Errorf("creating sync log: %w", err)
	}

	go s.runProjectSync(client, fonte, projectKey, syncLog.ID)

	return syncLog, nil
}
```

- [ ] **Step 3: Build and verify**

```bash
cd /home/emerson/code/myplanner/backend
go build ./...
```

Expected: no compilation errors.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/service/scheduler.go backend/internal/service/sync.go
git commit -m "feat: add scheduler service with 1-min ticker and SyncProjectScheduled"
```

---

### Task 4: HTTP Handler & Routes

**Files:**
- Create: `backend/internal/handler/sync_schedule.go`
- Modify: `backend/cmd/api/main.go`

**Interfaces:**
- Consumes:
  - `repository.SyncScheduleRepository` (all methods)
  - `service.SchedulerService`
- Produces:
  - REST endpoints:
    - `GET /api/v1/sync/schedules?fonte_dados_id=X` → `[]domain.SyncSchedule`
    - `PUT /api/v1/sync/schedules` body `{fonte_dados_id, project_key, horarios}` → `domain.SyncSchedule`
    - `DELETE /api/v1/sync/schedules?fonte_dados_id=X&project_key=Y` → 204
    - `PATCH /api/v1/sync/schedules/{id}/toggle` → `domain.SyncSchedule`

- [ ] **Step 1: Create handler**

```go
// backend/internal/handler/sync_schedule.go
package handler

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

var timeRegex = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

type SyncScheduleHandler struct {
	repo   *repository.SyncScheduleRepository
	logger *zap.Logger
}

func NewSyncScheduleHandler(repo *repository.SyncScheduleRepository, logger *zap.Logger) *SyncScheduleHandler {
	return &SyncScheduleHandler{repo: repo, logger: logger}
}

func (h *SyncScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
	fdIDStr := r.URL.Query().Get("fonte_dados_id")
	if fdIDStr == "" {
		respondError(w, http.StatusBadRequest, "fonte_dados_id é obrigatório")
		return
	}
	fdID, err := uuid.Parse(fdIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "fonte_dados_id inválido")
		return
	}
	schedules, err := h.repo.ListByFonte(r.Context(), fdID)
	if err != nil {
		h.logger.Error("failed to list schedules", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar agendamentos")
		return
	}
	respondJSON(w, http.StatusOK, schedules)
}

type upsertScheduleRequest struct {
	FonteDadosID uuid.UUID `json:"fonte_dados_id"`
	ProjectKey   string    `json:"project_key"`
	Horarios     []string  `json:"horarios"`
}

func (h *SyncScheduleHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	var req upsertScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	if req.FonteDadosID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "fonte_dados_id é obrigatório")
		return
	}
	if req.ProjectKey == "" {
		respondError(w, http.StatusBadRequest, "project_key é obrigatório")
		return
	}
	if len(req.Horarios) > 4 {
		respondError(w, http.StatusBadRequest, "máximo de 4 horários permitidos")
		return
	}
	for _, h := range req.Horarios {
		if !timeRegex.MatchString(h) {
			respondError(w, http.StatusBadRequest, "horário inválido: "+h+". Use formato HH:MM")
			return
		}
	}

	schedule, err := h.repo.Upsert(r.Context(), req.FonteDadosID, req.ProjectKey, req.Horarios)
	if err != nil {
		h.logger.Error("failed to upsert schedule", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao salvar agendamento")
		return
	}
	respondJSON(w, http.StatusOK, schedule)
}

func (h *SyncScheduleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	fdIDStr := r.URL.Query().Get("fonte_dados_id")
	projectKey := r.URL.Query().Get("project_key")

	if fdIDStr == "" || projectKey == "" {
		respondError(w, http.StatusBadRequest, "fonte_dados_id e project_key são obrigatórios")
		return
	}
	fdID, err := uuid.Parse(fdIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "fonte_dados_id inválido")
		return
	}

	if err := h.repo.Delete(r.Context(), fdID, projectKey); err != nil {
		h.logger.Error("failed to delete schedule", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao remover agendamento")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SyncScheduleHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var body struct {
		Ativo bool `json:"ativo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	if err := h.repo.SetAtivo(r.Context(), id, body.Ativo); err != nil {
		h.logger.Error("failed to toggle schedule", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao alterar agendamento")
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"ativo": body.Ativo})
}
```

- [ ] **Step 2: Wire up in `main.go`**

Add after `syncHandler` creation (around line 126):

```go
scheduleRepo := repository.NewSyncScheduleRepository(pool)
scheduleHandler := handler.NewSyncScheduleHandler(scheduleRepo, logger)
schedulerSvc := service.NewSchedulerService(syncService, scheduleRepo, logger)
go schedulerSvc.Start(ctx)
```

Add routes inside the authenticated group (after line 221):

```go
r.Get("/sync/schedules", scheduleHandler.List)
r.Put("/sync/schedules", scheduleHandler.Upsert)
r.Delete("/sync/schedules", scheduleHandler.Delete)
r.Patch("/sync/schedules/{id}/toggle", scheduleHandler.Toggle)
```

- [ ] **Step 3: Build and verify**

```bash
cd /home/emerson/code/myplanner/backend
go build ./...
```

Expected: no compilation errors.

- [ ] **Step 4: Start server and test endpoints**

```bash
cd /home/emerson/code/myplanner/backend
go run cmd/api/main.go &
```

Test list (should return `[]`):
```bash
curl -s -H "Authorization: Bearer $TOKEN" "http://localhost:8080/api/v1/sync/schedules?fonte_dados_id=<some-fonte-id>"
```

Test upsert:
```bash
curl -s -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"fonte_dados_id":"<fonte-id>","project_key":"PROJ","horarios":["08:30","14:00"]}' \
  "http://localhost:8080/api/v1/sync/schedules"
```

Expected: returns `SyncSchedule` JSON with horarios array.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/sync_schedule.go backend/cmd/api/main.go
git commit -m "feat: add sync schedule handler, routes, and start scheduler on boot"
```

---

### Task 5: Frontend — Schedule UI in Fonte Cards

**Files:**
- Modify: `frontend/index.html`

**Interfaces:**
- Consumes:
  - `GET /api/v1/sync/schedules?fonte_dados_id=X` → `[]SyncSchedule`
  - `PUT /api/v1/sync/schedules` body `{fonte_dados_id, project_key, horarios}`
  - `DELETE /api/v1/sync/schedules?fonte_dados_id=X&project_key=Y`
  - `PATCH /api/v1/sync/schedules/{id}/toggle` body `{ativo: bool}`
  - `GET /api/v1/sync/projects?fonte_dados_id=X` → `[]JiraProjectInfo` (already exists)
- Produces: Inline schedule UI in fonte card

- [ ] **Step 1: Add CSS styles**

Add to the `<style>` section, near the existing `.fonte-card` styles:

```css
.schedule-section { margin-top: 12px; border-top: 1px solid var(--border); padding-top: 12px; }
.schedule-toggle-btn { background: none; border: 1px solid var(--border); border-radius: 6px; padding: 6px 12px; cursor: pointer; font-size: 13px; color: var(--text); display: flex; align-items: center; gap: 6px; }
.schedule-toggle-btn:hover { background: var(--hover); }
.schedule-panel { margin-top: 10px; }
.schedule-project-row { display: flex; align-items: center; gap: 8px; padding: 6px 0; border-bottom: 1px solid var(--border); flex-wrap: wrap; }
.schedule-project-row .project-key { font-weight: 600; min-width: 80px; font-size: 13px; }
.schedule-times { display: flex; gap: 4px; align-items: center; flex-wrap: wrap; }
.schedule-times input[type="time"] { padding: 3px 6px; border: 1px solid var(--border); border-radius: 4px; font-size: 12px; background: var(--bg); color: var(--text); }
.schedule-add-time { background: none; border: 1px dashed var(--border); border-radius: 4px; padding: 3px 8px; cursor: pointer; font-size: 12px; color: var(--text-secondary); }
.schedule-add-time:hover { border-color: var(--primary); color: var(--primary); }
.schedule-remove-time { background: none; border: none; cursor: pointer; color: var(--danger, #e74c3c); font-size: 14px; padding: 0 2px; }
.schedule-actions { margin-top: 10px; display: flex; gap: 8px; }
.schedule-badge { display: inline-flex; align-items: center; gap: 4px; background: var(--primary-light, #e8f4fd); color: var(--primary); font-size: 11px; padding: 2px 8px; border-radius: 10px; margin-left: 8px; }
.schedule-ativo-toggle { cursor: pointer; }
.schedule-next { font-size: 11px; color: var(--text-secondary); margin-top: 4px; }
```

- [ ] **Step 2: Add schedule badge to fonte card rendering**

In `renderFonteCards` function, after fetching `syncStatus`, also fetch schedules and compute badge:

After `let syncStatus = null;` line, add:
```javascript
let schedules = [];
try { schedules = await api('/sync/schedules?fonte_dados_id=' + f.id); } catch(e) {}
const activeSchedules = schedules.filter(s => s.ativo);
const scheduleBadge = activeSchedules.length > 0 ? '<span class="schedule-badge">🕐 Agendado</span>' : '';
```

Add `scheduleBadge` after the fonte name in the card header:
```javascript
'<span class="fonte-card-name">' + esc(f.nome) + '</span>' + scheduleBadge + authBadge
```

Add schedule button and section before the closing `</div>` of the card (after `fonte-card-actions` div):

```javascript
const nextSync = computeNextSync(activeSchedules);
const nextSyncHtml = nextSync ? '<div class="schedule-next">Próximo sync: ' + nextSync + '</div>' : '';
const scheduleBtn = '<div class="schedule-section"><button class="schedule-toggle-btn" onclick="toggleSchedulePanel(\'' + f.id + '\')">🕐 Agendar Sync</button>' + nextSyncHtml + '<div class="schedule-panel" id="schedule-panel-' + f.id + '" style="display:none"></div></div>';
```

Insert `scheduleBtn` into the card HTML before the final `</div>`.

- [ ] **Step 3: Add `computeNextSync` helper**

```javascript
function computeNextSync(schedules) {
  if (!schedules || !schedules.length) return '';
  const now = new Date();
  const nowMins = now.getHours() * 60 + now.getMinutes();
  let closest = null;
  for (const s of schedules) {
    for (const h of s.horarios) {
      const [hh, mm] = h.split(':').map(Number);
      const mins = hh * 60 + mm;
      const diff = mins > nowMins ? mins - nowMins : mins + 1440 - nowMins;
      if (closest === null || diff < closest.diff) {
        closest = { time: h, diff: diff };
      }
    }
  }
  return closest ? closest.time : '';
}
```

- [ ] **Step 4: Add `toggleSchedulePanel` function**

```javascript
async function toggleSchedulePanel(fonteId) {
  const panel = document.getElementById('schedule-panel-' + fonteId);
  if (panel.style.display !== 'none') {
    panel.style.display = 'none';
    return;
  }
  panel.style.display = 'block';
  panel.innerHTML = '<div style="padding:10px;color:var(--text-secondary)">Carregando...</div>';

  const [projects, schedules] = await Promise.all([
    api('/sync/projects?fonte_dados_id=' + fonteId),
    api('/sync/schedules?fonte_dados_id=' + fonteId)
  ]);

  const schedMap = {};
  for (const s of schedules) { schedMap[s.project_key] = s; }

  let html = '';
  for (const p of projects) {
    const sched = schedMap[p.key];
    const horarios = sched ? sched.horarios : [];
    const ativo = sched ? sched.ativo : true;
    const checked = sched ? 'checked' : '';
    const ativoChecked = ativo ? 'checked' : '';

    html += '<div class="schedule-project-row" data-project-key="' + esc(p.key) + '">';
    html += '<input type="checkbox" class="schedule-project-check" ' + checked + ' onchange="onScheduleProjectToggle(this,\'' + fonteId + '\',\'' + esc(p.key) + '\')">';
    html += '<span class="project-key">' + esc(p.key) + '</span>';
    html += '<span style="font-size:12px;color:var(--text-secondary)">' + esc(p.name) + '</span>';
    if (sched) {
      html += '<label class="schedule-ativo-toggle" style="margin-left:auto;font-size:11px;display:flex;align-items:center;gap:4px"><input type="checkbox" ' + ativoChecked + ' onchange="toggleScheduleAtivo(\'' + sched.id + '\',this.checked)"> ativo</label>';
    }
    html += '</div>';

    html += '<div class="schedule-times" id="schedule-times-' + fonteId + '-' + esc(p.key) + '" style="' + (sched ? '' : 'display:none') + ';padding-left:28px;padding-bottom:6px">';
    for (let i = 0; i < horarios.length; i++) {
      html += '<input type="time" value="' + horarios[i] + '" class="schedule-time-input">';
      html += '<button class="schedule-remove-time" onclick="this.previousElementSibling.remove();this.remove()">×</button>';
    }
    if (horarios.length < 4) {
      html += '<button class="schedule-add-time" onclick="addScheduleTime(this,\'' + fonteId + '\',\'' + esc(p.key) + '\')">+ horário</button>';
    }
    html += '</div>';
  }

  html += '<div class="schedule-actions"><button class="btn-sm primary" onclick="saveSchedules(\'' + fonteId + '\')">Salvar</button></div>';
  panel.innerHTML = html;
}
```

- [ ] **Step 5: Add helper functions**

```javascript
function onScheduleProjectToggle(checkbox, fonteId, projectKey) {
  const timesEl = document.getElementById('schedule-times-' + fonteId + '-' + projectKey);
  if (checkbox.checked) {
    timesEl.style.display = 'flex';
    if (!timesEl.querySelector('.schedule-time-input')) {
      const addBtn = timesEl.querySelector('.schedule-add-time');
      if (addBtn) addBtn.click();
    }
  } else {
    timesEl.style.display = 'none';
  }
}

function addScheduleTime(btn, fonteId, projectKey) {
  const container = document.getElementById('schedule-times-' + fonteId + '-' + projectKey);
  const inputs = container.querySelectorAll('.schedule-time-input');
  if (inputs.length >= 4) return;

  const input = document.createElement('input');
  input.type = 'time';
  input.className = 'schedule-time-input';
  const removeBtn = document.createElement('button');
  removeBtn.className = 'schedule-remove-time';
  removeBtn.textContent = '×';
  removeBtn.onclick = function() { input.remove(); removeBtn.remove(); checkAddBtnVisibility(container); };

  btn.before(input, removeBtn);
  if (inputs.length + 1 >= 4) btn.style.display = 'none';
}

function checkAddBtnVisibility(container) {
  const inputs = container.querySelectorAll('.schedule-time-input');
  const addBtn = container.querySelector('.schedule-add-time');
  if (addBtn) addBtn.style.display = inputs.length < 4 ? '' : 'none';
}

async function toggleScheduleAtivo(schedId, ativo) {
  try {
    await api('/sync/schedules/' + schedId + '/toggle', { method: 'PATCH', body: JSON.stringify({ ativo }) });
  } catch (e) {
    showToast('Erro ao alterar agendamento', 'error');
  }
}

async function saveSchedules(fonteId) {
  const panel = document.getElementById('schedule-panel-' + fonteId);
  const rows = panel.querySelectorAll('.schedule-project-row');
  let saved = 0;

  for (const row of rows) {
    const checkbox = row.querySelector('.schedule-project-check');
    const projectKey = row.dataset.projectKey;
    const timesContainer = document.getElementById('schedule-times-' + fonteId + '-' + projectKey);

    if (checkbox.checked) {
      const inputs = timesContainer.querySelectorAll('.schedule-time-input');
      const horarios = Array.from(inputs).map(i => i.value).filter(v => v);
      if (horarios.length === 0) continue;

      try {
        await api('/sync/schedules', {
          method: 'PUT',
          body: JSON.stringify({ fonte_dados_id: fonteId, project_key: projectKey, horarios })
        });
        saved++;
      } catch (e) {
        showToast('Erro ao salvar agendamento de ' + projectKey, 'error');
      }
    } else {
      try {
        await api('/sync/schedules?fonte_dados_id=' + fonteId + '&project_key=' + projectKey, { method: 'DELETE' });
      } catch (e) {}
    }
  }

  showToast(saved + ' agendamento(s) salvo(s)', 'success');
  loadFontes();
}
```

- [ ] **Step 6: Test in browser**

1. Start backend: `cd /home/emerson/code/myplanner/backend && go run cmd/api/main.go`
2. Open browser to the app
3. Navigate to "Fontes de Dados" tab
4. Click "Agendar Sync" on a fonte card
5. Select a project, add times, save
6. Verify badge "Agendado" appears
7. Verify "Próximo sync" time is correct
8. Check toggle on/off works
9. Remove a schedule and verify DELETE works

- [ ] **Step 7: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add scheduled sync UI to fonte de dados cards"
```

---

### Task 6: Integration Test & Final Verification

**Files:**
- Modify: `backend/internal/service/sync_test.go` (if exists, add scheduler test)

**Interfaces:**
- Consumes: all previous tasks
- Produces: verified working system

- [ ] **Step 1: Verify full flow end-to-end**

1. Start backend
2. Create a schedule via API with a time 1 minute from now
3. Watch backend logs for scheduler trigger
4. Verify sync_log created with `origem = 'scheduled'`

```bash
# Create schedule for 1 min from now
NEXT=$(date -d "+1 minute" +%H:%M)
curl -s -X PUT -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"fonte_dados_id\":\"<id>\",\"project_key\":\"PROJ\",\"horarios\":[\"$NEXT\"]}" \
  "http://localhost:8080/api/v1/sync/schedules"

# Watch logs
# After 1 minute, check sync logs
curl -s -H "Authorization: Bearer $TOKEN" "http://localhost:8080/api/v1/sync/logs?fonte_dados_id=<id>&limit=1"
```

Expected: sync log with `origem: "scheduled"`.

- [ ] **Step 2: Verify duplicate prevention**

Start a manual sync, then verify scheduler skips if sync is already running:
- Watch backend logs for `"scheduler: sync already running, skipping"` message.

- [ ] **Step 3: Verify frontend shows scheduled badge and next sync**

- Open Fontes de Dados
- Confirm badge "Agendado" appears on fonte with active schedules
- Confirm "Próximo sync: HH:MM" shows correctly

- [ ] **Step 4: Final commit with any fixes**

```bash
git add -A
git commit -m "feat: scheduled sync module complete — scheduler, API, frontend"
```
