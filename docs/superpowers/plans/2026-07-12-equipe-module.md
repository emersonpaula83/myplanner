# Equipe Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build backend API for the Equipe (Team) module — team listing, member listing, and a summary endpoint with tracked activity %, demand-type distribution (Metas/Compromissos/Iniciativas), and Iniciativas sub-breakdown.

**Architecture:** Three REST endpoints behind `/api/v1/equipes`. Repository layer fetches raw data (members, absences, task aggregates). Handler orchestrates a pure calculation function `calcularResumoEquipe` that computes all percentages. This keeps business logic testable without DB.

**Tech Stack:** Go, chi router, pgx/v5, PostgreSQL 16, zap logger. Standard library `testing` for tests.

## Global Constraints

- Domain names in Portuguese (existing convention)
- `estimativa_tempo` stores seconds (Jira API convention) — divide by 3600 for hours
- 40h work week = 8h per weekday
- Absences types counted: `dayoff`, `ferias`, `licenca_medica`, `licenca_paternidade`, `licenca_maternidade`
- Tasks "worked on" = filter by `COALESCE(data_atualizado, data_criacao)` within period
- Task assignment = `responsavel_id` (Jira assignee)
- Period filter: `1m`, `2m`, `3m`, `6m`, `1a` (months back from now; default `3m`)
- Follow existing patterns: handler interface + repository struct (see `FonteDadosStore`)

## File Structure

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `backend/internal/domain/equipe.go` | Response types + intermediate calculation types |
| Create | `backend/migrations/000002_equipe_indexes.up.sql` | Performance indexes for team queries |
| Create | `backend/migrations/000002_equipe_indexes.down.sql` | Rollback indexes |
| Create | `backend/internal/repository/equipe.go` | SQL queries for teams, members, absences, task aggregates |
| Create | `backend/internal/handler/equipe.go` | HTTP handlers, EquipeStore interface, calculation logic |
| Create | `backend/internal/handler/equipe_test.go` | Unit tests for calculation + handler |
| Modify | `backend/cmd/api/main.go:55-84` | Wire EquipeRepository + EquipeHandler + routes |

---

### Task 1: Domain Response Types

**Files:**
- Create: `backend/internal/domain/equipe.go`

**Interfaces:**
- Consumes: nothing
- Produces: `ResumoEquipe`, `DetalhesIniciativas`, `MembroResumo`, `HorasTarefasMembro` — used by repository and handler

- [ ] **Step 1: Create domain types file**

```go
package domain

import "github.com/google/uuid"

type ResumoEquipe struct {
	NomeEquipe             string              `json:"nome_equipe"`
	Periodo                string              `json:"periodo"`
	AtuacaoRastreada       float64             `json:"atuacao_rastreada"`
	PercentualMetas        float64             `json:"percentual_metas"`
	PercentualCompromissos float64             `json:"percentual_compromissos"`
	PercentualIniciativas  float64             `json:"percentual_iniciativas"`
	DetalhesIniciativas    DetalhesIniciativas  `json:"detalhes_iniciativas"`
	Membros                []MembroResumo       `json:"membros"`
}

type DetalhesIniciativas struct {
	PercentualManutencao    float64 `json:"percentual_manutencao"`
	PercentualMelhorias     float64 `json:"percentual_melhorias"`
	PercentualEvolucao float64 `json:"percentual_evolucao"`
	PercentualSuporte       float64 `json:"percentual_suporte"`
}

type MembroResumo struct {
	ID               uuid.UUID `json:"id"`
	Nome             string    `json:"nome"`
	Email            *string   `json:"email"`
	AvatarURL        *string   `json:"avatar_url"`
	AtuacaoRastreada float64   `json:"atuacao_rastreada"`
}

type HorasTarefasMembro struct {
	MembroID              uuid.UUID
	TotalSegundos         int64
	SegundosMetas         int64
	SegundosCompromissos  int64
	SegundosIniciativas   int64
	SegundosManutencao    int64
	SegundosMelhorias     int64
	SegundosEvolucao int64
	SegundosSuporte       int64
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd backend && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 3: Commit**

```bash
git add backend/internal/domain/equipe.go
git commit -m "feat(equipe): add domain response types for team module"
```

---

### Task 2: Migration — Team Query Indexes

**Files:**
- Create: `backend/migrations/000002_equipe_indexes.up.sql`
- Create: `backend/migrations/000002_equipe_indexes.down.sql`

**Interfaces:**
- Consumes: existing tables `membros`, `tarefas`, `disponibilidade`
- Produces: indexes for performant team queries

- [ ] **Step 1: Create up migration**

```sql
CREATE INDEX idx_membros_team ON membros(team) WHERE team IS NOT NULL AND ativo = true;

CREATE INDEX idx_tarefas_responsavel_periodo ON tarefas(responsavel_id, data_atualizado)
    WHERE responsavel_id IS NOT NULL;

CREATE INDEX idx_tarefas_tipo_demanda ON tarefas(tipo_demanda)
    WHERE tipo_demanda IS NOT NULL;

CREATE INDEX idx_disponibilidade_periodo ON disponibilidade(membro_id, data_inicio, data_fim);
```

- [ ] **Step 2: Create down migration**

```sql
DROP INDEX IF EXISTS idx_disponibilidade_periodo;
DROP INDEX IF EXISTS idx_tarefas_tipo_demanda;
DROP INDEX IF EXISTS idx_tarefas_responsavel_periodo;
DROP INDEX IF EXISTS idx_membros_team;
```

- [ ] **Step 3: Run migration**

Run: `cd backend && go run ./cmd/migrate -direction up`
Expected: `migration up completed successfully`

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/000002_equipe_indexes.up.sql backend/migrations/000002_equipe_indexes.down.sql
git commit -m "feat(equipe): add performance indexes for team queries"
```

---

### Task 3: Repository — Equipe Data Access

**Files:**
- Create: `backend/internal/repository/equipe.go`

**Interfaces:**
- Consumes: `domain.Membro`, `domain.HorasTarefasMembro`
- Produces: `EquipeRepository` with methods:
  - `ListEquipes(ctx context.Context) ([]string, error)`
  - `GetMembrosEquipe(ctx context.Context, team string) ([]domain.Membro, error)`
  - `GetDiasAusencia(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) (map[uuid.UUID]int, error)`
  - `GetHorasTarefasEquipe(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]domain.HorasTarefasMembro, error)`

- [ ] **Step 1: Create repository file**

```go
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
)

type EquipeRepository struct {
	pool *pgxpool.Pool
}

func NewEquipeRepository(pool *pgxpool.Pool) *EquipeRepository {
	return &EquipeRepository{pool: pool}
}

func (r *EquipeRepository) ListEquipes(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT team
		FROM membros
		WHERE team IS NOT NULL AND ativo = true
		ORDER BY team
	`)
	if err != nil {
		return nil, fmt.Errorf("listing equipes: %w", err)
	}
	defer rows.Close()

	var teams []string
	for rows.Next() {
		var team string
		if err := rows.Scan(&team); err != nil {
			return nil, fmt.Errorf("scanning team: %w", err)
		}
		teams = append(teams, team)
	}
	return teams, rows.Err()
}

func (r *EquipeRepository) GetMembrosEquipe(ctx context.Context, team string) ([]domain.Membro, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, jira_account_id, nome, email,
		       avatar_url, team, ativo, created_at, updated_at
		FROM membros
		WHERE team = $1 AND ativo = true
		ORDER BY nome
	`, team)
	if err != nil {
		return nil, fmt.Errorf("getting membros for team %s: %w", team, err)
	}
	defer rows.Close()

	var membros []domain.Membro
	for rows.Next() {
		var m domain.Membro
		if err := rows.Scan(
			&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email,
			&m.AvatarURL, &m.Team, &m.Ativo, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning membro: %w", err)
		}
		membros = append(membros, m)
	}
	return membros, rows.Err()
}

func (r *EquipeRepository) GetDiasAusencia(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) (map[uuid.UUID]int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT membro_id,
			COALESCE(SUM(
				(SELECT COUNT(*)
				 FROM generate_series(
					 GREATEST(data_inicio, $2::date),
					 LEAST(data_fim, $3::date),
					 '1 day'::interval
				 ) d
				 WHERE EXTRACT(DOW FROM d) NOT IN (0, 6))
			), 0)::int AS dias_ausencia
		FROM disponibilidade
		WHERE membro_id = ANY($1)
		  AND tipo IN ('dayoff', 'ferias', 'licenca_medica', 'licenca_paternidade', 'licenca_maternidade')
		  AND data_fim >= $2::date
		  AND data_inicio <= $3::date
		GROUP BY membro_id
	`, membroIDs, inicio, fim)
	if err != nil {
		return nil, fmt.Errorf("getting dias ausencia: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]int)
	for rows.Next() {
		var membroID uuid.UUID
		var dias int
		if err := rows.Scan(&membroID, &dias); err != nil {
			return nil, fmt.Errorf("scanning dias ausencia: %w", err)
		}
		result[membroID] = dias
	}
	return result, rows.Err()
}

func (r *EquipeRepository) GetHorasTarefasEquipe(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]domain.HorasTarefasMembro, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			responsavel_id,
			COALESCE(SUM(estimativa_tempo), 0) AS total_segundos,
			COALESCE(SUM(CASE WHEN tipo_demanda = 'Meta' THEN estimativa_tempo ELSE 0 END), 0) AS segundos_metas,
			COALESCE(SUM(CASE WHEN tipo_demanda = 'Compromisso' THEN estimativa_tempo ELSE 0 END), 0) AS segundos_compromissos,
			COALESCE(SUM(CASE WHEN tipo_demanda = 'Iniciativa' THEN estimativa_tempo ELSE 0 END), 0) AS segundos_iniciativas,
			COALESCE(SUM(CASE WHEN tipo_demanda = 'Iniciativa' AND tipo IN ('Bug', 'Incidente') THEN estimativa_tempo ELSE 0 END), 0) AS segundos_manutencao,
			COALESCE(SUM(CASE WHEN tipo_demanda = 'Iniciativa' AND tipo = 'Melhoria' THEN estimativa_tempo ELSE 0 END), 0) AS segundos_melhorias,
			COALESCE(SUM(CASE WHEN tipo_demanda = 'Iniciativa' AND tipo = 'História' THEN estimativa_tempo ELSE 0 END), 0) AS segundos_evolucao,
			COALESCE(SUM(CASE WHEN tipo_demanda = 'Iniciativa' AND tipo IN ('Suporte', 'Tarefa') THEN estimativa_tempo ELSE 0 END), 0) AS segundos_suporte
		FROM tarefas
		WHERE responsavel_id = ANY($1)
		  AND COALESCE(data_atualizado, data_criacao) >= $2
		  AND COALESCE(data_atualizado, data_criacao) < $3
		GROUP BY responsavel_id
	`, membroIDs, inicio, fim)
	if err != nil {
		return nil, fmt.Errorf("getting horas tarefas equipe: %w", err)
	}
	defer rows.Close()

	var result []domain.HorasTarefasMembro
	for rows.Next() {
		var h domain.HorasTarefasMembro
		if err := rows.Scan(
			&h.MembroID, &h.TotalSegundos,
			&h.SegundosMetas, &h.SegundosCompromissos, &h.SegundosIniciativas,
			&h.SegundosManutencao, &h.SegundosMelhorias, &h.SegundosEvolucao, &h.SegundosSuporte,
		); err != nil {
			return nil, fmt.Errorf("scanning horas tarefas: %w", err)
		}
		result = append(result, h)
	}
	return result, rows.Err()
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd backend && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 3: Commit**

```bash
git add backend/internal/repository/equipe.go
git commit -m "feat(equipe): add repository with team data access queries"
```

---

### Task 4: Handler — Calculation Logic + HTTP Endpoints

**Files:**
- Create: `backend/internal/handler/equipe.go`

**Interfaces:**
- Consumes: `EquipeRepository` methods (via `EquipeStore` interface), `domain.ResumoEquipe`, `domain.HorasTarefasMembro`, `domain.Membro`
- Produces:
  - `EquipeStore` interface (satisfied by `EquipeRepository`)
  - `NewEquipeHandler(store EquipeStore, logger *zap.Logger) *EquipeHandler`
  - `(*EquipeHandler).List(w, r)` — `GET /api/v1/equipes`
  - `(*EquipeHandler).GetResumo(w, r)` — `GET /api/v1/equipes/{team}/resumo?periodo=3m`
  - `(*EquipeHandler).GetMembros(w, r)` — `GET /api/v1/equipes/{team}/membros`
  - `ParsePeriodo(p string) (time.Time, time.Time, bool)` — exported for tests
  - `ContarDiasUteis(inicio, fim time.Time) int` — exported for tests
  - `CalcularResumoEquipe(team, periodo string, membros []domain.Membro, ausencias map[uuid.UUID]int, tarefas []domain.HorasTarefasMembro, diasUteis int) domain.ResumoEquipe` — exported pure function for tests

- [ ] **Step 1: Create handler file**

```go
package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"go.uber.org/zap"
)

type EquipeStore interface {
	ListEquipes(ctx context.Context) ([]string, error)
	GetMembrosEquipe(ctx context.Context, team string) ([]domain.Membro, error)
	GetDiasAusencia(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) (map[uuid.UUID]int, error)
	GetHorasTarefasEquipe(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]domain.HorasTarefasMembro, error)
}

type EquipeHandler struct {
	store  EquipeStore
	logger *zap.Logger
}

func NewEquipeHandler(store EquipeStore, logger *zap.Logger) *EquipeHandler {
	return &EquipeHandler{store: store, logger: logger}
}

var periodos = map[string]int{
	"1m": 1,
	"2m": 2,
	"3m": 3,
	"6m": 6,
	"1a": 12,
}

func ParsePeriodo(p string) (time.Time, time.Time, bool) {
	meses, ok := periodos[p]
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	fim := time.Now().Truncate(24 * time.Hour)
	inicio := fim.AddDate(0, -meses, 0)
	return inicio, fim, true
}

func ContarDiasUteis(inicio, fim time.Time) int {
	dias := 0
	for d := inicio; !d.After(fim); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			dias++
		}
	}
	return dias
}

func CalcularResumoEquipe(
	team string,
	periodo string,
	membros []domain.Membro,
	ausencias map[uuid.UUID]int,
	tarefas []domain.HorasTarefasMembro,
	diasUteis int,
) domain.ResumoEquipe {
	horasTotalPeriodo := float64(diasUteis) * 8.0

	tarefasMap := make(map[uuid.UUID]domain.HorasTarefasMembro)
	for _, t := range tarefas {
		tarefasMap[t.MembroID] = t
	}

	var somaAtuacao float64
	var totalSegundosEquipe int64
	var totalMetas, totalCompromissos, totalIniciativas int64
	var totalManutencao, totalMelhorias, totalEvolucao, totalSuporte int64

	membrosResumo := make([]domain.MembroResumo, len(membros))
	for i, m := range membros {
		diasAusencia := ausencias[m.ID]
		horasReais := horasTotalPeriodo - float64(diasAusencia)*8.0
		if horasReais < 0 {
			horasReais = 0
		}

		t := tarefasMap[m.ID]
		horasCards := float64(t.TotalSegundos) / 3600.0

		var pctAtuacao float64
		if horasReais > 0 {
			pctAtuacao = (horasCards / horasReais) * 100
		}

		membrosResumo[i] = domain.MembroResumo{
			ID:               m.ID,
			Nome:             m.Nome,
			Email:            m.Email,
			AvatarURL:        m.AvatarURL,
			AtuacaoRastreada: pctAtuacao,
		}

		somaAtuacao += pctAtuacao
		totalSegundosEquipe += t.TotalSegundos
		totalMetas += t.SegundosMetas
		totalCompromissos += t.SegundosCompromissos
		totalIniciativas += t.SegundosIniciativas
		totalManutencao += t.SegundosManutencao
		totalMelhorias += t.SegundosMelhorias
		totalEvolucao += t.SegundosEvolucao
		totalSuporte += t.SegundosSuporte
	}

	mediaAtuacao := 0.0
	if len(membros) > 0 {
		mediaAtuacao = somaAtuacao / float64(len(membros))
	}

	pctMetas, pctCompromissos, pctIniciativas := 0.0, 0.0, 0.0
	if totalSegundosEquipe > 0 {
		pctMetas = float64(totalMetas) / float64(totalSegundosEquipe) * 100
		pctCompromissos = float64(totalCompromissos) / float64(totalSegundosEquipe) * 100
		pctIniciativas = float64(totalIniciativas) / float64(totalSegundosEquipe) * 100
	}

	detalhes := domain.DetalhesIniciativas{}
	if totalIniciativas > 0 {
		detalhes.PercentualManutencao = float64(totalManutencao) / float64(totalIniciativas) * 100
		detalhes.PercentualMelhorias = float64(totalMelhorias) / float64(totalIniciativas) * 100
		detalhes.PercentualEvolucao = float64(totalEvolucao) / float64(totalIniciativas) * 100
		detalhes.PercentualSuporte = float64(totalSuporte) / float64(totalIniciativas) * 100
	}

	return domain.ResumoEquipe{
		NomeEquipe:             team,
		Periodo:                periodo,
		AtuacaoRastreada:       mediaAtuacao,
		PercentualMetas:        pctMetas,
		PercentualCompromissos: pctCompromissos,
		PercentualIniciativas:  pctIniciativas,
		DetalhesIniciativas:    detalhes,
		Membros:                membrosResumo,
	}
}

func (h *EquipeHandler) List(w http.ResponseWriter, r *http.Request) {
	teams, err := h.store.ListEquipes(r.Context())
	if err != nil {
		h.logger.Error("failed to list equipes", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list equipes")
		return
	}
	respondJSON(w, http.StatusOK, teams)
}

func (h *EquipeHandler) GetResumo(w http.ResponseWriter, r *http.Request) {
	team := chi.URLParam(r, "team")
	periodo := r.URL.Query().Get("periodo")
	if periodo == "" {
		periodo = "3m"
	}

	inicio, fim, ok := ParsePeriodo(periodo)
	if !ok {
		respondError(w, http.StatusBadRequest, "periodo inválido: use 1m, 2m, 3m, 6m, 1a")
		return
	}

	membros, err := h.store.GetMembrosEquipe(r.Context(), team)
	if err != nil {
		h.logger.Error("failed to get membros equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get team members")
		return
	}
	if len(membros) == 0 {
		respondError(w, http.StatusNotFound, "equipe não encontrada")
		return
	}

	membroIDs := make([]uuid.UUID, len(membros))
	for i, m := range membros {
		membroIDs[i] = m.ID
	}

	ausencias, err := h.store.GetDiasAusencia(r.Context(), membroIDs, inicio, fim)
	if err != nil {
		h.logger.Error("failed to get ausencias", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get absences")
		return
	}

	tarefas, err := h.store.GetHorasTarefasEquipe(r.Context(), membroIDs, inicio, fim)
	if err != nil {
		h.logger.Error("failed to get horas tarefas", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get task hours")
		return
	}

	diasUteis := ContarDiasUteis(inicio, fim)
	resumo := CalcularResumoEquipe(team, periodo, membros, ausencias, tarefas, diasUteis)
	respondJSON(w, http.StatusOK, resumo)
}

func (h *EquipeHandler) GetMembros(w http.ResponseWriter, r *http.Request) {
	team := chi.URLParam(r, "team")
	membros, err := h.store.GetMembrosEquipe(r.Context(), team)
	if err != nil {
		h.logger.Error("failed to get membros equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get team members")
		return
	}
	respondJSON(w, http.StatusOK, membros)
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd backend && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handler/equipe.go
git commit -m "feat(equipe): add handler with calculation logic and HTTP endpoints"
```

---

### Task 5: Unit Tests — Calculation Logic

**Files:**
- Create: `backend/internal/handler/equipe_test.go`

**Interfaces:**
- Consumes: `ParsePeriodo`, `ContarDiasUteis`, `CalcularResumoEquipe` from `handler` package
- Produces: test coverage for core business logic

- [ ] **Step 1: Write tests**

```go
package handler

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
)

func TestParsePeriodo(t *testing.T) {
	tests := []struct {
		input string
		valid bool
		meses int
	}{
		{"1m", true, 1},
		{"2m", true, 2},
		{"3m", true, 3},
		{"6m", true, 6},
		{"1a", true, 12},
		{"5m", false, 0},
		{"", false, 0},
		{"abc", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			inicio, fim, ok := ParsePeriodo(tt.input)
			if ok != tt.valid {
				t.Fatalf("ParsePeriodo(%q) valid = %v, want %v", tt.input, ok, tt.valid)
			}
			if !ok {
				return
			}
			diff := fim.Sub(inicio)
			expectedDays := tt.meses * 30
			if math.Abs(diff.Hours()/24-float64(expectedDays)) > 5 {
				t.Errorf("ParsePeriodo(%q) range = %.0f days, want ~%d", tt.input, diff.Hours()/24, expectedDays)
			}
		})
	}
}

func TestContarDiasUteis(t *testing.T) {
	// Monday 2026-07-06 to Friday 2026-07-10 = 5 weekdays
	inicio := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	fim := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	got := ContarDiasUteis(inicio, fim)
	if got != 5 {
		t.Errorf("ContarDiasUteis Mon-Fri = %d, want 5", got)
	}

	// Monday 2026-07-06 to Sunday 2026-07-12 = 5 weekdays
	fim2 := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	got2 := ContarDiasUteis(inicio, fim2)
	if got2 != 5 {
		t.Errorf("ContarDiasUteis Mon-Sun = %d, want 5", got2)
	}

	// Two full weeks: Mon 2026-07-06 to Fri 2026-07-17 = 10 weekdays
	fim3 := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	got3 := ContarDiasUteis(inicio, fim3)
	if got3 != 10 {
		t.Errorf("ContarDiasUteis 2 weeks = %d, want 10", got3)
	}
}

func TestCalcularResumoEquipe(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()

	membros := []domain.Membro{
		{ID: id1, Nome: "Alice"},
		{ID: id2, Nome: "Bob"},
	}

	ausencias := map[uuid.UUID]int{
		id1: 2, // 2 weekdays absent
		// id2: 0 (no absences)
	}

	tarefas := []domain.HorasTarefasMembro{
		{
			MembroID:              id1,
			TotalSegundos:         144000, // 40h
			SegundosMetas:         72000,  // 20h
			SegundosCompromissos:  36000,  // 10h
			SegundosIniciativas:   36000,  // 10h
			SegundosManutencao:    7200,   // 2h
			SegundosMelhorias:     14400,  // 4h
			SegundosEvolucao: 10800,  // 3h
			SegundosSuporte:       3600,   // 1h
		},
		{
			MembroID:              id2,
			TotalSegundos:         72000, // 20h
			SegundosMetas:         36000, // 10h
			SegundosCompromissos:  18000, // 5h
			SegundosIniciativas:   18000, // 5h
			SegundosManutencao:    3600,  // 1h
			SegundosMelhorias:     7200,  // 2h
			SegundosEvolucao: 3600,  // 1h
			SegundosSuporte:       3600,  // 1h
		},
	}

	// 10 weekdays = 80h total per person
	diasUteis := 10

	resumo := CalcularResumoEquipe("TeamA", "2m", membros, ausencias, tarefas, diasUteis)

	if resumo.NomeEquipe != "TeamA" {
		t.Errorf("NomeEquipe = %q, want TeamA", resumo.NomeEquipe)
	}

	// Alice: horasReais = 80 - 2*8 = 64h, horasCards = 40h, atuacao = 40/64*100 = 62.5%
	// Bob:   horasReais = 80 - 0   = 80h, horasCards = 20h, atuacao = 20/80*100 = 25.0%
	// Media: (62.5 + 25.0) / 2 = 43.75%
	if math.Abs(resumo.AtuacaoRastreada-43.75) > 0.01 {
		t.Errorf("AtuacaoRastreada = %.2f, want 43.75", resumo.AtuacaoRastreada)
	}

	// Total seconds: 144000 + 72000 = 216000
	// Metas: (72000+36000)/216000*100 = 50%
	if math.Abs(resumo.PercentualMetas-50.0) > 0.01 {
		t.Errorf("PercentualMetas = %.2f, want 50.00", resumo.PercentualMetas)
	}

	// Compromissos: (36000+18000)/216000*100 = 25%
	if math.Abs(resumo.PercentualCompromissos-25.0) > 0.01 {
		t.Errorf("PercentualCompromissos = %.2f, want 25.00", resumo.PercentualCompromissos)
	}

	// Iniciativas: (36000+18000)/216000*100 = 25%
	if math.Abs(resumo.PercentualIniciativas-25.0) > 0.01 {
		t.Errorf("PercentualIniciativas = %.2f, want 25.00", resumo.PercentualIniciativas)
	}

	// Sub-breakdown within Iniciativas (total = 54000)
	// Manutencao: (7200+3600)/54000*100 = 20%
	if math.Abs(resumo.DetalhesIniciativas.PercentualManutencao-20.0) > 0.01 {
		t.Errorf("PercentualManutencao = %.2f, want 20.00", resumo.DetalhesIniciativas.PercentualManutencao)
	}

	// Melhorias: (14400+7200)/54000*100 = 40%
	if math.Abs(resumo.DetalhesIniciativas.PercentualMelhorias-40.0) > 0.01 {
		t.Errorf("PercentualMelhorias = %.2f, want 40.00", resumo.DetalhesIniciativas.PercentualMelhorias)
	}

	// Evolucao: (10800+3600)/54000*100 ≈ 26.67%
	if math.Abs(resumo.DetalhesIniciativas.PercentualEvolucao-26.67) > 0.01 {
		t.Errorf("PercentualEvolucao = %.2f, want 26.67", resumo.DetalhesIniciativas.PercentualEvolucao)
	}

	// Suporte: (3600+3600)/54000*100 ≈ 13.33%
	if math.Abs(resumo.DetalhesIniciativas.PercentualSuporte-13.33) > 0.01 {
		t.Errorf("PercentualSuporte = %.2f, want 13.33", resumo.DetalhesIniciativas.PercentualSuporte)
	}

	if len(resumo.Membros) != 2 {
		t.Fatalf("Membros count = %d, want 2", len(resumo.Membros))
	}
}

func TestCalcularResumoEquipe_NoTasks(t *testing.T) {
	id1 := uuid.New()
	membros := []domain.Membro{{ID: id1, Nome: "Solo"}}
	ausencias := map[uuid.UUID]int{}
	tarefas := []domain.HorasTarefasMembro{}

	resumo := CalcularResumoEquipe("Empty", "1m", membros, ausencias, tarefas, 22)

	if resumo.AtuacaoRastreada != 0 {
		t.Errorf("AtuacaoRastreada = %.2f, want 0", resumo.AtuacaoRastreada)
	}
	if resumo.PercentualMetas != 0 {
		t.Errorf("PercentualMetas = %.2f, want 0", resumo.PercentualMetas)
	}
}

func TestCalcularResumoEquipe_AllAbsent(t *testing.T) {
	id1 := uuid.New()
	membros := []domain.Membro{{ID: id1, Nome: "Absent"}}
	ausencias := map[uuid.UUID]int{id1: 22} // all days absent
	tarefas := []domain.HorasTarefasMembro{
		{MembroID: id1, TotalSegundos: 3600},
	}

	resumo := CalcularResumoEquipe("Team", "1m", membros, ausencias, tarefas, 22)

	// horasReais = 0, so atuacao = 0 (no division by zero)
	if resumo.AtuacaoRastreada != 0 {
		t.Errorf("AtuacaoRastreada = %.2f, want 0", resumo.AtuacaoRastreada)
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd backend && go test ./internal/handler/ -v -run TestParsePeriodo`
Expected: PASS

Run: `cd backend && go test ./internal/handler/ -v -run TestContarDiasUteis`
Expected: PASS

Run: `cd backend && go test ./internal/handler/ -v -run TestCalcularResumoEquipe`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handler/equipe_test.go
git commit -m "test(equipe): add unit tests for team calculation logic"
```

---

### Task 6: Wire Routes in main.go

**Files:**
- Modify: `backend/cmd/api/main.go:55-84`

**Interfaces:**
- Consumes: `NewEquipeRepository`, `NewEquipeHandler`
- Produces: three new routes under `/api/v1/equipes`

- [ ] **Step 1: Add EquipeRepository + EquipeHandler initialization after line 56**

Add after the `fonteDadosHandler` initialization block:

```go
	equipeRepo := repository.NewEquipeRepository(pool)
	equipeHandler := handler.NewEquipeHandler(equipeRepo, logger)
```

- [ ] **Step 2: Add equipe routes inside the `r.Route("/api/v1", ...)` block**

Add after the `/fontes/{id}` routes:

```go
		r.Get("/equipes", equipeHandler.List)
		r.Get("/equipes/{team}/resumo", equipeHandler.GetResumo)
		r.Get("/equipes/{team}/membros", equipeHandler.GetMembros)
```

- [ ] **Step 3: Verify compilation**

Run: `cd backend && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 4: Start server and test endpoints**

Run: `cd backend && go run ./cmd/api &`

```bash
# List teams (empty if no data, but should return 200)
curl -s http://localhost:8080/api/v1/equipes | jq .

# Test invalid periodo
curl -s http://localhost:8080/api/v1/equipes/TestTeam/resumo?periodo=5m | jq .
# Expected: {"error":"periodo inválido: use 1m, 2m, 3m, 6m, 1a"}

# Test non-existent team
curl -s http://localhost:8080/api/v1/equipes/NonExistent/resumo | jq .
# Expected: {"error":"equipe não encontrada"}
```

- [ ] **Step 5: Commit**

```bash
git add backend/cmd/api/main.go
git commit -m "feat(equipe): wire team endpoints in API router"
```

---

## API Reference

| Method | Path | Query Params | Description |
|--------|------|-------------|-------------|
| `GET` | `/api/v1/equipes` | — | List distinct team names |
| `GET` | `/api/v1/equipes/{team}/resumo` | `periodo=1m\|2m\|3m\|6m\|1a` (default: `3m`) | Team summary with all metrics |
| `GET` | `/api/v1/equipes/{team}/membros` | — | List team members |

### `GET /api/v1/equipes/{team}/resumo` Response

```json
{
  "nome_equipe": "Platform",
  "periodo": "3m",
  "atuacao_rastreada": 67.5,
  "percentual_metas": 40.0,
  "percentual_compromissos": 25.0,
  "percentual_iniciativas": 35.0,
  "detalhes_iniciativas": {
    "percentual_manutencao": 15.0,
    "percentual_melhorias": 30.0,
    "percentual_evolucao": 40.0,
    "percentual_suporte": 15.0
  },
  "membros": [
    {
      "id": "uuid",
      "nome": "Alice",
      "email": "alice@totvs.com",
      "avatar_url": "https://...",
      "atuacao_rastreada": 75.0
    }
  ]
}
```

### Calculation Formulas

**% Atuação Rastreada (per member):**
```
weekdays_in_period = count Mon-Fri in [inicio, fim]
total_horas_possiveis = weekdays_in_period × 8
horas_ausencia = dias_ausencia_uteis × 8
total_horas_reais = total_horas_possiveis - horas_ausencia
total_horas_cards = SUM(estimativa_tempo) / 3600
% = (total_horas_cards / total_horas_reais) × 100
```
Team average = mean of all members' %.

**% Metas / Compromissos / Iniciativas (team-level):**
```
total_team_seconds = SUM(estimativa_tempo) for all team cards in period
% = SUM(estimativa_tempo WHERE tipo_demanda = X) / total_team_seconds × 100
```

**Detalhes Iniciativas (within Iniciativas only):**
```
total_iniciativas_seconds = SUM(estimativa_tempo WHERE tipo_demanda = 'Iniciativa')
% Manutenção = SUM(WHERE tipo IN ('Bug','Incidente')) / total_iniciativas × 100  [red]
% Melhorias  = SUM(WHERE tipo = 'Melhoria') / total_iniciativas × 100
% Evolução = SUM(WHERE tipo = 'História') / total_iniciativas × 100
% Suporte = SUM(WHERE tipo IN ('Suporte','Tarefa')) / total_iniciativas × 100
```
