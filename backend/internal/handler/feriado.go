package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

type FeriadoStore interface {
	List(ctx context.Context) ([]repository.Feriado, error)
	Create(ctx context.Context, data string, nome string) (*repository.Feriado, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type FeriadoHandler struct {
	store  FeriadoStore
	logger *zap.Logger
}

func NewFeriadoHandler(store FeriadoStore, logger *zap.Logger) *FeriadoHandler {
	return &FeriadoHandler{store: store, logger: logger}
}

func (h *FeriadoHandler) List(w http.ResponseWriter, r *http.Request) {
	feriados, err := h.store.List(r.Context())
	if err != nil {
		h.logger.Error("listing feriados", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar feriados")
		return
	}
	respondJSON(w, http.StatusOK, feriados)
}

func (h *FeriadoHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data string `json:"data"`
		Nome string `json:"nome"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if req.Data == "" || req.Nome == "" {
		respondError(w, http.StatusBadRequest, "data e nome são obrigatórios")
		return
	}

	f, err := h.store.Create(r.Context(), req.Data, req.Nome)
	if err != nil {
		h.logger.Error("creating feriado", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao criar feriado")
		return
	}
	respondJSON(w, http.StatusCreated, f)
}

func (h *FeriadoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	if err := h.store.Delete(r.Context(), id); err != nil {
		h.logger.Error("deleting feriado", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao excluir feriado")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "excluído"})
}
