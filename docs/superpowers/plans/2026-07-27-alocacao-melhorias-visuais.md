# Alocação de Projetos — Melhorias e Visuais Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix bugs (status badge, pct_no_projeto), add encerramento flow, status/product filters, tipo_demanda sections, deadline badges/Gantt line, responsável/avatars in modal, sidebar reorganization, and manual épico↔equipe association (N:N) with "Todas as Equipes" filter.

**Architecture:** Repository methods extended with new queries and types. Service layer adds close/reopen logic, fixes GetProjectDetail to use GetEpicByID, computes pct_no_projeto. Handler adds 5 new endpoints. Frontend adds filters, tipo_demanda sections, encerrar modal, deadline badges, avatars, Gantt deadline line, and equipe multi-select in project metadata modal. New `epico_equipes` table enables manual team assignment; allocation queries use OR logic (implicit via fonte_dados_id + explicit via epico_equipes).

**Tech Stack:** Go (chi, pgx/v5, zap), PostgreSQL, vanilla JS frontend

## Global Constraints

- Frontend: `var`/`function` only, NO ES6+ (no `const`, `let`, `=>`, template literals). Use `esc()` for text, `escAttr()` for attributes (XSS prevention)
- CSS custom properties: `--surface`, `--text-primary`, `--accent`, `--border`, `--text-secondary`
- Dark mode: `@media (prefers-color-scheme: dark)` + `:root[data-theme="dark"]` + `:root[data-theme="light"]`
- Go: chi router, pgx/v5, zap logger
- Do NOT commit changes — leave all changes unstaged

---

### Task 1: Migration + Repository

**Files:**
- Create: `backend/migrations/000017_projeto_encerramentos.up.sql`
- Create: `backend/migrations/000017_projeto_encerramentos.down.sql`
- Create: `backend/migrations/000021_epico_equipes.up.sql`
- Create: `backend/migrations/000021_epico_equipes.down.sql`
- Modify: `backend/internal/repository/allocation.go`
- Modify: `backend/internal/repository/timeline.go`

**Interfaces:**
- Consumes: nothing from prior tasks
- Produces:
  - Types: `ProjectClosureRow{Descricao string, DataEncerramento time.Time, EncerradoPor string, CreatedAt time.Time}`, `ProdutoRow{ID uuid.UUID, Nome string}`
  - `EpicAllocationRow` gains 4 fields: `ResponsavelID *uuid.UUID`, `ResponsavelNome *string`, `ResponsavelAvatar *string`, `ResponsavelCargo *string`
  - `PersonAllocationRow` gains: `AvatarURL *string`
  - `TaskAllocationRow` gains: `ResponsavelAvatar *string`
  - `GetEpicsByEquipeAndProduto(ctx, equipeID, produtoID uuid.UUID, statusFilter string) ([]EpicAllocationRow, error)`
  - `GetEpicByID(ctx, epicID uuid.UUID) (*EpicAllocationRow, error)`
  - `GetEpicPeople(ctx, epicID uuid.UUID) ([]PersonAllocationRow, error)` — now returns AvatarURL
  - `GetEpicTasks(ctx, epicID uuid.UUID) ([]TaskAllocationRow, error)` — now returns ResponsavelAvatar
  - `CloseProject(ctx, epicID uuid.UUID, descricao string, dataEncerramento time.Time, encerradoPor string) error`
  - `ReopenProject(ctx, epicID uuid.UUID) error`
  - `GetClosedEpicIDs(ctx, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error)`
  - `GetProjectClosure(ctx, epicID uuid.UUID) (*ProjectClosureRow, error)`
  - `GetProdutosComProjetosAtivos(ctx, equipeID uuid.UUID) ([]ProdutoRow, error)` — equipeID=uuid.Nil means "todas"
  - `GetPersonTotalAllocatedHours(ctx, membroIDs []uuid.UUID) (map[uuid.UUID]float64, error)`
  - `SalvarEpicoEquipes(ctx, epicoID uuid.UUID, equipeIDs []uuid.UUID) error`
  - `BuscarEpicoEquipes(ctx, epicoID uuid.UUID) ([]uuid.UUID, error)`
  - Modified `ListarEpicos` in timeline.go — also matches épicos via `epico_equipes` table

- [ ] **Step 1: Create migration up file**

Create `backend/migrations/000017_projeto_encerramentos.up.sql`:

```sql
CREATE TABLE projeto_encerramentos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    epic_id UUID NOT NULL REFERENCES tarefas(id),
    descricao TEXT NOT NULL,
    data_encerramento DATE NOT NULL,
    encerrado_por TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(epic_id)
);
```

- [ ] **Step 2: Create migration down file**

Create `backend/migrations/000017_projeto_encerramentos.down.sql`:

```sql
DROP TABLE IF EXISTS projeto_encerramentos;
```

- [ ] **Step 2b: Create epico_equipes migration up file**

Create `backend/migrations/000021_epico_equipes.up.sql`:

```sql
CREATE TABLE epico_equipes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    epico_id UUID NOT NULL REFERENCES tarefas(id) ON DELETE CASCADE,
    equipe_id UUID NOT NULL REFERENCES equipes(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(epico_id, equipe_id)
);
CREATE INDEX idx_epico_equipes_epico ON epico_equipes(epico_id);
CREATE INDEX idx_epico_equipes_equipe ON epico_equipes(equipe_id);
```

- [ ] **Step 2c: Create epico_equipes migration down file**

Create `backend/migrations/000021_epico_equipes.down.sql`:

```sql
DROP TABLE IF EXISTS epico_equipes;
```

- [ ] **Step 3: Add new types to repository**

In `backend/internal/repository/allocation.go`, after the `TaskPreviousState` struct (around line 70), add:

```go
type ProjectClosureRow struct {
	Descricao        string
	DataEncerramento time.Time
	EncerradoPor     string
	CreatedAt        time.Time
}

type ProdutoRow struct {
	ID   uuid.UUID
	Nome string
}
```

- [ ] **Step 4: Add responsável fields to EpicAllocationRow**

In `backend/internal/repository/allocation.go`, add 4 fields at the end of `EpicAllocationRow`:

```go
type EpicAllocationRow struct {
	EpicID              uuid.UUID
	NumeroTicket        string
	Resumo              string
	Apelido             *string
	DataLimite          *time.Time
	Prioridade          *string
	TipoDemanda         *string
	Produtos            []string
	TotalFilhas         int
	FilhasComEstimativa int
	HorasEstimadas      float64
	HorasEmSprint       float64
	ResponsavelID       *uuid.UUID
	ResponsavelNome     *string
	ResponsavelAvatar   *string
	ResponsavelCargo    *string
}
```

- [ ] **Step 5: Add AvatarURL to PersonAllocationRow**

```go
type PersonAllocationRow struct {
	MembroID       uuid.UUID
	Nome           string
	HorasNoProjeto float64
	AvatarURL      *string
}
```

- [ ] **Step 6: Add ResponsavelAvatar to TaskAllocationRow**

Add after `ResponsavelNome`:

```go
type TaskAllocationRow struct {
	TarefaID        uuid.UUID
	NumeroTicket    string
	Resumo          string
	Tipo            string
	TipoDemanda     *string
	Status          string
	EstimativaTempo *int
	SprintID        *uuid.UUID
	SprintNome      *string
	SprintInicio    *time.Time
	SprintFim       *time.Time
	ResponsavelID   *uuid.UUID
	ResponsavelNome *string
	ResponsavelAvatar *string
}
```

- [ ] **Step 7: Modify GetEpicsByEquipeAndProduto**

Replace the entire `GetEpicsByEquipeAndProduto` method. New signature adds `statusFilter string` parameter. Adds `LEFT JOIN membros m ON m.id = e.responsavel_id`, 4 responsável columns in SELECT, and dynamic status filter clause:

```go
func (r *AllocationRepository) GetEpicsByEquipeAndProduto(ctx context.Context, equipeID, produtoID uuid.UUID, statusFilter string) ([]EpicAllocationRow, error) {
	var statusClause string
	switch statusFilter {
	case "encerrados":
		statusClause = " AND EXISTS (SELECT 1 FROM projeto_encerramentos pe WHERE pe.epic_id = e.id)"
	case "todos":
		statusClause = ""
	default:
		statusClause = " AND NOT EXISTS (SELECT 1 FROM projeto_encerramentos pe WHERE pe.epic_id = e.id)"
	}

	query := `
		SELECT
			e.id,
			e.numero_ticket,
			e.resumo,
			e.apelido,
			e.data_limite::timestamptz,
			e.prioridade,
			COALESCE(e.tipo_demanda,
				CASE
					WHEN e.tipo IN ('Épico', 'Projeto') THEN 'Meta'
					WHEN e.tipo IN ('Spike', 'Implantação', 'Aditivo - Delivery') THEN 'Compromisso'
					ELSE 'Iniciativa'
				END
			),
			COALESCE(
				(SELECT ARRAY_AGG(DISTINCT p.nome ORDER BY p.nome)
				 FROM tarefas c
				 JOIN tarefa_produtos tp ON tp.tarefa_id = c.id
				 JOIN produtos p ON p.id = tp.produto_id
				 WHERE c.parent_id = e.id),
				'{}'
			),
			(SELECT COUNT(*) FROM tarefas c WHERE c.parent_id = e.id AND c.status NOT IN ('Cancelado', 'Rejeitada'))::int,
			(SELECT COUNT(*) FROM tarefas c WHERE c.parent_id = e.id AND c.status NOT IN ('Cancelado', 'Rejeitada') AND c.estimativa_tempo IS NOT NULL AND c.estimativa_tempo > 0)::int,
			COALESCE(
				(SELECT SUM(c.estimativa_tempo) FROM tarefas c WHERE c.parent_id = e.id AND c.status NOT IN ('Cancelado', 'Rejeitada') AND c.estimativa_tempo IS NOT NULL AND c.estimativa_tempo > 0),
				0
			)::float8 / 3600.0,
			COALESCE(
				(SELECT SUM(c.estimativa_tempo) FROM tarefas c
				 JOIN sprints s ON s.id = c.sprint_id
				 WHERE c.parent_id = e.id
				   AND c.status NOT IN ('Cancelado', 'Rejeitada')
				   AND c.estimativa_tempo IS NOT NULL AND c.estimativa_tempo > 0
				   AND s.estado IN ('active', 'future')),
				0
			)::float8 / 3600.0,
			rm.id, rm.nome, rm.avatar_url, rm.cargo
		FROM tarefas e
		LEFT JOIN membros rm ON rm.id = e.responsavel_id
		WHERE e.tipo IN ('Épico', 'Epico')
		  AND ($1 = '00000000-0000-0000-0000-000000000000'::uuid OR (
			e.fonte_dados_id IN (
				SELECT DISTINCT m.fonte_dados_id
				FROM equipe_membros em
				JOIN membros m ON em.membro_id = m.id
				WHERE em.equipe_id = $1
			)
			OR e.id IN (SELECT epico_id FROM epico_equipes WHERE equipe_id = $1)
		  ))
		  AND EXISTS (
			SELECT 1 FROM tarefas c
			JOIN tarefa_produtos tp ON tp.tarefa_id = c.id
			WHERE c.parent_id = e.id AND tp.produto_id = $2
		  )
		  AND e.status NOT IN ('Cancelado', 'Rejeitada', 'Concluído')` + statusClause + `
		ORDER BY
			CASE e.prioridade
				WHEN 'Highest' THEN 1
				WHEN 'High' THEN 2
				WHEN 'Medium' THEN 3
				WHEN 'Low' THEN 4
				WHEN 'Lowest' THEN 5
				ELSE 6
			END,
			CASE
				WHEN COALESCE(e.tipo_demanda, '') = 'Meta' THEN 1
				WHEN COALESCE(e.tipo_demanda, '') = 'Compromisso' THEN 2
				WHEN COALESCE(e.tipo_demanda, '') = 'Iniciativa' THEN 3
				ELSE 4
			END,
			e.numero_ticket
	`

	rows, err := r.pool.Query(ctx, query, equipeID, produtoID)
	if err != nil {
		return nil, fmt.Errorf("querying epics: %w", err)
	}
	defer rows.Close()

	result := make([]EpicAllocationRow, 0)
	for rows.Next() {
		var e EpicAllocationRow
		if err := rows.Scan(
			&e.EpicID, &e.NumeroTicket, &e.Resumo, &e.Apelido,
			&e.DataLimite, &e.Prioridade, &e.TipoDemanda, &e.Produtos,
			&e.TotalFilhas, &e.FilhasComEstimativa,
			&e.HorasEstimadas, &e.HorasEmSprint,
			&e.ResponsavelID, &e.ResponsavelNome, &e.ResponsavelAvatar, &e.ResponsavelCargo,
		); err != nil {
			return nil, fmt.Errorf("scanning epic: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
```

Note: the JOIN alias is `rm` (not `m`) to avoid conflict with `m` used in the equipe subquery. When `equipeID = uuid.Nil` (all zeros), the `$1 = '000...'` shortcircuit makes the equipe clause a no-op → returns all teams' projects ("Todas as Equipes"). The `epico_equipes` OR clause finds épicos manually assigned to a team even when they have no members in that team via `fonte_dados_id`.

- [ ] **Step 8: Add GetEpicByID method**

Add after `GetEpicsByEquipeAndProduto`:

```go
func (r *AllocationRepository) GetEpicByID(ctx context.Context, epicID uuid.UUID) (*EpicAllocationRow, error) {
	var e EpicAllocationRow
	err := r.pool.QueryRow(ctx, `
		SELECT
			e.id, e.numero_ticket, e.resumo, e.apelido,
			e.data_limite::timestamptz, e.prioridade,
			COALESCE(e.tipo_demanda,
				CASE
					WHEN e.tipo IN ('Épico', 'Projeto') THEN 'Meta'
					WHEN e.tipo IN ('Spike', 'Implantação', 'Aditivo - Delivery') THEN 'Compromisso'
					ELSE 'Iniciativa'
				END
			),
			COALESCE(
				(SELECT ARRAY_AGG(DISTINCT p.nome ORDER BY p.nome)
				 FROM tarefas c
				 JOIN tarefa_produtos tp ON tp.tarefa_id = c.id
				 JOIN produtos p ON p.id = tp.produto_id
				 WHERE c.parent_id = e.id),
				'{}'
			),
			(SELECT COUNT(*) FROM tarefas c WHERE c.parent_id = e.id AND c.status NOT IN ('Cancelado', 'Rejeitada'))::int,
			(SELECT COUNT(*) FROM tarefas c WHERE c.parent_id = e.id AND c.status NOT IN ('Cancelado', 'Rejeitada') AND c.estimativa_tempo IS NOT NULL AND c.estimativa_tempo > 0)::int,
			COALESCE(
				(SELECT SUM(c.estimativa_tempo) FROM tarefas c WHERE c.parent_id = e.id AND c.status NOT IN ('Cancelado', 'Rejeitada') AND c.estimativa_tempo IS NOT NULL AND c.estimativa_tempo > 0),
				0
			)::float8 / 3600.0,
			COALESCE(
				(SELECT SUM(c.estimativa_tempo) FROM tarefas c
				 JOIN sprints s ON s.id = c.sprint_id
				 WHERE c.parent_id = e.id
				   AND c.status NOT IN ('Cancelado', 'Rejeitada')
				   AND c.estimativa_tempo IS NOT NULL AND c.estimativa_tempo > 0
				   AND s.estado IN ('active', 'future')),
				0
			)::float8 / 3600.0,
			rm.id, rm.nome, rm.avatar_url, rm.cargo
		FROM tarefas e
		LEFT JOIN membros rm ON rm.id = e.responsavel_id
		WHERE e.id = $1
	`, epicID).Scan(
		&e.EpicID, &e.NumeroTicket, &e.Resumo, &e.Apelido,
		&e.DataLimite, &e.Prioridade, &e.TipoDemanda, &e.Produtos,
		&e.TotalFilhas, &e.FilhasComEstimativa,
		&e.HorasEstimadas, &e.HorasEmSprint,
		&e.ResponsavelID, &e.ResponsavelNome, &e.ResponsavelAvatar, &e.ResponsavelCargo,
	)
	if err != nil {
		return nil, fmt.Errorf("querying epic by id: %w", err)
	}
	return &e, nil
}
```

- [ ] **Step 9: Modify GetEpicPeople to return avatar**

Replace the entire `GetEpicPeople` method:

```go
func (r *AllocationRepository) GetEpicPeople(ctx context.Context, epicID uuid.UUID) ([]PersonAllocationRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			m.id,
			m.nome,
			COALESCE(SUM(t.estimativa_tempo), 0)::float8 / 3600.0,
			m.avatar_url
		FROM tarefas t
		JOIN membros m ON m.id = t.responsavel_id
		WHERE t.parent_id = $1
		  AND t.status NOT IN ('Cancelado', 'Rejeitada')
		GROUP BY m.id, m.nome, m.avatar_url
		ORDER BY SUM(t.estimativa_tempo) DESC NULLS LAST
	`, epicID)
	if err != nil {
		return nil, fmt.Errorf("querying epic people: %w", err)
	}
	defer rows.Close()

	result := make([]PersonAllocationRow, 0)
	for rows.Next() {
		var p PersonAllocationRow
		if err := rows.Scan(&p.MembroID, &p.Nome, &p.HorasNoProjeto, &p.AvatarURL); err != nil {
			return nil, fmt.Errorf("scanning person: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
```

- [ ] **Step 10: Modify GetEpicTasks to return avatar**

Replace the `GetEpicTasks` method. Add `m.avatar_url` to SELECT and Scan:

```go
func (r *AllocationRepository) GetEpicTasks(ctx context.Context, epicID uuid.UUID) ([]TaskAllocationRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			t.id, t.numero_ticket, t.resumo, t.tipo, t.tipo_demanda, t.status,
			t.estimativa_tempo,
			t.sprint_id, s.nome, s.data_inicio, s.data_fim,
			t.responsavel_id, m.nome, m.avatar_url
		FROM tarefas t
		LEFT JOIN sprints s ON s.id = t.sprint_id
		LEFT JOIN membros m ON m.id = t.responsavel_id
		WHERE t.parent_id = $1
		  AND t.status NOT IN ('Cancelado', 'Rejeitada')
		ORDER BY t.numero_ticket
	`, epicID)
	if err != nil {
		return nil, fmt.Errorf("querying epic tasks: %w", err)
	}
	defer rows.Close()

	result := make([]TaskAllocationRow, 0)
	for rows.Next() {
		var t TaskAllocationRow
		if err := rows.Scan(
			&t.TarefaID, &t.NumeroTicket, &t.Resumo, &t.Tipo, &t.TipoDemanda, &t.Status,
			&t.EstimativaTempo,
			&t.SprintID, &t.SprintNome, &t.SprintInicio, &t.SprintFim,
			&t.ResponsavelID, &t.ResponsavelNome, &t.ResponsavelAvatar,
		); err != nil {
			return nil, fmt.Errorf("scanning task: %w", err)
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
```

- [ ] **Step 11: Add CloseProject method**

```go
func (r *AllocationRepository) CloseProject(ctx context.Context, epicID uuid.UUID, descricao string, dataEncerramento time.Time, encerradoPor string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO projeto_encerramentos (epic_id, descricao, data_encerramento, encerrado_por)
		VALUES ($1, $2, $3, $4)
	`, epicID, descricao, dataEncerramento, encerradoPor)
	if err != nil {
		return fmt.Errorf("closing project: %w", err)
	}
	return nil
}
```

- [ ] **Step 12: Add ReopenProject method**

```go
func (r *AllocationRepository) ReopenProject(ctx context.Context, epicID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM projeto_encerramentos WHERE epic_id = $1`, epicID)
	if err != nil {
		return fmt.Errorf("reopening project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("project not closed")
	}
	return nil
}
```

- [ ] **Step 13: Add GetClosedEpicIDs method**

```go
func (r *AllocationRepository) GetClosedEpicIDs(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if len(epicIDs) == 0 {
		return make(map[uuid.UUID]bool), nil
	}
	rows, err := r.pool.Query(ctx, `SELECT epic_id FROM projeto_encerramentos WHERE epic_id = ANY($1)`, epicIDs)
	if err != nil {
		return nil, fmt.Errorf("querying closed epics: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]bool)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning closed epic: %w", err)
		}
		result[id] = true
	}
	return result, rows.Err()
}
```

- [ ] **Step 14: Add GetProjectClosure method**

```go
func (r *AllocationRepository) GetProjectClosure(ctx context.Context, epicID uuid.UUID) (*ProjectClosureRow, error) {
	var c ProjectClosureRow
	err := r.pool.QueryRow(ctx, `
		SELECT descricao, data_encerramento, encerrado_por, created_at
		FROM projeto_encerramentos WHERE epic_id = $1
	`, epicID).Scan(&c.Descricao, &c.DataEncerramento, &c.EncerradoPor, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}
```

- [ ] **Step 15: Add GetProdutosComProjetosAtivos method**

```go
func (r *AllocationRepository) GetProdutosComProjetosAtivos(ctx context.Context, equipeID uuid.UUID) ([]ProdutoRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT p.id, p.nome
		FROM produtos p
		JOIN tarefa_produtos tp ON tp.produto_id = p.id
		JOIN tarefas c ON c.id = tp.tarefa_id
		JOIN tarefas e ON e.id = c.parent_id
		WHERE e.tipo IN ('Épico', 'Epico')
		  AND e.status NOT IN ('Cancelado', 'Rejeitada', 'Concluído')
		  AND NOT EXISTS (SELECT 1 FROM projeto_encerramentos pe WHERE pe.epic_id = e.id)
		  AND ($1 = '00000000-0000-0000-0000-000000000000'::uuid OR (
			e.fonte_dados_id IN (
				SELECT DISTINCT m.fonte_dados_id
				FROM equipe_membros em
				JOIN membros m ON em.membro_id = m.id
				WHERE em.equipe_id = $1
			)
			OR e.id IN (SELECT epico_id FROM epico_equipes WHERE equipe_id = $1)
		  ))
		ORDER BY p.nome
	`, equipeID)
	if err != nil {
		return nil, fmt.Errorf("querying active products: %w", err)
	}
	defer rows.Close()

	result := make([]ProdutoRow, 0)
	for rows.Next() {
		var p ProdutoRow
		if err := rows.Scan(&p.ID, &p.Nome); err != nil {
			return nil, fmt.Errorf("scanning product: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
```

- [ ] **Step 16: Add GetPersonTotalAllocatedHours method**

```go
func (r *AllocationRepository) GetPersonTotalAllocatedHours(ctx context.Context, membroIDs []uuid.UUID) (map[uuid.UUID]float64, error) {
	if len(membroIDs) == 0 {
		return make(map[uuid.UUID]float64), nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT t.responsavel_id, COALESCE(SUM(t.estimativa_tempo), 0)::float8 / 3600.0
		FROM tarefas t
		WHERE t.responsavel_id = ANY($1)
		  AND t.sprint_id IS NOT NULL
		  AND t.status NOT IN ('Cancelado', 'Rejeitada')
		GROUP BY t.responsavel_id
	`, membroIDs)
	if err != nil {
		return nil, fmt.Errorf("querying total allocated hours: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]float64)
	for rows.Next() {
		var id uuid.UUID
		var hours float64
		if err := rows.Scan(&id, &hours); err != nil {
			return nil, fmt.Errorf("scanning allocated hours: %w", err)
		}
		result[id] = hours
	}
	return result, rows.Err()
}
```

- [ ] **Step 17: Add SalvarEpicoEquipes method**

```go
func (r *AllocationRepository) SalvarEpicoEquipes(ctx context.Context, epicoID uuid.UUID, equipeIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM epico_equipes WHERE epico_id = $1`, epicoID); err != nil {
		return fmt.Errorf("clearing epico equipes: %w", err)
	}

	for _, eqID := range equipeIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO epico_equipes (epico_id, equipe_id) VALUES ($1, $2)`, epicoID, eqID); err != nil {
			return fmt.Errorf("inserting epico equipe: %w", err)
		}
	}

	return tx.Commit(ctx)
}
```

- [ ] **Step 18: Add BuscarEpicoEquipes method**

```go
func (r *AllocationRepository) BuscarEpicoEquipes(ctx context.Context, epicoID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `SELECT equipe_id FROM epico_equipes WHERE epico_id = $1`, epicoID)
	if err != nil {
		return nil, fmt.Errorf("querying epico equipes: %w", err)
	}
	defer rows.Close()

	var result []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning equipe id: %w", err)
		}
		result = append(result, id)
	}
	return result, rows.Err()
}
```

- [ ] **Step 19: Modify ListarEpicos in timeline.go**

In `backend/internal/repository/timeline.go`, modify the `ListarEpicos` method. The equipe filter (which currently checks `ch.responsavel_id IN equipe_membros`) should also match épicos via `epico_equipes`:

Replace the equipe-filtered query (the `if equipeID != nil` branch) WHERE clause from:
```sql
AND EXISTS (
    SELECT 1 FROM tarefas ch
    WHERE ch.parent_id = e.id
      AND ch.responsavel_id IN (SELECT membro_id FROM equipe_membros WHERE equipe_id = $1)
)
```

To:
```sql
AND (
    EXISTS (
        SELECT 1 FROM tarefas ch
        WHERE ch.parent_id = e.id
          AND ch.responsavel_id IN (SELECT membro_id FROM equipe_membros WHERE equipe_id = $1)
    )
    OR e.id IN (SELECT epico_id FROM epico_equipes WHERE equipe_id = $1)
)
```

- [ ] **Step 20: Build and vet**

Run: `cd backend && go build ./... && go vet ./...`

Expected: clean build. The service layer will have compile errors because `GetEpicsByEquipeAndProduto` signature changed — that is expected and will be fixed in Task 2.

---

### Task 2: Service Layer

**Files:**
- Modify: `backend/internal/service/allocation.go`
- Modify: `backend/internal/service/allocation_test.go`

**Interfaces:**
- Consumes (from Task 1):
  - `repo.GetEpicsByEquipeAndProduto(ctx, equipeID, produtoID, statusFilter)` — added `statusFilter string` param
  - `repo.GetEpicByID(ctx, epicID)` — returns `*EpicAllocationRow`
  - `repo.CloseProject(ctx, epicID, descricao, dataEncerramento, encerradoPor)` — returns error
  - `repo.ReopenProject(ctx, epicID)` — returns error
  - `repo.GetClosedEpicIDs(ctx, epicIDs)` — returns `map[uuid.UUID]bool`
  - `repo.GetProjectClosure(ctx, epicID)` — returns `*ProjectClosureRow`
  - `repo.GetProdutosComProjetosAtivos(ctx, equipeID)` — returns `[]ProdutoRow`
  - `repo.GetPersonTotalAllocatedHours(ctx, membroIDs)` — returns `map[uuid.UUID]float64`
  - `EpicAllocationRow` has `.ResponsavelID`, `.ResponsavelNome`, `.ResponsavelAvatar`, `.ResponsavelCargo`
  - `PersonAllocationRow` has `.AvatarURL`
  - `TaskAllocationRow` has `.ResponsavelAvatar`
- Produces:
  - `CloseProjectRequest{Descricao string, DataEncerramento string}`
  - `ProjectClosure{Descricao string, DataEncerramento time.Time, EncerradoPor string}`
  - `ProjectAllocation` gains: `ResponsavelNome *string`, `ResponsavelAvatar *string`, `ResponsavelCargo *string`, `Encerrado bool`, `Encerramento *ProjectClosure`
  - `PersonAllocation` gains: `AvatarURL string`
  - `TaskAllocation` gains: `ResponsavelAvatar *string`
  - `ListProjectAllocations(ctx, equipeID, produtoID uuid.UUID, statusFilter string) ([]ProjectAllocation, error)`
  - `GetProjectDetail(ctx, epicID, equipeID uuid.UUID) (*ProjectDetail, error)` — fixed, now uses GetEpicByID
  - `CloseProject(ctx, epicID uuid.UUID, req CloseProjectRequest, encerradoPor string) error`
  - `ReopenProject(ctx, epicID uuid.UUID) error`
  - `GetFilteredProducts(ctx, equipeID uuid.UUID) ([]repository.ProdutoRow, error)`

- [ ] **Step 1: Add new structs**

In `backend/internal/service/allocation.go`, after `AllocateTaskResult` (line 86), add:

```go
type CloseProjectRequest struct {
	Descricao        string `json:"descricao"`
	DataEncerramento string `json:"data_encerramento"`
}

type ProjectClosure struct {
	Descricao        string    `json:"descricao"`
	DataEncerramento time.Time `json:"data_encerramento"`
	EncerradoPor     string    `json:"encerrado_por"`
}
```

- [ ] **Step 2: Add new fields to ProjectAllocation**

Add after the `Status` field:

```go
type ProjectAllocation struct {
	EpicID            uuid.UUID        `json:"epic_id"`
	NumeroTicket      string           `json:"numero_ticket"`
	Resumo            string           `json:"resumo"`
	Apelido           *string          `json:"apelido"`
	DataLimite        *time.Time       `json:"data_limite"`
	Prioridade        *string          `json:"prioridade"`
	TipoDemanda       *string          `json:"tipo_demanda"`
	Produtos          []string         `json:"produtos"`
	PctEstimado       float64          `json:"pct_estimado"`
	PctPlanejado      float64          `json:"pct_planejado"`
	TarefasSemEst     int              `json:"tarefas_sem_estimativa"`
	TotalTarefas      int              `json:"total_tarefas"`
	IsGDPTC           bool             `json:"is_gdptc"`
	Status            string           `json:"status"`
	ResponsavelNome   *string          `json:"responsavel_nome"`
	ResponsavelAvatar *string          `json:"responsavel_avatar"`
	ResponsavelCargo  *string          `json:"responsavel_cargo"`
	Encerrado         bool             `json:"encerrado"`
	Encerramento      *ProjectClosure  `json:"encerramento,omitempty"`
}
```

- [ ] **Step 3: Add AvatarURL to PersonAllocation**

```go
type PersonAllocation struct {
	MembroID       uuid.UUID `json:"membro_id"`
	Nome           string    `json:"nome"`
	HorasNoProjeto float64   `json:"horas_no_projeto"`
	HorasCapTotal  float64   `json:"horas_cap_total"`
	PctNoProjeto   float64   `json:"pct_no_projeto"`
	AvatarURL      string    `json:"avatar_url"`
}
```

- [ ] **Step 4: Add ResponsavelAvatar to TaskAllocation**

```go
type TaskAllocation struct {
	TarefaID          uuid.UUID  `json:"tarefa_id"`
	NumeroTicket      string     `json:"numero_ticket"`
	Resumo            string     `json:"resumo"`
	Tipo              string     `json:"tipo"`
	TipoDemanda       *string    `json:"tipo_demanda"`
	Status            string     `json:"status"`
	EstimativaHoras   *float64   `json:"estimativa_horas"`
	SprintID          *uuid.UUID `json:"sprint_id"`
	SprintNome        *string    `json:"sprint_nome"`
	SprintInicio      *time.Time `json:"sprint_inicio"`
	SprintFim         *time.Time `json:"sprint_fim"`
	ResponsavelID     *uuid.UUID `json:"responsavel_id"`
	ResponsavelNome   *string    `json:"responsavel_nome"`
	ResponsavelAvatar *string    `json:"responsavel_avatar"`
}
```

- [ ] **Step 5: Modify ListProjectAllocations**

Replace the entire method. Key changes: add `statusFilter string` param, pass to repo, add responsável fields, get closed IDs for encerramento data:

```go
func (s *AllocationService) ListProjectAllocations(ctx context.Context, equipeID, produtoID uuid.UUID, statusFilter string) ([]ProjectAllocation, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("allocation repository not configured")
	}

	rows, err := s.repo.GetEpicsByEquipeAndProduto(ctx, equipeID, produtoID, statusFilter)
	if err != nil {
		return nil, fmt.Errorf("listing epics: %w", err)
	}

	epicIDs := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		epicIDs[i] = r.EpicID
	}

	gdptcMap, err := s.repo.CheckGDPTCAncestors(ctx, epicIDs)
	if err != nil {
		s.logger.Warn("checking GDPTC ancestors", zap.Error(err))
		gdptcMap = make(map[uuid.UUID]bool)
	}

	closedMap, err := s.repo.GetClosedEpicIDs(ctx, epicIDs)
	if err != nil {
		s.logger.Warn("checking closed epics", zap.Error(err))
		closedMap = make(map[uuid.UUID]bool)
	}

	result := make([]ProjectAllocation, 0, len(rows))
	for _, r := range rows {
		var pctEstimado, pctPlanejado float64
		if r.TotalFilhas > 0 {
			pctEstimado = float64(r.FilhasComEstimativa) / float64(r.TotalFilhas) * 100
		}
		if r.HorasEstimadas > 0 {
			pctPlanejado = r.HorasEmSprint / r.HorasEstimadas * 100
		}

		status := "nao_planejado"
		if pctPlanejado >= 100 {
			status = "planejado"
			pctPlanejado = 100
		} else if pctPlanejado > 0 {
			status = "em_planejamento"
		}

		pa := ProjectAllocation{
			EpicID:            r.EpicID,
			NumeroTicket:      r.NumeroTicket,
			Resumo:            r.Resumo,
			Apelido:           r.Apelido,
			DataLimite:        r.DataLimite,
			Prioridade:        r.Prioridade,
			TipoDemanda:       r.TipoDemanda,
			Produtos:          r.Produtos,
			PctEstimado:       pctEstimado,
			PctPlanejado:      pctPlanejado,
			TarefasSemEst:     r.TotalFilhas - r.FilhasComEstimativa,
			TotalTarefas:      r.TotalFilhas,
			IsGDPTC:           gdptcMap[r.EpicID],
			Status:            status,
			ResponsavelNome:   r.ResponsavelNome,
			ResponsavelAvatar: r.ResponsavelAvatar,
			ResponsavelCargo:  r.ResponsavelCargo,
			Encerrado:         closedMap[r.EpicID],
		}

		if pa.Encerrado {
			closure, cerr := s.repo.GetProjectClosure(ctx, r.EpicID)
			if cerr == nil {
				pa.Encerramento = &ProjectClosure{
					Descricao:        closure.Descricao,
					DataEncerramento: closure.DataEncerramento,
					EncerradoPor:     closure.EncerradoPor,
				}
			}
		}

		result = append(result, pa)
	}

	return result, nil
}
```

- [ ] **Step 6: Rewrite GetProjectDetail**

Replace the entire method. Key changes: use `GetEpicByID` instead of `GetEpicsByEquipeAndProduto`, compute `pct_no_projeto`, populate avatar fields:

```go
func (s *AllocationService) GetProjectDetail(ctx context.Context, epicID, equipeID uuid.UUID) (*ProjectDetail, error) {
	epicRow, err := s.repo.GetEpicByID(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("getting epic: %w", err)
	}

	tasks, err := s.repo.GetEpicTasks(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("getting tasks: %w", err)
	}

	people, err := s.repo.GetEpicPeople(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("getting people: %w", err)
	}

	gdptcMap, _ := s.repo.CheckGDPTCAncestors(ctx, []uuid.UUID{epicID})

	var pctEstimado, pctPlanejado float64
	if epicRow.TotalFilhas > 0 {
		pctEstimado = float64(epicRow.FilhasComEstimativa) / float64(epicRow.TotalFilhas) * 100
	}
	if epicRow.HorasEstimadas > 0 {
		pctPlanejado = epicRow.HorasEmSprint / epicRow.HorasEstimadas * 100
	}
	status := "nao_planejado"
	if pctPlanejado >= 100 {
		status = "planejado"
		pctPlanejado = 100
	} else if pctPlanejado > 0 {
		status = "em_planejamento"
	}

	epic := ProjectAllocation{
		EpicID:            epicRow.EpicID,
		NumeroTicket:      epicRow.NumeroTicket,
		Resumo:            epicRow.Resumo,
		Apelido:           epicRow.Apelido,
		DataLimite:        epicRow.DataLimite,
		Prioridade:        epicRow.Prioridade,
		TipoDemanda:       epicRow.TipoDemanda,
		Produtos:          epicRow.Produtos,
		PctEstimado:       pctEstimado,
		PctPlanejado:      pctPlanejado,
		TarefasSemEst:     epicRow.TotalFilhas - epicRow.FilhasComEstimativa,
		TotalTarefas:      epicRow.TotalFilhas,
		IsGDPTC:           gdptcMap[epicID],
		Status:            status,
		ResponsavelNome:   epicRow.ResponsavelNome,
		ResponsavelAvatar: epicRow.ResponsavelAvatar,
		ResponsavelCargo:  epicRow.ResponsavelCargo,
	}

	membroIDs := make([]uuid.UUID, len(people))
	for i, p := range people {
		membroIDs[i] = p.MembroID
	}

	totalHoursMap, err := s.repo.GetPersonTotalAllocatedHours(ctx, membroIDs)
	if err != nil {
		s.logger.Warn("getting total allocated hours", zap.Error(err))
		totalHoursMap = make(map[uuid.UUID]float64)
	}

	pessoas := make([]PersonAllocation, 0, len(people))
	for _, p := range people {
		pctNoProjeto := 0.0
		totalHours := totalHoursMap[p.MembroID]
		if totalHours > 0 {
			pctNoProjeto = p.HorasNoProjeto / totalHours * 100
		}
		avatarURL := ""
		if p.AvatarURL != nil {
			avatarURL = *p.AvatarURL
		}
		pessoas = append(pessoas, PersonAllocation{
			MembroID:       p.MembroID,
			Nome:           p.Nome,
			HorasNoProjeto: p.HorasNoProjeto,
			HorasCapTotal:  totalHours,
			PctNoProjeto:   pctNoProjeto,
			AvatarURL:      avatarURL,
		})
	}

	var naoAlocadas, parciais, completas []TaskAllocation
	for _, t := range tasks {
		ta := taskRowToAllocation(t)
		hasEstimate := t.EstimativaTempo != nil && *t.EstimativaTempo > 0
		hasSprint := t.SprintID != nil
		hasPerson := t.ResponsavelID != nil

		if !hasEstimate || !hasSprint {
			naoAlocadas = append(naoAlocadas, ta)
		} else if !hasPerson {
			parciais = append(parciais, ta)
		} else {
			completas = append(completas, ta)
		}
	}

	return &ProjectDetail{
		Epic:        epic,
		Pessoas:     pessoas,
		NaoAlocadas: naoAlocadas,
		Parciais:    parciais,
		Completas:   completas,
	}, nil
}
```

- [ ] **Step 7: Update taskRowToAllocation**

Add `ResponsavelAvatar` field:

```go
func taskRowToAllocation(t repository.TaskAllocationRow) TaskAllocation {
	ta := TaskAllocation{
		TarefaID:          t.TarefaID,
		NumeroTicket:      t.NumeroTicket,
		Resumo:            t.Resumo,
		Tipo:              t.Tipo,
		TipoDemanda:       t.TipoDemanda,
		Status:            t.Status,
		SprintID:          t.SprintID,
		SprintNome:        t.SprintNome,
		SprintInicio:      t.SprintInicio,
		SprintFim:         t.SprintFim,
		ResponsavelID:     t.ResponsavelID,
		ResponsavelNome:   t.ResponsavelNome,
		ResponsavelAvatar: t.ResponsavelAvatar,
	}
	if t.EstimativaTempo != nil && *t.EstimativaTempo > 0 {
		h := float64(*t.EstimativaTempo) / 3600.0
		ta.EstimativaHoras = &h
	}
	return ta
}
```

- [ ] **Step 8: Add CloseProject method**

After `SyncProjectTasks`:

```go
func (s *AllocationService) CloseProject(ctx context.Context, epicID uuid.UUID, req CloseProjectRequest, encerradoPor string) error {
	dataEnc, err := time.Parse("2006-01-02", req.DataEncerramento)
	if err != nil {
		return fmt.Errorf("invalid date format: %w", err)
	}
	return s.repo.CloseProject(ctx, epicID, req.Descricao, dataEnc, encerradoPor)
}

func (s *AllocationService) ReopenProject(ctx context.Context, epicID uuid.UUID) error {
	return s.repo.ReopenProject(ctx, epicID)
}

func (s *AllocationService) GetFilteredProducts(ctx context.Context, equipeID uuid.UUID) ([]repository.ProdutoRow, error) {
	return s.repo.GetProdutosComProjetosAtivos(ctx, equipeID)
}
```

- [ ] **Step 9: Update test**

In `backend/internal/service/allocation_test.go`, update the call to `ListProjectAllocations` to pass the new `statusFilter` parameter:

Find any call to `svc.ListProjectAllocations(ctx, equipeID, produtoID)` and change to `svc.ListProjectAllocations(ctx, equipeID, produtoID, "ativos")`.

- [ ] **Step 10: Build and test**

Run: `cd backend && go build ./... && go vet ./... && go test ./...`

Expected: all pass.

---

### Task 3: Handler + Routes

**Files:**
- Modify: `backend/internal/handler/allocation.go`
- Modify: `backend/internal/handler/timeline.go`
- Modify: `backend/internal/domain/timeline.go`
- Modify: `backend/cmd/api/main.go`

**Interfaces:**
- Consumes (from Task 2):
  - `svc.ListProjectAllocations(ctx, equipeID, produtoID, statusFilter)`
  - `svc.CloseProject(ctx, epicID, req, encerradoPor)` — returns error
  - `svc.ReopenProject(ctx, epicID)` — returns error
  - `svc.GetFilteredProducts(ctx, equipeID)` — returns `[]repository.ProdutoRow`
  - `service.CloseProjectRequest{Descricao, DataEncerramento string}`
- Consumes (from Task 1):
  - `repo.SalvarEpicoEquipes(ctx, epicoID, equipeIDs)` — called from timeline handler
  - `repo.BuscarEpicoEquipes(ctx, epicoID)` — called from timeline handler
- Produces: HTTP endpoints accessible from frontend

- [ ] **Step 1: Modify ListProjects handler**

In `backend/internal/handler/allocation.go`, modify the `ListProjects` method to accept `status` query param and make `equipe_id` optional (empty = "todas", passes `uuid.Nil` to service):

```go
func (h *AllocationHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	var equipeID uuid.UUID
	equipeStr := r.URL.Query().Get("equipe_id")
	if equipeStr != "" {
		id, err := uuid.Parse(equipeStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid equipe_id")
			return
		}
		equipeID = id
	}

	produtoStr := r.URL.Query().Get("produto_id")
	if produtoStr == "" {
		respondError(w, http.StatusBadRequest, "produto_id is required")
		return
	}
	produtoID, err := uuid.Parse(produtoStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid produto_id")
		return
	}

	statusFilter := r.URL.Query().Get("status")
	if statusFilter == "" {
		statusFilter = "ativos"
	}

	result, err := h.svc.ListProjectAllocations(r.Context(), equipeID, produtoID, statusFilter)
	if err != nil {
		h.logger.Error("listing project allocations", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar projetos")
		return
	}
	respondJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 2: Add CloseProject handler**

After `ListSprints`:

```go
func (h *AllocationHandler) CloseProject(w http.ResponseWriter, r *http.Request) {
	epicID, err := uuid.Parse(chi.URLParam(r, "epicId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid epicId")
		return
	}

	var req service.CloseProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if req.Descricao == "" {
		respondError(w, http.StatusBadRequest, "descricao obrigatória")
		return
	}
	if req.DataEncerramento == "" {
		respondError(w, http.StatusBadRequest, "data_encerramento obrigatória")
		return
	}

	if err := h.svc.CloseProject(r.Context(), epicID, req, ""); err != nil {
		h.logger.Error("closing project", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao encerrar projeto")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "closed"})
}

func (h *AllocationHandler) ReopenProject(w http.ResponseWriter, r *http.Request) {
	epicID, err := uuid.Parse(chi.URLParam(r, "epicId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid epicId")
		return
	}

	if err := h.svc.ReopenProject(r.Context(), epicID); err != nil {
		h.logger.Error("reopening project", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao reabrir projeto")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "reopened"})
}

func (h *AllocationHandler) ListFilteredProducts(w http.ResponseWriter, r *http.Request) {
	var equipeID uuid.UUID
	equipeStr := r.URL.Query().Get("equipe_id")
	if equipeStr != "" {
		id, err := uuid.Parse(equipeStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid equipe_id")
			return
		}
		equipeID = id
	}

	result, err := h.svc.GetFilteredProducts(r.Context(), equipeID)
	if err != nil {
		h.logger.Error("listing filtered products", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar produtos")
		return
	}
	respondJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 3: Add EquipeIDs to MetadataProjetoRequest**

In `backend/internal/domain/timeline.go`, add `EquipeIDs` to `MetadataProjetoRequest`:

```go
type MetadataProjetoRequest struct {
	Apelido            *string     `json:"apelido"`
	DataInicioExecucao *time.Time  `json:"data_inicio_execucao"`
	DataLimite         *string     `json:"data_limite"`
	EquipeIDs          []uuid.UUID `json:"equipe_ids"`
}
```

Add `uuid` import: `"github.com/google/uuid"` (if not already present).

- [ ] **Step 4: Modify UpdateProjetoMetadata handler**

In `backend/internal/handler/timeline.go`, after the existing `AtualizarMetadataProjeto` call (line ~211), add equipe saving:

After the line `if err := h.store.AtualizarMetadataProjeto(...); err != nil { ... }`, add:

```go
	if req.EquipeIDs != nil {
		if err := h.store.SalvarEpicoEquipes(r.Context(), id, req.EquipeIDs); err != nil {
			h.logger.Error("failed to save epico equipes", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "falha ao salvar equipes do projeto")
			return
		}
	}
```

Note: `h.store` here is the `TimelineRepository` which embeds the pool — but `SalvarEpicoEquipes` is on `AllocationRepository`. The cleanest approach: add `SalvarEpicoEquipes` and `BuscarEpicoEquipes` as methods on `TimelineRepository` too (same SQL, same pool). Alternatively, add them to a shared interface. Since both repos use the same `*pgxpool.Pool`, just duplicate the methods on `TimelineRepository` in `timeline.go`.

Add to `backend/internal/repository/timeline.go` (these are identical to allocation.go versions):

```go
func (r *TimelineRepository) SalvarEpicoEquipes(ctx context.Context, epicoID uuid.UUID, equipeIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM epico_equipes WHERE epico_id = $1`, epicoID); err != nil {
		return fmt.Errorf("clearing epico equipes: %w", err)
	}

	for _, eqID := range equipeIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO epico_equipes (epico_id, equipe_id) VALUES ($1, $2)`, epicoID, eqID); err != nil {
			return fmt.Errorf("inserting epico equipe: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *TimelineRepository) BuscarEpicoEquipes(ctx context.Context, epicoID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `SELECT equipe_id FROM epico_equipes WHERE epico_id = $1`, epicoID)
	if err != nil {
		return nil, fmt.Errorf("querying epico equipes: %w", err)
	}
	defer rows.Close()

	var result []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning equipe id: %w", err)
		}
		result = append(result, id)
	}
	return result, rows.Err()
}
```

- [ ] **Step 5: Add GetProjetoEquipes handler**

In `backend/internal/handler/timeline.go`, add a new handler:

```go
func (h *TimelineHandler) GetProjetoEquipes(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	equipeIDs, err := h.store.BuscarEpicoEquipes(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to fetch epico equipes", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar equipes do projeto")
		return
	}
	if equipeIDs == nil {
		equipeIDs = []uuid.UUID{}
	}
	respondJSON(w, http.StatusOK, equipeIDs)
}
```

- [ ] **Step 6: Register new routes**

In `backend/cmd/api/main.go`, after the existing allocation routes (line 283), add:

```go
r.Post("/allocation/projects/{epicId}/close", allocHandler.CloseProject)
r.Delete("/allocation/projects/{epicId}/close", allocHandler.ReopenProject)
r.Get("/allocation/products", allocHandler.ListFilteredProducts)
```

After the existing projetos routes (line ~209), add:

```go
r.Get("/projetos/{id}/equipes", timelineHandler.GetProjetoEquipes)
```

- [ ] **Step 7: Build and test**

Run: `cd backend && go build ./... && go vet ./... && go test ./...`

Expected: all pass.

---

### Task 4: Frontend — CSS, Sidebar, Filters

**Files:**
- Modify: `frontend/index.html` (CSS section ~line 773-838, sidebar HTML ~line 870-901, navigate JS ~line 1731-1744, onAllocFilterChange ~line 5855, loadAlocacao ~line 5841)

**Interfaces:**
- Consumes: `GET /allocation/products?equipe_id=X` (from Task 3), `GET /equipes`, `GET /projetos/{id}/equipes`, `PUT /projetos/{id}/metadata` with `equipe_ids` field
- Produces: filter state passed to `loadProjectAllocations()`, sidebar reorganization, new CSS classes used by Tasks 5-6, projmeta modal with equipe checkboxes

- [ ] **Step 1: Add new CSS**

In `frontend/index.html`, before the closing `</style>` tag (line 839), add these new CSS rules:

```css
.alloc-avatar { width: 24px; height: 24px; border-radius: 50%; object-fit: cover; vertical-align: middle; }
.alloc-avatar-sm { width: 20px; height: 20px; border-radius: 50%; object-fit: cover; vertical-align: middle; margin-right: 4px; }
.alloc-avatar-xs { width: 16px; height: 16px; border-radius: 50%; object-fit: cover; vertical-align: middle; margin-right: 3px; }
.alloc-avatar-placeholder { display: inline-flex; align-items: center; justify-content: center; border-radius: 50%; background: var(--border); color: var(--text-secondary); font-size: 10px; font-weight: 600; vertical-align: middle; }
.alloc-responsavel { display: flex; align-items: center; gap: 6px; font-size: 13px; color: var(--text-secondary); margin-top: 4px; }
.alloc-cargo { font-size: 11px; color: var(--text-secondary); opacity: .7; }
.alloc-badge-gdptc { background: rgba(13,124,102,.15); color: var(--accent); }
.alloc-badge-atrasado { background: #ef4444; color: #fff; }
.alloc-badge-proximo { background: #eab308; color: #1a1a1a; }
.alloc-badge-encerrado { background: rgba(156,163,175,.15); color: #9ca3af; }
.alloc-box.encerrado { opacity: .6; }
.alloc-tipo-section { margin-bottom: 28px; }
.alloc-tipo-header { font-size: 15px; font-weight: 600; color: var(--text-primary); margin-bottom: 12px; padding-bottom: 6px; border-bottom: 2px solid var(--border); display: flex; align-items: center; gap: 8px; }
.alloc-tipo-count { font-size: 12px; font-weight: 400; color: var(--text-secondary); }
.alloc-box-encerrar { position: absolute; top: 8px; right: 8px; background: none; border: none; cursor: pointer; font-size: 14px; padding: 2px 6px; border-radius: 4px; color: var(--text-secondary); }
.alloc-box-encerrar:hover { background: var(--border); color: var(--text-primary); }
.alloc-encerrar-form { position: absolute; top: 0; left: 0; right: 0; bottom: 0; background: var(--surface); border-radius: 10px; padding: 16px; display: flex; flex-direction: column; gap: 10px; z-index: 5; border: 1px solid var(--border); }
.alloc-encerrar-form textarea { width: 100%; min-height: 60px; padding: 8px; border-radius: 6px; border: 1px solid var(--border); background: var(--surface); color: var(--text-primary); font-size: 13px; resize: vertical; box-sizing: border-box; }
.alloc-encerrar-form input[type="date"] { padding: 6px 10px; border-radius: 6px; border: 1px solid var(--border); background: var(--surface); color: var(--text-primary); font-size: 13px; }
.alloc-encerrar-form .alloc-encerrar-actions { display: flex; gap: 8px; justify-content: flex-end; }
.alloc-encerrar-form .alloc-encerrar-actions button { padding: 6px 14px; border-radius: 6px; font-size: 13px; cursor: pointer; border: none; }
.alloc-gantt-deadline { position: absolute; top: 0; bottom: 0; width: 2px; border-left: 2px dashed #ef4444; z-index: 2; pointer-events: none; }
.alloc-gantt-deadline-label { position: absolute; top: -18px; left: 4px; font-size: 10px; color: #ef4444; white-space: nowrap; }
.alloc-deadline-badges { display: flex; gap: 4px; flex-wrap: wrap; margin-top: 4px; }
.projmeta-equipes { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; }
.projmeta-equipes label { display: flex; align-items: center; gap: 4px; font-size: 13px; color: var(--text-primary); cursor: pointer; padding: 4px 8px; border-radius: 6px; border: 1px solid var(--border); }
.projmeta-equipes label:has(input:checked) { background: rgba(13,124,102,.15); border-color: var(--accent); }
```

- [ ] **Step 2: Reorganize sidebar**

In `frontend/index.html`, replace the standalone Projetos button (lines 874-877) AND the Alocação item inside sidebar-group-relatorios (lines 897-900) with a new Projetos group:

Replace lines 874-877 (`<button class="sidebar-item" data-page="projetos"...>...</button>`) with:

```html
    <div class="sidebar-group" id="sidebar-group-projetos">
      <button class="sidebar-group-header" title="Projetos" onclick="toggleSidebarGroup('projetos')">
        <svg viewBox="0 0 24 24"><path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7l-2-2H4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2z"/></svg>
        <span class="sidebar-item-label">Projetos</span>
        <span class="sidebar-arrow">▶</span>
      </button>
      <div class="sidebar-group-items">
        <button class="sidebar-item" data-page="projetos" title="Lista de Projetos" onclick="navigate('projetos')">
          <svg viewBox="0 0 24 24"><path d="M4 6h16M4 12h16M4 18h16" stroke-width="2"/></svg>
          <span class="sidebar-item-label">Lista de Projetos</span>
        </button>
        <button class="sidebar-item" data-page="alocacao" title="Alocação" onclick="navigate('alocacao')">
          <svg viewBox="0 0 24 24"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>
          <span class="sidebar-item-label">Alocação</span>
        </button>
      </div>
    </div>
```

Then remove the Alocação button from the Relatórios group (the `<button class="sidebar-item" data-page="alocacao"...>` block inside `sidebar-group-relatorios`, lines 897-900). Delete those 4 lines.

- [ ] **Step 3: Update navigate() function**

In `frontend/index.html`, update the `navigate` function (around line 1731):

Replace:
```js
  var reportPages = ['timeline', 'sprints-timeline', 'alocacao'];
  const group = document.getElementById('sidebar-group-relatorios');
  if (group && reportPages.includes(page)) { group.classList.add('open'); }
```

With:
```js
  var reportPages = ['timeline', 'sprints-timeline'];
  var group = document.getElementById('sidebar-group-relatorios');
  if (group && reportPages.indexOf(page) >= 0) { group.classList.add('open'); }
  var projPages = ['projetos', 'alocacao'];
  var projGroup = document.getElementById('sidebar-group-projetos');
  if (projGroup && projPages.indexOf(page) >= 0) { projGroup.classList.add('open'); }
```

- [ ] **Step 4: Add status filter to HTML**

In `frontend/index.html`, in the `#page-alocacao` section (around line 1023), add the status select after the equipe select:

Replace the alloc-filters div:
```html
      <div class="alloc-filters">
        <select id="alloc-equipe" onchange="onAllocFilterChange()">
          <option value="">Selecione a Equipe</option>
        </select>
        <select id="alloc-produto" onchange="onAllocFilterChange()">
          <option value="">Selecione o Produto</option>
        </select>
      </div>
```

With:
```html
      <div class="alloc-filters">
        <select id="alloc-equipe" onchange="onAllocFilterChange()">
          <option value="">Selecione a Equipe</option>
          <option value="todas">Todas as Equipes</option>
        </select>
        <select id="alloc-status" onchange="onAllocFilterChange()">
          <option value="ativos">Ativos</option>
          <option value="encerrados">Encerrados</option>
          <option value="todos">Todos</option>
        </select>
        <select id="alloc-produto" onchange="onAllocFilterChange()">
          <option value="">Selecione o Produto</option>
        </select>
      </div>
```

Note: "Todas as Equipes" has value `"todas"` — the frontend passes empty string as equipe_id to the backend when this is selected (see onAllocFilterChange below).

- [ ] **Step 5: Update onAllocFilterChange**

Replace the entire `onAllocFilterChange` function:

```js
function onAllocFilterChange() {
  var rawEquipe = document.getElementById('alloc-equipe').value;
  allocEquipeId = rawEquipe === 'todas' ? '' : rawEquipe;
  allocProdutoId = document.getElementById('alloc-produto').value;
  var equipeSelected = rawEquipe !== '';

  if (equipeSelected && !allocProdutoId) {
    var prodSel = document.getElementById('alloc-produto');
    prodSel.innerHTML = '<option value="">Selecione o Produto</option>';
    var prodUrl = '/allocation/products' + (allocEquipeId ? '?equipe_id=' + allocEquipeId : '');
    api(prodUrl).then(function(produtos) {
      produtos.forEach(function(p) {
        var opt = document.createElement('option');
        opt.value = p.id;
        opt.textContent = p.nome;
        prodSel.appendChild(opt);
      });
    });
  }

  if (equipeSelected && allocProdutoId) {
    loadProjectAllocations();
  } else {
    document.getElementById('alloc-content').innerHTML =
      '<div class="alloc-empty">Selecione equipe e produto para ver os projetos.</div>';
  }
}
```

Note: when "Todas as Equipes" is selected, `rawEquipe = 'todas'`, `allocEquipeId = ''` (empty). The API call to `/allocation/products` has no `equipe_id` param → backend receives empty equipe_id → passes `uuid.Nil` → returns all products. Similarly `loadProjectAllocations` sends empty equipe_id → backend returns projects from all teams.

- [ ] **Step 6: Update loadProjectAllocations**

Replace the entire function to pass status filter:

```js
function loadProjectAllocations() {
  var container = document.getElementById('alloc-content');
  container.innerHTML = '<div class="alloc-empty">Carregando...</div>';
  var statusFilter = document.getElementById('alloc-status').value || 'ativos';
  var url = '/allocation/projects?produto_id=' + allocProdutoId + '&status=' + statusFilter;
  if (allocEquipeId) url += '&equipe_id=' + allocEquipeId;

  api(url)
    .then(function(projects) {
      if (!projects || projects.length === 0) {
        container.innerHTML = '<div class="alloc-empty">Nenhum projeto encontrado para esta equipe e produto.</div>';
        return;
      }
      renderAllocationBoxes(projects, container);
    })
    .catch(function(err) {
      container.innerHTML = '<div class="alloc-empty">Erro ao carregar projetos.</div>';
    });
}
```

- [ ] **Step 7: Add equipe checkboxes to projmeta modal**

In `frontend/index.html`, modify the `projmeta-modal` form (around line 1121). Add a new form-group after the Data Limite field:

After the line `<div class="form-group"><label class="form-label">Data Limite</label><input ...></div>`:

```html
      <div class="form-group"><label class="form-label">Equipes</label><div class="projmeta-equipes" id="projmeta-equipes"></div></div>
```

- [ ] **Step 8: Update openProjMetaModal**

Replace the entire `openProjMetaModal` function to also load equipes and current assignments:

```js
function openProjMetaModal(id, ticket, apelido, dataInicio, dataLimite) {
  document.getElementById('projmeta-id').value = id;
  document.getElementById('projmeta-title').textContent = 'Editar — ' + ticket;
  document.getElementById('projmeta-apelido').value = apelido;
  document.getElementById('projmeta-data-inicio').value = fmtDateBR(dataInicio);
  document.getElementById('projmeta-data-limite').value = fmtDateBR(dataLimite);
  document.getElementById('projmeta-modal').classList.add('open');

  var container = document.getElementById('projmeta-equipes');
  container.innerHTML = 'Carregando...';

  Promise.all([
    api('/equipes'),
    api('/projetos/' + id + '/equipes')
  ]).then(function(results) {
    var equipes = results[0];
    var assigned = results[1] || [];
    var assignedSet = {};
    assigned.forEach(function(eqId) { assignedSet[eqId] = true; });

    var html = '';
    equipes.forEach(function(eq) {
      var checked = assignedSet[eq.id] ? ' checked' : '';
      html += '<label><input type="checkbox" name="projmeta-eq" value="' + eq.id + '"' + checked + '> ' + esc(eq.nome) + '</label>';
    });
    container.innerHTML = html;
  });
}
```

- [ ] **Step 9: Update handleProjMetaSubmit**

Replace the entire `handleProjMetaSubmit` function to include equipe_ids:

```js
function handleProjMetaSubmit(e) {
  e.preventDefault();
  var id = document.getElementById('projmeta-id').value;
  var body = {};
  var apelido = document.getElementById('projmeta-apelido').value.trim();
  if (apelido) body.apelido = apelido;
  var dataInicioRaw = document.getElementById('projmeta-data-inicio').value;
  if (dataInicioRaw) { var iso = parseDateBR(dataInicioRaw); body.data_inicio_execucao = new Date(iso + 'T12:00:00').toISOString(); }
  var dataLimiteRaw = document.getElementById('projmeta-data-limite').value;
  if (dataLimiteRaw) { body.data_limite = parseDateBR(dataLimiteRaw); }

  var equipeIds = [];
  var checkboxes = document.querySelectorAll('#projmeta-equipes input[name="projmeta-eq"]:checked');
  for (var i = 0; i < checkboxes.length; i++) {
    equipeIds.push(checkboxes[i].value);
  }
  body.equipe_ids = equipeIds;

  api('/projetos/' + id + '/metadata', { method: 'PUT', body: JSON.stringify(body) })
    .then(function() {
      closeProjMetaModal();
      loadProjetos();
    })
    .catch(function(err) { alert('Erro: ' + err.message); });
  return false;
}
```

Note: this replaces the `async` version with a `var`/`function` only version (no ES6+). The `equipe_ids` array is always sent (even empty) so backend clears all assignments if none selected.

- [ ] **Step 10: Verify JS syntax**

Run: `node --check <(sed -n '1640,6260p' frontend/index.html)` (adjust line range to match the `<script>` block).

Expected: no syntax errors.

---

### Task 5: Frontend — Boxes (tipo_demanda, encerrar, deadline badges)

**Files:**
- Modify: `frontend/index.html` (renderAllocationBoxes, new helper functions)

**Interfaces:**
- Consumes: `POST /allocation/projects/{epicId}/close` and `DELETE /allocation/projects/{epicId}/close` (from Task 3). CSS classes from Task 4.
- Produces: rendered allocation boxes grouped by tipo_demanda, with encerrar/reabrir buttons and deadline badges

- [ ] **Step 1: Add helper functions**

Before `renderAllocationBoxes` (around line 5903), add:

```js
function firstName(nome) {
  if (!nome) return '--';
  var parts = nome.split(' ');
  return parts[0];
}

function avatarHtml(url, nome, sizeClass) {
  if (url) {
    return '<img class="' + sizeClass + '" src="' + escAttr(url) + '" title="' + escAttr(nome || '') + '">';
  }
  var initial = (nome || '?').charAt(0).toUpperCase();
  var sz = sizeClass === 'alloc-avatar' ? '24px' : sizeClass === 'alloc-avatar-sm' ? '20px' : '16px';
  return '<span class="alloc-avatar-placeholder" style="width:' + sz + ';height:' + sz + '" title="' + escAttr(nome || '') + '">' + initial + '</span>';
}

function deadlineBadgeHtml(dataLimite, hasPending) {
  if (!dataLimite || !hasPending) return '';
  var now = new Date();
  var limite = new Date(dataLimite);
  var dias = Math.ceil((limite - now) / 86400000);
  var formatted = limite.toLocaleDateString('pt-BR');
  if (dias < 0) {
    return '<span class="alloc-badge alloc-badge-atrasado">Em Atraso - Data Limite: ' + formatted + '</span>';
  }
  if (dias <= 30) {
    return '<span class="alloc-badge alloc-badge-proximo">Data limite próxima - ' + formatted + '</span>';
  }
  return '';
}

var cargoLabels = {
  'coordenador_desenvolvimento': 'Coord. Dev',
  'po_produto': 'PO',
  'gerente_tecnologia': 'Ger. Tecnologia',
  'gerente_executivo': 'Ger. Executivo',
  'scrum_master': 'Scrum Master',
  'agile_master': 'Agile Master',
  'desenvolvedor': 'Dev'
};
```

- [ ] **Step 2: Rewrite renderAllocationBoxes**

Replace the entire function with tipo_demanda sections, encerrar button, and deadline badges:

```js
function renderAllocationBoxes(projects, container) {
  var groups = {Meta: [], Compromisso: [], Iniciativa: []};
  projects.forEach(function(p) {
    var tipo = p.tipo_demanda || 'Iniciativa';
    if (tipo === 'Meta') groups.Meta.push(p);
    else if (tipo === 'Compromisso') groups.Compromisso.push(p);
    else groups.Iniciativa.push(p);
  });

  var sectionNames = [
    {key: 'Meta', label: 'Metas'},
    {key: 'Compromisso', label: 'Compromissos'},
    {key: 'Iniciativa', label: 'Iniciativas'}
  ];

  var html = '';
  sectionNames.forEach(function(sec) {
    var items = groups[sec.key];
    if (items.length === 0) return;

    html += '<div class="alloc-tipo-section">';
    html += '<div class="alloc-tipo-header">' + sec.label + ' <span class="alloc-tipo-count">(' + items.length + ')</span></div>';
    html += '<div class="alloc-grid">';

    items.forEach(function(p) {
      var color = getAllocColor(p.pct_planejado);
      var title = p.apelido ? esc(p.apelido) : esc(p.resumo);
      var dataLimite = p.data_limite ? new Date(p.data_limite).toLocaleDateString('pt-BR') : '--';
      var encerradoClass = p.encerrado ? ' encerrado' : '';

      html += '<div class="alloc-box' + encerradoClass + '" onclick="openProjectModal(\'' + p.epic_id + '\')" style="border-left: 4px solid ' + color + '">';

      if (!p.encerrado) {
        html += '<button class="alloc-box-encerrar" onclick="event.stopPropagation();showEncerrarForm(\'' + p.epic_id + '\',this)" title="Encerrar Projeto">🔒</button>';
      } else {
        html += '<button class="alloc-box-encerrar" onclick="event.stopPropagation();reabrirProjeto(\'' + p.epic_id + '\')" title="Reabrir Projeto">🔓</button>';
      }

      html += '<div class="alloc-box-header">';
      if (p.is_gdptc) {
        html += '<span class="alloc-box-star" title="Projeto do Portfólio Unificado">★</span>';
      }
      html += '<span class="alloc-box-ticket">' + esc(p.numero_ticket) + '</span>';
      html += '</div>';
      html += '<div class="alloc-box-title" title="' + escAttr(p.resumo) + '">' + title + '</div>';
      html += '<div class="alloc-box-meta">Limite: ' + dataLimite + '</div>';

      if (p.produtos && p.produtos.length > 0) {
        html += '<div class="alloc-box-produtos">';
        p.produtos.forEach(function(prod) {
          html += '<span class="alloc-box-produto">' + esc(prod) + '</span>';
        });
        html += '</div>';
      }

      html += '<div class="alloc-bar-group">';
      html += '<div class="alloc-bar-label"><span>Estimado</span><span>' + Math.round(p.pct_estimado) + '%</span></div>';
      html += '<div class="alloc-bar"><div class="alloc-bar-fill" style="width:' + Math.min(p.pct_estimado, 100) + '%;background:' + getAllocColor(p.pct_estimado) + '"></div></div>';
      html += '<div class="alloc-bar-label"><span>Planejado</span><span>' + Math.round(p.pct_planejado) + '%</span></div>';
      html += '<div class="alloc-bar"><div class="alloc-bar-fill" style="width:' + Math.min(p.pct_planejado, 100) + '%;background:' + color + '"></div></div>';
      html += '</div>';

      if (p.tarefas_sem_estimativa > 0) {
        html += '<div class="alloc-alert">⚠ ' + p.tarefas_sem_estimativa + ' tarefas sem estimativa</div>';
      }

      var badgeClass = 'alloc-badge-' + p.status;
      var badgeText = p.status === 'planejado' ? 'Planejado' : p.status === 'em_planejamento' ? 'Em Planejamento' : 'Não Planejado';
      html += '<div class="alloc-deadline-badges">';
      if (p.encerrado) {
        html += '<span class="alloc-badge alloc-badge-encerrado">Encerrado</span>';
      } else {
        html += '<span class="alloc-badge ' + badgeClass + '">' + badgeText + '</span>';
      }
      var hasPending = p.tarefas_sem_estimativa > 0 || p.pct_planejado < 100;
      html += deadlineBadgeHtml(p.data_limite, hasPending);
      html += '</div>';

      html += '</div>';
    });

    html += '</div></div>';
  });

  container.innerHTML = html;
}
```

- [ ] **Step 3: Add encerrar/reabrir functions**

After `renderAllocationBoxes`:

```js
function showEncerrarForm(epicId, btn) {
  var box = btn.closest('.alloc-box');
  var today = new Date().toISOString().split('T')[0];
  var form = document.createElement('div');
  form.className = 'alloc-encerrar-form';
  form.onclick = function(e) { e.stopPropagation(); };
  form.innerHTML = '<div style="font-weight:600;font-size:14px;color:var(--text-primary)">Encerrar Projeto</div>' +
    '<textarea id="enc-desc-' + epicId + '" placeholder="Descrição do encerramento..."></textarea>' +
    '<input type="date" id="enc-date-' + epicId + '" value="' + today + '">' +
    '<div class="alloc-encerrar-actions">' +
    '<button style="background:var(--border);color:var(--text-primary)" onclick="this.closest(\'.alloc-encerrar-form\').remove()">Cancelar</button>' +
    '<button style="background:#ef4444;color:#fff" onclick="confirmarEncerramento(\'' + epicId + '\')">Confirmar</button>' +
    '</div>';
  box.appendChild(form);
}

function confirmarEncerramento(epicId) {
  var desc = document.getElementById('enc-desc-' + epicId).value;
  var date = document.getElementById('enc-date-' + epicId).value;
  if (!desc) { alert('Informe a descrição do encerramento.'); return; }
  if (!date) { alert('Informe a data do encerramento.'); return; }

  fetch('/api/v1/allocation/projects/' + epicId + '/close', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({descricao: desc, data_encerramento: date})
  }).then(function(resp) {
    if (!resp.ok) throw new Error('Erro ao encerrar');
    loadProjectAllocations();
  }).catch(function(err) {
    alert('Erro: ' + err.message);
  });
}

function reabrirProjeto(epicId) {
  if (!confirm('Deseja reabrir este projeto?')) return;
  fetch('/api/v1/allocation/projects/' + epicId + '/close', {
    method: 'DELETE'
  }).then(function(resp) {
    if (!resp.ok) throw new Error('Erro ao reabrir');
    loadProjectAllocations();
  }).catch(function(err) {
    alert('Erro: ' + err.message);
  });
}
```

- [ ] **Step 4: Verify JS syntax**

Run: `node --check <(sed -n '1640,6320p' frontend/index.html)` (adjust range).

Expected: no syntax errors.

---

### Task 6: Frontend — Modal + Gantt

**Files:**
- Modify: `frontend/index.html` (renderAllocModal, renderAllocGantt, renderAllocTaskEditable, alloc-task-readonly section)

**Interfaces:**
- Consumes: helper functions `firstName`, `avatarHtml`, `deadlineBadgeHtml`, `cargoLabels` (from Task 5). Backend returns `responsavel_nome`, `responsavel_avatar`, `responsavel_cargo`, `avatar_url`, `is_gdptc`, `data_limite` (from Tasks 1-3).
- Produces: fully rendered modal with responsável, badges, photos, deadline line in Gantt

- [ ] **Step 1: Rewrite renderAllocModal**

Replace the entire `renderAllocModal` function:

```js
function renderAllocModal(detail) {
  var modal = document.querySelector('.alloc-modal');
  if (!modal) return;

  var epic = detail.epic;
  var badgeClass = 'alloc-badge-' + epic.status;
  var badgeText = epic.status === 'planejado' ? 'Planejado' : epic.status === 'em_planejamento' ? 'Em Planejamento' : 'Não Planejado';

  var html = '<div class="alloc-modal-header">';
  html += '<div>';
  html += '<h2>' + esc(epic.numero_ticket) + ': ' + esc(epic.apelido || epic.resumo) + ' <span class="alloc-badge ' + badgeClass + '">' + badgeText + '</span>';
  if (epic.is_gdptc) {
    html += ' <span class="alloc-badge alloc-badge-gdptc">★ Projeto do Portfólio Unificado</span>';
  }
  var hasPending = (detail.nao_alocadas && detail.nao_alocadas.length > 0) || (detail.parciais && detail.parciais.length > 0);
  html += ' ' + deadlineBadgeHtml(epic.data_limite, hasPending);
  html += '</h2>';

  if (epic.responsavel_nome) {
    var cargoText = cargoLabels[epic.responsavel_cargo] || epic.responsavel_cargo || '';
    html += '<div class="alloc-responsavel">';
    html += avatarHtml(epic.responsavel_avatar, epic.responsavel_nome, 'alloc-avatar');
    html += ' <span title="' + escAttr(epic.responsavel_nome) + '">' + esc(firstName(epic.responsavel_nome)) + '</span>';
    if (cargoText) html += ' <span class="alloc-cargo">' + esc(cargoText) + '</span>';
    html += '</div>';
  }

  html += '</div>';
  html += '<div style="display:flex;gap:8px;align-items:center">';
  html += '<button class="alloc-sync-btn" id="alloc-sync-btn" onclick="syncProjectTasks(\'' + epic.epic_id + '\')"><span id="alloc-sync-icon">🔄</span> Sincronizar Tarefas</button>';
  html += '<button class="alloc-modal-close" onclick="closeAllocModal()">✕</button>';
  html += '</div></div>';

  // People section
  if (detail.pessoas && detail.pessoas.length > 0) {
    html += '<div class="alloc-section"><div class="alloc-section-title">Equipe Envolvida</div>';
    html += '<table class="alloc-people-table"><thead><tr><th>Nome</th><th>Horas no Projeto</th><th>% no Projeto</th></tr></thead><tbody>';
    detail.pessoas.forEach(function(p) {
      html += '<tr><td title="' + escAttr(p.nome) + '">';
      html += avatarHtml(p.avatar_url, p.nome, 'alloc-avatar-sm');
      html += ' ' + esc(firstName(p.nome));
      html += '</td><td>' + p.horas_no_projeto.toFixed(1) + 'h</td><td>' + (p.pct_no_projeto || 0).toFixed(0) + '%</td></tr>';
    });
    html += '</tbody></table></div>';
  }

  // Unallocated tasks
  if (detail.nao_alocadas && detail.nao_alocadas.length > 0) {
    html += '<div class="alloc-section"><div class="alloc-section-title">Tarefas Não Alocadas (' + detail.nao_alocadas.length + ')</div>';
    detail.nao_alocadas.forEach(function(t) {
      html += renderAllocTaskEditable(t);
    });
    html += '</div>';
  }

  // Partial tasks
  if (detail.parciais && detail.parciais.length > 0) {
    html += '<div class="alloc-section"><div class="alloc-section-title">Estimadas sem Pessoa (' + detail.parciais.length + ')</div>';
    detail.parciais.forEach(function(t) {
      html += renderAllocTaskEditable(t);
    });
    html += '</div>';
  }

  // Planned tasks (renamed from "Tarefas Completas")
  if (detail.completas && detail.completas.length > 0) {
    html += '<div class="alloc-section"><div class="alloc-section-title">Tarefas Planejadas (' + detail.completas.length + ')</div>';
    detail.completas.forEach(function(t) {
      html += '<div class="alloc-task-readonly">';
      html += '<span class="alloc-task-ticket">' + esc(t.numero_ticket) + '</span>';
      html += '<span class="alloc-task-resumo" title="' + escAttr(t.resumo) + '">' + esc(t.resumo) + '</span>';
      html += '<span>' + (t.estimativa_horas ? t.estimativa_horas.toFixed(1) + 'h' : '--') + '</span>';
      html += '<span>' + esc(t.sprint_nome || '--') + '</span>';
      html += '<span title="' + escAttr(t.responsavel_nome || '') + '">';
      html += avatarHtml(t.responsavel_avatar, t.responsavel_nome, 'alloc-avatar-xs');
      html += ' ' + esc(firstName(t.responsavel_nome));
      html += '</span>';
      html += '</div>';
    });
    html += '</div>';
  }

  // Gantt
  html += '<div class="alloc-section" id="alloc-gantt-section"></div>';

  modal.innerHTML = html;
  renderAllocGantt(detail);
  setTimeout(loadAllocMembers, 0);
}
```

- [ ] **Step 2: Update renderAllocTaskEditable**

Replace the function to add avatar display for the current responsável:

```js
function renderAllocTaskEditable(t) {
  var tid = t.tarefa_id;
  var sprintVal = t.sprint_id || '';
  var estVal = t.estimativa_horas ? t.estimativa_horas.toFixed(1) : '';

  var html = '<div class="alloc-task-row">';
  html += '<span class="alloc-task-ticket">' + esc(t.numero_ticket) + '</span>';
  html += '<span class="alloc-task-resumo" title="' + escAttr(t.resumo) + '">' + esc(t.resumo) + '</span>';
  html += '<div class="alloc-task-controls">';

  // Sprint select
  html += '<select id="alloc-sprint-' + tid + '">';
  html += '<option value="">Sprint</option>';
  allocSprints.forEach(function(s) {
    var sel = s.id === sprintVal ? ' selected' : '';
    html += '<option value="' + s.id + '"' + sel + '>' + esc(s.nome) + '</option>';
  });
  html += '</select>';

  // Person select — loaded from equipe members
  html += '<select id="alloc-person-' + tid + '">';
  html += '<option value="">Pessoa</option>';
  html += '</select>';

  // Estimate input
  html += '<input type="number" id="alloc-est-' + tid + '" placeholder="Horas" step="0.5" min="0.5" value="' + estVal + '">';

  // Current assignee indicator
  if (t.responsavel_nome) {
    html += '<span title="' + escAttr(t.responsavel_nome) + '" style="font-size:12px">';
    html += avatarHtml(t.responsavel_avatar, t.responsavel_nome, 'alloc-avatar-xs');
    html += '</span>';
  }

  // Allocate button
  html += '<button class="alloc-task-btn" onclick="allocateTask(\'' + tid + '\')">✓</button>';

  html += '</div></div>';
  return html;
}
```

- [ ] **Step 3: Add deadline line to Gantt**

Replace the entire `renderAllocGantt` function:

```js
function renderAllocGantt(detail) {
  var section = document.getElementById('alloc-gantt-section');
  if (!section) return;

  var allTasks = (detail.completas || []).concat(detail.parciais || []);
  var unallocated = detail.nao_alocadas || [];

  if (allTasks.length === 0 && unallocated.length === 0) {
    section.innerHTML = '';
    return;
  }

  var year = new Date().getFullYear();
  var yearStart = new Date(year, 0, 1).getTime();
  var yearEnd = new Date(year, 11, 31).getTime();
  var yearRange = yearEnd - yearStart;

  var months = ['Jan', 'Fev', 'Mar', 'Abr', 'Mai', 'Jun', 'Jul', 'Ago', 'Set', 'Out', 'Nov', 'Dez'];

  var html = '<div class="alloc-section-title">Timeline do Projeto</div>';
  html += '<div class="alloc-gantt"><div class="alloc-gantt-container">';

  // Header with months
  html += '<div class="alloc-gantt-header">';
  html += '<div style="font-size:11px;color:var(--text-secondary);padding:4px 6px">' + year + '</div>';
  html += '<div class="alloc-gantt-months" style="position:relative">';
  months.forEach(function(m) { html += '<div class="alloc-gantt-month">' + m + '</div>'; });

  // Deadline line in header
  if (detail.epic.data_limite) {
    var deadlineDate = new Date(detail.epic.data_limite).getTime();
    var deadlineLeft = (deadlineDate - yearStart) / yearRange * 100;
    if (deadlineLeft >= 0 && deadlineLeft <= 100) {
      var formatted = new Date(detail.epic.data_limite).toLocaleDateString('pt-BR');
      html += '<div class="alloc-gantt-deadline" style="left:' + deadlineLeft + '%">';
      html += '<div class="alloc-gantt-deadline-label">Limite: ' + formatted + '</div>';
      html += '</div>';
    }
  }

  html += '</div></div>';

  // Allocated tasks
  allTasks.forEach(function(t) {
    if (!t.sprint_inicio || !t.sprint_fim) return;
    var start = new Date(t.sprint_inicio).getTime();
    var end = new Date(t.sprint_fim).getTime();
    var left = Math.max(0, (start - yearStart) / yearRange * 100);
    var width = Math.max(1, (end - start) / yearRange * 100);

    html += '<div class="alloc-gantt-row">';
    html += '<div class="alloc-gantt-label" title="' + escAttr(t.resumo) + '">' + esc(t.numero_ticket) + '</div>';
    html += '<div class="alloc-gantt-timeline" style="position:relative">';

    // Repeat deadline line in timeline area
    if (detail.epic.data_limite) {
      var dlDate = new Date(detail.epic.data_limite).getTime();
      var dlLeft = (dlDate - yearStart) / yearRange * 100;
      if (dlLeft >= 0 && dlLeft <= 100) {
        html += '<div class="alloc-gantt-deadline" style="left:' + dlLeft + '%"></div>';
      }
    }

    html += '<div class="alloc-gantt-bar alloc-gantt-bar-allocated" style="left:' + left + '%;width:' + width + '%" title="' + escAttr(t.numero_ticket + ' - ' + t.resumo + ' (' + (t.sprint_nome || '') + ')') + '"></div>';
    html += '</div></div>';
  });

  // Separator
  if (unallocated.length > 0) {
    html += '<div class="alloc-gantt-separator">Não Alocadas</div>';
    unallocated.forEach(function(t) {
      html += '<div class="alloc-gantt-row">';
      html += '<div class="alloc-gantt-label" title="' + escAttr(t.resumo) + '">' + esc(t.numero_ticket) + '</div>';
      html += '<div class="alloc-gantt-timeline">';
      html += '<div class="alloc-gantt-bar alloc-gantt-bar-unallocated" style="left:0%;width:100%" title="' + escAttr(t.numero_ticket + ' - ' + t.resumo + ' — Não Alocada') + '"></div>';
      html += '</div></div>';
    });
  }

  html += '</div></div>';
  section.innerHTML = html;
}
```

- [ ] **Step 4: Verify JS syntax**

Run: `node --check <(sed -n '1640,6350p' frontend/index.html)` (adjust range).

Expected: no syntax errors.

- [ ] **Step 5: Full backend build + test**

Run: `cd backend && go build ./... && go vet ./... && go test ./...`

Expected: all pass.
