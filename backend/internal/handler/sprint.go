package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"go.uber.org/zap"
)

type SprintStore interface {
	ListProjetosComSprints(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error)
	ListByProjeto(ctx context.Context, projetoID uuid.UUID, estado *string) ([]repository.SprintListItem, error)
	ListSprints(ctx context.Context, equipeID *uuid.UUID, estado *string) ([]repository.SprintListItem, error)
	GetCapacity(ctx context.Context, sprintID uuid.UUID) (*service.SprintCapacityResult, error)
}

type SprintHandler struct {
	store  SprintStore
	logger *zap.Logger
}

func NewSprintHandler(store SprintStore, logger *zap.Logger) *SprintHandler {
	return &SprintHandler{store: store, logger: logger}
}

func (h *SprintHandler) ListProjetos(w http.ResponseWriter, r *http.Request) {
	var equipeID *uuid.UUID
	if e := r.URL.Query().Get("equipe"); e != "" {
		id, err := uuid.Parse(e)
		if err == nil {
			equipeID = &id
		}
	}

	projetos, err := h.store.ListProjetosComSprints(r.Context(), equipeID)
	if err != nil {
		h.logger.Error("listing projetos com sprints", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list projetos")
		return
	}
	respondJSON(w, http.StatusOK, projetos)
}

func (h *SprintHandler) ListSprints(w http.ResponseWriter, r *http.Request) {
	var equipeID *uuid.UUID
	if e := r.URL.Query().Get("equipe"); e != "" {
		id, err := uuid.Parse(e)
		if err == nil {
			equipeID = &id
		}
	}

	var estado *string
	if e := r.URL.Query().Get("estado"); e != "" {
		estado = &e
	}

	sprints, err := h.store.ListSprints(r.Context(), equipeID, estado)
	if err != nil {
		h.logger.Error("listing sprints", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list sprints")
		return
	}
	respondJSON(w, http.StatusOK, sprints)
}

func (h *SprintHandler) ListByProjeto(w http.ResponseWriter, r *http.Request) {
	projetoID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid projeto id")
		return
	}

	var estado *string
	if e := r.URL.Query().Get("estado"); e != "" {
		estado = &e
	}

	sprints, err := h.store.ListByProjeto(r.Context(), projetoID, estado)
	if err != nil {
		h.logger.Error("listing sprints", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list sprints")
		return
	}

	respondJSON(w, http.StatusOK, sprints)
}

func (h *SprintHandler) GetCapacity(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}

	result, err := h.store.GetCapacity(r.Context(), sprintID)
	if err != nil {
		h.logger.Error("getting sprint capacity", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get capacity")
		return
	}

	respondJSON(w, http.StatusOK, result)
}
