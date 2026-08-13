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
	configRepo *repository.ImportConfigRepository
	httpClient *http.Client
	logger     *zap.Logger
}

func NewImportService(membroRepo *repository.MembroRepository, equipeRepo *repository.EquipeRepository, configRepo *repository.ImportConfigRepository, logger *zap.Logger) *ImportService {
	return &ImportService{
		membroRepo: membroRepo,
		equipeRepo: equipeRepo,
		configRepo: configRepo,
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

func (s *ImportService) ConfirmImport(ctx context.Context, req domain.ConfirmImportRequest) (*domain.ConfirmImportResponse, error) {
	resp := &domain.ConfirmImportResponse{}

	for _, linha := range req.Linhas {
		if linha.Ignorar || linha.MembroID == nil {
			resp.Ignorados++
			continue
		}

		var dataAdmissao *time.Time
		if linha.Dados.DataAdmissao != nil {
			t, err := time.Parse("2006-01-02", *linha.Dados.DataAdmissao)
			if err != nil {
				return nil, fmt.Errorf("linha %d: data_admissao inválida: %w", linha.Linha, err)
			}
			dataAdmissao = &t
		}

		var ultimoAumento *time.Time
		if linha.Dados.UltimoAumento != nil {
			t, err := time.Parse("2006-01-02", *linha.Dados.UltimoAumento)
			if err != nil {
				return nil, fmt.Errorf("linha %d: ultimo_aumento inválido: %w", linha.Linha, err)
			}
			ultimoAumento = &t
		}

		if err := s.membroRepo.UpdateCamposImport(ctx, *linha.MembroID, linha.Dados.Salario, linha.Dados.Cargo, dataAdmissao, linha.Dados.Matricula, ultimoAumento, linha.Dados.GestorID); err != nil {
			return nil, fmt.Errorf("linha %d: atualizando membro: %w", linha.Linha, err)
		}

		if linha.EquipeID != nil {
			if err := s.equipeRepo.AddMembroEquipe(ctx, *linha.EquipeID, *linha.MembroID); err != nil {
				return nil, fmt.Errorf("linha %d: associando equipe: %w", linha.Linha, err)
			}
		}

		resp.Atualizados++
	}

	if req.Tipo != "" {
		if err := s.configRepo.Save(ctx, req.Tipo, req.URL, req.Gid); err != nil {
			s.logger.Warn("failed to save import config", zap.Error(err))
		}
	}

	return resp, nil
}

func (s *ImportService) GetSyncConfig(ctx context.Context) (*domain.ImportConfigResponse, error) {
	cfg, err := s.configRepo.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting import config: %w", err)
	}
	if cfg == nil {
		return nil, nil
	}
	resp := &domain.ImportConfigResponse{Tipo: cfg.Tipo, URL: cfg.URL, Gid: cfg.Gid}
	if cfg.UltimoSync != nil {
		formatted := cfg.UltimoSync.Format(time.RFC3339)
		resp.UltimoSync = &formatted
	}
	return resp, nil
}

func (s *ImportService) Sync(ctx context.Context) (*domain.ImportMatchResult, error) {
	cfg, err := s.configRepo.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting import config: %w", err)
	}
	if cfg == nil {
		return nil, fmt.Errorf("nenhuma configuração de sincronização salva")
	}
	if cfg.Tipo != "sheets_url" {
		return nil, fmt.Errorf("configuração é do tipo CSV; faça o upload de um novo arquivo")
	}
	if cfg.URL == nil {
		return nil, fmt.Errorf("configuração sem URL salva")
	}

	csvContent, _, _, err := s.FetchGoogleSheetCSV(ctx, *cfg.URL)
	if err != nil {
		return nil, err
	}

	result, err := s.MatchPlanilha(ctx, csvContent)
	if err != nil {
		return nil, err
	}

	if err := s.configRepo.UpdateUltimoSync(ctx); err != nil {
		s.logger.Warn("failed to update ultimo_sync", zap.Error(err))
	}

	return result, nil
}
