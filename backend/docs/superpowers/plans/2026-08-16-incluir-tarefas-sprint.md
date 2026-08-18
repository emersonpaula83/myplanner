# Incluir Tarefas na Sprint — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow users in the Planning tab to search Jira tickets by key, preview results, and add selected tasks to the sprint — with a sticky header that stays visible while scrolling.

**Architecture:** Two new backend endpoints (`search-tasks`, `include-tasks`) on PlanningHandler/Service/Repository. One new Jira client method (`SearchIssuesByKeys`). Frontend changes to `index.html`: sticky header wrapper, restructured action bar with "Incluir Tarefas" button, and a new modal for search + selection + inclusion.

**Tech Stack:** Go (chi, pgx, zap), vanilla JS, CSS custom properties

## Global Constraints

- Monolithic vanilla JS frontend (`frontend/index.html`) — `var` for globals, `function` declarations
- Go backend: chi router, pgx/v5 pool, zap logger, repository pattern with `*pgxpool.Pool`
- Handler pattern: local interface matching service methods, struct holds `svc` + `logger`
- Auth: `middleware.UserIDFromContext(r.Context())` returns `uuid.UUID`
- CSS: uses custom properties (`--surface-primary`, `--text-muted`, `--border-subtle`, `--accent`, etc.)
- All user-facing strings in Portuguese
- No commits without explicit user consent
- Changes applied to main at `/home/emerson/code/myplanner` via python/cp (worktree isolation)

---

### Task 1: Jira Client — SearchIssuesByKeys Method

**Files:**
- Modify: `backend/internal/jira/client.go` (interface ~line 34, implementation after GetProjectIssues ~line 253)

**Interfaces:**
- Consumes: existing `Client` interface, `doPost`, `JiraIssue`, field construction pattern from `GetProjectIssues`
- Produces: `SearchIssuesByKeys(ctx context.Context, projectKey string, issueKeys []string) ([]JiraIssue, error)` — added to `Client` interface and implemented on `HTTPClient`

- [ ] **Step 1: Add method to Client interface**

In `backend/internal/jira/client.go`, add to the `Client` interface (after `RemoveFromSprint` at line 50):

```go
SearchIssuesByKeys(ctx context.Context, projectKey string, issueKeys []string) ([]JiraIssue, error)
```

- [ ] **Step 2: Implement SearchIssuesByKeys on HTTPClient**

Insert after the `GetProjectIssues` method (after line 253). Same pagination pattern as `GetProjectIssues` but with a different JQL:

```go
func (c *HTTPClient) SearchIssuesByKeys(ctx context.Context, projectKey string, issueKeys []string) ([]JiraIssue, error) {
	if len(issueKeys) == 0 {
		return nil, nil
	}

	quoted := make([]string, len(issueKeys))
	for i, k := range issueKeys {
		quoted[i] = fmt.Sprintf("%q", k)
	}
	jql := fmt.Sprintf("project = %s AND key IN (%s)", projectKey, strings.Join(quoted, ","))

	fields := []string{"summary", "issuetype", "status", "priority", "assignee", "reporter",
		"project", "created", "updated", "duedate", "resolutiondate", "timetracking",
		"sprint", "parent", "labels", "components"}
	if c.sprintFieldID != "" && c.sprintFieldID != "sprint" {
		fields = append(fields, c.sprintFieldID)
	}
	fields = append(fields, c.customFieldIDs...)

	all := make([]JiraIssue, 0)
	var nextPageToken string
	for {
		payload := map[string]any{
			"jql":        jql,
			"maxResults": 100,
			"fields":     fields,
		}
		if nextPageToken != "" {
			payload["nextPageToken"] = nextPageToken
		}
		body, err := c.doPost(ctx, "/rest/api/3/search/jql", payload)
		if err != nil {
			return nil, err
		}
		var rawResult struct {
			Issues        []json.RawMessage `json:"issues"`
			NextPageToken string            `json:"nextPageToken"`
			IsLast        bool              `json:"isLast"`
		}
		if err := json.Unmarshal(body, &rawResult); err != nil {
			return nil, fmt.Errorf("unmarshalling search results: %w", err)
		}
		for _, raw := range rawResult.Issues {
			var issue JiraIssue
			if err := json.Unmarshal(raw, &issue); err != nil {
				continue
			}
			if c.sprintFieldID != "" && c.sprintFieldID != "sprint" {
				var fieldMap map[string]json.RawMessage
				if gjson := struct{ Fields json.RawMessage `json:"fields"` }{}; json.Unmarshal(raw, &gjson) == nil {
					json.Unmarshal(gjson.Fields, &fieldMap)
					if sprintRaw, ok := fieldMap[c.sprintFieldID]; ok {
						var sprints []JiraSprint
						json.Unmarshal(sprintRaw, &sprints)
						if len(sprints) > 0 {
							issue.Fields.Sprint = &sprints[len(sprints)-1]
						}
					}
				}
			}
			if len(c.customFieldIDs) > 0 {
				var fieldMap map[string]json.RawMessage
				if gjson := struct{ Fields json.RawMessage `json:"fields"` }{}; json.Unmarshal(raw, &gjson) == nil {
					json.Unmarshal(gjson.Fields, &fieldMap)
					issue.Fields.CustomFields = make(map[string]any)
					for _, cfID := range c.customFieldIDs {
						if v, ok := fieldMap[cfID]; ok {
							var parsed any
							json.Unmarshal(v, &parsed)
							issue.Fields.CustomFields[cfID] = parsed
						}
					}
				}
			}
			all = append(all, issue)
		}
		if rawResult.IsLast || len(rawResult.Issues) == 0 || rawResult.NextPageToken == "" {
			break
		}
		nextPageToken = rawResult.NextPageToken
	}
	return all, nil
}
```

- [ ] **Step 3: Add `strings` import if not present**

Check imports at top of `client.go` — add `"strings"` if missing.

- [ ] **Step 4: Build verification**

Run: `go build ./internal/jira/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/jira/client.go
git commit -m "feat: add SearchIssuesByKeys to Jira client"
```

---

### Task 2: Repository — Search, Upsert, Batch Move, and GetByIDs

**Files:**
- Modify: `backend/internal/repository/planning.go` (~line 134, after existing methods)
- Modify: `backend/internal/service/interfaces.go` (~line 148, add to PlanningRepoStore)

**Interfaces:**
- Consumes: existing `PlanningRepository` struct with `pool *pgxpool.Pool`, `PlanningTarefa` struct, `UpsertTarefaParams` struct (from `sync.go` in same package)
- Produces:
  - `SearchTarefaResult` struct: `{ID uuid.UUID, NumeroTicket string, Resumo string, Tipo string, Status string, Prioridade *string, SprintID *uuid.UUID, ResponsavelNome *string}`
  - `SearchTarefasByKeys(ctx, projetoID uuid.UUID, keys []string) ([]SearchTarefaResult, error)`
  - `UpsertTarefaFromJira(ctx, params *UpsertTarefaParams) (uuid.UUID, error)` — same INSERT ON CONFLICT as sync's UpsertTarefa
  - `MoveTarefasToSprint(ctx, sprintID uuid.UUID, tarefaIDs []uuid.UUID) error`
  - `GetTarefasByIDs(ctx, ids []uuid.UUID) ([]PlanningTarefa, error)`
  - `GetProjetoChaveByID(ctx, projetoID uuid.UUID) (string, uuid.UUID, error)` — returns (chave, fonteDadosID)

- [ ] **Step 1: Add SearchTarefaResult struct and SearchTarefasByKeys method**

Append to `backend/internal/repository/planning.go`:

```go
type SearchTarefaResult struct {
	ID              uuid.UUID  `json:"id"`
	NumeroTicket    string     `json:"key"`
	Resumo          string     `json:"resumo"`
	Tipo            string     `json:"tipo"`
	Status          string     `json:"status"`
	Prioridade      *string    `json:"prioridade"`
	SprintID        *uuid.UUID `json:"-"`
	ResponsavelNome *string    `json:"responsavel_nome"`
}

func (r *PlanningRepository) SearchTarefasByKeys(ctx context.Context, projetoID uuid.UUID, keys []string) ([]SearchTarefaResult, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.status, t.prioridade, t.sprint_id, m.nome
		FROM tarefas t
		LEFT JOIN membros m ON m.id = t.responsavel_id
		WHERE t.projeto_id = $1 AND t.numero_ticket = ANY($2) AND t.removido_em IS NULL
	`, projetoID, keys)
	if err != nil {
		return nil, fmt.Errorf("searching tarefas by keys: %w", err)
	}
	defer rows.Close()

	var result []SearchTarefaResult
	for rows.Next() {
		var t SearchTarefaResult
		if err := rows.Scan(&t.ID, &t.NumeroTicket, &t.Resumo, &t.Tipo, &t.Status, &t.Prioridade, &t.SprintID, &t.ResponsavelNome); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating search results: %w", err)
	}
	return result, nil
}
```

- [ ] **Step 2: Add UpsertTarefaFromJira method**

This reuses the same `UpsertTarefaParams` struct from `sync.go` and the same INSERT ON CONFLICT SQL:

```go
func (r *PlanningRepository) UpsertTarefaFromJira(ctx context.Context, t *UpsertTarefaParams) (uuid.UUID, error) {
	ce := t.CamposExtras
	if ce == nil {
		ce = json.RawMessage(`{}`)
	}
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO tarefas (id, fonte_dados_id, projeto_id, jira_id, numero_ticket, resumo,
		                     tipo, status, prioridade, estimativa_pontos, estimativa_tempo, tempo_gasto,
		                     responsavel_id, relator_id, team, sprint_id, data_criacao, data_limite,
		                     data_resolvido, data_atualizado, tipo_demanda, data_componente,
		                     status_categoria, campos_extras, parent_id, apelido, data_inicio_execucao,
		                     data_entrada_sprint, marcacao)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
		        $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28)
		ON CONFLICT (fonte_dados_id, jira_id)
		DO UPDATE SET resumo = EXCLUDED.resumo, tipo = EXCLUDED.tipo, status = EXCLUDED.status,
		              prioridade = EXCLUDED.prioridade, estimativa_pontos = EXCLUDED.estimativa_pontos,
		              estimativa_tempo = EXCLUDED.estimativa_tempo, tempo_gasto = EXCLUDED.tempo_gasto,
		              responsavel_id = EXCLUDED.responsavel_id, relator_id = EXCLUDED.relator_id,
		              team = EXCLUDED.team,
		              data_resolvido = EXCLUDED.data_resolvido,
		              data_atualizado = EXCLUDED.data_atualizado, tipo_demanda = EXCLUDED.tipo_demanda,
		              status_categoria = EXCLUDED.status_categoria,
		              campos_extras = EXCLUDED.campos_extras, updated_at = NOW()
		RETURNING id
	`, t.FonteDadosID, t.ProjetoID, t.JiraID, t.NumeroTicket, t.Resumo,
		t.Tipo, t.Status, t.Prioridade, t.EstimativaPontos, t.EstimativaTempo, t.TempoGasto,
		t.ResponsavelID, t.RelatorID, t.Team, t.SprintID, t.DataCriacao, t.DataLimite,
		t.DataResolvido, t.DataAtualizado, t.TipoDemanda, t.DataComponente,
		t.StatusCategoria, ce, t.ParentID, t.Apelido, t.DataInicioExecucao,
		t.DataEntradaSprint, t.Marcacao).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upserting tarefa from jira %s: %w", t.NumeroTicket, err)
	}
	return id, nil
}
```

- [ ] **Step 3: Add MoveTarefasToSprint method**

```go
func (r *PlanningRepository) MoveTarefasToSprint(ctx context.Context, sprintID uuid.UUID, tarefaIDs []uuid.UUID) error {
	if len(tarefaIDs) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE tarefas SET sprint_id = $1, data_entrada_sprint = NOW(), updated_at = NOW()
		WHERE id = ANY($2)
	`, sprintID, tarefaIDs)
	if err != nil {
		return fmt.Errorf("moving tarefas to sprint %s: %w", sprintID, err)
	}
	return nil
}
```

- [ ] **Step 4: Add GetTarefasByIDs method**

```go
func (r *PlanningRepository) GetTarefasByIDs(ctx context.Context, ids []uuid.UUID) ([]PlanningTarefa, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.status, t.prioridade,
		       t.estimativa_tempo, t.tipo_demanda, t.responsavel_id,
		       p.id, p.chave
		FROM tarefas t
		INNER JOIN projetos p ON p.id = t.projeto_id
		WHERE t.id = ANY($1) AND t.removido_em IS NULL
		ORDER BY t.numero_ticket
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("getting tarefas by ids: %w", err)
	}
	defer rows.Close()

	var result []PlanningTarefa
	for rows.Next() {
		var t PlanningTarefa
		if err := rows.Scan(&t.ID, &t.NumeroTicket, &t.Resumo, &t.Tipo, &t.Status,
			&t.Prioridade, &t.EstimativaTempo, &t.TipoDemanda, &t.ResponsavelID,
			&t.ProjetoID, &t.ProjetoChave); err != nil {
			return nil, fmt.Errorf("scanning tarefa by id: %w", err)
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tarefas by ids: %w", err)
	}
	return result, nil
}
```

- [ ] **Step 5: Add GetProjetoChaveByID method**

```go
func (r *PlanningRepository) GetProjetoChaveByID(ctx context.Context, projetoID uuid.UUID) (string, uuid.UUID, error) {
	var chave string
	var fonteDadosID uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT chave, fonte_dados_id FROM projetos WHERE id = $1`, projetoID).Scan(&chave, &fonteDadosID)
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("getting projeto chave for %s: %w", projetoID, err)
	}
	return chave, fonteDadosID, nil
}
```

- [ ] **Step 6: Add imports**

Add to `planning.go` imports if not present: `"encoding/json"`. Note: `UpsertTarefaParams` is defined in `sync.go` in the same `repository` package, so it's accessible without import.

- [ ] **Step 7: Update PlanningRepoStore interface**

In `backend/internal/service/interfaces.go`, add to `PlanningRepoStore` interface (after `GetSprintJiraID` at line 156):

```go
SearchTarefasByKeys(ctx context.Context, projetoID uuid.UUID, keys []string) ([]repository.SearchTarefaResult, error)
UpsertTarefaFromJira(ctx context.Context, t *repository.UpsertTarefaParams) (uuid.UUID, error)
MoveTarefasToSprint(ctx context.Context, sprintID uuid.UUID, tarefaIDs []uuid.UUID) error
GetTarefasByIDs(ctx context.Context, ids []uuid.UUID) ([]repository.PlanningTarefa, error)
GetProjetoChaveByID(ctx context.Context, projetoID uuid.UUID) (string, uuid.UUID, error)
```

- [ ] **Step 8: Build verification**

Run: `go build ./internal/repository/` and `go build ./internal/service/`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add backend/internal/repository/planning.go backend/internal/service/interfaces.go
git commit -m "feat: add planning repository methods for task search and inclusion"
```

---

### Task 3: Service — SearchTasks and IncludeTasks

**Files:**
- Modify: `backend/internal/service/planning.go` (after existing methods, ~line 417)

**Interfaces:**
- Consumes:
  - `PlanningRepoStore.SearchTarefasByKeys(ctx, projetoID, keys)` → `[]repository.SearchTarefaResult` (Task 2)
  - `PlanningRepoStore.UpsertTarefaFromJira(ctx, params)` → `uuid.UUID` (Task 2)
  - `PlanningRepoStore.MoveTarefasToSprint(ctx, sprintID, tarefaIDs)` (Task 2)
  - `PlanningRepoStore.GetTarefasByIDs(ctx, ids)` → `[]repository.PlanningTarefa` (Task 2)
  - `PlanningRepoStore.GetProjetoChaveByID(ctx, projetoID)` → `(string, uuid.UUID)` (Task 2)
  - `SprintRepoStore.GetByID(ctx, id)` → `*domain.Sprint` (existing)
  - `PlanningRepoStore.GetSprintJiraID(ctx, sprintID)` → `int` (existing)
  - `buildClient(ctx, fonteDadosID)` → `jira.Client` (existing private method on PlanningService)
  - `jira.Client.SearchIssuesByKeys(ctx, projectKey, keys)` → `[]jira.JiraIssue` (Task 1)
  - `jira.Client.MoveToSprint(ctx, sprintJiraID, issueKey)` (existing)
  - `repository.UpsertTarefaParams` (existing struct in repository/sync.go)
  - `repository.SearchTarefaResult` (Task 2)
  - `repository.PlanningTarefa` (existing)
- Produces:
  - `SearchTasksResult` struct with fields: `Found []SearchTasksFoundItem`, `NotFound []string`, `AlreadyInSprint []SearchTasksAlreadyItem`
  - `SearchTasksFoundItem` struct: `{ID uuid.UUID, Key string, Resumo string, Tipo string, Status string, Prioridade *string, ResponsavelNome *string, Source string}`
  - `SearchTasksAlreadyItem` struct: `{ID uuid.UUID, Key string, Resumo string, Tipo string, Status string}`
  - `(s *PlanningService) SearchTasks(ctx context.Context, sprintID uuid.UUID, ticketKeys []string) (*SearchTasksResult, error)`
  - `(s *PlanningService) IncludeTasks(ctx context.Context, sprintID uuid.UUID, tarefaIDs []uuid.UUID) ([]repository.PlanningTarefa, error)`

- [ ] **Step 1: Add result types**

Append to `backend/internal/service/planning.go`:

```go
type SearchTasksFoundItem struct {
	ID              uuid.UUID `json:"id"`
	Key             string    `json:"key"`
	Resumo          string    `json:"resumo"`
	Tipo            string    `json:"tipo"`
	Status          string    `json:"status"`
	Prioridade      *string   `json:"prioridade"`
	ResponsavelNome *string   `json:"responsavel_nome"`
	Source          string    `json:"source"`
}

type SearchTasksAlreadyItem struct {
	ID     uuid.UUID `json:"id"`
	Key    string    `json:"key"`
	Resumo string    `json:"resumo"`
	Tipo   string    `json:"tipo"`
	Status string    `json:"status"`
}

type SearchTasksResult struct {
	Found           []SearchTasksFoundItem   `json:"found"`
	NotFound        []string                 `json:"not_found"`
	AlreadyInSprint []SearchTasksAlreadyItem `json:"already_in_sprint"`
}
```

- [ ] **Step 2: Implement SearchTasks**

```go
func (s *PlanningService) SearchTasks(ctx context.Context, sprintID uuid.UUID, ticketKeys []string) (*SearchTasksResult, error) {
	sprint, err := s.sprintRepo.GetByID(ctx, sprintID)
	if err != nil {
		return nil, fmt.Errorf("getting sprint: %w", err)
	}
	if sprint.ProjetoID == nil {
		return nil, fmt.Errorf("sprint has no projeto_id")
	}

	projetoChave, fonteDadosID, err := s.planRepo.GetProjetoChaveByID(ctx, *sprint.ProjetoID)
	if err != nil {
		return nil, fmt.Errorf("getting projeto chave: %w", err)
	}

	localResults, err := s.planRepo.SearchTarefasByKeys(ctx, *sprint.ProjetoID, ticketKeys)
	if err != nil {
		return nil, fmt.Errorf("local search: %w", err)
	}

	foundMap := make(map[string]repository.SearchTarefaResult)
	for _, r := range localResults {
		foundMap[r.NumeroTicket] = r
	}

	var missingKeys []string
	for _, key := range ticketKeys {
		if _, ok := foundMap[key]; !ok {
			missingKeys = append(missingKeys, key)
		}
	}

	if len(missingKeys) > 0 {
		client, err := s.buildClient(ctx, fonteDadosID)
		if err != nil {
			s.logger.Warn("failed to build jira client for search fallback", zap.Error(err))
		} else {
			jiraIssues, err := client.SearchIssuesByKeys(ctx, projetoChave, missingKeys)
			if err != nil {
				s.logger.Warn("jira search fallback failed", zap.Error(err))
			} else {
				for _, issue := range jiraIssues {
					var estSeconds *int
					if issue.Fields.TimeTracking != nil && issue.Fields.TimeTracking.OriginalEstimateSeconds > 0 {
						v := issue.Fields.TimeTracking.OriginalEstimateSeconds
						estSeconds = &v
					}
					var prio *string
					if issue.Fields.Priority != nil {
						prio = &issue.Fields.Priority.Name
					}
					created, _ := time.Parse("2006-01-02T15:04:05.000-0700", issue.Fields.Created)
					var updated *time.Time
					if issue.Fields.Updated != "" {
						t, _ := time.Parse("2006-01-02T15:04:05.000-0700", issue.Fields.Updated)
						updated = &t
					}
					var statusCat *string
					if issue.Fields.Status.StatusCategory.Key != "" {
						statusCat = &issue.Fields.Status.StatusCategory.Key
					}

					params := &repository.UpsertTarefaParams{
						FonteDadosID:    fonteDadosID,
						ProjetoID:       *sprint.ProjetoID,
						JiraID:          issue.ID,
						NumeroTicket:    issue.Key,
						Resumo:          issue.Fields.Summary,
						Tipo:            issue.Fields.IssueType.Name,
						Status:          issue.Fields.Status.Name,
						Prioridade:      prio,
						EstimativaTempo: estSeconds,
						DataCriacao:     created,
						DataAtualizado:  updated,
						StatusCategoria: statusCat,
					}

					id, err := s.planRepo.UpsertTarefaFromJira(ctx, params)
					if err != nil {
						s.logger.Warn("failed to upsert jira issue", zap.String("key", issue.Key), zap.Error(err))
						continue
					}

					var assigneeName *string
					if issue.Fields.Assignee != nil {
						assigneeName = &issue.Fields.Assignee.DisplayName
					}

					foundMap[issue.Key] = repository.SearchTarefaResult{
						ID:              id,
						NumeroTicket:    issue.Key,
						Resumo:          issue.Fields.Summary,
						Tipo:            issue.Fields.IssueType.Name,
						Status:          issue.Fields.Status.Name,
						Prioridade:      prio,
						ResponsavelNome: assigneeName,
					}
				}
			}
		}
	}

	result := &SearchTasksResult{
		Found:           make([]SearchTasksFoundItem, 0),
		NotFound:        make([]string, 0),
		AlreadyInSprint: make([]SearchTasksAlreadyItem, 0),
	}

	for _, key := range ticketKeys {
		r, ok := foundMap[key]
		if !ok {
			result.NotFound = append(result.NotFound, key)
			continue
		}
		if r.SprintID != nil && *r.SprintID == sprintID {
			result.AlreadyInSprint = append(result.AlreadyInSprint, SearchTasksAlreadyItem{
				ID: r.ID, Key: r.NumeroTicket, Resumo: r.Resumo, Tipo: r.Tipo, Status: r.Status,
			})
			continue
		}
		source := "local"
		for _, mk := range missingKeys {
			if mk == key {
				source = "jira"
				break
			}
		}
		result.Found = append(result.Found, SearchTasksFoundItem{
			ID: r.ID, Key: r.NumeroTicket, Resumo: r.Resumo, Tipo: r.Tipo,
			Status: r.Status, Prioridade: r.Prioridade,
			ResponsavelNome: r.ResponsavelNome, Source: source,
		})
	}

	return result, nil
}
```

- [ ] **Step 3: Implement IncludeTasks**

```go
func (s *PlanningService) IncludeTasks(ctx context.Context, sprintID uuid.UUID, tarefaIDs []uuid.UUID) ([]repository.PlanningTarefa, error) {
	sprint, err := s.sprintRepo.GetByID(ctx, sprintID)
	if err != nil {
		return nil, fmt.Errorf("getting sprint: %w", err)
	}

	err = s.planRepo.MoveTarefasToSprint(ctx, sprintID, tarefaIDs)
	if err != nil {
		return nil, fmt.Errorf("moving tarefas to sprint: %w", err)
	}

	tarefas, err := s.planRepo.GetTarefasByIDs(ctx, tarefaIDs)
	if err != nil {
		return nil, fmt.Errorf("getting included tarefas: %w", err)
	}

	if sprint.ProjetoID != nil {
		_, fonteDadosID, err := s.planRepo.GetProjetoChaveByID(ctx, *sprint.ProjetoID)
		if err == nil {
			client, err := s.buildClient(ctx, fonteDadosID)
			if err == nil {
				sprintJiraID, err := s.planRepo.GetSprintJiraID(ctx, sprintID)
				if err == nil && sprintJiraID > 0 {
					for _, t := range tarefas {
						if err := client.MoveToSprint(ctx, sprintJiraID, t.NumeroTicket); err != nil {
							s.logger.Warn("failed to move issue to sprint in jira",
								zap.String("key", t.NumeroTicket), zap.Error(err))
						}
					}
				}
			}
		}
	}

	return tarefas, nil
}
```

- [ ] **Step 4: Add imports if needed**

Ensure `"time"` and `repository` import are present (likely already there).

- [ ] **Step 5: Build verification**

Run: `go build ./internal/service/`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/planning.go
git commit -m "feat: add SearchTasks and IncludeTasks to PlanningService"
```

---

### Task 4: Handler + Routes — SearchTasks and IncludeTasks Endpoints

**Files:**
- Modify: `backend/internal/handler/planning.go` (add to interface ~line 17, add handlers after line 107)
- Modify: `backend/cmd/api/main.go` (add routes after line 346)

**Interfaces:**
- Consumes:
  - `service.PlanningService.SearchTasks(ctx, sprintID, ticketKeys)` → `*service.SearchTasksResult` (Task 3)
  - `service.PlanningService.IncludeTasks(ctx, sprintID, tarefaIDs)` → `[]repository.PlanningTarefa` (Task 3)
  - `chi.URLParam(r, "id")` for sprint ID parsing (existing pattern)
  - `respondJSON(w, status, data)`, `respondError(w, status, msg)` (existing helpers in handler package)
- Produces:
  - `POST /sprints/{id}/search-tasks` endpoint
  - `POST /sprints/{id}/include-tasks` endpoint

- [ ] **Step 1: Add methods to PlanningServiceInterface**

In `backend/internal/handler/planning.go`, add to `PlanningServiceInterface` (after `GetProgress` at line 18):

```go
SearchTasks(ctx context.Context, sprintID uuid.UUID, ticketKeys []string) (*service.SearchTasksResult, error)
IncludeTasks(ctx context.Context, sprintID uuid.UUID, tarefaIDs []uuid.UUID) ([]repository.PlanningTarefa, error)
```

Also add `repository` import:
```go
"github.com/emersonpaula83/myplanner/backend/internal/repository"
```

- [ ] **Step 2: Add SearchTasks handler**

Append to `planning.go`:

```go
func (h *PlanningHandler) SearchTasks(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "ID inválido")
		return
	}

	var input struct {
		TicketKeys []string `json:"ticket_keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	normalized := make([]string, 0, len(input.TicketKeys))
	for _, k := range input.TicketKeys {
		k = strings.TrimSpace(strings.ToUpper(k))
		if k != "" {
			normalized = append(normalized, k)
		}
	}
	if len(normalized) == 0 {
		respondError(w, http.StatusBadRequest, "nenhuma chave informada")
		return
	}

	result, err := h.svc.SearchTasks(r.Context(), sprintID, normalized)
	if err != nil {
		h.logger.Error("failed to search tasks", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro ao pesquisar tarefas")
		return
	}

	respondJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 3: Add IncludeTasks handler**

```go
func (h *PlanningHandler) IncludeTasks(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "ID inválido")
		return
	}

	var input struct {
		TarefaIDs []uuid.UUID `json:"tarefa_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if len(input.TarefaIDs) == 0 {
		respondError(w, http.StatusBadRequest, "nenhuma tarefa selecionada")
		return
	}

	tarefas, err := h.svc.IncludeTasks(r.Context(), sprintID, input.TarefaIDs)
	if err != nil {
		h.logger.Error("failed to include tasks", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro ao incluir tarefas")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"tarefas": tarefas})
}
```

- [ ] **Step 4: Add imports to planning.go**

Add `"strings"` and `"encoding/json"` to imports if not present.

- [ ] **Step 5: Register routes in main.go**

In `backend/cmd/api/main.go`, add after the existing planning routes (after line 346):

```go
r.Post("/sprints/{id}/search-tasks", planningHandler.SearchTasks)
r.Post("/sprints/{id}/include-tasks", planningHandler.IncludeTasks)
```

- [ ] **Step 6: Build verification**

Run: `go build ./cmd/api/`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add backend/internal/handler/planning.go backend/cmd/api/main.go
git commit -m "feat: add search-tasks and include-tasks endpoints"
```

---

### Task 5: Frontend — Sticky Header and Action Bar Restructure

**Files:**
- Modify: `frontend/index.html` (CSS ~line 491, JS `renderPlanningTab` ~line 9697, `togglePlanningSelect` ~line 9899)

**Interfaces:**
- Consumes: existing `renderPlanningTab()`, `calcPlanningStats()`, `togglePlanningSelect()`, `planningState`, CSS classes `.planning-header-stats`, `.planning-bulk-bar`
- Produces:
  - New CSS: `.planning-sticky-header`, `.planning-action-bar`
  - Modified `renderPlanningTab()` — wraps title+stats+action bar in sticky header div
  - Modified `togglePlanningSelect()` — targets `planning-bulk-actions` instead of `planning-bulk-bar`
  - `openAddTasksModal()` stub (Task 6 implements the real function)

- [ ] **Step 1: Add CSS classes**

Insert after `.planning-bulk-count` CSS (after line 494 in `frontend/index.html`):

```css
.planning-sticky-header { position:sticky; top:0; z-index:10; background:var(--surface-primary); padding-bottom:8px; margin-bottom:12px; border-bottom:1px solid var(--border-subtle); }
.planning-action-bar { display:flex; align-items:center; justify-content:space-between; padding:8px 0; }
```

- [ ] **Step 2: Modify renderPlanningTab — wrap header in sticky div**

Replace the header generation block in `renderPlanningTab()` (lines 9702-9724):

Old code (lines 9702-9724):
```javascript
  var html = '<div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:16px">';
  html += '<div><div style="font-size:16px;font-weight:700">Planning: ' + esc(s.sprint.nome) + '</div>';
  html += '<div style="font-size:13px;color:var(--text-muted)">' + formatSprintDates(s.sprint.dataInicio, s.sprint.dataFim) + ' &bull; ' + s.diasUteis + ' dias uteis</div></div>';
  html += '<button class="planning-btn-finalizar" onclick="finalizarPlanning()" id="btn-finalizar-planning">Finalizar Planning</button>';
  html += '</div>';

  // stats bar
  html += '<div class="planning-header-stats">';
  html += '<div class="planning-stat tasks"><div class="planning-stat-label">Total Tarefas</div><div class="planning-stat-value" id="ps-total">' + stats.totalTarefas + '</div></div>';
  html += '<div class="planning-stat tasks"><div class="planning-stat-label">Alocadas</div><div class="planning-stat-value" id="ps-alocadas">' + stats.alocadas + '</div></div>';
  html += '<div class="planning-stat tasks"><div class="planning-stat-label">Nao Alocadas</div><div class="planning-stat-value" id="ps-nao-alocadas">' + stats.naoAlocadas + '</div></div>';
  html += '<div class="planning-stat hours"><div class="planning-stat-label">Horas Disponiveis</div><div class="planning-stat-value" id="ps-horas-disp">' + stats.horasDisponiveis.toFixed(1) + 'h</div></div>';
  html += '<div class="planning-stat hours"><div class="planning-stat-label">Horas Pendentes</div><div class="planning-stat-value" id="ps-horas-pend">' + stats.horasPendentes.toFixed(1) + 'h</div></div>';
  html += '<div class="planning-stat hours"><div class="planning-stat-label">Horas Alocadas</div><div class="planning-stat-value" id="ps-horas-aloc">' + stats.horasAlocadas.toFixed(1) + 'h</div></div>';
  html += '</div>';

  // bulk bar
  html += '<div class="planning-bulk-bar" id="planning-bulk-bar" style="display:none">';
  html += '<span class="planning-bulk-count" id="planning-bulk-count">0 selecionadas</span>';
  html += '<button class="btn-bulk" onclick="bulkAlocar()">Alocar</button>';
  html += '<button class="btn-bulk" onclick="bulkMoverProximaSprint()">Mover proxima Sprint</button>';
  html += '<button class="btn-bulk" onclick="bulkVoltarBacklog()">Voltar para Backlog</button>';
  html += '</div>';
```

New code:
```javascript
  var html = '<div class="planning-sticky-header">';

  html += '<div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px">';
  html += '<div><div style="font-size:16px;font-weight:700">Planning: ' + esc(s.sprint.nome) + '</div>';
  html += '<div style="font-size:13px;color:var(--text-muted)">' + formatSprintDates(s.sprint.dataInicio, s.sprint.dataFim) + ' &bull; ' + s.diasUteis + ' dias uteis</div></div>';
  html += '<button class="planning-btn-finalizar" onclick="finalizarPlanning()" id="btn-finalizar-planning">Finalizar Planning</button>';
  html += '</div>';

  html += '<div class="planning-header-stats">';
  html += '<div class="planning-stat tasks"><div class="planning-stat-label">Total Tarefas</div><div class="planning-stat-value" id="ps-total">' + stats.totalTarefas + '</div></div>';
  html += '<div class="planning-stat tasks"><div class="planning-stat-label">Alocadas</div><div class="planning-stat-value" id="ps-alocadas">' + stats.alocadas + '</div></div>';
  html += '<div class="planning-stat tasks"><div class="planning-stat-label">Nao Alocadas</div><div class="planning-stat-value" id="ps-nao-alocadas">' + stats.naoAlocadas + '</div></div>';
  html += '<div class="planning-stat hours"><div class="planning-stat-label">Horas Disponiveis</div><div class="planning-stat-value" id="ps-horas-disp">' + stats.horasDisponiveis.toFixed(1) + 'h</div></div>';
  html += '<div class="planning-stat hours"><div class="planning-stat-label">Horas Pendentes</div><div class="planning-stat-value" id="ps-horas-pend">' + stats.horasPendentes.toFixed(1) + 'h</div></div>';
  html += '<div class="planning-stat hours"><div class="planning-stat-label">Horas Alocadas</div><div class="planning-stat-value" id="ps-horas-aloc">' + stats.horasAlocadas.toFixed(1) + 'h</div></div>';
  html += '</div>';

  html += '<div class="planning-action-bar">';
  html += '<div class="planning-bulk-bar" id="planning-bulk-actions" style="display:none">';
  html += '<span class="planning-bulk-count" id="planning-bulk-count">0 selecionadas</span>';
  html += '<button class="btn-bulk" onclick="bulkAlocar()">Alocar</button>';
  html += '<button class="btn-bulk" onclick="bulkMoverProximaSprint()">Mover proxima Sprint</button>';
  html += '<button class="btn-bulk" onclick="bulkVoltarBacklog()">Voltar para Backlog</button>';
  html += '</div>';
  html += '<button class="btn-sm primary" onclick="openAddTasksModal()">+ Incluir Tarefas</button>';
  html += '</div>';

  html += '</div>';
```

- [ ] **Step 3: Update togglePlanningSelect**

In `togglePlanningSelect` function (~line 9899), change the bulk bar target from `planning-bulk-bar` to `planning-bulk-actions`:

Replace:
```javascript
  var bar = document.getElementById('planning-bulk-bar');
```
With:
```javascript
  var bar = document.getElementById('planning-bulk-actions');
```

- [ ] **Step 4: Add openAddTasksModal stub**

Insert after `togglePlanningSelect` function (before `// --- Planning Modals ---` comment):

```javascript
function openAddTasksModal() {}
```

- [ ] **Step 5: Manual test**

1. Open Planning tab
2. Verify header sticks to top when scrolling down through member cards
3. Verify stats bar stays visible
4. Verify "+ Incluir Tarefas" button visible in action bar
5. Select tasks — verify bulk buttons appear on left, "+ Incluir Tarefas" stays on right
6. Deselect all — verify bulk buttons hide, "+ Incluir Tarefas" remains
7. Test light/dark mode — verify sticky header background matches theme

- [ ] **Step 6: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add sticky header and restructured action bar to Planning tab"
```

---

### Task 6: Frontend — Add Tasks Modal (Search + Selection + Inclusion)

**Files:**
- Modify: `frontend/index.html` (modal HTML after existing modals ~line 1712, CSS, JS after `openAddTasksModal` stub)

**Interfaces:**
- Consumes:
  - `POST /sprints/{id}/search-tasks` with `{"ticket_keys": [...]}` → `{found, not_found, already_in_sprint}` (Task 4)
  - `POST /sprints/{id}/include-tasks` with `{"tarefa_ids": [...]}` → `{tarefas: [...]}` (Task 4)
  - `window.planningState` — `sprint.id`, `tasks` map, `unassigned` array (existing)
  - `api(path, opts)` helper (existing, ~line 2276)
  - `esc(s)` HTML escaper (existing, ~line 2290)
  - `renderPlanningTab()` (existing)
  - `openAddTasksModal()` stub (Task 5 — replace with real implementation)
- Produces:
  - Modal HTML with id `add-tasks-modal`
  - CSS: `.add-tasks-results`, `.add-tasks-section`, `.add-tasks-section-title`, `.add-tasks-item`, `.add-tasks-tag`, `.add-tasks-notfound`
  - `openAddTasksModal()` — replaces stub, opens modal, clears state
  - `closeAddTasksModal()` — closes modal
  - `searchTasksForSprint()` — POST search, render results
  - `renderAddTasksResults(data)` — render found/already/not_found sections
  - `toggleAddTaskCheck(id)` — toggle selection, update button count
  - `includeTasksInSprint()` — POST include, update planningState, close modal
  - Globals: `var _addTasksResults`, `var _addTasksSelected`

- [ ] **Step 1: Add CSS for modal**

Insert after `.planning-action-bar` CSS (added in Task 5):

```css
.add-tasks-results { margin-top:12px; max-height:400px; overflow-y:auto; }
.add-tasks-section { margin-bottom:12px; }
.add-tasks-section-title { font-size:12px; font-weight:600; color:var(--text-tertiary); text-transform:uppercase; letter-spacing:.5px; margin-bottom:6px; }
.add-tasks-item { display:flex; align-items:center; gap:8px; padding:8px 12px; border-bottom:1px solid var(--border-subtle); }
.add-tasks-item.disabled { opacity:0.5; }
.add-tasks-item label { display:flex; align-items:center; gap:8px; flex:1; cursor:pointer; min-width:0; }
.add-tasks-tag { font-size:10px; padding:2px 6px; border-radius:4px; font-weight:600; flex-shrink:0; }
.add-tasks-tag.local { background:var(--accent-soft); color:var(--accent); }
.add-tasks-tag.jira { background:#E3F2FD; color:#1565C0; }
.add-tasks-tag.already { background:var(--surface-secondary); color:var(--text-muted); }
.add-tasks-notfound { color:var(--red); font-size:13px; padding:4px 12px; }
```

- [ ] **Step 2: Add modal HTML**

Insert after the last `modal-overlay` div (after `msg-settings-modal` ~line 1712):

```html
<div class="modal-overlay" id="add-tasks-modal" onclick="if(event.target===this)closeAddTasksModal()">
  <div class="modal" style="max-width:620px">
    <div class="modal-title">Incluir Tarefas na Sprint</div>
    <div style="margin-bottom:12px">
      <textarea id="add-tasks-input" rows="3" style="width:100%;padding:8px 12px;border:1px solid var(--border);border-radius:6px;background:var(--surface);color:var(--text-primary);font-size:13px;resize:vertical" placeholder="Cole as chaves dos tickets (ex: PROJ-123, PROJ-456)"></textarea>
    </div>
    <div style="margin-bottom:12px">
      <button class="btn-sm primary" id="btn-search-tasks" onclick="searchTasksForSprint()">Pesquisar</button>
    </div>
    <div id="add-tasks-results-area"></div>
    <div class="modal-actions">
      <button class="btn-sm primary" id="btn-include-tasks" style="display:none" onclick="includeTasksInSprint()">Incluir na Sprint</button>
      <button class="btn-cancel" type="button" onclick="closeAddTasksModal()">Fechar</button>
    </div>
  </div>
</div>
```

- [ ] **Step 3: Replace openAddTasksModal stub and add closeAddTasksModal**

Replace the stub `function openAddTasksModal() {}` with:

```javascript
var _addTasksResults = [];
var _addTasksSelected = new Set();

function openAddTasksModal() {
  _addTasksResults = [];
  _addTasksSelected = new Set();
  document.getElementById('add-tasks-input').value = '';
  document.getElementById('add-tasks-results-area').innerHTML = '';
  document.getElementById('btn-include-tasks').style.display = 'none';
  document.getElementById('add-tasks-modal').classList.add('open');
}

function closeAddTasksModal() {
  document.getElementById('add-tasks-modal').classList.remove('open');
}
```

- [ ] **Step 4: Add searchTasksForSprint**

```javascript
async function searchTasksForSprint() {
  var input = document.getElementById('add-tasks-input').value;
  var keys = input.split(/[\s,;\n]+/).map(function(k) { return k.trim().toUpperCase(); }).filter(function(k) { return k.length > 0; });
  if (keys.length === 0) { alert('Informe ao menos uma chave de ticket.'); return; }

  var btn = document.getElementById('btn-search-tasks');
  btn.disabled = true; btn.textContent = 'Pesquisando...';
  var area = document.getElementById('add-tasks-results-area');
  area.innerHTML = '<div class="loading"><div class="spinner"></div></div>';

  try {
    var s = window.planningState;
    var data = await api('/sprints/' + s.sprint.id + '/search-tasks', {
      method: 'POST',
      body: JSON.stringify({ ticket_keys: keys })
    });
    _addTasksResults = data.found || [];
    _addTasksSelected = new Set();
    renderAddTasksResults(data);
  } catch (err) {
    area.innerHTML = '<div style="color:var(--red);padding:12px">' + esc(err.message) + '</div>';
  } finally {
    btn.disabled = false; btn.textContent = 'Pesquisar';
  }
}
```

- [ ] **Step 5: Add renderAddTasksResults**

```javascript
function renderAddTasksResults(data) {
  var area = document.getElementById('add-tasks-results-area');
  var html = '<div class="add-tasks-results">';

  if (data.found && data.found.length > 0) {
    html += '<div class="add-tasks-section"><div class="add-tasks-section-title">Encontrados (' + data.found.length + ')</div>';
    data.found.forEach(function(t) {
      html += '<div class="add-tasks-item">';
      html += '<label><input type="checkbox" onchange="toggleAddTaskCheck(\'' + t.id + '\')">';
      html += '<span class="project-key">' + esc(t.key) + '</span>';
      html += '<span style="font-size:13px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(t.resumo) + '</span></label>';
      html += '<span class="add-tasks-tag ' + t.source + '">' + t.source + '</span>';
      html += '</div>';
    });
    html += '</div>';
  }

  if (data.already_in_sprint && data.already_in_sprint.length > 0) {
    html += '<div class="add-tasks-section"><div class="add-tasks-section-title">Ja na sprint (' + data.already_in_sprint.length + ')</div>';
    data.already_in_sprint.forEach(function(t) {
      html += '<div class="add-tasks-item disabled">';
      html += '<label><input type="checkbox" disabled>';
      html += '<span class="project-key">' + esc(t.key) + '</span>';
      html += '<span style="font-size:13px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + esc(t.resumo) + '</span></label>';
      html += '<span class="add-tasks-tag already">Ja na sprint</span>';
      html += '</div>';
    });
    html += '</div>';
  }

  if (data.not_found && data.not_found.length > 0) {
    html += '<div class="add-tasks-section"><div class="add-tasks-section-title">Nao encontrados</div>';
    data.not_found.forEach(function(k) {
      html += '<div class="add-tasks-notfound">' + esc(k) + '</div>';
    });
    html += '</div>';
  }

  if ((!data.found || data.found.length === 0) && (!data.already_in_sprint || data.already_in_sprint.length === 0) && (!data.not_found || data.not_found.length === 0)) {
    html += '<div style="padding:20px;text-align:center;color:var(--text-muted)">Nenhum ticket encontrado</div>';
  }

  html += '</div>';
  area.innerHTML = html;

  var includeBtn = document.getElementById('btn-include-tasks');
  includeBtn.style.display = (data.found && data.found.length > 0) ? '' : 'none';
  includeBtn.textContent = 'Incluir na Sprint (0)';
  includeBtn.disabled = true;
}
```

- [ ] **Step 6: Add toggleAddTaskCheck**

```javascript
function toggleAddTaskCheck(id) {
  if (_addTasksSelected.has(id)) {
    _addTasksSelected.delete(id);
  } else {
    _addTasksSelected.add(id);
  }
  var btn = document.getElementById('btn-include-tasks');
  btn.textContent = 'Incluir na Sprint (' + _addTasksSelected.size + ')';
  btn.disabled = _addTasksSelected.size === 0;
}
```

- [ ] **Step 7: Add includeTasksInSprint**

```javascript
async function includeTasksInSprint() {
  if (_addTasksSelected.size === 0) return;

  var btn = document.getElementById('btn-include-tasks');
  btn.disabled = true; btn.textContent = 'Incluindo...';

  try {
    var s = window.planningState;
    var result = await api('/sprints/' + s.sprint.id + '/include-tasks', {
      method: 'POST',
      body: JSON.stringify({ tarefa_ids: Array.from(_addTasksSelected) })
    });

    if (result.tarefas) {
      result.tarefas.forEach(function(t) {
        var estSec = t.estimativa_tempo || 0;
        s.tasks[t.id] = {
          id: t.id,
          key: t.numero_ticket,
          resumo: t.resumo,
          tipo: t.tipo,
          status: t.status,
          prioridade: t.prioridade,
          responsavelId: t.responsavel_id || null,
          estimativaTempo: estSec,
          tipoDemanda: t.tipo_demanda || null,
          projetoChave: t.projeto_chave,
          originalResponsavelId: t.responsavel_id || null,
          originalEstimativa: estSec,
          originalTipoDemanda: t.tipo_demanda || null,
          originalSprintId: s.sprint.id
        };
        if (!s.unassigned.includes(t.id)) {
          s.unassigned.push(t.id);
        }
      });
    }

    closeAddTasksModal();
    renderPlanningTab();
  } catch (err) {
    alert('Erro ao incluir tarefas: ' + err.message);
    btn.disabled = false;
    btn.textContent = 'Incluir na Sprint (' + _addTasksSelected.size + ')';
  }
}
```

- [ ] **Step 8: Manual test**

1. Open Planning tab, click "+ Incluir Tarefas"
2. Verify modal opens with empty textarea
3. Type valid ticket keys (e.g. `PROJ-123, PROJ-456`), click "Pesquisar"
4. Verify results appear with checkboxes, key badges, source tags
5. Select 2 tasks — verify "Incluir na Sprint (2)" button enables
6. Click "Incluir na Sprint" — verify tasks appear in "Nao Alocadas" section
7. Reopen modal, search same keys — verify they now show as "Ja na sprint"
8. Search invalid key — verify "Nao encontrados" section shows
9. Test light/dark mode for modal styling
10. Test sticky header still works after adding tasks

- [ ] **Step 9: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add search and include tasks modal to Planning tab"
```

---
