# Equalizador de Capacidade — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a capacity equalizer tool to the sprint detail that suggests and applies task reassignments between overloaded and underloaded team members via JIRA API.

**Architecture:** The equalizer reuses `SprintService.GetCapacity()` for capacity data, runs a greedy algorithm server-side to generate transfer suggestions, and exposes two endpoints — GET for suggestions, POST to apply. The POST endpoint calls JIRA API to reassign issues and add comments, then updates the local DB. Frontend renders a modal with before/after visualization and selectable task transfers.

**Tech Stack:** Go backend (chi router, pgx, zap), JIRA REST API v3, vanilla JS frontend (single-file SPA).

## Global Constraints

- Capacity measured in hours (6h/day constant), not story points.
- Only `da_equipe = true` members participate. Externos and desligados excluded.
- Doador threshold: `percentual_alocacao > 100%`. Receptor threshold: `< 70%`.
- Transfer minimum: ≥10% capacity shift per transfer. Max 10 suggestions.
- Candidatas: tasks where status is not in-progress or done (using `status` field names, same as GetCapacity: excluding statuses in `statusExecutado` and `statusAmbos` maps).
- JIRA API uses `PUT /rest/api/3/issue/{issueKey}/assignee` with `{"accountId":"..."}` for reassignment.
- JIRA API uses `POST /rest/api/3/issue/{issueKey}/comment` for commenting.

---

### Task 1: Add JIRA Client methods for issue mutation

**Files:**
- Modify: `backend/internal/jira/client.go:16-26` (Client interface)
- Modify: `backend/internal/jira/client.go:69-78` (add doPut helper)

**Interfaces:**
- Consumes: existing `doRequest(ctx, method, path, body)` method
- Produces: `Client.AssignIssue(ctx, issueKey, accountID string) error` and `Client.AddComment(ctx, issueKey, body string) error` — used by Task 3 (EqualizerService.Apply)

- [ ] **Step 1: Add `doPut` helper to HTTPClient**

In `backend/internal/jira/client.go`, add after the `doPost` method (line 79):

```go
func (c *HTTPClient) doPut(ctx context.Context, path string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling payload: %w", err)
	}
	return c.doRequest(ctx, http.MethodPut, path, data)
}
```

- [ ] **Step 2: Add AssignIssue and AddComment to Client interface**

In `backend/internal/jira/client.go`, add to the `Client` interface (after `CreateSprint`):

```go
AssignIssue(ctx context.Context, issueKey, accountID string) error
AddComment(ctx context.Context, issueKey, body string) error
```

- [ ] **Step 3: Implement AssignIssue on HTTPClient**

Add at end of `backend/internal/jira/client.go`:

```go
func (c *HTTPClient) AssignIssue(ctx context.Context, issueKey, accountID string) error {
	payload := map[string]string{"accountId": accountID}
	_, err := c.doPut(ctx, "/rest/api/3/issue/"+issueKey+"/assignee", payload)
	return err
}
```

- [ ] **Step 4: Implement AddComment on HTTPClient**

Add at end of `backend/internal/jira/client.go`:

```go
func (c *HTTPClient) AddComment(ctx context.Context, issueKey, body string) error {
	payload := map[string]any{
		"body": map[string]any{
			"type":    "doc",
			"version": 1,
			"content": []any{
				map[string]any{
					"type": "paragraph",
					"content": []any{
						map[string]any{"type": "text", "text": body},
					},
				},
			},
		},
	}
	_, err := c.doPost(ctx, "/rest/api/3/issue/"+issueKey+"/comment", payload)
	return err
}
```

- [ ] **Step 5: Verify compilation**

Run: `cd backend && go build ./...`
Expected: compiles without errors

- [ ] **Step 6: Commit**

```bash
git add backend/internal/jira/client.go
git commit -m "feat: add AssignIssue and AddComment methods to JIRA client"
```

---

### Task 2: Add Equalizer repository query

**Files:**
- Modify: `backend/internal/repository/sprint.go` (add new query method)

**Interfaces:**
- Consumes: `tarefas` and `membros` tables
- Produces: `SprintRepository.GetEqualizerTarefas(ctx, sprintID, membroID) ([]EqualizerTarefa, error)` — used by Task 3

- [ ] **Step 1: Add EqualizerTarefa struct**

In `backend/internal/repository/sprint.go`, add after `TarefaDetail` struct (after line 247):

```go
type EqualizerTarefa struct {
	ID             uuid.UUID `json:"id"`
	NumeroTicket   string    `json:"numero_ticket"`
	Resumo         string    `json:"resumo"`
	Tipo           string    `json:"tipo"`
	Status         string    `json:"status"`
	Prioridade     *string   `json:"prioridade"`
	Horas          float64   `json:"horas"`
	ResponsavelID  uuid.UUID `json:"-"`
}
```

- [ ] **Step 2: Add GetEqualizerTarefas method**

Query returns non-started tasks for a specific member in a sprint, ordered by hours descending (biggest tasks first for greedy algorithm). Uses status names consistent with GetCapacity — excludes statuses that count as executed or in-progress.

```go
func (r *SprintRepository) GetEqualizerTarefas(ctx context.Context, sprintID, membroID uuid.UUID) ([]EqualizerTarefa, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.status, t.prioridade,
		       COALESCE(t.estimativa_tempo, 0) / 3600.0
		FROM tarefas t
		WHERE t.sprint_id = $1
		  AND t.responsavel_id = $2
		  AND t.status NOT IN (
			'Code Review', 'Teste', 'Validacao do Solicitante', 'Deploy', 'Concluido',
			'Em Desenvolvimento', 'Desenvolvimento', 'Cancelado'
		  )
		  AND COALESCE(t.estimativa_tempo, 0) > 0
		ORDER BY t.estimativa_tempo DESC
	`, sprintID, membroID)
	if err != nil {
		return nil, fmt.Errorf("getting equalizer tarefas: %w", err)
	}
	defer rows.Close()

	var result []EqualizerTarefa
	for rows.Next() {
		var t EqualizerTarefa
		if err := rows.Scan(&t.ID, &t.NumeroTicket, &t.Resumo, &t.Tipo, &t.Status, &t.Prioridade, &t.Horas); err != nil {
			return nil, fmt.Errorf("scanning equalizer tarefa: %w", err)
		}
		t.ResponsavelID = membroID
		result = append(result, t)
	}
	return result, nil
}
```

- [ ] **Step 3: Add UpdateTarefaResponsavel method**

For local DB update after JIRA reassignment:

```go
func (r *SprintRepository) UpdateTarefaResponsavel(ctx context.Context, tarefaID, novoResponsavelID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tarefas SET responsavel_id = $2, updated_at = NOW() WHERE id = $1
	`, tarefaID, novoResponsavelID)
	if err != nil {
		return fmt.Errorf("updating tarefa responsavel: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Add GetMembroJiraAccountID method**

For JIRA API calls, we need the Jira account ID from the local membro UUID:

```go
func (r *SprintRepository) GetMembroJiraAccountID(ctx context.Context, membroID uuid.UUID) (string, error) {
	var accountID string
	err := r.pool.QueryRow(ctx, `SELECT jira_account_id FROM membros WHERE id = $1`, membroID).Scan(&accountID)
	if err != nil {
		return "", fmt.Errorf("getting membro jira account id: %w", err)
	}
	return accountID, nil
}
```

- [ ] **Step 5: Verify compilation**

Run: `cd backend && go build ./...`
Expected: compiles without errors

- [ ] **Step 6: Commit**

```bash
git add backend/internal/repository/sprint.go
git commit -m "feat: add equalizer repository queries for task reassignment"
```

---

### Task 3: Implement EqualizerService

**Files:**
- Create: `backend/internal/service/equalizer.go`

**Interfaces:**
- Consumes:
  - `SprintService.GetCapacity(ctx, sprintID, equipeID) (*SprintCapacityResult, error)` — from existing service
  - `SprintRepository.GetEqualizerTarefas(ctx, sprintID, membroID) ([]EqualizerTarefa, error)` — from Task 2
  - `SprintRepository.UpdateTarefaResponsavel(ctx, tarefaID, novoResponsavelID) error` — from Task 2
  - `SprintRepository.GetMembroJiraAccountID(ctx, membroID) (string, error)` — from Task 2
  - `FonteDadosRepository.GetByID(ctx, id) (*domain.FonteDados, error)` — existing
  - `jira.Client.AssignIssue(ctx, issueKey, accountID) error` — from Task 1
  - `jira.Client.AddComment(ctx, issueKey, body) error` — from Task 1
  - `SyncService.buildClient` pattern — existing (duplicated here)
- Produces:
  - `EqualizerService.Calculate(ctx, sprintID, equipeID) (*EqualizerResult, error)` — used by Task 4
  - `EqualizerService.Apply(ctx, sprintID, fonteDadosID, transfers) (*ApplyResult, error)` — used by Task 4

- [ ] **Step 1: Create equalizer.go with types**

Create `backend/internal/service/equalizer.go`:

```go
package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

type EqualizerMembro struct {
	MembroID  uuid.UUID `json:"membro_id"`
	Nome      string    `json:"nome"`
	AvatarURL *string   `json:"avatar_url"`
	PctAntes  float64   `json:"pct_antes"`
	PctDepois float64   `json:"pct_depois"`
}

type EqualizerTarefa struct {
	ID           uuid.UUID `json:"id"`
	NumeroTicket string    `json:"numero_ticket"`
	Resumo       string    `json:"resumo"`
	Horas        float64   `json:"horas"`
	Tipo         string    `json:"tipo"`
	Prioridade   *string   `json:"prioridade"`
}

type EqualizerSugestao struct {
	De                EqualizerMembro   `json:"de"`
	Para              EqualizerMembro   `json:"para"`
	Tarefas           []EqualizerTarefa `json:"tarefas"`
	HorasTransferidas float64           `json:"horas_transferidas"`
	PctTransferido    float64           `json:"pct_transferido"`
}

type MembroAntesDepois struct {
	MembroID  uuid.UUID `json:"membro_id"`
	Nome      string    `json:"nome"`
	AvatarURL *string   `json:"avatar_url"`
	PctAntes  float64   `json:"pct_antes"`
	PctDepois float64   `json:"pct_depois"`
	HorasAntes  float64 `json:"horas_antes"`
	HorasDepois float64 `json:"horas_depois"`
}

type EqualizerResult struct {
	Sugestoes        []EqualizerSugestao `json:"sugestoes"`
	MembrosAntesDepois []MembroAntesDepois `json:"membros_antes_depois"`
	NadaASugerir     bool                `json:"nada_a_sugerir"`
	Motivo           string              `json:"motivo,omitempty"`
}

type TransferRequest struct {
	TarefaID          uuid.UUID `json:"tarefa_id"`
	TarefaKey         string    `json:"tarefa_key"`
	NovoResponsavelID uuid.UUID `json:"novo_responsavel_id"`
}

type ApplyRequest struct {
	FonteDadosID  uuid.UUID         `json:"fonte_dados_id"`
	Transferencias []TransferRequest `json:"transferencias"`
}

type ApplyResult struct {
	Aplicadas int           `json:"aplicadas"`
	Erros     []ApplyError  `json:"erros"`
}

type ApplyError struct {
	TarefaKey string `json:"tarefa_key"`
	Erro      string `json:"erro"`
}

type EqualizerService struct {
	sprintSvc          *SprintService
	sprintRepo         *repository.SprintRepository
	fdRepo             *repository.FonteDadosRepository
	clientFactory      ClientFactory
	oauthClientFactory OAuthClientFactory
	oauthSvc           *jira.OAuthService
	rateLimit          int
	logger             *zap.Logger
}

type membroState struct {
	mc       MembroCapacity
	horasMov float64
}

func NewEqualizerService(
	sprintSvc *SprintService,
	sprintRepo *repository.SprintRepository,
	fdRepo *repository.FonteDadosRepository,
	clientFactory ClientFactory,
	oauthClientFactory OAuthClientFactory,
	oauthSvc *jira.OAuthService,
	rateLimit int,
	logger *zap.Logger,
) *EqualizerService {
	return &EqualizerService{
		sprintSvc:          sprintSvc,
		sprintRepo:         sprintRepo,
		fdRepo:             fdRepo,
		clientFactory:      clientFactory,
		oauthClientFactory: oauthClientFactory,
		oauthSvc:           oauthSvc,
		rateLimit:          rateLimit,
		logger:             logger,
	}
}
```

- [ ] **Step 2: Add buildClient method (same pattern as SyncService)**

Append to `backend/internal/service/equalizer.go`:

```go
func (s *EqualizerService) buildClient(ctx context.Context, fonteDadosID uuid.UUID) (jira.Client, error) {
	fonte, err := s.fdRepo.GetByID(ctx, fonteDadosID)
	if err != nil {
		return nil, fmt.Errorf("getting fonte dados: %w", err)
	}
	if fonte.AuthType == "oauth2" {
		if fonte.OAuth2AccessToken == nil || fonte.OAuth2RefreshToken == nil {
			return nil, fmt.Errorf("fonte %s: oauth2 tokens missing", fonte.Nome)
		}
		accessToken := *fonte.OAuth2AccessToken
		if fonte.OAuth2TokenExpiry != nil && time.Now().After(*fonte.OAuth2TokenExpiry) {
			if s.oauthSvc == nil {
				return nil, fmt.Errorf("fonte %s: oauth token expired and no oauth service configured", fonte.Nome)
			}
			newTokens, err := s.oauthSvc.RefreshAccessToken(ctx, *fonte.OAuth2RefreshToken)
			if err != nil {
				return nil, fmt.Errorf("refreshing oauth token for %s: %w", fonte.Nome, err)
			}
			expiry := newTokens.Expiry()
			if err := s.fdRepo.SaveOAuthTokens(ctx, fonte.ID, fonte.BaseURL, newTokens.AccessToken, newTokens.RefreshToken, expiry); err != nil {
				return nil, fmt.Errorf("saving refreshed tokens: %w", err)
			}
			accessToken = newTokens.AccessToken
		}
		return s.oauthClientFactory(fonte.BaseURL, accessToken, s.rateLimit, s.logger), nil
	}
	email := ""
	if fonte.UserEmail != nil {
		email = *fonte.UserEmail
	}
	apiToken := ""
	if fonte.APIToken != nil {
		apiToken = *fonte.APIToken
	}
	return s.clientFactory(fonte.BaseURL, email, apiToken, s.rateLimit, s.logger), nil
}
```

Note: add `"time"` to the import block.

- [ ] **Step 3: Implement Calculate method**

Append to `backend/internal/service/equalizer.go`:

```go
func (s *EqualizerService) Calculate(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*EqualizerResult, error) {
	cap, err := s.sprintSvc.GetCapacity(ctx, sprintID, equipeID)
	if err != nil {
		return nil, fmt.Errorf("getting capacity: %w", err)
	}

	states := make(map[uuid.UUID]*membroState)
	var doadores, receptores []uuid.UUID

	for _, m := range cap.Membros {
		if !m.DaEquipe || m.Desligado {
			continue
		}
		states[m.MembroID] = &membroState{mc: m}
		if m.PercentualAlocacao > 100 {
			doadores = append(doadores, m.MembroID)
		} else if m.PercentualAlocacao < 70 {
			receptores = append(receptores, m.MembroID)
		}
	}

	if len(doadores) == 0 {
		return s.nadaASugerir(cap, states, "Nenhum membro com alocação acima de 100%"), nil
	}
	if len(receptores) == 0 {
		return s.nadaASugerir(cap, states, "Nenhum membro com alocação abaixo de 70%"), nil
	}

	sort.Slice(doadores, func(i, j int) bool {
		return states[doadores[i]].mc.PercentualAlocacao > states[doadores[j]].mc.PercentualAlocacao
	})
	sort.Slice(receptores, func(i, j int) bool {
		return states[receptores[i]].mc.PercentualAlocacao < states[receptores[j]].mc.PercentualAlocacao
	})

	var sugestoes []EqualizerSugestao
	sugestaoMap := make(map[string]*EqualizerSugestao)

	for _, dID := range doadores {
		if len(sugestoes) >= 10 {
			break
		}
		d := states[dID]
		if d.mc.HorasEstimadas <= 0 {
			continue
		}

		tarefas, err := s.sprintRepo.GetEqualizerTarefas(ctx, sprintID, dID)
		if err != nil {
			s.logger.Error("getting equalizer tarefas", zap.Error(err))
			continue
		}
		if len(tarefas) == 0 {
			continue
		}

		for _, t := range tarefas {
			if len(sugestoes) >= 10 {
				break
			}
			pctShift := t.Horas / d.mc.HorasEstimadas * 100
			if pctShift < 10 {
				continue
			}

			var bestR uuid.UUID
			bestDisp := -1.0
			for _, rID := range receptores {
				r := states[rID]
				disp := r.mc.HorasDisponiveis - r.horasMov
				newPct := (r.mc.HorasAlocadas + r.horasMov + t.Horas) / r.mc.HorasEstimadas * 100
				if disp > bestDisp && newPct <= 100 {
					bestDisp = disp
					bestR = rID
				}
			}
			if bestR == uuid.Nil {
				continue
			}

			key := dID.String() + "->" + bestR.String()
			if existing, ok := sugestaoMap[key]; ok {
				existing.Tarefas = append(existing.Tarefas, EqualizerTarefa{
					ID: t.ID, NumeroTicket: t.NumeroTicket, Resumo: t.Resumo,
					Horas: t.Horas, Tipo: t.Tipo, Prioridade: t.Prioridade,
				})
				existing.HorasTransferidas += t.Horas
			} else {
				sug := EqualizerSugestao{
					De:   EqualizerMembro{MembroID: dID, Nome: d.mc.Nome, AvatarURL: d.mc.AvatarURL},
					Para: EqualizerMembro{MembroID: bestR, Nome: states[bestR].mc.Nome, AvatarURL: states[bestR].mc.AvatarURL},
					Tarefas: []EqualizerTarefa{{
						ID: t.ID, NumeroTicket: t.NumeroTicket, Resumo: t.Resumo,
						Horas: t.Horas, Tipo: t.Tipo, Prioridade: t.Prioridade,
					}},
					HorasTransferidas: t.Horas,
				}
				sugestaoMap[key] = &sug
				sugestoes = append(sugestoes, sug)
			}
			d.horasMov += t.Horas
			states[bestR].horasMov += t.Horas
		}
	}

	if len(sugestoes) == 0 {
		return s.nadaASugerir(cap, states, "Nenhuma transferência viável atinge o limiar mínimo de 10%"), nil
	}

	// Fix sugestaoMap references back into sugestoes slice
	for i := range sugestoes {
		key := sugestoes[i].De.MembroID.String() + "->" + sugestoes[i].Para.MembroID.String()
		if ref, ok := sugestaoMap[key]; ok {
			sugestoes[i] = *ref
		}
	}

	// Calculate before/after percentages
	for i := range sugestoes {
		sug := &sugestoes[i]
		d := states[sug.De.MembroID]
		r := states[sug.Para.MembroID]
		sug.De.PctAntes = d.mc.PercentualAlocacao
		sug.De.PctDepois = (d.mc.HorasAlocadas - d.horasMov) / d.mc.HorasEstimadas * 100
		sug.Para.PctAntes = r.mc.PercentualAlocacao
		sug.Para.PctDepois = (r.mc.HorasAlocadas + r.horasMov) / r.mc.HorasEstimadas * 100
		sug.PctTransferido = sug.HorasTransferidas / d.mc.HorasEstimadas * 100
	}

	membrosAD := s.buildMembrosAntesDepois(cap, states)

	return &EqualizerResult{
		Sugestoes:          sugestoes,
		MembrosAntesDepois: membrosAD,
		NadaASugerir:       false,
	}, nil
}

func (s *EqualizerService) nadaASugerir(cap *SprintCapacityResult, states map[uuid.UUID]*membroState, motivo string) *EqualizerResult {
	return &EqualizerResult{
		Sugestoes:          nil,
		MembrosAntesDepois: s.buildMembrosAntesDepois(cap, states),
		NadaASugerir:       true,
		Motivo:             motivo,
	}
}

func (s *EqualizerService) buildMembrosAntesDepois(cap *SprintCapacityResult, states map[uuid.UUID]*membroState) []MembroAntesDepois {
	var result []MembroAntesDepois
	for _, m := range cap.Membros {
		if !m.DaEquipe || m.Desligado {
			continue
		}
		st := states[m.MembroID]
		horasDepois := m.HorasAlocadas - st.horasMov
		if st.horasMov > 0 {
			horasDepois = m.HorasAlocadas - st.horasMov
		} else {
			horasDepois = m.HorasAlocadas + st.horasMov
		}
		// horasMov is positive for receptores (added) and for doadores (subtracted)
		// Need to track direction: doadores have tasks removed, receptores have tasks added
		// Actually horasMov tracks total hours moved. For doadores it means hours leaving.
		// For receptores it means hours arriving.
		// The states map was built with doadores adding to their own horasMov (hours they donated)
		// and receptores adding to their own horasMov (hours they received).
		// So: doador depois = horas_alocadas - horasMov
		//     receptor depois = horas_alocadas + horasMov
		// But both use the same field. Need a role flag.
		pctDepois := m.PercentualAlocacao
		if m.HorasEstimadas > 0 {
			if m.PercentualAlocacao > 100 {
				horasDepois = m.HorasAlocadas - st.horasMov
			} else {
				horasDepois = m.HorasAlocadas + st.horasMov
			}
			pctDepois = horasDepois / m.HorasEstimadas * 100
		}
		result = append(result, MembroAntesDepois{
			MembroID:    m.MembroID,
			Nome:        m.Nome,
			AvatarURL:   m.AvatarURL,
			PctAntes:    m.PercentualAlocacao,
			PctDepois:   pctDepois,
			HorasAntes:  m.HorasAlocadas,
			HorasDepois: horasDepois,
		})
	}
	return result
}
```

- [ ] **Step 4: Implement Apply method**

Append to `backend/internal/service/equalizer.go`:

```go
func (s *EqualizerService) Apply(ctx context.Context, req ApplyRequest) (*ApplyResult, error) {
	client, err := s.buildClient(ctx, req.FonteDadosID)
	if err != nil {
		return nil, fmt.Errorf("building jira client: %w", err)
	}

	result := &ApplyResult{}

	for _, tr := range req.Transferencias {
		jiraAccountID, err := s.sprintRepo.GetMembroJiraAccountID(ctx, tr.NovoResponsavelID)
		if err != nil {
			result.Erros = append(result.Erros, ApplyError{TarefaKey: tr.TarefaKey, Erro: "membro não encontrado"})
			continue
		}

		if err := client.AssignIssue(ctx, tr.TarefaKey, jiraAccountID); err != nil {
			result.Erros = append(result.Erros, ApplyError{TarefaKey: tr.TarefaKey, Erro: err.Error()})
			continue
		}

		novoNome := ""
		if n, err := s.getMembroNome(ctx, tr.NovoResponsavelID); err == nil {
			novoNome = n
		}
		comment := fmt.Sprintf("Tarefa transferida para %s via Equalizador de Capacidade", novoNome)
		if err := client.AddComment(ctx, tr.TarefaKey, comment); err != nil {
			s.logger.Warn("failed to add comment", zap.String("key", tr.TarefaKey), zap.Error(err))
		}

		if err := s.sprintRepo.UpdateTarefaResponsavel(ctx, tr.TarefaID, tr.NovoResponsavelID); err != nil {
			s.logger.Error("failed to update local responsavel", zap.String("key", tr.TarefaKey), zap.Error(err))
		}

		result.Aplicadas++
	}

	return result, nil
}

func (s *EqualizerService) getMembroNome(ctx context.Context, membroID uuid.UUID) (string, error) {
	var nome string
	err := s.sprintRepo.Pool().QueryRow(ctx, `SELECT nome FROM membros WHERE id = $1`, membroID).Scan(&nome)
	return nome, err
}
```

Note: `Pool()` method needs to exist on `SprintRepository`. Add it if missing:
```go
func (r *SprintRepository) Pool() *pgxpool.Pool { return r.pool }
```

- [ ] **Step 5: Verify compilation**

Run: `cd backend && go build ./...`
Expected: compiles without errors

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/equalizer.go backend/internal/repository/sprint.go
git commit -m "feat: implement equalizer service with greedy algorithm and JIRA apply"
```

---

### Task 4: Add Equalizer HTTP handler and routes

**Files:**
- Create: `backend/internal/handler/equalizer.go`
- Modify: `backend/cmd/api/main.go` (wiring + routes)

**Interfaces:**
- Consumes:
  - `EqualizerService.Calculate(ctx, sprintID, equipeID) (*EqualizerResult, error)` — from Task 3
  - `EqualizerService.Apply(ctx, req) (*ApplyResult, error)` — from Task 3
- Produces: HTTP endpoints `GET /sprints/{id}/equalizer` and `POST /sprints/{id}/equalizer/apply`

- [ ] **Step 1: Create handler**

Create `backend/internal/handler/equalizer.go`:

```go
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"go.uber.org/zap"
)

type EqualizerHandler struct {
	svc    *service.EqualizerService
	logger *zap.Logger
}

func NewEqualizerHandler(svc *service.EqualizerService, logger *zap.Logger) *EqualizerHandler {
	return &EqualizerHandler{svc: svc, logger: logger}
}

func (h *EqualizerHandler) GetSuggestions(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}
	var equipeID *uuid.UUID
	if e := r.URL.Query().Get("equipe"); e != "" {
		id, err := uuid.Parse(e)
		if err == nil {
			equipeID = &id
		}
	}
	result, err := h.svc.Calculate(r.Context(), sprintID, equipeID)
	if err != nil {
		h.logger.Error("calculating equalizer", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao calcular equalização")
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func (h *EqualizerHandler) ApplyTransfers(w http.ResponseWriter, r *http.Request) {
	var req service.ApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if len(req.Transferencias) == 0 {
		respondError(w, http.StatusBadRequest, "nenhuma transferência selecionada")
		return
	}
	if req.FonteDadosID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "fonte_dados_id obrigatório")
		return
	}
	result, err := h.svc.Apply(r.Context(), req)
	if err != nil {
		h.logger.Error("applying equalizer", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao aplicar transferências")
		return
	}
	respondJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 2: Wire up in main.go**

In `backend/cmd/api/main.go`, after the `schedulerSvc` block (around line 131), add:

```go
equalizerSvc := service.NewEqualizerService(sprintService, sprintRepo, fonteDadosRepo, clientFactory, oauthClientFactory, oauthSvc, cfg.Sync.RateLimitPerSec, logger)
equalizerHandler := handler.NewEqualizerHandler(equalizerSvc, logger)
```

In the authenticated route group, after the sprint routes (around line 221), add:

```go
r.Get("/sprints/{id}/equalizer", equalizerHandler.GetSuggestions)
r.Post("/sprints/{id}/equalizer/apply", equalizerHandler.ApplyTransfers)
```

- [ ] **Step 3: Verify compilation**

Run: `cd backend && go build ./...`
Expected: compiles without errors

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handler/equalizer.go backend/cmd/api/main.go
git commit -m "feat: add equalizer HTTP handler and routes"
```

---

### Task 5: Frontend — Equalizer modal and button

**Files:**
- Modify: `frontend/index.html` (CSS + HTML modal + JS functions)

**Interfaces:**
- Consumes: `GET /api/v1/sprints/{id}/equalizer?equipe=X` and `POST /api/v1/sprints/{id}/equalizer/apply`
- Produces: Equalizer button in capacity summary, modal with before/after and task selection

- [ ] **Step 1: Add CSS for equalizer modal**

In `frontend/index.html`, after the existing `.modal-actions` CSS rule (around line 328), add:

```css
.eq-modal { max-width: 720px; }
.eq-loading { text-align: center; padding: 40px; color: var(--text-secondary); }
.eq-nada { text-align: center; padding: 40px; }
.eq-nada-icon { font-size: 48px; margin-bottom: 12px; }
.eq-nada-text { font-size: 16px; font-weight: 600; color: var(--text-primary); }
.eq-nada-motivo { font-size: 13px; color: var(--text-secondary); margin-top: 8px; }
.eq-section-title { font-size: 14px; font-weight: 650; color: var(--text-primary); margin: 20px 0 10px; letter-spacing: -.2px; }
.eq-antes-depois { display: flex; flex-direction: column; gap: 6px; }
.eq-ad-row { display: flex; align-items: center; gap: 10px; padding: 6px 10px; border-radius: 8px; background: var(--surface-secondary); }
.eq-ad-avatar { width: 28px; height: 28px; border-radius: 50%; object-fit: cover; flex-shrink: 0; }
.eq-ad-avatar-fallback { width: 28px; height: 28px; border-radius: 50%; background: var(--primary); color: #fff; display: flex; align-items: center; justify-content: center; font-size: 11px; font-weight: 700; flex-shrink: 0; }
.eq-ad-nome { flex: 1; font-size: 13px; font-weight: 500; color: var(--text-primary); }
.eq-ad-pct { font-size: 13px; font-weight: 600; min-width: 55px; text-align: right; }
.eq-ad-arrow { color: var(--text-secondary); font-size: 14px; }
.eq-ad-bar { flex: 0 0 80px; height: 6px; border-radius: 3px; background: var(--border); overflow: hidden; }
.eq-ad-bar-fill { height: 100%; border-radius: 3px; transition: width .3s; }
.eq-sug-card { border: 1px solid var(--border); border-radius: 10px; padding: 16px; margin-bottom: 12px; background: var(--surface-secondary); }
.eq-sug-header { display: flex; align-items: center; gap: 12px; margin-bottom: 12px; }
.eq-sug-member { display: flex; flex-direction: column; align-items: center; gap: 4px; flex: 0 0 100px; }
.eq-sug-avatar { width: 40px; height: 40px; border-radius: 50%; object-fit: cover; }
.eq-sug-avatar-fallback { width: 40px; height: 40px; border-radius: 50%; background: var(--primary); color: #fff; display: flex; align-items: center; justify-content: center; font-size: 14px; font-weight: 700; }
.eq-sug-nome { font-size: 12px; font-weight: 600; text-align: center; color: var(--text-primary); }
.eq-sug-pct { font-size: 11px; }
.eq-sug-arrow { flex: 1; display: flex; flex-direction: column; align-items: center; gap: 2px; color: var(--text-secondary); }
.eq-sug-arrow-icon { font-size: 24px; }
.eq-sug-arrow-label { font-size: 11px; font-weight: 600; }
.eq-task-list { display: flex; flex-direction: column; gap: 4px; }
.eq-task-row { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-radius: 6px; background: var(--surface-primary); cursor: pointer; }
.eq-task-row:hover { background: var(--border); }
.eq-task-check { width: 16px; height: 16px; accent-color: var(--primary); }
.eq-task-key { font-size: 12px; font-weight: 600; color: var(--primary); min-width: 80px; }
.eq-task-resumo { font-size: 12px; color: var(--text-primary); flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.eq-task-badge { font-size: 10px; padding: 2px 6px; border-radius: 4px; background: var(--border); color: var(--text-secondary); }
.eq-task-horas { font-size: 12px; font-weight: 600; color: var(--text-secondary); min-width: 35px; text-align: right; }
.eq-btn { display: inline-flex; align-items: center; gap: 6px; padding: 8px 16px; border-radius: 8px; font-size: 13px; font-weight: 600; cursor: pointer; border: 1px solid var(--border); background: var(--surface-secondary); color: var(--text-primary); position: relative; }
.eq-btn:hover { background: var(--border); }
.eq-badge { position: absolute; top: -6px; right: -6px; width: 18px; height: 18px; border-radius: 50%; background: var(--red); color: #fff; font-size: 11px; font-weight: 700; display: flex; align-items: center; justify-content: center; display: none; }
.eq-badge.visible { display: flex; }
```

- [ ] **Step 2: Add modal HTML**

In `frontend/index.html`, after the existing modals (after the last `</div>` of the last modal-overlay), add:

```html
<div class="modal-overlay" id="equalizer-modal" onclick="if(event.target===this)closeEqualizerModal()">
  <div class="modal eq-modal">
    <div class="modal-title">⚖️ Equalizador de Capacidade</div>
    <div id="equalizer-content"></div>
    <div class="modal-actions" id="equalizer-actions" style="display:none">
      <button class="btn-cancel" onclick="closeEqualizerModal()">Cancelar</button>
      <button class="btn-primary" id="equalizer-apply-btn" onclick="applyEqualizer()" disabled>Aplicar Selecionadas (0)</button>
    </div>
  </div>
</div>
```

- [ ] **Step 3: Add Equalizer button in capacity summary**

In `frontend/index.html`, in the `openSprintCapacity` function, after the capacity summary closing `</div>` (after the feriados section, around line 1587), add the equalizer button:

Find the line that closes the capacity-summary div and ends with the breakdown section. Insert after `html += '</div>';` (the capacity-summary close):

```javascript
html += '<div style="margin:12px 0"><button class="eq-btn" id="eq-btn-' + sprintID + '" onclick="openEqualizer(\'' + sprintID + '\')">⚖️ Equalizador<span class="eq-badge" id="eq-badge-' + sprintID + '">!</span></button></div>';
```

- [ ] **Step 4: Add JavaScript functions**

At the end of the `<script>` section in `frontend/index.html`, add:

```javascript
let equalizerData = null;
let equalizerSprintID = null;

async function openEqualizer(sprintID) {
  equalizerSprintID = sprintID;
  const modal = document.getElementById('equalizer-modal');
  const content = document.getElementById('equalizer-content');
  const actions = document.getElementById('equalizer-actions');
  content.innerHTML = '<div class="eq-loading">Calculando equalização...</div>';
  actions.style.display = 'none';
  modal.classList.add('open');

  const equipeID = document.getElementById('sprint-equipe-filter') ? document.getElementById('sprint-equipe-filter').value : '';
  const url = '/api/v1/sprints/' + sprintID + '/equalizer' + (equipeID ? '?equipe=' + equipeID : '');
  try {
    const resp = await fetch(url, { headers: authHeaders() });
    if (!resp.ok) throw new Error('Erro ' + resp.status);
    equalizerData = await resp.json();
    renderEqualizerModal();
  } catch (e) {
    content.innerHTML = '<div class="eq-nada"><div class="eq-nada-icon">❌</div><div class="eq-nada-text">Erro ao calcular</div><div class="eq-nada-motivo">' + esc(e.message) + '</div></div>';
  }
}

function closeEqualizerModal() {
  document.getElementById('equalizer-modal').classList.remove('open');
  equalizerData = null;
}

function renderEqualizerModal() {
  const content = document.getElementById('equalizer-content');
  const actions = document.getElementById('equalizer-actions');
  const data = equalizerData;

  if (data.nada_a_sugerir) {
    content.innerHTML = '<div class="eq-nada"><div class="eq-nada-icon">✅</div><div class="eq-nada-text">Nada a sugerir de transferência de tarefas</div>' +
      (data.motivo ? '<div class="eq-nada-motivo">' + esc(data.motivo) + '</div>' : '') + '</div>';
    actions.style.display = 'none';
    return;
  }

  let html = '';

  // Antes/Depois section
  html += '<div class="eq-section-title">Antes / Depois da Equalização</div>';
  html += '<div class="eq-antes-depois">';
  (data.membros_antes_depois || []).forEach(function(m) {
    const avatar = m.avatar_url
      ? '<img class="eq-ad-avatar" src="' + esc(m.avatar_url) + '">'
      : '<div class="eq-ad-avatar-fallback">' + esc(m.nome.split(' ').map(function(p){return p[0]}).join('').substring(0,2)) + '</div>';
    const antesColor = m.pct_antes > 100 ? 'var(--red)' : m.pct_antes > 80 ? 'var(--amber)' : 'var(--green)';
    const depoisColor = m.pct_depois > 100 ? 'var(--red)' : m.pct_depois > 80 ? 'var(--amber)' : 'var(--green)';
    html += '<div class="eq-ad-row">' + avatar +
      '<div class="eq-ad-nome">' + esc(m.nome) + '</div>' +
      '<div class="eq-ad-bar"><div class="eq-ad-bar-fill" style="width:' + Math.min(100, m.pct_antes) + '%;background:' + antesColor + '"></div></div>' +
      '<div class="eq-ad-pct" style="color:' + antesColor + '">' + m.pct_antes.toFixed(0) + '%</div>' +
      '<div class="eq-ad-arrow">→</div>' +
      '<div class="eq-ad-bar"><div class="eq-ad-bar-fill" style="width:' + Math.min(100, m.pct_depois) + '%;background:' + depoisColor + '"></div></div>' +
      '<div class="eq-ad-pct" style="color:' + depoisColor + '">' + m.pct_depois.toFixed(0) + '%</div>' +
      '</div>';
  });
  html += '</div>';

  // Sugestões
  html += '<div class="eq-section-title">Sugestões de Transferência</div>';
  (data.sugestoes || []).forEach(function(sug, si) {
    const deAvatar = sug.de.avatar_url
      ? '<img class="eq-sug-avatar" src="' + esc(sug.de.avatar_url) + '">'
      : '<div class="eq-sug-avatar-fallback">' + esc(sug.de.nome.split(' ').map(function(p){return p[0]}).join('').substring(0,2)) + '</div>';
    const paraAvatar = sug.para.avatar_url
      ? '<img class="eq-sug-avatar" src="' + esc(sug.para.avatar_url) + '">'
      : '<div class="eq-sug-avatar-fallback">' + esc(sug.para.nome.split(' ').map(function(p){return p[0]}).join('').substring(0,2)) + '</div>';
    const deAntesColor = sug.de.pct_antes > 100 ? 'var(--red)' : 'var(--amber)';
    const deDepoisColor = sug.de.pct_depois > 100 ? 'var(--red)' : sug.de.pct_depois > 80 ? 'var(--amber)' : 'var(--green)';
    const paraAntesColor = sug.para.pct_antes < 70 ? 'var(--green)' : 'var(--amber)';
    const paraDepoisColor = sug.para.pct_depois > 100 ? 'var(--red)' : sug.para.pct_depois > 80 ? 'var(--amber)' : 'var(--green)';

    html += '<div class="eq-sug-card">';
    html += '<div class="eq-sug-header">';
    html += '<div class="eq-sug-member">' + deAvatar + '<div class="eq-sug-nome">' + esc(sug.de.nome) + '</div>' +
      '<div class="eq-sug-pct"><span style="color:' + deAntesColor + '">' + sug.de.pct_antes.toFixed(0) + '%</span> → <span style="color:' + deDepoisColor + '">' + sug.de.pct_depois.toFixed(0) + '%</span></div></div>';
    html += '<div class="eq-sug-arrow"><div class="eq-sug-arrow-icon">→</div><div class="eq-sug-arrow-label">' + sug.horas_transferidas.toFixed(1) + 'h</div></div>';
    html += '<div class="eq-sug-member">' + paraAvatar + '<div class="eq-sug-nome">' + esc(sug.para.nome) + '</div>' +
      '<div class="eq-sug-pct"><span style="color:' + paraAntesColor + '">' + sug.para.pct_antes.toFixed(0) + '%</span> → <span style="color:' + paraDepoisColor + '">' + sug.para.pct_depois.toFixed(0) + '%</span></div></div>';
    html += '</div>';

    html += '<div class="eq-task-list">';
    sug.tarefas.forEach(function(t, ti) {
      html += '<label class="eq-task-row">' +
        '<input type="checkbox" class="eq-task-check" data-sug="' + si + '" data-task="' + ti + '" checked onchange="updateEqualizerCount()">' +
        '<span class="eq-task-key">' + esc(t.numero_ticket) + '</span>' +
        '<span class="eq-task-resumo">' + esc(t.resumo) + '</span>' +
        '<span class="eq-task-badge">' + esc(t.tipo) + '</span>' +
        '<span class="eq-task-horas">' + t.horas.toFixed(1) + 'h</span>' +
        '</label>';
    });
    html += '</div></div>';
  });

  content.innerHTML = html;
  actions.style.display = 'flex';
  updateEqualizerCount();
}

function updateEqualizerCount() {
  const checks = document.querySelectorAll('.eq-task-check:checked');
  const btn = document.getElementById('equalizer-apply-btn');
  btn.textContent = 'Aplicar Selecionadas (' + checks.length + ')';
  btn.disabled = checks.length === 0;
}

async function applyEqualizer() {
  const checks = document.querySelectorAll('.eq-task-check:checked');
  if (checks.length === 0) return;

  const btn = document.getElementById('equalizer-apply-btn');
  btn.disabled = true;
  btn.textContent = 'Aplicando...';

  const transferencias = [];
  checks.forEach(function(ch) {
    const si = parseInt(ch.dataset.sug);
    const ti = parseInt(ch.dataset.task);
    const sug = equalizerData.sugestoes[si];
    const t = sug.tarefas[ti];
    transferencias.push({
      tarefa_id: t.id,
      tarefa_key: t.numero_ticket,
      novo_responsavel_id: sug.para.membro_id
    });
  });

  try {
    const resp = await fetch('/api/v1/sprints/' + equalizerSprintID + '/equalizer/apply', {
      method: 'POST',
      headers: { ...authHeaders(), 'Content-Type': 'application/json' },
      body: JSON.stringify({
        fonte_dados_id: equalizerData.membros_antes_depois.length > 0 ? '' : '',
        transferencias: transferencias
      })
    });
    if (!resp.ok) throw new Error('Erro ' + resp.status);
    const result = await resp.json();

    if (result.erros && result.erros.length > 0) {
      let errMsg = result.aplicadas + ' transferidas com sucesso.\\nErros:\\n';
      result.erros.forEach(function(e) { errMsg += e.tarefa_key + ': ' + e.erro + '\\n'; });
      alert(errMsg);
    } else {
      alert(result.aplicadas + ' tarefas transferidas com sucesso!');
      closeEqualizerModal();
    }
    openSprintCapacity(equalizerSprintID);
  } catch (e) {
    alert('Erro ao aplicar: ' + e.message);
    btn.disabled = false;
    updateEqualizerCount();
  }
}
```

- [ ] **Step 5: Fix fonte_dados_id in apply call**

The `fonte_dados_id` is available from the capacity data stored globally. In the `openSprintCapacity` function, the capacity data is already fetched. Store it globally:

Find in `openSprintCapacity` where capacity data is loaded (around line 1488-1493). After the `const data = await resp.json()` for the capacity call, add:

```javascript
window._currentSprintCapacity = data;
```

Then update the `applyEqualizer` function's fetch body to use it:

```javascript
body: JSON.stringify({
  fonte_dados_id: window._currentSprintCapacity.fonte_dados_id,
  transferencias: transferencias
})
```

- [ ] **Step 6: Add background badge check**

In `openSprintCapacity`, after the page has been rendered (after `document.getElementById('sprints-content').innerHTML = ...`), add a background fetch to check for equalizer suggestions and show/hide the badge:

```javascript
// Background check for equalizer badge
(async function() {
  const eqUrl = '/api/v1/sprints/' + sprintID + '/equalizer' + (equipeID ? '?equipe=' + equipeID : '');
  try {
    const eqResp = await fetch(eqUrl, { headers: authHeaders() });
    if (eqResp.ok) {
      const eqData = await eqResp.json();
      const badge = document.getElementById('eq-badge-' + sprintID);
      if (badge && !eqData.nada_a_sugerir) {
        badge.classList.add('visible');
      }
      window._cachedEqualizerData = eqData;
    }
  } catch(e) {}
})();
```

Then in `openEqualizer`, use the cached data if available to avoid double-fetch:

At the start of `openEqualizer`, after `modal.classList.add('open');`, add:

```javascript
if (window._cachedEqualizerData) {
  equalizerData = window._cachedEqualizerData;
  window._cachedEqualizerData = null;
  renderEqualizerModal();
  return;
}
```

- [ ] **Step 7: Verify in browser**

Run: Restart server with `cd backend && go build -o myplanner ./cmd/api && ./myplanner`
Navigate to a sprint detail page, click "⚖️ Equalizador" button, verify modal opens with suggestions or "nada a sugerir" message.

- [ ] **Step 7: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add equalizer modal with before/after visualization, badge, and apply flow"
```

---

### Task 6: Integration test and polish

**Files:**
- Modify: `frontend/index.html` (fix any rendering issues found during testing)
- Modify: `backend/internal/service/equalizer.go` (fix any edge cases)

**Interfaces:**
- Consumes: all prior tasks
- Produces: working end-to-end equalizer feature

- [ ] **Step 1: Test "nada a sugerir" scenario**

Open a sprint where all members are between 70-100% allocation. Click Equalizador. Verify modal shows ✅ icon and "Nada a sugerir de transferência de tarefas" with the specific reason.

- [ ] **Step 2: Test suggestion scenario**

Open a sprint with at least one member >100% allocation and one <70%. Click Equalizador. Verify:
- Before/after table shows all team members with colored bars and percentages
- Suggestion cards show donor → recipient with avatars and % change
- Task list has checkboxes, ticket numbers, summaries, types, hours
- "Aplicar Selecionadas (N)" button updates count as checkboxes toggle

- [ ] **Step 3: Test apply flow**

Select some tasks, click Apply. Verify:
- Button shows "Aplicando..."
- On success: alert with count, modal closes, capacity page reloads with updated assignments
- Check JIRA: cards reassigned, comments added

- [ ] **Step 4: Test edge cases**

- Sprint with no team members (only externals): should show "nada a sugerir"
- Member with tasks but zero time estimates: tasks should not appear as candidates
- Deselect all checkboxes: Apply button disabled
- Network error during apply: error alert, modal stays open

- [ ] **Step 5: Commit any fixes**

```bash
git add -A
git commit -m "fix: equalizer integration polish and edge case handling"
```
