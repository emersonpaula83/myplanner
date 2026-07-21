package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type SprintGenerationService struct {
	fdRepo             *repository.FonteDadosRepository
	equipeRepo         *repository.EquipeRepository
	syncRepo           *repository.SyncRepository
	sprintRepo         *repository.SprintRepository
	clientFactory      ClientFactory
	oauthClientFactory OAuthClientFactory
	oauthSvc           *jira.OAuthService
	rateLimit          int
	logger             *zap.Logger
}

func NewSprintGenerationService(
	fdRepo *repository.FonteDadosRepository,
	equipeRepo *repository.EquipeRepository,
	syncRepo *repository.SyncRepository,
	sprintRepo *repository.SprintRepository,
	clientFactory ClientFactory,
	oauthClientFactory OAuthClientFactory,
	oauthSvc *jira.OAuthService,
	rateLimit int,
	logger *zap.Logger,
) *SprintGenerationService {
	return &SprintGenerationService{
		fdRepo:             fdRepo,
		equipeRepo:         equipeRepo,
		syncRepo:           syncRepo,
		sprintRepo:         sprintRepo,
		clientFactory:      clientFactory,
		oauthClientFactory: oauthClientFactory,
		oauthSvc:           oauthSvc,
		rateLimit:          rateLimit,
		logger:             logger,
	}
}

type SprintSlot struct {
	Nome       string `json:"nome"`
	DataInicio string `json:"data_inicio"`
	DataFim    string `json:"data_fim"`
}

type PreviewResult struct {
	Sprints             []SprintSlot `json:"sprints"`
	ExistentesIgnoradas int          `json:"existentes_ignoradas"`
}

type GenerateResult struct {
	Criadas int      `json:"criadas"`
	Erros   []string `json:"erros"`
}

func (s *SprintGenerationService) buildClient(ctx context.Context, fonteDadosID uuid.UUID) (jira.Client, error) {
	fonte, err := s.fdRepo.GetByID(ctx, fonteDadosID)
	if err != nil {
		return nil, fmt.Errorf("getting fonte dados: %w", err)
	}
	if fonte == nil {
		return nil, fmt.Errorf("fonte dados %s not found", fonteDadosID)
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
				return nil, fmt.Errorf("saving refreshed tokens for %s: %w", fonte.Nome, err)
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

func (s *SprintGenerationService) getFonteDadosForEquipe(ctx context.Context, equipeID uuid.UUID) (uuid.UUID, error) {
	membros, err := s.equipeRepo.GetMembrosEquipe(ctx, equipeID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("getting membros: %w", err)
	}
	if len(membros) == 0 {
		return uuid.Nil, fmt.Errorf("equipe has no members")
	}
	return membros[0].FonteDadosID, nil
}

func (s *SprintGenerationService) GetBoardsForEquipe(ctx context.Context, equipeID uuid.UUID) ([]jira.JiraBoard, error) {
	fdID, err := s.getFonteDadosForEquipe(ctx, equipeID)
	if err != nil {
		return nil, err
	}

	client, err := s.buildClient(ctx, fdID)
	if err != nil {
		return nil, err
	}

	projetos, err := s.sprintRepo.ListProjetosComSprints(ctx, &equipeID)
	if err != nil {
		return nil, fmt.Errorf("listing projetos for equipe: %w", err)
	}
	if len(projetos) == 0 {
		return []jira.JiraBoard{}, nil
	}

	seen := make(map[int]bool)
	var boards []jira.JiraBoard
	for _, p := range projetos {
		pBoards, err := client.GetBoards(ctx, p.Chave)
		if err != nil {
			s.logger.Warn("failed to get boards for project, agile API scope may be missing", zap.String("project", p.Chave), zap.Error(err))
			continue
		}
		for _, b := range pBoards {
			if !seen[b.ID] {
				seen[b.ID] = true
				boards = append(boards, b)
			}
		}
	}
	if boards == nil {
		boards = []jira.JiraBoard{}
	}
	return boards, nil
}

func (s *SprintGenerationService) PreviewSprints(ctx context.Context, equipeID uuid.UUID, boardID int, prefixo string) (*PreviewResult, error) {
	localSprints, err := s.sprintRepo.ListSprints(ctx, &equipeID, nil)
	if err != nil {
		return nil, fmt.Errorf("listing local sprints: %w", err)
	}

	var existing []jira.JiraSprint
	for _, ls := range localSprints {
		js := jira.JiraSprint{Name: ls.Nome}
		if ls.DataInicio != nil {
			sd := ls.DataInicio.Format(time.RFC3339)
			js.StartDate = &sd
		}
		if ls.DataFim != nil {
			ed := ls.DataFim.Format(time.RFC3339)
			js.EndDate = &ed
		}
		existing = append(existing, js)
	}

	now := time.Now()
	ano := now.Year()
	slots := generateSprintSlots(now, 12, ano)
	missing, ignored := filterExistingSlots(slots, existing)

	result := &PreviewResult{
		Sprints:             make([]SprintSlot, 0, len(missing)),
		ExistentesIgnoradas: ignored,
	}
	for _, slot := range missing {
		result.Sprints = append(result.Sprints, SprintSlot{
			Nome:       formatSprintName(prefixo, slot.start, slot.end, ano),
			DataInicio: slot.start.Format("2006-01-02"),
			DataFim:    slot.end.Format("2006-01-02"),
		})
	}
	return result, nil
}

func (s *SprintGenerationService) GenerateSprints(ctx context.Context, equipeID uuid.UUID, boardID int, prefixo string) (*GenerateResult, error) {
	fdID, err := s.getFonteDadosForEquipe(ctx, equipeID)
	if err != nil {
		return nil, err
	}

	client, err := s.buildClient(ctx, fdID)
	if err != nil {
		return nil, err
	}

	localSprints, err := s.sprintRepo.ListSprints(ctx, &equipeID, nil)
	if err != nil {
		return nil, fmt.Errorf("listing local sprints: %w", err)
	}
	var existing []jira.JiraSprint
	for _, ls := range localSprints {
		js := jira.JiraSprint{Name: ls.Nome}
		if ls.DataInicio != nil {
			sd := ls.DataInicio.Format(time.RFC3339)
			js.StartDate = &sd
		}
		if ls.DataFim != nil {
			ed := ls.DataFim.Format(time.RFC3339)
			js.EndDate = &ed
		}
		existing = append(existing, js)
	}

	now := time.Now()
	ano := now.Year()
	slots := generateSprintSlots(now, 12, ano)
	missing, _ := filterExistingSlots(slots, existing)

	result := &GenerateResult{Erros: make([]string, 0)}
	for _, slot := range missing {
		name := formatSprintName(prefixo, slot.start, slot.end, ano)
		startDate := time.Date(slot.start.Year(), slot.start.Month(), slot.start.Day(), 0, 0, 0, 0, time.UTC)
		endDate := time.Date(slot.end.Year(), slot.end.Month(), slot.end.Day(), 18, 0, 0, 0, time.UTC)

		created, err := client.CreateSprint(ctx, boardID, name, startDate, endDate)
		if err != nil {
			result.Erros = append(result.Erros, fmt.Sprintf("%s: %v", name, err))
			s.logger.Warn("failed to create sprint", zap.String("name", name), zap.Error(err))
			continue
		}

		_, upsertErr := s.syncRepo.UpsertSprint(ctx, fdID, created.ID, name, nil, &startDate, &endDate, nil, &boardID, nil)
		if upsertErr != nil {
			s.logger.Warn("sprint created in jira but failed local upsert", zap.String("name", name), zap.Error(upsertErr))
		}
		result.Criadas++
	}
	return result, nil
}

type sprintSlot struct {
	start time.Time
	end   time.Time
}

var saoPaulo *time.Location

func init() {
	var err error
	saoPaulo, err = time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		panic("failed to load America/Sao_Paulo timezone: " + err.Error())
	}
}

func generateSprintSlots(startDate time.Time, durationDays int, year int) []sprintSlot {
	start := nextMonday(startDate)
	yearEnd := time.Date(year, 12, 31, 23, 59, 59, 0, saoPaulo)

	var slots []sprintSlot
	for !start.After(yearEnd) {
		end := start.AddDate(0, 0, durationDays-1)
		if end.After(yearEnd) {
			break
		}
		slotStart := time.Date(start.Year(), start.Month(), start.Day(), 8, 30, 0, 0, saoPaulo)
		slotEnd := time.Date(end.Year(), end.Month(), end.Day(), 18, 30, 0, 0, saoPaulo)
		slots = append(slots, sprintSlot{start: slotStart, end: slotEnd})
		start = end.AddDate(0, 0, 3)
	}
	return slots
}

func nextMonday(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, saoPaulo)
	wd := d.Weekday()
	if wd == time.Monday {
		return d
	}
	daysUntilMonday := (8 - int(wd)) % 7
	if daysUntilMonday == 0 {
		daysUntilMonday = 7
	}
	return d.AddDate(0, 0, daysUntilMonday)
}

func filterExistingSlots(slots []sprintSlot, existing []jira.JiraSprint) ([]sprintSlot, int) {
	ignored := 0
	var missing []sprintSlot
	for _, slot := range slots {
		overlaps := false
		for _, ex := range existing {
			exStart := parseOptionalDate(ex.StartDate)
			exEnd := parseOptionalDate(ex.EndDate)
			if exStart == nil || exEnd == nil {
				continue
			}
			if slot.start.Before(*exEnd) && slot.end.After(*exStart) {
				overlaps = true
				break
			}
		}
		if overlaps {
			ignored++
		} else {
			missing = append(missing, slot)
		}
	}
	return missing, ignored
}

func formatSprintName(prefixo string, start, end time.Time, ano int) string {
	return fmt.Sprintf("%s %02d/%02d - %02d/%02d [%d]",
		prefixo,
		start.Day(), start.Month(),
		end.Day(), end.Month(),
		ano)
}

var sprintNamePattern = regexp.MustCompile(`^(.+?)\s*\d{2}/\d{2}\s*-\s*\d{2}/\d{2}`)

func detectSprintPattern(sprints []jira.JiraSprint) (string, int, error) {
	if len(sprints) == 0 {
		return "", 0, fmt.Errorf("nenhuma sprint encontrada no board para detectar padrão")
	}

	prefixCounts := make(map[string]int)
	durationCounts := make(map[int]int)

	for _, s := range sprints {
		matches := sprintNamePattern.FindStringSubmatch(s.Name)
		if matches != nil {
			prefix := strings.TrimSpace(matches[1])
			prefixCounts[prefix]++
		}

		start := parseJiraDate(s.StartDate)
		end := parseJiraDate(s.EndDate)
		if start != nil && end != nil {
			days := int(end.Sub(*start).Hours()/24) + 1
			if days > 0 {
				durationCounts[days]++
			}
		}
	}

	if len(prefixCounts) == 0 {
		return "", 0, fmt.Errorf("não foi possível detectar prefixo das sprints do board")
	}

	prefix := modeString(prefixCounts)
	duration := 12
	if len(durationCounts) > 0 {
		duration = modeInt(durationCounts)
	}

	return prefix, duration, nil
}

func parseJiraDate(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02",
	} {
		t, err := time.Parse(layout, *s)
		if err == nil {
			return &t
		}
	}
	return nil
}

func modeString(counts map[string]int) string {
	var best string
	var bestCount int
	for k, v := range counts {
		if v > bestCount {
			best = k
			bestCount = v
		}
	}
	return best
}

func modeInt(counts map[int]int) int {
	var best, bestCount int
	for k, v := range counts {
		if v > bestCount {
			best = k
			bestCount = v
		}
	}
	return best
}
