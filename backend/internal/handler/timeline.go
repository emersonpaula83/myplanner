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
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"go.uber.org/zap"
)

type TimelineStore interface {
	BuscarEpicosEquipe(ctx context.Context, equipeID uuid.UUID, ano int, projetoIDs []uuid.UUID) ([]domain.EpicoEquipe, error)
	ContarMembrosAtivosEquipe(ctx context.Context, equipeID uuid.UUID, ano int) (int, error)
	ContarMembrosAtivosEquipes(ctx context.Context, equipeIDs []uuid.UUID, ano int) (int, error)
	BuscarAusenciasMensais(ctx context.Context, equipeID uuid.UUID, ano int) ([]domain.AusenciaMensal, error)
	BuscarAusenciasMensaisEquipes(ctx context.Context, equipeIDs []uuid.UUID, ano int) ([]domain.AusenciaMensal, error)
	BuscarFeriadosAno(ctx context.Context, ano int) ([]time.Time, error)
	BuscarMembrosComAusencias(ctx context.Context, equipeIDs []uuid.UUID, ano int) ([]domain.MembroTimeline, error)
	AtualizarMetadataProjeto(ctx context.Context, id uuid.UUID, apelido *string, dataInicioExecucao *time.Time) error
	BuscarEpicoPorID(ctx context.Context, id uuid.UUID) (*domain.Tarefa, error)
	ListarEpicos(ctx context.Context, equipeID *uuid.UUID, projetoIDs []uuid.UUID) ([]domain.ProjetoListItem, error)
}

type TimelineHandler struct {
	store    TimelineStore
	analyzer service.AnalisadorCapacidade
	logger   *zap.Logger
}

func NewTimelineHandler(store TimelineStore, analyzer service.AnalisadorCapacidade, logger *zap.Logger) *TimelineHandler {
	return &TimelineHandler{store: store, analyzer: analyzer, logger: logger}
}

func parseEquipeIDs(param string) ([]uuid.UUID, error) {
	parts := strings.Split(param, ",")
	ids := make([]uuid.UUID, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := uuid.Parse(p)
		if err != nil {
			return nil, fmt.Errorf("equipe id inválido: %s", p)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (h *TimelineHandler) ListTimeline(w http.ResponseWriter, r *http.Request) {
	equipeStr := r.URL.Query().Get("equipe")
	if equipeStr == "" {
		respondError(w, http.StatusBadRequest, "equipe é obrigatório")
		return
	}

	equipeIDs, err := parseEquipeIDs(equipeStr)
	if err != nil || len(equipeIDs) == 0 {
		respondError(w, http.StatusBadRequest, "equipe id inválido")
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

	projetoIDs := middleware.ProjetoIDsFromContext(r.Context())

	membrosCount, err := h.store.ContarMembrosAtivosEquipes(r.Context(), equipeIDs, ano)
	if err != nil {
		h.logger.Error("failed to count members", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao contar membros")
		return
	}

	ausencias, err := h.store.BuscarAusenciasMensaisEquipes(r.Context(), equipeIDs, ano)
	if err != nil {
		h.logger.Error("failed to fetch absences", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar ausências")
		return
	}

	feriados, err := h.store.BuscarFeriadosAno(r.Context(), ano)
	if err != nil {
		h.logger.Error("failed to fetch feriados", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar feriados")
		return
	}

	membrosTimeline, err := h.store.BuscarMembrosComAusencias(r.Context(), equipeIDs, ano)
	if err != nil {
		h.logger.Error("failed to fetch membros timeline", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar membros timeline")
		return
	}

	var allEpicos []domain.EpicoEquipe
	for _, eqID := range equipeIDs {
		epicos, err := h.store.BuscarEpicosEquipe(r.Context(), eqID, ano, projetoIDs)
		if err != nil {
			h.logger.Error("failed to fetch epicos", zap.Error(err))
			continue
		}
		allEpicos = append(allEpicos, epicos...)
	}

	projetos := make([]domain.ProjetoTimeline, len(allEpicos))
	for i, e := range allEpicos {
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
			TotalDiasEstimados: float64(e.TotalSegundosEquipe) / 3600.0 / 6.0,
			ProjetoCI:          e.ProjetoCI,
			ProjetoCITicket:    e.ProjetoCITicket,
		}
	}

	capacidade := CalcularCapacidadeMensal(ano, membrosCount, ausencias, feriados)

	equipeStrs := make([]string, len(equipeIDs))
	for i, id := range equipeIDs {
		equipeStrs[i] = id.String()
	}

	respondJSON(w, http.StatusOK, domain.TimelineResponse{
		Equipes:          equipeStrs,
		Ano:              ano,
		Projetos:         projetos,
		CapacidadeMensal: capacidade,
		MembrosTimeline:  membrosTimeline,
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
	var equipeID *uuid.UUID
	if t := r.URL.Query().Get("equipe"); t != "" {
		id, err := uuid.Parse(t)
		if err != nil {
			respondError(w, http.StatusBadRequest, "equipe id inválido")
			return
		}
		equipeID = &id
	}

	projetoIDs := middleware.ProjetoIDsFromContext(r.Context())
	epicos, err := h.store.ListarEpicos(r.Context(), equipeID, projetoIDs)
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
	equipeID, err := uuid.Parse(req.Equipe)
	if err != nil {
		respondError(w, http.StatusBadRequest, "equipe id inválido")
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

	equipeIDs := []uuid.UUID{equipeID}

	membrosCount, err := h.store.ContarMembrosAtivosEquipes(r.Context(), equipeIDs, req.Ano)
	if err != nil {
		h.logger.Error("failed to count members", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao contar membros")
		return
	}

	ausencias, err := h.store.BuscarAusenciasMensaisEquipes(r.Context(), equipeIDs, req.Ano)
	if err != nil {
		h.logger.Error("failed to fetch absences", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar ausências")
		return
	}

	feriados, err := h.store.BuscarFeriadosAno(r.Context(), req.Ano)
	if err != nil {
		h.logger.Error("failed to fetch feriados", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar feriados")
		return
	}

	capacidade := CalcularCapacidadeMensal(req.Ano, membrosCount, ausencias, feriados)

	var mesCap domain.CapacidadeMes
	for _, c := range capacidade {
		if c.Mes == req.Mes {
			mesCap = c
			break
		}
	}

	input := domain.AnaliseCapacidadeInput{
		Equipe:           req.Equipe,
		Ano:              req.Ano,
		Mes:              req.Mes,
		HorasDisponiveis: mesCap.HorasDisponiveis,
		MembrosAusentes:  mesCap.MembrosAusentes,
	}

	analise, err := h.analyzer.Analisar(r.Context(), input)
	if err != nil {
		h.logger.Error("gemini analysis failed", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha na análise por IA")
		return
	}

	respondJSON(w, http.StatusOK, domain.AnaliseResponse{Analise: analise})
}
