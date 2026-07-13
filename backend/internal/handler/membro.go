package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"go.uber.org/zap"
)

type MembroStore interface {
	List(ctx context.Context) ([]domain.Membro, error)
	UpdateTeam(ctx context.Context, id uuid.UUID, team string) error
	ListTeams(ctx context.Context) ([]string, error)
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

func (h *MembroHandler) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req struct {
		Team string `json:"team"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}
	if req.Team == "" {
		respondError(w, http.StatusBadRequest, "team é obrigatório")
		return
	}

	if err := h.store.UpdateTeam(r.Context(), id, req.Team); err != nil {
		h.logger.Error("failed to update membro team", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar equipe")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "equipe atualizada"})
}

func (h *MembroHandler) ListTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := h.store.ListTeams(r.Context())
	if err != nil {
		h.logger.Error("failed to list teams", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar equipes")
		return
	}
	respondJSON(w, http.StatusOK, teams)
}
