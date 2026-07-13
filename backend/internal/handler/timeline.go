package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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

// timelineData holds the raw data needed to compute team capacity for a
// given equipe/ano, shared by ListTimeline and AnalisarCapacidade.
type timelineData struct {
	epicos             []domain.EpicoEquipe
	membrosCount       int
	ausencias          []domain.AusenciaMensal
	projetosCapacidade []domain.ProjetoCapacidade
}

// fetchTimelineData fetches épicos, active member count and monthly
// absences for the given equipe/ano, and builds the derived
// projetosCapacidade slice used for capacity calculations.
func (h *TimelineHandler) fetchTimelineData(ctx context.Context, equipe string, ano int) (*timelineData, error) {
	epicos, err := h.store.BuscarEpicosEquipe(ctx, equipe, ano)
	if err != nil {
		return nil, fmt.Errorf("buscando épicos: %w", err)
	}

	membrosCount, err := h.store.ContarMembrosAtivosEquipe(ctx, equipe)
	if err != nil {
		return nil, fmt.Errorf("contando membros ativos: %w", err)
	}

	ausencias, err := h.store.BuscarAusenciasMensais(ctx, equipe, ano)
	if err != nil {
		return nil, fmt.Errorf("buscando ausências mensais: %w", err)
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

	return &timelineData{
		epicos:             epicos,
		membrosCount:       membrosCount,
		ausencias:          ausencias,
		projetosCapacidade: projetosCapacidade,
	}, nil
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

	data, err := h.fetchTimelineData(r.Context(), equipe, ano)
	if err != nil {
		h.logger.Error("failed to fetch timeline data", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar dados")
		return
	}

	projetos := make([]domain.ProjetoTimeline, len(data.epicos))
	for i, e := range data.epicos {
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
	}

	capacidade := CalcularCapacidadeMensal(ano, data.membrosCount, data.ausencias, data.projetosCapacidade)

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
		upper := strings.ToUpper(*req.Apelido)
		req.Apelido = &upper
		if utf8.RuneCountInString(*req.Apelido) > 15 {
			respondError(w, http.StatusBadRequest, "apelido deve ter no máximo 15 caracteres")
			return
		}
	}

	if err := h.store.AtualizarMetadataProjeto(r.Context(), id, req.Apelido, req.DataInicioExecucao); err != nil {
		h.logger.Error("failed to update metadata", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar metadados")
		return
	}

	atualizado, err := h.store.BuscarEpicoPorID(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to fetch updated epico", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar épico atualizado")
		return
	}
	if atualizado == nil {
		respondError(w, http.StatusNotFound, "tarefa não encontrada")
		return
	}

	var dataLimite *string
	if atualizado.DataLimite != nil && atualizado.DataLimite.Valid {
		s := atualizado.DataLimite.Time.Format("2006-01-02")
		dataLimite = &s
	}

	respondJSON(w, http.StatusOK, domain.ProjetoListItem{
		ID:                 atualizado.ID,
		NumeroTicket:       atualizado.NumeroTicket,
		Resumo:             atualizado.Resumo,
		Apelido:            atualizado.Apelido,
		DataInicioExecucao: atualizado.DataInicioExecucao,
		DataLimite:         dataLimite,
		TipoDemanda:        atualizado.TipoDemanda,
		Status:             atualizado.Status,
	})
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

	data, err := h.fetchTimelineData(r.Context(), req.Equipe, req.Ano)
	if err != nil {
		h.logger.Error("failed to fetch timeline data for analysis", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar dados")
		return
	}

	capacidade := CalcularCapacidadeMensal(req.Ano, data.membrosCount, data.ausencias, data.projetosCapacidade)

	var mesCap domain.CapacidadeMes
	for _, c := range capacidade {
		if c.Mes == req.Mes {
			mesCap = c
			break
		}
	}

	projetosAnalise := make([]domain.ProjetoAnalise, 0)
	for _, e := range data.epicos {
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
