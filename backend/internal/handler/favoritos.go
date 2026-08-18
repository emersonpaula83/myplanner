package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type FavoritosStore interface {
	List(ctx context.Context, usuarioID uuid.UUID, fonteDadosID uuid.UUID) ([]string, error)
	Replace(ctx context.Context, usuarioID uuid.UUID, fonteDadosID uuid.UUID, projectKeys []string) error
}

type FavoritosSyncService interface {
	SyncProject(ctx context.Context, fonteDadosID uuid.UUID, projectKey string) (*domain.SyncLog, error)
}

type FavoritosHandler struct {
	store   FavoritosStore
	syncSvc FavoritosSyncService
	logger  *zap.Logger
}

func NewFavoritosHandler(store FavoritosStore, syncSvc FavoritosSyncService, logger *zap.Logger) *FavoritosHandler {
	return &FavoritosHandler{store: store, syncSvc: syncSvc, logger: logger}
}

func (h *FavoritosHandler) List(w http.ResponseWriter, r *http.Request) {
	fonteID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "ID inválido")
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		respondError(w, http.StatusUnauthorized, "usuário não autenticado")
		return
	}

	keys, err := h.store.List(r.Context(), userID, fonteID)
	if err != nil {
		h.logger.Error("failed to list favoritos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro ao listar favoritos")
		return
	}

	respondJSON(w, http.StatusOK, keys)
}

func (h *FavoritosHandler) Replace(w http.ResponseWriter, r *http.Request) {
	fonteID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "ID inválido")
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		respondError(w, http.StatusUnauthorized, "usuário não autenticado")
		return
	}

	var input struct {
		ProjectKeys []string `json:"project_keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if input.ProjectKeys == nil {
		input.ProjectKeys = []string{}
	}

	if err := h.store.Replace(r.Context(), userID, fonteID, input.ProjectKeys); err != nil {
		h.logger.Error("failed to replace favoritos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro ao salvar favoritos")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"project_keys": input.ProjectKeys})
}

func (h *FavoritosHandler) TriggerBatch(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		respondError(w, http.StatusUnauthorized, "usuário não autenticado")
		return
	}

	var input struct {
		FonteDadosID uuid.UUID `json:"fonte_dados_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if input.FonteDadosID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "fonte_dados_id obrigatório")
		return
	}

	keys, err := h.store.List(r.Context(), userID, input.FonteDadosID)
	if err != nil {
		h.logger.Error("failed to list favoritos for batch", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro ao buscar favoritos")
		return
	}
	if len(keys) == 0 {
		respondError(w, http.StatusBadRequest, "nenhum projeto favorito para sincronizar")
		return
	}

	for _, key := range keys {
		if _, err := h.syncSvc.SyncProject(r.Context(), input.FonteDadosID, key); err != nil {
			h.logger.Warn("batch sync failed for project", zap.String("key", key), zap.Error(err))
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{"triggered": keys, "count": len(keys)})
}
