# Sprint Review IA Analysis — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add AI-powered sprint review analysis using OpenRouter (free model `openai/gpt-oss-20b:free`), with user-configurable API key stored in DB, cached analysis results, and structured cards UI.

**Architecture:** New `configuracoes` table for key-value app settings. New `sprint_review_analises` table for cached AI results. New `openrouter.go` service following existing `gemini.go` pattern. New `config.go` repository for settings CRUD. Review handler/service extended with analysis endpoints. Frontend renders structured JSON response as product-grouped cards between pie charts and task tables.

**Tech Stack:** Go (chi router, pgx/v5, zap), PostgreSQL, OpenRouter API (OpenAI-compatible chat completions), vanilla JS frontend

## Global Constraints

- OpenRouter base URL: `https://openrouter.ai/api/v1/chat/completions`
- Default model: `openai/gpt-oss-20b:free`
- Config whitelist keys: `openrouter_api_key`, `openrouter_model`
- GET `/api/config/openrouter_api_key` returns `{"exists": true/false}` — NEVER the actual key value
- API key stored plain text in DB (single-tenant app)
- AI analysis timeout: 60 seconds
- Retry: 1 retry on API failure
- `estimativa_tempo` field in `tarefas` table is INTEGER in seconds; convert to hours with `/ 3600.0`
- Migration numbers: 000019 (configuracoes), 000020 (sprint_review_analises)
- All code follows existing codebase patterns (repository → service → handler)

---

### Task 1: Database Migrations + Config Repository

**Files:**
- Create: `backend/migrations/000019_configuracoes.up.sql`
- Create: `backend/migrations/000019_configuracoes.down.sql`
- Create: `backend/migrations/000020_sprint_review_analises.up.sql`
- Create: `backend/migrations/000020_sprint_review_analises.down.sql`
- Create: `backend/internal/repository/config.go`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `ConfigRepository` struct with `pool *pgxpool.Pool`
  - `NewConfigRepository(pool *pgxpool.Pool) *ConfigRepository`
  - `GetConfig(ctx context.Context, chave string) (string, error)` — returns value or `pgx.ErrNoRows`
  - `SetConfig(ctx context.Context, chave, valor string) error` — upsert
  - `ConfigExists(ctx context.Context, chave string) (bool, error)`

- [ ] **Step 1: Create migration 000019 up**

```sql
-- backend/migrations/000019_configuracoes.up.sql
CREATE TABLE configuracoes (
    chave VARCHAR(100) PRIMARY KEY,
    valor TEXT NOT NULL,
    atualizado_em TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

- [ ] **Step 2: Create migration 000019 down**

```sql
-- backend/migrations/000019_configuracoes.down.sql
DROP TABLE IF EXISTS configuracoes;
```

- [ ] **Step 3: Create migration 000020 up**

```sql
-- backend/migrations/000020_sprint_review_analises.up.sql
CREATE TABLE sprint_review_analises (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sprint_id UUID NOT NULL REFERENCES sprints(id),
    equipe_id UUID NOT NULL REFERENCES equipes(id),
    produto_ids UUID[] NOT NULL DEFAULT '{}',
    analise_json JSONB NOT NULL,
    modelo VARCHAR(100) NOT NULL,
    criado_em TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(sprint_id, equipe_id, produto_ids)
);
```

- [ ] **Step 4: Create migration 000020 down**

```sql
-- backend/migrations/000020_sprint_review_analises.down.sql
DROP TABLE IF EXISTS sprint_review_analises;
```

- [ ] **Step 5: Create config repository**

```go
// backend/internal/repository/config.go
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ConfigRepository struct {
	pool *pgxpool.Pool
}

func NewConfigRepository(pool *pgxpool.Pool) *ConfigRepository {
	return &ConfigRepository{pool: pool}
}

func (r *ConfigRepository) GetConfig(ctx context.Context, chave string) (string, error) {
	var valor string
	err := r.pool.QueryRow(ctx,
		`SELECT valor FROM configuracoes WHERE chave = $1`, chave,
	).Scan(&valor)
	if err != nil {
		return "", fmt.Errorf("getting config %s: %w", chave, err)
	}
	return valor, nil
}

func (r *ConfigRepository) SetConfig(ctx context.Context, chave, valor string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO configuracoes (chave, valor, atualizado_em)
		VALUES ($1, $2, NOW())
		ON CONFLICT (chave) DO UPDATE SET valor = $2, atualizado_em = NOW()
	`, chave, valor)
	if err != nil {
		return fmt.Errorf("setting config %s: %w", chave, err)
	}
	return nil
}

func (r *ConfigRepository) ConfigExists(ctx context.Context, chave string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM configuracoes WHERE chave = $1)`, chave,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking config %s: %w", chave, err)
	}
	return exists, nil
}
```

- [ ] **Step 6: Run migrations and verify**

```bash
cd backend
# Apply migrations (adjust to your migrate tool)
migrate -path migrations -database "$DATABASE_URL" up
```

- [ ] **Step 7: Build to verify compilation**

```bash
cd backend && go build ./...
```

- [ ] **Step 8: Commit**

```bash
git add backend/migrations/000019_configuracoes.up.sql \
       backend/migrations/000019_configuracoes.down.sql \
       backend/migrations/000020_sprint_review_analises.up.sql \
       backend/migrations/000020_sprint_review_analises.down.sql \
       backend/internal/repository/config.go
git commit -m "feat(review-ia): add configuracoes + sprint_review_analises tables and config repository"
```

---

### Task 2: OpenRouter Service

**Files:**
- Create: `backend/internal/service/openrouter.go`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `OpenRouterClient` struct
  - `NewOpenRouterClient(apiKey, model string) *OpenRouterClient`
  - `ChatCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error)` — returns raw text response
  - 60s timeout, 1 retry on failure

- [ ] **Step 1: Create OpenRouter client**

```go
// backend/internal/service/openrouter.go
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type OpenRouterClient struct {
	apiKey  string
	model   string
	baseURL string
}

func NewOpenRouterClient(apiKey, model string) *OpenRouterClient {
	return &OpenRouterClient{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://openrouter.ai/api/v1",
	}
}

type openRouterRequest struct {
	Model    string              `json:"model"`
	Messages []openRouterMessage `json:"messages"`
}

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func (c *OpenRouterClient) ChatCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	result, err := c.callAPI(ctx, systemPrompt, userPrompt)
	if err != nil {
		result, err = c.callAPI(ctx, systemPrompt, userPrompt)
		if err != nil {
			return "", fmt.Errorf("openrouter failed after retry: %w", err)
		}
	}
	return result, nil
}

func (c *OpenRouterClient) callAPI(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := openRouterRequest{
		Model: c.model,
		Messages: []openRouterMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling openrouter: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openrouter returned %d: %s", resp.StatusCode, string(respBody))
	}

	var orResp openRouterResponse
	if err := json.Unmarshal(respBody, &orResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if orResp.Error != nil {
		return "", fmt.Errorf("openrouter error: %s", orResp.Error.Message)
	}

	if len(orResp.Choices) == 0 || orResp.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("empty response from openrouter")
	}

	return orResp.Choices[0].Message.Content, nil
}
```

- [ ] **Step 2: Build to verify compilation**

```bash
cd backend && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add backend/internal/service/openrouter.go
git commit -m "feat(review-ia): add OpenRouter client with OpenAI-compatible chat completions"
```

---

### Task 3: Add estimativa_tempo to Review Data + Analysis Repository

**Files:**
- Modify: `backend/internal/repository/review.go` (ReviewTaskRow struct ~line 26, SQL query ~line 86, Scan ~line 126)
- Modify: `backend/internal/service/review.go` (ReviewTarefa struct ~line 69, tarefaList append ~line 223)
- Modify: `backend/internal/repository/review.go` — add analysis CRUD methods

**Interfaces:**
- Consumes: `ReviewTaskRow`, `ReviewTarefa` existing structs
- Produces:
  - `ReviewTaskRow.EstimativaTempo *int` field (seconds, nullable)
  - `ReviewTarefa.EstimativaHoras float64` field (hours, converted)
  - `ReviewAnalise` struct: `ID uuid.UUID`, `SprintID uuid.UUID`, `EquipeID uuid.UUID`, `ProdutoIDs []uuid.UUID`, `AnaliseJSON json.RawMessage`, `Modelo string`, `CriadoEm time.Time`
  - `GetReviewAnalise(ctx, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*ReviewAnalise, error)`
  - `SaveReviewAnalise(ctx, analise ReviewAnalise) error` — upsert by sprint+equipe+produtos

- [ ] **Step 1: Add EstimativaTempo to ReviewTaskRow struct**

In `backend/internal/repository/review.go`, add field to `ReviewTaskRow` struct after `NaoPlanejada`:

```go
// Add after NaoPlanejada bool field (line ~35)
EstimativaTempo *int `json:"estimativa_tempo"`
```

- [ ] **Step 2: Add estimativa_tempo to SQL SELECT**

In `backend/internal/repository/review.go`, in the `GetReviewTasks` query, add `t.estimativa_tempo` after the `nao_planejada` CASE expression (before the ARRAY_AGG lines):

```sql
-- After the nao_planejada CASE WHEN line, add:
       t.estimativa_tempo,
```

The SELECT should now read:
```sql
SELECT t.id, t.numero_ticket, t.resumo, t.tipo,
       COALESCE(t.tipo_demanda, ...),
       t.status,
       t.parent_id, m.nome,
       CASE WHEN ... THEN true ELSE false END AS nao_planejada,
       t.estimativa_tempo,
       ARRAY_AGG(...) AS produtos,
       ARRAY_AGG(...) AS produto_ids
```

- [ ] **Step 3: Add estimativa_tempo to Scan**

In the `rows.Scan(...)` call (~line 126), add `&row.EstimativaTempo` after `&row.NaoPlanejada` and before `&row.Produtos`:

```go
if err := rows.Scan(
    &row.ID, &row.NumeroTicket, &row.Resumo, &row.Tipo, &row.TipoDemanda,
    &row.Status, &row.ParentID, &row.RelatorNome, &row.NaoPlanejada,
    &row.EstimativaTempo,
    &row.Produtos, &row.ProdutoIDs,
); err != nil {
```

- [ ] **Step 4: Add EstimativaHoras to ReviewTarefa struct**

In `backend/internal/service/review.go`, add field to `ReviewTarefa` struct after `NaoPlanejada`:

```go
EstimativaHoras float64 `json:"estimativa_horas"`
```

- [ ] **Step 5: Populate EstimativaHoras in tarefaList append**

In `backend/internal/service/review.go`, in the `tarefaList = append(tarefaList, ReviewTarefa{...})` block (~line 223), add after `NaoPlanejada`:

```go
EstimativaHoras: func() float64 {
    if t.EstimativaTempo != nil {
        return float64(*t.EstimativaTempo) / 3600.0
    }
    return 0
}(),
```

- [ ] **Step 6: Add ReviewAnalise struct and CRUD to review repository**

Append to `backend/internal/repository/review.go` (after `DeleteDestaque` method):

```go
type ReviewAnalise struct {
	ID          uuid.UUID       `json:"id"`
	SprintID    uuid.UUID       `json:"sprint_id"`
	EquipeID    uuid.UUID       `json:"equipe_id"`
	ProdutoIDs  []uuid.UUID     `json:"produto_ids"`
	AnaliseJSON json.RawMessage `json:"analise_json"`
	Modelo      string          `json:"modelo"`
	CriadoEm    time.Time       `json:"criado_em"`
}

func (r *ReviewRepository) GetReviewAnalise(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*ReviewAnalise, error) {
	if produtoIDs == nil {
		produtoIDs = []uuid.UUID{}
	}
	var a ReviewAnalise
	err := r.pool.QueryRow(ctx, `
		SELECT id, sprint_id, equipe_id, produto_ids, analise_json, modelo, criado_em
		FROM sprint_review_analises
		WHERE sprint_id = $1 AND equipe_id = $2 AND produto_ids = $3
	`, sprintID, equipeID, produtoIDs).Scan(
		&a.ID, &a.SprintID, &a.EquipeID, &a.ProdutoIDs,
		&a.AnaliseJSON, &a.Modelo, &a.CriadoEm,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *ReviewRepository) SaveReviewAnalise(ctx context.Context, a ReviewAnalise) error {
	if a.ProdutoIDs == nil {
		a.ProdutoIDs = []uuid.UUID{}
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sprint_review_analises (sprint_id, equipe_id, produto_ids, analise_json, modelo)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (sprint_id, equipe_id, produto_ids)
		DO UPDATE SET analise_json = $4, modelo = $5, criado_em = NOW()
	`, a.SprintID, a.EquipeID, a.ProdutoIDs, a.AnaliseJSON, a.Modelo)
	if err != nil {
		return fmt.Errorf("saving review analise: %w", err)
	}
	return nil
}
```

Add `"encoding/json"` to the imports of `review.go` repository file if not already present.

- [ ] **Step 7: Build to verify compilation**

```bash
cd backend && go build ./...
```

- [ ] **Step 8: Commit**

```bash
git add backend/internal/repository/review.go backend/internal/service/review.go
git commit -m "feat(review-ia): add estimativa_tempo to review data and analysis CRUD repository"
```

---

### Task 4: Review Analysis Service + Prompt Builder

**Files:**
- Modify: `backend/internal/service/review.go` — add `GenerateAnalise` method and prompt builder

**Interfaces:**
- Consumes:
  - `OpenRouterClient.ChatCompletion(ctx, systemPrompt, userPrompt string) (string, error)`
  - `ReviewRepository.GetReviewTasks(ctx, sprintID, equipeID, produtoIDs) ([]ReviewTaskRow, error)`
  - `ReviewRepository.GetReviewAnalise(ctx, sprintID, equipeID, produtoIDs) (*ReviewAnalise, error)`
  - `ReviewRepository.SaveReviewAnalise(ctx, analise ReviewAnalise) error`
  - `ConfigRepository.GetConfig(ctx, chave string) (string, error)`
  - `ReviewTarefa.EstimativaHoras float64`
- Produces:
  - `ReviewService.GenerateAnalise(ctx, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*repository.ReviewAnalise, error)`
  - `ReviewService.GetAnalise(ctx, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*repository.ReviewAnalise, error)`
  - Updated `NewReviewService` accepting `*repository.ConfigRepository`

- [ ] **Step 1: Update ReviewService struct to include ConfigRepository**

In `backend/internal/service/review.go`, update the struct and constructor:

```go
type ReviewService struct {
	repo       *repository.ReviewRepository
	configRepo *repository.ConfigRepository
	logger     *zap.Logger
}

func NewReviewService(repo *repository.ReviewRepository, configRepo *repository.ConfigRepository, logger *zap.Logger) *ReviewService {
	return &ReviewService{repo: repo, configRepo: configRepo, logger: logger}
}
```

- [ ] **Step 2: Add GetAnalise method**

```go
func (s *ReviewService) GetAnalise(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*repository.ReviewAnalise, error) {
	return s.repo.GetReviewAnalise(ctx, sprintID, equipeID, produtoIDs)
}
```

- [ ] **Step 3: Add prompt builder helper**

```go
type promptTarefa struct {
	Ticket         string  `json:"ticket"`
	Resumo         string  `json:"resumo"`
	Tipo           string  `json:"tipo"`
	TipoDemanda    string  `json:"tipo_demanda"`
	Status         string  `json:"status"`
	Produto        string  `json:"produto"`
	NaoPlanejada   bool    `json:"nao_planejada"`
	EstimativaHoras float64 `json:"estimativa_horas"`
}

func buildReviewAnalisePrompt(tarefas []ReviewTarefa) (string, string) {
	systemPrompt := `Você é um analista de sprints de desenvolvimento de software.
Analise os dados da sprint e retorne um JSON com a análise separada por produto.

REGRAS:
1. Foco da Sprint: identifique onde a maior parte das horas estimadas foi gasta por produto
2. Top 3 Entregas: as 3 tarefas com maior estimativa por produto. Se tipo_demanda for "Meta" ou "Compromisso", marque destaque=true
3. Incidentes: avalie todos com tipo "Bug" ou contendo "Incidente". Se houver causa raiz similar entre eles, informe na causa_comum
4. Não Planejadas: liste tarefas com nao_planejada=true EXCLUINDO bugs e incidentes. Informe horas e percentual

Responda APENAS com JSON válido (sem markdown fences) no formato:
{
  "analises_por_produto": [
    {
      "produto": "Nome",
      "foco_sprint": {
        "descricao": "texto",
        "categoria_principal": "melhorias|manutencao|novos_projetos|outros",
        "horas_estimadas": 0
      },
      "top3_entregas": [
        {"ticket": "", "resumo": "", "tipo_demanda": "", "destaque": false, "horas_estimadas": 0}
      ],
      "analise_incidentes": {
        "total": 0,
        "resumo": "texto",
        "causa_comum": "texto ou null",
        "incidentes": [{"ticket": "", "resumo": "", "horas_estimadas": 0}]
      },
      "nao_planejadas": {
        "total": 0,
        "horas_total": 0,
        "percentual_sprint": 0,
        "resumo": "texto",
        "tarefas": [{"ticket": "", "resumo": "", "produto": "", "horas_estimadas": 0}]
      }
    }
  ]
}`

	pts := make([]promptTarefa, 0, len(tarefas))
	for _, t := range tarefas {
		pts = append(pts, promptTarefa{
			Ticket:          t.NumeroTicket,
			Resumo:          t.Resumo,
			Tipo:            t.Tipo,
			TipoDemanda:     t.TipoDemanda,
			Status:          t.Status,
			Produto:         t.Produto,
			NaoPlanejada:    t.NaoPlanejada,
			EstimativaHoras: t.EstimativaHoras,
		})
	}

	data, _ := json.Marshal(pts)
	userPrompt := "DADOS DA SPRINT:\n" + string(data)

	return systemPrompt, userPrompt
}
```

- [ ] **Step 4: Add GenerateAnalise method**

```go
func (s *ReviewService) GenerateAnalise(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*repository.ReviewAnalise, error) {
	apiKey, err := s.configRepo.GetConfig(ctx, "openrouter_api_key")
	if err != nil {
		return nil, fmt.Errorf("openrouter API key not configured: %w", err)
	}

	model := "openai/gpt-oss-20b:free"
	if m, err := s.configRepo.GetConfig(ctx, "openrouter_model"); err == nil && m != "" {
		model = m
	}

	reviewData, err := s.GetReviewData(ctx, sprintID, equipeID, produtoIDs)
	if err != nil {
		return nil, fmt.Errorf("getting review data for analysis: %w", err)
	}

	systemPrompt, userPrompt := buildReviewAnalisePrompt(reviewData.Tarefas)

	client := NewOpenRouterClient(apiKey, model)
	rawResponse, err := client.ChatCompletion(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("AI analysis failed: %w", err)
	}

	// Strip markdown fences if present
	cleaned := strings.TrimSpace(rawResponse)
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.Split(cleaned, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
			lines = lines[:len(lines)-1]
		}
		cleaned = strings.Join(lines, "\n")
	}

	var parsed json.RawMessage
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return nil, fmt.Errorf("AI returned invalid JSON: %w", err)
	}

	analise := repository.ReviewAnalise{
		SprintID:    sprintID,
		EquipeID:    equipeID,
		ProdutoIDs:  produtoIDs,
		AnaliseJSON: parsed,
		Modelo:      model,
	}

	if err := s.repo.SaveReviewAnalise(ctx, analise); err != nil {
		return nil, fmt.Errorf("saving analysis: %w", err)
	}

	saved, err := s.repo.GetReviewAnalise(ctx, sprintID, equipeID, produtoIDs)
	if err != nil {
		return nil, fmt.Errorf("retrieving saved analysis: %w", err)
	}

	return saved, nil
}
```

- [ ] **Step 5: Add "encoding/json" to imports if missing, ensure "strings" is imported**

Check imports at top of `backend/internal/service/review.go` — add `"encoding/json"` if not present (it should already have `"strings"`).

- [ ] **Step 6: Build to verify compilation**

```bash
cd backend && go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/review.go
git commit -m "feat(review-ia): add AI analysis generation with OpenRouter prompt builder"
```

---

### Task 5: Config + Analysis Handler Endpoints + Wiring

**Files:**
- Modify: `backend/internal/handler/review.go` — add config and analysis endpoints, update ReviewStore interface
- Modify: `backend/cmd/api/main.go` — wire ConfigRepository, update NewReviewService call, register new routes

**Interfaces:**
- Consumes:
  - `ConfigRepository.GetConfig`, `SetConfig`, `ConfigExists`
  - `ReviewService.GenerateAnalise`, `GetAnalise`
  - `ReviewService` now requires `*ConfigRepository` in constructor
- Produces:
  - `GET /api/v1/config/{chave}` endpoint
  - `POST /api/v1/config` endpoint
  - `GET /api/v1/sprints/{id}/review/analise` endpoint
  - `POST /api/v1/sprints/{id}/review/analise` endpoint

- [ ] **Step 1: Update ReviewStore interface**

In `backend/internal/handler/review.go`, add methods to `ReviewStore` interface:

```go
type ReviewStore interface {
	GetReviewData(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*service.ReviewData, error)
	ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error)
	CreateDestaque(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error)
	UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error)
	DeleteDestaque(ctx context.Context, id uuid.UUID) error
	GenerateAnalise(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*repository.ReviewAnalise, error)
	GetAnalise(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*repository.ReviewAnalise, error)
}
```

- [ ] **Step 2: Add ConfigStore interface and update ReviewHandler struct**

```go
type ConfigStore interface {
	GetConfig(ctx context.Context, chave string) (string, error)
	SetConfig(ctx context.Context, chave, valor string) error
	ConfigExists(ctx context.Context, chave string) (bool, error)
}

type ReviewHandler struct {
	store       ReviewStore
	configStore ConfigStore
	logger      *zap.Logger
}

func NewReviewHandler(store ReviewStore, configStore ConfigStore, logger *zap.Logger) *ReviewHandler {
	return &ReviewHandler{store: store, configStore: configStore, logger: logger}
}
```

- [ ] **Step 3: Add GetConfig handler**

Config whitelist: only `openrouter_api_key` and `openrouter_model` allowed. For `openrouter_api_key`, return only existence, never the value.

```go
var configWhitelist = map[string]bool{
	"openrouter_api_key": true,
	"openrouter_model":   true,
}

func (h *ReviewHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	chave := chi.URLParam(r, "chave")
	if !configWhitelist[chave] {
		respondError(w, http.StatusBadRequest, "config key not allowed")
		return
	}

	if chave == "openrouter_api_key" {
		exists, err := h.configStore.ConfigExists(r.Context(), chave)
		if err != nil {
			h.logger.Error("checking config", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "error checking config")
			return
		}
		respondJSON(w, http.StatusOK, map[string]bool{"exists": exists})
		return
	}

	valor, err := h.configStore.GetConfig(r.Context(), chave)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "config not found")
			return
		}
		h.logger.Error("getting config", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "error getting config")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"chave": chave, "valor": valor})
}
```

- [ ] **Step 4: Add SetConfig handler**

```go
type setConfigRequest struct {
	Chave string `json:"chave"`
	Valor string `json:"valor"`
}

func (h *ReviewHandler) SetConfig(w http.ResponseWriter, r *http.Request) {
	var req setConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !configWhitelist[req.Chave] {
		respondError(w, http.StatusBadRequest, "config key not allowed")
		return
	}
	if req.Valor == "" {
		respondError(w, http.StatusBadRequest, "valor is required")
		return
	}

	if err := h.configStore.SetConfig(r.Context(), req.Chave, req.Valor); err != nil {
		h.logger.Error("setting config", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "error setting config")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 5: Add GetReviewAnalise handler**

```go
func (h *ReviewHandler) GetReviewAnalise(w http.ResponseWriter, r *http.Request) {
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

	analise, err := h.store.GetAnalise(r.Context(), sprintID, equipeID, produtoIDs)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "analysis not found")
			return
		}
		h.logger.Error("getting review analysis", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "error getting analysis")
		return
	}

	respondJSON(w, http.StatusOK, analise)
}
```

- [ ] **Step 6: Add PostReviewAnalise handler**

```go
func (h *ReviewHandler) PostReviewAnalise(w http.ResponseWriter, r *http.Request) {
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

	analise, err := h.store.GenerateAnalise(r.Context(), sprintID, equipeID, produtoIDs)
	if err != nil {
		h.logger.Error("generating review analysis", zap.Error(err))
		if strings.Contains(err.Error(), "not configured") {
			respondError(w, http.StatusServiceUnavailable, "OpenRouter API key not configured")
			return
		}
		respondError(w, http.StatusInternalServerError, "error generating analysis: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, analise)
}
```

- [ ] **Step 7: Wire in main.go**

In `backend/cmd/api/main.go`:

1. After `reviewRepo` creation (~line 142), add ConfigRepository:

```go
configRepo := repository.NewConfigRepository(pool)
```

2. Update `NewReviewService` call to pass configRepo:

```go
reviewService := service.NewReviewService(reviewRepo, configRepo, logger)
```

3. Update `NewReviewHandler` call to pass configRepo:

```go
reviewHandler := handler.NewReviewHandler(reviewService, configRepo, logger)
```

4. Add new routes after the existing review routes (~line 256):

```go
r.Get("/sprints/{id}/review/analise", reviewHandler.GetReviewAnalise)
r.Post("/sprints/{id}/review/analise", reviewHandler.PostReviewAnalise)

r.Get("/config/{chave}", reviewHandler.GetConfig)
r.Post("/config", reviewHandler.SetConfig)
```

- [ ] **Step 8: Build to verify compilation**

```bash
cd backend && go build ./...
```

- [ ] **Step 9: Commit**

```bash
git add backend/internal/handler/review.go backend/cmd/api/main.go
git commit -m "feat(review-ia): add config and analysis handler endpoints with route wiring"
```

---

### Task 6: Frontend — Button, API Key Modal, Analysis Cards, Export

**Files:**
- Modify: `frontend/index.html` — add CSS styles, API key modal, analysis button/cards rendering, export integration

**Interfaces:**
- Consumes:
  - `GET /api/v1/config/openrouter_api_key` → `{"exists": true/false}`
  - `POST /api/v1/config` → `{"chave": "openrouter_api_key", "valor": "..."}`
  - `GET /api/v1/sprints/{id}/review/analise?equipe_id=...&produtos=...` → `ReviewAnalise` JSON
  - `POST /api/v1/sprints/{id}/review/analise?equipe_id=...&produtos=...` → `ReviewAnalise` JSON
  - `_reviewData` global variable (set by `loadSprintReview`)
  - `_currentSprintId`, `_currentEquipeId`, `_currentProdutoIds` globals
- Produces:
  - `loadReviewAnalise()` function — loads cached analysis
  - `generateReviewAnalise()` function — triggers AI generation
  - `renderAnaliseCards(analiseJSON)` function — renders structured cards
  - `showApiKeyModal()` function — API key configuration modal
  - Analysis cards HTML between pie charts div and task tables

- [ ] **Step 1: Add CSS styles for analysis cards**

In the `<style>` section of `index.html`, find the `.review-destaques` styles (~line 488) and add after them:

```css
.review-analise-section { margin: 24px 0; }
.review-analise-btn-bar { display: flex; gap: 8px; align-items: center; margin-bottom: 16px; }
.review-analise-btn { display: inline-flex; align-items: center; gap: 6px; padding: 8px 16px; border: none; border-radius: 6px; background: #7C3AED; color: #fff; font-size: 13px; font-weight: 600; cursor: pointer; }
.review-analise-btn:hover { background: #6D28D9; }
.review-analise-btn:disabled { opacity: 0.6; cursor: not-allowed; }
.review-analise-btn-settings { padding: 8px; border: 1px solid #ddd; border-radius: 6px; background: #fff; cursor: pointer; font-size: 14px; }
.review-analise-btn-regen { padding: 8px 12px; border: 1px solid #7C3AED; border-radius: 6px; background: #fff; color: #7C3AED; font-size: 12px; font-weight: 600; cursor: pointer; }
.review-analise-produto { background: #f8f7ff; border: 1px solid #e5e1f5; border-radius: 10px; margin-bottom: 16px; overflow: hidden; }
.review-analise-produto-header { background: #7C3AED; color: #fff; padding: 10px 16px; font-weight: 700; font-size: 14px; }
.review-analise-card { padding: 14px 16px; border-bottom: 1px solid #e5e1f5; }
.review-analise-card:last-child { border-bottom: none; }
.review-analise-card-title { font-weight: 700; font-size: 13px; margin-bottom: 6px; display: flex; align-items: center; gap: 6px; }
.review-analise-card-body { font-size: 12px; line-height: 1.5; color: #444; }
.review-analise-card-body ul { margin: 6px 0 0; padding-left: 16px; }
.review-analise-card-body li { margin-bottom: 4px; }
.review-analise-badge-meta { display: inline-block; padding: 1px 6px; border-radius: 3px; font-size: 10px; font-weight: 700; background: #F59E0B; color: #fff; }
.review-analise-badge-compromisso { display: inline-block; padding: 1px 6px; border-radius: 3px; font-size: 10px; font-weight: 700; background: #3B82F6; color: #fff; }
.review-analise-loading { text-align: center; padding: 24px; color: #7C3AED; font-size: 13px; }
.review-analise-loading .spinner { display: inline-block; width: 18px; height: 18px; border: 2px solid #7C3AED; border-top-color: transparent; border-radius: 50%; animation: spin 0.8s linear infinite; margin-right: 8px; vertical-align: middle; }
```

- [ ] **Step 2: Add API Key modal HTML**

In the `<body>` section, after existing modals (search for the last `</dialog>` or modal div), add:

```html
<div id="apikey-modal" style="display:none;position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.5);z-index:9999;justify-content:center;align-items:center">
  <div style="background:#fff;border-radius:10px;padding:24px;max-width:420px;width:90%">
    <h3 style="margin:0 0 12px;font-size:16px">Configurar API OpenRouter</h3>
    <p style="font-size:12px;color:#666;margin:0 0 16px">Insira sua chave da API OpenRouter para habilitar análise por IA. Obtenha em openrouter.ai</p>
    <input id="apikey-input" type="password" placeholder="sk-or-..." style="width:100%;padding:8px 12px;border:1px solid #ddd;border-radius:6px;font-size:13px;box-sizing:border-box;margin-bottom:12px">
    <div style="display:flex;gap:8px;justify-content:flex-end">
      <button onclick="document.getElementById('apikey-modal').style.display='none'" style="padding:8px 16px;border:1px solid #ddd;border-radius:6px;background:#fff;cursor:pointer;font-size:13px">Cancelar</button>
      <button onclick="saveApiKey()" style="padding:8px 16px;border:none;border-radius:6px;background:#7C3AED;color:#fff;cursor:pointer;font-size:13px;font-weight:600">Salvar</button>
    </div>
  </div>
</div>
```

- [ ] **Step 3: Add analysis JS functions**

In the `<script>` section, after `loadReviewDestaques` function (~line 2918 area), add:

```javascript
function showApiKeyModal() {
  var modal = document.getElementById('apikey-modal');
  modal.style.display = 'flex';
  document.getElementById('apikey-input').value = '';
  document.getElementById('apikey-input').focus();
}

function saveApiKey() {
  var key = document.getElementById('apikey-input').value.trim();
  if (!key) return;
  fetch('/api/v1/config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + localStorage.getItem('token') },
    body: JSON.stringify({ chave: 'openrouter_api_key', valor: key })
  }).then(function(r) {
    if (!r.ok) throw new Error('Erro ao salvar chave');
    document.getElementById('apikey-modal').style.display = 'none';
    generateReviewAnalise();
  }).catch(function(e) { alert(e.message); });
}

function generateReviewAnalise() {
  var container = document.getElementById('review-analise-cards');
  if (!container) return;
  container.innerHTML = '<div class="review-analise-loading"><span class="spinner"></span>Gerando análise com IA...</div>';

  var btn = document.getElementById('review-analise-gen-btn');
  if (btn) btn.disabled = true;

  var sprintId = _currentSprintId;
  var equipeId = _currentEquipeId;
  var produtos = _currentProdutoIds || [];
  var qs = 'equipe_id=' + equipeId;
  if (produtos.length) qs += '&produtos=' + produtos.join(',');

  fetch('/api/v1/sprints/' + sprintId + '/review/analise?' + qs, {
    method: 'POST',
    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
  }).then(function(r) {
    if (r.status === 503) { showApiKeyModal(); container.innerHTML = ''; if (btn) btn.disabled = false; return null; }
    if (!r.ok) return r.json().then(function(e) { throw new Error(e.error || 'Erro ao gerar análise'); });
    return r.json();
  }).then(function(data) {
    if (!data) return;
    if (btn) btn.disabled = false;
    renderAnaliseCards(container, data.analise_json);
    document.getElementById('review-analise-regen-btn').style.display = 'inline-flex';
  }).catch(function(e) {
    container.innerHTML = '<div style="color:#D4483B;padding:12px;font-size:13px">Erro: ' + esc(e.message) + '</div>';
    if (btn) btn.disabled = false;
  });
}

function loadReviewAnalise() {
  var container = document.getElementById('review-analise-cards');
  if (!container) return;

  var sprintId = _currentSprintId;
  var equipeId = _currentEquipeId;
  var produtos = _currentProdutoIds || [];
  var qs = 'equipe_id=' + equipeId;
  if (produtos.length) qs += '&produtos=' + produtos.join(',');

  fetch('/api/v1/sprints/' + sprintId + '/review/analise?' + qs, {
    headers: { 'Authorization': 'Bearer ' + localStorage.getItem('token') }
  }).then(function(r) {
    if (r.status === 404) return null;
    if (!r.ok) return null;
    return r.json();
  }).then(function(data) {
    if (!data) return;
    renderAnaliseCards(container, data.analise_json);
    document.getElementById('review-analise-regen-btn').style.display = 'inline-flex';
  }).catch(function() {});
}

function renderAnaliseCards(container, analiseJSON) {
  var analise = typeof analiseJSON === 'string' ? JSON.parse(analiseJSON) : analiseJSON;
  var produtos = analise.analises_por_produto || [];
  if (produtos.length === 0) { container.innerHTML = '<div style="color:#666;padding:12px;font-size:13px">Nenhuma análise disponível.</div>'; return; }

  var html = '';
  produtos.forEach(function(p) {
    html += '<div class="review-analise-produto">';
    html += '<div class="review-analise-produto-header">' + esc(p.produto || 'Geral') + '</div>';

    // Foco
    if (p.foco_sprint) {
      html += '<div class="review-analise-card">';
      html += '<div class="review-analise-card-title">📊 Foco da Sprint</div>';
      html += '<div class="review-analise-card-body">';
      html += '<div>' + esc(p.foco_sprint.descricao || '') + '</div>';
      if (p.foco_sprint.horas_estimadas) html += '<div style="margin-top:4px;font-weight:600">Total estimado: ' + p.foco_sprint.horas_estimadas.toFixed(1) + 'h</div>';
      html += '</div></div>';
    }

    // Top 3
    if (p.top3_entregas && p.top3_entregas.length) {
      html += '<div class="review-analise-card">';
      html += '<div class="review-analise-card-title">🏆 Top 3 Entregas</div>';
      html += '<div class="review-analise-card-body"><ul>';
      p.top3_entregas.forEach(function(e, i) {
        var badge = '';
        if (e.destaque && e.tipo_demanda === 'Meta') badge = ' <span class="review-analise-badge-meta">Meta ⭐</span>';
        else if (e.destaque && e.tipo_demanda === 'Compromisso') badge = ' <span class="review-analise-badge-compromisso">Compromisso ⭐</span>';
        html += '<li><strong>' + (i + 1) + '. ' + esc(e.ticket || '') + '</strong> — ' + esc(e.resumo || '') + badge;
        if (e.horas_estimadas) html += ' <span style="color:#888">(' + e.horas_estimadas.toFixed(1) + 'h)</span>';
        html += '</li>';
      });
      html += '</ul></div></div>';
    }

    // Incidentes
    if (p.analise_incidentes && p.analise_incidentes.total > 0) {
      html += '<div class="review-analise-card">';
      html += '<div class="review-analise-card-title">🚨 Incidentes (' + p.analise_incidentes.total + ')</div>';
      html += '<div class="review-analise-card-body">';
      if (p.analise_incidentes.resumo) html += '<div>' + esc(p.analise_incidentes.resumo) + '</div>';
      if (p.analise_incidentes.causa_comum) html += '<div style="margin-top:4px;padding:6px 10px;background:#FEF2F2;border-radius:4px;color:#991B1B;font-size:11px"><strong>Causa comum:</strong> ' + esc(p.analise_incidentes.causa_comum) + '</div>';
      if (p.analise_incidentes.incidentes && p.analise_incidentes.incidentes.length) {
        html += '<ul>';
        p.analise_incidentes.incidentes.forEach(function(inc) {
          html += '<li>' + esc(inc.ticket || '') + ' — ' + esc(inc.resumo || '');
          if (inc.horas_estimadas) html += ' <span style="color:#888">(' + inc.horas_estimadas.toFixed(1) + 'h)</span>';
          html += '</li>';
        });
        html += '</ul>';
      }
      html += '</div></div>';
    }

    // Não Planejadas
    if (p.nao_planejadas && p.nao_planejadas.total > 0) {
      html += '<div class="review-analise-card">';
      html += '<div class="review-analise-card-title">📋 Não Planejadas (excl. bugs/incidentes)</div>';
      html += '<div class="review-analise-card-body">';
      html += '<div style="font-weight:600">Total: ' + p.nao_planejadas.total + ' tarefas | ' + (p.nao_planejadas.horas_total || 0).toFixed(1) + 'h (' + (p.nao_planejadas.percentual_sprint || 0).toFixed(1) + '% da sprint)</div>';
      if (p.nao_planejadas.resumo) html += '<div style="margin-top:4px">' + esc(p.nao_planejadas.resumo) + '</div>';
      if (p.nao_planejadas.tarefas && p.nao_planejadas.tarefas.length) {
        html += '<ul>';
        p.nao_planejadas.tarefas.forEach(function(t) {
          html += '<li>' + esc(t.ticket || '') + ' — ' + esc(t.resumo || '');
          if (t.horas_estimadas) html += ' <span style="color:#888">(' + t.horas_estimadas.toFixed(1) + 'h)</span>';
          html += '</li>';
        });
        html += '</ul>';
      }
      html += '</div></div>';
    }

    html += '</div>';
  });

  container.innerHTML = html;
}
```

- [ ] **Step 4: Add analysis section to renderReviewContent**

In `renderReviewContent` function, after the destaques placeholder line:

```javascript
html += '<div id="review-destaques-container" class="review-destaques"><h3>Destaques</h3><div id="review-destaques-list"></div></div>';
```

Add the analysis section:

```javascript
  // AI Analysis section
  html += '<div class="review-analise-section">';
  html += '<div class="review-analise-btn-bar">';
  html += '<button id="review-analise-gen-btn" class="review-analise-btn" onclick="generateReviewAnalise()">🤖 Gerar Análise IA</button>';
  html += '<button class="review-analise-btn-settings" onclick="showApiKeyModal()" title="Configurar API Key">⚙️</button>';
  html += '<button id="review-analise-regen-btn" class="review-analise-btn-regen" onclick="generateReviewAnalise()" style="display:none">🔄 Regerar</button>';
  html += '</div>';
  html += '<div id="review-analise-cards"></div>';
  html += '</div>';
```

- [ ] **Step 5: Load cached analysis on review tab open**

At the end of `renderReviewContent`, after the `loadReviewDestaques()` call:

```javascript
  // Load cached AI analysis
  loadReviewAnalise();
```

- [ ] **Step 6: Store current IDs for analysis calls**

In `loadSprintReview` function (~line 2460), right after `_reviewData = data;`, ensure the current sprint/equipe/produto IDs are stored. Find where `_reviewData = data;` is set and add nearby:

```javascript
_currentSprintId = sprintId; // (the sprintId variable from the loadSprintReview function scope)
_currentEquipeId = equipeId; // (from the function scope)
_currentProdutoIds = produtoIDs; // (from the function scope)
```

Declare these as global variables near `var _reviewData = null;`:

```javascript
var _currentSprintId = null;
var _currentEquipeId = null;
var _currentProdutoIds = [];
```

Verify the variable names match what `loadSprintReview` uses internally for sprint ID, equipe ID, and produto IDs — read the function to confirm the local variable names, then assign them to the globals.

- [ ] **Step 7: Add analysis cards to export**

In the export HTML builder function (the one that builds `buildReviewExportHTML` or inline in `exportReviewImage`/`exportReviewPDF`), after the charts section and before the task tables, add:

```javascript
// If analysis cards exist, include in export
var analiseCards = document.getElementById('review-analise-cards');
if (analiseCards && analiseCards.innerHTML.trim() && !analiseCards.querySelector('.review-analise-loading')) {
  html += '<div style="margin:16px 0">';
  html += '<h3 style="font-size:14px;margin-bottom:8px">Análise IA</h3>';
  html += analiseCards.innerHTML;
  html += '</div>';
}
```

Find the exact location in the export builder by searching for where pie charts HTML ends and task table HTML begins in the export function. Insert the analysis cards between them.

- [ ] **Step 8: Build backend and test**

```bash
cd backend && go build ./...
```

Start dev server and test:
1. Open Review tab → verify "Gerar Análise IA" button appears
2. Click button → should show API key modal (first time)
3. Enter key → should start generating
4. Verify cards render correctly
5. Reload page → verify cached analysis loads
6. Click "Regerar" → verify new analysis generated
7. Test export with analysis cards

- [ ] **Step 9: Commit**

```bash
git add frontend/index.html
git commit -m "feat(review-ia): add AI analysis button, API key modal, and structured cards in review tab"
```
