# Cargos de Membros — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Associate a fixed-list role (cargo) to each member, with a special P.O.→products relationship, displayed as badges in team listings and managed from the member detail page.

**Architecture:** Add `cargo` column to `membros` table (CHECK constraint enum), `membro_produtos` N:N table for P.O.→products. New repo methods on `EquipeRepository`, new handler endpoints on `EquipeHandler`. Frontend: cargo dropdown + conditional products section in member detail, cargo badges in equipe member list.

**Tech Stack:** Go 1.21+, pgx/v5, chi router, zap logger, vanilla JS SPA

## Global Constraints

- Cargo values are exactly: `coordenador_desenvolvimento`, `po_produto`, `gerente_tecnologia`, `gerente_executivo`, `scrum_master`, `agile_master`, `desenvolvedor`
- Cargo is nullable (member can have no cargo)
- Cargo is global per member (same across all teams)
- One cargo per member
- P.O. (`po_produto`) requires 1+ products — enforced client-side only
- Products come from existing `produtos` table (populated via JIRA sync)
- Follow existing patterns: handler→repo (no service layer for equipes), interface `EquipeStore`, `respondJSON`/`respondError` helpers
- All SQL queries scan fields in the same order as SELECT columns
- Frontend is a single `index.html` file — vanilla JS, no framework

---

### Task 1: Database Migration + Domain Constants

**Files:**
- Create: `backend/migrations/000014_cargos_membros.up.sql`
- Create: `backend/migrations/000014_cargos_membros.down.sql`
- Modify: `backend/internal/domain/models.go:31-43` (Membro struct)
- Create: `backend/internal/domain/cargo.go`

**Interfaces:**
- Consumes: nothing
- Produces: `domain.Membro.Cargo *string`, `domain.CargosValidos []string`, `domain.CargoLabels map[string]string`, `domain.IsCargoValido(cargo string) bool`

- [ ] **Step 1: Create up migration**

Create file `backend/migrations/000014_cargos_membros.up.sql`:

```sql
ALTER TABLE membros ADD COLUMN cargo VARCHAR(50)
  CHECK (cargo IN (
    'coordenador_desenvolvimento',
    'po_produto',
    'gerente_tecnologia',
    'gerente_executivo',
    'scrum_master',
    'agile_master',
    'desenvolvedor'
  ));

CREATE TABLE membro_produtos (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  membro_id UUID NOT NULL REFERENCES membros(id) ON DELETE CASCADE,
  produto_id UUID NOT NULL REFERENCES produtos(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(membro_id, produto_id)
);

CREATE INDEX idx_membro_produtos_membro ON membro_produtos(membro_id);
CREATE INDEX idx_membro_produtos_produto ON membro_produtos(produto_id);
```

- [ ] **Step 2: Create down migration**

Create file `backend/migrations/000014_cargos_membros.down.sql`:

```sql
DROP TABLE IF EXISTS membro_produtos;
ALTER TABLE membros DROP COLUMN IF EXISTS cargo;
```

- [ ] **Step 3: Add Cargo field to Membro struct**

In `backend/internal/domain/models.go`, add `Cargo` field to the `Membro` struct after `DataDesligamento`:

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
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}
```

- [ ] **Step 4: Create cargo domain file**

Create file `backend/internal/domain/cargo.go`:

```go
package domain

const (
	CargoCoordenadorDesenvolvimento = "coordenador_desenvolvimento"
	CargoPOProduto                  = "po_produto"
	CargoGerenteTecnologia          = "gerente_tecnologia"
	CargoGerenteExecutivo           = "gerente_executivo"
	CargoScrumMaster                = "scrum_master"
	CargoAgileMaster                = "agile_master"
	CargoDesenvolvedor              = "desenvolvedor"
)

var CargosValidos = []string{
	CargoCoordenadorDesenvolvimento,
	CargoPOProduto,
	CargoGerenteTecnologia,
	CargoGerenteExecutivo,
	CargoScrumMaster,
	CargoAgileMaster,
	CargoDesenvolvedor,
}

var CargoLabels = map[string]string{
	CargoCoordenadorDesenvolvimento: "Coordenador de Desenvolvimento",
	CargoPOProduto:                  "P.O. Produto",
	CargoGerenteTecnologia:          "Gerente de Tecnologia",
	CargoGerenteExecutivo:           "Gerente Executivo",
	CargoScrumMaster:                "Scrum Master",
	CargoAgileMaster:                "Agile Master",
	CargoDesenvolvedor:              "Desenvolvedor",
}

func IsCargoValido(cargo string) bool {
	for _, c := range CargosValidos {
		if c == cargo {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Update all Membro SQL scans**

Every query that scans `domain.Membro` must now include `cargo`. There are 4 locations to update:

**`backend/internal/repository/membro.go` — `List` (line 25):**

Change SELECT to:
```sql
SELECT id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo, data_desligamento, cargo, created_at, updated_at
FROM membros
WHERE ativo = true
ORDER BY nome
```

Change Scan (line 38) to:
```go
if err := rows.Scan(&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email, &m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo, &m.CreatedAt, &m.UpdatedAt); err != nil {
```

**`backend/internal/repository/membro.go` — `GetByID` (line 49):**

Change SELECT to:
```sql
SELECT id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo, data_desligamento, cargo, created_at, updated_at
FROM membros WHERE id = $1
```

Change Scan (line 51) to:
```go
).Scan(&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email, &m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo, &m.CreatedAt, &m.UpdatedAt)
```

**`backend/internal/repository/membro.go` — `Search` (around line 191):**

Add `cargo` to SELECT and Scan in the same pattern.

**`backend/internal/repository/equipe.go` — `GetMembrosEquipe` (line 94):**

Change SELECT to:
```sql
SELECT m.id, m.fonte_dados_id, m.jira_account_id, m.nome, m.email,
       m.avatar_url, m.team, m.ativo, m.data_desligamento, m.cargo, m.created_at, m.updated_at
```

Change Scan (line 110-112) to:
```go
if err := rows.Scan(
    &m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email,
    &m.AvatarURL, &m.Team, &m.Ativo, &m.DataDesligamento, &m.Cargo, &m.CreatedAt, &m.UpdatedAt,
); err != nil {
```

- [ ] **Step 6: Add Cargo to MembroResumo**

In `backend/internal/domain/equipe.go`, add `Cargo` to `MembroResumo`:

```go
type MembroResumo struct {
	ID               uuid.UUID `json:"id"`
	Nome             string    `json:"nome"`
	Email            *string   `json:"email"`
	AvatarURL        *string   `json:"avatar_url"`
	Cargo            *string   `json:"cargo"`
	AtuacaoRastreada float64   `json:"atuacao_rastreada"`
}
```

In `backend/internal/handler/equipe.go`, update `CalcularResumoEquipe` (line 104) to populate `Cargo`:

```go
membrosResumo[i] = domain.MembroResumo{
    ID:               m.ID,
    Nome:             m.Nome,
    Email:            m.Email,
    AvatarURL:        m.AvatarURL,
    Cargo:            m.Cargo,
    AtuacaoRastreada: pctAtuacao,
}
```

- [ ] **Step 7: Verify build**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 8: Run migration locally**

Run the migration against local database to verify SQL is valid.

- [ ] **Step 9: Commit**

```bash
git add backend/migrations/000014_cargos_membros.up.sql backend/migrations/000014_cargos_membros.down.sql backend/internal/domain/cargo.go backend/internal/domain/models.go backend/internal/domain/equipe.go backend/internal/repository/membro.go backend/internal/repository/equipe.go backend/internal/handler/equipe.go
git commit -m "feat: add cargo field to membros with domain constants and migration"
```

---

### Task 2: Repository Methods (Cargo + Produtos)

**Files:**
- Modify: `backend/internal/repository/equipe.go`

**Interfaces:**
- Consumes: `domain.Membro.Cargo`, `domain.Produto` (existing struct)
- Produces:
  - `(*EquipeRepository) UpdateMembroCargo(ctx context.Context, membroID uuid.UUID, cargo *string) error`
  - `(*EquipeRepository) ListProdutos(ctx context.Context) ([]domain.Produto, error)`
  - `(*EquipeRepository) GetMembroProdutos(ctx context.Context, membroID uuid.UUID) ([]domain.Produto, error)`
  - `(*EquipeRepository) SetMembroProdutos(ctx context.Context, membroID uuid.UUID, produtoIDs []uuid.UUID) error`

- [ ] **Step 1: Add UpdateMembroCargo**

Add to `backend/internal/repository/equipe.go`:

```go
func (r *EquipeRepository) UpdateMembroCargo(ctx context.Context, membroID uuid.UUID, cargo *string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE membros SET cargo = $2, updated_at = NOW() WHERE id = $1
	`, membroID, cargo)
	if err != nil {
		return fmt.Errorf("updating membro cargo: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not found", membroID)
	}
	return nil
}
```

- [ ] **Step 2: Add ListProdutos**

```go
func (r *EquipeRepository) ListProdutos(ctx context.Context) ([]domain.Produto, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, jira_id, nome, descricao, projeto_id, ativo, created_at, updated_at
		FROM produtos
		WHERE ativo = true
		ORDER BY nome
	`)
	if err != nil {
		return nil, fmt.Errorf("listing produtos: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Produto, 0)
	for rows.Next() {
		var p domain.Produto
		if err := rows.Scan(&p.ID, &p.FonteDadosID, &p.JiraID, &p.Nome, &p.Descricao, &p.ProjetoID, &p.Ativo, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning produto: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
```

- [ ] **Step 3: Add GetMembroProdutos**

```go
func (r *EquipeRepository) GetMembroProdutos(ctx context.Context, membroID uuid.UUID) ([]domain.Produto, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.fonte_dados_id, p.jira_id, p.nome, p.descricao, p.projeto_id, p.ativo, p.created_at, p.updated_at
		FROM produtos p
		INNER JOIN membro_produtos mp ON mp.produto_id = p.id
		WHERE mp.membro_id = $1
		ORDER BY p.nome
	`, membroID)
	if err != nil {
		return nil, fmt.Errorf("getting membro produtos: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Produto, 0)
	for rows.Next() {
		var p domain.Produto
		if err := rows.Scan(&p.ID, &p.FonteDadosID, &p.JiraID, &p.Nome, &p.Descricao, &p.ProjetoID, &p.Ativo, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning membro produto: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
```

- [ ] **Step 4: Add SetMembroProdutos**

```go
func (r *EquipeRepository) SetMembroProdutos(ctx context.Context, membroID uuid.UUID, produtoIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM membro_produtos WHERE membro_id = $1`, membroID); err != nil {
		return fmt.Errorf("clearing membro produtos: %w", err)
	}

	for _, pid := range produtoIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO membro_produtos (membro_id, produto_id) VALUES ($1, $2)
		`, membroID, pid); err != nil {
			return fmt.Errorf("inserting membro produto: %w", err)
		}
	}

	return tx.Commit(ctx)
}
```

- [ ] **Step 5: Verify build**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/repository/equipe.go
git commit -m "feat: add cargo and produto repository methods"
```

---

### Task 3: Handler Endpoints + Routes

**Files:**
- Modify: `backend/internal/handler/equipe.go:15-26` (EquipeStore interface)
- Modify: `backend/internal/handler/equipe.go` (add handler methods)
- Modify: `backend/cmd/api/main.go:188-195` (routes)

**Interfaces:**
- Consumes: `EquipeRepository.UpdateMembroCargo`, `EquipeRepository.ListProdutos`, `EquipeRepository.GetMembroProdutos`, `EquipeRepository.SetMembroProdutos`, `domain.IsCargoValido`, `domain.CargosValidos`, `domain.CargoLabels`
- Produces: HTTP endpoints `PUT /membros/{id}/cargo`, `GET /membros/{id}/produtos`, `PUT /membros/{id}/produtos`, `GET /produtos`, `GET /cargos`

- [ ] **Step 1: Update EquipeStore interface**

In `backend/internal/handler/equipe.go`, add to the `EquipeStore` interface (after `GetHorasTarefasEquipe`):

```go
UpdateMembroCargo(ctx context.Context, membroID uuid.UUID, cargo *string) error
ListProdutos(ctx context.Context) ([]domain.Produto, error)
GetMembroProdutos(ctx context.Context, membroID uuid.UUID) ([]domain.Produto, error)
SetMembroProdutos(ctx context.Context, membroID uuid.UUID, produtoIDs []uuid.UUID) error
```

- [ ] **Step 2: Add UpdateCargo handler**

Add to `backend/internal/handler/equipe.go`:

```go
func (h *EquipeHandler) UpdateCargo(w http.ResponseWriter, r *http.Request) {
	membroID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		Cargo *string `json:"cargo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if req.Cargo != nil && *req.Cargo != "" && !domain.IsCargoValido(*req.Cargo) {
		respondError(w, http.StatusBadRequest, "cargo inválido")
		return
	}
	if req.Cargo != nil && *req.Cargo == "" {
		req.Cargo = nil
	}
	if err := h.store.UpdateMembroCargo(r.Context(), membroID, req.Cargo); err != nil {
		h.logger.Error("failed to update membro cargo", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar cargo")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "cargo atualizado"})
}
```

- [ ] **Step 3: Add ListCargos handler**

```go
func (h *EquipeHandler) ListCargos(w http.ResponseWriter, r *http.Request) {
	type cargoItem struct {
		Value string `json:"value"`
		Label string `json:"label"`
	}
	result := make([]cargoItem, len(domain.CargosValidos))
	for i, c := range domain.CargosValidos {
		result[i] = cargoItem{Value: c, Label: domain.CargoLabels[c]}
	}
	respondJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 4: Add ListProdutos handler**

```go
func (h *EquipeHandler) ListProdutos(w http.ResponseWriter, r *http.Request) {
	produtos, err := h.store.ListProdutos(r.Context())
	if err != nil {
		h.logger.Error("failed to list produtos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar produtos")
		return
	}
	respondJSON(w, http.StatusOK, produtos)
}
```

- [ ] **Step 5: Add GetMembroProdutos handler**

```go
func (h *EquipeHandler) GetMembroProdutos(w http.ResponseWriter, r *http.Request) {
	membroID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}
	produtos, err := h.store.GetMembroProdutos(r.Context(), membroID)
	if err != nil {
		h.logger.Error("failed to get membro produtos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar produtos do membro")
		return
	}
	respondJSON(w, http.StatusOK, produtos)
}
```

- [ ] **Step 6: Add SetMembroProdutos handler**

```go
func (h *EquipeHandler) SetMembroProdutos(w http.ResponseWriter, r *http.Request) {
	membroID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		ProdutoIDs []string `json:"produto_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	ids := make([]uuid.UUID, 0, len(req.ProdutoIDs))
	for _, s := range req.ProdutoIDs {
		id, err := uuid.Parse(s)
		if err != nil {
			respondError(w, http.StatusBadRequest, "produto_id inválido: "+s)
			return
		}
		ids = append(ids, id)
	}
	if err := h.store.SetMembroProdutos(r.Context(), membroID, ids); err != nil {
		h.logger.Error("failed to set membro produtos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar produtos do membro")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "produtos atualizados"})
}
```

- [ ] **Step 7: Add import for domain package**

In `backend/internal/handler/equipe.go`, add to the import block:

```go
"github.com/emersonpaula83/myplanner/backend/internal/domain"
```

- [ ] **Step 8: Register routes**

In `backend/cmd/api/main.go`, after the existing `/membros/{id}/skills/{skillId}` route (line 216), add:

```go
r.Put("/membros/{id}/cargo", equipeHandler.UpdateCargo)
r.Get("/membros/{id}/produtos", equipeHandler.GetMembroProdutos)
r.Put("/membros/{id}/produtos", equipeHandler.SetMembroProdutos)

r.Get("/produtos", equipeHandler.ListProdutos)
r.Get("/cargos", equipeHandler.ListCargos)
```

- [ ] **Step 9: Verify build**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 10: Commit**

```bash
git add backend/internal/handler/equipe.go backend/cmd/api/main.go
git commit -m "feat: add cargo and produto handler endpoints with routes"
```

---

### Task 4: Frontend — Cargo Dropdown in Member Detail

**Files:**
- Modify: `frontend/index.html` (CSS + JS)

**Interfaces:**
- Consumes: `GET /cargos`, `PUT /membros/{id}/cargo`, `GET /membros/{id}/produtos`, `PUT /membros/{id}/produtos`, `GET /produtos`
- Produces: Cargo dropdown UI, conditional P.O. products section

- [ ] **Step 1: Add CSS for cargo section and produto chips**

In `frontend/index.html`, after the `.skill-dropdown-item.create` CSS rule (line 530), add:

```css
.cargo-section { margin-bottom: 16px; }
.cargo-section .form-label { font-size: 12px; margin-bottom: 4px; display: block; color: var(--text-secondary); }
.cargo-select { padding: 5px 10px; font-size: 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--card-bg); color: var(--text-primary); min-width: 220px; }
.produto-section { margin-top: 10px; }
.produto-chips { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; margin-top: 6px; }
.produto-chip { display: inline-flex; align-items: center; gap: 4px; background: var(--accent-soft); color: var(--accent); border-radius: 12px; padding: 2px 10px; font-size: 12px; font-weight: 600; }
.produto-chip .produto-remove { cursor: pointer; opacity: .6; font-size: 14px; line-height: 1; }
.produto-chip .produto-remove:hover { opacity: 1; }
.produto-chip .produto-remove.disabled { pointer-events: none; opacity: .2; }
.produto-input-wrap { position: relative; display: inline-block; margin-top: 6px; }
.produto-input-wrap input { padding: 4px 10px; font-size: 12px; border: 1px solid var(--border); border-radius: 6px; background: var(--card-bg); color: var(--text-primary); width: 220px; }
.produto-dropdown { position: absolute; top: 100%; left: 0; width: 220px; max-height: 160px; overflow-y: auto; background: var(--card-bg); border: 1px solid var(--border); border-radius: 6px; margin-top: 2px; z-index: 100; display: none; }
.produto-dropdown.open { display: block; }
.produto-dropdown-item { padding: 6px 10px; font-size: 12px; cursor: pointer; color: var(--text-primary); }
.produto-dropdown-item:hover { background: var(--accent-soft); }
.po-warning { color: var(--red); font-size: 11px; margin-top: 4px; }
.cargo-badge { display: inline-block; font-size: 10px; font-weight: 700; padding: 1px 8px; border-radius: 10px; margin-left: 6px; vertical-align: middle; }
.cargo-badge.blue { background: rgba(59,130,246,.15); color: #3b82f6; }
.cargo-badge.purple { background: rgba(139,92,246,.15); color: #8b5cf6; }
.cargo-badge.green { background: rgba(34,197,94,.15); color: #22c55e; }
.cargo-badge.gray { background: rgba(148,163,184,.15); color: #94a3b8; }
```

- [ ] **Step 2: Add cargo section to member detail HTML**

In the `loadMembroDetail` function, find the line that renders the skill section (currently line 2136):

```javascript
'<div class="skill-section" id="membro-skills-section"></div>' +
```

Insert BEFORE this line:

```javascript
'<div class="cargo-section" id="membro-cargo-section"></div>' +
```

- [ ] **Step 3: Add loadMembroCargo function**

Before the `loadMembroSkills` function (line 3535), add:

```javascript
var allCargos = null;
var allProdutos = null;

async function loadMembroCargo(membroId) {
  var section = document.getElementById('membro-cargo-section');
  if (!section) return;
  try {
    if (!allCargos) allCargos = await api('/cargos');
    var membro = await api('/membros/' + membroId);
    var cargo = membro.membro.cargo;
    renderMembroCargo(cargo, membroId, section);
  } catch (err) {
    section.innerHTML = '';
  }
}

function renderMembroCargo(cargo, membroId, container) {
  var options = '<option value="">Sem cargo</option>';
  (allCargos || []).forEach(function(c) {
    options += '<option value="' + c.value + '"' + (cargo === c.value ? ' selected' : '') + '>' + esc(c.label) + '</option>';
  });
  var html = '<label class="form-label">Cargo</label>' +
    '<select class="cargo-select" id="cargo-select-' + membroId + '" onchange="updateMembroCargo(\'' + membroId + '\')">' + options + '</select>' +
    '<div id="produto-section-' + membroId + '"></div>';
  container.innerHTML = html;
  if (cargo === 'po_produto') {
    loadMembroProdutos(membroId);
  }
}

async function updateMembroCargo(membroId) {
  var sel = document.getElementById('cargo-select-' + membroId);
  var cargo = sel.value || null;
  try {
    await api('/membros/' + membroId + '/cargo', { method: 'PUT', body: JSON.stringify({ cargo: cargo }) });
    var prodSection = document.getElementById('produto-section-' + membroId);
    if (cargo === 'po_produto') {
      loadMembroProdutos(membroId);
    } else {
      prodSection.innerHTML = '';
    }
  } catch (err) {
    alert('Falha ao atualizar cargo');
  }
}

async function loadMembroProdutos(membroId) {
  var section = document.getElementById('produto-section-' + membroId);
  if (!section) return;
  try {
    if (!allProdutos) allProdutos = await api('/produtos');
    var produtos = await api('/membros/' + membroId + '/produtos');
    renderMembroProdutos(produtos, membroId, section);
  } catch (err) {
    section.innerHTML = '';
  }
}

function renderMembroProdutos(produtos, membroId, container) {
  var html = '<div class="produto-section"><label class="form-label">Produtos (obrigatório para P.O.)</label><div class="produto-chips">';
  var disableRemove = produtos.length <= 1;
  produtos.forEach(function(p) {
    html += '<span class="produto-chip">' + esc(p.nome) +
      '<span class="produto-remove' + (disableRemove ? ' disabled' : '') + '" onclick="removeProdutoFromMembro(\'' + membroId + '\',\'' + p.id + '\',' + produtos.length + ')">&times;</span></span>';
  });
  html += '<button class="skill-add-btn" onclick="showProdutoInput(\'' + membroId + '\')" title="Adicionar produto">+</button>';
  html += '</div>';
  if (produtos.length === 0) {
    html += '<div class="po-warning">Selecione ao menos 1 produto</div>';
  }
  html += '<div class="produto-input-wrap" id="produto-input-wrap-' + membroId + '" style="display:none">' +
    '<input type="text" id="produto-input-' + membroId + '" placeholder="Buscar produto..." ' +
    'oninput="searchProdutos(\'' + membroId + '\')" onkeydown="produtoInputKeydown(event,\'' + membroId + '\')">' +
    '<div class="produto-dropdown" id="produto-dropdown-' + membroId + '"></div></div>';
  html += '</div>';
  container.innerHTML = html;
}

function showProdutoInput(membroId) {
  var wrap = document.getElementById('produto-input-wrap-' + membroId);
  wrap.style.display = 'inline-block';
  var inp = document.getElementById('produto-input-' + membroId);
  inp.value = '';
  inp.focus();
}

var produtoSearchTimer;

function searchProdutos(membroId) {
  clearTimeout(produtoSearchTimer);
  var inp = document.getElementById('produto-input-' + membroId);
  var q = inp.value.trim().toLowerCase();
  var dd = document.getElementById('produto-dropdown-' + membroId);
  if (q.length === 0) { dd.classList.remove('open'); return; }
  produtoSearchTimer = setTimeout(function() {
    var currentChips = document.querySelectorAll('#produto-section-' + membroId + ' .produto-chip');
    var currentIds = new Set();
    currentChips.forEach(function(c) {
      var onclick = c.querySelector('.produto-remove')?.getAttribute('onclick') || '';
      var match = onclick.match(/'([a-f0-9-]+)'/g);
      if (match && match[1]) currentIds.add(match[1].replace(/'/g, ''));
    });
    var filtered = (allProdutos || []).filter(function(p) {
      return p.nome.toLowerCase().includes(q);
    }).slice(0, 20);
    var html = '';
    filtered.forEach(function(p) {
      html += '<div class="produto-dropdown-item" onclick="addProdutoToMembro(\'' + membroId + '\',\'' + p.id + '\')">' + esc(p.nome) + '</div>';
    });
    if (filtered.length === 0) {
      html = '<div class="produto-dropdown-item" style="color:var(--text-secondary)">Nenhum produto encontrado</div>';
    }
    dd.innerHTML = html;
    dd.classList.add('open');
  }, 200);
}

function produtoInputKeydown(e, membroId) {
  if (e.key === 'Escape') {
    var wrap = document.getElementById('produto-input-wrap-' + membroId);
    wrap.style.display = 'none';
    document.getElementById('produto-dropdown-' + membroId).classList.remove('open');
  }
}

async function addProdutoToMembro(membroId, produtoId) {
  try {
    var existing = await api('/membros/' + membroId + '/produtos');
    var ids = existing.map(function(p) { return p.id; });
    if (ids.indexOf(produtoId) === -1) ids.push(produtoId);
    await api('/membros/' + membroId + '/produtos', { method: 'PUT', body: JSON.stringify({ produto_ids: ids }) });
    var wrap = document.getElementById('produto-input-wrap-' + membroId);
    wrap.style.display = 'none';
    document.getElementById('produto-dropdown-' + membroId).classList.remove('open');
    loadMembroProdutos(membroId);
  } catch (err) {
    alert('Falha ao adicionar produto');
  }
}

async function removeProdutoFromMembro(membroId, produtoId, currentCount) {
  if (currentCount <= 1) return;
  try {
    var existing = await api('/membros/' + membroId + '/produtos');
    var ids = existing.filter(function(p) { return p.id !== produtoId; }).map(function(p) { return p.id; });
    await api('/membros/' + membroId + '/produtos', { method: 'PUT', body: JSON.stringify({ produto_ids: ids }) });
    loadMembroProdutos(membroId);
  } catch (err) {
    alert('Falha ao remover produto');
  }
}
```

- [ ] **Step 4: Call loadMembroCargo in loadMembroDetail**

In `loadMembroDetail`, find the line `loadMembroSkills(membroId);` (line 2159). Add BEFORE it:

```javascript
loadMembroCargo(membroId);
```

- [ ] **Step 5: Verify in browser**

Start dev server, open member detail page, verify:
1. Cargo dropdown appears between desligamento and skills
2. Selecting a cargo makes PUT request
3. Selecting P.O. Produto shows products section
4. Adding/removing products works
5. Warning shows when no products selected for P.O.
6. Changing cargo away from P.O. hides products section

- [ ] **Step 6: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add cargo dropdown and produto selection in member detail page"
```

---

### Task 5: Frontend — Cargo Badges in Equipe Member List

**Files:**
- Modify: `frontend/index.html` (renderEquipeResumo function)

**Interfaces:**
- Consumes: `MembroResumo.cargo` field from `GET /equipes/{id}/resumo`
- Produces: Colored cargo badges in equipe member listing

- [ ] **Step 1: Add cargo badge helper function**

Before the `renderEquipeResumo` function (line 1178), add:

```javascript
function cargoLabelMap(cargo) {
  var labels = {
    'coordenador_desenvolvimento': 'Coord. Dev',
    'po_produto': 'P.O.',
    'gerente_tecnologia': 'Ger. Tecnologia',
    'gerente_executivo': 'Ger. Executivo',
    'scrum_master': 'Scrum Master',
    'agile_master': 'Agile Master',
    'desenvolvedor': 'Dev'
  };
  return labels[cargo] || '';
}

function cargoBadgeColor(cargo) {
  if (!cargo) return '';
  if (cargo === 'po_produto') return 'purple';
  if (cargo === 'scrum_master' || cargo === 'agile_master') return 'green';
  if (cargo === 'desenvolvedor') return 'gray';
  return 'blue';
}
```

- [ ] **Step 2: Add cargo badge to member row**

In `renderEquipeResumo` (around line 1190), find the line that renders the member name:

```javascript
'<div class="member-info"><div class="member-name">' + esc(m.nome) + '</div><div class="member-email">' + esc(m.email) + '</div></div>' +
```

Replace with:

```javascript
'<div class="member-info"><div class="member-name">' + esc(m.nome) + (m.cargo ? '<span class="cargo-badge ' + cargoBadgeColor(m.cargo) + '">' + cargoLabelMap(m.cargo) + '</span>' : '') + '</div><div class="member-email">' + esc(m.email) + '</div></div>' +
```

- [ ] **Step 3: Verify in browser**

Start dev server, select a team, verify:
1. Members with cargo show colored badge next to name
2. Members without cargo show no badge
3. Badge colors match spec (blue for managers, purple for P.O., green for masters, gray for dev)
4. Badges don't break layout

- [ ] **Step 4: Commit**

```bash
git add frontend/index.html
git commit -m "feat: show cargo badges in equipe member listing"
```
