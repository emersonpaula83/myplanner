package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

type FonteDadosStore interface {
	List(ctx context.Context) ([]domain.FonteDados, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error)
	Create(ctx context.Context, req *repository.CreateFonteDadosRequest) (*domain.FonteDados, error)
	Update(ctx context.Context, id uuid.UUID, req *repository.UpdateFonteDadosRequest) (*domain.FonteDados, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type FonteDadosHandler struct {
	store  FonteDadosStore
	logger *zap.Logger
}

func NewFonteDadosHandler(store FonteDadosStore, logger *zap.Logger) *FonteDadosHandler {
	return &FonteDadosHandler{
		store:  store,
		logger: logger,
	}
}

func (h *FonteDadosHandler) List(w http.ResponseWriter, r *http.Request) {
	fontes, err := h.store.List(r.Context())
	if err != nil {
		h.logger.Error("failed to list fontes de dados", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list fontes de dados")
		return
	}

	respondJSON(w, http.StatusOK, fontes)
}

func (h *FonteDadosHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id format")
		return
	}

	fonte, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get fonte de dados", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get fonte de dados")
		return
	}
	if fonte == nil {
		respondError(w, http.StatusNotFound, "fonte de dados not found")
		return
	}

	respondJSON(w, http.StatusOK, fonte)
}

func (h *FonteDadosHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input repository.CreateFonteDadosRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if input.Nome == "" {
		respondError(w, http.StatusBadRequest, "nome is required")
		return
	}
	if input.BaseURL == "" {
		respondError(w, http.StatusBadRequest, "base_url is required")
		return
	}
	if input.AuthType == "" {
		respondError(w, http.StatusBadRequest, "auth_type is required")
		return
	}
	if input.Tipo == "" {
		input.Tipo = "jira"
	}

	fonte, err := h.store.Create(r.Context(), &input)
	if err != nil {
		h.logger.Error("failed to create fonte de dados", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create fonte de dados")
		return
	}

	respondJSON(w, http.StatusCreated, fonte)
}

func (h *FonteDadosHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id format")
		return
	}

	var input repository.UpdateFonteDadosRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	fonte, err := h.store.Update(r.Context(), id, &input)
	if err != nil {
		h.logger.Error("failed to update fonte de dados", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update fonte de dados")
		return
	}
	if fonte == nil {
		respondError(w, http.StatusNotFound, "fonte de dados not found")
		return
	}

	respondJSON(w, http.StatusOK, fonte)
}

func (h *FonteDadosHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id format")
		return
	}

	if err := h.store.Delete(r.Context(), id); err != nil {
		h.logger.Error("failed to delete fonte de dados", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to delete fonte de dados")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
