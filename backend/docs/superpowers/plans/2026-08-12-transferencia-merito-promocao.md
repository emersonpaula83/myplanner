# Transferência de Equipe + Mérito/Promoção Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable team transfers with historical preservation and register merit/promotion events with salary/cargo tracking for investment reports.

**Architecture:** Add temporal columns (data_entrada, data_saida) to equipe_membros for membership history. Transfer = atomic close-old + create-new. New historico_salario table records mérito/promoção events with before/after snapshots. All equipe_membros queries gain temporal filtering. Frontend adds transfer button, auto-transfer on conflict, and mérito/promoção modal.

**Tech Stack:** Go 1.22, chi router, pgxpool (PostgreSQL), vanilla JS/HTML/CSS monolith

## Global Constraints

- Membro can have at most 1 active team membership (enforced by partial unique index WHERE data_saida IS NULL)
- Transferência = close old membership + create new (atomic transaction)
- Past reports must reflect team composition as it was during that period
- Salário novo >= salário atual (mérito/promoção)
- Promoções follow cargo hierarchy defined in domain/cargo.go
- Existing membro_salarios table continues tracking raw salary history for investimentos gastos-mensais; the new historico_salario table tracks mérito/promoção event records with before/after snapshots
- No service layer for equipes — handler calls repository directly via EquipeStore interface
- Frontend is a single monolithic index.html (~8500 lines), vanilla JS

---

### Task 1: Migration + Domain Types + Critical Repository Fixes

**Files:**
- Create: `backend/migrations/000032_transferencia_temporal.up.sql`
- Create: `backend/migrations/000032_transferencia_temporal.down.sql`
- Modify: `backend/internal/domain/cargo.go` — add PromocoesValidas map
- Create: `backend/internal/domain/merito.go` — new types for mérito/promoção
- Modify: `backend/internal/repository/equipe.go:110-165` — fix GetMembrosEquipe, AddMembroEquipe, RemoveMembroEquipe for temporal model
- Modify: `backend/internal/handler/equipe.go:15-29` — add new methods to EquipeStore interface
- Test: `backend/internal/domain/cargo_test.go`

**Interfaces:**
- Consumes: existing `domain.Membro`, `domain.Equipe`, cargo constants from `domain/cargo.go`
- Produces:
  - `domain.PromocoesValidas` — `map[string][]string` mapping cargo slug → valid promotion targets
  - `domain.HistoricoMeritoPromocao` — struct with ID, MembroID, Tipo, CargoAnterior, CargoNovo, SalarioAnterior, SalarioNovo, DataVigencia, CreatedAt
  - `domain.MeritoPromocaoRequest` — struct with Tipo, CargoNovo, SalarioNovo, DataVigencia
  - `domain.MeritoPromocaoResponse` — struct with HistoricoID, Antes, Depois snapshots
  - Updated `EquipeRepository.AddMembroEquipe` — now uses `ON CONFLICT ON CONSTRAINT equipe_membros_active_unique DO NOTHING`
  - Updated `EquipeRepository.RemoveMembroEquipe` — soft-delete via UPDATE SET data_saida = NOW()
  - Updated `EquipeRepository.GetMembrosEquipe` — filters WHERE data_saida IS NULL
  - `EquipeStore` interface gains: `GetEquipeAtivaMembro(ctx, membroID) (*domain.Equipe, error)`, `TransferirMembro(ctx, equipeOrigemID, equipeDestinoID, membroID) error`, `InsertMeritoPromocao(ctx, ...) (*domain.HistoricoMeritoPromocao, error)`, `GetMembrosEquipeComEntrada(ctx, equipeID) ([]MembroComEntrada, error)`

- [ ] **Step 1: Write migration up**

```sql
-- backend/migrations/000032_transferencia_temporal.up.sql

-- 1. Add temporal columns to equipe_membros
ALTER TABLE equipe_membros
  ADD COLUMN data_entrada TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN data_saida TIMESTAMPTZ;

-- 2. Replace UNIQUE constraint with partial unique index
ALTER TABLE equipe_membros DROP CONSTRAINT equipe_membros_equipe_id_membro_id_key;
CREATE UNIQUE INDEX equipe_membros_active_unique
  ON equipe_membros(membro_id)
  WHERE data_saida IS NULL;

-- 3. Index for temporal queries
CREATE INDEX idx_equipe_membros_temporal
  ON equipe_membros(equipe_id, data_entrada, data_saida);

-- 4. New table for mérito/promoção event records
CREATE TABLE historico_salario (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  membro_id UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
  tipo VARCHAR(20) NOT NULL CHECK (tipo IN ('merito', 'promocao')),
  cargo_anterior VARCHAR(100),
  cargo_novo VARCHAR(100),
  salario_anterior NUMERIC(12,2),
  salario_novo NUMERIC(12,2) NOT NULL,
  data_vigencia DATE NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_historico_salario_membro ON historico_salario(membro_id);
```

Note: The partial unique index is on `(membro_id)` alone — not `(equipe_id, membro_id)` — enforcing at most 1 active membership across ALL teams, which is the business rule.

- [ ] **Step 2: Write migration down**

```sql
-- backend/migrations/000032_transferencia_temporal.down.sql
DROP TABLE IF EXISTS historico_salario;
DROP INDEX IF EXISTS idx_equipe_membros_temporal;
DROP INDEX IF EXISTS equipe_membros_active_unique;
ALTER TABLE equipe_membros ADD CONSTRAINT equipe_membros_equipe_id_membro_id_key UNIQUE (equipe_id, membro_id);
ALTER TABLE equipe_membros DROP COLUMN IF EXISTS data_saida;
ALTER TABLE equipe_membros DROP COLUMN IF EXISTS data_entrada;
```

- [ ] **Step 3: Run migration**

```bash
PGPASSWORD=$(grep DB_PASS .env | cut -d= -f2) psql -h localhost -U myplanner -d myplanner -f migrations/000032_transferencia_temporal.up.sql
PGPASSWORD=$(grep DB_PASS .env | cut -d= -f2) psql -h localhost -U myplanner -d myplanner -c "UPDATE schema_migrations SET version = 32, dirty = false;"
```

Verify:
```bash
PGPASSWORD=$(grep DB_PASS .env | cut -d= -f2) psql -h localhost -U myplanner -d myplanner -c "\d equipe_membros"
```
Expected: data_entrada and data_saida columns present. No UNIQUE constraint on (equipe_id, membro_id). Partial unique index equipe_membros_active_unique visible.

- [ ] **Step 4: Add cargo promotion hierarchy to domain/cargo.go**

Add after `IsCargoValido` function:

```go
var PromocoesValidas = map[string][]string{
	CargoAnalistaI:                  {CargoAnalistaII},
	CargoAnalistaII:                 {CargoAnalistaIII},
	CargoAnalistaIII:                {CargoEspecialistaI, CargoCoordenadorDesenvolvimento},
	CargoEspecialistaI:              {CargoEspecialistaII, CargoCoordenadorDesenvolvimento, CargoLiderTecnico},
	CargoEspecialistaII:             {CargoMaster, CargoLiderTecnico},
	CargoCoordenadorDesenvolvimento: {CargoLiderTecnico},
}

func IsPromocaoValida(cargoAtual, cargoNovo string) bool {
	validas, ok := PromocoesValidas[cargoAtual]
	if !ok {
		return false
	}
	return slices.Contains(validas, cargoNovo)
}
```

- [ ] **Step 5: Write test for cargo promotion hierarchy**

Create `backend/internal/domain/cargo_test.go`:

```go
package domain

import "testing"

func TestIsPromocaoValida(t *testing.T) {
	tests := []struct {
		atual, novo string
		want        bool
	}{
		{CargoAnalistaI, CargoAnalistaII, true},
		{CargoAnalistaII, CargoAnalistaIII, true},
		{CargoAnalistaIII, CargoEspecialistaI, true},
		{CargoAnalistaIII, CargoCoordenadorDesenvolvimento, true},
		{CargoEspecialistaI, CargoEspecialistaII, true},
		{CargoEspecialistaI, CargoLiderTecnico, true},
		{CargoEspecialistaII, CargoMaster, true},
		{CargoEspecialistaII, CargoLiderTecnico, true},
		{CargoCoordenadorDesenvolvimento, CargoLiderTecnico, true},
		// Invalid promotions
		{CargoAnalistaI, CargoAnalistaIII, false},
		{CargoAnalistaII, CargoEspecialistaI, false},
		{CargoMaster, CargoLiderTecnico, false},
		{CargoLiderTecnico, CargoMaster, false},
		{"", CargoAnalistaI, false},
		{CargoAnalistaI, CargoAnalistaI, false},
	}
	for _, tt := range tests {
		t.Run(tt.atual+"->"+tt.novo, func(t *testing.T) {
			got := IsPromocaoValida(tt.atual, tt.novo)
			if got != tt.want {
				t.Errorf("IsPromocaoValida(%q, %q) = %v, want %v", tt.atual, tt.novo, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 6: Run cargo tests**

```bash
go test ./internal/domain/ -run TestIsPromocaoValida -v
```
Expected: all cases PASS.

- [ ] **Step 7: Create domain/merito.go with new types**

```go
package domain

import (
	"time"

	"github.com/google/uuid"
)

type HistoricoMeritoPromocao struct {
	ID               uuid.UUID `json:"id"`
	MembroID         uuid.UUID `json:"membro_id"`
	Tipo             string    `json:"tipo"`
	CargoAnterior    *string   `json:"cargo_anterior"`
	CargoNovo        *string   `json:"cargo_novo"`
	SalarioAnterior  *float64  `json:"salario_anterior"`
	SalarioNovo      float64   `json:"salario_novo"`
	DataVigencia     time.Time `json:"data_vigencia"`
	CreatedAt        time.Time `json:"created_at"`
}

type MeritoPromocaoRequest struct {
	Tipo         string  `json:"tipo"`
	CargoNovo    *string `json:"cargo_novo"`
	SalarioNovo  float64 `json:"salario_novo"`
	DataVigencia string  `json:"data_vigencia"`
}

type MembroSnapshot struct {
	Cargo   *string  `json:"cargo"`
	Salario *float64 `json:"salario"`
}

type MeritoPromocaoResponse struct {
	HistoricoID uuid.UUID      `json:"historico_id"`
	Antes       MembroSnapshot `json:"antes"`
	Depois      MembroSnapshot `json:"depois"`
}

type MembroComEntrada struct {
	Membro      Membro    `json:"membro"`
	DataEntrada time.Time `json:"data_entrada"`
}

type TransferConflict struct {
	Conflito   bool      `json:"conflito"`
	EquipeAtual struct {
		ID   uuid.UUID `json:"id"`
		Nome string    `json:"nome"`
	} `json:"equipe_atual"`
}
```

- [ ] **Step 8: Fix GetMembrosEquipe — add data_saida IS NULL filter**

In `backend/internal/repository/equipe.go`, modify `GetMembrosEquipe` (line ~117):

Change the JOIN line from:
```sql
INNER JOIN equipe_membros em ON em.membro_id = m.id AND em.equipe_id = $1
```
to:
```sql
INNER JOIN equipe_membros em ON em.membro_id = m.id AND em.equipe_id = $1 AND em.data_saida IS NULL
```

- [ ] **Step 9: Fix AddMembroEquipe — use partial unique index constraint name**

Replace the existing `AddMembroEquipe` function body (equipe.go:143-152) with:

```go
func (r *EquipeRepository) AddMembroEquipe(ctx context.Context, equipeID uuid.UUID, membroID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO equipe_membros (equipe_id, membro_id, data_entrada)
		VALUES ($1, $2, NOW())
		ON CONFLICT ON CONSTRAINT equipe_membros_active_unique DO NOTHING
	`, equipeID, membroID)
	if err != nil {
		return fmt.Errorf("adding membro to equipe: %w", err)
	}
	return nil
}
```

Note: ON CONFLICT uses the named partial unique index. If membro has no active membership, insert succeeds. If membro already active in ANY team, conflict → DO NOTHING.

- [ ] **Step 10: Fix RemoveMembroEquipe — soft-delete via UPDATE**

Replace the existing `RemoveMembroEquipe` function body (equipe.go:154-165) with:

```go
func (r *EquipeRepository) RemoveMembroEquipe(ctx context.Context, equipeID uuid.UUID, membroID uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE equipe_membros SET data_saida = NOW()
		WHERE equipe_id = $1 AND membro_id = $2 AND data_saida IS NULL
	`, equipeID, membroID)
	if err != nil {
		return fmt.Errorf("removing membro from equipe: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not active in equipe %s", membroID, equipeID)
	}
	return nil
}
```

- [ ] **Step 11: Add GetEquipeAtivaMembro to repository**

Add to `backend/internal/repository/equipe.go` after RemoveMembroEquipe:

```go
func (r *EquipeRepository) GetEquipeAtivaMembro(ctx context.Context, membroID uuid.UUID) (*domain.Equipe, error) {
	var e domain.Equipe
	err := r.pool.QueryRow(ctx, `
		SELECT eq.id, eq.nome, eq.board_id, eq.created_at, eq.updated_at
		FROM equipes eq
		INNER JOIN equipe_membros em ON em.equipe_id = eq.id
		WHERE em.membro_id = $1 AND em.data_saida IS NULL
	`, membroID).Scan(&e.ID, &e.Nome, &e.BoardID, &e.CreatedAt, &e.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting equipe ativa for membro: %w", err)
	}
	return &e, nil
}
```

- [ ] **Step 12: Add TransferirMembro to repository**

Add to `backend/internal/repository/equipe.go`:

```go
func (r *EquipeRepository) TransferirMembro(ctx context.Context, equipeOrigemID, equipeDestinoID, membroID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, `
		UPDATE equipe_membros SET data_saida = NOW()
		WHERE membro_id = $1 AND equipe_id = $2 AND data_saida IS NULL
	`, membroID, equipeOrigemID)
	if err != nil {
		return fmt.Errorf("closing old membership: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not active in equipe %s", membroID, equipeOrigemID)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO equipe_membros (equipe_id, membro_id, data_entrada)
		VALUES ($1, $2, NOW())
	`, equipeDestinoID, membroID)
	if err != nil {
		return fmt.Errorf("creating new membership: %w", err)
	}

	return tx.Commit(ctx)
}
```

- [ ] **Step 13: Add GetMembrosEquipeComEntrada to repository**

Add to `backend/internal/repository/equipe.go`:

```go
func (r *EquipeRepository) GetMembrosEquipeComEntrada(ctx context.Context, equipeID uuid.UUID) ([]domain.MembroComEntrada, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT m.id, m.fonte_dados_id, m.jira_account_id, m.nome, m.email,
		       m.avatar_url, m.team, m.ativo, m.data_desligamento, m.cargo,
		       m.salario, m.data_admissao, m.banco_horas,
		       m.created_at, m.updated_at, em.data_entrada
		FROM membros m
		INNER JOIN equipe_membros em ON em.membro_id = m.id AND em.equipe_id = $1 AND em.data_saida IS NULL
		WHERE m.ativo = true
		  AND (m.data_desligamento IS NULL OR m.data_desligamento > NOW())
		ORDER BY m.nome
	`, equipeID)
	if err != nil {
		return nil, fmt.Errorf("getting membros com entrada: %w", err)
	}
	defer rows.Close()

	result := make([]domain.MembroComEntrada, 0)
	for rows.Next() {
		var mc domain.MembroComEntrada
		if err := rows.Scan(
			&mc.Membro.ID, &mc.Membro.FonteDadosID, &mc.Membro.JiraAccountID, &mc.Membro.Nome, &mc.Membro.Email,
			&mc.Membro.AvatarURL, &mc.Membro.Team, &mc.Membro.Ativo, &mc.Membro.DataDesligamento, &mc.Membro.Cargo,
			&mc.Membro.Salario, &mc.Membro.DataAdmissao, &mc.Membro.BancoHoras,
			&mc.Membro.CreatedAt, &mc.Membro.UpdatedAt, &mc.DataEntrada,
		); err != nil {
			return nil, fmt.Errorf("scanning membro com entrada: %w", err)
		}
		result = append(result, mc)
	}
	return result, rows.Err()
}
```

- [ ] **Step 14: Add InsertMeritoPromocao to repository**

Add to `backend/internal/repository/equipe.go`:

```go
func (r *EquipeRepository) InsertMeritoPromocao(ctx context.Context, membroID uuid.UUID, tipo string, cargoAnterior, cargoNovo *string, salarioAnterior *float64, salarioNovo float64, dataVigencia time.Time) (*domain.HistoricoMeritoPromocao, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var h domain.HistoricoMeritoPromocao
	err = tx.QueryRow(ctx, `
		INSERT INTO historico_salario (membro_id, tipo, cargo_anterior, cargo_novo, salario_anterior, salario_novo, data_vigencia)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, membro_id, tipo, cargo_anterior, cargo_novo, salario_anterior, salario_novo, data_vigencia, created_at
	`, membroID, tipo, cargoAnterior, cargoNovo, salarioAnterior, salarioNovo, dataVigencia).Scan(
		&h.ID, &h.MembroID, &h.Tipo, &h.CargoAnterior, &h.CargoNovo,
		&h.SalarioAnterior, &h.SalarioNovo, &h.DataVigencia, &h.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting historico salario: %w", err)
	}

	// Update membro salary + cargo + ultimo_aumento
	updateCargo := ""
	args := []interface{}{membroID, salarioNovo, dataVigencia}
	if tipo == "promocao" && cargoNovo != nil {
		updateCargo = ", cargo = $4"
		args = append(args, *cargoNovo)
	}
	_, err = tx.Exec(ctx, `
		UPDATE membros SET salario = $2, ultimo_aumento = $3`+updateCargo+`, updated_at = NOW() WHERE id = $1
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("updating membro salary: %w", err)
	}

	// Insert into membro_salarios for investimentos gastos-mensais tracking
	_, err = tx.Exec(ctx, `
		INSERT INTO membro_salarios (membro_id, valor, data_vigencia) VALUES ($1, $2, $3)
	`, membroID, salarioNovo, dataVigencia)
	if err != nil {
		return nil, fmt.Errorf("inserting membro_salarios: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &h, nil
}
```

- [ ] **Step 15: Update EquipeStore interface**

In `backend/internal/handler/equipe.go`, add to the `EquipeStore` interface (after `SetMembroProdutos`):

```go
	GetEquipeAtivaMembro(ctx context.Context, membroID uuid.UUID) (*domain.Equipe, error)
	TransferirMembro(ctx context.Context, equipeOrigemID, equipeDestinoID, membroID uuid.UUID) error
	InsertMeritoPromocao(ctx context.Context, membroID uuid.UUID, tipo string, cargoAnterior, cargoNovo *string, salarioAnterior *float64, salarioNovo float64, dataVigencia time.Time) (*domain.HistoricoMeritoPromocao, error)
	GetMembrosEquipeComEntrada(ctx context.Context, equipeID uuid.UUID) ([]domain.MembroComEntrada, error)
```

- [ ] **Step 16: Build and verify**

```bash
go build ./...
go test ./internal/domain/ -v
```
Expected: compiles cleanly, domain tests pass.

- [ ] **Step 17: Commit**

```bash
git add backend/migrations/000032_transferencia_temporal.up.sql \
       backend/migrations/000032_transferencia_temporal.down.sql \
       backend/internal/domain/cargo.go \
       backend/internal/domain/cargo_test.go \
       backend/internal/domain/merito.go \
       backend/internal/repository/equipe.go \
       backend/internal/handler/equipe.go
git commit -m "feat: add temporal equipe_membros columns, cargo hierarchy, transfer + mérito/promoção repo operations"
```

---

### Task 2: Temporal Query Updates Across All Repositories

**Files:**
- Modify: `backend/internal/repository/sprint.go:452-473` — GetMembrosEquipeIDs
- Modify: `backend/internal/repository/sprint.go:496-526` — GetUnplannedStats
- Modify: `backend/internal/repository/sprint.go:55` — sprint equipe join
- Modify: `backend/internal/repository/sprint.go:149,157,165` — nested sprint queries
- Modify: `backend/internal/repository/sprint.go:519,585,681,771,796,865,914` — all remaining sprint equipe joins
- Modify: `backend/internal/repository/review.go:70,188` — review equipe joins
- Modify: `backend/internal/repository/tarefa.go:68,106` — task filtering
- Modify: `backend/internal/repository/sync.go:489,546` — sync queries
- Modify: `backend/internal/repository/allocation.go:168` — allocation query
- Modify: `backend/internal/repository/timeline.go:37,49,86,108,202,281,304` — timeline queries
- Modify: `backend/internal/repository/investimento.go:89` — GetMembrosEquipeNoMes

**Interfaces:**
- Consumes: Task 1's migration (data_entrada, data_saida columns exist)
- Produces: All equipe_membros queries filter by `data_saida IS NULL` for current-member lookups

- [ ] **Step 1: Update all equipe_membros JOINs in sprint.go**

For every `INNER JOIN equipe_membros em ON em.membro_id = ...` in `sprint.go`, add `AND em.data_saida IS NULL` to the JOIN condition.

Affected lines (search for `equipe_membros` in sprint.go and add the filter):
- Line ~55: `INNER JOIN equipe_membros em ON em.membro_id = t.responsavel_id` → add `AND em.data_saida IS NULL`
- Line ~149: same pattern
- Line ~157: same pattern
- Line ~165: same pattern
- Line ~454 (GetMembrosEquipeIDs): `SELECT em.membro_id FROM equipe_membros em` — add `AND em.data_saida IS NULL` to WHERE clause
- Line ~519: same JOIN pattern
- Line ~585: same JOIN pattern
- Line ~681: same JOIN pattern
- Line ~771: same JOIN pattern
- Line ~796: same JOIN pattern
- Line ~865: subquery `SELECT membro_id FROM equipe_membros WHERE equipe_id = $2` → add `AND data_saida IS NULL`
- Line ~914: same subquery pattern

For `GetMembrosEquipeIDs` (line ~452), change the query to:
```sql
SELECT em.membro_id FROM equipe_membros em
JOIN membros m ON m.id = em.membro_id
WHERE em.equipe_id = $1
  AND em.data_saida IS NULL
  AND (m.data_desligamento IS NULL OR m.data_desligamento > $2)
```

- [ ] **Step 2: Update equipe_membros JOINs in review.go**

Line ~70: `equipeJoin = "INNER JOIN equipe_membros em ON em.membro_id = t.responsavel_id"` → append `AND em.data_saida IS NULL`

Line ~188: `JOIN equipe_membros em ON em.membro_id = m.id` → append `AND em.data_saida IS NULL`

- [ ] **Step 3: Update equipe_membros references in tarefa.go**

Line ~68: subquery `SELECT membro_id FROM equipe_membros WHERE equipe_id = $N` → add `AND data_saida IS NULL`

Line ~106: subquery `SELECT eq.nome FROM equipe_membros em JOIN equipes eq ON eq.id = em.equipe_id WHERE em.membro_id = t.responsavel_id LIMIT 1` → add `AND em.data_saida IS NULL`

- [ ] **Step 4: Update equipe_membros JOINs in sync.go**

Line ~489: `INNER JOIN equipe_membros em ON em.membro_id = t.responsavel_id` → add `AND em.data_saida IS NULL`

Line ~546: same pattern

- [ ] **Step 5: Update equipe_membros reference in allocation.go**

Line ~168: `FROM equipe_membros em` — add `AND em.data_saida IS NULL` to its WHERE clause

- [ ] **Step 6: Update equipe_membros references in timeline.go**

All references (lines ~37, 49, 86, 108, 202, 281, 304): add `AND em.data_saida IS NULL` or `AND data_saida IS NULL` to JOINs and subqueries.

- [ ] **Step 7: Update GetMembrosEquipeNoMes in investimento.go**

Line ~89: `INNER JOIN equipe_membros em ON em.membro_id = m.id AND em.equipe_id = $1` → add `AND em.data_saida IS NULL`

Note: This query already filters by data_admissao/data_desligamento. For now we use data_saida IS NULL (current members). When period-based temporal queries are needed for historical reports, this can be enhanced to use `em.data_entrada <= $lastDay AND (em.data_saida IS NULL OR em.data_saida >= $firstDay)`.

- [ ] **Step 8: Build and run all tests**

```bash
go build ./...
go test ./... -count=1
```
Expected: compiles cleanly, all tests pass. No behavioral changes — all existing members have data_saida IS NULL.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/repository/
git commit -m "feat: add temporal filtering (data_saida IS NULL) to all equipe_membros queries"
```

---

### Task 3: Transfer + Conflict Detection Handlers

**Files:**
- Modify: `backend/internal/handler/equipe.go` — add TransferMembro handler, modify AddMembro handler for 409 conflict
- Modify: `backend/cmd/api/main.go:252-253` — register transfer route
- Test: `backend/internal/handler/equipe_transfer_test.go`

**Interfaces:**
- Consumes: `EquipeStore.GetEquipeAtivaMembro`, `EquipeStore.TransferirMembro`, `EquipeStore.GetEquipeByID` from Task 1
- Produces:
  - `POST /equipes/{id}/membros/{membroId}/transferir` handler — body `{"equipe_destino_id": "uuid"}`, returns 200 with equipe names or 404
  - Modified `POST /equipes/{id}/membros` handler — returns 409 with `{"conflito": true, "equipe_atual": {"id": "...", "nome": "..."}}` when membro active in another team

- [ ] **Step 1: Add TransferMembro handler to equipe.go**

Add after RemoveMembro function:

```go
func (h *EquipeHandler) TransferMembro(w http.ResponseWriter, r *http.Request) {
	equipeOrigemID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}
	membroID, err := uuid.Parse(chi.URLParam(r, "membroId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "membro_id inválido")
		return
	}

	var req struct {
		EquipeDestinoID string `json:"equipe_destino_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	equipeDestinoID, err := uuid.Parse(req.EquipeDestinoID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "equipe_destino_id inválido")
		return
	}

	origem, err := h.store.GetEquipeByID(r.Context(), equipeOrigemID)
	if err != nil || origem == nil {
		respondError(w, http.StatusNotFound, "equipe origem não encontrada")
		return
	}
	destino, err := h.store.GetEquipeByID(r.Context(), equipeDestinoID)
	if err != nil || destino == nil {
		respondError(w, http.StatusNotFound, "equipe destino não encontrada")
		return
	}

	if err := h.store.TransferirMembro(r.Context(), equipeOrigemID, equipeDestinoID, membroID); err != nil {
		h.logger.Error("failed to transfer membro", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao transferir membro")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"equipe_origem":  origem.Nome,
		"equipe_destino": destino.Nome,
	})
}
```

- [ ] **Step 2: Modify AddMembro handler for 409 conflict**

Replace the existing AddMembro function body with:

```go
func (h *EquipeHandler) AddMembro(w http.ResponseWriter, r *http.Request) {
	equipeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		MembroID string `json:"membro_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	membroID, err := uuid.Parse(req.MembroID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "membro_id inválido")
		return
	}

	equipeAtiva, err := h.store.GetEquipeAtivaMembro(r.Context(), membroID)
	if err != nil {
		h.logger.Error("failed to check active equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao verificar equipe ativa")
		return
	}

	if equipeAtiva != nil {
		if equipeAtiva.ID == equipeID {
			respondJSON(w, http.StatusOK, map[string]string{"message": "membro já está nesta equipe"})
			return
		}
		respondJSON(w, http.StatusConflict, domain.TransferConflict{
			Conflito: true,
			EquipeAtual: struct {
				ID   uuid.UUID `json:"id"`
				Nome string    `json:"nome"`
			}{ID: equipeAtiva.ID, Nome: equipeAtiva.Nome},
		})
		return
	}

	if err := h.store.AddMembroEquipe(r.Context(), equipeID, membroID); err != nil {
		h.logger.Error("failed to add membro to equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao adicionar membro")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "membro adicionado"})
}
```

- [ ] **Step 3: Register transfer route in main.go**

In `backend/cmd/api/main.go`, after line 253 (`r.Delete("/equipes/{id}/membros/{membroId}"...)`), add:

```go
			r.Post("/equipes/{id}/membros/{membroId}/transferir", equipeHandler.TransferMembro)
```

- [ ] **Step 4: Build and test**

```bash
go build ./...
go test ./... -count=1
```
Expected: compiles cleanly, all tests pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/equipe.go backend/cmd/api/main.go
git commit -m "feat: add transfer endpoint and 409 conflict detection on AddMembro"
```

---

### Task 4: Mérito/Promoção Handler

**Files:**
- Modify: `backend/internal/handler/equipe.go` — add MeritoPromocao handler
- Modify: `backend/cmd/api/main.go` — register route
- Modify: `backend/internal/repository/membro.go` — add GetByID if needed (already exists at line 46)

**Interfaces:**
- Consumes: `EquipeStore.InsertMeritoPromocao` from Task 1, `domain.IsPromocaoValida` from Task 1, `MembroRepository.GetByID` (existing)
- Produces: `POST /membros/{id}/merito-promocao` handler — validates salary >= current, cargo hierarchy, returns before/after snapshot

- [ ] **Step 1: Add MeritoPromocao handler**

The handler needs access to MembroRepository for GetByID. Looking at the existing pattern, `EquipeHandler` only has `EquipeStore`. Add a `membroStore` field or add GetByID to EquipeStore interface.

Add to EquipeStore interface in equipe.go:

```go
	GetMembroByID(ctx context.Context, id uuid.UUID) (*domain.Membro, error)
```

Add to EquipeRepository a delegating method (in equipe.go, at the bottom — or better: since the handler needs it and EquipeRepository doesn't have membros access, we need the handler to accept a second store. Looking at how investHandler works — it has its own repo. The simpler approach: add the method to EquipeStore and have main.go wire it up).

Actually, looking at the existing code more carefully: `EquipeHandler.store` is an `EquipeStore` interface, and `EquipeRepository` implements it. But `GetByID` is on `MembroRepository`. The cleanest approach: add `GetMembroByID` to the `EquipeStore` interface and add a wrapper on `EquipeRepository` that delegates to a `MembroRepository`.

Simpler alternative: add a `membroRepo` field to `EquipeHandler`. Looking at `NewEquipeHandler(store EquipeStore, logger *zap.Logger)`, we can add a parameter.

Use the interface approach since it follows existing patterns:

Add to EquipeStore interface:
```go
	GetMembroByID(ctx context.Context, id uuid.UUID) (*domain.Membro, error)
```

Add to EquipeRepository:
```go
func (r *EquipeRepository) GetMembroByID(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
	var m domain.Membro
	err := r.pool.QueryRow(ctx, `
		SELECT id, fonte_dados_id, jira_account_id, nome, email,
		       avatar_url, team, ativo, data_desligamento, cargo,
		       salario, data_admissao, banco_horas,
		       created_at, updated_at
		FROM membros WHERE id = $1
	`, id).Scan(
		&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email,
		&m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo,
		&m.Salario, &m.DataAdmissao, &m.BancoHoras,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting membro by id: %w", err)
	}
	return &m, nil
}
```

- [ ] **Step 2: Add MeritoPromocao handler function**

Add to `backend/internal/handler/equipe.go`:

```go
func (h *EquipeHandler) MeritoPromocao(w http.ResponseWriter, r *http.Request) {
	membroID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req domain.MeritoPromocaoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	if req.Tipo != "merito" && req.Tipo != "promocao" {
		respondError(w, http.StatusBadRequest, "tipo deve ser 'merito' ou 'promocao'")
		return
	}

	dataVigencia, err := time.Parse("2006-01-02", req.DataVigencia)
	if err != nil {
		respondError(w, http.StatusBadRequest, "data_vigencia inválida (use YYYY-MM-DD)")
		return
	}

	membro, err := h.store.GetMembroByID(r.Context(), membroID)
	if err != nil {
		h.logger.Error("failed to get membro", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar membro")
		return
	}
	if membro == nil {
		respondError(w, http.StatusNotFound, "membro não encontrado")
		return
	}

	if membro.Salario != nil && req.SalarioNovo < *membro.Salario {
		respondError(w, http.StatusBadRequest, "salário novo não pode ser menor que o atual")
		return
	}

	var cargoNovo *string
	if req.Tipo == "promocao" {
		if req.CargoNovo == nil || *req.CargoNovo == "" {
			respondError(w, http.StatusBadRequest, "cargo_novo obrigatório para promoção")
			return
		}
		if !domain.IsCargoValido(*req.CargoNovo) {
			respondError(w, http.StatusBadRequest, "cargo_novo inválido")
			return
		}
		cargoAtual := ""
		if membro.Cargo != nil {
			cargoAtual = *membro.Cargo
		}
		if !domain.IsPromocaoValida(cargoAtual, *req.CargoNovo) {
			respondError(w, http.StatusBadRequest, "promoção inválida: "+cargoAtual+" não pode ser promovido para "+*req.CargoNovo)
			return
		}
		cargoNovo = req.CargoNovo
	} else {
		if req.CargoNovo != nil && *req.CargoNovo != "" && (membro.Cargo == nil || *req.CargoNovo != *membro.Cargo) {
			respondError(w, http.StatusBadRequest, "mérito não altera cargo")
			return
		}
	}

	historico, err := h.store.InsertMeritoPromocao(
		r.Context(), membroID, req.Tipo,
		membro.Cargo, cargoNovo,
		membro.Salario, req.SalarioNovo, dataVigencia,
	)
	if err != nil {
		h.logger.Error("failed to insert merito/promocao", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao registrar mérito/promoção")
		return
	}

	depois := domain.MembroSnapshot{Salario: &req.SalarioNovo}
	if req.Tipo == "promocao" && cargoNovo != nil {
		depois.Cargo = cargoNovo
	} else {
		depois.Cargo = membro.Cargo
	}

	respondJSON(w, http.StatusOK, domain.MeritoPromocaoResponse{
		HistoricoID: historico.ID,
		Antes:       domain.MembroSnapshot{Cargo: membro.Cargo, Salario: membro.Salario},
		Depois:      depois,
	})
}
```

- [ ] **Step 3: Register route in main.go**

Add after the cargo route or in the membro financial section (~line 260):

```go
			r.Post("/membros/{id}/merito-promocao", equipeHandler.MeritoPromocao)
```

- [ ] **Step 4: Build and test**

```bash
go build ./...
go test ./... -count=1
```

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/equipe.go backend/internal/repository/equipe.go backend/cmd/api/main.go
git commit -m "feat: add mérito/promoção endpoint with salary and cargo hierarchy validation"
```

---

### Task 5: Frontend — Transfer UI + Mérito/Promoção Modal

**Files:**
- Modify: `frontend/index.html:2429-2504` — member list buttons (transfer, mérito/promoção), auto-transfer on add, modal

**Interfaces:**
- Consumes:
  - `POST /equipes/{id}/membros/{membroId}/transferir` — body `{equipe_destino_id}`, returns `{equipe_origem, equipe_destino}`
  - `POST /equipes/{id}/membros` — may return 409 with `{conflito: true, equipe_atual: {id, nome}}`
  - `POST /membros/{id}/merito-promocao` — body `{tipo, cargo_novo, salario_novo, data_vigencia}`, returns `{historico_id, antes, depois}`
  - `GET /equipes` — list of teams for transfer dropdown
  - `GET /cargos` — existing cargo list endpoint
- Produces: Transfer button + dropdown, auto-transfer confirm dialog, mérito/promoção two-step modal

- [ ] **Step 1: Modify addMembroToEquipe for 409 conflict handling**

Replace the `addMembroToEquipe` function (around line 2414) with:

```javascript
async function addMembroToEquipe(equipeId, membroId) {
  try {
    var resp = await api('/equipes/' + equipeId + '/membros', {
      method: 'POST',
      body: JSON.stringify({ membro_id: membroId })
    });
    if (resp.conflito) {
      if (confirm(resp.equipe_atual.nome + ' → esta equipe?\nTransferir membro?')) {
        await api('/equipes/' + resp.equipe_atual.id + '/membros/' + membroId + '/transferir', {
          method: 'POST',
          body: JSON.stringify({ equipe_destino_id: equipeId })
        });
      } else { return; }
    }
    var el = document.querySelector('.equipe-search-result[data-id="' + membroId + '"]');
    if (el) {
      el.innerHTML += '<span class="sr-added">Adicionado!</span>';
      el.style.pointerEvents = 'none';
      el.style.opacity = '.6';
    }
    setTimeout(() => loadEquipeResumo(), 500);
  } catch (e) { alert('Erro: ' + e.message); }
}
```

Note: the `api()` function must handle 409 responses by returning the parsed JSON rather than throwing. Check how `api()` works — if it throws on non-2xx, we need to catch 409 specifically. Read the existing `api()` function first. If it throws, wrap the POST in a try/catch that checks for 409:

```javascript
async function addMembroToEquipe(equipeId, membroId) {
  try {
    var response = await fetch('/api/equipes/' + equipeId + '/membros', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...authHeaders() },
      body: JSON.stringify({ membro_id: membroId })
    });
    var data = await response.json();
    if (response.status === 409 && data.conflito) {
      if (confirm('Membro já está em "' + data.equipe_atual.nome + '". Transferir para esta equipe?')) {
        await api('/equipes/' + data.equipe_atual.id + '/membros/' + membroId + '/transferir', {
          method: 'POST',
          body: JSON.stringify({ equipe_destino_id: equipeId })
        });
      } else { return; }
    } else if (!response.ok) {
      throw new Error(data.error || 'Erro ao adicionar');
    }
    var el = document.querySelector('.equipe-search-result[data-id="' + membroId + '"]');
    if (el) {
      el.innerHTML += '<span class="sr-added">Adicionado!</span>';
      el.style.pointerEvents = 'none';
      el.style.opacity = '.6';
    }
    setTimeout(() => loadEquipeResumo(), 500);
  } catch (e) { alert('Erro: ' + e.message); }
}
```

Important: Read the existing `api()` helper and `authHeaders()` before implementing, to match how they handle auth tokens. The above uses raw fetch because `api()` likely throws on non-2xx and we need to intercept 409.

- [ ] **Step 2: Add transfer and mérito/promoção buttons to member list**

Modify the `membersHtml` generation in `renderEquipeResumo` (around line 2467-2473). Change the button area from just "Remover" to include "↗ Transferir" and "⭐ Mérito" buttons:

Replace the line that builds `membersHtml +=` (line ~2469-2473) with:

```javascript
    membersHtml += '<div class="member-row" onclick="openMembroDetail(\'' + m.id + '\')">' +
      '<div class="member-avatar" style="background:' + c + '">' + initials(m.nome) + '</div>' +
      '<div class="member-info"><div class="member-name">' + esc(m.nome) + (m.cargo ? '<span class="cargo-badge ' + cargoBadgeColor(m.cargo) + '">' + cargoLabelMap(m.cargo) + '</span>' : '') + '</div><div class="member-email">' + esc(m.email) + '</div></div>' +
      '<div class="member-metric-wrap"><div class="member-bar-wrap"><div class="member-bar"><div class="member-bar-fill ' + barClass(pct) + '" style="width:' + pctBar(pct) + '%"></div></div></div><div class="member-pct">' + pct.toFixed(1) + '%</div>' +
      '<button class="btn-member-action" onclick="event.stopPropagation();openTransferDropdown(\'' + equipeId + '\',\'' + m.id + '\',this)" title="Transferir">↗</button>' +
      '<button class="btn-member-action btn-merito" onclick="event.stopPropagation();openMeritoModal(\'' + m.id + '\',\'' + esc(m.nome) + '\',\'' + (m.cargo||'') + '\',' + (m.salario!=null?m.salario:'null') + ')" title="Mérito/Promoção">⭐</button>' +
      '<button class="btn-remove-member" onclick="event.stopPropagation();removeMembroFromEquipe(\'' + equipeId + '\',\'' + m.id + '\')">Remover</button></div></div>';
```

- [ ] **Step 3: Add CSS for new buttons**

Add to the `<style>` section (find existing `.btn-remove-member` styles and add nearby):

```css
.btn-member-action {
  background: none;
  border: 1px solid var(--border-subtle);
  color: var(--text-secondary);
  cursor: pointer;
  padding: 2px 8px;
  border-radius: 6px;
  font-size: 14px;
  margin-right: 4px;
  transition: all 0.15s;
}
.btn-member-action:hover {
  background: var(--accent-soft);
  color: var(--accent);
  border-color: var(--accent);
}
.btn-merito:hover {
  background: rgba(245,158,11,0.1);
  color: var(--amber, #f59e0b);
  border-color: var(--amber, #f59e0b);
}
.transfer-dropdown {
  position: absolute;
  right: 0;
  top: 100%;
  background: var(--surface);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  box-shadow: 0 4px 12px rgba(0,0,0,0.15);
  z-index: 50;
  min-width: 200px;
  max-height: 250px;
  overflow-y: auto;
  padding: 4px;
}
.transfer-dropdown-item {
  padding: 8px 12px;
  cursor: pointer;
  border-radius: 6px;
  font-size: 13px;
}
.transfer-dropdown-item:hover {
  background: var(--accent-soft);
}
```

- [ ] **Step 4: Add transfer dropdown function**

```javascript
var activeTransferDropdown = null;

async function openTransferDropdown(equipeId, membroId, btn) {
  if (activeTransferDropdown) { activeTransferDropdown.remove(); activeTransferDropdown = null; return; }
  try {
    var equipes = await api('/equipes');
    var dd = document.createElement('div');
    dd.className = 'transfer-dropdown';
    equipes.filter(function(e) { return e.id !== equipeId; }).forEach(function(e) {
      var item = document.createElement('div');
      item.className = 'transfer-dropdown-item';
      item.textContent = e.nome;
      item.onclick = async function(ev) {
        ev.stopPropagation();
        if (!confirm('Transferir para "' + e.nome + '"?')) return;
        try {
          await api('/equipes/' + equipeId + '/membros/' + membroId + '/transferir', {
            method: 'POST', body: JSON.stringify({ equipe_destino_id: e.id })
          });
          dd.remove(); activeTransferDropdown = null;
          loadEquipeResumo();
        } catch (err) { alert('Erro: ' + err.message); }
      };
      dd.appendChild(item);
    });
    btn.parentElement.style.position = 'relative';
    btn.parentElement.appendChild(dd);
    activeTransferDropdown = dd;
    setTimeout(function() {
      document.addEventListener('click', function closeDD() {
        if (activeTransferDropdown) { activeTransferDropdown.remove(); activeTransferDropdown = null; }
        document.removeEventListener('click', closeDD);
      }, { once: true });
    }, 0);
  } catch (e) { alert('Erro: ' + e.message); }
}
```

- [ ] **Step 5: Add Mérito/Promoção modal HTML**

Add the modal HTML to the page body (find existing modal markup and add nearby):

```html
<div class="modal-overlay" id="merito-modal" style="display:none">
  <div class="modal-card" style="max-width:480px">
    <div class="modal-header">
      <h3 id="merito-modal-title">Mérito/Promoção</h3>
      <button class="modal-close" onclick="closeMeritoModal()">✕</button>
    </div>
    <div id="merito-modal-body"></div>
  </div>
</div>
```

- [ ] **Step 6: Add Mérito/Promoção modal JS**

```javascript
var meritoState = {};

function openMeritoModal(membroId, nome, cargoAtual, salarioAtual) {
  meritoState = { membroId: membroId, nome: nome, cargo: cargoAtual, salario: salarioAtual };
  document.getElementById('merito-modal-title').textContent = 'Mérito/Promoção — ' + nome;
  renderMeritoForm();
  document.getElementById('merito-modal').style.display = 'flex';
}

function closeMeritoModal() {
  document.getElementById('merito-modal').style.display = 'none';
  meritoState = {};
}

function renderMeritoForm() {
  var s = meritoState;
  var promoOpts = getPromocaoOptions(s.cargo);
  var html = '<div style="padding:16px">';
  html += '<div style="display:flex;gap:8px;margin-bottom:16px">';
  html += '<button class="period-chip' + (s.tipo !== 'promocao' ? ' active' : '') + '" onclick="meritoState.tipo=\'merito\';renderMeritoForm()">Mérito</button>';
  html += '<button class="period-chip' + (s.tipo === 'promocao' ? ' active' : '') + '" onclick="meritoState.tipo=\'promocao\';renderMeritoForm()">Promoção</button>';
  html += '</div>';

  if (s.tipo === 'promocao') {
    html += '<label style="font-size:13px;color:var(--text-secondary);display:block;margin-bottom:4px">Novo Cargo</label>';
    html += '<select id="merito-cargo" style="width:100%;padding:8px;border:1px solid var(--border-subtle);border-radius:6px;background:var(--surface);color:var(--text-primary);margin-bottom:12px">';
    if (promoOpts.length === 0) {
      html += '<option value="">Cargo terminal — sem promoções disponíveis</option>';
    } else {
      html += '<option value="">Selecionar...</option>';
      promoOpts.forEach(function(o) { html += '<option value="' + o.value + '">' + o.label + '</option>'; });
    }
    html += '</select>';
  }

  html += '<label style="font-size:13px;color:var(--text-secondary);display:block;margin-bottom:4px">Data</label>';
  html += '<input type="date" id="merito-data" value="' + new Date().toISOString().slice(0,10) + '" style="width:100%;padding:8px;border:1px solid var(--border-subtle);border-radius:6px;background:var(--surface);color:var(--text-primary);margin-bottom:12px" />';

  html += '<label style="font-size:13px;color:var(--text-secondary);display:block;margin-bottom:4px">Salário Atual</label>';
  html += '<input type="text" readonly value="' + (s.salario != null ? formatSalarioBR(s.salario) : 'Não definido') + '" style="width:100%;padding:8px;border:1px solid var(--border-subtle);border-radius:6px;background:var(--bg-secondary, #1a1a2e);color:var(--text-secondary);margin-bottom:12px" />';

  html += '<label style="font-size:13px;color:var(--text-secondary);display:block;margin-bottom:4px">Novo Salário</label>';
  html += '<div style="display:flex;gap:8px;align-items:center;margin-bottom:16px">';
  html += '<input type="number" id="merito-salario" step="0.01" min="' + (s.salario || 0) + '" placeholder="0.00" style="flex:1;padding:8px;border:1px solid var(--border-subtle);border-radius:6px;background:var(--surface);color:var(--text-primary)" oninput="updateMeritoPct()" />';
  html += '<span id="merito-pct" style="font-weight:600;color:var(--accent);min-width:60px;text-align:right"></span>';
  html += '</div>';

  html += '<button class="btn-primary" onclick="submitMeritoForm()" style="width:100%">Confirmar</button>';
  html += '</div>';
  document.getElementById('merito-modal-body').innerHTML = html;
}

function getPromocaoOptions(cargoAtual) {
  var hierarchy = {
    'analista_i': ['analista_ii'],
    'analista_ii': ['analista_iii'],
    'analista_iii': ['especialista_i', 'coordenador_desenvolvimento'],
    'especialista_i': ['especialista_ii', 'coordenador_desenvolvimento', 'lider_tecnico'],
    'especialista_ii': ['master', 'lider_tecnico'],
    'coordenador_desenvolvimento': ['lider_tecnico']
  };
  var labels = {
    'analista_i': 'Analista I', 'analista_ii': 'Analista II', 'analista_iii': 'Analista III',
    'especialista_i': 'Especialista I', 'especialista_ii': 'Especialista II', 'master': 'Master',
    'coordenador_desenvolvimento': 'Coord. Dev', 'lider_tecnico': 'Líder Técnico'
  };
  var validos = hierarchy[cargoAtual] || [];
  return validos.map(function(v) { return { value: v, label: labels[v] || v }; });
}

function updateMeritoPct() {
  var el = document.getElementById('merito-pct');
  var novoVal = parseFloat(document.getElementById('merito-salario').value);
  if (!novoVal || !meritoState.salario || meritoState.salario === 0) { el.textContent = ''; return; }
  var pct = ((novoVal - meritoState.salario) / meritoState.salario * 100);
  el.textContent = (pct >= 0 ? '+' : '') + pct.toFixed(1) + '%';
  el.style.color = pct >= 0 ? 'var(--accent)' : 'var(--red)';
}

async function submitMeritoForm() {
  var s = meritoState;
  var tipo = s.tipo || 'merito';
  var salarioNovo = parseFloat(document.getElementById('merito-salario').value);
  var dataVigencia = document.getElementById('merito-data').value;
  if (!salarioNovo || salarioNovo <= 0) { alert('Informe o novo salário'); return; }
  if (s.salario != null && salarioNovo < s.salario) { alert('Salário não pode ser menor que o atual'); return; }
  if (!dataVigencia) { alert('Informe a data'); return; }

  var body = { tipo: tipo, salario_novo: salarioNovo, data_vigencia: dataVigencia };
  if (tipo === 'promocao') {
    var cargoNovo = document.getElementById('merito-cargo').value;
    if (!cargoNovo) { alert('Selecione o novo cargo'); return; }
    body.cargo_novo = cargoNovo;
  }

  try {
    var resp = await api('/membros/' + s.membroId + '/merito-promocao', {
      method: 'POST', body: JSON.stringify(body)
    });
    renderMeritoConfirmacao(resp);
  } catch (e) { alert('Erro: ' + e.message); }
}

function renderMeritoConfirmacao(resp) {
  var labels = {
    'analista_i': 'Analista I', 'analista_ii': 'Analista II', 'analista_iii': 'Analista III',
    'especialista_i': 'Especialista I', 'especialista_ii': 'Especialista II', 'master': 'Master',
    'coordenador_desenvolvimento': 'Coord. Dev', 'lider_tecnico': 'Líder Técnico'
  };
  var a = resp.antes, d = resp.depois;
  var html = '<div style="padding:16px">';
  html += '<div style="text-align:center;margin-bottom:16px;color:var(--accent);font-weight:600">✓ Registrado com sucesso</div>';
  html += '<table style="width:100%;border-collapse:collapse;font-size:14px">';
  html += '<tr style="border-bottom:1px solid var(--border-subtle)"><th style="text-align:left;padding:8px">Campo</th><th style="padding:8px">Antes</th><th style="padding:8px">Depois</th></tr>';

  if (a.cargo !== d.cargo) {
    html += '<tr><td style="padding:8px">Cargo</td><td style="padding:8px;text-align:center">' + (labels[a.cargo] || a.cargo || '—') + '</td><td style="padding:8px;text-align:center;font-weight:600;color:var(--accent)">' + (labels[d.cargo] || d.cargo || '—') + '</td></tr>';
  }

  var salAntes = a.salario != null ? formatSalarioBR(a.salario) : '—';
  var salDepois = d.salario != null ? formatSalarioBR(d.salario) : '—';
  var pctAumento = (a.salario && d.salario) ? ((d.salario - a.salario) / a.salario * 100).toFixed(1) : '—';
  html += '<tr><td style="padding:8px">Salário</td><td style="padding:8px;text-align:center">' + salAntes + '</td><td style="padding:8px;text-align:center;font-weight:600;color:var(--accent)">' + salDepois + '</td></tr>';
  html += '<tr><td style="padding:8px">Aumento</td><td style="padding:8px;text-align:center">—</td><td style="padding:8px;text-align:center;font-weight:600;color:var(--accent)">+' + pctAumento + '%</td></tr>';

  html += '</table>';
  html += '<button class="btn-primary" onclick="closeMeritoModal();loadEquipeResumo()" style="width:100%;margin-top:16px">Fechar</button>';
  html += '</div>';
  document.getElementById('merito-modal-body').innerHTML = html;
}
```

- [ ] **Step 7: Test in browser**

1. Open the app, navigate to an equipe page
2. Verify member list shows ↗ (transfer) and ⭐ (mérito) buttons
3. Test transfer: click ↗ → dropdown with other equipes → select one → confirm → member moves
4. Test auto-transfer: search for a member already in another team → add → 409 dialog → confirm → transfers
5. Test mérito: click ⭐ → toggle Mérito → set salary → see % → confirm → see before/after
6. Test promoção: click ⭐ → toggle Promoção → select valid cargo → set salary → confirm → see before/after with cargo change
7. Verify invalid promoção: try skipping levels (Analista I → Especialista I) → error

- [ ] **Step 8: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add transfer button, auto-transfer on conflict, and mérito/promoção modal"
```
