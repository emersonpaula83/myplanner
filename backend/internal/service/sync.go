package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

var ErrSyncAlreadyRunning = errors.New("projeto já em sincronização")

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
	running, err := s.repo.HasRunningSync(ctx, fonteDadosID)
	if err != nil {
		return nil, err
	}
	if running {
		return nil, ErrSyncAlreadyRunning
	}

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
	running, err := s.repo.HasRunningSync(ctx, fonte.ID)
	if err != nil {
		return nil, err
	}
	if running {
		return nil, ErrSyncAlreadyRunning
	}

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

	if sprintFieldID, err := client.GetSprintFieldID(ctx); err == nil {
		client.SetSprintFieldID(sprintFieldID)
		s.logger.Info("sprint field discovered", zap.String("fieldID", sprintFieldID))
	} else {
		s.logger.Debug("could not discover sprint field", zap.Error(err))
	}

	projectKeys, err := s.repo.GetProjectKeysForSync(ctx, fonte.ID)
	if err != nil {
		return totals, []error{fmt.Errorf("getting project keys: %w", err)}
	}

	if len(projectKeys) == 0 {
		s.logger.Info("no equipe-linked projects found, falling back to full project list")
		projects, err := client.GetProjects(ctx)
		if err != nil {
			return totals, []error{fmt.Errorf("fetching projects: %w", err)}
		}
		for _, p := range projects {
			projectKeys = append(projectKeys, p.Key)
		}
	}

	s.logger.Info("syncing projects", zap.Int("count", len(projectKeys)), zap.Strings("keys", projectKeys))

	issues, err := client.GetIssuesByProjects(ctx, projectKeys, nil)
	if err != nil {
		return totals, []error{fmt.Errorf("fetching issues: %w", err)}
	}

	memberCache := make(map[string]uuid.UUID)
	sprintCache := make(map[int]uuid.UUID)
	projectCache := make(map[string]uuid.UUID)
	var allPendingParents []parentRef

	for i, issue := range issues {
		projetoID, err := s.ensureProject(ctx, fonte, issue, projectCache)
		if err != nil {
			syncErrors = append(syncErrors, err)
			continue
		}

		s.ensureMember(ctx, fonte, issue.Fields.Assignee, issue.Fields.Project.Name, memberCache, &totals)
		s.ensureMember(ctx, fonte, issue.Fields.Reporter, issue.Fields.Project.Name, memberCache, &totals)

		tarefaID, err := s.processIssue(ctx, fonte, projetoID, issue, memberCache, sprintCache)
		if err != nil {
			syncErrors = append(syncErrors, err)
			continue
		}

		if issue.Fields.Parent != nil && issue.Fields.Parent.ID != "" {
			allPendingParents = append(allPendingParents, parentRef{
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

		if (i+1)%50 == 0 {
			s.logger.Info("sync progress", zap.Int("processed", i+1), zap.Int("total", len(issues)))
		}
	}

	totals.Projetos = len(projectCache)
	totals.Sprints = len(sprintCache)
	syncErrors = append(syncErrors, s.resolveParents(ctx, fonte, allPendingParents)...)
	return totals, syncErrors
}

func (s *SyncService) ensureProject(ctx context.Context, fonte *domain.FonteDados, issue jira.JiraIssue, cache map[string]uuid.UUID) (uuid.UUID, error) {
	jp := issue.Fields.Project
	if id, ok := cache[jp.ID]; ok {
		return id, nil
	}
	var leadID *uuid.UUID
	var categoria *string
	projetoID, err := s.repo.UpsertProjeto(ctx, fonte.ID, jp.ID, jp.Key, jp.Name, nil, leadID, categoria)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upserting project %s: %w", jp.Key, err)
	}
	cache[jp.ID] = projetoID
	return projetoID, nil
}

func (s *SyncService) ensureMember(ctx context.Context, fonte *domain.FonteDados, user *jira.JiraUser, teamName string, cache map[string]uuid.UUID, totals *repository.SyncTotals) {
	if user == nil || user.AccountID == "" {
		return
	}
	if _, ok := cache[user.AccountID]; ok {
		return
	}
	var emailPtr *string
	if user.EmailAddress != "" {
		emailPtr = &user.EmailAddress
	}
	var avatarPtr *string
	if user.AvatarUrls.Small != "" {
		avatarPtr = &user.AvatarUrls.Small
	}
	id, err := s.repo.UpsertMembro(ctx, fonte.ID, user.AccountID, user.DisplayName, emailPtr, avatarPtr, &teamName)
	if err != nil {
		s.logger.Warn("failed to upsert member from issue", zap.String("accountID", user.AccountID), zap.Error(err))
		return
	}
	cache[user.AccountID] = id
	totals.Membros++
}

func (s *SyncService) executSyncProject(ctx context.Context, client jira.Client, fonte *domain.FonteDados, projectKey string, syncLogID *uuid.UUID) (repository.SyncTotals, []error) {
	if sprintFieldID, err := client.GetSprintFieldID(ctx); err == nil {
		client.SetSprintFieldID(sprintFieldID)
	}

	issues, err := client.GetIssuesByProjects(ctx, []string{projectKey}, nil)
	if err != nil {
		return repository.SyncTotals{}, []error{fmt.Errorf("fetching issues for %s: %w", projectKey, err)}
	}

	var totals repository.SyncTotals
	var syncErrors []error
	memberCache := make(map[string]uuid.UUID)
	sprintCache := make(map[int]uuid.UUID)
	projectCache := make(map[string]uuid.UUID)
	var pendingParents []parentRef

	for i, issue := range issues {
		projetoID, err := s.ensureProject(ctx, fonte, issue, projectCache)
		if err != nil {
			syncErrors = append(syncErrors, err)
			continue
		}

		s.ensureMember(ctx, fonte, issue.Fields.Assignee, issue.Fields.Project.Name, memberCache, &totals)
		s.ensureMember(ctx, fonte, issue.Fields.Reporter, issue.Fields.Project.Name, memberCache, &totals)

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

	totals.Projetos = len(projectCache)
	totals.Sprints = len(sprintCache)
	s.flushProgress(ctx, syncLogID, totals)
	syncErrors = append(syncErrors, s.resolveParents(ctx, fonte, pendingParents)...)
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
		} else {
			var estado *string
			if f.Sprint.State != "" {
				estado = &f.Sprint.State
			}
			var boardID *int
			if f.Sprint.OriginBoardID > 0 {
				bid := f.Sprint.OriginBoardID
				boardID = &bid
			}
			spID, err := s.repo.UpsertSprint(ctx, fonte.ID, f.Sprint.ID, f.Sprint.Name, estado,
				parseOptionalTime(f.Sprint.StartDate), parseOptionalTime(f.Sprint.EndDate),
				parseOptionalTime(f.Sprint.CompleteDate), boardID, &projetoID)
			if err == nil {
				sprintCache[f.Sprint.ID] = spID
				sprintID = &spID
			}
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

	var dataEntradaSprint *time.Time
	if sprintID != nil && f.Sprint != nil {
		if issue.Changelog != nil {
			dataEntradaSprint = extractSprintEntryDate(issue.Changelog, f.Sprint.Name)
		}
		if dataEntradaSprint == nil {
			dataEntradaSprint = &dataCriacao
		}
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
		DataEntradaSprint:  dataEntradaSprint,
	}

	return s.repo.UpsertTarefa(ctx, params)
}

var inProgressStatuses = map[string]bool{
	"In Progress":      true,
	"Em Andamento":     true,
	"Desenvolvimento":  true,
	"Em Desenvolvimento": true,
}

func extractSprintEntryDate(cl *jira.JiraChangelog, sprintName string) *time.Time {
	var latest *time.Time
	for _, h := range cl.Histories {
		for _, item := range h.Items {
			if item.Field != "Sprint" {
				continue
			}
			if !strings.Contains(item.ToString, sprintName) {
				continue
			}
			t := parseOptionalJiraTime(h.Created)
			if t != nil && (latest == nil || t.After(*latest)) {
				latest = t
			}
		}
	}
	return latest
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
