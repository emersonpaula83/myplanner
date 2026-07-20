package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"go.uber.org/zap"
)

type MembroStore interface {
	List(ctx context.Context) ([]domain.Membro, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Membro, error)
	Search(ctx context.Context, query string) ([]domain.Membro, error)
	ListDisponibilidade(ctx context.Context, membroID uuid.UUID) ([]domain.Disponibilidade, error)
	CreateDisponibilidade(ctx context.Context, d *domain.Disponibilidade) error
	UpdateDisponibilidade(ctx context.Context, id uuid.UUID, tipo string, dataInicio, dataFim pgtype.Date, descricao *string) error
	DeleteDisponibilidade(ctx context.Context, id uuid.UUID) error
	GetMembroStats(ctx context.Context, membroID uuid.UUID, inicio, fim time.Time) (*domain.MembroStats, error)
	UpdateDataDesligamento(ctx context.Context, id uuid.UUID, dataDesligamento *time.Time) error
}

type MembroHandler struct {
	store  MembroStore
	logger *zap.Logger
}

func NewMembroHandler(store MembroStore, logger *zap.Logger) *MembroHandler {
	return &MembroHandler{store: store, logger: logger}
}

func (h *MembroHandler) List(w http.ResponseWriter, r *http.Request) {
	membros, err := h.store.List(r.Context())
	if err != nil {
		h.logger.Error("failed to list membros", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar membros")
		return
	}
	respondJSON(w, http.StatusOK, membros)
}

func (h *MembroHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		respondJSON(w, http.StatusOK, []domain.Membro{})
		return
	}
	membros, err := h.store.Search(r.Context(), q)
	if err != nil {
		h.logger.Error("failed to search membros", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar membros")
		return
	}
	respondJSON(w, http.StatusOK, membros)
}

func (h *MembroHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	periodo := r.URL.Query().Get("periodo")
	if periodo == "" {
		periodo = "3m"
	}
	inicio, fim, ok := ParsePeriodo(periodo)
	if !ok {
		respondError(w, http.StatusBadRequest, "periodo inválido: use 1m, 2m, 3m, 6m, 1a")
		return
	}

	membro, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get membro", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar membro")
		return
	}
	if membro == nil {
		respondError(w, http.StatusNotFound, "membro não encontrado")
		return
	}

	stats, err := h.store.GetMembroStats(r.Context(), id, inicio, fim)
	if err != nil {
		h.logger.Error("failed to get membro stats", zap.Error(err))
		stats = &domain.MembroStats{}
	}

	disponibilidade, err := h.store.ListDisponibilidade(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to list disponibilidade", zap.Error(err))
		disponibilidade = make([]domain.Disponibilidade, 0)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"membro":          membro,
		"stats":           stats,
		"periodo":         periodo,
		"disponibilidade": disponibilidade,
	})
}


func (h *MembroHandler) CreateDisponibilidade(w http.ResponseWriter, r *http.Request) {
	membroID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req struct {
		Tipo       string  `json:"tipo"`
		DataInicio string  `json:"data_inicio"`
		DataFim    string  `json:"data_fim"`
		Descricao  *string `json:"descricao"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if req.Tipo == "" || req.DataInicio == "" || req.DataFim == "" {
		respondError(w, http.StatusBadRequest, "tipo, data_inicio e data_fim são obrigatórios")
		return
	}

	dataInicio, err := parsePgDate(req.DataInicio)
	if err != nil {
		respondError(w, http.StatusBadRequest, "data_inicio inválida")
		return
	}
	dataFim, err := parsePgDate(req.DataFim)
	if err != nil {
		respondError(w, http.StatusBadRequest, "data_fim inválida")
		return
	}

	d := &domain.Disponibilidade{
		ID:         uuid.New(),
		MembroID:   membroID,
		Tipo:       req.Tipo,
		DataInicio: dataInicio,
		DataFim:    dataFim,
		Descricao:  req.Descricao,
	}

	if err := h.store.CreateDisponibilidade(r.Context(), d); err != nil {
		h.logger.Error("failed to create disponibilidade", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao criar disponibilidade")
		return
	}

	respondJSON(w, http.StatusCreated, d)
}

func (h *MembroHandler) UpdateDisponibilidade(w http.ResponseWriter, r *http.Request) {
	dispID, err := uuid.Parse(chi.URLParam(r, "dispId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id de disponibilidade inválido")
		return
	}

	var req struct {
		Tipo       string  `json:"tipo"`
		DataInicio string  `json:"data_inicio"`
		DataFim    string  `json:"data_fim"`
		Descricao  *string `json:"descricao"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	dataInicio, err := parsePgDate(req.DataInicio)
	if err != nil {
		respondError(w, http.StatusBadRequest, "data_inicio inválida")
		return
	}
	dataFim, err := parsePgDate(req.DataFim)
	if err != nil {
		respondError(w, http.StatusBadRequest, "data_fim inválida")
		return
	}

	if err := h.store.UpdateDisponibilidade(r.Context(), dispID, req.Tipo, dataInicio, dataFim, req.Descricao); err != nil {
		h.logger.Error("failed to update disponibilidade", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar disponibilidade")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "atualizado"})
}

func (h *MembroHandler) DeleteDisponibilidade(w http.ResponseWriter, r *http.Request) {
	dispID, err := uuid.Parse(chi.URLParam(r, "dispId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id de disponibilidade inválido")
		return
	}

	if err := h.store.DeleteDisponibilidade(r.Context(), dispID); err != nil {
		h.logger.Error("failed to delete disponibilidade", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao excluir disponibilidade")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "excluído"})
}

func (h *MembroHandler) UpdateDataDesligamento(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req struct {
		DataDesligamento *string `json:"data_desligamento"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	var dt *time.Time
	if req.DataDesligamento != nil && *req.DataDesligamento != "" {
		parsed, err := time.Parse("2006-01-02", *req.DataDesligamento)
		if err != nil {
			respondError(w, http.StatusBadRequest, "data_desligamento inválida")
			return
		}
		dt = &parsed
	}

	if err := h.store.UpdateDataDesligamento(r.Context(), id, dt); err != nil {
		h.logger.Error("failed to update data_desligamento", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar data de desligamento")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "atualizado"})
}

func parsePgDate(s string) (pgtype.Date, error) {
	var d pgtype.Date
	if err := d.Scan(s); err != nil {
		return d, err
	}
	return d, nil
}
