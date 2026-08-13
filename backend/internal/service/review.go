package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

type ReviewService struct {
	repo       ReviewStore
	configRepo ConfigStore
	logger     *zap.Logger
}

func NewReviewService(repo ReviewStore, configRepo ConfigStore, logger *zap.Logger) *ReviewService {
	return &ReviewService{repo: repo, configRepo: configRepo, logger: logger}
}

func (s *ReviewService) GetAnalise(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*repository.ReviewAnalise, error) {
	return s.repo.GetReviewAnalise(ctx, sprintID, equipeID, produtoIDs)
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
	NaoIniciadas       int                 `json:"nao_iniciadas"`
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
	NumeroTicket    string  `json:"numero_ticket"`
	Produto         string  `json:"produto"`
	Resumo          string  `json:"resumo"`
	Tipo            string  `json:"tipo"`
	TipoDemanda     string  `json:"tipo_demanda"`
	Categoria       string  `json:"categoria"`
	Status          string  `json:"status"`
	NaoPlanejada    bool    `json:"nao_planejada"`
	EstimativaHoras float64 `json:"estimativa_horas"`
}

var statusEmAndamento = map[string]bool{
	"Desenvolvimento":          true,
	"Deploy":                   true,
	"Code Review":              true,
	"Teste":                    true,
	"Validação do Solicitante": true,
}

func (s *ReviewService) GetReviewData(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*ReviewData, error) {
	var tasks []repository.ReviewTaskRow
	var err error

	estado, _ := s.repo.GetSprintEstado(ctx, sprintID)
	isClosed := estado != nil && *estado == "closed"

	if isClosed {
		tasks, err = s.repo.GetSprintSnapshot(ctx, sprintID)
		if err != nil {
			tasks = nil
		}
	}

	if tasks == nil {
		tasks, err = s.repo.GetReviewTasks(ctx, sprintID, &equipeID, produtoIDs)
		if err != nil {
			return nil, fmt.Errorf("getting review tasks: %w", err)
		}
	} else if len(produtoIDs) > 0 {
		tasks = filterSnapshotTasks(tasks, produtoIDs)
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
		} else {
			stats.NaoIniciadas++
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

		var taskCategoria string
		if isBugIncidente {
			stats.BugsIncidentes++
			stats.Detalhes.BugsIncidentes[t.Tipo]++
			catManutencao++
			taskCategoria = "manutencao"
		} else if isGDPTC {
			stats.MelhoriasInovacoes++
			stats.Detalhes.MelhoriasInovacoes["Portfólio (GDPTC)"]++
			catNovos++
			taskCategoria = "novos_projetos"
		} else if tipoLower == "melhoria" || tipoLower == "história" || tipoLower == "historia" {
			stats.MelhoriasInovacoes++
			stats.Detalhes.MelhoriasInovacoes[t.Tipo]++
			catMelhorias++
			taskCategoria = "melhorias"
		} else {
			stats.Outros++
			catOutros++
			taskCategoria = "outros"
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
		tarefaList = append(tarefaList, ReviewTarefa{
			NumeroTicket: t.NumeroTicket,
			Produto:      produtoStr,
			Resumo:       t.Resumo,
			Tipo:         t.Tipo,
			TipoDemanda:  t.TipoDemanda,
			Categoria:    taskCategoria,
			Status:       t.Status,
			NaoPlanejada: t.NaoPlanejada,
			EstimativaHoras: func() float64 {
				if t.EstimativaTempo != nil {
					return float64(*t.EstimativaTempo) / 3600.0
				}
				return 0
			}(),
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

type promptTarefa struct {
	Ticket          string  `json:"ticket"`
	Resumo          string  `json:"resumo"`
	Tipo            string  `json:"tipo"`
	TipoDemanda     string  `json:"tipo_demanda"`
	Status          string  `json:"status"`
	Produto         string  `json:"produto"`
	NaoPlanejada    bool    `json:"nao_planejada"`
	EstimativaHoras float64 `json:"estimativa_horas"`
}

func buildReviewAnalisePrompt(tarefas []ReviewTarefa) (string, string) {
	systemPrompt := `Você é um analista de sprints de desenvolvimento de software.
Analise os dados da sprint e retorne um JSON com a análise separada por produto.

REGRAS:
1. Foco da Sprint: identifique onde a maior parte das horas estimadas foi gasta por produto
2. Top 3 Entregas: as 3 tarefas com maior estimativa por produto. Se tipo_demanda for "Meta" ou "Compromisso", marque destaque=true
3. Incidentes: avalie todos com tipo "Bug" ou contendo "Incidente". Se houver causa raiz similar entre eles, informe na causa_comum
4. Não Planejadas: liste tarefas com nao_planejada=true EXCLUINDO bugs e incidentes. Informe horas e percentual

Responda APENAS com JSON válido (sem markdown fences) no formato:
{
  "analises_por_produto": [
    {
      "produto": "Nome",
      "foco_sprint": {
        "descricao": "texto",
        "categoria_principal": "melhorias|manutencao|novos_projetos|outros",
        "horas_estimadas": 0
      },
      "top3_entregas": [
        {"ticket": "", "resumo": "", "tipo_demanda": "", "destaque": false, "horas_estimadas": 0}
      ],
      "analise_incidentes": {
        "total": 0,
        "resumo": "texto",
        "causa_comum": "texto ou null",
        "incidentes": [{"ticket": "", "resumo": "", "horas_estimadas": 0}]
      },
      "nao_planejadas": {
        "total": 0,
        "horas_total": 0,
        "percentual_sprint": 0,
        "resumo": "texto",
        "tarefas": [{"ticket": "", "resumo": "", "produto": "", "horas_estimadas": 0}]
      }
    }
  ]
}`

	pts := make([]promptTarefa, 0, len(tarefas))
	for _, t := range tarefas {
		pts = append(pts, promptTarefa{
			Ticket:          t.NumeroTicket,
			Resumo:          t.Resumo,
			Tipo:            t.Tipo,
			TipoDemanda:     t.TipoDemanda,
			Status:          t.Status,
			Produto:         t.Produto,
			NaoPlanejada:    t.NaoPlanejada,
			EstimativaHoras: t.EstimativaHoras,
		})
	}

	data, _ := json.Marshal(pts)
	userPrompt := "DADOS DA SPRINT:\n" + string(data)

	return systemPrompt, userPrompt
}

func (s *ReviewService) GenerateAnalise(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*repository.ReviewAnalise, error) {
	apiKey, err := s.configRepo.GetConfig(ctx, "openrouter_api_key")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("openrouter API key not configured: %w", err)
		}
		return nil, err
	}

	model := "openai/gpt-oss-20b:free"
	if m, err := s.configRepo.GetConfig(ctx, "openrouter_model"); err == nil && m != "" {
		model = m
	}

	reviewData, err := s.GetReviewData(ctx, sprintID, equipeID, produtoIDs)
	if err != nil {
		return nil, fmt.Errorf("getting review data for analysis: %w", err)
	}

	systemPrompt, userPrompt := buildReviewAnalisePrompt(reviewData.Tarefas)

	client := NewOpenRouterClient(apiKey, model)
	rawResponse, err := client.ChatCompletion(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("AI analysis failed: %w", err)
	}

	// Strip markdown fences and any trailing prose after closing fence
	cleaned := strings.TrimSpace(rawResponse)
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.Split(cleaned, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
		}
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "```") {
				lines = lines[:i]
				break
			}
		}
		cleaned = strings.TrimSpace(strings.Join(lines, "\n"))
	}

	var parsed json.RawMessage
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return nil, fmt.Errorf("AI returned invalid JSON: %w", err)
	}

	analise := repository.ReviewAnalise{
		SprintID:    sprintID,
		EquipeID:    equipeID,
		ProdutoIDs:  produtoIDs,
		AnaliseJSON: parsed,
		Modelo:      model,
	}

	if err := s.repo.SaveReviewAnalise(ctx, analise); err != nil {
		return nil, fmt.Errorf("saving analysis: %w", err)
	}

	saved, err := s.repo.GetReviewAnalise(ctx, sprintID, equipeID, produtoIDs)
	if err != nil {
		return nil, fmt.Errorf("retrieving saved analysis: %w", err)
	}

	return saved, nil
}

func filterSnapshotTasks(tasks []repository.ReviewTaskRow, produtoIDs []uuid.UUID) []repository.ReviewTaskRow {
	if len(produtoIDs) == 0 {
		return tasks
	}

	produtoSet := make(map[uuid.UUID]bool, len(produtoIDs))
	for _, pid := range produtoIDs {
		produtoSet[pid] = true
	}

	filtered := make([]repository.ReviewTaskRow, 0, len(tasks))
	for _, t := range tasks {
		for _, pid := range t.ProdutoIDs {
			if produtoSet[pid] {
				filtered = append(filtered, t)
				break
			}
		}
	}
	return filtered
}
