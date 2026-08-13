package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

var sheetsIDRegex = regexp.MustCompile(`/spreadsheets/d/([a-zA-Z0-9-_]+)`)
var sheetsGidRegex = regexp.MustCompile(`[#&?]gid=(\d+)`)

func ExtractSheetsIDAndGid(sheetsURL string) (id string, gid string, err error) {
	m := sheetsIDRegex.FindStringSubmatch(sheetsURL)
	if len(m) < 2 {
		return "", "", fmt.Errorf("URL do Google Sheets inválida")
	}
	id = m[1]
	gid = "0"
	if gm := sheetsGidRegex.FindStringSubmatch(sheetsURL); len(gm) >= 2 {
		gid = gm[1]
	}
	return id, gid, nil
}

type ImportService struct {
	membroRepo *repository.MembroRepository
	equipeRepo *repository.EquipeRepository
	httpClient *http.Client
	logger     *zap.Logger
}

func NewImportService(membroRepo *repository.MembroRepository, equipeRepo *repository.EquipeRepository, logger *zap.Logger) *ImportService {
	return &ImportService{
		membroRepo: membroRepo,
		equipeRepo: equipeRepo,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
	}
}

func (s *ImportService) MatchPlanilha(ctx context.Context, csvContent string) (*domain.ImportMatchResult, error) {
	parsed, err := ParseCSVPlanilha(csvContent)
	if err != nil {
		return nil, fmt.Errorf("parsing csv: %w", err)
	}

	membros, err := s.membroRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing membros: %w", err)
	}
	equipes, err := s.equipeRepo.ListEquipes(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing equipes: %w", err)
	}

	return MatchLinhas(parsed.Linhas, parsed.Ignorados, membros, equipes), nil
}

func (s *ImportService) FetchGoogleSheetCSV(ctx context.Context, sheetsURL string) (csvContent string, id string, gid string, err error) {
	id, gid, err = ExtractSheetsIDAndGid(sheetsURL)
	if err != nil {
		return "", "", "", err
	}
	exportURL := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/gviz/tq?tqx=out:csv&gid=%s", id, gid)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, exportURL, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("creating request: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("fetching planilha: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("reading response: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if resp.StatusCode != http.StatusOK || !strings.Contains(contentType, "csv") {
		return "", "", "", fmt.Errorf("planilha não está pública. Configure o compartilhamento como \"qualquer pessoa com o link\" e tente novamente")
	}

	return string(body), id, gid, nil
}
