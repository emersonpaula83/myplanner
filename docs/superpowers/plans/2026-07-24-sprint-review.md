# Sprint Review Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Review" tab to sprint detail with stats cards, pie charts, editable highlights (destaques), task table, and PDF/image export.

**Architecture:** Single `GET /api/sprints/{id}/review` endpoint returns all review data (stats, charts, tasks). Separate CRUD endpoints for destaques. Frontend adds tab navigation (Acompanhamento/Review) inside existing sprint detail, reuses `drawPieChart`. Export via inlined html2canvas + jsPDF.

**Tech Stack:** Go (pgx/v5, chi, zap), vanilla JS SPA (frontend/index.html), PostgreSQL

## Global Constraints

- Excluded statuses: `Cancelado`, `Rejeitada` — never counted in any stat or chart
- Em Andamento statuses: `Desenvolvimento`, `Deploy`, `Code Review`, `Teste`, `Validação do Solicitante`
- Unplanned detection: `t.data_entrada_sprint > s.data_inicio OR (t.data_entrada_sprint IS NULL AND t.data_criacao > s.data_inicio)` — same as existing codebase
- Type classification: Bug/Incidente = manutenção; GDPTC ancestor = novos_projetos; Melhoria/História without GDPTC = melhorias; everything else = outros
- GDPTC ancestor: recursive `parent_id` chain, any ancestor with `numero_ticket LIKE 'GDPTC-%'`
- Products = JIRA Components, already synced via `tarefa_produtos` join table
- Relator join: `LEFT JOIN membros m ON m.id = t.relator_id` → `m.nome`
- PO: `membros.cargo = 'po_produto'` joined via `membro_produtos`
- `drawPieChart(wrapEl, slices)` — slices need `.horas` (value) and `.label` properties
- Repository pattern: `pool *pgxpool.Pool`, receiver `r`, errors wrapped with `fmt.Errorf`
- Handler pattern: `SprintStore` interface, `respondJSON`/`respondError` helpers
- Service: `repo *repository.XxxRepository`, orchestrates repo calls
- Migration naming: `000015_sprint_review_destaques.{up,down}.sql`

## File Structure

```
backend/migrations/
  000015_sprint_review_destaques.up.sql    (CREATE TABLE)
  000015_sprint_review_destaques.down.sql  (DROP TABLE)

backend/internal/repository/
  review.go          (ReviewRepository — all SQL queries)

backend/internal/service/
  review.go          (ReviewService — business logic, stats computation)

backend/internal/handler/
  review.go          (ReviewHandler — HTTP endpoints)
  review_test.go     (handler tests with mock store)

backend/cmd/api/
  main.go            (wire repo→service→handler, register routes)

frontend/
  index.html         (tabs, review UI, destaques CRUD, export)
```

---

### Task 1: Database Migration + Repository

**Files:**
- Create: `backend/migrations/000015_sprint_review_destaques.up.sql`
- Create: `backend/migrations/000015_sprint_review_destaques.down.sql`
- Create: `backend/internal/repository/review.go`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `type ReviewRepository struct` with `NewReviewRepository(pool *pgxpool.Pool) *ReviewRepository`
  - `func (r *ReviewRepository) Pool() *pgxpool.Pool`
  - `func (r *ReviewRepository) GetReviewTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, produtoIDs []uuid.UUID) ([]ReviewTaskRow, error)`
  - `func (r *ReviewRepository) GetGDPTCAncestorTaskIDs(ctx context.Context, taskIDs []uuid.UUID) ([]uuid.UUID, error)`
  - `func (r *ReviewRepository) GetReviewPOs(ctx context.Context, equipeID uuid.UUID, produtoIDs []uuid.UUID) ([]ReviewPO, error)`
  - `func (r *ReviewRepository) ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]ReviewDestaque, error)`
  - `func (r *ReviewRepository) CreateDestaque(ctx context.Context, d ReviewDestaque) (ReviewDestaque, error)`
  - `func (r *ReviewRepository) UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (ReviewDestaque, error)`
  - `func (r *ReviewRepository) DeleteDestaque(ctx context.Context, id uuid.UUID) error`
  - Types: `ReviewTaskRow`, `ReviewPO`, `ReviewDestaque`

- [ ] **Step 1: Create migration up**

Create `backend/migrations/000015_sprint_review_destaques.up.sql`:

```sql
CREATE TABLE sprint_review_destaques (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sprint_id UUID NOT NULL REFERENCES sprints(id) ON DELETE CASCADE,
    equipe_id UUID NOT NULL REFERENCES equipes(id) ON DELETE CASCADE,
    produto_id UUID NOT NULL REFERENCES produtos(id) ON DELETE CASCADE,
    titulo VARCHAR(200) NOT NULL,
    descricao TEXT NOT NULL,
    link VARCHAR(500),
    ordem INT NOT NULL DEFAULT 0,
    criado_em TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    atualizado_em TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_destaques_sprint_equipe ON sprint_review_destaques(sprint_id, equipe_id);
```

- [ ] **Step 2: Create migration down**

Create `backend/migrations/000015_sprint_review_destaques.down.sql`:

```sql
DROP TABLE IF EXISTS sprint_review_destaques;
```

- [ ] **Step 3: Write repository with types and GetReviewTasks**

Create `backend/internal/repository/review.go`:

```go
package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ReviewRepository struct {
	pool *pgxpool.Pool
}

func NewReviewRepository(pool *pgxpool.Pool) *ReviewRepository {
	return &ReviewRepository{pool: pool}
}

func (r *ReviewRepository) Pool() *pgxpool.Pool {
	return r.pool
}

type ReviewTaskRow struct {
	ID              uuid.UUID   `json:"id"`
	NumeroTicket    string      `json:"numero_ticket"`
	Resumo          string      `json:"resumo"`
	Tipo            string      `json:"tipo"`
	Status          string      `json:"status"`
	ParentID        *uuid.UUID  `json:"parent_id"`
	RelatorNome     *string     `json:"relator_nome"`
	NaoPlanejada    bool        `json:"nao_planejada"`
	Produtos        []string    `json:"produtos"`
	ProdutoIDs      []uuid.UUID `json:"produto_ids"`
}

type ReviewPO struct {
	Nome     string   `json:"nome"`
	Produtos []string `json:"produtos"`
}

type ReviewDestaque struct {
	ID          uuid.UUID  `json:"id"`
	SprintID    uuid.UUID  `json:"sprint_id"`
	EquipeID    uuid.UUID  `json:"equipe_id"`
	ProdutoID   uuid.UUID  `json:"produto_id"`
	ProdutoNome string     `json:"produto_nome"`
	Titulo      string     `json:"titulo"`
	Descricao   string     `json:"descricao"`
	Link        *string    `json:"link"`
	Ordem       int        `json:"ordem"`
	CriadoEm    time.Time  `json:"criado_em"`
	AtualizadoEm time.Time `json:"atualizado_em"`
}

func (r *ReviewRepository) GetReviewTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, produtoIDs []uuid.UUID) ([]ReviewTaskRow, error) {
	argN := 1
	args := []interface{}{sprintID}

	equipeJoin := ""
	equipeWhere := ""
	if equipeID != nil {
		argN++
		args = append(args, *equipeID)
		equipeJoin = "INNER JOIN equipe_membros em ON em.membro_id = t.responsavel_id"
		equipeWhere = fmt.Sprintf("AND em.equipe_id = $%d", argN)
	}

	produtoJoin := ""
	produtoWhere := ""
	if len(produtoIDs) > 0 {
		placeholders := make([]string, len(produtoIDs))
		for i, pid := range produtoIDs {
			argN++
			args = append(args, pid)
			placeholders[i] = fmt.Sprintf("$%d", argN)
		}
		produtoJoin = "INNER JOIN tarefa_produtos tp_filter ON tp_filter.tarefa_id = t.id"
		produtoWhere = fmt.Sprintf("AND tp_filter.produto_id IN (%s)", strings.Join(placeholders, ","))
	}

	query := fmt.Sprintf(`
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.status,
		       t.parent_id, m.nome,
		       CASE WHEN t.data_entrada_sprint > s.data_inicio
		            OR (t.data_entrada_sprint IS NULL AND t.data_criacao > s.data_inicio)
		            THEN true ELSE false END AS nao_planejada,
		       ARRAY_AGG(DISTINCT p.nome) FILTER (WHERE p.nome IS NOT NULL) AS produtos,
		       ARRAY_AGG(DISTINCT p.id) FILTER (WHERE p.id IS NOT NULL) AS produto_ids
		FROM tarefas t
		INNER JOIN sprints s ON s.id = t.sprint_id
		LEFT JOIN membros m ON m.id = t.relator_id
		LEFT JOIN tarefa_produtos tp ON tp.tarefa_id = t.id
		LEFT JOIN produtos p ON p.id = tp.produto_id
		%s
		%s
		WHERE t.sprint_id = $1
		  AND t.status NOT IN ('Cancelado', 'Rejeitada')
		  %s %s
		GROUP BY t.id, t.numero_ticket, t.resumo, t.tipo, t.status,
		         t.parent_id, m.nome, t.data_entrada_sprint, t.data_criacao,
		         s.data_inicio
		ORDER BY t.numero_ticket
	`, equipeJoin, produtoJoin, equipeWhere, produtoWhere)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying review tasks: %w", err)
	}
	defer rows.Close()

	result := make([]ReviewTaskRow, 0)
	for rows.Next() {
		var row ReviewTaskRow
		if err := rows.Scan(
			&row.ID, &row.NumeroTicket, &row.Resumo, &row.Tipo, &row.Status,
			&row.ParentID, &row.RelatorNome, &row.NaoPlanejada, &row.Produtos, &row.ProdutoIDs,
		); err != nil {
			return nil, fmt.Errorf("scanning review task: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
```

- [ ] **Step 4: Add GetGDPTCAncestorTaskIDs method**

Append to `backend/internal/repository/review.go`:

```go
func (r *ReviewRepository) GetGDPTCAncestorTaskIDs(ctx context.Context, taskIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(taskIDs) == 0 {
		return nil, nil
	}

	rows, err := r.pool.Query(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT t.id AS original_id, t.id, t.parent_id, t.numero_ticket
			FROM tarefas t WHERE t.id = ANY($1)
			UNION ALL
			SELECT a.original_id, p.id, p.parent_id, p.numero_ticket
			FROM tarefas p JOIN ancestors a ON p.id = a.parent_id
		)
		SELECT DISTINCT original_id FROM ancestors
		WHERE numero_ticket LIKE 'GDPTC-%' AND original_id != id
	`, taskIDs)
	if err != nil {
		return nil, fmt.Errorf("querying GDPTC ancestors: %w", err)
	}
	defer rows.Close()

	result := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning GDPTC ancestor: %w", err)
		}
		result = append(result, id)
	}
	return result, rows.Err()
}
```

- [ ] **Step 5: Add GetReviewPOs method**

Append to `backend/internal/repository/review.go`:

```go
func (r *ReviewRepository) GetReviewPOs(ctx context.Context, equipeID uuid.UUID, produtoIDs []uuid.UUID) ([]ReviewPO, error) {
	args := []interface{}{equipeID}
	produtoFilter := ""
	if len(produtoIDs) > 0 {
		args = append(args, produtoIDs)
		produtoFilter = "AND p.id = ANY($2)"
	}

	query := fmt.Sprintf(`
		SELECT m.nome, ARRAY_AGG(DISTINCT p.nome) AS produtos
		FROM membros m
		JOIN membro_produtos mp ON mp.membro_id = m.id
		JOIN produtos p ON p.id = mp.produto_id
		WHERE m.equipe_id = $1 AND m.cargo = 'po_produto'
		  %s
		GROUP BY m.id, m.nome
		ORDER BY m.nome
	`, produtoFilter)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying review POs: %w", err)
	}
	defer rows.Close()

	result := make([]ReviewPO, 0)
	for rows.Next() {
		var po ReviewPO
		if err := rows.Scan(&po.Nome, &po.Produtos); err != nil {
			return nil, fmt.Errorf("scanning review PO: %w", err)
		}
		result = append(result, po)
	}
	return result, rows.Err()
}
```

- [ ] **Step 6: Add Destaques CRUD methods**

Append to `backend/internal/repository/review.go`:

```go
func (r *ReviewRepository) ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]ReviewDestaque, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT d.id, d.sprint_id, d.equipe_id, d.produto_id, p.nome,
		       d.titulo, d.descricao, d.link, d.ordem,
		       d.criado_em, d.atualizado_em
		FROM sprint_review_destaques d
		JOIN produtos p ON p.id = d.produto_id
		WHERE d.sprint_id = $1 AND d.equipe_id = $2
		ORDER BY p.nome, d.ordem
	`, sprintID, equipeID)
	if err != nil {
		return nil, fmt.Errorf("listing destaques: %w", err)
	}
	defer rows.Close()

	result := make([]ReviewDestaque, 0)
	for rows.Next() {
		var d ReviewDestaque
		if err := rows.Scan(
			&d.ID, &d.SprintID, &d.EquipeID, &d.ProdutoID, &d.ProdutoNome,
			&d.Titulo, &d.Descricao, &d.Link, &d.Ordem,
			&d.CriadoEm, &d.AtualizadoEm,
		); err != nil {
			return nil, fmt.Errorf("scanning destaque: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (r *ReviewRepository) CreateDestaque(ctx context.Context, d ReviewDestaque) (ReviewDestaque, error) {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO sprint_review_destaques (sprint_id, equipe_id, produto_id, titulo, descricao, link, ordem)
		VALUES ($1, $2, $3, $4, $5, $6,
			COALESCE((SELECT MAX(ordem) + 1 FROM sprint_review_destaques
			          WHERE sprint_id = $1 AND equipe_id = $2 AND produto_id = $3), 0))
		RETURNING id, ordem, criado_em, atualizado_em
	`, d.SprintID, d.EquipeID, d.ProdutoID, d.Titulo, d.Descricao, d.Link,
	).Scan(&d.ID, &d.Ordem, &d.CriadoEm, &d.AtualizadoEm)
	if err != nil {
		return ReviewDestaque{}, fmt.Errorf("creating destaque: %w", err)
	}
	return d, nil
}

func (r *ReviewRepository) UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (ReviewDestaque, error) {
	var d ReviewDestaque
	err := r.pool.QueryRow(ctx, `
		UPDATE sprint_review_destaques
		SET titulo = $2, descricao = $3, link = $4, atualizado_em = NOW()
		WHERE id = $1
		RETURNING id, sprint_id, equipe_id, produto_id, titulo, descricao, link, ordem, criado_em, atualizado_em
	`, id, titulo, descricao, link).Scan(
		&d.ID, &d.SprintID, &d.EquipeID, &d.ProdutoID,
		&d.Titulo, &d.Descricao, &d.Link, &d.Ordem,
		&d.CriadoEm, &d.AtualizadoEm,
	)
	if err != nil {
		return ReviewDestaque{}, fmt.Errorf("updating destaque: %w", err)
	}
	return d, nil
}

func (r *ReviewRepository) DeleteDestaque(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM sprint_review_destaques WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting destaque: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("destaque not found")
	}
	return nil
}
```

- [ ] **Step 7: Run migration and verify build**

```bash
cd /home/emerson/code/myplanner/backend
PGPASSWORD=myplanner_dev psql -U myplanner -d myplanner -h localhost \
  -f migrations/000015_sprint_review_destaques.up.sql
```

Expected: `CREATE TABLE` + `CREATE INDEX`

```bash
go build ./...
```

Expected: build succeeds

- [ ] **Step 8: Commit**

```bash
cd /home/emerson/code/myplanner
git add backend/migrations/000015_sprint_review_destaques.up.sql \
        backend/migrations/000015_sprint_review_destaques.down.sql \
        backend/internal/repository/review.go
git commit -m "feat(review): add migration and repository for sprint review module"
```

---

### Task 2: Service Layer

**Files:**
- Create: `backend/internal/service/review.go`

**Interfaces:**
- Consumes:
  - `repository.ReviewRepository` — `GetReviewTasks`, `GetGDPTCAncestorTaskIDs`, `GetReviewPOs`, `ListDestaques`, `CreateDestaque`, `UpdateDestaque`, `DeleteDestaque`
  - Types: `repository.ReviewTaskRow`, `repository.ReviewPO`, `repository.ReviewDestaque`
- Produces:
  - `type ReviewService struct` with `NewReviewService(repo *repository.ReviewRepository, logger *zap.Logger) *ReviewService`
  - `func (s *ReviewService) GetReviewData(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*ReviewData, error)`
  - `func (s *ReviewService) ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error)`
  - `func (s *ReviewService) CreateDestaque(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error)`
  - `func (s *ReviewService) UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error)`
  - `func (s *ReviewService) DeleteDestaque(ctx context.Context, id uuid.UUID) error`
  - Types: `ReviewData`, `ReviewStats`, `ReviewStatsDetalhes`, `ReviewGraficoProduto`, `ReviewGraficoCategoria`, `ReviewGraficoPlanejamento`, `ReviewTarefa`

- [ ] **Step 1: Create service with types and constructor**

Create `backend/internal/service/review.go`:

```go
package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"myplanner/internal/repository"
)

type ReviewService struct {
	repo   *repository.ReviewRepository
	logger *zap.Logger
}

func NewReviewService(repo *repository.ReviewRepository, logger *zap.Logger) *ReviewService {
	return &ReviewService{repo: repo, logger: logger}
}

type ReviewData struct {
	POs                  []repository.ReviewPO      `json:"pos"`
	Stats                ReviewStats                `json:"stats"`
	GraficoProdutos      []ReviewGraficoProduto     `json:"grafico_produtos"`
	GraficoCategorias    []ReviewGraficoCategoria   `json:"grafico_categorias"`
	GraficoPlanejamento  ReviewGraficoPlanejamento  `json:"grafico_planejamento"`
	Tarefas              []ReviewTarefa             `json:"tarefas"`
}

type ReviewStats struct {
	Total              int                 `json:"total"`
	Concluidas         int                 `json:"concluidas"`
	EmAndamento        int                 `json:"em_andamento"`
	PlanejadasTotal    int                 `json:"planejadas_total"`
	PlanejadasConcl    int                 `json:"planejadas_concluidas"`
	BugsIncidentes     int                 `json:"bugs_incidentes"`
	MelhoriasInovacoes int                 `json:"melhorias_inovacoes"`
	Outros             int                 `json:"outros"`
	Detalhes           ReviewStatsDetalhes `json:"detalhes"`
}

type ReviewStatsDetalhes struct {
	EmAndamento        map[string]int `json:"em_andamento"`
	BugsIncidentes     map[string]int `json:"bugs_incidentes"`
	MelhoriasInovacoes map[string]int `json:"melhorias_inovacoes"`
}

type ReviewGraficoProduto struct {
	ProdutoID  uuid.UUID `json:"produto_id"`
	Produto    string    `json:"produto"`
	Total      int       `json:"total"`
	Concluidas int       `json:"concluidas"`
}

type ReviewGraficoCategoria struct {
	Categoria string `json:"categoria"`
	Total     int    `json:"total"`
}

type ReviewGraficoPlanejamento struct {
	Planejadas         int `json:"planejadas"`
	NaoPlanejadas      int `json:"nao_planejadas"`
	NaoPlanejadasBugs  int `json:"nao_planejadas_bugs"`
	NaoPlanejadasOutras int `json:"nao_planejadas_outras"`
}

type ReviewTarefa struct {
	NumeroTicket string  `json:"numero_ticket"`
	Produto      string  `json:"produto"`
	Resumo       string  `json:"resumo"`
	Relator      string  `json:"relator"`
}
```

- [ ] **Step 2: Implement GetReviewData method**

Append to `backend/internal/service/review.go`:

```go
var statusEmAndamento = map[string]bool{
	"Desenvolvimento":        true,
	"Deploy":                 true,
	"Code Review":            true,
	"Teste":                  true,
	"Validação do Solicitante": true,
}

func (s *ReviewService) GetReviewData(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*ReviewData, error) {
	tasks, err := s.repo.GetReviewTasks(ctx, sprintID, &equipeID, produtoIDs)
	if err != nil {
		return nil, fmt.Errorf("getting review tasks: %w", err)
	}

	// Collect task IDs with parent for GDPTC check (exclude bugs/incidents)
	var parentTaskIDs []uuid.UUID
	for _, t := range tasks {
		if t.ParentID != nil {
			tipoLower := strings.ToLower(t.Tipo)
			if tipoLower != "bug" && !strings.Contains(tipoLower, "incidente") {
				parentTaskIDs = append(parentTaskIDs, t.ID)
			}
		}
	}

	gdptcIDs, err := s.repo.GetGDPTCAncestorTaskIDs(ctx, parentTaskIDs)
	if err != nil {
		return nil, fmt.Errorf("getting GDPTC ancestors: %w", err)
	}
	gdptcSet := make(map[uuid.UUID]bool, len(gdptcIDs))
	for _, id := range gdptcIDs {
		gdptcSet[id] = true
	}

	pos, err := s.repo.GetReviewPOs(ctx, equipeID, produtoIDs)
	if err != nil {
		return nil, fmt.Errorf("getting review POs: %w", err)
	}

	// Compute stats, charts, and task list
	stats := ReviewStats{
		Detalhes: ReviewStatsDetalhes{
			EmAndamento:        make(map[string]int),
			BugsIncidentes:     make(map[string]int),
			MelhoriasInovacoes: make(map[string]int),
		},
	}
	produtoMap := make(map[string]*ReviewGraficoProduto)
	var catManutencao, catNovos, catMelhorias, catOutros int
	planejamento := ReviewGraficoPlanejamento{}
	tarefaList := make([]ReviewTarefa, 0, len(tasks))

	for _, t := range tasks {
		stats.Total++
		tipoLower := strings.ToLower(t.Tipo)

		// Status classification
		if t.Status == "Concluído" {
			stats.Concluidas++
			if !t.NaoPlanejada {
				stats.PlanejadasConcl++
			}
		} else if statusEmAndamento[t.Status] {
			stats.EmAndamento++
			stats.Detalhes.EmAndamento[t.Status]++
		}

		// Planejada tracking
		if !t.NaoPlanejada {
			stats.PlanejadasTotal++
			planejamento.Planejadas++
		} else {
			planejamento.NaoPlanejadas++
			if tipoLower == "bug" || strings.Contains(tipoLower, "incidente") {
				planejamento.NaoPlanejadasBugs++
			} else {
				planejamento.NaoPlanejadasOutras++
			}
		}

		// Type classification
		isBugIncidente := tipoLower == "bug" || strings.Contains(tipoLower, "incidente")
		isGDPTC := gdptcSet[t.ID]

		if isBugIncidente {
			stats.BugsIncidentes++
			stats.Detalhes.BugsIncidentes[t.Tipo]++
			catManutencao++
		} else if isGDPTC {
			stats.MelhoriasInovacoes++
			stats.Detalhes.MelhoriasInovacoes["Portfólio (GDPTC)"]++
			catNovos++
		} else if tipoLower == "melhoria" || tipoLower == "história" || tipoLower == "historia" {
			stats.MelhoriasInovacoes++
			stats.Detalhes.MelhoriasInovacoes[t.Tipo]++
			catMelhorias++
		} else {
			stats.Outros++
			catOutros++
		}

		// Products chart (use ProdutoIDs for unique key, Produtos for display name)
		for i, prod := range t.Produtos {
			var pid uuid.UUID
			if i < len(t.ProdutoIDs) {
				pid = t.ProdutoIDs[i]
			}
			key := pid.String()
			entry, ok := produtoMap[key]
			if !ok {
				entry = &ReviewGraficoProduto{ProdutoID: pid, Produto: prod}
				produtoMap[key] = entry
			}
			entry.Total++
			if t.Status == "Concluído" {
				entry.Concluidas++
			}
		}
		if len(t.Produtos) == 0 {
			entry, ok := produtoMap["sem-produto"]
			if !ok {
				entry = &ReviewGraficoProduto{Produto: "Sem Produto"}
				produtoMap["sem-produto"] = entry
			}
			entry.Total++
			if t.Status == "Concluído" {
				entry.Concluidas++
			}
		}

		// Task list
		produtoStr := ""
		if len(t.Produtos) > 0 {
			produtoStr = strings.Join(t.Produtos, ", ")
		}
		relator := ""
		if t.RelatorNome != nil {
			relator = *t.RelatorNome
		}
		tarefaList = append(tarefaList, ReviewTarefa{
			NumeroTicket: t.NumeroTicket,
			Produto:      produtoStr,
			Resumo:       t.Resumo,
			Relator:      relator,
		})
	}

	// Build grafico_produtos slice
	graficoProdutos := make([]ReviewGraficoProduto, 0, len(produtoMap))
	for _, v := range produtoMap {
		graficoProdutos = append(graficoProdutos, *v)
	}

	graficoCategorias := []ReviewGraficoCategoria{
		{Categoria: "manutencao", Total: catManutencao},
		{Categoria: "novos_projetos", Total: catNovos},
		{Categoria: "melhorias", Total: catMelhorias},
		{Categoria: "outros", Total: catOutros},
	}

	return &ReviewData{
		POs:                 pos,
		Stats:               stats,
		GraficoProdutos:     graficoProdutos,
		GraficoCategorias:   graficoCategorias,
		GraficoPlanejamento: planejamento,
		Tarefas:             tarefaList,
	}, nil
}
```

- [ ] **Step 3: Add destaques pass-through methods**

Append to `backend/internal/service/review.go`:

```go
func (s *ReviewService) ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error) {
	return s.repo.ListDestaques(ctx, sprintID, equipeID)
}

func (s *ReviewService) CreateDestaque(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error) {
	return s.repo.CreateDestaque(ctx, d)
}

func (s *ReviewService) UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error) {
	return s.repo.UpdateDestaque(ctx, id, titulo, descricao, link)
}

func (s *ReviewService) DeleteDestaque(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteDestaque(ctx, id)
}
```

- [ ] **Step 4: Verify build**

```bash
cd /home/emerson/code/myplanner/backend && go build ./...
```

Expected: build succeeds

- [ ] **Step 5: Commit**

```bash
cd /home/emerson/code/myplanner
git add backend/internal/service/review.go
git commit -m "feat(review): add service layer with stats computation and GDPTC classification"
```

---

### Task 3: Handler + Tests + Route Wiring

**Files:**
- Create: `backend/internal/handler/review.go`
- Create: `backend/internal/handler/review_test.go`
- Modify: `backend/cmd/api/main.go`

**Interfaces:**
- Consumes:
  - `service.ReviewService` — `GetReviewData`, `ListDestaques`, `CreateDestaque`, `UpdateDestaque`, `DeleteDestaque`
  - Types: `service.ReviewData`, `repository.ReviewDestaque`
- Produces:
  - `type ReviewHandler struct` with `NewReviewHandler(store ReviewStore, logger *zap.Logger) *ReviewHandler`
  - HTTP methods: `GetReviewData`, `ListDestaques`, `CreateDestaque`, `UpdateDestaque`, `DeleteDestaque`
  - Routes registered in main.go

- [ ] **Step 1: Write handler test file**

Create `backend/internal/handler/review_test.go`:

```go
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"myplanner/internal/repository"
	"myplanner/internal/service"
)

type mockReviewStore struct {
	getReviewDataFn   func(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*service.ReviewData, error)
	listDestaquesFn   func(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error)
	createDestaqueFn  func(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error)
	updateDestaqueFn  func(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error)
	deleteDestaqueFn  func(ctx context.Context, id uuid.UUID) error
}

func (m *mockReviewStore) GetReviewData(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*service.ReviewData, error) {
	return m.getReviewDataFn(ctx, sprintID, equipeID, produtoIDs)
}
func (m *mockReviewStore) ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error) {
	return m.listDestaquesFn(ctx, sprintID, equipeID)
}
func (m *mockReviewStore) CreateDestaque(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error) {
	return m.createDestaqueFn(ctx, d)
}
func (m *mockReviewStore) UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error) {
	return m.updateDestaqueFn(ctx, id, titulo, descricao, link)
}
func (m *mockReviewStore) DeleteDestaque(ctx context.Context, id uuid.UUID) error {
	return m.deleteDestaqueFn(ctx, id)
}

func newTestReviewHandler(store *mockReviewStore) *ReviewHandler {
	return NewReviewHandler(store, zap.NewNop())
}

func TestGetReviewData(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()

	store := &mockReviewStore{
		getReviewDataFn: func(ctx context.Context, sid, eid uuid.UUID, pids []uuid.UUID) (*service.ReviewData, error) {
			if sid != sprintID || eid != equipeID {
				t.Errorf("unexpected IDs: sprint=%s equipe=%s", sid, eid)
			}
			return &service.ReviewData{
				Stats: service.ReviewStats{Total: 10, Concluidas: 5},
			}, nil
		},
	}
	h := newTestReviewHandler(store)

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/review?equipe_id="+equipeID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetReviewData(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result service.ReviewData
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Stats.Total != 10 {
		t.Errorf("expected total=10, got %d", result.Stats.Total)
	}
}

func TestGetReviewDataMissingEquipe(t *testing.T) {
	h := newTestReviewHandler(&mockReviewStore{})
	sprintID := uuid.New()

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/review", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetReviewData(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateDestaque(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	produtoID := uuid.New()
	destaqueID := uuid.New()

	store := &mockReviewStore{
		createDestaqueFn: func(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error) {
			d.ID = destaqueID
			return d, nil
		},
	}
	h := newTestReviewHandler(store)

	body, _ := json.Marshal(map[string]string{
		"equipe_id":  equipeID.String(),
		"produto_id": produtoID.String(),
		"titulo":     "Test Title",
		"descricao":  "Test Description",
	})
	req := httptest.NewRequest("POST", "/sprints/"+sprintID.String()+"/review/destaques", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("sprintId", sprintID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.CreateDestaque(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteDestaque(t *testing.T) {
	destaqueID := uuid.New()
	store := &mockReviewStore{
		deleteDestaqueFn: func(ctx context.Context, id uuid.UUID) error {
			if id != destaqueID {
				t.Errorf("unexpected ID: %s", id)
			}
			return nil
		},
	}
	h := newTestReviewHandler(store)

	req := httptest.NewRequest("DELETE", "/destaques/"+destaqueID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", destaqueID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.DeleteDestaque(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/emerson/code/myplanner/backend && go test ./internal/handler/ -run TestGetReviewData -v
```

Expected: FAIL — `ReviewHandler` and `ReviewStore` not defined

- [ ] **Step 3: Write handler implementation**

Create `backend/internal/handler/review.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"myplanner/internal/repository"
	"myplanner/internal/service"
)

type ReviewStore interface {
	GetReviewData(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*service.ReviewData, error)
	ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error)
	CreateDestaque(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error)
	UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error)
	DeleteDestaque(ctx context.Context, id uuid.UUID) error
}

type ReviewHandler struct {
	store  ReviewStore
	logger *zap.Logger
}

func NewReviewHandler(store ReviewStore, logger *zap.Logger) *ReviewHandler {
	return &ReviewHandler{store: store, logger: logger}
}

func (h *ReviewHandler) GetReviewData(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}

	equipeStr := r.URL.Query().Get("equipe_id")
	if equipeStr == "" {
		respondError(w, http.StatusBadRequest, "equipe_id is required")
		return
	}
	equipeID, err := uuid.Parse(equipeStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid equipe_id")
		return
	}

	var produtoIDs []uuid.UUID
	if produtosStr := r.URL.Query().Get("produtos"); produtosStr != "" {
		for _, p := range strings.Split(produtosStr, ",") {
			pid, err := uuid.Parse(strings.TrimSpace(p))
			if err != nil {
				respondError(w, http.StatusBadRequest, "invalid produto id: "+p)
				return
			}
			produtoIDs = append(produtoIDs, pid)
		}
	}

	data, err := h.store.GetReviewData(r.Context(), sprintID, equipeID, produtoIDs)
	if err != nil {
		h.logger.Error("getting review data", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "error getting review data")
		return
	}

	respondJSON(w, http.StatusOK, data)
}

func (h *ReviewHandler) ListDestaques(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "sprintId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}

	equipeStr := r.URL.Query().Get("equipe_id")
	if equipeStr == "" {
		respondError(w, http.StatusBadRequest, "equipe_id is required")
		return
	}
	equipeID, err := uuid.Parse(equipeStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid equipe_id")
		return
	}

	destaques, err := h.store.ListDestaques(r.Context(), sprintID, equipeID)
	if err != nil {
		h.logger.Error("listing destaques", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "error listing destaques")
		return
	}

	respondJSON(w, http.StatusOK, destaques)
}

type createDestaqueRequest struct {
	EquipeID  uuid.UUID `json:"equipe_id"`
	ProdutoID uuid.UUID `json:"produto_id"`
	Titulo    string    `json:"titulo"`
	Descricao string    `json:"descricao"`
	Link      *string   `json:"link"`
}

func (h *ReviewHandler) CreateDestaque(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "sprintId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}

	var req createDestaqueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Titulo == "" || req.Descricao == "" {
		respondError(w, http.StatusBadRequest, "titulo and descricao are required")
		return
	}
	if len(req.Titulo) > 200 {
		respondError(w, http.StatusBadRequest, "titulo max 200 characters")
		return
	}
	if req.EquipeID == uuid.Nil || req.ProdutoID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "equipe_id and produto_id are required")
		return
	}

	d := repository.ReviewDestaque{
		SprintID:  sprintID,
		EquipeID:  req.EquipeID,
		ProdutoID: req.ProdutoID,
		Titulo:    req.Titulo,
		Descricao: req.Descricao,
		Link:      req.Link,
	}

	created, err := h.store.CreateDestaque(r.Context(), d)
	if err != nil {
		h.logger.Error("creating destaque", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "error creating destaque")
		return
	}

	respondJSON(w, http.StatusOK, created)
}

type updateDestaqueRequest struct {
	Titulo    string  `json:"titulo"`
	Descricao string  `json:"descricao"`
	Link      *string `json:"link"`
}

func (h *ReviewHandler) UpdateDestaque(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid destaque id")
		return
	}

	var req updateDestaqueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Titulo == "" || req.Descricao == "" {
		respondError(w, http.StatusBadRequest, "titulo and descricao are required")
		return
	}
	if len(req.Titulo) > 200 {
		respondError(w, http.StatusBadRequest, "titulo max 200 characters")
		return
	}

	updated, err := h.store.UpdateDestaque(r.Context(), id, req.Titulo, req.Descricao, req.Link)
	if err != nil {
		h.logger.Error("updating destaque", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "error updating destaque")
		return
	}

	respondJSON(w, http.StatusOK, updated)
}

func (h *ReviewHandler) DeleteDestaque(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid destaque id")
		return
	}

	if err := h.store.DeleteDestaque(r.Context(), id); err != nil {
		h.logger.Error("deleting destaque", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "error deleting destaque")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
```

- [ ] **Step 4: Run tests**

```bash
cd /home/emerson/code/myplanner/backend && go test ./internal/handler/ -run TestGetReviewData -v && go test ./internal/handler/ -run TestCreateDestaque -v && go test ./internal/handler/ -run TestDeleteDestaque -v && go test ./internal/handler/ -run TestGetReviewDataMissingEquipe -v
```

Expected: all PASS

- [ ] **Step 5: Wire routes in main.go**

In `backend/cmd/api/main.go`, add after the existing sprint route registrations (after the `r.Post("/sprints/{id}/equalizer/apply", ...)` line):

Instantiation (add after `equalizerHandler` instantiation around line 97):

```go
reviewRepo := repository.NewReviewRepository(pool)
reviewService := service.NewReviewService(reviewRepo, logger)
reviewHandler := handler.NewReviewHandler(reviewService, logger)
```

Routes (add after the equalizer routes, around line 244):

```go
r.Get("/sprints/{id}/review", reviewHandler.GetReviewData)
r.Get("/sprints/{sprintId}/review/destaques", reviewHandler.ListDestaques)
r.Post("/sprints/{sprintId}/review/destaques", reviewHandler.CreateDestaque)
r.Put("/destaques/{id}", reviewHandler.UpdateDestaque)
r.Delete("/destaques/{id}", reviewHandler.DeleteDestaque)
```

- [ ] **Step 6: Run all tests and verify build**

```bash
cd /home/emerson/code/myplanner/backend && go build ./... && go test ./...
```

Expected: build succeeds, all tests pass

- [ ] **Step 7: Commit**

```bash
cd /home/emerson/code/myplanner
git add backend/internal/handler/review.go \
        backend/internal/handler/review_test.go \
        backend/cmd/api/main.go
git commit -m "feat(review): add handler with tests and wire routes for sprint review"
```

---

### Task 4: Frontend — Navigation Tabs + Review Display

**Files:**
- Modify: `frontend/index.html`

**Interfaces:**
- Consumes: `GET /api/v1/sprints/{id}/review?equipe_id=X&produtos=Y,Z` → `ReviewData` JSON
- Produces:
  - Tab navigation UI (Acompanhamento / Review)
  - `loadSprintReview(sprintID)` function
  - `renderReviewContent(data, sprintID, sprintNome, sprintInicio, sprintFim)` function
  - Stats cards with hover tooltips
  - 3 pie charts via `drawPieChart`
  - Task table

- [ ] **Step 1: Add CSS for review module**

Add after the existing `.unplanned-badge` dark mode styles (around line 461 in the CSS section) in `frontend/index.html`:

```css
.sprint-tabs { display: flex; gap: 0; margin-bottom: 20px; border-bottom: 2px solid #e0e0e0; }
.sprint-tab { padding: 10px 20px; cursor: pointer; font-size: 14px; font-weight: 500; color: #666; border-bottom: 2px solid transparent; margin-bottom: -2px; transition: all 0.2s; }
.sprint-tab:hover { color: #1976d2; }
.sprint-tab.active { color: #1976d2; border-bottom-color: #1976d2; }

.review-selectors { display: flex; gap: 12px; align-items: center; flex-wrap: wrap; margin-bottom: 20px; }
.review-selectors select { padding: 8px 12px; border: 1px solid #ddd; border-radius: 6px; font-size: 13px; background: #fff; }
.review-produto-filter { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
.review-produto-chip { display: inline-flex; align-items: center; gap: 4px; padding: 4px 10px; border-radius: 16px; font-size: 12px; cursor: pointer; border: 1px solid #ddd; background: #f5f5f5; transition: all 0.2s; }
.review-produto-chip.selected { background: #1976d2; color: #fff; border-color: #1976d2; }
.review-produto-chip:hover { border-color: #1976d2; }

.review-po-header { background: #e3f2fd; border-radius: 8px; padding: 12px 18px; margin-bottom: 20px; font-size: 14px; color: #1565c0; }

.review-stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 12px; margin-bottom: 24px; }
.review-stat-card { background: #fff; border: 1px solid #e0e0e0; border-radius: 10px; padding: 16px; text-align: center; position: relative; cursor: default; }
.review-stat-card .stat-pct { font-size: 28px; font-weight: 700; color: #1976d2; }
.review-stat-card .stat-label { font-size: 11px; color: #666; margin-top: 4px; text-transform: uppercase; letter-spacing: 0.5px; }
.review-stat-tooltip { display: none; position: absolute; bottom: calc(100% + 8px); left: 50%; transform: translateX(-50%); background: #333; color: #fff; padding: 8px 12px; border-radius: 6px; font-size: 12px; white-space: nowrap; z-index: 100; pointer-events: none; }
.review-stat-card:hover .review-stat-tooltip { display: block; }

.review-charts { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 20px; margin-bottom: 24px; }
.review-chart-box { background: #fff; border: 1px solid #e0e0e0; border-radius: 10px; padding: 16px; }
.review-chart-box h4 { margin: 0 0 12px; font-size: 14px; color: #333; }

.review-destaques { margin-bottom: 24px; }
.review-destaques h3 { font-size: 16px; margin-bottom: 12px; }
.review-produto-destaques { background: #f9f9f9; border: 1px solid #e0e0e0; border-radius: 10px; padding: 16px; margin-bottom: 12px; }
.review-produto-destaques h4 { margin: 0 0 10px; font-size: 14px; color: #333; }
.review-destaque-item { background: #fff; border: 1px solid #eee; border-radius: 8px; padding: 12px; margin-bottom: 8px; position: relative; }
.review-destaque-item .destaque-title { font-weight: 600; font-size: 13px; margin-bottom: 4px; }
.review-destaque-item .destaque-desc { font-size: 12px; color: #555; }
.review-destaque-item .destaque-link { font-size: 11px; color: #1976d2; margin-top: 4px; display: block; }
.review-destaque-actions { position: absolute; top: 8px; right: 8px; display: flex; gap: 4px; }
.review-destaque-actions button { background: none; border: none; cursor: pointer; font-size: 14px; padding: 2px 4px; border-radius: 4px; }
.review-destaque-actions button:hover { background: #f0f0f0; }
.review-add-destaque { background: none; border: 1px dashed #ccc; border-radius: 6px; padding: 8px 14px; cursor: pointer; color: #888; font-size: 12px; width: 100%; text-align: left; }
.review-add-destaque:hover { border-color: #1976d2; color: #1976d2; }
.review-destaque-form { background: #fff; border: 1px solid #1976d2; border-radius: 8px; padding: 12px; margin-bottom: 8px; }
.review-destaque-form input, .review-destaque-form textarea { width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px; font-size: 13px; margin-bottom: 8px; box-sizing: border-box; }
.review-destaque-form textarea { resize: vertical; min-height: 60px; }
.review-destaque-form .form-actions { display: flex; gap: 8px; justify-content: flex-end; }
.review-destaque-form .form-actions button { padding: 6px 14px; border-radius: 4px; border: none; cursor: pointer; font-size: 12px; }
.review-destaque-form .btn-save { background: #1976d2; color: #fff; }
.review-destaque-form .btn-cancel { background: #f5f5f5; color: #333; }

.review-tasks-table { width: 100%; border-collapse: collapse; font-size: 13px; margin-bottom: 24px; }
.review-tasks-table th { text-align: left; padding: 10px 12px; background: #f5f5f5; border-bottom: 2px solid #e0e0e0; font-weight: 600; font-size: 12px; text-transform: uppercase; color: #555; }
.review-tasks-table td { padding: 8px 12px; border-bottom: 1px solid #f0f0f0; }
.review-tasks-table tr:hover td { background: #f8f9fa; }

.review-export-bar { display: flex; gap: 10px; margin-top: 20px; }
.review-export-bar button { padding: 10px 20px; border-radius: 6px; border: none; cursor: pointer; font-size: 13px; font-weight: 500; }
.review-export-bar .btn-pdf { background: #d32f2f; color: #fff; }
.review-export-bar .btn-img { background: #1976d2; color: #fff; }
```

Add dark mode overrides in the dark mode section:

```css
@media (prefers-color-scheme: dark) {
  .sprint-tabs { border-bottom-color: #444; }
  .sprint-tab { color: #aaa; }
  .sprint-tab:hover, .sprint-tab.active { color: #64b5f6; }
  .sprint-tab.active { border-bottom-color: #64b5f6; }
  .review-selectors select { background: #2a2a2a; color: #eee; border-color: #444; }
  .review-po-header { background: #0d47a120; color: #90caf9; }
  .review-stat-card { background: #2a2a2a; border-color: #444; }
  .review-stat-card .stat-pct { color: #64b5f6; }
  .review-stat-card .stat-label { color: #aaa; }
  .review-chart-box { background: #2a2a2a; border-color: #444; }
  .review-chart-box h4 { color: #eee; }
  .review-produto-destaques { background: #1e1e1e; border-color: #444; }
  .review-produto-destaques h4 { color: #eee; }
  .review-destaque-item { background: #2a2a2a; border-color: #444; }
  .review-destaque-item .destaque-desc { color: #bbb; }
  .review-destaque-actions button:hover { background: #333; }
  .review-add-destaque { border-color: #555; color: #888; }
  .review-add-destaque:hover { border-color: #64b5f6; color: #64b5f6; }
  .review-destaque-form { background: #2a2a2a; border-color: #64b5f6; }
  .review-destaque-form input, .review-destaque-form textarea { background: #1e1e1e; color: #eee; border-color: #444; }
  .review-tasks-table th { background: #1e1e1e; border-color: #444; color: #aaa; }
  .review-tasks-table td { border-color: #333; }
  .review-tasks-table tr:hover td { background: #333; }
  .review-produto-chip { background: #333; color: #ccc; border-color: #555; }
  .review-produto-chip.selected { background: #1565c0; color: #fff; border-color: #1565c0; }
}
:root[data-theme="dark"] .sprint-tabs { border-bottom-color: #444; }
:root[data-theme="dark"] .sprint-tab { color: #aaa; }
:root[data-theme="dark"] .sprint-tab:hover, :root[data-theme="dark"] .sprint-tab.active { color: #64b5f6; }
:root[data-theme="dark"] .sprint-tab.active { border-bottom-color: #64b5f6; }
:root[data-theme="dark"] .review-selectors select { background: #2a2a2a; color: #eee; border-color: #444; }
:root[data-theme="dark"] .review-po-header { background: #0d47a120; color: #90caf9; }
:root[data-theme="dark"] .review-stat-card { background: #2a2a2a; border-color: #444; }
:root[data-theme="dark"] .review-stat-card .stat-pct { color: #64b5f6; }
:root[data-theme="dark"] .review-stat-card .stat-label { color: #aaa; }
:root[data-theme="dark"] .review-chart-box { background: #2a2a2a; border-color: #444; }
:root[data-theme="dark"] .review-chart-box h4 { color: #eee; }
:root[data-theme="dark"] .review-produto-destaques { background: #1e1e1e; border-color: #444; }
:root[data-theme="dark"] .review-produto-destaques h4 { color: #eee; }
:root[data-theme="dark"] .review-destaque-item { background: #2a2a2a; border-color: #444; }
:root[data-theme="dark"] .review-destaque-item .destaque-desc { color: #bbb; }
:root[data-theme="dark"] .review-destaque-actions button:hover { background: #333; }
:root[data-theme="dark"] .review-add-destaque { border-color: #555; color: #888; }
:root[data-theme="dark"] .review-add-destaque:hover { border-color: #64b5f6; color: #64b5f6; }
:root[data-theme="dark"] .review-destaque-form { background: #2a2a2a; border-color: #64b5f6; }
:root[data-theme="dark"] .review-destaque-form input, :root[data-theme="dark"] .review-destaque-form textarea { background: #1e1e1e; color: #eee; border-color: #444; }
:root[data-theme="dark"] .review-tasks-table th { background: #1e1e1e; border-color: #444; color: #aaa; }
:root[data-theme="dark"] .review-tasks-table td { border-color: #333; }
:root[data-theme="dark"] .review-tasks-table tr:hover td { background: #333; }
:root[data-theme="dark"] .review-produto-chip { background: #333; color: #ccc; border-color: #555; }
:root[data-theme="dark"] .review-produto-chip.selected { background: #1565c0; color: #fff; border-color: #1565c0; }
```

- [ ] **Step 2: Modify openSprintCapacity to add tab navigation**

In `frontend/index.html`, find the `openSprintCapacity` function (around line 1622). After the `Promise.all` call fetches data and builds the initial HTML, add tab navigation to the beginning of the HTML output.

Replace the first line of the HTML build (the `<div class="sprint-detail-layout">` opening) with tabs:

```javascript
// After Promise.all resolves and before building the main HTML:
// Store current sprint info for review tab access
window._currentSprintID = sprintID;
window._currentSprintNome = data.sprint ? data.sprint.nome : '';
window._currentSprintInicio = data.sprint ? data.sprint.data_inicio : null;
window._currentSprintFim = data.sprint ? data.sprint.data_fim : null;

var html = '<div class="sprint-tabs">';
html += '<div class="sprint-tab active" onclick="showSprintTab(\'acompanhamento\')">Acompanhamento</div>';
html += '<div class="sprint-tab" onclick="showSprintTab(\'review\')">Review</div>';
html += '</div>';
html += '<div id="sprint-tab-acompanhamento">';
// ... existing sprint detail HTML ...
// At the end of the existing HTML, close the tab div:
html += '</div>'; // close sprint-tab-acompanhamento
html += '<div id="sprint-tab-review" style="display:none"></div>';
```

- [ ] **Step 3: Add showSprintTab function**

Add in the JavaScript section of `frontend/index.html`:

```javascript
function showSprintTab(tab) {
  var tabs = document.querySelectorAll('.sprint-tab');
  tabs.forEach(function(t) { t.classList.remove('active'); });
  event.target.classList.add('active');

  document.getElementById('sprint-tab-acompanhamento').style.display = tab === 'acompanhamento' ? '' : 'none';
  var reviewEl = document.getElementById('sprint-tab-review');
  reviewEl.style.display = tab === 'review' ? '' : 'none';

  if (tab === 'review' && !reviewEl.dataset.loaded) {
    loadSprintReview();
  }
}
```

- [ ] **Step 4: Add loadSprintReview function**

Add in the JavaScript section:

```javascript
var _reviewEquipeID = '';
var _reviewProdutoIDs = [];
var _reviewData = null;
var _allReviewProdutos = [];

function loadSprintReview() {
  var container = document.getElementById('sprint-tab-review');
  var qs = window.location.search;
  var params = new URLSearchParams(qs);
  var equipeID = params.get('equipe') || '';

  if (!equipeID) {
    container.innerHTML = '<div style="padding:20px;color:#888">Selecione uma equipe para ver o Review.</div>';
    return;
  }

  _reviewEquipeID = equipeID;
  _reviewProdutoIDs = [];
  _allReviewProdutos = [];

  var produtoFilter = _reviewProdutoIDs.length > 0 ? '&produtos=' + _reviewProdutoIDs.join(',') : '';
  var url = '/sprints/' + window._currentSprintID + '/review?equipe_id=' + equipeID + produtoFilter;

  container.innerHTML = '<div style="padding:20px;color:#888">Carregando review...</div>';

  api(url).then(function(data) {
    _reviewData = data;
    container.dataset.loaded = '1';
    renderReviewContent(container, data);
  }).catch(function(err) {
    container.innerHTML = '<div style="padding:20px;color:#c00">Erro ao carregar review: ' + err.message + '</div>';
  });
}

function reloadReviewWithFilter() {
  var container = document.getElementById('sprint-tab-review');
  container.dataset.loaded = '';
  loadSprintReview();
}
```

- [ ] **Step 5: Add renderReviewContent function**

Add in the JavaScript section:

```javascript
function renderReviewContent(container, data) {
  var html = '';

  // Product filter chips
  // Cache all products on first load so chips persist when filtering
  if (_allReviewProdutos.length === 0) {
    _allReviewProdutos = (data.grafico_produtos || []).filter(function(p) { return p.produto !== 'Sem Produto'; }).map(function(p) { return { id: p.produto_id, nome: p.produto }; }).sort(function(a, b) { return a.nome.localeCompare(b.nome); });
  }
  if (_allReviewProdutos.length > 0) {
    html += '<div class="review-produto-filter">';
    html += '<span style="font-size:12px;color:#888;margin-right:4px">Filtrar produto:</span>';
    _allReviewProdutos.forEach(function(p) {
      html += '<span class="review-produto-chip" onclick="toggleReviewProduto(this)" data-produto-id="' + p.id + '">' + p.nome + '</span>';
    });
    html += '</div>';
  }

  // PO header
  if (data.pos && data.pos.length > 0) {
    var poText = data.pos.map(function(po) { return '<b>' + po.nome + '</b> (' + po.produtos.join(', ') + ')'; }).join(' | ');
    html += '<div class="review-po-header">PO: ' + poText + '</div>';
  }

  // Stats cards
  var total = data.stats.total || 1;
  var stats = data.stats;
  var cards = [
    { label: 'Concluídas', pct: ((stats.concluidas / total) * 100).toFixed(1),
      tooltip: stats.concluidas + ' de ' + total + ' tarefas concluídas' },
    { label: 'Em Andamento', pct: ((stats.em_andamento / total) * 100).toFixed(1),
      tooltip: buildEmAndamentoTooltip(stats.detalhes ? stats.detalhes.em_andamento : {}) },
    { label: 'Plan. Concluídas', pct: stats.planejadas_total > 0 ? ((stats.planejadas_concluidas / stats.planejadas_total) * 100).toFixed(1) : '0.0',
      tooltip: stats.planejadas_concluidas + ' de ' + stats.planejadas_total + ' planejadas concluídas' },
    { label: 'Bugs e Incidentes', pct: ((stats.bugs_incidentes / total) * 100).toFixed(1),
      tooltip: buildMapTooltip(stats.detalhes ? stats.detalhes.bugs_incidentes : {}) },
    { label: 'Melhorias e Inovações', pct: ((stats.melhorias_inovacoes / total) * 100).toFixed(1),
      tooltip: buildMapTooltip(stats.detalhes ? stats.detalhes.melhorias_inovacoes : {}) },
    { label: 'Outros', pct: ((stats.outros / total) * 100).toFixed(1),
      tooltip: 'Tarefas: ' + stats.outros }
  ];

  html += '<div class="review-stats">';
  cards.forEach(function(c) {
    html += '<div class="review-stat-card">';
    html += '<div class="review-stat-tooltip">' + c.tooltip + '</div>';
    html += '<div class="stat-pct">' + c.pct + '%</div>';
    html += '<div class="stat-label">' + c.label + '</div>';
    html += '</div>';
  });
  html += '</div>';

  // Charts
  html += '<div class="review-charts">';
  html += '<div class="review-chart-box"><h4>Entregas por Produto</h4><div id="review-pie-produtos"></div></div>';
  html += '<div class="review-chart-box"><h4>Categorias</h4><div id="review-pie-categorias"></div></div>';
  html += '<div class="review-chart-box"><h4>Planejamento</h4><div id="review-pie-planejamento"></div></div>';
  html += '</div>';

  // Destaques placeholder (loaded separately in Task 5)
  html += '<div id="review-destaques-container" class="review-destaques"><h3>Destaques</h3><div id="review-destaques-list"></div></div>';

  // Tasks table
  html += '<h3 style="font-size:16px;margin-bottom:12px">Tarefas</h3>';
  html += '<table class="review-tasks-table"><thead><tr>';
  html += '<th>Ticket</th><th>Produto</th><th>Resumo</th><th>Relator</th>';
  html += '</tr></thead><tbody>';
  (data.tarefas || []).forEach(function(t) {
    html += '<tr>';
    html += '<td>' + t.numero_ticket + '</td>';
    html += '<td>' + (t.produto || '') + '</td>';
    html += '<td>' + t.resumo + '</td>';
    html += '<td>' + (t.relator || '') + '</td>';
    html += '</tr>';
  });
  html += '</tbody></table>';

  // Export bar (wired in Task 6)
  html += '<div class="review-export-bar">';
  html += '<button class="btn-pdf" onclick="exportReviewPDF()">Exportar PDF</button>';
  html += '<button class="btn-img" onclick="exportReviewImage()">Exportar Imagem</button>';
  html += '</div>';

  container.innerHTML = html;

  // Draw pie charts
  drawReviewPieCharts(data);

  // Load destaques
  if (typeof loadReviewDestaques === 'function') {
    loadReviewDestaques();
  }
}

function buildEmAndamentoTooltip(map) {
  if (!map || Object.keys(map).length === 0) return 'Nenhuma tarefa em andamento';
  return Object.keys(map).map(function(k) { return k + ': ' + map[k]; }).join(', ');
}

function buildMapTooltip(map) {
  if (!map || Object.keys(map).length === 0) return 'Nenhuma';
  return Object.keys(map).map(function(k) { return k + ': ' + map[k]; }).join(', ');
}

function toggleReviewProduto(chip) {
  chip.classList.toggle('selected');
  var selected = document.querySelectorAll('.review-produto-chip.selected');
  _reviewProdutoIDs = [];
  selected.forEach(function(s) {
    if (s.dataset.produtoId) _reviewProdutoIDs.push(s.dataset.produtoId);
  });

  // Re-fetch from API with produto filter for accurate stats
  var container = document.getElementById('sprint-tab-review');
  var produtoFilter = _reviewProdutoIDs.length > 0 ? '&produtos=' + _reviewProdutoIDs.join(',') : '';
  var url = '/sprints/' + window._currentSprintID + '/review?equipe_id=' + _reviewEquipeID + produtoFilter;

  api(url).then(function(data) {
    _reviewData = data;
    renderReviewContent(container, data);
    // Re-apply chip selections after re-render
    setTimeout(function() {
      document.querySelectorAll('.review-produto-chip').forEach(function(c) {
        if (_reviewProdutoIDs.indexOf(c.dataset.produtoId) >= 0) c.classList.add('selected');
      });
    }, 0);
  });
}

function drawReviewPieCharts(data) {
  // Produtos pie
  var prodSlices = (data.grafico_produtos || []).map(function(p) {
    return { label: p.produto, horas: p.total };
  });
  var prodEl = document.getElementById('review-pie-produtos');
  if (prodEl) drawPieChart(prodEl, prodSlices);

  // Categorias pie
  var catLabels = { manutencao: 'Manutenção', novos_projetos: 'Novos Projetos', melhorias: 'Melhorias', outros: 'Outros' };
  var catColors = { manutencao: '#e53935', novos_projetos: '#1976d2', melhorias: '#43a047', outros: '#ff9800' };
  var catSlices = (data.grafico_categorias || []).filter(function(c) { return c.total > 0; }).map(function(c) {
    return { label: catLabels[c.categoria] || c.categoria, horas: c.total };
  });
  var catEl = document.getElementById('review-pie-categorias');
  if (catEl) drawPieChart(catEl, catSlices);

  // Planejamento pie
  var plan = data.grafico_planejamento || {};
  var planSlices = [];
  if (plan.planejadas > 0) planSlices.push({ label: 'Planejadas', horas: plan.planejadas });
  if (plan.nao_planejadas > 0) planSlices.push({
    label: 'Não Planejadas (Bugs: ' + (plan.nao_planejadas_bugs || 0) + ', Outras: ' + (plan.nao_planejadas_outras || 0) + ')',
    horas: plan.nao_planejadas
  });
  var planEl = document.getElementById('review-pie-planejamento');
  if (planEl) drawPieChart(planEl, planSlices);
}
```

- [ ] **Step 6: Verify build by running backend**

```bash
cd /home/emerson/code/myplanner/backend && go build ./... && go test ./...
```

Expected: build passes (frontend is static HTML, no build step)

- [ ] **Step 7: Browser test**

Open `http://localhost:8080`, navigate to a sprint, verify:
1. Tabs "Acompanhamento" and "Review" appear
2. Clicking "Acompanhamento" shows existing sprint detail
3. Clicking "Review" loads data and shows stats cards, pie charts, task table
4. Hover on stat cards shows tooltip

- [ ] **Step 8: Commit**

```bash
cd /home/emerson/code/myplanner
git add frontend/index.html
git commit -m "feat(review): add sprint review tab with stats, pie charts, and task table"
```

---

### Task 5: Frontend — Destaques CRUD

**Files:**
- Modify: `frontend/index.html`

**Interfaces:**
- Consumes:
  - `GET /api/v1/sprints/{sprintId}/review/destaques?equipe_id=X` → `ReviewDestaque[]`
  - `POST /api/v1/sprints/{sprintId}/review/destaques` → `ReviewDestaque`
  - `PUT /api/v1/destaques/{id}` → `ReviewDestaque`
  - `DELETE /api/v1/destaques/{id}` → `{status: "deleted"}`
  - `_reviewEquipeID`, `window._currentSprintID` from Task 4
  - DOM element `#review-destaques-list` from Task 4's `renderReviewContent`
- Produces:
  - `loadReviewDestaques()` function
  - `renderDestaques(destaques)` function
  - `showDestaqueForm(produtoID, produtoNome, destaque?)` function
  - `saveDestaque(produtoID, destaqueID?)` function
  - `deleteDestaque(id)` function

- [ ] **Step 1: Add loadReviewDestaques function**

Add in the JavaScript section of `frontend/index.html` after the review functions from Task 4:

```javascript
function loadReviewDestaques() {
  var container = document.getElementById('review-destaques-list');
  if (!container || !_reviewEquipeID) return;

  api('/sprints/' + window._currentSprintID + '/review/destaques?equipe_id=' + _reviewEquipeID)
    .then(function(destaques) {
      renderDestaques(container, destaques || []);
    })
    .catch(function(err) {
      container.innerHTML = '<div style="color:#c00;font-size:12px">Erro ao carregar destaques: ' + err.message + '</div>';
    });
}

function renderDestaques(container, destaques) {
  // Group by produto
  var byProduto = {};
  var produtoNames = {};
  destaques.forEach(function(d) {
    if (!byProduto[d.produto_id]) {
      byProduto[d.produto_id] = [];
      produtoNames[d.produto_id] = d.produto_nome;
    }
    byProduto[d.produto_id].push(d);
  });

  // Also add produto blocks from review data that have no destaques yet
  if (_reviewData && _reviewData.grafico_produtos) {
    _reviewData.grafico_produtos.forEach(function(p) {
      // We need produto IDs but grafico_produtos only has names.
      // Destaques that exist give us the mapping. For products without destaques,
      // we won't show an "add" block since we don't have the ID.
      // Solution: include produto_id in grafico_produtos response (Task 2 service change).
    });
  }

  var html = '';
  Object.keys(byProduto).forEach(function(prodID) {
    var items = byProduto[prodID];
    var prodName = produtoNames[prodID] || 'Produto';
    html += '<div class="review-produto-destaques">';
    html += '<h4>' + prodName + '</h4>';
    html += '<button class="review-add-destaque" onclick="showDestaqueForm(\'' + prodID + '\',\'' + prodName + '\')">+ Adicionar destaque</button>';
    html += '<div id="destaque-form-' + prodID + '"></div>';
    items.forEach(function(d) {
      html += '<div class="review-destaque-item">';
      html += '<div class="review-destaque-actions">';
      html += '<button onclick="showDestaqueForm(\'' + prodID + '\',\'' + prodName + '\',' + JSON.stringify(d).replace(/'/g, "\\'") + ')" title="Editar">&#9998;</button>';
      html += '<button onclick="deleteDestaque(\'' + d.id + '\')" title="Remover">&#128465;</button>';
      html += '</div>';
      html += '<div class="destaque-title">' + d.titulo + '</div>';
      html += '<div class="destaque-desc">' + d.descricao + '</div>';
      if (d.link) html += '<a class="destaque-link" href="' + d.link + '" target="_blank">' + d.link + '</a>';
      html += '</div>';
    });
    html += '</div>';
  });

  // Add button for products that have no destaques (if we have produto data with IDs)
  // This will be handled after we add produto_id to the API response

  container.innerHTML = html;
}

function showDestaqueForm(produtoID, produtoNome, destaque) {
  var formContainer = document.getElementById('destaque-form-' + produtoID);
  if (!formContainer) {
    // If no existing destaques for this product, we need to create the block
    var list = document.getElementById('review-destaques-list');
    if (!list) return;
    var block = document.createElement('div');
    block.className = 'review-produto-destaques';
    block.innerHTML = '<h4>' + produtoNome + '</h4><div id="destaque-form-' + produtoID + '"></div>';
    list.appendChild(block);
    formContainer = document.getElementById('destaque-form-' + produtoID);
  }

  var titulo = destaque ? destaque.titulo : '';
  var descricao = destaque ? destaque.descricao : '';
  var link = destaque && destaque.link ? destaque.link : '';
  var destaqueID = destaque ? destaque.id : '';

  formContainer.innerHTML = '<div class="review-destaque-form">' +
    '<input type="text" id="destaque-titulo-' + produtoID + '" placeholder="Titulo" value="' + titulo.replace(/"/g, '&quot;') + '" maxlength="200">' +
    '<textarea id="destaque-desc-' + produtoID + '" placeholder="Descricao">' + descricao + '</textarea>' +
    '<input type="text" id="destaque-link-' + produtoID + '" placeholder="Link (opcional)" value="' + link.replace(/"/g, '&quot;') + '">' +
    '<div class="form-actions">' +
    '<button class="btn-cancel" onclick="document.getElementById(\'destaque-form-' + produtoID + '\').innerHTML=\'\'">Cancelar</button>' +
    '<button class="btn-save" onclick="saveDestaque(\'' + produtoID + '\',\'' + destaqueID + '\')">Salvar</button>' +
    '</div></div>';
}

function saveDestaque(produtoID, destaqueID) {
  var titulo = document.getElementById('destaque-titulo-' + produtoID).value.trim();
  var descricao = document.getElementById('destaque-desc-' + produtoID).value.trim();
  var link = document.getElementById('destaque-link-' + produtoID).value.trim() || null;

  if (!titulo || !descricao) {
    alert('Titulo e descricao sao obrigatorios');
    return;
  }

  var body, url, method;
  if (destaqueID) {
    url = '/destaques/' + destaqueID;
    method = 'PUT';
    body = { titulo: titulo, descricao: descricao, link: link };
  } else {
    url = '/sprints/' + window._currentSprintID + '/review/destaques';
    method = 'POST';
    body = {
      equipe_id: _reviewEquipeID,
      produto_id: produtoID,
      titulo: titulo,
      descricao: descricao,
      link: link
    };
  }

  api(url, { method: method, body: JSON.stringify(body), headers: { 'Content-Type': 'application/json' } })
    .then(function() { loadReviewDestaques(); })
    .catch(function(err) { alert('Erro: ' + err.message); });
}

function deleteDestaque(id) {
  if (!confirm('Remover destaque?')) return;
  api('/destaques/' + id, { method: 'DELETE' })
    .then(function() { loadReviewDestaques(); })
    .catch(function(err) { alert('Erro: ' + err.message); });
}
```

- [ ] **Step 2: Verify product IDs work end-to-end**

`ReviewGraficoProduto` already has `ProdutoID uuid.UUID` from Task 2, and `ReviewTaskRow` already has `ProdutoIDs []uuid.UUID` from Task 1. The service already maps product IDs to the chart data. Verify the destaques JS correctly uses `produto_id` from the API response. No code changes needed in this step — it's a verification checkpoint.

- [ ] **Step 3: Update renderDestaques to show add buttons for all products**

Update `renderDestaques` to use `_reviewData.grafico_produtos` for produto IDs:

```javascript
// After rendering existing destaques by product, add blocks for products without destaques
if (_reviewData && _reviewData.grafico_produtos) {
  _reviewData.grafico_produtos.forEach(function(p) {
    if (p.produto === 'Sem Produto' || byProduto[p.produto_id]) return;
    html += '<div class="review-produto-destaques">';
    html += '<h4>' + p.produto + '</h4>';
    html += '<button class="review-add-destaque" onclick="showDestaqueForm(\'' + p.produto_id + '\',\'' + p.produto + '\')">+ Adicionar destaque</button>';
    html += '<div id="destaque-form-' + p.produto_id + '"></div>';
    html += '</div>';
  });
}
```

- [ ] **Step 4: Verify build and tests**

```bash
cd /home/emerson/code/myplanner/backend && go build ./... && go test ./...
```

Expected: build passes, tests pass

- [ ] **Step 5: Browser test**

Open `http://localhost:8080`, navigate to a sprint, click "Review" tab:
1. Destaques section shows products from the sprint
2. Click `+ Adicionar destaque` — form appears with título, descrição, link fields
3. Fill in and click "Salvar" — destaque appears in the list
4. Click edit (pencil) — form pre-filled, save updates it
5. Click delete (trash) — confirmation, destaque removed

- [ ] **Step 6: Commit**

```bash
cd /home/emerson/code/myplanner
git add frontend/index.html backend/internal/service/review.go backend/internal/repository/review.go
git commit -m "feat(review): add destaques CRUD UI with product grouping"
```

---

### Task 6: Frontend — Export PDF/Image

**Files:**
- Modify: `frontend/index.html`

**Interfaces:**
- Consumes:
  - `_reviewData` from Task 4
  - `window._currentSprintNome`, `window._currentSprintInicio`, `window._currentSprintFim` from Task 4
  - `html2canvas` and `jsPDF` libraries (inlined)
  - DOM elements from Task 4's `renderReviewContent`
- Produces:
  - `exportReviewPDF()` function
  - `exportReviewImage()` function
  - Inlined `html2canvas` and `jsPDF` libraries in `<script>` tags

- [ ] **Step 1: Download and inline html2canvas + jsPDF**

Download minified versions:

```bash
cd /home/emerson/code/myplanner
curl -sL https://cdnjs.cloudflare.com/ajax/libs/html2canvas/1.4.1/html2canvas.min.js -o /home/emerson/.claude/jobs/30c85d42/tmp/html2canvas.min.js
curl -sL https://cdnjs.cloudflare.com/ajax/libs/jspdf/2.5.1/jspdf.umd.min.js -o /home/emerson/.claude/jobs/30c85d42/tmp/jspdf.min.js
```

Then inline both as `<script>` tags just before the closing `</body>` tag in `frontend/index.html` (before the main application script):

```html
<script>/* html2canvas v1.4.1 */
... (paste minified content) ...
</script>
<script>/* jsPDF v2.5.1 */
... (paste minified content) ...
</script>
```

- [ ] **Step 2: Add export functions**

Add in the JavaScript section of `frontend/index.html`:

```javascript
function buildExportContainer() {
  if (!_reviewData) return null;

  var div = document.createElement('div');
  div.id = 'review-export-container';
  div.style.cssText = 'position:absolute;left:-9999px;top:0;width:1200px;background:#fff;color:#333;padding:40px;font-family:Arial,sans-serif;';

  var data = _reviewData;
  var stats = data.stats;
  var total = stats.total || 1;
  var sprintNome = window._currentSprintNome || '';
  var inicio = window._currentSprintInicio ? new Date(window._currentSprintInicio).toLocaleDateString('pt-BR') : '';
  var fim = window._currentSprintFim ? new Date(window._currentSprintFim).toLocaleDateString('pt-BR') : '';

  var html = '<div style="margin-bottom:24px">';
  html += '<h1 style="font-size:22px;margin:0 0 4px">Review ' + sprintNome + '</h1>';
  html += '<div style="font-size:14px;color:#555">' + inicio + ' — ' + fim + '</div>';
  if (_reviewProdutoIDs.length > 0) {
    html += '<div style="font-size:13px;color:#888;margin-top:4px">Produtos filtrados: ' + _reviewProdutoIDs.join(', ') + '</div>';
  }
  if (data.pos && data.pos.length > 0) {
    html += '<div style="font-size:13px;margin-top:4px">PO: ' + data.pos.map(function(po) { return po.nome + ' (' + po.produtos.join(', ') + ')'; }).join(' | ') + '</div>';
  }
  html += '</div>';

  // Stats row
  var cards = [
    { label: 'Concluídas', pct: ((stats.concluidas / total) * 100).toFixed(1) + '%' },
    { label: 'Em Andamento', pct: ((stats.em_andamento / total) * 100).toFixed(1) + '%' },
    { label: 'Plan. Concluídas', pct: stats.planejadas_total > 0 ? ((stats.planejadas_concluidas / stats.planejadas_total) * 100).toFixed(1) + '%' : '0.0%' },
    { label: 'Bugs e Incidentes', pct: ((stats.bugs_incidentes / total) * 100).toFixed(1) + '%' },
    { label: 'Melhorias e Inovações', pct: ((stats.melhorias_inovacoes / total) * 100).toFixed(1) + '%' },
    { label: 'Outros', pct: ((stats.outros / total) * 100).toFixed(1) + '%' }
  ];
  html += '<div style="display:flex;gap:12px;margin-bottom:24px">';
  cards.forEach(function(c) {
    html += '<div style="flex:1;text-align:center;border:1px solid #ddd;border-radius:8px;padding:12px">';
    html += '<div style="font-size:24px;font-weight:700;color:#1976d2">' + c.pct + '</div>';
    html += '<div style="font-size:10px;color:#666;text-transform:uppercase">' + c.label + '</div>';
    html += '</div>';
  });
  html += '</div>';

  // Charts placeholder divs
  html += '<div style="display:flex;gap:20px;margin-bottom:24px">';
  html += '<div style="flex:1;border:1px solid #ddd;border-radius:8px;padding:12px"><h4 style="margin:0 0 8px;font-size:13px">Entregas por Produto</h4><div id="export-pie-produtos"></div></div>';
  html += '<div style="flex:1;border:1px solid #ddd;border-radius:8px;padding:12px"><h4 style="margin:0 0 8px;font-size:13px">Categorias</h4><div id="export-pie-categorias"></div></div>';
  html += '<div style="flex:1;border:1px solid #ddd;border-radius:8px;padding:12px"><h4 style="margin:0 0 8px;font-size:13px">Planejamento</h4><div id="export-pie-planejamento"></div></div>';
  html += '</div>';

  // Destaques (static, no buttons)
  var destaqueEls = document.querySelectorAll('.review-destaque-item');
  if (destaqueEls.length > 0) {
    html += '<h3 style="font-size:15px;margin-bottom:8px">Destaques</h3>';
    destaqueEls.forEach(function(el) {
      var title = el.querySelector('.destaque-title');
      var desc = el.querySelector('.destaque-desc');
      var link = el.querySelector('.destaque-link');
      html += '<div style="border:1px solid #eee;border-radius:6px;padding:10px;margin-bottom:6px">';
      if (title) html += '<div style="font-weight:600;font-size:12px">' + title.textContent + '</div>';
      if (desc) html += '<div style="font-size:11px;color:#555">' + desc.textContent + '</div>';
      if (link) html += '<div style="font-size:10px;color:#1976d2">' + link.textContent + '</div>';
      html += '</div>';
    });
  }

  // Tasks table
  html += '<h3 style="font-size:15px;margin:16px 0 8px">Tarefas</h3>';
  html += '<table style="width:100%;border-collapse:collapse;font-size:11px">';
  html += '<thead><tr style="background:#f5f5f5"><th style="padding:6px 8px;text-align:left;border-bottom:1px solid #ddd">Ticket</th><th style="padding:6px 8px;text-align:left;border-bottom:1px solid #ddd">Produto</th><th style="padding:6px 8px;text-align:left;border-bottom:1px solid #ddd">Resumo</th><th style="padding:6px 8px;text-align:left;border-bottom:1px solid #ddd">Relator</th></tr></thead><tbody>';
  (data.tarefas || []).forEach(function(t) {
    html += '<tr><td style="padding:4px 8px;border-bottom:1px solid #f0f0f0">' + t.numero_ticket + '</td>';
    html += '<td style="padding:4px 8px;border-bottom:1px solid #f0f0f0">' + (t.produto || '') + '</td>';
    html += '<td style="padding:4px 8px;border-bottom:1px solid #f0f0f0">' + t.resumo + '</td>';
    html += '<td style="padding:4px 8px;border-bottom:1px solid #f0f0f0">' + (t.relator || '') + '</td></tr>';
  });
  html += '</tbody></table>';

  div.innerHTML = html;
  document.body.appendChild(div);

  // Draw charts into export container
  var catLabels = { manutencao: 'Manutenção', novos_projetos: 'Novos Projetos', melhorias: 'Melhorias', outros: 'Outros' };
  drawPieChart(document.getElementById('export-pie-produtos'),
    (data.grafico_produtos || []).map(function(p) { return { label: p.produto, horas: p.total }; }));
  drawPieChart(document.getElementById('export-pie-categorias'),
    (data.grafico_categorias || []).filter(function(c) { return c.total > 0; }).map(function(c) { return { label: catLabels[c.categoria] || c.categoria, horas: c.total }; }));
  var plan = data.grafico_planejamento || {};
  var planSlices = [];
  if (plan.planejadas > 0) planSlices.push({ label: 'Planejadas', horas: plan.planejadas });
  if (plan.nao_planejadas > 0) planSlices.push({ label: 'Não Planejadas', horas: plan.nao_planejadas });
  drawPieChart(document.getElementById('export-pie-planejamento'), planSlices);

  return div;
}

function exportReviewImage() {
  var div = buildExportContainer();
  if (!div) return;

  html2canvas(div, { scale: 2, backgroundColor: '#ffffff', useCORS: true }).then(function(canvas) {
    var link = document.createElement('a');
    var nome = (window._currentSprintNome || 'sprint').replace(/\s+/g, '-');
    link.download = 'review-' + nome + '.png';
    link.href = canvas.toDataURL('image/png');
    link.click();
    div.remove();
  }).catch(function(err) {
    alert('Erro ao exportar imagem: ' + err.message);
    div.remove();
  });
}

function exportReviewPDF() {
  var div = buildExportContainer();
  if (!div) return;

  html2canvas(div, { scale: 2, backgroundColor: '#ffffff', useCORS: true }).then(function(canvas) {
    var imgData = canvas.toDataURL('image/png');
    var imgW = canvas.width;
    var imgH = canvas.height;

    var orientation = imgW > imgH ? 'l' : 'p';
    var pdf = new jspdf.jsPDF(orientation, 'mm', 'a4');
    var pageW = pdf.internal.pageSize.getWidth();
    var pageH = pdf.internal.pageSize.getHeight();
    var margin = 10;
    var contentW = pageW - 2 * margin;
    var contentH = (imgH / imgW) * contentW;

    if (contentH <= pageH - 2 * margin) {
      pdf.addImage(imgData, 'PNG', margin, margin, contentW, contentH);
    } else {
      // Multi-page: split canvas
      var pageContentH = pageH - 2 * margin;
      var srcPageH = (pageContentH / contentW) * imgW;
      var pages = Math.ceil(imgH / srcPageH);
      for (var i = 0; i < pages; i++) {
        if (i > 0) pdf.addPage();
        var srcY = i * srcPageH;
        var thisH = Math.min(srcPageH, imgH - srcY);
        var pageCanvas = document.createElement('canvas');
        pageCanvas.width = imgW;
        pageCanvas.height = thisH;
        pageCanvas.getContext('2d').drawImage(canvas, 0, srcY, imgW, thisH, 0, 0, imgW, thisH);
        var pageImg = pageCanvas.toDataURL('image/png');
        var drawH = (thisH / imgW) * contentW;
        pdf.addImage(pageImg, 'PNG', margin, margin, contentW, drawH);
      }
    }

    var nome = (window._currentSprintNome || 'sprint').replace(/\s+/g, '-');
    pdf.save('review-' + nome + '.pdf');
    div.remove();
  }).catch(function(err) {
    alert('Erro ao exportar PDF: ' + err.message);
    div.remove();
  });
}
```

- [ ] **Step 3: Browser test**

Open `http://localhost:8080`, navigate to a sprint Review tab:
1. Click "Exportar Imagem" — downloads PNG with full review layout
2. Click "Exportar PDF" — downloads PDF with header, stats, charts, destaques, table
3. Verify export header shows sprint name, dates, PO

- [ ] **Step 4: Commit**

```bash
cd /home/emerson/code/myplanner
git add frontend/index.html
git commit -m "feat(review): add PDF and image export with html2canvas and jsPDF"
```
