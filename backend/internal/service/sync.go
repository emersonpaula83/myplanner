package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"github.com/totvs/tcloud-planner/backend/internal/jira"
	"github.com/totvs/tcloud-planner/backend/internal/repository"
	"go.uber.org/zap"
)

type ClientFactory func(baseURL, email, apiToken string, rateLimit int, logger *zap.Logger) jira.Client
type OAuthClientFactory func(baseURL, accessToken string, rateLimit int, logger *zap.Logger) jira.Client

// SyncStore is the interface consumed by the sync HTTP handler.
type SyncStore interface {
	SyncFonteDados(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error)
	SyncAll(ctx context.Context) ([]domain.SyncLog, error)
	SyncProject(ctx context.Context, fonteDadosID uuid.UUID, projectKey string) (*domain.SyncLog, error)
	ListJiraProjects(ctx context.Context, fonteDadosID uuid.UUID) ([]domain.JiraProjectInfo, error)
	GetStatus(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error)
	ListLogs(ctx context.Context, fonteDadosID uuid.UUID, limit int) ([]domain.SyncLog, error)
}

type SyncService struct {
	repo               *repository.SyncRepository
	fdRepo             *repository.FonteDadosRepository
	clientFactory      ClientFactory
	oauthClientFactory OAuthClientFactory
	oauthSvc           *jira.OAuthService
	rateLimit          int
	logger             *zap.Logger
}

func NewSyncService(repo *repository.SyncRepository, fdRepo *repository.FonteDadosRepository, clientFactory ClientFactory, oauthClientFactory OAuthClientFactory, oauthSvc *jira.OAuthService, rateLimit int, logger *zap.Logger) *SyncService {
	return &SyncService{
		repo:               repo,
		fdRepo:             fdRepo,
		clientFactory:      clientFactory,
		oauthClientFactory: oauthClientFactory,
		oauthSvc:           oauthSvc,
		rateLimit:          rateLimit,
		logger:             logger,
	}
}

func (s *SyncService) buildClient(ctx context.Context, fonte *domain.FonteDados) (jira.Client, error) {
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
				return nil, fmt.Errorf("saving refreshed tokens for %s: %w", fonte.Nome, err)
			}
			accessToken = newTokens.AccessToken
			s.logger.Info("oauth token refreshed", zap.String("fonte", fonte.Nome))
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

func (s *SyncService) getFonte(ctx context.Context, fonteDadosID uuid.UUID) (*domain.FonteDados, error) {
	fonte, err := s.fdRepo.GetByID(ctx, fonteDadosID)
	if err != nil {
		return nil, fmt.Errorf("getting fonte dados: %w", err)
	}
	if fonte == nil {
		return nil, fmt.Errorf("fonte dados %s not found", fonteDadosID)
	}
	return fonte, nil
}

func (s *SyncService) ListJiraProjects(ctx context.Context, fonteDadosID uuid.UUID) ([]domain.JiraProjectInfo, error) {
	fonte, err := s.getFonte(ctx, fonteDadosID)
	if err != nil {
		return nil, err
	}
	client, err := s.buildClient(ctx, fonte)
	if err != nil {
		return nil, err
	}
	projects, err := client.GetProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching projects: %w", err)
	}
	result := make([]domain.JiraProjectInfo, len(projects))
	for i, p := range projects {
		result[i] = domain.JiraProjectInfo{ID: p.ID, Key: p.Key, Name: p.Name}
	}
	return result, nil
}

func (s *SyncService) SyncProject(ctx context.Context, fonteDadosID uuid.UUID, projectKey string) (*domain.SyncLog, error) {
	fonte, err := s.getFonte(ctx, fonteDadosID)
	if err != nil {
		return nil, err
	}
	client, err := s.buildClient(ctx, fonte)
	if err != nil {
		return nil, err
	}

	syncLog := &domain.SyncLog{
		ID:           uuid.New(),
		FonteDadosID: fonte.ID,
		Tipo:         "project",
		Status:       "running",
		IniciadoEm:   time.Now(),
	}
	if err := s.repo.CreateSyncLog(ctx, syncLog); err != nil {
		return nil, fmt.Errorf("creating sync log: %w", err)
	}

	go s.runProjectSync(client, fonte, projectKey, syncLog.ID)

	return syncLog, nil
}

func (s *SyncService) runProjectSync(client jira.Client, fonte *domain.FonteDados, projectKey string, syncLogID uuid.UUID) {
	ctx := context.Background()

	totals, syncErrors := s.executSyncProject(ctx, client, fonte, projectKey, &syncLogID)

	now := time.Now()
	status := "success"
	var mensagem *string
	var errosJSON json.RawMessage

	if len(syncErrors) > 0 {
		status = "partial"
		msg := fmt.Sprintf("%d errors occurred", len(syncErrors))
		mensagem = &msg
		errStrs := make([]string, 0, len(syncErrors))
		for _, e := range syncErrors {
			errStrs = append(errStrs, e.Error())
		}
		errosJSON, _ = json.Marshal(errStrs)
	}

	if err := s.repo.UpdateSyncLog(ctx, syncLogID, status, now, totals, errosJSON, mensagem); err != nil {
		s.logger.Error("failed to update sync log", zap.Error(err))
	}

	if err := s.fdRepo.UpdateUltimoSync(ctx, fonte.ID, now); err != nil {
		s.logger.Error("failed to update ultimo_sync", zap.Error(err))
	}

	s.logger.Info("project sync completed",
		zap.String("fonte", fonte.Nome),
		zap.String("project", projectKey),
		zap.String("status", status),
		zap.Int("tarefas", totals.Tarefas),
		zap.Int("membros", totals.Membros),
		zap.Int("sprints", totals.Sprints),
	)
}

// SyncFonteDados performs a full sync of a single data source.
func (s *SyncService) SyncFonteDados(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error) {
	fonte, err := s.getFonte(ctx, fonteDadosID)
	if err != nil {
		return nil, err
	}
	return s.syncOne(ctx, fonte)
}

// SyncAll syncs every active fonte de dados.
func (s *SyncService) SyncAll(ctx context.Context) ([]domain.SyncLog, error) {
	fontes, err := s.repo.GetFonteDadosAtivas(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting active fontes: %w", err)
	}

	logs := make([]domain.SyncLog, 0, len(fontes))
	for i := range fontes {
		log, err := s.syncOne(ctx, &fontes[i])
		if err != nil {
			s.logger.Error("sync failed for fonte", zap.String("nome", fontes[i].Nome), zap.Error(err))
			continue
		}
		logs = append(logs, *log)
	}
	return logs, nil
}

// GetStatus returns the latest sync log for a fonte de dados.
func (s *SyncService) GetStatus(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error) {
	return s.repo.GetLatestSyncLog(ctx, fonteDadosID)
}

// ListLogs returns the sync history for a fonte de dados.
func (s *SyncService) ListLogs(ctx context.Context, fonteDadosID uuid.UUID, limit int) ([]domain.SyncLog, error) {
	return s.repo.ListSyncLogs(ctx, fonteDadosID, limit)
}

func (s *SyncService) syncOne(ctx context.Context, fonte *domain.FonteDados) (*domain.SyncLog, error) {
	client, err := s.buildClient(ctx, fonte)
	if err != nil {
		return nil, err
	}

	syncLog := &domain.SyncLog{
		ID:           uuid.New(),
		FonteDadosID: fonte.ID,
		Tipo:         "full",
		Status:       "running",
		IniciadoEm:   time.Now(),
	}
	if err := s.repo.CreateSyncLog(ctx, syncLog); err != nil {
		return nil, fmt.Errorf("creating sync log: %w", err)
	}

	totals, syncErrors := s.executSync(ctx, client, fonte)

	now := time.Now()
	status := "success"
	var mensagem *string
	var errosJSON json.RawMessage

	if len(syncErrors) > 0 {
		status = "partial"
		msg := fmt.Sprintf("%d errors occurred", len(syncErrors))
		mensagem = &msg
		errStrs := make([]string, 0, len(syncErrors))
		for _, e := range syncErrors {
			errStrs = append(errStrs, e.Error())
		}
		errosJSON, _ = json.Marshal(errStrs)
	}

	if err := s.repo.UpdateSyncLog(ctx, syncLog.ID, status, now, totals, errosJSON, mensagem); err != nil {
		s.logger.Error("failed to update sync log", zap.Error(err))
	}

	if err := s.fdRepo.UpdateUltimoSync(ctx, fonte.ID, now); err != nil {
		s.logger.Error("failed to update ultimo_sync", zap.Error(err))
	}

	syncLog.Status = status
	syncLog.FinalizadoEm = &now
	syncLog.TotalProjetos = totals.Projetos
	syncLog.TotalTarefas = totals.Tarefas
	syncLog.TotalMembros = totals.Membros
	syncLog.TotalSprints = totals.Sprints
	syncLog.Mensagem = mensagem

	s.logger.Info("sync completed",
		zap.String("fonte", fonte.Nome),
		zap.String("status", status),
		zap.Int("projetos", totals.Projetos),
		zap.Int("tarefas", totals.Tarefas),
		zap.Int("membros", totals.Membros),
		zap.Int("sprints", totals.Sprints),
	)

	return syncLog, nil
}

type parentRef struct {
	tarefaID     uuid.UUID
	parentJiraID string
}

func (s *SyncService) flushProgress(ctx context.Context, syncLogID *uuid.UUID, totals repository.SyncTotals) {
	if syncLogID == nil {
		return
	}
	if err := s.repo.UpdateSyncLogTotals(ctx, *syncLogID, totals); err != nil {
		s.logger.Warn("failed to flush sync progress", zap.Error(err))
	}
}

func (s *SyncService) syncProjectData(ctx context.Context, client jira.Client, fonte *domain.FonteDados, jp jira.JiraProject, memberCache map[string]uuid.UUID, sprintCache map[int]uuid.UUID, syncLogID *uuid.UUID) (repository.SyncTotals, []parentRef, []error) {
	var totals repository.SyncTotals
	var syncErrors []error
	var pendingParents []parentRef
	teamName := jp.Name

	users, err := client.GetUsers(ctx, jp.Key)
	if err != nil {
		syncErrors = append(syncErrors, fmt.Errorf("fetching users for %s: %w", jp.Key, err))
	} else {
		for _, u := range users {
			if _, cached := memberCache[u.AccountID]; cached {
				continue
			}
			var emailPtr *string
			if u.EmailAddress != "" {
				emailPtr = &u.EmailAddress
			}
			var avatarPtr *string
			if u.AvatarUrls.Small != "" {
				avatarPtr = &u.AvatarUrls.Small
			}
			id, err := s.repo.UpsertMembro(ctx, fonte.ID, u.AccountID, u.DisplayName, emailPtr, avatarPtr, &teamName)
			if err != nil {
				syncErrors = append(syncErrors, err)
				continue
			}
			memberCache[u.AccountID] = id
			totals.Membros++
		}
		s.flushProgress(ctx, syncLogID, totals)
	}

	var leadID *uuid.UUID
	if jp.Lead != nil {
		if lid, ok := memberCache[jp.Lead.AccountID]; ok {
			leadID = &lid
		}
	}
	var categoria *string
	if jp.ProjectCategory != nil && jp.ProjectCategory.Name != "" {
		categoria = &jp.ProjectCategory.Name
	}
	var descricao *string
	if jp.Description != "" {
		descricao = &jp.Description
	}

	projetoID, err := s.repo.UpsertProjeto(ctx, fonte.ID, jp.ID, jp.Key, jp.Name, descricao, leadID, categoria)
	if err != nil {
		return totals, nil, append(syncErrors, err)
	}
	totals.Projetos++
	s.flushProgress(ctx, syncLogID, totals)

	boards, err := client.GetBoards(ctx, jp.Key)
	if err != nil {
		syncErrors = append(syncErrors, fmt.Errorf("fetching boards for %s: %w", jp.Key, err))
	} else {
		for _, b := range boards {
			sprints, err := client.GetBoardSprints(ctx, b.ID)
			if err != nil {
				syncErrors = append(syncErrors, fmt.Errorf("fetching sprints for board %d: %w", b.ID, err))
				continue
			}
			for _, sp := range sprints {
				if _, cached := sprintCache[sp.ID]; cached {
					continue
				}
				var estado *string
				if sp.State != "" {
					estado = &sp.State
				}
				boardID := b.ID
				spID, err := s.repo.UpsertSprint(ctx, fonte.ID, sp.ID, sp.Name, estado,
					parseOptionalTime(sp.StartDate), parseOptionalTime(sp.EndDate),
					parseOptionalTime(sp.CompleteDate), &boardID, &projetoID)
				if err != nil {
					syncErrors = append(syncErrors, err)
					continue
				}
				sprintCache[sp.ID] = spID
				totals.Sprints++
			}
		}
		s.flushProgress(ctx, syncLogID, totals)
	}

	issues, err := client.GetProjectIssues(ctx, jp.Key, fonte.UltimoSync)
	if err != nil {
		syncErrors = append(syncErrors, fmt.Errorf("fetching issues for %s: %w", jp.Key, err))
		return totals, pendingParents, syncErrors
	}

	for i, issue := range issues {
		tarefaID, err := s.processIssue(ctx, fonte, projetoID, issue, memberCache, sprintCache)
		if err != nil {
			syncErrors = append(syncErrors, err)
			continue
		}

		if issue.Fields.Parent != nil && issue.Fields.Parent.ID != "" {
			pendingParents = append(pendingParents, parentRef{
				tarefaID:     tarefaID,
				parentJiraID: issue.Fields.Parent.ID,
			})
		}

		for _, comp := range issue.Fields.Components {
			prodID, err := s.repo.UpsertProduto(ctx, fonte.ID, comp.ID, comp.Name, nil, &projetoID)
			if err != nil {
				syncErrors = append(syncErrors, err)
				continue
			}
			if err := s.repo.LinkTarefaProduto(ctx, tarefaID, prodID); err != nil {
				syncErrors = append(syncErrors, err)
			}
		}
		totals.Tarefas++

		if (i+1)%20 == 0 {
			s.flushProgress(ctx, syncLogID, totals)
		}
	}
	s.flushProgress(ctx, syncLogID, totals)

	return totals, pendingParents, syncErrors
}

func (s *SyncService) resolveParents(ctx context.Context, fonte *domain.FonteDados, pendingParents []parentRef) []error {
	var errs []error
	for _, pp := range pendingParents {
		parentID, err := s.repo.LookupTarefaIDByJiraID(ctx, fonte.ID, pp.parentJiraID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if parentID == uuid.Nil {
			continue
		}
		if err := s.repo.UpdateTarefaParent(ctx, pp.tarefaID, parentID); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func (s *SyncService) executSync(ctx context.Context, client jira.Client, fonte *domain.FonteDados) (repository.SyncTotals, []error) {
	var totals repository.SyncTotals
	var syncErrors []error

	projects, err := client.GetProjects(ctx)
	if err != nil {
		return totals, []error{fmt.Errorf("fetching projects: %w", err)}
	}

	memberCache := make(map[string]uuid.UUID)
	sprintCache := make(map[int]uuid.UUID)
	var allPendingParents []parentRef

	for _, jp := range projects {
		pTotals, parents, pErrors := s.syncProjectData(ctx, client, fonte, jp, memberCache, sprintCache, nil)
		totals.Projetos += pTotals.Projetos
		totals.Tarefas += pTotals.Tarefas
		totals.Membros += pTotals.Membros
		totals.Sprints += pTotals.Sprints
		syncErrors = append(syncErrors, pErrors...)
		allPendingParents = append(allPendingParents, parents...)
	}

	syncErrors = append(syncErrors, s.resolveParents(ctx, fonte, allPendingParents)...)
	return totals, syncErrors
}

func (s *SyncService) executSyncProject(ctx context.Context, client jira.Client, fonte *domain.FonteDados, projectKey string, syncLogID *uuid.UUID) (repository.SyncTotals, []error) {
	projects, err := client.GetProjects(ctx)
	if err != nil {
		return repository.SyncTotals{}, []error{fmt.Errorf("fetching projects: %w", err)}
	}

	var target *jira.JiraProject
	for i := range projects {
		if projects[i].Key == projectKey {
			target = &projects[i]
			break
		}
	}
	if target == nil {
		return repository.SyncTotals{}, []error{fmt.Errorf("project %s not found in JIRA", projectKey)}
	}

	memberCache := make(map[string]uuid.UUID)
	sprintCache := make(map[int]uuid.UUID)

	totals, parents, syncErrors := s.syncProjectData(ctx, client, fonte, *target, memberCache, sprintCache, syncLogID)
	syncErrors = append(syncErrors, s.resolveParents(ctx, fonte, parents)...)
	return totals, syncErrors
}

func (s *SyncService) processIssue(ctx context.Context, fonte *domain.FonteDados, projetoID uuid.UUID, issue jira.JiraIssue, memberCache map[string]uuid.UUID, sprintCache map[int]uuid.UUID) (uuid.UUID, error) {
	f := issue.Fields

	var responsavelID, relatorID *uuid.UUID
	if f.Assignee != nil {
		if id, ok := memberCache[f.Assignee.AccountID]; ok {
			responsavelID = &id
		}
	}
	if f.Reporter != nil {
		if id, ok := memberCache[f.Reporter.AccountID]; ok {
			relatorID = &id
		}
	}

	var sprintID *uuid.UUID
	if f.Sprint != nil {
		if id, ok := sprintCache[f.Sprint.ID]; ok {
			sprintID = &id
		}
	}

	var estimativaTempo, tempoGasto *int
	if f.TimeTracking != nil {
		if f.TimeTracking.OriginalEstimateSeconds > 0 {
			v := f.TimeTracking.OriginalEstimateSeconds
			estimativaTempo = &v
		}
		if f.TimeTracking.TimeSpentSeconds > 0 {
			v := f.TimeTracking.TimeSpentSeconds
			tempoGasto = &v
		}
	}

	dataCriacao := parseJiraTime(f.Created)
	dataAtualizado := parseOptionalJiraTime(f.Updated)
	dataResolvido := parseOptionalJiraTimePtr(f.ResolutionDate)
	dataLimite := timeToPgDate(parseOptionalDate(f.DueDate))

	statusCat := f.Status.StatusCategory.Key

	var tipoDemanda *string
	if fonte.CustomFieldMap != nil {
		var cfMap map[string]string
		if err := json.Unmarshal(fonte.CustomFieldMap, &cfMap); err == nil {
			for fieldID, fieldName := range cfMap {
				if fieldName == "tipo_demanda" {
					if val, ok := f.CustomFields[fieldID]; ok {
						if m, ok := val.(map[string]any); ok {
							if v, ok := m["value"].(string); ok {
								tipoDemanda = &v
							}
						} else if v, ok := val.(string); ok {
							tipoDemanda = &v
						}
					}
				}
			}
		}
	}

	var dataInicioExecucao *time.Time
	if issue.Changelog != nil {
		dataInicioExecucao = extractFirstInProgressDate(issue.Changelog)
	}

	params := &repository.UpsertTarefaParams{
		FonteDadosID:       fonte.ID,
		ProjetoID:          projetoID,
		JiraID:             issue.ID,
		NumeroTicket:       issue.Key,
		Resumo:             f.Summary,
		Tipo:               f.IssueType.Name,
		Status:             f.Status.Name,
		Prioridade:         nilIfEmpty(f.Priority),
		EstimativaPontos:   f.StoryPoints,
		EstimativaTempo:    estimativaTempo,
		TempoGasto:         tempoGasto,
		ResponsavelID:      responsavelID,
		RelatorID:          relatorID,
		SprintID:           sprintID,
		DataCriacao:        dataCriacao,
		DataLimite:         dataLimite,
		DataResolvido:      dataResolvido,
		DataAtualizado:     dataAtualizado,
		TipoDemanda:        tipoDemanda,
		StatusCategoria:    &statusCat,
		CamposExtras:       json.RawMessage(`{}`),
		DataInicioExecucao: dataInicioExecucao,
	}

	return s.repo.UpsertTarefa(ctx, params)
}

var inProgressStatuses = map[string]bool{
	"In Progress":      true,
	"Em Andamento":     true,
	"Desenvolvimento":  true,
	"Em Desenvolvimento": true,
}

func extractFirstInProgressDate(cl *jira.JiraChangelog) *time.Time {
	var earliest *time.Time
	for _, h := range cl.Histories {
		for _, item := range h.Items {
			if item.Field != "status" {
				continue
			}
			if !inProgressStatuses[item.ToString] {
				continue
			}
			t := parseOptionalJiraTime(h.Created)
			if t != nil && (earliest == nil || t.Before(*earliest)) {
				earliest = t
			}
		}
	}
	return earliest
}

func nilIfEmpty(p *jira.JiraPrio) *string {
	if p == nil || p.Name == "" {
		return nil
	}
	return &p.Name
}

func parseJiraTime(s string) time.Time {
	for _, layout := range []string{
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05.000Z",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Now()
}

func parseOptionalJiraTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t := parseJiraTime(s)
	return &t
}

func parseOptionalJiraTimePtr(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	return parseOptionalJiraTime(*s)
}

func parseOptionalTime(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	return parseOptionalJiraTime(*s)
}

func parseOptionalDate(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", *s)
	if err != nil {
		return nil
	}
	return &t
}

// timeToPgDate converts a *time.Time into a *pgtype.Date suitable for the
// DATE columns used by tarefas (data_limite, data_componente).
func timeToPgDate(t *time.Time) *pgtype.Date {
	if t == nil {
		return nil
	}
	d := &pgtype.Date{}
	d.Time = *t
	d.Valid = true
	return d
}
