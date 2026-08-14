package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type InvestimentoStore interface {
	GetDashboard(ctx context.Context, equipeID uuid.UUID) (*domain.InvestimentoDashboard, error)
	GetGastosMensais(ctx context.Context, equipeID uuid.UUID, ano int) (*domain.GastosMensaisResponse, error)
	GetAlocacoesProjetos(ctx context.Context, membroID uuid.UUID) (*domain.AlocacoesProjetosResponse, error)
}

type MembroFinanceiroStore interface {
	UpdateSalario(ctx context.Context, id uuid.UUID, valor float64) error
	UpdateBancoHoras(ctx context.Context, id uuid.UUID, valor float64) error
	UpdateDataAdmissao(ctx context.Context, id uuid.UUID, data *time.Time) error
	GetHistoricoSalario(ctx context.Context, membroID uuid.UUID) ([]domain.SalarioHistorico, error)
	GetHistoricoBancoHoras(ctx context.Context, membroID uuid.UUID) ([]domain.BancoHorasHistorico, error)
}

type InvestimentoHandler struct {
	store       InvestimentoStore
	membroStore MembroFinanceiroStore
	logger      *zap.Logger
}

func NewInvestimentoHandler(store InvestimentoStore, membroStore MembroFinanceiroStore, logger *zap.Logger) *InvestimentoHandler {
	return &InvestimentoHandler{store: store, membroStore: membroStore, logger: logger}
}

func (h *InvestimentoHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	equipeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	dashboard, err := h.store.GetDashboard(r.Context(), equipeID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, "equipe não encontrada")
			return
		}
		h.logger.Error("failed to get investimento dashboard", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao carregar dashboard")
		return
	}

	// Travado, o valor não sai do servidor: é o que impede de lê-lo no F12.
	if !middleware.PodeVerSalarios(r.Context()) {
		dashboard.Sumario.CustoMensalTotal = nil
		for i := range dashboard.Membros {
			dashboard.Membros[i].Salario = nil
		}
	}

	respondJSON(w, http.StatusOK, dashboard)
}

func (h *InvestimentoHandler) GetGastosMensais(w http.ResponseWriter, r *http.Request) {
	equipeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	anoStr := r.URL.Query().Get("ano")
	ano := time.Now().Year()
	if anoStr != "" {
		ano, err = strconv.Atoi(anoStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "ano inválido")
			return
		}
	}

	result, err := h.store.GetGastosMensais(r.Context(), equipeID, ano)
	if err != nil {
		h.logger.Error("failed to get gastos mensais", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao carregar gastos mensais")
		return
	}

	// Travado, o valor não sai do servidor: é o que impede de lê-lo no F12.
	if !middleware.PodeVerSalarios(r.Context()) {
		for i := range result.Meses {
			result.Meses[i].CustoTotal = nil
		}
	}

	respondJSON(w, http.StatusOK, result)
}

func (h *InvestimentoHandler) UpdateSalario(w http.ResponseWriter, r *http.Request) {
	// Alterar salário sem poder vê-lo seria alterar às cegas — e seria o
	// caminho aberto para quem monta a requisição na mão.
	if !middleware.PodeVerSalarios(r.Context()) {
		respondError(w, http.StatusForbidden, "destrave os valores salariais para alterar salário")
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req struct {
		Valor float64 `json:"valor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if req.Valor < 0 {
		respondError(w, http.StatusBadRequest, "valor deve ser >= 0")
		return
	}

	if err := h.membroStore.UpdateSalario(r.Context(), id, req.Valor); err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, "membro não encontrado")
			return
		}
		h.logger.Error("failed to update salario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar salário")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "salário atualizado"})
}

func (h *InvestimentoHandler) UpdateBancoHoras(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req struct {
		Valor float64 `json:"valor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	if err := h.membroStore.UpdateBancoHoras(r.Context(), id, req.Valor); err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, "membro não encontrado")
			return
		}
		h.logger.Error("failed to update banco_horas", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar banco de horas")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "banco de horas atualizado"})
}

func (h *InvestimentoHandler) UpdateDataAdmissao(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req struct {
		DataAdmissao *string `json:"data_admissao"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	var dt *time.Time
	if req.DataAdmissao != nil && *req.DataAdmissao != "" {
		parsed, err := time.Parse("2006-01-02", *req.DataAdmissao)
		if err != nil {
			respondError(w, http.StatusBadRequest, "data_admissao inválida (formato: YYYY-MM-DD)")
			return
		}
		dt = &parsed
	}

	if err := h.membroStore.UpdateDataAdmissao(r.Context(), id, dt); err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, "membro não encontrado")
			return
		}
		h.logger.Error("failed to update data_admissao", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar data de admissão")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "data de admissão atualizada"})
}

func (h *InvestimentoHandler) GetHistoricoSalario(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	historico, err := h.membroStore.GetHistoricoSalario(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get historico salario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao carregar histórico salarial")
		return
	}
	if historico == nil {
		historico = []domain.SalarioHistorico{}
	}
	// Travado, a lista vem vazia: cada item carregaria um salário no corpo da
	// resposta, e isso é justamente o que o F12 não pode enxergar.
	if !middleware.PodeVerSalarios(r.Context()) {
		historico = []domain.SalarioHistorico{}
	}

	respondJSON(w, http.StatusOK, historico)
}

func (h *InvestimentoHandler) GetHistoricoBancoHoras(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	historico, err := h.membroStore.GetHistoricoBancoHoras(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get historico banco_horas", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao carregar histórico banco de horas")
		return
	}
	if historico == nil {
		historico = []domain.BancoHorasHistorico{}
	}

	respondJSON(w, http.StatusOK, historico)
}

func (h *InvestimentoHandler) GetAlocacoesProjetos(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	result, err := h.store.GetAlocacoesProjetos(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get alocacoes projetos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao carregar alocações")
		return
	}

	respondJSON(w, http.StatusOK, result)
}
