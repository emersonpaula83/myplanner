package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ReviewService struct {
	repo   *repository.ReviewRepository
	logger *zap.Logger
}

func NewReviewService(repo *repository.ReviewRepository, logger *zap.Logger) *ReviewService {
	return &ReviewService{repo: repo, logger: logger}
}

type ReviewData struct {
	POs                 []repository.ReviewPO     `json:"pos"`
	Stats               ReviewStats               `json:"stats"`
	GraficoProdutos     []ReviewGraficoProduto    `json:"grafico_produtos"`
	GraficoCategorias   []ReviewGraficoCategoria  `json:"grafico_categorias"`
	GraficoPlanejamento ReviewGraficoPlanejamento `json:"grafico_planejamento"`
	Tarefas             []ReviewTarefa            `json:"tarefas"`
}

type ReviewStats struct {
	Total              int                 `json:"total"`
	Concluidas         int                 `json:"concluidas"`
	EmAndamento        int                 `json:"em_andamento"`
	PlanejadasTotal    int                 `json:"planejadas_total"`
	PlanejadasConcl    int                 `json:"planejadas_concluidas"`
	BugsIncidentes     int                 `json:"bugs_incidentes"`
	MelhoriasInovacoes int                 `json:"melhorias_inovacoes"`
	Outros             int                 `json:"outros"`
	Detalhes           ReviewStatsDetalhes `json:"detalhes"`
}

type ReviewStatsDetalhes struct {
	EmAndamento        map[string]int `json:"em_andamento"`
	BugsIncidentes     map[string]int `json:"bugs_incidentes"`
	MelhoriasInovacoes map[string]int `json:"melhorias_inovacoes"`
}

type ReviewGraficoProduto struct {
	ProdutoID  uuid.UUID `json:"produto_id"`
	Produto    string    `json:"produto"`
	Total      int       `json:"total"`
	Concluidas int       `json:"concluidas"`
}

type ReviewGraficoCategoria struct {
	Categoria string `json:"categoria"`
	Total     int    `json:"total"`
}

type ReviewGraficoPlanejamento struct {
	Planejadas          int `json:"planejadas"`
	NaoPlanejadas       int `json:"nao_planejadas"`
	NaoPlanejadasBugs   int `json:"nao_planejadas_bugs"`
	NaoPlanejadasOutras int `json:"nao_planejadas_outras"`
}

type ReviewTarefa struct {
	NumeroTicket string `json:"numero_ticket"`
	Produto      string `json:"produto"`
	Resumo       string `json:"resumo"`
	Relator      string `json:"relator"`
}

var statusEmAndamento = map[string]bool{
	"Desenvolvimento":          true,
	"Deploy":                   true,
	"Code Review":              true,
	"Teste":                    true,
	"Validação do Solicitante": true,
}

func (s *ReviewService) GetReviewData(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*ReviewData, error) {
	tasks, err := s.repo.GetReviewTasks(ctx, sprintID, &equipeID, produtoIDs)
	if err != nil {
		return nil, fmt.Errorf("getting review tasks: %w", err)
	}

	// Collect task IDs with parent for GDPTC check (exclude bugs/incidents)
	var parentTaskIDs []uuid.UUID
	for _, t := range tasks {
		if t.ParentID != nil {
			tipoLower := strings.ToLower(t.Tipo)
			if tipoLower != "bug" && !strings.Contains(tipoLower, "incidente") {
				parentTaskIDs = append(parentTaskIDs, t.ID)
			}
		}
	}

	gdptcIDs, err := s.repo.GetGDPTCAncestorTaskIDs(ctx, parentTaskIDs)
	if err != nil {
		return nil, fmt.Errorf("getting GDPTC ancestors: %w", err)
	}
	gdptcSet := make(map[uuid.UUID]bool, len(gdptcIDs))
	for _, id := range gdptcIDs {
		gdptcSet[id] = true
	}

	pos, err := s.repo.GetReviewPOs(ctx, equipeID, produtoIDs)
	if err != nil {
		return nil, fmt.Errorf("getting review POs: %w", err)
	}

	// Compute stats, charts, and task list
	stats := ReviewStats{
		Detalhes: ReviewStatsDetalhes{
			EmAndamento:        make(map[string]int),
			BugsIncidentes:     make(map[string]int),
			MelhoriasInovacoes: make(map[string]int),
		},
	}
	produtoMap := make(map[string]*ReviewGraficoProduto)
	var catManutencao, catNovos, catMelhorias, catOutros int
	planejamento := ReviewGraficoPlanejamento{}
	tarefaList := make([]ReviewTarefa, 0, len(tasks))

	for _, t := range tasks {
		stats.Total++
		tipoLower := strings.ToLower(t.Tipo)

		// Status classification
		if t.Status == "Concluído" {
			stats.Concluidas++
			if !t.NaoPlanejada {
				stats.PlanejadasConcl++
			}
		} else if statusEmAndamento[t.Status] {
			stats.EmAndamento++
			stats.Detalhes.EmAndamento[t.Status]++
		}

		isBugIncidente := tipoLower == "bug" || strings.Contains(tipoLower, "incidente")

		// Planejada tracking
		if !t.NaoPlanejada {
			stats.PlanejadasTotal++
			planejamento.Planejadas++
		} else {
			planejamento.NaoPlanejadas++
			if isBugIncidente {
				planejamento.NaoPlanejadasBugs++
			} else {
				planejamento.NaoPlanejadasOutras++
			}
		}

		// Type classification
		isGDPTC := gdptcSet[t.ID]

		if isBugIncidente {
			stats.BugsIncidentes++
			stats.Detalhes.BugsIncidentes[t.Tipo]++
			catManutencao++
		} else if isGDPTC {
			stats.MelhoriasInovacoes++
			stats.Detalhes.MelhoriasInovacoes["Portfólio (GDPTC)"]++
			catNovos++
		} else if tipoLower == "melhoria" || tipoLower == "história" || tipoLower == "historia" {
			stats.MelhoriasInovacoes++
			stats.Detalhes.MelhoriasInovacoes[t.Tipo]++
			catMelhorias++
		} else {
			stats.Outros++
			catOutros++
		}

		// Products chart (use ProdutoIDs for unique key, Produtos for display name)
		for i, prod := range t.Produtos {
			var pid uuid.UUID
			if i < len(t.ProdutoIDs) {
				pid = t.ProdutoIDs[i]
			}
			key := pid.String()
			entry, ok := produtoMap[key]
			if !ok {
				entry = &ReviewGraficoProduto{ProdutoID: pid, Produto: prod}
				produtoMap[key] = entry
			}
			entry.Total++
			if t.Status == "Concluído" {
				entry.Concluidas++
			}
		}
		if len(t.Produtos) == 0 {
			entry, ok := produtoMap["sem-produto"]
			if !ok {
				entry = &ReviewGraficoProduto{Produto: "Sem Produto"}
				produtoMap["sem-produto"] = entry
			}
			entry.Total++
			if t.Status == "Concluído" {
				entry.Concluidas++
			}
		}

		// Task list
		produtoStr := ""
		if len(t.Produtos) > 0 {
			produtoStr = strings.Join(t.Produtos, ", ")
		}
		relator := ""
		if t.RelatorNome != nil {
			relator = *t.RelatorNome
		}
		tarefaList = append(tarefaList, ReviewTarefa{
			NumeroTicket: t.NumeroTicket,
			Produto:      produtoStr,
			Resumo:       t.Resumo,
			Relator:      relator,
		})
	}

	// Build grafico_produtos slice
	graficoProdutos := make([]ReviewGraficoProduto, 0, len(produtoMap))
	for _, v := range produtoMap {
		graficoProdutos = append(graficoProdutos, *v)
	}

	graficoCategorias := []ReviewGraficoCategoria{
		{Categoria: "manutencao", Total: catManutencao},
		{Categoria: "novos_projetos", Total: catNovos},
		{Categoria: "melhorias", Total: catMelhorias},
		{Categoria: "outros", Total: catOutros},
	}

	return &ReviewData{
		POs:                 pos,
		Stats:               stats,
		GraficoProdutos:     graficoProdutos,
		GraficoCategorias:   graficoCategorias,
		GraficoPlanejamento: planejamento,
		Tarefas:             tarefaList,
	}, nil
}

func (s *ReviewService) ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error) {
	return s.repo.ListDestaques(ctx, sprintID, equipeID)
}

func (s *ReviewService) CreateDestaque(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error) {
	return s.repo.CreateDestaque(ctx, d)
}

func (s *ReviewService) UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error) {
	return s.repo.UpdateDestaque(ctx, id, titulo, descricao, link)
}

func (s *ReviewService) DeleteDestaque(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteDestaque(ctx, id)
}
