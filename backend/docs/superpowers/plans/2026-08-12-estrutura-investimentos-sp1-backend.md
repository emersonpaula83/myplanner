# Estrutura — Investimentos SP1 (Schema + Backend) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add salary, hire date, and hours-bank fields to members, with automatic history tracking, and expose investment dashboard endpoints for team financial overview.

**Architecture:** New migration adds 3 columns to `membros` + 2 history tables. New `InvestimentoService` orchestrates queries across equipe, membro, tarefa, and allocation repositories. New `InvestimentoHandler` exposes REST endpoints. Member update endpoints (salary, banco_horas, data_admissao) live in `MembroHandler` following existing patterns.

**Tech Stack:** Go, pgxpool, chi router, zap logger, pgx v5

## Global Constraints

- Follow existing handler/service/repository layering — no shortcuts
- Handler interfaces defined at consumer (handler package), not provider
- Response structs in `domain/` package, following `domain/equipe.go` pattern
- Migrations numbered sequentially from `000028`
- All SQL uses parameterized queries (`$1`, `$2`, etc.)
- Tests use stdlib `testing` package only — no external test frameworks
- `respondJSON` / `respondError` from `handler/response.go` for all HTTP responses
- UUID parsing via `chi.URLParam` + `uuid.Parse` for path params

---

### Task 1: Database Migration — New Fields and History Tables

**Files:**
- Create: `migrations/000028_investimentos_campos.up.sql`
- Create: `migrations/000028_investimentos_campos.down.sql`

**Interfaces:**
- Consumes: nothing
- Produces: DB schema changes consumed by all subsequent tasks

- [ ] **Step 1: Write the up migration**

```sql
-- Add financial fields to membros
ALTER TABLE membros ADD COLUMN salario DECIMAL(12,2);
ALTER TABLE membros ADD COLUMN data_admissao DATE;
ALTER TABLE membros ADD COLUMN banco_horas DECIMAL(8,2) DEFAULT 0;

-- Salary history
CREATE TABLE membro_salarios (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    membro_id UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
    valor DECIMAL(12,2) NOT NULL,
    data_vigencia DATE NOT NULL DEFAULT CURRENT_DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_membro_salarios_membro ON membro_salarios(membro_id, data_vigencia);

-- Hours bank history
CREATE TABLE membro_banco_horas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    membro_id UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
    valor DECIMAL(8,2) NOT NULL,
    data_registro TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_membro_banco_horas_membro ON membro_banco_horas(membro_id, data_registro);
```

- [ ] **Step 2: Write the down migration**

```sql
DROP TABLE IF EXISTS membro_banco_horas;
DROP TABLE IF EXISTS membro_salarios;
ALTER TABLE membros DROP COLUMN IF EXISTS banco_horas;
ALTER TABLE membros DROP COLUMN IF EXISTS data_admissao;
ALTER TABLE membros DROP COLUMN IF EXISTS salario;
```

- [ ] **Step 3: Run the migration**

Run: `cd /home/emerson/code/myplanner/backend && go run cmd/migrate/main.go up`

If no migrate command exists, apply manually:
```bash
psql "$DATABASE_URL" -f migrations/000028_investimentos_campos.up.sql
```

- [ ] **Step 4: Verify schema**

```bash
psql "$DATABASE_URL" -c "\d membros" | grep -E "salario|data_admissao|banco_horas"
psql "$DATABASE_URL" -c "\d membro_salarios"
psql "$DATABASE_URL" -c "\d membro_banco_horas"
```

Expected: 3 new columns on membros, 2 new tables with indexes.

- [ ] **Step 5: Commit**

```bash
git add migrations/000028_investimentos_campos.up.sql migrations/000028_investimentos_campos.down.sql
git commit -m "feat: add salary, hire date, hours bank fields and history tables"
```

---

### Task 2: Domain Models and Membro Struct Update

**Files:**
- Modify: `internal/domain/models.go` (Membro struct, lines 31-44)
- Create: `internal/domain/investimento.go`

**Interfaces:**
- Consumes: DB schema from Task 1
- Produces: `domain.Membro` updated struct (used by Tasks 3-6), domain types `SalarioHistorico`, `BancoHorasHistorico`, `InvestimentoDashboard`, `InvestimentoSumario`, `MembroInvestimento`, `GastoMensal`, `ProjetoAlocacao` (used by Tasks 4-6)

- [ ] **Step 1: Update Membro struct in `internal/domain/models.go`**

Add 3 fields after `Cargo`:

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
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}
```

- [ ] **Step 2: Update ALL SQL queries and Scan calls that read from `membros`**

Every `SELECT` and `Scan` for `domain.Membro` must include the 3 new columns. The following locations need updating:

**`internal/repository/membro.go` — `GetByID` (line ~46):**
```go
err := r.pool.QueryRow(ctx, `
    SELECT id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo, data_desligamento, cargo, salario, data_admissao, banco_horas, created_at, updated_at
    FROM membros WHERE id = $1
`, id).Scan(&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email, &m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo, &m.Salario, &m.DataAdmissao, &m.BancoHoras, &m.CreatedAt, &m.UpdatedAt)
```

**`internal/repository/membro.go` — `List` method:** same pattern — add `salario, data_admissao, banco_horas` to SELECT and Scan.

**`internal/repository/membro.go` — `Search` method:** same pattern.

**`internal/repository/equipe.go` — `GetMembrosEquipe` (line ~110):**
```go
rows, err := r.pool.Query(ctx, `
    SELECT m.id, m.fonte_dados_id, m.jira_account_id, m.nome, m.email,
           m.avatar_url, m.team, m.ativo, m.data_desligamento, m.cargo,
           m.salario, m.data_admissao, m.banco_horas,
           m.created_at, m.updated_at
    FROM membros m
    INNER JOIN equipe_membros em ON em.membro_id = m.id AND em.equipe_id = $1
    WHERE m.ativo = true
      AND (m.data_desligamento IS NULL OR m.data_desligamento > NOW())
    ORDER BY m.nome
`, equipeID)
```

And the corresponding Scan:
```go
if err := rows.Scan(
    &m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email,
    &m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo,
    &m.Salario, &m.DataAdmissao, &m.BancoHoras,
    &m.CreatedAt, &m.UpdatedAt,
); err != nil {
```

**Find all other locations** by grepping:
```bash
grep -rn "FROM membros" internal/repository/ | grep -v "_test.go"
grep -rn "\.Scan.*\.Nome.*\.Email" internal/repository/ | grep -v "_test.go"
```

Update every occurrence — add `salario, data_admissao, banco_horas` in the SELECT column list right after `cargo`, and add `&m.Salario, &m.DataAdmissao, &m.BancoHoras` in the Scan call right after `&m.Cargo`.

- [ ] **Step 3: Create `internal/domain/investimento.go`**

```go
package domain

import (
	"time"

	"github.com/google/uuid"
)

type SalarioHistorico struct {
	ID           uuid.UUID `json:"id"`
	MembroID     uuid.UUID `json:"membro_id"`
	Valor        float64   `json:"valor"`
	DataVigencia time.Time `json:"data_vigencia"`
	CreatedAt    time.Time `json:"created_at"`
}

type BancoHorasHistorico struct {
	ID           uuid.UUID `json:"id"`
	MembroID     uuid.UUID `json:"membro_id"`
	Valor        float64   `json:"valor"`
	DataRegistro time.Time `json:"data_registro"`
	CreatedAt    time.Time `json:"created_at"`
}

type InvestimentoDashboard struct {
	Equipe  EquipeInfo           `json:"equipe"`
	Sumario InvestimentoSumario  `json:"sumario"`
	Membros []MembroInvestimento `json:"membros"`
}

type EquipeInfo struct {
	ID   uuid.UUID `json:"id"`
	Nome string    `json:"nome"`
}

type InvestimentoSumario struct {
	CustoMensalTotal     float64 `json:"custo_mensal_total"`
	TotalMembros         int     `json:"total_membros"`
	TempoCasaMedioMeses  int     `json:"tempo_casa_medio_meses"`
	BancoHorasTotal      float64 `json:"banco_horas_total"`
}

type MembroInvestimento struct {
	ID             uuid.UUID `json:"id"`
	Nome           string    `json:"nome"`
	AvatarURL      *string   `json:"avatar_url"`
	Salario        *float64  `json:"salario"`
	DataAdmissao   *string   `json:"data_admissao"`
	TempoCasaMeses int       `json:"tempo_casa_meses"`
	BancoHoras     *float64  `json:"banco_horas"`
	Cargo          *string   `json:"cargo"`
	TopProdutos    []string  `json:"top_produtos"`
}

type GastoMensal struct {
	Mes        int     `json:"mes"`
	CustoTotal float64 `json:"custo_total"`
}

type GastosMensaisResponse struct {
	Ano   int          `json:"ano"`
	Meses []GastoMensal `json:"meses"`
}

type ProjetoAlocacao struct {
	Apelido             string  `json:"apelido"`
	ChaveJira           string  `json:"chave_jira"`
	PercentualAlocacao  float64 `json:"percentual_alocacao"`
}

type AlocacoesProjetosResponse struct {
	Projetos []ProjetoAlocacao `json:"projetos"`
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`

Expected: builds successfully. Fix any Scan mismatches if compilation fails.

- [ ] **Step 5: Run existing tests**

Run: `go test ./internal/...`

Expected: all existing tests pass — no regressions from struct changes.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/models.go internal/domain/investimento.go internal/repository/membro.go internal/repository/equipe.go
git commit -m "feat: add salary/hire-date/hours-bank to Membro struct and domain types for investimentos"
```

---

### Task 3: Repository — Membro Financial Updates and History

**Files:**
- Modify: `internal/repository/membro.go`

**Interfaces:**
- Consumes: `domain.Membro` (updated), `domain.SalarioHistorico`, `domain.BancoHorasHistorico` from Task 2
- Produces: Repository methods used by Tasks 5 and 6:
  - `UpdateSalario(ctx context.Context, id uuid.UUID, valor float64) error`
  - `UpdateBancoHoras(ctx context.Context, id uuid.UUID, valor float64) error`
  - `UpdateDataAdmissao(ctx context.Context, id uuid.UUID, data *time.Time) error`
  - `GetHistoricoSalario(ctx context.Context, membroID uuid.UUID) ([]domain.SalarioHistorico, error)`
  - `GetHistoricoBancoHoras(ctx context.Context, membroID uuid.UUID) ([]domain.BancoHorasHistorico, error)`

- [ ] **Step 1: Write tests for UpdateSalario**

Create or append to `internal/repository/membro_test.go`:

Since repository tests would need a real DB and we're mocking all DB access, these methods will be tested through service-level tests in Task 5. Skip to implementation.

- [ ] **Step 2: Add `UpdateSalario` method to `internal/repository/membro.go`**

```go
func (r *MembroRepository) UpdateSalario(ctx context.Context, id uuid.UUID, valor float64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `
		UPDATE membros SET salario = $2, updated_at = NOW() WHERE id = $1
	`, id, valor)
	if err != nil {
		return fmt.Errorf("updating salario: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not found", id)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO membro_salarios (membro_id, valor, data_vigencia)
		VALUES ($1, $2, CURRENT_DATE)
	`, id, valor)
	if err != nil {
		return fmt.Errorf("inserting salary history: %w", err)
	}

	return tx.Commit(ctx)
}
```

- [ ] **Step 3: Add `UpdateBancoHoras` method**

```go
func (r *MembroRepository) UpdateBancoHoras(ctx context.Context, id uuid.UUID, valor float64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `
		UPDATE membros SET banco_horas = $2, updated_at = NOW() WHERE id = $1
	`, id, valor)
	if err != nil {
		return fmt.Errorf("updating banco_horas: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not found", id)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO membro_banco_horas (membro_id, valor)
		VALUES ($1, $2)
	`, id, valor)
	if err != nil {
		return fmt.Errorf("inserting banco_horas history: %w", err)
	}

	return tx.Commit(ctx)
}
```

- [ ] **Step 4: Add `UpdateDataAdmissao` method**

```go
func (r *MembroRepository) UpdateDataAdmissao(ctx context.Context, id uuid.UUID, data *time.Time) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE membros SET data_admissao = $2, updated_at = NOW() WHERE id = $1
	`, id, data)
	if err != nil {
		return fmt.Errorf("updating data_admissao: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not found", id)
	}
	return nil
}
```

- [ ] **Step 5: Add `GetHistoricoSalario` method**

```go
func (r *MembroRepository) GetHistoricoSalario(ctx context.Context, membroID uuid.UUID) ([]domain.SalarioHistorico, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, membro_id, valor, data_vigencia, created_at
		FROM membro_salarios
		WHERE membro_id = $1
		ORDER BY data_vigencia ASC
	`, membroID)
	if err != nil {
		return nil, fmt.Errorf("querying salary history: %w", err)
	}
	defer rows.Close()

	var result []domain.SalarioHistorico
	for rows.Next() {
		var s domain.SalarioHistorico
		if err := rows.Scan(&s.ID, &s.MembroID, &s.Valor, &s.DataVigencia, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning salary history: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
```

- [ ] **Step 6: Add `GetHistoricoBancoHoras` method**

```go
func (r *MembroRepository) GetHistoricoBancoHoras(ctx context.Context, membroID uuid.UUID) ([]domain.BancoHorasHistorico, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, membro_id, valor, data_registro, created_at
		FROM membro_banco_horas
		WHERE membro_id = $1
		ORDER BY data_registro ASC
	`, membroID)
	if err != nil {
		return nil, fmt.Errorf("querying banco_horas history: %w", err)
	}
	defer rows.Close()

	var result []domain.BancoHorasHistorico
	for rows.Next() {
		var b domain.BancoHorasHistorico
		if err := rows.Scan(&b.ID, &b.MembroID, &b.Valor, &b.DataRegistro, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning banco_horas history: %w", err)
		}
		result = append(result, b)
	}
	return result, rows.Err()
}
```

- [ ] **Step 7: Verify compilation**

Run: `go build ./...`

- [ ] **Step 8: Commit**

```bash
git add internal/repository/membro.go
git commit -m "feat: add salary, hours-bank, hire-date update methods with history tracking"
```

---

### Task 4: Repository — Investimento Queries (Dashboard & Gastos Mensais)

**Files:**
- Create: `internal/repository/investimento.go`

**Interfaces:**
- Consumes: `domain.MembroInvestimento`, `domain.GastoMensal`, `domain.ProjetoAlocacao` from Task 2
- Produces: Repository used by Task 5 (service layer):
  - `GetTopProdutosMembro(ctx context.Context, membroID uuid.UUID, limit int) ([]string, error)`
  - `GetSalarioVigenteNoMes(ctx context.Context, membroID uuid.UUID, ano, mes int) (*float64, error)`
  - `GetAlocacoesProjetos(ctx context.Context, membroID uuid.UUID) ([]domain.ProjetoAlocacao, error)`
  - `GetMembrosEquipeNoMes(ctx context.Context, equipeID uuid.UUID, ano, mes int) ([]domain.Membro, error)`

- [ ] **Step 1: Create `internal/repository/investimento.go`**

```go
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InvestimentoRepository struct {
	pool *pgxpool.Pool
}

func NewInvestimentoRepository(pool *pgxpool.Pool) *InvestimentoRepository {
	return &InvestimentoRepository{pool: pool}
}
```

- [ ] **Step 2: Add `GetTopProdutosMembro`**

Returns top N product names by task count for a member.

```go
func (r *InvestimentoRepository) GetTopProdutosMembro(ctx context.Context, membroID uuid.UUID, limit int) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.nome, COUNT(*) as cnt
		FROM tarefas t
		JOIN tarefa_produtos tp ON tp.tarefa_id = t.id
		JOIN produtos p ON p.id = tp.produto_id
		WHERE t.responsavel_id = $1
		  AND t.removido_em IS NULL
		GROUP BY p.nome
		ORDER BY cnt DESC
		LIMIT $2
	`, membroID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying top produtos: %w", err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var nome string
		var cnt int
		if err := rows.Scan(&nome, &cnt); err != nil {
			return nil, fmt.Errorf("scanning top produto: %w", err)
		}
		result = append(result, nome)
	}
	return result, rows.Err()
}
```

- [ ] **Step 3: Add `GetSalarioVigenteNoMes`**

Returns the salary in effect for a given month. Looks at `membro_salarios` for the most recent record with `data_vigencia <= last day of month`. Falls back to `membros.salario` if no history.

```go
func (r *InvestimentoRepository) GetSalarioVigenteNoMes(ctx context.Context, membroID uuid.UUID, ano, mes int) (*float64, error) {
	lastDay := time.Date(ano, time.Month(mes)+1, 0, 0, 0, 0, 0, time.UTC)

	var valor *float64
	err := r.pool.QueryRow(ctx, `
		SELECT valor FROM membro_salarios
		WHERE membro_id = $1 AND data_vigencia <= $2
		ORDER BY data_vigencia DESC
		LIMIT 1
	`, membroID, lastDay).Scan(&valor)
	if err != nil {
		err2 := r.pool.QueryRow(ctx, `
			SELECT salario FROM membros WHERE id = $1
		`, membroID).Scan(&valor)
		if err2 != nil {
			return nil, fmt.Errorf("getting fallback salario: %w", err2)
		}
	}
	return valor, nil
}
```

- [ ] **Step 4: Add `GetMembrosEquipeNoMes`**

Returns members who were active in a given month (admitted before end of month, not terminated before start of month).

```go
func (r *InvestimentoRepository) GetMembrosEquipeNoMes(ctx context.Context, equipeID uuid.UUID, ano, mes int) ([]domain.Membro, error) {
	firstDay := time.Date(ano, time.Month(mes), 1, 0, 0, 0, 0, time.UTC)
	lastDay := time.Date(ano, time.Month(mes)+1, 0, 0, 0, 0, 0, time.UTC)

	rows, err := r.pool.Query(ctx, `
		SELECT m.id, m.fonte_dados_id, m.jira_account_id, m.nome, m.email,
		       m.avatar_url, m.team, m.ativo, m.data_desligamento, m.cargo,
		       m.salario, m.data_admissao, m.banco_horas,
		       m.created_at, m.updated_at
		FROM membros m
		INNER JOIN equipe_membros em ON em.membro_id = m.id AND em.equipe_id = $1
		WHERE (m.data_admissao IS NULL OR m.data_admissao <= $2)
		  AND (m.data_desligamento IS NULL OR m.data_desligamento >= $3)
	`, equipeID, lastDay, firstDay)
	if err != nil {
		return nil, fmt.Errorf("querying membros for month: %w", err)
	}
	defer rows.Close()

	var membros []domain.Membro
	for rows.Next() {
		var m domain.Membro
		if err := rows.Scan(
			&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email,
			&m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo,
			&m.Salario, &m.DataAdmissao, &m.BancoHoras,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning membro: %w", err)
		}
		membros = append(membros, m)
	}
	return membros, rows.Err()
}
```

- [ ] **Step 5: Add `GetAlocacoesProjetos`**

Returns project allocations for a member, using existing allocation data. Percentage = hours on project / total hours × 100.

```go
func (r *InvestimentoRepository) GetAlocacoesProjetos(ctx context.Context, membroID uuid.UUID) ([]domain.ProjetoAlocacao, error) {
	rows, err := r.pool.Query(ctx, `
		WITH member_hours AS (
			SELECT
				p.nome AS apelido,
				p.jira_id AS chave_jira,
				COALESCE(SUM(t.estimativa_tempo), 0)::float8 / 3600.0 AS horas
			FROM tarefas t
			JOIN tarefa_produtos tp ON tp.tarefa_id = t.id
			JOIN produtos p ON p.id = tp.produto_id
			WHERE t.responsavel_id = $1
			  AND t.removido_em IS NULL
			  AND t.status NOT IN ('Cancelado', 'Rejeitada')
			GROUP BY p.nome, p.jira_id
		),
		total AS (
			SELECT COALESCE(SUM(horas), 0) AS total_horas FROM member_hours
		)
		SELECT
			mh.apelido,
			COALESCE(mh.chave_jira, ''),
			CASE WHEN t.total_horas > 0 THEN ROUND((mh.horas / t.total_horas * 100)::numeric, 1) ELSE 0 END
		FROM member_hours mh, total t
		ORDER BY mh.horas DESC
	`, membroID)
	if err != nil {
		return nil, fmt.Errorf("querying project allocations: %w", err)
	}
	defer rows.Close()

	var result []domain.ProjetoAlocacao
	for rows.Next() {
		var p domain.ProjetoAlocacao
		if err := rows.Scan(&p.Apelido, &p.ChaveJira, &p.PercentualAlocacao); err != nil {
			return nil, fmt.Errorf("scanning allocation: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
```

- [ ] **Step 6: Verify compilation**

Run: `go build ./...`

- [ ] **Step 7: Commit**

```bash
git add internal/repository/investimento.go
git commit -m "feat: add investimento repository with dashboard and allocation queries"
```

---

### Task 5: Service — InvestimentoService

**Files:**
- Create: `internal/service/investimento.go`
- Create: `internal/service/investimento_test.go`

**Interfaces:**
- Consumes:
  - `*repository.EquipeRepository` — `GetEquipeByID`, `GetMembrosEquipe`
  - `*repository.MembroRepository` — `UpdateSalario`, `UpdateBancoHoras`, `UpdateDataAdmissao`, `GetHistoricoSalario`, `GetHistoricoBancoHoras`
  - `*repository.InvestimentoRepository` — `GetTopProdutosMembro`, `GetSalarioVigenteNoMes`, `GetMembrosEquipeNoMes`, `GetAlocacoesProjetos`
- Produces: `InvestimentoService` with methods consumed by Task 6 (handler):
  - `GetDashboard(ctx context.Context, equipeID uuid.UUID) (*domain.InvestimentoDashboard, error)`
  - `GetGastosMensais(ctx context.Context, equipeID uuid.UUID, ano int) (*domain.GastosMensaisResponse, error)`
  - `GetAlocacoesProjetos(ctx context.Context, membroID uuid.UUID) (*domain.AlocacoesProjetosResponse, error)`

- [ ] **Step 1: Create `internal/service/investimento.go`**

```go
package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type InvestimentoService struct {
	equipeRepo     *repository.EquipeRepository
	membroRepo     *repository.MembroRepository
	investRepo     *repository.InvestimentoRepository
	logger         *zap.Logger
}

func NewInvestimentoService(
	equipeRepo *repository.EquipeRepository,
	membroRepo *repository.MembroRepository,
	investRepo *repository.InvestimentoRepository,
	logger *zap.Logger,
) *InvestimentoService {
	return &InvestimentoService{
		equipeRepo: equipeRepo,
		membroRepo: membroRepo,
		investRepo: investRepo,
		logger:     logger,
	}
}

func (s *InvestimentoService) GetDashboard(ctx context.Context, equipeID uuid.UUID) (*domain.InvestimentoDashboard, error) {
	equipe, err := s.equipeRepo.GetEquipeByID(ctx, equipeID)
	if err != nil {
		return nil, fmt.Errorf("getting equipe: %w", err)
	}
	if equipe == nil {
		return nil, fmt.Errorf("equipe %s not found", equipeID)
	}

	membros, err := s.equipeRepo.GetMembrosEquipe(ctx, equipeID)
	if err != nil {
		return nil, fmt.Errorf("getting membros: %w", err)
	}

	now := time.Now()
	var custoTotal float64
	var temposCasa []int
	var bancoHorasTotal float64
	membrosList := make([]domain.MembroInvestimento, 0, len(membros))

	for _, m := range membros {
		tempoCasa := 0
		if m.DataAdmissao != nil {
			tempoCasa = calcTempoCasaMeses(*m.DataAdmissao, now)
		}
		temposCasa = append(temposCasa, tempoCasa)

		if m.Salario != nil {
			custoTotal += *m.Salario
		}
		if m.BancoHoras != nil {
			bancoHorasTotal += *m.BancoHoras
		}

		topProd, err := s.investRepo.GetTopProdutosMembro(ctx, m.ID, 3)
		if err != nil {
			s.logger.Warn("failed to get top produtos", zap.String("membro", m.Nome), zap.Error(err))
			topProd = []string{}
		}

		var dataAdmStr *string
		if m.DataAdmissao != nil {
			s := m.DataAdmissao.Format("2006-01-02")
			dataAdmStr = &s
		}

		membrosList = append(membrosList, domain.MembroInvestimento{
			ID:             m.ID,
			Nome:           m.Nome,
			AvatarURL:      m.AvatarURL,
			Salario:        m.Salario,
			DataAdmissao:   dataAdmStr,
			TempoCasaMeses: tempoCasa,
			BancoHoras:     m.BancoHoras,
			Cargo:          m.Cargo,
			TopProdutos:    topProd,
		})
	}

	// Sort by salary descending
	sortMembrosBySalarioDesc(membrosList)

	tempoMedio := 0
	if len(temposCasa) > 0 {
		sum := 0
		for _, t := range temposCasa {
			sum += t
		}
		tempoMedio = sum / len(temposCasa)
	}

	return &domain.InvestimentoDashboard{
		Equipe: domain.EquipeInfo{ID: equipeID, Nome: equipe.Nome},
		Sumario: domain.InvestimentoSumario{
			CustoMensalTotal:    custoTotal,
			TotalMembros:        len(membros),
			TempoCasaMedioMeses: tempoMedio,
			BancoHorasTotal:     bancoHorasTotal,
		},
		Membros: membrosList,
	}, nil
}

func (s *InvestimentoService) GetGastosMensais(ctx context.Context, equipeID uuid.UUID, ano int) (*domain.GastosMensaisResponse, error) {
	meses := make([]domain.GastoMensal, 0, 12)

	for mes := 1; mes <= 12; mes++ {
		membros, err := s.investRepo.GetMembrosEquipeNoMes(ctx, equipeID, ano, mes)
		if err != nil {
			return nil, fmt.Errorf("getting membros for month %d: %w", mes, err)
		}

		var custoMes float64
		for _, m := range membros {
			salario, err := s.investRepo.GetSalarioVigenteNoMes(ctx, m.ID, ano, mes)
			if err != nil {
				s.logger.Warn("failed to get salary for month", zap.Int("mes", mes), zap.Error(err))
				continue
			}
			if salario != nil {
				custoMes += *salario
			}
		}

		meses = append(meses, domain.GastoMensal{
			Mes:        mes,
			CustoTotal: math.Round(custoMes*100) / 100,
		})
	}

	return &domain.GastosMensaisResponse{Ano: ano, Meses: meses}, nil
}

func (s *InvestimentoService) GetAlocacoesProjetos(ctx context.Context, membroID uuid.UUID) (*domain.AlocacoesProjetosResponse, error) {
	projetos, err := s.investRepo.GetAlocacoesProjetos(ctx, membroID)
	if err != nil {
		return nil, fmt.Errorf("getting alocacoes: %w", err)
	}
	if projetos == nil {
		projetos = []domain.ProjetoAlocacao{}
	}
	return &domain.AlocacoesProjetosResponse{Projetos: projetos}, nil
}

func calcTempoCasaMeses(admissao, now time.Time) int {
	years := now.Year() - admissao.Year()
	months := int(now.Month()) - int(admissao.Month())
	if now.Day() < admissao.Day() {
		months--
	}
	total := years*12 + months
	if total < 0 {
		return 0
	}
	return total
}

func sortMembrosBySalarioDesc(membros []domain.MembroInvestimento) {
	for i := 0; i < len(membros); i++ {
		for j := i + 1; j < len(membros); j++ {
			si := 0.0
			sj := 0.0
			if membros[i].Salario != nil {
				si = *membros[i].Salario
			}
			if membros[j].Salario != nil {
				sj = *membros[j].Salario
			}
			if sj > si {
				membros[i], membros[j] = membros[j], membros[i]
			}
		}
	}
}
```

- [ ] **Step 2: Write tests — `internal/service/investimento_test.go`**

```go
package service

import (
	"testing"
	"time"
)

func TestCalcTempoCasaMeses(t *testing.T) {
	tests := []struct {
		name     string
		admissao time.Time
		now      time.Time
		want     int
	}{
		{
			name:     "exact 2 years",
			admissao: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
			want:     24,
		},
		{
			name:     "partial month",
			admissao: time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
			want:     28,
		},
		{
			name:     "same month",
			admissao: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
			want:     0,
		},
		{
			name:     "future admission returns 0",
			admissao: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
			now:      time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcTempoCasaMeses(tt.admissao, tt.now)
			if got != tt.want {
				t.Errorf("calcTempoCasaMeses() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSortMembrosBySalarioDesc(t *testing.T) {
	s1 := 5000.0
	s2 := 12000.0
	s3 := 8000.0

	membros := []domain.MembroInvestimento{
		{Nome: "A", Salario: &s1},
		{Nome: "B", Salario: &s2},
		{Nome: "C", Salario: &s3},
		{Nome: "D", Salario: nil},
	}

	sortMembrosBySalarioDesc(membros)

	if membros[0].Nome != "B" {
		t.Errorf("first = %s, want B (12000)", membros[0].Nome)
	}
	if membros[1].Nome != "C" {
		t.Errorf("second = %s, want C (8000)", membros[1].Nome)
	}
	if membros[2].Nome != "A" {
		t.Errorf("third = %s, want A (5000)", membros[2].Nome)
	}
	if membros[3].Nome != "D" {
		t.Errorf("fourth = %s, want D (nil)", membros[3].Nome)
	}
}
```

Note: `TestSortMembrosBySalarioDesc` uses `domain.MembroInvestimento` — since tests are in `package service`, import `domain` and use the full type. Adjust if needed:

```go
import "github.com/emersonpaula83/myplanner/backend/internal/domain"
```

And change `MembroInvestimento` to `domain.MembroInvestimento` in the test.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/service/ -run "TestCalcTempoCasa|TestSortMembros" -v`

Expected: PASS

- [ ] **Step 4: Verify full compilation**

Run: `go build ./...`

- [ ] **Step 5: Commit**

```bash
git add internal/service/investimento.go internal/service/investimento_test.go
git commit -m "feat: add InvestimentoService with dashboard, gastos mensais, and allocation logic"
```

---

### Task 6: Handler and Route Registration

**Files:**
- Create: `internal/handler/investimento.go`
- Modify: `cmd/api/main.go` (add routes and service/handler instantiation)

**Interfaces:**
- Consumes: `InvestimentoService` from Task 5, `*repository.MembroRepository` from Task 3
- Produces: HTTP endpoints — the final deliverable of SP1

- [ ] **Step 1: Create `internal/handler/investimento.go`**

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type InvestimentoStore interface {
	GetDashboard(ctx context.Context, equipeID uuid.UUID) (*domain.InvestimentoDashboard, error)
	GetGastosMensais(ctx context.Context, equipeID uuid.UUID, ano int) (*domain.GastosMensaisResponse, error)
	GetAlocacoesProjetos(ctx context.Context, membroID uuid.UUID) (*domain.AlocacoesProjetosResponse, error)
}

type MembroFinanceiroStore interface {
	UpdateSalario(ctx context.Context, id uuid.UUID, valor float64) error
	UpdateBancoHoras(ctx context.Context, id uuid.UUID, valor float64) error
	UpdateDataAdmissao(ctx context.Context, id uuid.UUID, data *time.Time) error
	GetHistoricoSalario(ctx context.Context, membroID uuid.UUID) ([]domain.SalarioHistorico, error)
	GetHistoricoBancoHoras(ctx context.Context, membroID uuid.UUID) ([]domain.BancoHorasHistorico, error)
}

type InvestimentoHandler struct {
	store       InvestimentoStore
	membroStore MembroFinanceiroStore
	logger      *zap.Logger
}

func NewInvestimentoHandler(store InvestimentoStore, membroStore MembroFinanceiroStore, logger *zap.Logger) *InvestimentoHandler {
	return &InvestimentoHandler{store: store, membroStore: membroStore, logger: logger}
}

func (h *InvestimentoHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	equipeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	dashboard, err := h.store.GetDashboard(r.Context(), equipeID)
	if err != nil {
		h.logger.Error("failed to get investimento dashboard", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao carregar dashboard")
		return
	}

	respondJSON(w, http.StatusOK, dashboard)
}

func (h *InvestimentoHandler) GetGastosMensais(w http.ResponseWriter, r *http.Request) {
	equipeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	anoStr := r.URL.Query().Get("ano")
	ano := time.Now().Year()
	if anoStr != "" {
		ano, err = strconv.Atoi(anoStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "ano inválido")
			return
		}
	}

	result, err := h.store.GetGastosMensais(r.Context(), equipeID, ano)
	if err != nil {
		h.logger.Error("failed to get gastos mensais", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao carregar gastos mensais")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (h *InvestimentoHandler) UpdateSalario(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req struct {
		Valor float64 `json:"valor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if req.Valor < 0 {
		respondError(w, http.StatusBadRequest, "valor deve ser >= 0")
		return
	}

	if err := h.membroStore.UpdateSalario(r.Context(), id, req.Valor); err != nil {
		h.logger.Error("failed to update salario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar salário")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "salário atualizado"})
}

func (h *InvestimentoHandler) UpdateBancoHoras(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req struct {
		Valor float64 `json:"valor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	if err := h.membroStore.UpdateBancoHoras(r.Context(), id, req.Valor); err != nil {
		h.logger.Error("failed to update banco_horas", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar banco de horas")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "banco de horas atualizado"})
}

func (h *InvestimentoHandler) UpdateDataAdmissao(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req struct {
		DataAdmissao *string `json:"data_admissao"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	var dt *time.Time
	if req.DataAdmissao != nil && *req.DataAdmissao != "" {
		parsed, err := time.Parse("2006-01-02", *req.DataAdmissao)
		if err != nil {
			respondError(w, http.StatusBadRequest, "data_admissao inválida (formato: YYYY-MM-DD)")
			return
		}
		dt = &parsed
	}

	if err := h.membroStore.UpdateDataAdmissao(r.Context(), id, dt); err != nil {
		h.logger.Error("failed to update data_admissao", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar data de admissão")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "data de admissão atualizada"})
}

func (h *InvestimentoHandler) GetHistoricoSalario(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	historico, err := h.membroStore.GetHistoricoSalario(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get historico salario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao carregar histórico salarial")
		return
	}
	if historico == nil {
		historico = []domain.SalarioHistorico{}
	}

	respondJSON(w, http.StatusOK, historico)
}

func (h *InvestimentoHandler) GetHistoricoBancoHoras(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	historico, err := h.membroStore.GetHistoricoBancoHoras(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get historico banco_horas", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao carregar histórico banco de horas")
		return
	}
	if historico == nil {
		historico = []domain.BancoHorasHistorico{}
	}

	respondJSON(w, http.StatusOK, historico)
}

func (h *InvestimentoHandler) GetAlocacoesProjetos(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	result, err := h.store.GetAlocacoesProjetos(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get alocacoes projetos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao carregar alocações")
		return
	}

	respondJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 2: Register routes and instantiate in `cmd/api/main.go`**

**After the existing `membroHandler` instantiation (~line 104), add:**

```go
investRepo := repository.NewInvestimentoRepository(pool)
investService := service.NewInvestimentoService(equipeRepo, membroRepo, investRepo, logger)
investHandler := handler.NewInvestimentoHandler(investService, membroRepo, logger)
```

**In the route registration section, after the equipe routes (~line 245), add:**

```go
// Investimentos
r.Get("/equipes/{id}/investimentos", investHandler.GetDashboard)
r.Get("/equipes/{id}/investimentos/gastos-mensais", investHandler.GetGastosMensais)

// Membro financial
r.Put("/membros/{id}/salario", investHandler.UpdateSalario)
r.Put("/membros/{id}/banco-horas", investHandler.UpdateBancoHoras)
r.Put("/membros/{id}/data-admissao", investHandler.UpdateDataAdmissao)
r.Get("/membros/{id}/salario/historico", investHandler.GetHistoricoSalario)
r.Get("/membros/{id}/banco-horas/historico", investHandler.GetHistoricoBancoHoras)
r.Get("/membros/{id}/alocacoes-projetos", investHandler.GetAlocacoesProjetos)
```

Make sure to add the `service` import if not already present in main.go.

- [ ] **Step 3: Verify compilation**

Run: `go build ./...`

Fix any import issues.

- [ ] **Step 4: Run all tests**

Run: `go test ./internal/... -v`

Expected: all tests pass — no regressions.

- [ ] **Step 5: Manual smoke test**

Start the server and test one endpoint:

```bash
go run cmd/api/main.go &
# Get an equipe ID from the database
curl -s http://localhost:8080/api/v1/equipes | jq '.[0].id'
# Test dashboard endpoint
curl -s http://localhost:8080/api/v1/equipes/<EQUIPE_ID>/investimentos | jq .
# Test gastos mensais
curl -s http://localhost:8080/api/v1/equipes/<EQUIPE_ID>/investimentos/gastos-mensais | jq .
```

Expected: valid JSON responses with team data. Salaries/banco_horas will be null until values are set.

- [ ] **Step 6: Commit**

```bash
git add internal/handler/investimento.go cmd/api/main.go
git commit -m "feat: add investimento handler with dashboard and financial update endpoints"
```

---
