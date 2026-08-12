# Padronizar Cargos de Membros — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace current cargo values with organization's real job title hierarchy and migrate existing data.

**Architecture:** Update domain constants, SQL migration, frontend labels/dropdown. Three-file change.

**Tech Stack:** Go (domain constants), SQL (migration), vanilla JS/HTML (frontend)

## Global Constraints

- Do not touch `handler/usuario.go` `cargosValidos` — those are access roles, not job titles
- `coordenador_desenvolvimento` keeps the same slug (already exists in DB)
- Migration must be idempotent (use WHERE clauses)
- Single monolithic frontend file: `frontend/index.html`
- Follow existing patterns in all files
- Migration number is `000029` (last existing migration is `000028_investimentos_campos`)
- The `membros.cargo` column has a `CHECK` constraint added in `000014_cargos_membros.up.sql` restricting values to the old list — the constraint must be dropped and recreated with the new list, or inserting `analista_iii` etc. will fail
- No other Go files reference the `domain.Cargo*` constants outside `domain/cargo.go` itself (verified via grep) — no call-site changes needed in handler/service/repository code
- No existing `_test.go` files reference the literal old cargo values (`desenvolvedor`, `po_produto`, `scrum_master`, `agile_master`, `gerente_tecnologia`, `gerente_executivo`) — Task 3 verifies this and is a no-op if the grep stays empty

---

### Task 1: Backend — update `domain/cargo.go` + migration files

**Files:**
- Modify: `backend/internal/domain/cargo.go`
- Create: `backend/migrations/000029_padronizar_cargos.up.sql`
- Create: `backend/migrations/000029_padronizar_cargos.down.sql`

**Interfaces:**
- Consumes: nothing
- Produces: `domain.CargosValidos`, `domain.CargoLabels`, `domain.IsCargoValido` (consumed by `handler/equipe.go` `UpdateCargo` and `ListCargos`, already wired — no call-site changes needed) and updated DB schema/data

- [ ] **Step 1: Rewrite `backend/internal/domain/cargo.go`**

```go
package domain

import "slices"

const (
	CargoAnalistaI                  = "analista_i"
	CargoAnalistaII                 = "analista_ii"
	CargoAnalistaIII                = "analista_iii"
	CargoEspecialistaI              = "especialista_i"
	CargoEspecialistaII             = "especialista_ii"
	CargoMaster                     = "master"
	CargoCoordenadorDesenvolvimento = "coordenador_desenvolvimento"
	CargoLiderTecnico               = "lider_tecnico"
)

var CargosValidos = []string{
	CargoAnalistaI,
	CargoAnalistaII,
	CargoAnalistaIII,
	CargoEspecialistaI,
	CargoEspecialistaII,
	CargoMaster,
	CargoCoordenadorDesenvolvimento,
	CargoLiderTecnico,
}

var CargoLabels = map[string]string{
	CargoAnalistaI:                  "Analista I",
	CargoAnalistaII:                 "Analista II",
	CargoAnalistaIII:                "Analista III",
	CargoEspecialistaI:              "Especialista I",
	CargoEspecialistaII:             "Especialista II",
	CargoMaster:                     "Master",
	CargoCoordenadorDesenvolvimento: "Coord. Dev",
	CargoLiderTecnico:               "Líder Técnico",
}

func IsCargoValido(cargo string) bool {
	return slices.Contains(CargosValidos, cargo)
}
```

- [ ] **Step 2: Write the up migration — `backend/migrations/000029_padronizar_cargos.up.sql`**

```sql
-- Drop old CHECK constraint (auto-generated name from ALTER TABLE ADD COLUMN in 000014)
ALTER TABLE membros DROP CONSTRAINT IF EXISTS membros_cargo_check;

-- Migrate desenvolvedor -> analista_iii
UPDATE membros SET cargo = 'analista_iii' WHERE cargo = 'desenvolvedor';

-- Cargos removidos -> NULL (reassignar manualmente via dropdown)
UPDATE membros SET cargo = NULL WHERE cargo IN (
  'po_produto', 'gerente_tecnologia', 'gerente_executivo',
  'scrum_master', 'agile_master'
);

-- Recreate CHECK constraint with new allowed values
ALTER TABLE membros ADD CONSTRAINT membros_cargo_check
  CHECK (cargo IN (
    'analista_i',
    'analista_ii',
    'analista_iii',
    'especialista_i',
    'especialista_ii',
    'master',
    'coordenador_desenvolvimento',
    'lider_tecnico'
  ));
```

- [ ] **Step 3: Write the down migration — `backend/migrations/000029_padronizar_cargos.down.sql`**

```sql
-- Data is not reverted (desenvolvedor->analista_iii and NULLed cargos cannot be
-- reconstructed with precision). Only the CHECK constraint is restored to the
-- pre-000029 list so old code/data remain structurally consistent if rolled back.
ALTER TABLE membros DROP CONSTRAINT IF EXISTS membros_cargo_check;

ALTER TABLE membros ADD CONSTRAINT membros_cargo_check
  CHECK (cargo IN (
    'coordenador_desenvolvimento',
    'po_produto',
    'gerente_tecnologia',
    'gerente_executivo',
    'scrum_master',
    'agile_master',
    'desenvolvedor'
  ));
```

- [ ] **Step 4: Verify compilation**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`

Expected: builds successfully with no errors.

- [ ] **Step 5: Apply the migration**

Run: `cd /home/emerson/code/myplanner/backend && go run cmd/migrate/main.go up`

If no `migrate` CLI wrapper is wired to a specific DSN flag, apply manually:
```bash
psql "$DATABASE_URL" -f migrations/000029_padronizar_cargos.up.sql
```

- [ ] **Step 6: Verify schema and data**

```bash
psql "$DATABASE_URL" -c "\d membros" | grep -A1 "cargo"
psql "$DATABASE_URL" -c "SELECT DISTINCT cargo FROM membros;"
```

Expected: the `Check constraints` section shows `membros_cargo_check` with the new 8 values; `SELECT DISTINCT cargo` only returns values from the new list or `NULL` — no `desenvolvedor`, `po_produto`, `gerente_tecnologia`, `gerente_executivo`, `scrum_master`, or `agile_master`.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/domain/cargo.go backend/migrations/000029_padronizar_cargos.up.sql backend/migrations/000029_padronizar_cargos.down.sql
git commit -m "feat: standardize member job titles (cargo) to organization hierarchy"
```

---

### Task 2: Frontend — update `cargoLabelMap`, `cargoBadgeColor`, dropdown items

**Files:**
- Modify: `frontend/index.html` (`cargoLabelMap` ~line 2375, `cargoBadgeColor` ~line 2388, `renderMembroCargo`/`selectCargo` `po_produto` special-case ~line 6383 and ~6414)

**Interfaces:**
- Consumes: `domain.CargosValidos`/`domain.CargoLabels` from Task 1 via `GET /cargos` (`allCargos` in `renderMembroCargo`, already wired — no fetch/wiring changes needed)
- Produces: updated badge/dropdown rendering, consumed by `renderEquipeResumo` and `renderMembroCargo`

- [ ] **Step 1: Replace `cargoLabelMap` in `frontend/index.html` (currently lines 2375-2386)**

```javascript
function cargoLabelMap(cargo) {
  var labels = {
    'analista_i': 'Analista I',
    'analista_ii': 'Analista II',
    'analista_iii': 'Analista III',
    'especialista_i': 'Especialista I',
    'especialista_ii': 'Especialista II',
    'master': 'Master',
    'coordenador_desenvolvimento': 'Coord. Dev',
    'lider_tecnico': 'Líder Técnico'
  };
  return labels[cargo] || '';
}
```

- [ ] **Step 2: Replace `cargoBadgeColor` in `frontend/index.html` (currently lines 2388-2394)**

```javascript
function cargoBadgeColor(cargo) {
  if (!cargo) return '';
  if (cargo === 'analista_i' || cargo === 'analista_ii') return 'gray';
  if (cargo === 'analista_iii' || cargo === 'especialista_i') return 'blue';
  if (cargo === 'especialista_ii' || cargo === 'master') return 'purple';
  if (cargo === 'coordenador_desenvolvimento' || cargo === 'lider_tecnico') return 'green';
  return '';
}
```

- [ ] **Step 3: Remove dead `po_produto` special-case in `renderMembroCargo` (currently lines 6383-6385)**

`po_produto` no longer exists in `CargosValidos`, so this branch can never trigger again. Remove it to avoid leaving a reference to a removed slug:

Before:
```javascript
  container.innerHTML = html;
  if (cargo === 'po_produto') {
    loadMembroProdutos(membroId);
  }
}
```

After:
```javascript
  container.innerHTML = html;
}
```

- [ ] **Step 4: Remove dead `po_produto` special-case in `selectCargo` (currently around line 6414)**

Before:
```javascript
    var prodSection = document.getElementById('produto-section-' + membroId);
    if (cargo === 'po_produto') {
      loadMembroProdutos(membroId);
    } else {
      prodSection.innerHTML = '';
    }
```

After:
```javascript
    var prodSection = document.getElementById('produto-section-' + membroId);
    if (prodSection) prodSection.innerHTML = '';
```

- [ ] **Step 5: Manual smoke test in browser**

```bash
cd /home/emerson/code/myplanner && python3 -m http.server 8000 --directory frontend &
```

Open the app, navigate to an equipe, open a member's detail panel, open the cargo dropdown. Expected: dropdown lists the 8 new labels (Analista I/II/III, Especialista I/II, Master, Coord. Dev, Líder Técnico) sourced from `GET /cargos`; selecting one updates the badge color per the new mapping (gray/blue/purple/green as specified); no console errors referencing `po_produto` or `loadMembroProdutos`.

- [ ] **Step 6: Commit**

```bash
git add frontend/index.html
git commit -m "feat: update cargo labels, badge colors, and dropdown for new job title hierarchy"
```

---

### Task 3: Tests — update any test files that reference old cargo values

**Files:**
- Verify: `backend/internal/handler/equipe_test.go`
- Verify: `backend/internal/service/investimento_test.go`
- Verify: any other `*_test.go` under `backend/internal/`

**Interfaces:**
- Consumes: `domain.CargosValidos` from Task 1
- Produces: green test suite confirming no test relies on removed cargo slugs

- [ ] **Step 1: Grep for old cargo slug literals across all Go test files**

```bash
cd /home/emerson/code/myplanner/backend
grep -rn "desenvolvedor\|po_produto\|scrum_master\|agile_master\|gerente_tecnologia\|gerente_executivo" --include="*_test.go" .
```

Expected: no output. This was verified during planning — `equipe_test.go` and `investimento_test.go` do not reference cargo at all today, and no other test file uses these literals. If this grep returns matches (e.g. a test added after this plan was written), replace each occurrence: `desenvolvedor` → `analista_iii`; any of `po_produto`/`gerente_tecnologia`/`gerente_executivo`/`scrum_master`/`agile_master` → remove the assertion or replace with a still-valid slug (e.g. `especialista_i`), per what the specific test is asserting.

- [ ] **Step 2: Grep for old `domain.Cargo*` constant names in Go source (non-test)**

```bash
cd /home/emerson/code/myplanner/backend
grep -rn "CargoDesenvolvedor\|CargoPOProduto\|CargoGerenteTecnologia\|CargoGerenteExecutivo\|CargoScrumMaster\|CargoAgileMaster" --include="*.go" .
```

Expected: no output (confirmed during planning — these constants are only defined and referenced within `domain/cargo.go` itself, no external call sites). If any match appears, update it to the corresponding new constant from Task 1 (e.g. `CargoDesenvolvedor` → `CargoAnalistaIII`).

- [ ] **Step 3: Run the full backend test suite**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/... -v`

Expected: `ok` for all packages, in particular `internal/handler` and `internal/service`; no failures related to cargo validation or labels.

- [ ] **Step 4: Run `go vet` and full build as a final sanity check**

```bash
cd /home/emerson/code/myplanner/backend
go vet ./...
go build ./...
```

Expected: both commands exit 0 with no output.

- [ ] **Step 5: Commit (only if Step 1 or 2 required changes)**

If the greps in Steps 1-2 were clean (no matches), there is nothing to commit for this task — skip this step.

If changes were made:
```bash
git add -u backend/internal
git commit -m "test: update cargo references to new job title slugs"
```

---
