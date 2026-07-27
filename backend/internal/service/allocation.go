package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

type ProjectAllocation struct {
	EpicID        uuid.UUID  `json:"epic_id"`
	NumeroTicket  string     `json:"numero_ticket"`
	Resumo        string     `json:"resumo"`
	Apelido       *string    `json:"apelido"`
	DataLimite    *time.Time `json:"data_limite"`
	Prioridade    *string    `json:"prioridade"`
	TipoDemanda   *string    `json:"tipo_demanda"`
	Produtos      []string   `json:"produtos"`
	PctEstimado   float64    `json:"pct_estimado"`
	PctPlanejado  float64    `json:"pct_planejado"`
	TarefasSemEst int        `json:"tarefas_sem_estimativa"`
	TotalTarefas  int        `json:"total_tarefas"`
	IsGDPTC       bool       `json:"is_gdptc"`
	Status        string     `json:"status"`
}

type TaskAllocation struct {
	TarefaID        uuid.UUID  `json:"tarefa_id"`
	NumeroTicket    string     `json:"numero_ticket"`
	Resumo          string     `json:"resumo"`
	Tipo            string     `json:"tipo"`
	TipoDemanda     *string    `json:"tipo_demanda"`
	Status          string     `json:"status"`
	EstimativaHoras *float64   `json:"estimativa_horas"`
	SprintID        *uuid.UUID `json:"sprint_id"`
	SprintNome      *string    `json:"sprint_nome"`
	SprintInicio    *time.Time `json:"sprint_inicio"`
	SprintFim       *time.Time `json:"sprint_fim"`
	ResponsavelID   *uuid.UUID `json:"responsavel_id"`
	ResponsavelNome *string    `json:"responsavel_nome"`
}

type PersonAllocation struct {
	MembroID       uuid.UUID `json:"membro_id"`
	Nome           string    `json:"nome"`
	HorasNoProjeto float64   `json:"horas_no_projeto"`
	HorasCapTotal  float64   `json:"horas_cap_total"`
	PctNoProjeto   float64   `json:"pct_no_projeto"`
}

type ProjectDetail struct {
	Epic        ProjectAllocation  `json:"epic"`
	Pessoas     []PersonAllocation `json:"pessoas"`
	NaoAlocadas []TaskAllocation   `json:"nao_alocadas"`
	Parciais    []TaskAllocation   `json:"parciais"`
	Completas   []TaskAllocation   `json:"completas"`
}

type SprintOption struct {
	ID     uuid.UUID `json:"id"`
	JiraID int       `json:"jira_id"`
	Nome   string    `json:"nome"`
	Inicio time.Time `json:"inicio"`
	Fim    time.Time `json:"fim"`
	Estado string    `json:"estado"`
}

type AllocateTaskRequest struct {
	TaskID        uuid.UUID  `json:"task_id"`
	SprintID      uuid.UUID  `json:"sprint_id"`
	AssigneeID    *uuid.UUID `json:"assignee_id"`
	EstimateHours float64    `json:"estimate_hours"`
	Force         bool       `json:"force"`
	EquipeID      uuid.UUID  `json:"equipe_id"`
}

type AllocateTaskResult struct {
	Conflict   bool    `json:"conflict,omitempty"`
	MembroNome string  `json:"membro_nome,omitempty"`
	SprintNome string  `json:"sprint_nome,omitempty"`
	PctAtual   float64 `json:"pct_atual,omitempty"`
}

type AllocationService struct {
	repo          *repository.AllocationRepository
	sprintSvc     *SprintService
	sprintRepo    *repository.SprintRepository
	fdRepo        *repository.FonteDadosRepository
	syncSvc       *SyncService
	clientFactory ClientFactory
	oauthFactory  OAuthClientFactory
	oauthSvc      *jira.OAuthService
	rateLimit     int
	logger        *zap.Logger
}

func NewAllocationService(
	repo *repository.AllocationRepository,
	sprintSvc *SprintService,
	sprintRepo *repository.SprintRepository,
	fdRepo *repository.FonteDadosRepository,
	syncSvc *SyncService,
	clientFactory ClientFactory,
	oauthFactory OAuthClientFactory,
	oauthSvc *jira.OAuthService,
	rateLimit int,
	logger *zap.Logger,
) *AllocationService {
	return &AllocationService{
		repo:          repo,
		sprintSvc:     sprintSvc,
		sprintRepo:    sprintRepo,
		fdRepo:        fdRepo,
		syncSvc:       syncSvc,
		clientFactory: clientFactory,
		oauthFactory:  oauthFactory,
		oauthSvc:      oauthSvc,
		rateLimit:     rateLimit,
		logger:        logger,
	}
}

func (s *AllocationService) ListProjectAllocations(ctx context.Context, equipeID, produtoID uuid.UUID) ([]ProjectAllocation, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("allocation repository not configured")
	}

	rows, err := s.repo.GetEpicsByEquipeAndProduto(ctx, equipeID, produtoID)
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

		result = append(result, ProjectAllocation{
			EpicID:        r.EpicID,
			NumeroTicket:  r.NumeroTicket,
			Resumo:        r.Resumo,
			Apelido:       r.Apelido,
			DataLimite:    r.DataLimite,
			Prioridade:    r.Prioridade,
			TipoDemanda:   r.TipoDemanda,
			Produtos:      r.Produtos,
			PctEstimado:   pctEstimado,
			PctPlanejado:  pctPlanejado,
			TarefasSemEst: r.TotalFilhas - r.FilhasComEstimativa,
			TotalTarefas:  r.TotalFilhas,
			IsGDPTC:       gdptcMap[r.EpicID],
			Status:        status,
		})
	}

	return result, nil
}

func (s *AllocationService) GetProjectDetail(ctx context.Context, epicID, equipeID uuid.UUID) (*ProjectDetail, error) {
	epics, err := s.repo.GetEpicsByEquipeAndProduto(ctx, equipeID, uuid.Nil)
	if err != nil {
		return nil, fmt.Errorf("getting epic: %w", err)
	}

	var epicRow *repository.EpicAllocationRow
	for i := range epics {
		if epics[i].EpicID == epicID {
			epicRow = &epics[i]
			break
		}
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

	var epic ProjectAllocation
	if epicRow != nil {
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
		epic = ProjectAllocation{
			EpicID: epicRow.EpicID, NumeroTicket: epicRow.NumeroTicket,
			Resumo: epicRow.Resumo, Apelido: epicRow.Apelido,
			DataLimite: epicRow.DataLimite, Prioridade: epicRow.Prioridade,
			TipoDemanda: epicRow.TipoDemanda, Produtos: epicRow.Produtos,
			PctEstimado: pctEstimado, PctPlanejado: pctPlanejado,
			TarefasSemEst: epicRow.TotalFilhas - epicRow.FilhasComEstimativa,
			TotalTarefas: epicRow.TotalFilhas, IsGDPTC: gdptcMap[epicID],
			Status: status,
		}
	} else {
		epic = ProjectAllocation{EpicID: epicID, Status: "nao_planejado"}
	}

	pessoas := make([]PersonAllocation, 0, len(people))
	for _, p := range people {
		pessoas = append(pessoas, PersonAllocation{
			MembroID:       p.MembroID,
			Nome:           p.Nome,
			HorasNoProjeto: p.HorasNoProjeto,
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

func taskRowToAllocation(t repository.TaskAllocationRow) TaskAllocation {
	ta := TaskAllocation{
		TarefaID:        t.TarefaID,
		NumeroTicket:    t.NumeroTicket,
		Resumo:          t.Resumo,
		Tipo:            t.Tipo,
		TipoDemanda:     t.TipoDemanda,
		Status:          t.Status,
		SprintID:        t.SprintID,
		SprintNome:      t.SprintNome,
		SprintInicio:    t.SprintInicio,
		SprintFim:       t.SprintFim,
		ResponsavelID:   t.ResponsavelID,
		ResponsavelNome: t.ResponsavelNome,
	}
	if t.EstimativaTempo != nil && *t.EstimativaTempo > 0 {
		h := float64(*t.EstimativaTempo) / 3600.0
		ta.EstimativaHoras = &h
	}
	return ta
}

func (s *AllocationService) AllocateTask(ctx context.Context, req AllocateTaskRequest) (*AllocateTaskResult, error) {
	if req.EstimateHours <= 0 {
		return nil, fmt.Errorf("estimate_hours must be > 0")
	}

	estimateSeconds := int(req.EstimateHours * 3600)

	if req.AssigneeID != nil && !req.Force {
		capResult, err := s.sprintSvc.GetCapacity(ctx, req.SprintID, &req.EquipeID)
		if err == nil {
			for _, m := range capResult.Membros {
				if m.MembroID == *req.AssigneeID {
					newHours := m.HorasAlocadas + req.EstimateHours
					newPct := 0.0
					if m.HorasDisponiveis > 0 {
						newPct = (newHours / m.HorasDisponiveis) * 100
					}
					if newPct > 100 {
						return &AllocateTaskResult{
							Conflict:   true,
							MembroNome: m.Nome,
							SprintNome: capResult.Sprint.Nome,
							PctAtual:   newPct,
						}, nil
					}
					break
				}
			}
		}
	}

	prev, err := s.repo.GetTaskPreviousState(ctx, req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("getting previous state: %w", err)
	}

	if err := s.repo.UpdateTaskAllocation(ctx, req.TaskID, req.SprintID, req.AssigneeID, estimateSeconds); err != nil {
		return nil, fmt.Errorf("updating allocation: %w", err)
	}

	go s.writeToJira(req.TaskID, req.SprintID, req.AssigneeID, estimateSeconds, prev)

	return &AllocateTaskResult{}, nil
}

func (s *AllocationService) writeToJira(taskID, sprintID uuid.UUID, assigneeID *uuid.UUID, estimateSeconds int, prev *repository.TaskPreviousState) {
	ctx := context.Background()

	issueKey, err := s.repo.GetTaskJiraKey(ctx, taskID)
	if err != nil {
		s.logger.Error("jira write: getting issue key", zap.Error(err))
		s.rollback(ctx, taskID, prev)
		return
	}

	fonteDadosID, err := s.repo.GetTaskFonteDadosID(ctx, taskID)
	if err != nil {
		s.logger.Error("jira write: getting fonte_dados_id", zap.Error(err))
		s.rollback(ctx, taskID, prev)
		return
	}

	client, err := s.buildClient(ctx, fonteDadosID)
	if err != nil {
		s.logger.Error("jira write: building client", zap.Error(err))
		s.rollback(ctx, taskID, prev)
		return
	}

	if err := client.UpdateTimeEstimate(ctx, issueKey, estimateSeconds); err != nil {
		s.logger.Warn("jira write: update estimate failed", zap.String("key", issueKey), zap.Error(err))
		s.rollback(ctx, taskID, prev)
		return
	}

	sprintJiraID, err := s.repo.GetSprintJiraID(ctx, sprintID)
	if err != nil {
		s.logger.Error("jira write: getting sprint jira_id", zap.Error(err))
		s.rollback(ctx, taskID, prev)
		return
	}

	if err := client.MoveToSprint(ctx, sprintJiraID, issueKey); err != nil {
		s.logger.Warn("jira write: move to sprint failed", zap.String("key", issueKey), zap.Error(err))
		s.rollback(ctx, taskID, prev)
		return
	}

	if assigneeID != nil {
		accountID, err := s.sprintRepo.GetMembroJiraAccountID(ctx, *assigneeID)
		if err != nil {
			s.logger.Warn("jira write: getting account id", zap.Error(err))
			return
		}
		if err := client.AssignIssue(ctx, issueKey, accountID); err != nil {
			s.logger.Warn("jira write: assign failed", zap.String("key", issueKey), zap.Error(err))
			return
		}
	}

	s.logger.Info("jira write complete", zap.String("key", issueKey))
}

func (s *AllocationService) rollback(ctx context.Context, taskID uuid.UUID, prev *repository.TaskPreviousState) {
	if err := s.repo.RollbackTaskAllocation(ctx, taskID, prev); err != nil {
		s.logger.Error("rollback failed", zap.String("taskID", taskID.String()), zap.Error(err))
	}
}

func (s *AllocationService) buildClient(ctx context.Context, fonteDadosID uuid.UUID) (jira.Client, error) {
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
				return nil, fmt.Errorf("fonte %s: oauth token expired", fonte.Nome)
			}
			newTokens, err := s.oauthSvc.RefreshAccessToken(ctx, *fonte.OAuth2RefreshToken)
			if err != nil {
				return nil, fmt.Errorf("refreshing oauth token: %w", err)
			}
			expiry := newTokens.Expiry()
			if err := s.fdRepo.SaveOAuthTokens(ctx, fonte.ID, fonte.BaseURL, newTokens.AccessToken, newTokens.RefreshToken, expiry); err != nil {
				return nil, fmt.Errorf("saving refreshed tokens: %w", err)
			}
			accessToken = newTokens.AccessToken
		}
		return s.oauthFactory(fonte.BaseURL, accessToken, s.rateLimit, s.logger), nil
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

func (s *AllocationService) GetAvailableSprints(ctx context.Context, equipeID uuid.UUID) ([]SprintOption, error) {
	rows, err := s.repo.GetFutureSprintsByEquipe(ctx, equipeID)
	if err != nil {
		return nil, err
	}
	result := make([]SprintOption, len(rows))
	for i, r := range rows {
		result[i] = SprintOption{
			ID: r.ID, JiraID: r.JiraID, Nome: r.Nome,
			Inicio: r.Inicio, Fim: r.Fim, Estado: r.Estado,
		}
	}
	return result, nil
}

func (s *AllocationService) SyncProjectTasks(ctx context.Context, epicID uuid.UUID) (int, error) {
	issueKey, err := s.repo.GetTaskJiraKey(ctx, epicID)
	if err != nil {
		return 0, fmt.Errorf("getting epic key: %w", err)
	}
	fonteDadosID, err := s.repo.GetTaskFonteDadosID(ctx, epicID)
	if err != nil {
		return 0, fmt.Errorf("getting fonte_dados_id: %w", err)
	}
	if s.syncSvc == nil {
		return 0, fmt.Errorf("sync service not configured")
	}
	return s.syncSvc.SyncEpicTasks(ctx, fonteDadosID, issueKey)
}
