package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type NextSprintResult struct {
	Sprint  domain.Sprint               `json:"sprint"`
	Tarefas []repository.PlanningTarefa `json:"tarefas"`
}

type ReassignChange struct {
	TarefaID          uuid.UUID  `json:"tarefa_id"`
	TarefaKey         string     `json:"tarefa_key"`
	NovoResponsavelID *uuid.UUID `json:"novo_responsavel_id"`
}

type EstimateChange struct {
	TarefaID  uuid.UUID `json:"tarefa_id"`
	TarefaKey string    `json:"tarefa_key"`
	Segundos  int       `json:"segundos"`
}

type TipoDemandaChange struct {
	TarefaID  uuid.UUID `json:"tarefa_id"`
	TarefaKey string    `json:"tarefa_key"`
	Valor     string    `json:"valor"`
}

type MoveSprintChange struct {
	TarefaID            uuid.UUID `json:"tarefa_id"`
	TarefaKey           string    `json:"tarefa_key"`
	DestinoSprintJiraID int       `json:"destino_sprint_jira_id"`
}

type MoveBacklogChange struct {
	TarefaID  uuid.UUID `json:"tarefa_id"`
	TarefaKey string    `json:"tarefa_key"`
}

type PlanningChanges struct {
	Reassigned      []ReassignChange    `json:"reassigned"`
	Estimated       []EstimateChange    `json:"estimated"`
	TipoDemanda     []TipoDemandaChange `json:"tipo_demanda"`
	MovedNextSprint []MoveSprintChange  `json:"moved_next_sprint"`
	MovedBacklog    []MoveBacklogChange `json:"moved_backlog"`
}

type PlanningApplyRequest struct {
	SprintID     uuid.UUID       `json:"-"`
	FonteDadosID uuid.UUID       `json:"fonte_dados_id"`
	Changes      PlanningChanges `json:"changes"`
}

type PlanningOperation struct {
	Key    string `json:"key"`
	Action string `json:"action"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type PlanningJobProgress struct {
	mu         sync.RWMutex        `json:"-"`
	Total      int                 `json:"total"`
	Completed  int                 `json:"completed"`
	Current    string              `json:"current"`
	Operations []PlanningOperation `json:"operations"`
	Finished   bool                `json:"finished"`
	Errors     []PlanningOperation `json:"errors"`
}

type PlanningService struct {
	planRepo           PlanningRepoStore
	sprintRepo         SprintRepoStore
	fdRepo             FonteDadosStore
	clientFactory      ClientFactory
	oauthClientFactory OAuthClientFactory
	oauthSvc           *jira.OAuthService
	rateLimit          int
	logger             *zap.Logger
	jobs               sync.Map
}

func NewPlanningService(
	planRepo PlanningRepoStore,
	sprintRepo SprintRepoStore,
	fdRepo FonteDadosStore,
	clientFactory ClientFactory,
	oauthClientFactory OAuthClientFactory,
	oauthSvc *jira.OAuthService,
	rateLimit int,
	logger *zap.Logger,
) *PlanningService {
	return &PlanningService{
		planRepo:           planRepo,
		sprintRepo:         sprintRepo,
		fdRepo:             fdRepo,
		clientFactory:      clientFactory,
		oauthClientFactory: oauthClientFactory,
		oauthSvc:           oauthSvc,
		rateLimit:          rateLimit,
		logger:             logger,
	}
}

func (s *PlanningService) GetNextSprint(ctx context.Context, currentSprintID uuid.UUID, equipeID *uuid.UUID) (*NextSprintResult, error) {
	currentSprint, err := s.sprintRepo.GetByID(ctx, currentSprintID)
	if err != nil {
		return nil, fmt.Errorf("getting current sprint: %w", err)
	}
	if currentSprint.BoardID == nil {
		return nil, fmt.Errorf("current sprint has no board_id")
	}
	if currentSprint.DataInicio == nil {
		return nil, fmt.Errorf("current sprint has no data_inicio")
	}

	nextSprint, err := s.planRepo.GetNextSprint(ctx, *currentSprint.BoardID, *currentSprint.DataInicio)
	if err != nil {
		return nil, fmt.Errorf("finding next sprint: %w", err)
	}
	if nextSprint == nil {
		return nil, nil
	}

	tarefas, err := s.planRepo.GetAllTarefasBySprint(ctx, nextSprint.ID)
	if err != nil {
		return nil, fmt.Errorf("getting tarefas for next sprint: %w", err)
	}
	if tarefas == nil {
		tarefas = []repository.PlanningTarefa{}
	}

	return &NextSprintResult{
		Sprint:  *nextSprint,
		Tarefas: tarefas,
	}, nil
}

func (s *PlanningService) Apply(ctx context.Context, req PlanningApplyRequest) (string, error) {
	client, err := s.buildClient(ctx, req.FonteDadosID)
	if err != nil {
		return "", fmt.Errorf("building jira client: %w", err)
	}

	ops := s.buildOperations(req.Changes)
	if len(ops) == 0 {
		return "", fmt.Errorf("no operations to apply")
	}

	jobID := uuid.New().String()
	progress := &PlanningJobProgress{
		Total:      len(ops),
		Operations: ops,
		Errors:     []PlanningOperation{},
	}
	s.jobs.Store(jobID, progress)

	go s.processJob(context.Background(), jobID, req, client, progress)

	return jobID, nil
}

func (s *PlanningService) GetProgress(jobID string) *PlanningJobProgress {
	val, ok := s.jobs.Load(jobID)
	if !ok {
		return nil
	}
	p := val.(*PlanningJobProgress)
	p.mu.RLock()
	snapshot := &PlanningJobProgress{
		Total:      p.Total,
		Completed:  p.Completed,
		Current:    p.Current,
		Operations: make([]PlanningOperation, len(p.Operations)),
		Finished:   p.Finished,
		Errors:     make([]PlanningOperation, len(p.Errors)),
	}
	copy(snapshot.Operations, p.Operations)
	copy(snapshot.Errors, p.Errors)
	p.mu.RUnlock()
	return snapshot
}

func (s *PlanningService) buildOperations(changes PlanningChanges) []PlanningOperation {
	var ops []PlanningOperation
	for _, c := range changes.Reassigned {
		ops = append(ops, PlanningOperation{Key: c.TarefaKey, Action: "assign", Status: "pending"})
	}
	for _, c := range changes.Estimated {
		ops = append(ops, PlanningOperation{Key: c.TarefaKey, Action: "estimate", Status: "pending"})
	}
	for _, c := range changes.TipoDemanda {
		ops = append(ops, PlanningOperation{Key: c.TarefaKey, Action: "tipo_demanda", Status: "pending"})
	}
	for _, c := range changes.MovedNextSprint {
		ops = append(ops, PlanningOperation{Key: c.TarefaKey, Action: "move_sprint", Status: "pending"})
	}
	for _, c := range changes.MovedBacklog {
		ops = append(ops, PlanningOperation{Key: c.TarefaKey, Action: "move_backlog", Status: "pending"})
	}
	return ops
}

func (p *PlanningJobProgress) setRunning(idx int, current string) {
	p.mu.Lock()
	p.Operations[idx].Status = "running"
	p.Current = current
	p.mu.Unlock()
}

func (p *PlanningJobProgress) setDone(idx int) {
	p.mu.Lock()
	p.Operations[idx].Status = "done"
	p.Completed++
	p.mu.Unlock()
}

func (p *PlanningJobProgress) setError(idx int, errMsg string) {
	p.mu.Lock()
	p.Operations[idx].Status = "error"
	p.Operations[idx].Error = errMsg
	p.Errors = append(p.Errors, p.Operations[idx])
	p.Completed++
	p.mu.Unlock()
}

func (p *PlanningJobProgress) setFinished() {
	p.mu.Lock()
	p.Current = ""
	p.Finished = true
	p.mu.Unlock()
}

func (s *PlanningService) processJob(ctx context.Context, jobID string, req PlanningApplyRequest, client jira.Client, progress *PlanningJobProgress) {
	idx := 0

	for _, c := range req.Changes.Reassigned {
		progress.setRunning(idx, c.TarefaKey+" — Atualizando responsável")

		var jiraErr error
		if c.NovoResponsavelID != nil {
			jiraAccountID, err := s.sprintRepo.GetMembroJiraAccountID(ctx, *c.NovoResponsavelID)
			if err != nil {
				jiraErr = fmt.Errorf("membro não encontrado: %w", err)
			} else {
				if err := s.planRepo.UpdateTarefaResponsavel(ctx, c.TarefaID, c.NovoResponsavelID); err != nil {
					jiraErr = err
				} else if err := client.AssignIssue(ctx, c.TarefaKey, jiraAccountID); err != nil {
					jiraErr = err
				}
			}
		} else {
			if err := s.planRepo.UpdateTarefaResponsavel(ctx, c.TarefaID, nil); err != nil {
				jiraErr = err
			}
		}

		if jiraErr != nil {
			s.logger.Warn("planning assign failed", zap.String("key", c.TarefaKey), zap.Error(jiraErr))
			progress.setError(idx, jiraErr.Error())
		} else {
			progress.setDone(idx)
		}
		idx++
	}

	for _, c := range req.Changes.Estimated {
		progress.setRunning(idx, c.TarefaKey+" — Atualizando estimativa")

		var jiraErr error
		if err := s.planRepo.UpdateTarefaEstimativa(ctx, c.TarefaID, c.Segundos); err != nil {
			jiraErr = err
		} else if err := client.UpdateTimeEstimate(ctx, c.TarefaKey, c.Segundos); err != nil {
			jiraErr = err
		}

		if jiraErr != nil {
			s.logger.Warn("planning estimate failed", zap.String("key", c.TarefaKey), zap.Error(jiraErr))
			progress.setError(idx, jiraErr.Error())
		} else {
			progress.setDone(idx)
		}
		idx++
	}

	for _, c := range req.Changes.TipoDemanda {
		progress.setRunning(idx, c.TarefaKey+" — Atualizando tipo de demanda")

		if err := s.planRepo.UpdateTarefaTipoDemanda(ctx, c.TarefaID, c.Valor); err != nil {
			s.logger.Warn("planning tipo_demanda failed", zap.String("key", c.TarefaKey), zap.Error(err))
			progress.setError(idx, err.Error())
		} else {
			progress.setDone(idx)
		}
		idx++
	}

	for _, c := range req.Changes.MovedNextSprint {
		progress.setRunning(idx, c.TarefaKey+" — Movendo para próxima sprint")

		var jiraErr error
		sprintID, err := s.findSprintIDByJiraID(ctx, req.SprintID, c.DestinoSprintJiraID)
		if err != nil {
			jiraErr = err
		} else {
			if err := s.planRepo.MoveTarefaToSprint(ctx, c.TarefaID, sprintID); err != nil {
				jiraErr = err
			} else if err := client.MoveToSprint(ctx, c.DestinoSprintJiraID, c.TarefaKey); err != nil {
				jiraErr = err
			}
		}

		if jiraErr != nil {
			s.logger.Warn("planning move_sprint failed", zap.String("key", c.TarefaKey), zap.Error(jiraErr))
			progress.setError(idx, jiraErr.Error())
		} else {
			progress.setDone(idx)
		}
		idx++
	}

	for _, c := range req.Changes.MovedBacklog {
		progress.setRunning(idx, c.TarefaKey+" — Movendo para backlog")

		var jiraErr error
		if err := s.planRepo.RemoveTarefaFromSprint(ctx, c.TarefaID); err != nil {
			jiraErr = err
		} else if err := client.RemoveFromSprint(ctx, c.TarefaKey); err != nil {
			jiraErr = err
		}

		if jiraErr != nil {
			s.logger.Warn("planning move_backlog failed", zap.String("key", c.TarefaKey), zap.Error(jiraErr))
			progress.setError(idx, jiraErr.Error())
		} else {
			progress.setDone(idx)
		}
		idx++
	}

	progress.setFinished()
}

func (s *PlanningService) findSprintIDByJiraID(ctx context.Context, currentSprintID uuid.UUID, jiraID int) (uuid.UUID, error) {
	currentSprint, err := s.sprintRepo.GetByID(ctx, currentSprintID)
	if err != nil {
		return uuid.Nil, err
	}
	if currentSprint.BoardID == nil || currentSprint.DataInicio == nil {
		return uuid.Nil, fmt.Errorf("current sprint missing board_id or data_inicio")
	}
	nextSprint, err := s.planRepo.GetNextSprint(ctx, *currentSprint.BoardID, *currentSprint.DataInicio)
	if err != nil {
		return uuid.Nil, err
	}
	if nextSprint == nil {
		return uuid.Nil, fmt.Errorf("next sprint not found")
	}
	// the next-next sprint (for "mover próxima sprint" from the planning sprint)
	nextNextSprint, err := s.planRepo.GetNextSprint(ctx, *currentSprint.BoardID, *nextSprint.DataInicio)
	if err != nil {
		return uuid.Nil, err
	}
	if nextNextSprint != nil && nextNextSprint.JiraID == jiraID {
		return nextNextSprint.ID, nil
	}
	return uuid.Nil, fmt.Errorf("no local sprint found with jira_id %d", jiraID)
}

func (s *PlanningService) buildClient(ctx context.Context, fonteDadosID uuid.UUID) (jira.Client, error) {
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
