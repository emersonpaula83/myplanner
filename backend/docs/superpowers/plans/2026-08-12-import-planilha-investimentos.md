# Import Planilha Investimentos Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user import member financial data (salary, admission date, matrícula, cargo, manager, last raise) from a spreadsheet (CSV upload or public Google Sheets URL) into the Investimentos module, resolving unmatched names/teams/managers by hand before confirming, with a repeatable Sync button for future updates.

**Architecture:** SP1 (backend) adds `matricula`, `ultimo_aumento`, `gestor_id` to `membros` and a single-row `import_configs` table; a pure CSV parser (`internal/service/import_parser.go`) turns spreadsheet text into typed rows; a pure matcher (`internal/service/import_match.go`) matches rows against existing membros/equipes by normalized name; `ImportService` wires DB reads/writes and Google Sheets fetching around those pure functions; `ImportHandler` exposes 4 REST endpoints under `/investimentos/import*`. SP2 (frontend) adds an Importar button + tabbed upload modal, a resolution modal with dropdowns for unmatched entities and a live preview table, and a Sync button that reruns the last import automatically — all inside the existing single-file `frontend/index.html`.

**Tech Stack:** Go, pgxpool, chi router, zap logger, pgx v5, stdlib `encoding/csv` — vanilla JavaScript/HTML/CSS on the frontend, no frameworks.

## Global Constraints

- Follow existing handler → service → repository layering — no shortcuts.
- Handler interfaces are defined at the consumer (handler package), not the provider — follow the `InvestimentoStore` / `MembroFinanceiroStore` pattern in `internal/handler/investimento.go`.
- Response/request structs live in `internal/domain/`, following the `internal/domain/investimento.go` pattern.
- Migrations are numbered sequentially from `000029` (last existing is `000028_investimentos_campos`).
- All SQL uses parameterized queries (`$1`, `$2`, …).
- Backend tests use stdlib `testing` only — no external test frameworks. DB-touching repository/service code is not unit-tested (no test DB in this repo); pure functions (parsing, matching, cargo extraction) must be extracted so they *are* unit-testable, following the `calcTempoCasaMeses` / `sortMembrosBySalarioDesc` pattern in `internal/service/investimento.go`.
- `respondJSON` / `respondError` from `internal/handler/response.go` for all HTTP responses.
- UUID parsing via `chi.URLParam` + `uuid.Parse` for path params.
- Import never creates or deletes membros — only updates existing ones (membros are sourced from Jira sync). It may create new equipes (explicit user action) and associate a membro to one.
- Google Sheets URL must be public ("qualquer pessoa com o link pode ver"); if private, the backend returns a clear, actionable error. CSV upload always works regardless of sharing settings.
- Only one `import_configs` row exists at a time — saving a new config replaces the old one (delete + insert in a transaction).
- Frontend: single monolithic file `frontend/index.html` (~8000 lines), vanilla JS/HTML/CSS, no frameworks, no build step, no automated JS tests — verification is manual in-browser.
- Frontend HTTP calls use the existing `api()` wrapper (`frontend/index.html:2008`) for JSON, and a new `apiUpload()` helper for `multipart/form-data` (must not set a `Content-Type` header manually — the browser sets the multipart boundary).
- Dark/light theme via CSS variables (`var(--bg)`, `var(--surface)`, `var(--text-primary)`, `var(--border)`, etc.) — never hardcode colors.
- Modal pattern: `<div class="modal-overlay" id="xxx-modal" onclick="if(event.target===this)closeXxxModal()">` + `.open` class toggle, matching `frontend/index.html:1519` (`investimento-modal`).
- CSS insertion point: before `</style>` at `frontend/index.html:989`.
- JS insertion point: after `closeInvestimentoModal()` (ends at `frontend/index.html:7957`) and before the `// === INIT ===` block.

---

### Task 1: Migration — membros fields + import_configs table

**Files:**
- Create: `backend/migrations/000029_import_planilha_investimentos.up.sql`
- Create: `backend/migrations/000029_import_planilha_investimentos.down.sql`
- Modify: `backend/internal/domain/models.go:31-47` (Membro struct)
- Modify: `backend/internal/repository/membro.go:23-44` (`List`)
- Modify: `backend/internal/repository/membro.go:46-59` (`GetByID`)
- Modify: `backend/internal/repository/membro.go:191-214` (`Search`)

**Interfaces:**
- Consumes: nothing
- Produces: DB columns `membros.matricula`, `membros.ultimo_aumento`, `membros.gestor_id`; table `import_configs`; `domain.Membro.Matricula *string`, `domain.Membro.UltimoAumento *time.Time`, `domain.Membro.GestorID *uuid.UUID` (consumed by Tasks 2-4)

- [ ] **Step 1: Write the up migration**

```sql
-- backend/migrations/000029_import_planilha_investimentos.up.sql
ALTER TABLE membros ADD COLUMN matricula VARCHAR(20);
ALTER TABLE membros ADD COLUMN ultimo_aumento DATE;
ALTER TABLE membros ADD COLUMN gestor_id UUID REFERENCES membros(id);

CREATE TABLE import_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tipo VARCHAR(20) NOT NULL, -- 'csv' or 'sheets_url'
    url TEXT,
    gid VARCHAR(20),
    ultimo_sync TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

- [ ] **Step 2: Write the down migration**

```sql
-- backend/migrations/000029_import_planilha_investimentos.down.sql
DROP TABLE IF EXISTS import_configs;
ALTER TABLE membros DROP COLUMN IF EXISTS gestor_id;
ALTER TABLE membros DROP COLUMN IF EXISTS ultimo_aumento;
ALTER TABLE membros DROP COLUMN IF EXISTS matricula;
```

- [ ] **Step 3: Run the migration**

```bash
cd /home/emerson/code/myplanner/backend && psql "$DATABASE_URL" -f migrations/000029_import_planilha_investimentos.up.sql
```

- [ ] **Step 4: Verify schema**

```bash
psql "$DATABASE_URL" -c "\d membros" | grep -E "matricula|ultimo_aumento|gestor_id"
psql "$DATABASE_URL" -c "\d import_configs"
```

Expected: `matricula` (character varying(20)), `ultimo_aumento` (date), `gestor_id` (uuid, references membros) on `membros`; `import_configs` table with columns `id, tipo, url, gid, ultimo_sync, created_at`.

- [ ] **Step 5: Update `domain.Membro` struct**

In `backend/internal/domain/models.go:31-47`, add 3 fields after `BancoHoras`:

```go
type Membro struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	FonteDadosID     uuid.UUID  `json:"fonte_dados_id" db:"fonte_dados_id"`
	JiraAccountID    string     `json:"jira_account_id" db:"jira_account_id"`
	Nome             string     `json:"nome" db:"nome"`
	Email            *string    `json:"email" db:"email"`
	AvatarURL        *string    `json:"avatar_url" db:"avatar_url"`
	Team             *string    `json:"team" db:"team"`
	Ativo            bool       `json:"ativo" db:"ativo"`
	DataDesligamento *time.Time `json:"data_desligamento" db:"data_desligamento"`
	Cargo            *string    `json:"cargo" db:"cargo"`
	Salario          *float64   `json:"salario,omitempty" db:"salario"`
	DataAdmissao     *time.Time `json:"data_admissao,omitempty" db:"data_admissao"`
	BancoHoras       *float64   `json:"banco_horas,omitempty" db:"banco_horas"`
	Matricula        *string    `json:"matricula,omitempty" db:"matricula"`
	UltimoAumento    *time.Time `json:"ultimo_aumento,omitempty" db:"ultimo_aumento"`
	GestorID         *uuid.UUID `json:"gestor_id,omitempty" db:"gestor_id"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}
```

- [ ] **Step 6: Add the 3 columns to `MembroRepository.List`**

In `backend/internal/repository/membro.go:23-44`, replace the method body:

```go
func (r *MembroRepository) List(ctx context.Context) ([]domain.Membro, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo, data_desligamento, cargo, salario, data_admissao, banco_horas, matricula, ultimo_aumento, gestor_id, created_at, updated_at
		FROM membros
		WHERE ativo = true
		ORDER BY nome
	`)
	if err != nil {
		return nil, fmt.Errorf("listing membros: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Membro, 0)
	for rows.Next() {
		var m domain.Membro
		if err := rows.Scan(&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email, &m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo, &m.Salario, &m.DataAdmissao, &m.BancoHoras, &m.Matricula, &m.UltimoAumento, &m.GestorID, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning membro: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}
```

- [ ] **Step 7: Add the 3 columns to `MembroRepository.GetByID`**

In `backend/internal/repository/membro.go:46-59`, replace the method body:

```go
func (r *MembroRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
	var m domain.Membro
	err := r.pool.QueryRow(ctx, `
		SELECT id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo, data_desligamento, cargo, salario, data_admissao, banco_horas, matricula, ultimo_aumento, gestor_id, created_at, updated_at
		FROM membros WHERE id = $1
	`, id).Scan(&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email, &m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo, &m.Salario, &m.DataAdmissao, &m.BancoHoras, &m.Matricula, &m.UltimoAumento, &m.GestorID, &m.CreatedAt, &m.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting membro: %w", err)
	}
	return &m, nil
}
```

- [ ] **Step 8: Add the 3 columns to `MembroRepository.Search`**

In `backend/internal/repository/membro.go:191-214`, replace the method body:

```go
func (r *MembroRepository) Search(ctx context.Context, query string) ([]domain.Membro, error) {
	pattern := "%" + query + "%"
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo, data_desligamento, cargo, salario, data_admissao, banco_horas, matricula, ultimo_aumento, gestor_id, created_at, updated_at
		FROM membros
		WHERE ativo = true AND (nome ILIKE $1 OR email ILIKE $1)
		ORDER BY nome
		LIMIT 50
	`, pattern)
	if err != nil {
		return nil, fmt.Errorf("searching membros: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Membro, 0)
	for rows.Next() {
		var m domain.Membro
		if err := rows.Scan(&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email, &m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo, &m.Salario, &m.DataAdmissao, &m.BancoHoras, &m.Matricula, &m.UltimoAumento, &m.GestorID, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning membro: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}
```

- [ ] **Step 9: Build and verify**

```bash
cd /home/emerson/code/myplanner/backend && go build ./...
```

Expected: exits 0, no errors.

- [ ] **Step 10: Commit**

```bash
cd /home/emerson/code/myplanner/backend
git add migrations/000029_import_planilha_investimentos.up.sql migrations/000029_import_planilha_investimentos.down.sql internal/domain/models.go internal/repository/membro.go
git commit -m "feat: add matricula, ultimo_aumento, gestor_id fields and import_configs table"
```

---

### Task 2: CSV parser service

**Files:**
- Create: `backend/internal/domain/import.go`
- Create: `backend/internal/service/import_parser.go`
- Test: `backend/internal/service/import_parser_test.go`

**Interfaces:**
- Consumes: nothing (pure functions, no DB)
- Produces: `domain.ImportPlanilhaLinha`, `domain.ImportIgnorado`, `domain.ImportParseResult`; `service.ParseCSVPlanilha(csvContent string) (*domain.ImportParseResult, error)`, `service.ParseSalarioBR(s string) (*float64, error)`, `service.ParseDataPlanilha(s string) (*string, error)`, `service.ExtractCargoNivel(funcao string) *string`, `service.NormalizeNome(s string) string` (all consumed by Task 3)

- [ ] **Step 1: Create `internal/domain/import.go` with parse-result types**

```go
package domain

// ImportPlanilhaLinha is a single parsed, non-ignored row from the import
// spreadsheet. Dates are pre-formatted as YYYY-MM-DD strings so they can be
// echoed back to the frontend and re-parsed at confirm time without losing
// precision.
type ImportPlanilhaLinha struct {
	Linha         int
	Nome          string
	Gestao        string
	TimeSquad     string
	Funcao        string
	Matricula     *string
	Admissao      *string
	Salario       *float64
	UltimoAumento *string
}

// ImportIgnorado is a spreadsheet row that was intentionally skipped (a SUB
// line or the trailing total-count row).
type ImportIgnorado struct {
	Linha  int    `json:"linha"`
	Nome   string `json:"nome"`
	Motivo string `json:"motivo"`
}

// ImportParseResult is the output of parsing raw CSV content, before any
// database matching happens.
type ImportParseResult struct {
	Linhas    []ImportPlanilhaLinha
	Ignorados []ImportIgnorado
}
```

- [ ] **Step 2: Write the failing tests for the pure helpers**

```go
// backend/internal/service/import_parser_test.go
package service

import (
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

func TestParseSalarioBR(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    *float64
		wantErr bool
	}{
		{"simple value", "R$ 6.480,00", floatPtr(6480.00), false},
		{"no thousands separator", "R$ 950,50", floatPtr(950.50), false},
		{"large value", "R$ 12.500,00", floatPtr(12500.00), false},
		{"dash means null", "-", nil, false},
		{"empty means null", "", nil, false},
		{"invalid", "abc", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSalarioBR(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.want == nil {
				if got != nil {
					t.Errorf("got %v, want nil", *got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Errorf("got %v, want %v", got, *tt.want)
			}
		})
	}
}

func TestParseDataPlanilha(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    *string
		wantErr bool
	}{
		{"dd/mm/yyyy", "18/05/2026", strPtr("2026-05-18"), false},
		{"excel serial", "46083", strPtr("2026-03-02"), false},
		{"dash means null", "-", nil, false},
		{"empty means null", "", nil, false},
		{"invalid", "not-a-date", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDataPlanilha(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.want == nil {
				if got != nil {
					t.Errorf("got %v, want nil", *got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Errorf("got %v, want %v", got, *tt.want)
			}
		})
	}
}

func TestExtractCargoNivel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want *string
	}{
		{"analista i", "ANALISTA I DE SUPORTE", strPtr(domain.CargoAnalistaI)},
		{"analista i end of string", "ANALISTA DE SUPORTE ANALISTA I", strPtr(domain.CargoAnalistaI)},
		{"analista ii", "ANALISTA II DE DESENVOLVIMENTO CLOUD", strPtr(domain.CargoAnalistaII)},
		{"analista iii", "ANALISTA III DE DADOS", strPtr(domain.CargoAnalistaIII)},
		{"especialista i", "ESPECIALISTA I DE DADOS", strPtr(domain.CargoEspecialistaI)},
		{"especialista ii", "ESPECIALISTA II DE DADOS", strPtr(domain.CargoEspecialistaII)},
		{"master", "MASTER DE ENGENHARIA", strPtr(domain.CargoMaster)},
		{"coordenador", "COORDENADOR DE TECNOLOGIA", strPtr(domain.CargoCoordenadorDesenvolvimento)},
		{"lider accented", "LÍDER TÉCNICO", strPtr(domain.CargoLiderTecnico)},
		{"lider unaccented", "LIDER DE EQUIPE", strPtr(domain.CargoLiderTecnico)},
		{"tecnico accented", "TÉCNICO DE SUPORTE", strPtr(domain.CargoAnalistaI)},
		{"tecnico unaccented", "TECNICO DE INFRAESTRUTURA", strPtr(domain.CargoAnalistaI)},
		{"no match", "GERENTE DE PROJETOS", nil},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractCargoNivel(tt.in)
			if tt.want == nil {
				if got != nil {
					t.Errorf("got %v, want nil", *got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Errorf("got %v, want %v", got, *tt.want)
			}
		})
	}
}

func TestParseCSVPlanilha(t *testing.T) {
	csvContent := "Nome,Gestão,Time / Squad,Função,Matrícula,Admissão,Salário,Último Aumento\n" +
		"RICARDO KAZUO DINIZ NOZAKI,Angela Kanegae Oda,DEVOPS RM,ANALISTA II DE DESENVOLVIMENTO CLOUD,000101016701,18/05/2026,\"R$ 6.480,00\",01/01/2026\n" +
		"SUB 167064 - AGILISTA,Angela Kanegae Oda,DEVOPS RM,AGILISTA,-,-,\"R$ 0,00\",-\n" +
		"FULANO DE TAL,Novo Gestor,DEVOPS NOVA,ESPECIALISTA I DE DADOS,-,10/03/2026,\"R$ 8.000,00\",46083\n" +
		"3,,,,,,,\n"

	result, err := ParseCSVPlanilha(csvContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Linhas) != 2 {
		t.Fatalf("got %d linhas, want 2", len(result.Linhas))
	}
	if len(result.Ignorados) != 2 {
		t.Fatalf("got %d ignorados, want 2", len(result.Ignorados))
	}
	if result.Ignorados[0].Motivo != "SUB" {
		t.Errorf("ignorados[0].Motivo = %q, want SUB", result.Ignorados[0].Motivo)
	}
	if result.Ignorados[1].Motivo != "total" {
		t.Errorf("ignorados[1].Motivo = %q, want total", result.Ignorados[1].Motivo)
	}

	ricardo := result.Linhas[0]
	if ricardo.Nome != "RICARDO KAZUO DINIZ NOZAKI" {
		t.Errorf("ricardo.Nome = %q", ricardo.Nome)
	}
	if ricardo.Matricula == nil || *ricardo.Matricula != "000101016701" {
		t.Errorf("ricardo.Matricula = %v, want 000101016701", ricardo.Matricula)
	}
	if ricardo.Salario == nil || *ricardo.Salario != 6480.00 {
		t.Errorf("ricardo.Salario = %v, want 6480.00", ricardo.Salario)
	}
	if ricardo.Admissao == nil || *ricardo.Admissao != "2026-05-18" {
		t.Errorf("ricardo.Admissao = %v, want 2026-05-18", ricardo.Admissao)
	}

	fulano := result.Linhas[1]
	if fulano.Matricula != nil {
		t.Errorf("fulano.Matricula = %v, want nil (dash)", *fulano.Matricula)
	}
	if fulano.UltimoAumento == nil || *fulano.UltimoAumento != "2026-03-02" {
		t.Errorf("fulano.UltimoAumento = %v, want 2026-03-02 (excel serial 46083)", fulano.UltimoAumento)
	}
	if fulano.TimeSquad != "DEVOPS NOVA" {
		t.Errorf("fulano.TimeSquad = %q", fulano.TimeSquad)
	}
}

func floatPtr(f float64) *float64 { return &f }
func strPtr(s string) *string     { return &s }
```

- [ ] **Step 3: Run tests to verify they fail (package doesn't compile yet)**

```bash
cd /home/emerson/code/myplanner/backend && go test ./internal/service/... -run "TestParseSalarioBR|TestParseDataPlanilha|TestExtractCargoNivel|TestParseCSVPlanilha" -v
```

Expected: FAIL — `undefined: ParseSalarioBR` (and similar).

- [ ] **Step 4: Implement the parser**

```go
// backend/internal/service/import_parser.go
package service

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

// ParseSalarioBR parses a Brazilian currency string like "R$ 6.480,00" into
// a float64. "-" and "" are treated as null (returns nil, nil).
func ParseSalarioBR(s string) (*float64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil, nil
	}
	s = strings.ReplaceAll(s, "R$", "")
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("salário inválido %q: %w", s, err)
	}
	return &v, nil
}

// ParseDataPlanilha parses a spreadsheet date cell: dd/mm/yyyy, an Excel
// serial day count (epoch 1899-12-30), or "-"/"" for null. Returns the date
// formatted as YYYY-MM-DD.
func ParseDataPlanilha(s string) (*string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil, nil
	}
	if t, err := time.Parse("02/01/2006", s); err == nil {
		formatted := t.Format("2006-01-02")
		return &formatted, nil
	}
	if serial, err := strconv.Atoi(s); err == nil {
		epoch := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
		t := epoch.AddDate(0, 0, serial)
		formatted := t.Format("2006-01-02")
		return &formatted, nil
	}
	return nil, fmt.Errorf("data inválida %q", s)
}

// NormalizeNome uppercases and collapses whitespace so names can be compared
// case-insensitively regardless of extra spacing.
func NormalizeNome(s string) string {
	fields := strings.Fields(strings.ToUpper(strings.TrimSpace(s)))
	return strings.Join(fields, " ")
}

type cargoRule struct {
	label string
	slug  string
	exact bool // true: must be a whole word (suffix or followed by a space); false: plain substring
}

var cargoNivelRules = []cargoRule{
	{"ANALISTA I", domain.CargoAnalistaI, true},
	{"ANALISTA II", domain.CargoAnalistaII, true},
	{"ANALISTA III", domain.CargoAnalistaIII, false},
	{"ESPECIALISTA I", domain.CargoEspecialistaI, true},
	{"ESPECIALISTA II", domain.CargoEspecialistaII, false},
	{"MASTER", domain.CargoMaster, false},
	{"COORDENADOR", domain.CargoCoordenadorDesenvolvimento, false},
	{"LÍDER", domain.CargoLiderTecnico, false},
	{"LIDER", domain.CargoLiderTecnico, false},
	{"TÉCNICO", domain.CargoAnalistaI, false},
	{"TECNICO", domain.CargoAnalistaI, false},
}

// ExtractCargoNivel maps a full spreadsheet job title (e.g. "ANALISTA II DE
// DESENVOLVIMENTO CLOUD") to a system cargo slug. Returns nil when no rule
// matches, so the caller can leave the member's existing cargo untouched.
func ExtractCargoNivel(funcao string) *string {
	upper := strings.ToUpper(strings.TrimSpace(funcao))
	if upper == "" {
		return nil
	}
	for _, rule := range cargoNivelRules {
		matched := false
		if rule.exact {
			matched = strings.HasSuffix(upper, rule.label) || strings.Contains(upper, rule.label+" ")
		} else {
			matched = strings.Contains(upper, rule.label)
		}
		if matched {
			slug := rule.slug
			return &slug
		}
	}
	return nil
}

var csvHeaderAliases = map[string]string{
	"NOME":            "nome",
	"GESTAO":          "gestao",
	"GESTÃO":          "gestao",
	"TIME / SQUAD":    "time_squad",
	"TIME/SQUAD":      "time_squad",
	"FUNCAO":          "funcao",
	"FUNÇÃO":          "funcao",
	"MATRICULA":       "matricula",
	"MATRÍCULA":       "matricula",
	"ADMISSAO":        "admissao",
	"ADMISSÃO":        "admissao",
	"SALARIO":         "salario",
	"SALÁRIO":         "salario",
	"ULTIMO AUMENTO":  "ultimo_aumento",
	"ÚLTIMO AUMENTO":  "ultimo_aumento",
}

// ParseCSVPlanilha parses the raw CSV export of the investment spreadsheet.
// It locates the header row by scanning for a "Nome" cell (the header may be
// on line 1 or further down, per the original spreadsheet layout), then
// reads data rows, skipping blank lines, SUB rows, and the trailing total
// row.
func ParseCSVPlanilha(csvContent string) (*domain.ImportParseResult, error) {
	reader := csv.NewReader(strings.NewReader(csvContent))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading csv: %w", err)
	}

	headerIdx := -1
	colIdx := map[string]int{}
	for i, rec := range records {
		if len(rec) == 0 {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(rec[0])) == "NOME" {
			headerIdx = i
			for j, cell := range rec {
				key := strings.ToUpper(strings.TrimSpace(cell))
				if norm, ok := csvHeaderAliases[key]; ok {
					colIdx[norm] = j
				}
			}
			break
		}
	}
	if headerIdx == -1 {
		return nil, fmt.Errorf("cabeçalho não encontrado (coluna 'Nome' não localizada)")
	}

	result := &domain.ImportParseResult{
		Linhas:    []domain.ImportPlanilhaLinha{},
		Ignorados: []domain.ImportIgnorado{},
	}

	linha := 0
	for _, rec := range records[headerIdx+1:] {
		if isBlankRow(rec) {
			continue
		}
		linha++

		nome := strings.TrimSpace(cellAt(rec, colIdx, "nome"))
		if nome == "" {
			linha--
			continue
		}

		if strings.Contains(strings.ToUpper(nome), "SUB") {
			result.Ignorados = append(result.Ignorados, domain.ImportIgnorado{Linha: linha, Nome: nome, Motivo: "SUB"})
			continue
		}
		if _, err := strconv.Atoi(nome); err == nil {
			result.Ignorados = append(result.Ignorados, domain.ImportIgnorado{Linha: linha, Nome: nome, Motivo: "total"})
			continue
		}

		matriculaRaw := strings.TrimSpace(cellAt(rec, colIdx, "matricula"))
		var matricula *string
		if matriculaRaw != "" && matriculaRaw != "-" {
			matricula = &matriculaRaw
		}

		salario, err := ParseSalarioBR(cellAt(rec, colIdx, "salario"))
		if err != nil {
			return nil, fmt.Errorf("linha %d: %w", linha, err)
		}
		admissao, err := ParseDataPlanilha(cellAt(rec, colIdx, "admissao"))
		if err != nil {
			return nil, fmt.Errorf("linha %d: %w", linha, err)
		}
		ultimoAumento, err := ParseDataPlanilha(cellAt(rec, colIdx, "ultimo_aumento"))
		if err != nil {
			return nil, fmt.Errorf("linha %d: %w", linha, err)
		}

		result.Linhas = append(result.Linhas, domain.ImportPlanilhaLinha{
			Linha:         linha,
			Nome:          nome,
			Gestao:        strings.TrimSpace(cellAt(rec, colIdx, "gestao")),
			TimeSquad:     strings.TrimSpace(cellAt(rec, colIdx, "time_squad")),
			Funcao:        strings.TrimSpace(cellAt(rec, colIdx, "funcao")),
			Matricula:     matricula,
			Admissao:      admissao,
			Salario:       salario,
			UltimoAumento: ultimoAumento,
		})
	}

	return result, nil
}

func cellAt(rec []string, colIdx map[string]int, key string) string {
	idx, ok := colIdx[key]
	if !ok || idx >= len(rec) {
		return ""
	}
	return rec[idx]
}

func isBlankRow(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd /home/emerson/code/myplanner/backend && go test ./internal/service/... -run "TestParseSalarioBR|TestParseDataPlanilha|TestExtractCargoNivel|TestParseCSVPlanilha" -v
```

Expected: all subtests PASS.

- [ ] **Step 6: Full build check**

```bash
cd /home/emerson/code/myplanner/backend && go build ./... && go vet ./...
```

Expected: exits 0.

- [ ] **Step 7: Commit**

```bash
cd /home/emerson/code/myplanner/backend
git add internal/domain/import.go internal/service/import_parser.go internal/service/import_parser_test.go
git commit -m "feat: add CSV spreadsheet parser for investimentos import (currency, dates, cargo extraction)"
```

---

### Task 3: Import handler + matching logic

**Files:**
- Modify: `backend/internal/domain/import.go` (append match-result types)
- Create: `backend/internal/service/import_match.go`
- Test: `backend/internal/service/import_match_test.go`
- Create: `backend/internal/service/import.go`
- Create: `backend/internal/handler/import.go`
- Test: `backend/internal/handler/import_test.go`
- Modify: `backend/cmd/api/main.go:186` (wiring), `backend/cmd/api/main.go:261` (route)

**Interfaces:**
- Consumes: `service.ParseCSVPlanilha`, `service.ExtractCargoNivel`, `service.NormalizeNome` (Task 2); `domain.Membro.{ID,Nome,Salario,DataAdmissao,Cargo,Matricula,UltimoAumento}` (Task 1); `repository.MembroRepository.List`, `repository.EquipeRepository.ListEquipes` (existing); `domain.Equipe{ID,Nome}` (existing)
- Produces: `domain.ImportDados`, `domain.ImportMatched`, `domain.ImportUnmatchedMembro`, `domain.ImportUnmatchedEquipe`, `domain.ImportUnmatchedGestor`, `domain.ImportMatchResult`; `service.MatchLinhas(linhas []domain.ImportPlanilhaLinha, ignorados []domain.ImportIgnorado, membros []domain.Membro, equipes []domain.Equipe) *domain.ImportMatchResult`; `service.ExtractSheetsIDAndGid(sheetsURL string) (id, gid string, err error)`; `*service.ImportService` with `MatchPlanilha(ctx, csvContent string) (*domain.ImportMatchResult, error)` and `FetchGoogleSheetCSV(ctx, sheetsURL string) (csvContent, id, gid string, err error)`; `handler.ImportStore` interface; `POST /investimentos/import` (consumed by Task 5 frontend)

- [ ] **Step 1: Append match-result types to `internal/domain/import.go`**

```go
// Append to backend/internal/domain/import.go (after ImportParseResult)

// ImportDados holds the spreadsheet-derived financial fields for one row,
// after cargo-slug extraction and gestor lookup.
type ImportDados struct {
	Cargo         *string    `json:"cargo"`
	Matricula     *string    `json:"matricula"`
	Salario       *float64   `json:"salario"`
	DataAdmissao  *string    `json:"data_admissao"`
	UltimoAumento *string    `json:"ultimo_aumento"`
	GestorNome    string     `json:"gestor_nome"`
	GestorID      *uuid.UUID `json:"gestor_id"`
}

// ImportMatched is a spreadsheet row whose Nome matched an existing membro.
// EquipeID/GestorID may still be nil if that particular sub-value wasn't
// found — those cases also appear in UnmatchedEquipes/UnmatchedGestores.
type ImportMatched struct {
	Linha        int         `json:"linha"`
	NomePlanilha string      `json:"nome_planilha"`
	MembroID     uuid.UUID   `json:"membro_id"`
	MembroNome   string      `json:"membro_nome"`
	EquipeID     *uuid.UUID  `json:"equipe_id"`
	EquipeNome   string      `json:"equipe_nome"`
	Dados        ImportDados `json:"dados"`
	Changes      []string    `json:"changes"`
}

// ImportUnmatchedMembro is a spreadsheet row whose Nome did not match any
// existing membro.
type ImportUnmatchedMembro struct {
	Linha        int         `json:"linha"`
	NomePlanilha string      `json:"nome_planilha"`
	Dados        ImportDados `json:"dados"`
}

// ImportUnmatchedEquipe groups spreadsheet row numbers by a Time/Squad name
// that didn't match any existing equipe.
type ImportUnmatchedEquipe struct {
	NomePlanilha string `json:"nome_planilha"`
	Linhas       []int  `json:"linhas"`
}

// ImportUnmatchedGestor groups spreadsheet row numbers by a Gestão name that
// didn't match any existing membro.
type ImportUnmatchedGestor struct {
	NomePlanilha string `json:"nome_planilha"`
	Linhas       []int  `json:"linhas"`
}

// ImportMatchResult is the full response of POST /investimentos/import (and
// /investimentos/import/sync).
type ImportMatchResult struct {
	Matched           []ImportMatched         `json:"matched"`
	UnmatchedMembros  []ImportUnmatchedMembro `json:"unmatched_membros"`
	UnmatchedEquipes  []ImportUnmatchedEquipe `json:"unmatched_equipes"`
	UnmatchedGestores []ImportUnmatchedGestor `json:"unmatched_gestores"`
	Ignorados         []ImportIgnorado        `json:"ignorados"`
}
```

`internal/domain/import.go` had no imports after Task 2 (its types only use `string`/`int`/`*float64`). This step introduces `uuid.UUID`, so add an import block at the very top of the file, right after `package domain`:

```go
import "github.com/google/uuid"
```

- [ ] **Step 2: Write the failing test for the pure matcher**

```go
// backend/internal/service/import_match_test.go
package service

import (
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
)

func TestMatchLinhas_MatchByNameCaseInsensitive(t *testing.T) {
	membroID := uuid.New()
	membros := []domain.Membro{{ID: membroID, Nome: "Ricardo Kazuo Diniz Nozaki"}}
	equipes := []domain.Equipe{}
	salario := 6480.00
	linhas := []domain.ImportPlanilhaLinha{
		{Linha: 1, Nome: "RICARDO KAZUO DINIZ NOZAKI", Salario: &salario},
	}

	result := MatchLinhas(linhas, nil, membros, equipes)

	if len(result.Matched) != 1 {
		t.Fatalf("got %d matched, want 1", len(result.Matched))
	}
	if result.Matched[0].MembroID != membroID {
		t.Errorf("MembroID = %v, want %v", result.Matched[0].MembroID, membroID)
	}
	if len(result.UnmatchedMembros) != 0 {
		t.Errorf("got %d unmatched membros, want 0", len(result.UnmatchedMembros))
	}
}

func TestMatchLinhas_UnmatchedMembro(t *testing.T) {
	membros := []domain.Membro{{ID: uuid.New(), Nome: "Outra Pessoa"}}
	linhas := []domain.ImportPlanilhaLinha{{Linha: 5, Nome: "Fulano De Tal"}}

	result := MatchLinhas(linhas, nil, membros, nil)

	if len(result.Matched) != 0 {
		t.Fatalf("got %d matched, want 0", len(result.Matched))
	}
	if len(result.UnmatchedMembros) != 1 || result.UnmatchedMembros[0].Linha != 5 {
		t.Fatalf("unexpected unmatched membros: %+v", result.UnmatchedMembros)
	}
}

func TestMatchLinhas_UnmatchedEquipeGroupsLinhas(t *testing.T) {
	m1, m2 := uuid.New(), uuid.New()
	membros := []domain.Membro{
		{ID: m1, Nome: "Pessoa Um"},
		{ID: m2, Nome: "Pessoa Dois"},
	}
	linhas := []domain.ImportPlanilhaLinha{
		{Linha: 1, Nome: "Pessoa Um", TimeSquad: "DEVOPS NOVA"},
		{Linha: 2, Nome: "Pessoa Dois", TimeSquad: "DEVOPS NOVA"},
	}

	result := MatchLinhas(linhas, nil, membros, nil)

	if len(result.Matched) != 2 {
		t.Fatalf("got %d matched, want 2", len(result.Matched))
	}
	if result.Matched[0].EquipeID != nil {
		t.Errorf("expected nil EquipeID for unmatched squad")
	}
	if len(result.UnmatchedEquipes) != 1 {
		t.Fatalf("got %d unmatched equipes, want 1", len(result.UnmatchedEquipes))
	}
	if got := result.UnmatchedEquipes[0].Linhas; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("UnmatchedEquipes[0].Linhas = %v, want [1 2]", got)
	}
}

func TestMatchLinhas_UnmatchedGestor(t *testing.T) {
	m1 := uuid.New()
	membros := []domain.Membro{{ID: m1, Nome: "Pessoa Um"}}
	linhas := []domain.ImportPlanilhaLinha{
		{Linha: 3, Nome: "Pessoa Um", Gestao: "Novo Gestor"},
	}

	result := MatchLinhas(linhas, nil, membros, nil)

	if len(result.UnmatchedGestores) != 1 || result.UnmatchedGestores[0].NomePlanilha != "Novo Gestor" {
		t.Fatalf("unexpected unmatched gestores: %+v", result.UnmatchedGestores)
	}
	if result.Matched[0].Dados.GestorID != nil {
		t.Errorf("expected nil GestorID for unmatched gestor")
	}
}

func TestComputeChanges(t *testing.T) {
	salarioAtual := 5000.0
	m := domain.Membro{Salario: &salarioAtual}
	novoSalario := 6000.0
	dados := domain.ImportDados{Salario: &novoSalario}

	changes := computeChanges(m, dados)

	if len(changes) != 1 || changes[0] != "salario" {
		t.Errorf("changes = %v, want [salario]", changes)
	}
}

func TestComputeChanges_NoChange(t *testing.T) {
	salarioAtual := 5000.0
	m := domain.Membro{Salario: &salarioAtual}
	dados := domain.ImportDados{Salario: &salarioAtual}

	changes := computeChanges(m, dados)

	if len(changes) != 0 {
		t.Errorf("changes = %v, want []", changes)
	}
}

func TestExtractSheetsIDAndGid(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantID  string
		wantGid string
		wantErr bool
	}{
		{"edit with hash gid", "https://docs.google.com/spreadsheets/d/1AbC-xyz_123/edit#gid=456", "1AbC-xyz_123", "456", false},
		{"edit no gid defaults to 0", "https://docs.google.com/spreadsheets/d/1AbC-xyz_123/edit", "1AbC-xyz_123", "0", false},
		{"query gid", "https://docs.google.com/spreadsheets/d/1AbC-xyz_123/edit?gid=789#gid=789", "1AbC-xyz_123", "789", false},
		{"invalid url", "https://example.com/not-a-sheet", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, gid, err := ExtractSheetsIDAndGid(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if id != tt.wantID || gid != tt.wantGid {
				t.Errorf("got id=%q gid=%q, want id=%q gid=%q", id, gid, tt.wantID, tt.wantGid)
			}
		})
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
cd /home/emerson/code/myplanner/backend && go test ./internal/service/... -run "TestMatchLinhas|TestComputeChanges|TestExtractSheetsIDAndGid" -v
```

Expected: FAIL — `undefined: MatchLinhas` (and similar).

- [ ] **Step 4: Implement the pure matcher**

```go
// backend/internal/service/import_match.go
package service

import (
	"strings"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
)

// MatchLinhas matches parsed spreadsheet rows against existing membros and
// equipes by normalized (case-insensitive, whitespace-collapsed) name. It is
// a pure function — no I/O — so it is unit-testable without a database.
func MatchLinhas(linhas []domain.ImportPlanilhaLinha, ignorados []domain.ImportIgnorado, membros []domain.Membro, equipes []domain.Equipe) *domain.ImportMatchResult {
	membroByNome := make(map[string]domain.Membro, len(membros))
	for _, m := range membros {
		membroByNome[NormalizeNome(m.Nome)] = m
	}
	equipeByNome := make(map[string]domain.Equipe, len(equipes))
	for _, e := range equipes {
		equipeByNome[NormalizeNome(e.Nome)] = e
	}

	result := &domain.ImportMatchResult{
		Matched:           []domain.ImportMatched{},
		UnmatchedMembros:  []domain.ImportUnmatchedMembro{},
		UnmatchedEquipes:  []domain.ImportUnmatchedEquipe{},
		UnmatchedGestores: []domain.ImportUnmatchedGestor{},
		Ignorados:         ignorados,
	}
	if result.Ignorados == nil {
		result.Ignorados = []domain.ImportIgnorado{}
	}

	unmatchedEquipeLinhas := map[string][]int{}
	var unmatchedEquipeOrder []string
	unmatchedGestorLinhas := map[string][]int{}
	var unmatchedGestorOrder []string

	for _, linha := range linhas {
		cargo := ExtractCargoNivel(linha.Funcao)

		var gestorID *uuid.UUID
		gestorNome := strings.TrimSpace(linha.Gestao)
		if gestorNome != "" {
			if g, ok := membroByNome[NormalizeNome(gestorNome)]; ok {
				id := g.ID
				gestorID = &id
			} else {
				if _, seen := unmatchedGestorLinhas[gestorNome]; !seen {
					unmatchedGestorOrder = append(unmatchedGestorOrder, gestorNome)
				}
				unmatchedGestorLinhas[gestorNome] = append(unmatchedGestorLinhas[gestorNome], linha.Linha)
			}
		}

		dados := domain.ImportDados{
			Cargo:         cargo,
			Matricula:     linha.Matricula,
			Salario:       linha.Salario,
			DataAdmissao:  linha.Admissao,
			UltimoAumento: linha.UltimoAumento,
			GestorNome:    gestorNome,
			GestorID:      gestorID,
		}

		membro, ok := membroByNome[NormalizeNome(linha.Nome)]
		if !ok {
			result.UnmatchedMembros = append(result.UnmatchedMembros, domain.ImportUnmatchedMembro{
				Linha:        linha.Linha,
				NomePlanilha: linha.Nome,
				Dados:        dados,
			})
			continue
		}

		var equipeID *uuid.UUID
		equipeNome := ""
		timeSquad := strings.TrimSpace(linha.TimeSquad)
		if timeSquad != "" {
			if eq, ok := equipeByNome[NormalizeNome(timeSquad)]; ok {
				id := eq.ID
				equipeID = &id
				equipeNome = eq.Nome
			} else {
				if _, seen := unmatchedEquipeLinhas[timeSquad]; !seen {
					unmatchedEquipeOrder = append(unmatchedEquipeOrder, timeSquad)
				}
				unmatchedEquipeLinhas[timeSquad] = append(unmatchedEquipeLinhas[timeSquad], linha.Linha)
			}
		}

		result.Matched = append(result.Matched, domain.ImportMatched{
			Linha:        linha.Linha,
			NomePlanilha: linha.Nome,
			MembroID:     membro.ID,
			MembroNome:   membro.Nome,
			EquipeID:     equipeID,
			EquipeNome:   equipeNome,
			Dados:        dados,
			Changes:      computeChanges(membro, dados),
		})
	}

	for _, nome := range unmatchedEquipeOrder {
		result.UnmatchedEquipes = append(result.UnmatchedEquipes, domain.ImportUnmatchedEquipe{NomePlanilha: nome, Linhas: unmatchedEquipeLinhas[nome]})
	}
	for _, nome := range unmatchedGestorOrder {
		result.UnmatchedGestores = append(result.UnmatchedGestores, domain.ImportUnmatchedGestor{NomePlanilha: nome, Linhas: unmatchedGestorLinhas[nome]})
	}

	return result
}

// computeChanges reports which financial fields will actually change if
// dados is applied to m, for the "Ação" column in the frontend preview.
func computeChanges(m domain.Membro, dados domain.ImportDados) []string {
	changes := []string{}
	if dados.Salario != nil && (m.Salario == nil || *m.Salario != *dados.Salario) {
		changes = append(changes, "salario")
	}
	if dados.DataAdmissao != nil {
		cur := ""
		if m.DataAdmissao != nil {
			cur = m.DataAdmissao.Format("2006-01-02")
		}
		if cur != *dados.DataAdmissao {
			changes = append(changes, "data_admissao")
		}
	}
	if dados.Cargo != nil && (m.Cargo == nil || *m.Cargo != *dados.Cargo) {
		changes = append(changes, "cargo")
	}
	if dados.Matricula != nil && (m.Matricula == nil || *m.Matricula != *dados.Matricula) {
		changes = append(changes, "matricula")
	}
	if dados.UltimoAumento != nil {
		cur := ""
		if m.UltimoAumento != nil {
			cur = m.UltimoAumento.Format("2006-01-02")
		}
		if cur != *dados.UltimoAumento {
			changes = append(changes, "ultimo_aumento")
		}
	}
	return changes
}
```

- [ ] **Step 5: Implement `ExtractSheetsIDAndGid` and `ImportService` in `internal/service/import.go`**

```go
// backend/internal/service/import.go
package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

var sheetsIDRegex = regexp.MustCompile(`/spreadsheets/d/([a-zA-Z0-9-_]+)`)
var sheetsGidRegex = regexp.MustCompile(`[#&?]gid=(\d+)`)

// ExtractSheetsIDAndGid pulls the spreadsheet ID and sheet gid out of a
// Google Sheets URL (any of the /edit, /edit#gid=, /edit?gid= forms).
// Defaults gid to "0" (the first sheet) when absent.
func ExtractSheetsIDAndGid(sheetsURL string) (id string, gid string, err error) {
	m := sheetsIDRegex.FindStringSubmatch(sheetsURL)
	if len(m) < 2 {
		return "", "", fmt.Errorf("URL do Google Sheets inválida")
	}
	id = m[1]
	gid = "0"
	if gm := sheetsGidRegex.FindStringSubmatch(sheetsURL); len(gm) >= 2 {
		gid = gm[1]
	}
	return id, gid, nil
}

type ImportService struct {
	membroRepo *repository.MembroRepository
	equipeRepo *repository.EquipeRepository
	httpClient *http.Client
	logger     *zap.Logger
}

func NewImportService(membroRepo *repository.MembroRepository, equipeRepo *repository.EquipeRepository, logger *zap.Logger) *ImportService {
	return &ImportService{
		membroRepo: membroRepo,
		equipeRepo: equipeRepo,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
	}
}

// MatchPlanilha parses csvContent and matches it against the current
// membros/equipes in the database.
func (s *ImportService) MatchPlanilha(ctx context.Context, csvContent string) (*domain.ImportMatchResult, error) {
	parsed, err := ParseCSVPlanilha(csvContent)
	if err != nil {
		return nil, fmt.Errorf("parsing csv: %w", err)
	}

	membros, err := s.membroRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing membros: %w", err)
	}
	equipes, err := s.equipeRepo.ListEquipes(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing equipes: %w", err)
	}

	return MatchLinhas(parsed.Linhas, parsed.Ignorados, membros, equipes), nil
}

// FetchGoogleSheetCSV downloads the CSV export of a public Google Sheet.
// Returns a clear error if the sheet isn't publicly shared (Google responds
// with an HTML sign-in page instead of CSV in that case).
func (s *ImportService) FetchGoogleSheetCSV(ctx context.Context, sheetsURL string) (csvContent string, id string, gid string, err error) {
	id, gid, err = ExtractSheetsIDAndGid(sheetsURL)
	if err != nil {
		return "", "", "", err
	}
	exportURL := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/gviz/tq?tqx=out:csv&gid=%s", id, gid)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, exportURL, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("creating request: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("fetching planilha: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("reading response: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if resp.StatusCode != http.StatusOK || !strings.Contains(contentType, "csv") {
		return "", "", "", fmt.Errorf("planilha não está pública. Configure o compartilhamento como \"qualquer pessoa com o link\" e tente novamente")
	}

	return string(body), id, gid, nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
cd /home/emerson/code/myplanner/backend && go test ./internal/service/... -run "TestMatchLinhas|TestComputeChanges|TestExtractSheetsIDAndGid" -v
```

Expected: all subtests PASS.

- [ ] **Step 7: Write the failing handler test**

```go
// backend/internal/handler/import_test.go
package handler

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"go.uber.org/zap"
)

type mockImportStore struct {
	matchPlanilhaFn       func(ctx context.Context, csvContent string) (*domain.ImportMatchResult, error)
	fetchGoogleSheetCSVFn func(ctx context.Context, sheetsURL string) (string, string, string, error)
}

func (m *mockImportStore) MatchPlanilha(ctx context.Context, csvContent string) (*domain.ImportMatchResult, error) {
	return m.matchPlanilhaFn(ctx, csvContent)
}
func (m *mockImportStore) FetchGoogleSheetCSV(ctx context.Context, sheetsURL string) (string, string, string, error) {
	return m.fetchGoogleSheetCSVFn(ctx, sheetsURL)
}

func newTestImportHandler(store *mockImportStore) *ImportHandler {
	return NewImportHandler(store, zap.NewNop())
}

func TestImport_Multipart_Success(t *testing.T) {
	var receivedCSV string
	store := &mockImportStore{
		matchPlanilhaFn: func(_ context.Context, csvContent string) (*domain.ImportMatchResult, error) {
			receivedCSV = csvContent
			return &domain.ImportMatchResult{Matched: []domain.ImportMatched{}}, nil
		},
	}
	h := newTestImportHandler(store)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "planilha.csv")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte("Nome,Gestão\nRICARDO,Chefe\n"))
	mw.Close()

	req := httptest.NewRequest("POST", "/api/v1/investimentos/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !strings.Contains(receivedCSV, "RICARDO") {
		t.Errorf("expected csv content passed to service to contain RICARDO, got %q", receivedCSV)
	}
}

func TestImport_SheetsURL_Success(t *testing.T) {
	store := &mockImportStore{
		fetchGoogleSheetCSVFn: func(_ context.Context, url string) (string, string, string, error) {
			if url != "https://docs.google.com/spreadsheets/d/abc/edit" {
				t.Errorf("unexpected url: %q", url)
			}
			return "Nome\nRICARDO\n", "abc", "0", nil
		},
		matchPlanilhaFn: func(_ context.Context, csvContent string) (*domain.ImportMatchResult, error) {
			return &domain.ImportMatchResult{Matched: []domain.ImportMatched{}}, nil
		},
	}
	h := newTestImportHandler(store)

	body := `{"sheets_url":"https://docs.google.com/spreadsheets/d/abc/edit"}`
	req := httptest.NewRequest("POST", "/api/v1/investimentos/import", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestImport_SheetsURL_MissingURL_BadRequest(t *testing.T) {
	store := &mockImportStore{}
	h := newTestImportHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/investimentos/import", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestImport_FetchError_BadRequest(t *testing.T) {
	store := &mockImportStore{
		fetchGoogleSheetCSVFn: func(_ context.Context, _ string) (string, string, string, error) {
			return "", "", "", errPlanilhaPrivada
		},
	}
	h := newTestImportHandler(store)

	body := `{"sheets_url":"https://docs.google.com/spreadsheets/d/abc/edit"}`
	req := httptest.NewRequest("POST", "/api/v1/investimentos/import", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

var errPlanilhaPrivada = &testError{"planilha não está pública"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
```

- [ ] **Step 8: Run handler tests to verify they fail**

```bash
cd /home/emerson/code/myplanner/backend && go test ./internal/handler/... -run "TestImport" -v
```

Expected: FAIL — `undefined: ImportHandler` / `undefined: NewImportHandler`.

- [ ] **Step 9: Implement the handler**

```go
// backend/internal/handler/import.go
package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"go.uber.org/zap"
)

// ImportStore is the subset of *service.ImportService the handler needs.
// Task 4 adds ConfirmImport, GetSyncConfig, and Sync to this interface.
type ImportStore interface {
	MatchPlanilha(ctx context.Context, csvContent string) (*domain.ImportMatchResult, error)
	FetchGoogleSheetCSV(ctx context.Context, sheetsURL string) (csvContent, id, gid string, err error)
}

type ImportHandler struct {
	store  ImportStore
	logger *zap.Logger
}

func NewImportHandler(store ImportStore, logger *zap.Logger) *ImportHandler {
	return &ImportHandler{store: store, logger: logger}
}

// Import handles POST /investimentos/import. It accepts either a
// multipart/form-data upload (field "file") or a JSON body {"sheets_url":
// "..."}, parses/matches the spreadsheet, and returns the match result.
func (h *ImportHandler) Import(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")
	var csvContent string

	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			respondError(w, http.StatusBadRequest, "arquivo inválido")
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			respondError(w, http.StatusBadRequest, "arquivo não enviado")
			return
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "falha ao ler arquivo")
			return
		}
		csvContent = string(body)
	} else {
		var req struct {
			SheetsURL string `json:"sheets_url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SheetsURL == "" {
			respondError(w, http.StatusBadRequest, "informe sheets_url ou envie um arquivo CSV")
			return
		}
		content, _, _, err := h.store.FetchGoogleSheetCSV(r.Context(), req.SheetsURL)
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		csvContent = content
	}

	result, err := h.store.MatchPlanilha(r.Context(), csvContent)
	if err != nil {
		h.logger.Error("failed to match planilha", zap.Error(err))
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 10: Run handler tests to verify they pass**

```bash
cd /home/emerson/code/myplanner/backend && go test ./internal/handler/... -run "TestImport" -v
```

Expected: all subtests PASS.

- [ ] **Step 11: Wire up in `cmd/api/main.go`**

After `tarefaHandler := handler.NewTarefaHandler(tarefaRepo, logger)` at `backend/cmd/api/main.go:186`, add:

```go
	importService := service.NewImportService(membroRepo, equipeRepo, logger)
	importHandler := handler.NewImportHandler(importService, logger)
```

After `r.Get("/membros/{id}/alocacoes-projetos", investHandler.GetAlocacoesProjetos)` at `backend/cmd/api/main.go:261`, add:

```go
			r.Post("/investimentos/import", importHandler.Import)
```

- [ ] **Step 12: Full build and test**

```bash
cd /home/emerson/code/myplanner/backend && go build ./... && go test ./... 2>&1 | tail -30
```

Expected: build succeeds, all tests pass (existing tests unaffected).

- [ ] **Step 13: Commit**

```bash
cd /home/emerson/code/myplanner/backend
git add internal/domain/import.go internal/service/import_match.go internal/service/import_match_test.go internal/service/import.go internal/handler/import.go internal/handler/import_test.go cmd/api/main.go
git commit -m "feat: add import matching logic and POST /investimentos/import endpoint"
```

---

### Task 4: Import confirm + sync config

**Files:**
- Modify: `backend/internal/domain/import.go` (append confirm/config types)
- Create: `backend/internal/repository/import_config.go`
- Modify: `backend/internal/repository/membro.go` (append `UpdateCamposImport`)
- Modify: `backend/internal/service/import.go` (append `ConfirmImport`, `GetSyncConfig`, `Sync`, add `configRepo` field)
- Modify: `backend/internal/handler/import.go` (extend `ImportStore` interface, add `Confirmar`, `GetConfig`, `Sync` handlers)
- Modify: `backend/internal/handler/import_test.go` (extend mock, add tests)
- Modify: `backend/cmd/api/main.go` (wiring + 3 routes)

**Interfaces:**
- Consumes: `domain.ImportMatchResult` (Task 3); `repository.EquipeRepository.AddMembroEquipe(ctx, equipeID, membroID uuid.UUID) error` (existing, idempotent via `ON CONFLICT DO NOTHING`)
- Produces: `domain.ConfirmImportRequest`, `domain.ConfirmImportLinha`, `domain.ImportDadosConfirm`, `domain.ConfirmImportResponse`, `domain.ImportConfigResponse`; `repository.ImportConfigRepository` with `Get`, `Save`, `UpdateUltimoSync`; `MembroRepository.UpdateCamposImport(...)`; `POST /investimentos/import/confirmar`, `GET /investimentos/import/config`, `POST /investimentos/import/sync` (consumed by Task 7 frontend)

- [ ] **Step 1: Append confirm/config types to `internal/domain/import.go`**

```go
// Append to backend/internal/domain/import.go

// ImportDadosConfirm is the (possibly user-edited) financial data for one
// row, sent by the frontend when confirming an import.
type ImportDadosConfirm struct {
	Cargo         *string    `json:"cargo"`
	Matricula     *string    `json:"matricula"`
	Salario       *float64   `json:"salario"`
	DataAdmissao  *string    `json:"data_admissao"`
	UltimoAumento *string    `json:"ultimo_aumento"`
	GestorID      *uuid.UUID `json:"gestor_id"`
}

// ConfirmImportLinha is one resolved spreadsheet row: membro_id and
// equipe_id are already resolved by the frontend (either from the original
// match or from manual resolution). Ignorar=true skips the row entirely.
type ConfirmImportLinha struct {
	Linha    int                 `json:"linha"`
	MembroID *uuid.UUID          `json:"membro_id"`
	EquipeID *uuid.UUID          `json:"equipe_id"`
	Ignorar  bool                `json:"ignorar"`
	Dados    ImportDadosConfirm  `json:"dados"`
}

// ConfirmImportRequest is the body of POST /investimentos/import/confirmar.
// Tipo/URL/Gid are optional — when present, they are saved as the sync
// config for the "Sync" button.
type ConfirmImportRequest struct {
	Linhas []ConfirmImportLinha `json:"linhas"`
	Tipo   string                `json:"tipo"`
	URL    *string               `json:"url"`
	Gid    *string               `json:"gid"`
}

type ConfirmImportResponse struct {
	Atualizados int `json:"atualizados"`
	Ignorados   int `json:"ignorados"`
}

// ImportConfigResponse is returned by GET /investimentos/import/config.
type ImportConfigResponse struct {
	Tipo       string  `json:"tipo"`
	URL        *string `json:"url"`
	Gid        *string `json:"gid"`
	UltimoSync *string `json:"ultimo_sync"`
}
```

- [ ] **Step 2: Create `ImportConfigRepository`**

```go
// backend/internal/repository/import_config.go
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ImportConfig struct {
	ID         uuid.UUID
	Tipo       string
	URL        *string
	Gid        *string
	UltimoSync *time.Time
}

type ImportConfigRepository struct {
	pool *pgxpool.Pool
}

func NewImportConfigRepository(pool *pgxpool.Pool) *ImportConfigRepository {
	return &ImportConfigRepository{pool: pool}
}

// Get returns the single active import config, or nil if none has been
// saved yet.
func (r *ImportConfigRepository) Get(ctx context.Context) (*ImportConfig, error) {
	var c ImportConfig
	err := r.pool.QueryRow(ctx, `
		SELECT id, tipo, url, gid, ultimo_sync FROM import_configs
		ORDER BY created_at DESC LIMIT 1
	`).Scan(&c.ID, &c.Tipo, &c.URL, &c.Gid, &c.UltimoSync)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting import config: %w", err)
	}
	return &c, nil
}

// Save replaces any existing config with a new one (only one config is
// ever active).
func (r *ImportConfigRepository) Save(ctx context.Context, tipo string, url, gid *string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM import_configs`); err != nil {
		return fmt.Errorf("clearing import config: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO import_configs (tipo, url, gid) VALUES ($1, $2, $3)
	`, tipo, url, gid); err != nil {
		return fmt.Errorf("saving import config: %w", err)
	}
	return tx.Commit(ctx)
}

// UpdateUltimoSync stamps the active config with the current time.
func (r *ImportConfigRepository) UpdateUltimoSync(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `UPDATE import_configs SET ultimo_sync = NOW()`)
	if err != nil {
		return fmt.Errorf("updating ultimo_sync: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Add `UpdateCamposImport` to `MembroRepository`**

Append to `backend/internal/repository/membro.go` (after `UpdateDataAdmissao`, which the repository already imports `time` for):

```go
// UpdateCamposImport applies all import-sourced financial fields in one
// UPDATE. Cargo uses COALESCE — a nil cargo (no rule matched the spreadsheet
// job title) leaves the member's existing cargo untouched. Every other
// field is always overwritten (including with NULL) since the spreadsheet
// is the source of truth for them.
func (r *MembroRepository) UpdateCamposImport(ctx context.Context, id uuid.UUID, salario *float64, cargo *string, dataAdmissao *time.Time, matricula *string, ultimoAumento *time.Time, gestorID *uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE membros SET
			salario = $2,
			cargo = COALESCE($3, cargo),
			data_admissao = $4,
			matricula = $5,
			ultimo_aumento = $6,
			gestor_id = $7,
			updated_at = NOW()
		WHERE id = $1
	`, id, salario, cargo, dataAdmissao, matricula, ultimoAumento, gestorID)
	if err != nil {
		return fmt.Errorf("updating campos import: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not found", id)
	}
	return nil
}
```

- [ ] **Step 4: Add `configRepo` field and confirm/sync methods to `ImportService`**

In `backend/internal/service/import.go`, change the struct and constructor:

```go
type ImportService struct {
	membroRepo *repository.MembroRepository
	equipeRepo *repository.EquipeRepository
	configRepo *repository.ImportConfigRepository
	httpClient *http.Client
	logger     *zap.Logger
}

func NewImportService(membroRepo *repository.MembroRepository, equipeRepo *repository.EquipeRepository, configRepo *repository.ImportConfigRepository, logger *zap.Logger) *ImportService {
	return &ImportService{
		membroRepo: membroRepo,
		equipeRepo: equipeRepo,
		configRepo: configRepo,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
	}
}
```

Then append these methods to the same file:

```go
// ConfirmImport applies the resolved rows: updates each membro's financial
// fields and, if an equipe was resolved for the row, associates the membro
// to it. Rows with Ignorar=true or a nil MembroID are skipped. If Tipo is
// set, the sync config is saved for the "Sync" button.
func (s *ImportService) ConfirmImport(ctx context.Context, req domain.ConfirmImportRequest) (*domain.ConfirmImportResponse, error) {
	resp := &domain.ConfirmImportResponse{}

	for _, linha := range req.Linhas {
		if linha.Ignorar || linha.MembroID == nil {
			resp.Ignorados++
			continue
		}

		var dataAdmissao *time.Time
		if linha.Dados.DataAdmissao != nil {
			t, err := time.Parse("2006-01-02", *linha.Dados.DataAdmissao)
			if err != nil {
				return nil, fmt.Errorf("linha %d: data_admissao inválida: %w", linha.Linha, err)
			}
			dataAdmissao = &t
		}

		var ultimoAumento *time.Time
		if linha.Dados.UltimoAumento != nil {
			t, err := time.Parse("2006-01-02", *linha.Dados.UltimoAumento)
			if err != nil {
				return nil, fmt.Errorf("linha %d: ultimo_aumento inválido: %w", linha.Linha, err)
			}
			ultimoAumento = &t
		}

		if err := s.membroRepo.UpdateCamposImport(ctx, *linha.MembroID, linha.Dados.Salario, linha.Dados.Cargo, dataAdmissao, linha.Dados.Matricula, ultimoAumento, linha.Dados.GestorID); err != nil {
			return nil, fmt.Errorf("linha %d: atualizando membro: %w", linha.Linha, err)
		}

		if linha.EquipeID != nil {
			if err := s.equipeRepo.AddMembroEquipe(ctx, *linha.EquipeID, *linha.MembroID); err != nil {
				return nil, fmt.Errorf("linha %d: associando equipe: %w", linha.Linha, err)
			}
		}

		resp.Atualizados++
	}

	if req.Tipo != "" {
		if err := s.configRepo.Save(ctx, req.Tipo, req.URL, req.Gid); err != nil {
			s.logger.Warn("failed to save import config", zap.Error(err))
		}
	}

	return resp, nil
}

// GetSyncConfig returns the saved sync config, or nil if none exists yet.
func (s *ImportService) GetSyncConfig(ctx context.Context) (*domain.ImportConfigResponse, error) {
	cfg, err := s.configRepo.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting import config: %w", err)
	}
	if cfg == nil {
		return nil, nil
	}
	resp := &domain.ImportConfigResponse{Tipo: cfg.Tipo, URL: cfg.URL, Gid: cfg.Gid}
	if cfg.UltimoSync != nil {
		formatted := cfg.UltimoSync.Format(time.RFC3339)
		resp.UltimoSync = &formatted
	}
	return resp, nil
}

// Sync re-fetches the spreadsheet for the saved sheets_url config and
// re-runs the match. Only supported for tipo=sheets_url — a csv config
// requires a fresh upload, which the frontend handles by reopening the
// upload modal instead of calling this endpoint.
func (s *ImportService) Sync(ctx context.Context) (*domain.ImportMatchResult, error) {
	cfg, err := s.configRepo.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting import config: %w", err)
	}
	if cfg == nil {
		return nil, fmt.Errorf("nenhuma configuração de sincronização salva")
	}
	if cfg.Tipo != "sheets_url" {
		return nil, fmt.Errorf("configuração é do tipo CSV; faça o upload de um novo arquivo")
	}
	if cfg.URL == nil {
		return nil, fmt.Errorf("configuração sem URL salva")
	}

	csvContent, _, _, err := s.FetchGoogleSheetCSV(ctx, *cfg.URL)
	if err != nil {
		return nil, err
	}

	result, err := s.MatchPlanilha(ctx, csvContent)
	if err != nil {
		return nil, err
	}

	if err := s.configRepo.UpdateUltimoSync(ctx); err != nil {
		s.logger.Warn("failed to update ultimo_sync", zap.Error(err))
	}

	return result, nil
}
```

- [ ] **Step 5: Extend `ImportStore` interface and add handler methods**

In `backend/internal/handler/import.go`, replace the interface:

```go
type ImportStore interface {
	MatchPlanilha(ctx context.Context, csvContent string) (*domain.ImportMatchResult, error)
	FetchGoogleSheetCSV(ctx context.Context, sheetsURL string) (csvContent, id, gid string, err error)
	ConfirmImport(ctx context.Context, req domain.ConfirmImportRequest) (*domain.ConfirmImportResponse, error)
	GetSyncConfig(ctx context.Context) (*domain.ImportConfigResponse, error)
	Sync(ctx context.Context) (*domain.ImportMatchResult, error)
}
```

Append these methods to the same file:

```go
// Confirmar handles POST /investimentos/import/confirmar.
func (h *ImportHandler) Confirmar(w http.ResponseWriter, r *http.Request) {
	var req domain.ConfirmImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if len(req.Linhas) == 0 {
		respondError(w, http.StatusBadRequest, "nenhuma linha para importar")
		return
	}

	resp, err := h.store.ConfirmImport(r.Context(), req)
	if err != nil {
		h.logger.Error("failed to confirm import", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao confirmar importação")
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

// GetConfig handles GET /investimentos/import/config.
func (h *ImportHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.store.GetSyncConfig(r.Context())
	if err != nil {
		h.logger.Error("failed to get import config", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao carregar configuração de sync")
		return
	}
	respondJSON(w, http.StatusOK, cfg)
}

// Sync handles POST /investimentos/import/sync.
func (h *ImportHandler) Sync(w http.ResponseWriter, r *http.Request) {
	result, err := h.store.Sync(r.Context())
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 6: Write the failing tests**

Append to `backend/internal/handler/import_test.go` — first extend the mock struct and its methods:

```go
// Extend mockImportStore in backend/internal/handler/import_test.go:
type mockImportStore struct {
	matchPlanilhaFn       func(ctx context.Context, csvContent string) (*domain.ImportMatchResult, error)
	fetchGoogleSheetCSVFn func(ctx context.Context, sheetsURL string) (string, string, string, error)
	confirmImportFn       func(ctx context.Context, req domain.ConfirmImportRequest) (*domain.ConfirmImportResponse, error)
	getSyncConfigFn       func(ctx context.Context) (*domain.ImportConfigResponse, error)
	syncFn                func(ctx context.Context) (*domain.ImportMatchResult, error)
}

func (m *mockImportStore) ConfirmImport(ctx context.Context, req domain.ConfirmImportRequest) (*domain.ConfirmImportResponse, error) {
	return m.confirmImportFn(ctx, req)
}
func (m *mockImportStore) GetSyncConfig(ctx context.Context) (*domain.ImportConfigResponse, error) {
	return m.getSyncConfigFn(ctx)
}
func (m *mockImportStore) Sync(ctx context.Context) (*domain.ImportMatchResult, error) {
	return m.syncFn(ctx)
}
```

Then append the new test functions:

```go
func TestConfirmar_Success(t *testing.T) {
	store := &mockImportStore{
		confirmImportFn: func(_ context.Context, req domain.ConfirmImportRequest) (*domain.ConfirmImportResponse, error) {
			if len(req.Linhas) != 1 {
				t.Errorf("got %d linhas, want 1", len(req.Linhas))
			}
			return &domain.ConfirmImportResponse{Atualizados: 1, Ignorados: 0}, nil
		},
	}
	h := newTestImportHandler(store)

	body := `{"linhas":[{"linha":1,"membro_id":"11111111-1111-1111-1111-111111111111","ignorar":false,"dados":{"salario":6480.00}}],"tipo":"csv"}`
	req := httptest.NewRequest("POST", "/api/v1/investimentos/import/confirmar", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Confirmar(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestConfirmar_EmptyLinhas_BadRequest(t *testing.T) {
	store := &mockImportStore{}
	h := newTestImportHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/investimentos/import/confirmar", bytes.NewBufferString(`{"linhas":[]}`))
	rr := httptest.NewRecorder()
	h.Confirmar(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetConfig_Success(t *testing.T) {
	url := "https://docs.google.com/spreadsheets/d/abc/edit"
	store := &mockImportStore{
		getSyncConfigFn: func(_ context.Context) (*domain.ImportConfigResponse, error) {
			return &domain.ImportConfigResponse{Tipo: "sheets_url", URL: &url}, nil
		},
	}
	h := newTestImportHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/investimentos/import/config", nil)
	rr := httptest.NewRecorder()
	h.GetConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestGetConfig_NilConfig(t *testing.T) {
	store := &mockImportStore{
		getSyncConfigFn: func(_ context.Context) (*domain.ImportConfigResponse, error) {
			return nil, nil
		},
	}
	h := newTestImportHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/investimentos/import/config", nil)
	rr := httptest.NewRecorder()
	h.GetConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if strings.TrimSpace(rr.Body.String()) != "null" {
		t.Errorf("body = %q, want null", rr.Body.String())
	}
}

func TestSync_Success(t *testing.T) {
	store := &mockImportStore{
		syncFn: func(_ context.Context) (*domain.ImportMatchResult, error) {
			return &domain.ImportMatchResult{Matched: []domain.ImportMatched{}}, nil
		},
	}
	h := newTestImportHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/investimentos/import/sync", nil)
	rr := httptest.NewRecorder()
	h.Sync(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestSync_NoConfig_BadRequest(t *testing.T) {
	store := &mockImportStore{
		syncFn: func(_ context.Context) (*domain.ImportMatchResult, error) {
			return nil, errPlanilhaPrivada
		},
	}
	h := newTestImportHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/investimentos/import/sync", nil)
	rr := httptest.NewRecorder()
	h.Sync(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}
```

You will also need `"strings"` imported in `import_test.go` if not already present from Task 3.

- [ ] **Step 7: Run tests to verify they fail, then pass**

```bash
cd /home/emerson/code/myplanner/backend && go test ./internal/handler/... -run "TestConfirmar|TestGetConfig|TestSync" -v
```

Expected: first run FAILs to compile (`mockImportStore` missing methods / `ImportHandler.Confirmar` undefined) until Steps 2-5 are in place; after implementing, all subtests PASS.

- [ ] **Step 8: Wire up remaining routes in `cmd/api/main.go`**

Update the wiring block added in Task 3 (`backend/cmd/api/main.go:186`) to include the config repo:

```go
	importConfigRepo := repository.NewImportConfigRepository(pool)
	importService := service.NewImportService(membroRepo, equipeRepo, importConfigRepo, logger)
	importHandler := handler.NewImportHandler(importService, logger)
```

Update the route block added in Task 3 (`backend/cmd/api/main.go:261`) to add the 3 new routes:

```go
			r.Post("/investimentos/import", importHandler.Import)
			r.Post("/investimentos/import/confirmar", importHandler.Confirmar)
			r.Get("/investimentos/import/config", importHandler.GetConfig)
			r.Post("/investimentos/import/sync", importHandler.Sync)
```

- [ ] **Step 9: Full build and test**

```bash
cd /home/emerson/code/myplanner/backend && go build ./... && go vet ./... && go test ./... 2>&1 | tail -40
```

Expected: build succeeds, `go vet` clean, all tests pass.

- [ ] **Step 10: Manual smoke test against a running server**

```bash
cd /home/emerson/code/myplanner/backend && go run cmd/api/main.go &
sleep 2
curl -s -X POST http://localhost:8080/api/v1/investimentos/import \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@/tmp/planilha-teste.csv" | head -c 2000
kill %1
```

Expected: JSON response with `matched`, `unmatched_membros`, `unmatched_equipes`, `unmatched_gestores`, `ignorados` keys (adjust `$TOKEN` and the sample CSV path to your local setup).

- [ ] **Step 11: Commit**

```bash
cd /home/emerson/code/myplanner/backend
git add internal/domain/import.go internal/repository/import_config.go internal/repository/membro.go internal/service/import.go internal/handler/import.go internal/handler/import_test.go cmd/api/main.go
git commit -m "feat: add import confirm and sync config endpoints"
```

---

### Task 5: Import button + upload modal (CSV / Google Sheets tabs)

**Files:**
- Modify: `frontend/index.html:989` (CSS, before `</style>`)
- Modify: `frontend/index.html:1206-1211` (`inv-filter-row` — add Importar/Sync buttons)
- Modify: `frontend/index.html:1519-1523` (add import modal after `investimento-modal`)
- Modify: `frontend/index.html` (JS, after `closeInvestimentoModal()` at line 7957)

**Interfaces:**
- Consumes: `esc()`, `token`, `API` (frontend/index.html:1963), existing `api()` wrapper (frontend/index.html:2008), `.modal`/`.modal-overlay`/`.btn-add`/`.form-label` CSS classes, `POST /investimentos/import` (Task 3)
- Produces: `openImportModal()`, `closeImportModal()`, `switchImportTab(tab)`, `apiUpload(path, formData)`, `submitImportCSV()`, `submitImportSheetsURL()` — calls `openImportResolveModal(result, meta)` (implemented in Task 6, guarded with `typeof`)

- [ ] **Step 1: Add CSS for the import modal and buttons**

Before `</style>` at `frontend/index.html:989`, insert:

```css
/* === IMPORT PLANILHA === */
.inv-sync-info { font-size: 12px; color: var(--text-tertiary); white-space: nowrap; }
.import-tabs { display: flex; gap: 4px; margin: 12px 0 16px; border-bottom: 1px solid var(--border); }
.import-tab-btn { padding: 8px 14px; border: none; background: none; cursor: pointer; font-size: 13px; font-weight: 600; color: var(--text-secondary); border-bottom: 2px solid transparent; margin-bottom: -1px; font-family: inherit; }
.import-tab-btn.active { color: var(--accent); border-bottom-color: var(--accent); }
.import-tab-btn:hover { color: var(--accent); }
.import-hint { font-size: 11px; color: var(--text-tertiary); margin-top: 6px; }
.import-status { margin-top: 10px; min-height: 18px; }
.import-error { color: #D4483B; font-size: 12px; font-weight: 600; }
```

- [ ] **Step 2: Add Importar/Sync buttons to the filter row**

In `frontend/index.html:1206-1211`, replace:

```html
      <div class="inv-filter-row">
        <select class="filter-select" id="inv-equipe" onchange="onInvEquipeChange()">
          <option value="">Selecione a equipe</option>
        </select>
        <div class="inv-avatar-row" id="inv-avatars"></div>
      </div>
```

with:

```html
      <div class="inv-filter-row">
        <select class="filter-select" id="inv-equipe" onchange="onInvEquipeChange()">
          <option value="">Selecione a equipe</option>
        </select>
        <div class="inv-avatar-row" id="inv-avatars"></div>
        <div style="margin-left:auto;display:flex;gap:8px;align-items:center">
          <span id="inv-sync-info" class="inv-sync-info" style="display:none"></span>
          <button class="btn-add" id="inv-sync-btn" style="display:none" onclick="typeof syncImportPlanilha === 'function' && syncImportPlanilha()" title="Sincronizar importação">↻ Sync</button>
          <button class="btn-add" onclick="openImportModal()" title="Importar planilha">⬆ Importar</button>
        </div>
      </div>
```

- [ ] **Step 3: Add the import modal HTML**

After the `investimento-modal` closing `</div>` at `frontend/index.html:1522`, add:

```html
<div class="modal-overlay" id="import-modal" onclick="if(event.target===this)closeImportModal()">
  <div class="modal" style="max-width:560px">
    <div class="modal-title">⬆ Importar Planilha de Investimentos</div>
    <div class="import-tabs">
      <button class="import-tab-btn active" id="import-tab-csv-btn" onclick="switchImportTab('csv')">Upload CSV</button>
      <button class="import-tab-btn" id="import-tab-url-btn" onclick="switchImportTab('url')">Google Sheets URL</button>
    </div>
    <div class="import-tab-content" id="import-tab-csv">
      <label class="form-label">Arquivo CSV</label>
      <input type="file" id="import-csv-file" accept=".csv" style="width:100%;padding:8px;border:1px solid var(--border);border-radius:6px;background:var(--card-bg);color:var(--text);box-sizing:border-box">
      <div id="import-csv-status" class="import-status"></div>
      <div class="modal-actions">
        <button class="btn-cancel" type="button" onclick="closeImportModal()">Cancelar</button>
        <button class="btn-add" type="button" onclick="submitImportCSV()">Enviar</button>
      </div>
    </div>
    <div class="import-tab-content" id="import-tab-url" style="display:none">
      <label class="form-label">URL da Planilha Google Sheets</label>
      <input type="text" id="import-sheets-url" placeholder="https://docs.google.com/spreadsheets/d/..." style="width:100%;padding:8px 12px;border:1px solid var(--border);border-radius:6px;background:var(--card-bg);color:var(--text);box-sizing:border-box">
      <div class="import-hint">A planilha precisa estar pública ("qualquer pessoa com o link pode ver").</div>
      <div id="import-url-status" class="import-status"></div>
      <div class="modal-actions">
        <button class="btn-cancel" type="button" onclick="closeImportModal()">Cancelar</button>
        <button class="btn-add" type="button" onclick="submitImportSheetsURL()">Buscar</button>
      </div>
    </div>
  </div>
</div>
```

- [ ] **Step 4: Add the JS functions**

After `closeInvestimentoModal()` (ends at `frontend/index.html:7957`), and before `// === INIT ===`, add:

```javascript
// === IMPORT PLANILHA ===
function openImportModal() {
  document.getElementById('import-csv-file').value = '';
  document.getElementById('import-sheets-url').value = '';
  document.getElementById('import-csv-status').innerHTML = '';
  document.getElementById('import-url-status').innerHTML = '';
  switchImportTab('csv');
  document.getElementById('import-modal').classList.add('open');
}

function closeImportModal() {
  document.getElementById('import-modal').classList.remove('open');
}

function switchImportTab(tab) {
  document.getElementById('import-tab-csv-btn').classList.toggle('active', tab === 'csv');
  document.getElementById('import-tab-url-btn').classList.toggle('active', tab === 'url');
  document.getElementById('import-tab-csv').style.display = tab === 'csv' ? 'block' : 'none';
  document.getElementById('import-tab-url').style.display = tab === 'url' ? 'block' : 'none';
}

function apiUpload(path, formData) {
  const headers = {};
  if (token) headers['Authorization'] = 'Bearer ' + token;
  return fetch(API + path, { method: 'POST', headers, body: formData }).then(async r => {
    const data = await r.json().catch(() => null);
    if (!r.ok) {
      if (r.status === 401) { logout(); return; }
      throw new Error((data && data.error) || r.statusText);
    }
    return data;
  });
}

async function submitImportCSV() {
  var fileInput = document.getElementById('import-csv-file');
  var statusEl = document.getElementById('import-csv-status');
  if (!fileInput.files || fileInput.files.length === 0) {
    statusEl.innerHTML = '<span class="import-error">Selecione um arquivo CSV.</span>';
    return;
  }
  statusEl.innerHTML = '<div class="loading"><div class="spinner"></div></div>';
  try {
    var formData = new FormData();
    formData.append('file', fileInput.files[0]);
    var result = await apiUpload('/investimentos/import', formData);
    closeImportModal();
    if (typeof openImportResolveModal === 'function') openImportResolveModal(result, { tipo: 'csv' });
  } catch (err) {
    statusEl.innerHTML = '<span class="import-error">' + esc(err.message) + '</span>';
  }
}

async function submitImportSheetsURL() {
  var urlInput = document.getElementById('import-sheets-url');
  var statusEl = document.getElementById('import-url-status');
  var url = urlInput.value.trim();
  if (!url) {
    statusEl.innerHTML = '<span class="import-error">Informe a URL da planilha.</span>';
    return;
  }
  statusEl.innerHTML = '<div class="loading"><div class="spinner"></div></div>';
  try {
    var result = await api('/investimentos/import', { method: 'POST', body: JSON.stringify({ sheets_url: url }) });
    closeImportModal();
    if (typeof openImportResolveModal === 'function') openImportResolveModal(result, { tipo: 'sheets_url', url: url });
  } catch (err) {
    statusEl.innerHTML = '<span class="import-error">' + esc(err.message) + '</span>';
  }
}
```

- [ ] **Step 5: Verify in browser**

1. Open the app, navigate to Estrutura → Investimentos.
2. Click "⬆ Importar" — modal opens on the "Upload CSV" tab.
3. Click "Google Sheets URL" tab — content switches, tab underline moves, CSV tab hides.
4. Click "Enviar" with no file selected — shows "Selecione um arquivo CSV." in red.
5. Click "Buscar" with no URL — shows "Informe a URL da planilha." in red.
6. Select a small test CSV (headers: `Nome,Gestão,Time / Squad,Função,Matrícula,Admissão,Salário,Último Aumento` + one data row) and click "Enviar" — network tab shows a `multipart/form-data` POST to `/api/v1/investimentos/import`; since `openImportResolveModal` doesn't exist yet, nothing further happens but no console error should appear (guarded by `typeof`).
7. Click outside the modal — it closes.
8. Check dark mode: tabs and inputs follow `var(--text)`/`var(--border)`/`var(--card-bg)`.

- [ ] **Step 6: Commit**

```bash
cd /home/emerson/code/myplanner/backend/..
git add frontend/index.html
git commit -m "feat(frontend): add import button and CSV/Google Sheets upload modal"
```

---

### Task 6: Resolution modal (unmatched entities + preview table)

**Files:**
- Modify: `frontend/index.html:989` (CSS)
- Modify: `frontend/index.html:1519-1523` region (add resolution modal after import modal)
- Modify: `frontend/index.html` (JS, appended after Task 5's functions)

**Interfaces:**
- Consumes: `api()`, `esc()`, `fmtDateBR()` (frontend/index.html), `formatSalarioBR()` (frontend/index.html:7568-area, from SP2 plan), `domain.ImportMatchResult` JSON shape (Task 3/4), `GET /membros`, `GET /equipes`, `GET /cargos`, `POST /equipes` (existing endpoints)
- Produces: `openImportResolveModal(result, meta)`, `closeImportResolveModal()`, `setImportResolvedMembro(idx, membroId)`, `setImportResolvedEquipe(idx, equipeId)`, `setImportResolvedGestor(idx, membroId)`, `criarEquipeImport(idx)`, `buildImportResolvedLinhas()`, `renderImportPreviewTable()`, `cargoSlugLabel(slug)` — `confirmImportSubmit()` referenced but implemented in Task 7 (guarded with `typeof`)

- [ ] **Step 1: Add CSS for the resolution modal**

Before `</style>` at `frontend/index.html:989` (appending after Task 5's `/* === IMPORT PLANILHA === */` block), insert:

```css
.import-resolve-body { max-height: 60vh; overflow-y: auto; padding-right: 4px; }
.import-resolve-section { margin-bottom: 20px; }
.import-resolve-section h4 { font-size: 13px; font-weight: 650; color: var(--text-primary); margin: 0 0 10px; }
.import-resolve-row { display: flex; align-items: center; gap: 10px; padding: 8px 0; border-bottom: 1px solid var(--border-subtle); flex-wrap: wrap; }
.import-resolve-label { flex: 1; min-width: 180px; font-size: 13px; color: var(--text-primary); }
.import-resolve-linha { font-size: 11px; color: var(--text-tertiary); margin-left: 6px; }
.import-ignored-list { display: flex; flex-direction: column; gap: 4px; }
.import-ignored-item { font-size: 12px; color: var(--text-tertiary); }
```

- [ ] **Step 2: Add the resolution modal HTML**

After the `import-modal` closing `</div>` added in Task 5, add:

```html
<div class="modal-overlay" id="import-resolve-modal" onclick="if(event.target===this)closeImportResolveModal()">
  <div class="modal" style="max-width:820px">
    <div class="modal-title">Resolver Importação</div>
    <div id="import-resolve-body" class="import-resolve-body"></div>
    <div class="modal-actions">
      <button class="btn-cancel" type="button" onclick="closeImportResolveModal()">Cancelar</button>
      <button class="btn-add" type="button" id="import-confirm-btn" onclick="typeof confirmImportSubmit === 'function' && confirmImportSubmit()">Confirmar Importação</button>
    </div>
  </div>
</div>
```

- [ ] **Step 3: Add the resolution modal JS**

After the functions added in Task 5, add:

```javascript
let importMatchResult = null;
let importMeta = null;
let importAllMembros = null;
let importAllEquipes = null;
let importLinhaToMembroOverride = {};
let importLinhaToEquipeId = {};
let importLinhaToGestorId = {};

async function openImportResolveModal(result, meta) {
  importMatchResult = result;
  importMeta = meta;
  importLinhaToMembroOverride = {};
  importLinhaToEquipeId = {};
  importLinhaToGestorId = {};

  if (!importAllMembros) importAllMembros = await api('/membros');
  if (!importAllEquipes) importAllEquipes = await api('/equipes');
  if (!allCargos) allCargos = await api('/cargos');

  renderImportResolveModal();
  document.getElementById('import-resolve-modal').classList.add('open');
}

function closeImportResolveModal() {
  document.getElementById('import-resolve-modal').classList.remove('open');
}

function cargoSlugLabel(slug) {
  if (!slug) return '—';
  if (allCargos) {
    var found = allCargos.find(function(c) { return c.value === slug; });
    if (found) return found.label;
  }
  return slug;
}

function renderImportResolveModal() {
  var r = importMatchResult;
  var body = document.getElementById('import-resolve-body');
  var html = '';

  if (r.ignorados && r.ignorados.length > 0) {
    html += '<div class="import-resolve-section"><h4>Ignorados (' + r.ignorados.length + ')</h4><div class="import-ignored-list">';
    r.ignorados.forEach(function(i) {
      html += '<div class="import-ignored-item">Linha ' + i.linha + ': ' + esc(i.nome) + ' <span class="inv-chip">' + esc(i.motivo) + '</span></div>';
    });
    html += '</div></div>';
  }

  if (r.unmatched_membros && r.unmatched_membros.length > 0) {
    html += '<div class="import-resolve-section"><h4>Membros não encontrados (' + r.unmatched_membros.length + ')</h4>';
    r.unmatched_membros.forEach(function(u, idx) {
      html += '<div class="import-resolve-row">';
      html += '<div class="import-resolve-label">' + esc(u.nome_planilha) + '<span class="import-resolve-linha">linha ' + u.linha + '</span></div>';
      html += '<select class="filter-select" onchange="setImportResolvedMembro(' + idx + ', this.value)">';
      html += '<option value="">Ignorar</option>';
      importAllMembros.forEach(function(m) { html += '<option value="' + m.id + '">' + esc(m.nome) + '</option>'; });
      html += '</select></div>';
    });
    html += '</div>';
  }

  if (r.unmatched_equipes && r.unmatched_equipes.length > 0) {
    html += '<div class="import-resolve-section"><h4>Equipes não encontradas (' + r.unmatched_equipes.length + ')</h4>';
    r.unmatched_equipes.forEach(function(u, idx) {
      html += '<div class="import-resolve-row">';
      html += '<div class="import-resolve-label">' + esc(u.nome_planilha) + '<span class="import-resolve-linha">' + u.linhas.length + ' membro(s)</span></div>';
      html += '<select class="filter-select" id="import-eq-select-' + idx + '" onchange="setImportResolvedEquipe(' + idx + ', this.value)">';
      html += '<option value="">Ignorar</option>';
      importAllEquipes.forEach(function(e) { html += '<option value="' + e.id + '">' + esc(e.nome) + '</option>'; });
      html += '</select>';
      html += '<button class="btn-add" type="button" onclick="criarEquipeImport(' + idx + ')">+ Criar equipe</button>';
      html += '</div>';
    });
    html += '</div>';
  }

  if (r.unmatched_gestores && r.unmatched_gestores.length > 0) {
    html += '<div class="import-resolve-section"><h4>Gestores não encontrados (' + r.unmatched_gestores.length + ')</h4>';
    r.unmatched_gestores.forEach(function(u, idx) {
      html += '<div class="import-resolve-row">';
      html += '<div class="import-resolve-label">' + esc(u.nome_planilha) + '<span class="import-resolve-linha">' + u.linhas.length + ' membro(s)</span></div>';
      html += '<select class="filter-select" onchange="setImportResolvedGestor(' + idx + ', this.value)">';
      html += '<option value="">Ignorar</option>';
      importAllMembros.forEach(function(m) { html += '<option value="' + m.id + '">' + esc(m.nome) + '</option>'; });
      html += '</select></div>';
    });
    html += '</div>';
  }

  html += '<div class="import-resolve-section"><h4>Preview (' + r.matched.length + ' membro(s) reconhecidos)</h4>';
  html += renderImportPreviewTable();
  html += '</div>';

  body.innerHTML = html;
}

function setImportResolvedMembro(idx, membroId) {
  var u = importMatchResult.unmatched_membros[idx];
  importLinhaToMembroOverride[u.linha] = membroId || null;
  refreshImportPreview();
}

function setImportResolvedEquipe(idx, equipeId) {
  var u = importMatchResult.unmatched_equipes[idx];
  u.linhas.forEach(function(linha) { importLinhaToEquipeId[linha] = equipeId || null; });
  refreshImportPreview();
}

function setImportResolvedGestor(idx, membroId) {
  var u = importMatchResult.unmatched_gestores[idx];
  u.linhas.forEach(function(linha) { importLinhaToGestorId[linha] = membroId || null; });
  refreshImportPreview();
}

async function criarEquipeImport(idx) {
  var u = importMatchResult.unmatched_equipes[idx];
  if (!confirm('Criar equipe "' + u.nome_planilha + '"?')) return;
  try {
    var nova = await api('/equipes', { method: 'POST', body: JSON.stringify({ nome: u.nome_planilha }) });
    importAllEquipes.push(nova);
    u.linhas.forEach(function(linha) { importLinhaToEquipeId[linha] = nova.id; });
    var sel = document.getElementById('import-eq-select-' + idx);
    if (sel) {
      var opt = document.createElement('option');
      opt.value = nova.id;
      opt.textContent = nova.nome;
      opt.selected = true;
      sel.appendChild(opt);
    }
    refreshImportPreview();
  } catch (err) {
    alert('Erro ao criar equipe: ' + err.message);
  }
}

function refreshImportPreview() {
  var container = document.getElementById('import-preview-table-wrap');
  if (container) container.outerHTML = renderImportPreviewTable();
}

function buildImportResolvedLinhas() {
  var r = importMatchResult;
  var linhas = [];

  r.matched.forEach(function(m) {
    var equipeId = m.equipe_id || importLinhaToEquipeId[m.linha] || null;
    var gestorId = m.dados.gestor_id || importLinhaToGestorId[m.linha] || null;
    linhas.push({
      linha: m.linha,
      membro_id: m.membro_id,
      equipe_id: equipeId,
      ignorar: false,
      changes: m.changes,
      dados: {
        cargo: m.dados.cargo,
        matricula: m.dados.matricula,
        salario: m.dados.salario,
        data_admissao: m.dados.data_admissao,
        ultimo_aumento: m.dados.ultimo_aumento,
        gestor_id: gestorId
      }
    });
  });

  r.unmatched_membros.forEach(function(u) {
    var membroId = importLinhaToMembroOverride.hasOwnProperty(u.linha) ? importLinhaToMembroOverride[u.linha] : null;
    var equipeId = importLinhaToEquipeId[u.linha] || null;
    var gestorId = u.dados.gestor_id || importLinhaToGestorId[u.linha] || null;
    linhas.push({
      linha: u.linha,
      membro_id: membroId,
      equipe_id: equipeId,
      ignorar: !membroId,
      changes: ['manual'],
      dados: {
        cargo: u.dados.cargo,
        matricula: u.dados.matricula,
        salario: u.dados.salario,
        data_admissao: u.dados.data_admissao,
        ultimo_aumento: u.dados.ultimo_aumento,
        gestor_id: gestorId
      }
    });
  });

  return linhas;
}

function renderImportPreviewTable() {
  var linhas = buildImportResolvedLinhas();
  var byId = {};
  importAllMembros.forEach(function(m) { byId[m.id] = m; });
  var eqById = {};
  importAllEquipes.forEach(function(e) { eqById[e.id] = e; });

  var html = '<div id="import-preview-table-wrap"><table class="inv-table"><thead><tr>';
  html += '<th>Nome</th><th>Equipe</th><th>Cargo</th><th>Salário</th><th>Admissão</th><th>Matrícula</th><th>Ação</th>';
  html += '</tr></thead><tbody>';

  linhas.forEach(function(l) {
    if (l.ignorar) return;
    var membro = l.membro_id ? byId[l.membro_id] : null;
    var nome = membro ? membro.nome : '—';
    var equipe = l.equipe_id ? eqById[l.equipe_id] : null;
    var equipeNome = equipe ? equipe.nome : '—';
    var cargoLabel = cargoSlugLabel(l.dados.cargo);
    var salario = l.dados.salario != null ? formatSalarioBR(l.dados.salario) : '—';
    var admissao = l.dados.data_admissao ? fmtDateBR(l.dados.data_admissao) : '—';
    var matricula = l.dados.matricula || '—';
    var acao = (l.changes && l.changes.length > 0) ? 'Atualizar' : 'Sem mudança';

    html += '<tr>';
    html += '<td>' + esc(nome) + '</td>';
    html += '<td>' + esc(equipeNome) + '</td>';
    html += '<td>' + esc(cargoLabel) + '</td>';
    html += '<td>' + esc(salario) + '</td>';
    html += '<td>' + esc(admissao) + '</td>';
    html += '<td>' + esc(matricula) + '</td>';
    html += '<td><span class="inv-chip">' + acao + '</span></td>';
    html += '</tr>';
  });

  html += '</tbody></table></div>';
  return html;
}
```

Note: this reuses `allCargos` (declared in the existing "CARGO / PRODUTOS" section, `frontend/index.html:2013`) and `formatSalarioBR()` / `fmtDateBR()` (already present from the SP2 Investimentos plan) — no new globals needed for those.

- [ ] **Step 4: Verify in browser**

1. Import a test CSV with: one row matching an existing membro + existing equipe, one row with an unrecognized membro name, one row with an unrecognized Time/Squad, one row with an unrecognized Gestão name.
2. After upload, the resolution modal opens showing: "Membros não encontrados" section with a name + dropdown, "Equipes não encontradas" section with a name + dropdown + "+ Criar equipe" button, "Gestores não encontrados" section with a name + dropdown, and a "Preview" table at the bottom.
3. Select a membro in the unmatched-membro dropdown — preview table updates to include that row.
4. Select an equipe in the unmatched-equipe dropdown — preview table's "Equipe" column updates for the affected row(s).
5. Click "+ Criar equipe" — confirm dialog appears; after confirming, a `POST /equipes` call fires, the new equipe appears selected in the dropdown, and the preview table updates.
6. Verify "Ação" column shows "Atualizar" for rows with financial differences and "Sem mudança" when spreadsheet values match the current membro record exactly.
7. Click "Confirmar Importação" — no error (guarded by `typeof`, since Task 7 implements it).
8. Click outside modal — closes.

- [ ] **Step 5: Commit**

```bash
cd /home/emerson/code/myplanner
git add frontend/index.html
git commit -m "feat(frontend): add import resolution modal with unmatched dropdowns and preview table"
```

---

### Task 7: Confirm import + Sync button + NOVO badge

**Files:**
- Modify: `frontend/index.html:989` (CSS — badges)
- Modify: `frontend/index.html:2173` (`navigate()` — load sync config on page show)
- Modify: `frontend/index.html` (JS — `confirmImportSubmit`, sync functions, badge helper)
- Modify: `frontend/index.html:~7593` (`renderInvestimentosDashboard` avatar loop — add NOVO badge)
- Modify: `frontend/index.html:~7833` (`renderInvMembrosTable` row loop — add NOVO badge)
- Modify: `frontend/index.html` (Task 6's `import-confirm-btn` and `inv-sync-btn` — remove `typeof` guards)

**Interfaces:**
- Consumes: `buildImportResolvedLinhas()`, `openImportResolveModal()`, `closeImportResolveModal()` (Task 6); `loadInvestimentos()`, `invCurrentEquipeId` (existing, from SP2 plan); `fmtDateTimeBR()` (existing); `POST /investimentos/import/confirmar`, `GET /investimentos/import/config`, `POST /investimentos/import/sync` (Task 4)
- Produces: `confirmImportSubmit()`, `loadImportSyncConfig()`, `syncImportPlanilha()`, `isDataAdmissaoFutura(dataAdmissao)`

- [ ] **Step 1: Add CSS for the NOVO badge**

Before `</style>` at `frontend/index.html:989` (appending after Task 6's CSS block), insert:

```css
.inv-badge-novo { position: absolute; top: -4px; right: -4px; background: #16a34a; color: #fff; font-size: 8px; font-weight: 700; padding: 1px 4px; border-radius: 6px; line-height: 1.4; letter-spacing: .3px; }
.inv-badge-novo-inline { background: #16a34a; color: #fff; font-size: 10px; font-weight: 700; padding: 1px 6px; border-radius: 8px; margin-left: 6px; letter-spacing: .3px; vertical-align: middle; }
```

Then find the `.inv-avatar` rule (around `frontend/index.html:950`, added by the SP2 plan) and add `position: relative;` to it:

```css
.inv-avatar { width: 34px; height: 34px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-weight: 600; font-size: 11px; color: #fff; background: var(--accent); cursor: pointer; overflow: hidden; transition: transform .15s, box-shadow .15s; border: 2px solid transparent; flex-shrink: 0; position: relative; }
```

- [ ] **Step 2: Add `confirmImportSubmit`, sync functions, and the badge helper**

After the functions added in Task 6, add:

```javascript
function isDataAdmissaoFutura(dataAdmissao) {
  if (!dataAdmissao) return false;
  var hoje = new Date();
  hoje.setHours(0, 0, 0, 0);
  var d = new Date(dataAdmissao + 'T00:00:00');
  return d > hoje;
}

async function confirmImportSubmit() {
  var linhas = buildImportResolvedLinhas().filter(function(l) { return !l.ignorar; });
  if (linhas.length === 0) {
    alert('Nenhum membro para importar.');
    return;
  }
  var btn = document.getElementById('import-confirm-btn');
  btn.disabled = true;
  btn.textContent = 'Importando...';
  try {
    var payload = { linhas: linhas, tipo: importMeta.tipo };
    if (importMeta.tipo === 'sheets_url') payload.url = importMeta.url;
    var resp = await api('/investimentos/import/confirmar', { method: 'POST', body: JSON.stringify(payload) });
    closeImportResolveModal();
    alert(resp.atualizados + ' membro(s) atualizado(s) com sucesso.');
    loadImportSyncConfig();
    if (invCurrentEquipeId) loadInvestimentos(invCurrentEquipeId);
  } catch (err) {
    alert('Erro ao confirmar importação: ' + err.message);
  } finally {
    btn.disabled = false;
    btn.textContent = 'Confirmar Importação';
  }
}

let importLastConfig = null;

async function loadImportSyncConfig() {
  var btn = document.getElementById('inv-sync-btn');
  var info = document.getElementById('inv-sync-info');
  if (!btn || !info) return;
  try {
    var cfg = await api('/investimentos/import/config');
    importLastConfig = cfg;
    if (!cfg) {
      btn.style.display = 'none';
      info.style.display = 'none';
      return;
    }
    btn.style.display = 'inline-flex';
    if (cfg.ultimo_sync) {
      info.style.display = 'inline';
      info.textContent = 'Último sync: ' + fmtDateTimeBR(cfg.ultimo_sync);
    } else {
      info.style.display = 'none';
    }
  } catch (err) {
    console.error('Failed to load import config', err);
  }
}

async function syncImportPlanilha() {
  if (!importLastConfig) return;
  if (importLastConfig.tipo === 'csv') {
    openImportModal();
    switchImportTab('csv');
    return;
  }
  var btn = document.getElementById('inv-sync-btn');
  btn.disabled = true;
  btn.textContent = '↻ Sincronizando...';
  try {
    var result = await api('/investimentos/import/sync', { method: 'POST' });
    openImportResolveModal(result, { tipo: 'sheets_url', url: importLastConfig.url });
  } catch (err) {
    alert('Erro ao sincronizar: ' + err.message);
  } finally {
    btn.disabled = false;
    btn.textContent = '↻ Sync';
  }
}
```

- [ ] **Step 3: Remove the `typeof` guards added in Tasks 5 and 6**

In the Sync button HTML added in Task 5 (`frontend/index.html`, `inv-filter-row`), replace:

```html
          <button class="btn-add" id="inv-sync-btn" style="display:none" onclick="typeof syncImportPlanilha === 'function' && syncImportPlanilha()" title="Sincronizar importação">↻ Sync</button>
```

with:

```html
          <button class="btn-add" id="inv-sync-btn" style="display:none" onclick="syncImportPlanilha()" title="Sincronizar importação">↻ Sync</button>
```

In the resolution modal's confirm button added in Task 6, replace:

```html
      <button class="btn-add" type="button" id="import-confirm-btn" onclick="typeof confirmImportSubmit === 'function' && confirmImportSubmit()">Confirmar Importação</button>
```

with:

```html
      <button class="btn-add" type="button" id="import-confirm-btn" onclick="confirmImportSubmit()">Confirmar Importação</button>
```

- [ ] **Step 4: Load sync config when the Investimentos page is shown**

In `navigate()` at `frontend/index.html:2173`, replace:

```javascript
  if (page === 'investimentos') { /* page shown, data loads on equipe select */ }
```

with:

```javascript
  if (page === 'investimentos') { loadImportSyncConfig(); }
```

- [ ] **Step 5: Add the NOVO badge to the avatar row**

In `renderInvestimentosDashboard()` (around `frontend/index.html:7593`), replace:

```javascript
    avatarsHtml += '<div class="inv-avatar" title="' + esc(m.nome) + '" style="background:' + stringColor(m.nome) + '" onclick="scrollToInvMembro(\'' + m.id + '\')">' + av + '</div>';
```

with:

```javascript
    var novoBadge = isDataAdmissaoFutura(m.data_admissao) ? '<span class="inv-badge-novo" title="Novo">NOVO</span>' : '';
    avatarsHtml += '<div class="inv-avatar" title="' + esc(m.nome) + '" style="background:' + stringColor(m.nome) + '" onclick="scrollToInvMembro(\'' + m.id + '\')">' + av + novoBadge + '</div>';
```

- [ ] **Step 6: Add the NOVO badge to the members table**

In `renderInvMembrosTable()` (around `frontend/index.html:7833`), replace:

```javascript
    html += '<td><div class="inv-member-cell"><div class="inv-member-avatar" style="background:' + stringColor(m.nome) + '">' + avatarHtml + '</div>' + esc(m.nome) + '</div></td>';
```

with:

```javascript
    var novoInline = isDataAdmissaoFutura(m.data_admissao) ? ' <span class="inv-badge-novo-inline">NOVO</span>' : '';
    html += '<td><div class="inv-member-cell"><div class="inv-member-avatar" style="background:' + stringColor(m.nome) + '">' + avatarHtml + '</div>' + esc(m.nome) + novoInline + '</div></td>';
```

- [ ] **Step 7: Verify in browser end-to-end**

1. Navigate to Investimentos — no Sync button visible yet (no config saved).
2. Upload a CSV with at least one fully-matched row, confirm resolution modal shows a correct preview, click "Confirmar Importação".
3. Verify: success alert shows the count of updated members; resolution modal closes; if an equipe was selected in the filter, the dashboard reloads with updated salary/admission data.
4. Reload the page, navigate to Investimentos again — "↻ Sync" button now appears (config was saved on confirm) — no last-sync text yet (first confirm doesn't set `ultimo_sync`, only `/sync` does).
5. Click "↻ Sync" — if the saved config is `csv`, the upload modal reopens on the CSV tab; if `sheets_url`, it calls `POST /investimentos/import/sync`, opens the resolution modal with fresh data, and after a manual confirm, "Último sync: dd/mm/yyyy hh:mm" appears next to the Sync button.
6. Import a row with a future `data_admissao` (e.g. next month) — after confirming, that member's avatar in the filter row shows a small green "NOVO" badge in the top-right corner, and their table row shows an inline "NOVO" pill next to their name.
7. Check dark and light themes for badge contrast.

- [ ] **Step 8: Commit**

```bash
cd /home/emerson/code/myplanner
git add frontend/index.html
git commit -m "feat(frontend): wire up import confirmation, sync button, and NOVO admission badge"
```
