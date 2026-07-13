package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"github.com/totvs/tcloud-planner/backend/internal/service"
	"go.uber.org/zap"
)

type TimelineStore interface {
	BuscarEpicosEquipe(ctx context.Context, team string, ano int) ([]domain.EpicoEquipe, error)
	ContarMembrosAtivosEquipe(ctx context.Context, team string) (int, error)
	BuscarAusenciasMensais(ctx context.Context, team string, ano int) ([]domain.AusenciaMensal, error)
	AtualizarMetadataProjeto(ctx context.Context, id uuid.UUID, apelido *string, dataInicioExecucao *time.Time) error
	BuscarEpicoPorID(ctx context.Context, id uuid.UUID) (*domain.Tarefa, error)
	ListarEpicos(ctx context.Context, team *string) ([]domain.ProjetoListItem, error)
}

type TimelineHandler struct {
	store    TimelineStore
	analyzer service.AnalisadorCapacidade
	logger   *zap.Logger
}

func NewTimelineHandler(store TimelineStore, analyzer service.AnalisadorCapacidade, logger *zap.Logger) *TimelineHandler {
	return &TimelineHandler{store: store, analyzer: analyzer, logger: logger}
}

func (h *TimelineHandler) ListTimeline(w http.ResponseWriter, r *http.Request) {
	equipe := r.URL.Query().Get("equipe")
	if equipe == "" {
		respondError(w, http.StatusBadRequest, "equipe é obrigatório")
		return
	}

	anoStr := r.URL.Query().Get("ano")
	if anoStr == "" {
		respondError(w, http.StatusBadRequest, "ano é obrigatório")
		return
	}
	ano, err := strconv.Atoi(anoStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "ano deve ser um número inteiro")
		return
	}

	epicos, err := h.store.BuscarEpicosEquipe(r.Context(), equipe, ano)
	if err != nil {
		h.logger.Error("failed to fetch epicos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar épicos")
		return
	}

	membrosCount, err := h.store.ContarMembrosAtivosEquipe(r.Context(), equipe)
	if err != nil {
		h.logger.Error("failed to count membros", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao contar membros")
		return
	}

	ausencias, err := h.store.BuscarAusenciasMensais(r.Context(), equipe, ano)
	if err != nil {
		h.logger.Error("failed to fetch ausencias", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar ausências")
		return
	}

	projetos := make([]domain.ProjetoTimeline, len(epicos))
	projetosCapacidade := make([]domain.ProjetoCapacidade, 0)

	for i, e := range epicos {
		var dataLimite *string
		if e.DataLimite != nil {
			s := e.DataLimite.Format("2006-01-02")
			dataLimite = &s
		}

		projetos[i] = domain.ProjetoTimeline{
			ID:                 e.ID,
			NumeroTicket:       e.NumeroTicket,
			Apelido:            e.Apelido,
			Resumo:             e.Resumo,
			TipoDemanda:        e.TipoDemanda,
			Status:             e.Status,
			DataInicioExecucao: e.DataInicioExecucao,
			DataLimite:         dataLimite,
			TotalDiasEstimados: float64(e.TotalSegundosEquipe) / 3600.0 / 8.0,
			ProjetoCI:          e.ProjetoCI,
			ProjetoCITicket:    e.ProjetoCITicket,
		}

		if e.DataInicioExecucao != nil && e.DataLimite != nil {
			projetosCapacidade = append(projetosCapacidade, domain.ProjetoCapacidade{
				DataInicioExecucao: *e.DataInicioExecucao,
				DataLimite:         *e.DataLimite,
				HorasEquipe:        float64(e.TotalSegundosEquipe) / 3600.0,
			})
		}
	}

	capacidade := CalcularCapacidadeMensal(ano, membrosCount, ausencias, projetosCapacidade)

	respondJSON(w, http.StatusOK, domain.TimelineResponse{
		Equipe:           equipe,
		Ano:              ano,
		Projetos:         projetos,
		CapacidadeMensal: capacidade,
	})
}

func (h *TimelineHandler) UpdateProjetoMetadata(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req domain.MetadataProjetoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	tarefa, err := h.store.BuscarEpicoPorID(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to fetch epico", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar épico")
		return
	}
	if tarefa == nil {
		respondError(w, http.StatusNotFound, "tarefa não encontrada")
		return
	}
	if tarefa.Tipo != "Épico" {
		respondError(w, http.StatusBadRequest, "tarefa não é um Épico")
		return
	}

	if req.Apelido != nil {
		if len(*req.Apelido) > 15 {
			respondError(w, http.StatusBadRequest, "apelido deve ter no máximo 15 caracteres")
			return
		}
		upper := strings.ToUpper(*req.Apelido)
		req.Apelido = &upper
	}

	if err := h.store.AtualizarMetadataProjeto(r.Context(), id, req.Apelido, req.DataInicioExecucao); err != nil {
		h.logger.Error("failed to update metadata", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar metadados")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "metadados atualizados"})
}

func (h *TimelineHandler) ListProjetos(w http.ResponseWriter, r *http.Request) {
	var team *string
	if t := r.URL.Query().Get("equipe"); t != "" {
		team = &t
	}

	epicos, err := h.store.ListarEpicos(r.Context(), team)
	if err != nil {
		h.logger.Error("failed to list epicos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar épicos")
		return
	}

	respondJSON(w, http.StatusOK, epicos)
}

func (h *TimelineHandler) AnalisarCapacidade(w http.ResponseWriter, r *http.Request) {
	if h.analyzer == nil {
		respondError(w, http.StatusServiceUnavailable, "análise por IA não configurada (GEMINI_API_KEY ausente)")
		return
	}

	var req domain.AnalisarCapacidadeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	if req.Equipe == "" {
		respondError(w, http.StatusBadRequest, "equipe é obrigatório")
		return
	}
	if req.Ano <= 0 {
		respondError(w, http.StatusBadRequest, "ano inválido")
		return
	}
	if req.Mes < 1 || req.Mes > 12 {
		respondError(w, http.StatusBadRequest, "mes deve ser entre 1 e 12")
		return
	}

	epicos, err := h.store.BuscarEpicosEquipe(r.Context(), req.Equipe, req.Ano)
	if err != nil {
		h.logger.Error("failed to fetch epicos for analysis", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar dados")
		return
	}

	membrosCount, err := h.store.ContarMembrosAtivosEquipe(r.Context(), req.Equipe)
	if err != nil {
		h.logger.Error("failed to count membros for analysis", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar dados")
		return
	}

	ausencias, err := h.store.BuscarAusenciasMensais(r.Context(), req.Equipe, req.Ano)
	if err != nil {
		h.logger.Error("failed to fetch ausencias for analysis", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar dados")
		return
	}

	projetosCapacidade := make([]domain.ProjetoCapacidade, 0)
	for _, e := range epicos {
		if e.DataInicioExecucao != nil && e.DataLimite != nil {
			projetosCapacidade = append(projetosCapacidade, domain.ProjetoCapacidade{
				DataInicioExecucao: *e.DataInicioExecucao,
				DataLimite:         *e.DataLimite,
				HorasEquipe:        float64(e.TotalSegundosEquipe) / 3600.0,
			})
		}
	}

	capacidade := CalcularCapacidadeMensal(req.Ano, membrosCount, ausencias, projetosCapacidade)

	var mesCap domain.CapacidadeMes
	for _, c := range capacidade {
		if c.Mes == req.Mes {
			mesCap = c
			break
		}
	}

	projetosAnalise := make([]domain.ProjetoAnalise, 0)
	for _, e := range epicos {
		if e.DataInicioExecucao == nil || e.DataLimite == nil {
			continue
		}
		pc := domain.ProjetoCapacidade{
			DataInicioExecucao: *e.DataInicioExecucao,
			DataLimite:         *e.DataLimite,
			HorasEquipe:        float64(e.TotalSegundosEquipe) / 3600.0,
		}
		dist := DistribuirHorasPorMes([]domain.ProjetoCapacidade{pc}, req.Ano)
		if horasMes, ok := dist[req.Mes]; ok && horasMes > 0 {
			apelido := e.NumeroTicket
			if e.Apelido != nil {
				apelido = *e.Apelido
			}
			projetosAnalise = append(projetosAnalise, domain.ProjetoAnalise{
				Apelido:  apelido,
				HorasMes: horasMes,
				Resumo:   e.Resumo,
			})
		}
	}

	input := domain.AnaliseCapacidadeInput{
		Equipe:           req.Equipe,
		Ano:              req.Ano,
		Mes:              req.Mes,
		HorasDisponiveis: mesCap.HorasDisponiveis,
		HorasEstimadas:   mesCap.HorasEstimadas,
		PercentualDelta:  mesCap.PercentualDelta,
		MembrosAusentes:  mesCap.MembrosAusentes,
		Projetos:         projetosAnalise,
	}

	analise, err := h.analyzer.Analisar(r.Context(), input)
	if err != nil {
		h.logger.Error("gemini analysis failed", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha na análise por IA")
		return
	}

	respondJSON(w, http.StatusOK, domain.AnaliseResponse{Analise: analise})
}
