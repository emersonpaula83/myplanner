package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"go.uber.org/zap"
)

// SyncStoreInterface is the subset of the sync service consumed by
// SyncHandler. It matches service.SyncStore.
type SyncStoreInterface interface {
	SyncFonteDados(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error)
	SyncAll(ctx context.Context) ([]domain.SyncLog, error)
	SyncProject(ctx context.Context, fonteDadosID uuid.UUID, projectKey string) (*domain.SyncLog, error)
	ListJiraProjects(ctx context.Context, fonteDadosID uuid.UUID) ([]domain.JiraProjectInfo, error)
	GetStatus(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error)
	ListLogs(ctx context.Context, fonteDadosID uuid.UUID, limit int) ([]domain.SyncLog, error)
}

type SyncHandler struct {
	store  SyncStoreInterface
	logger *zap.Logger
}

func NewSyncHandler(store SyncStoreInterface, logger *zap.Logger) *SyncHandler {
	return &SyncHandler{store: store, logger: logger}
}

func (h *SyncHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	fdIDStr := r.URL.Query().Get("fonte_dados_id")
	projectKey := r.URL.Query().Get("project_key")

	if fdIDStr == "" {
		logs, err := h.store.SyncAll(r.Context())
		if err != nil {
			h.logger.Error("failed to sync all", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "falha ao sincronizar")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"message": "sincronização concluída",
			"logs":    logs,
		})
		return
	}

	fdID, err := uuid.Parse(fdIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "fonte_dados_id inválido")
		return
	}

	if projectKey != "" {
		log, err := h.store.SyncProject(r.Context(), fdID, projectKey)
		if err != nil {
			h.logger.Error("failed to sync project", zap.String("project", projectKey), zap.Error(err))
			respondError(w, http.StatusInternalServerError, "falha ao sincronizar projeto: "+err.Error())
			return
		}
		respondJSON(w, http.StatusOK, log)
		return
	}

	log, err := h.store.SyncFonteDados(r.Context(), fdID)
	if err != nil {
		h.logger.Error("failed to sync fonte dados", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao sincronizar: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, log)
}

func (h *SyncHandler) ListJiraProjects(w http.ResponseWriter, r *http.Request) {
	fdIDStr := r.URL.Query().Get("fonte_dados_id")
	if fdIDStr == "" {
		respondError(w, http.StatusBadRequest, "fonte_dados_id é obrigatório")
		return
	}

	fdID, err := uuid.Parse(fdIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "fonte_dados_id inválido")
		return
	}

	projects, err := h.store.ListJiraProjects(r.Context(), fdID)
	if err != nil {
		h.logger.Error("failed to list jira projects", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar projetos: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, projects)
}

func (h *SyncHandler) GetSyncStatus(w http.ResponseWriter, r *http.Request) {
	fdIDStr := r.URL.Query().Get("fonte_dados_id")
	if fdIDStr == "" {
		respondError(w, http.StatusBadRequest, "fonte_dados_id é obrigatório")
		return
	}

	fdID, err := uuid.Parse(fdIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "fonte_dados_id inválido")
		return
	}

	log, err := h.store.GetStatus(r.Context(), fdID)
	if err != nil {
		h.logger.Error("failed to get sync status", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar status")
		return
	}

	if log == nil {
		respondJSON(w, http.StatusOK, map[string]string{"status": "never_synced"})
		return
	}

	respondJSON(w, http.StatusOK, log)
}

func (h *SyncHandler) ListSyncLogs(w http.ResponseWriter, r *http.Request) {
	fdIDStr := r.URL.Query().Get("fonte_dados_id")
	if fdIDStr == "" {
		respondError(w, http.StatusBadRequest, "fonte_dados_id é obrigatório")
		return
	}

	fdID, err := uuid.Parse(fdIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "fonte_dados_id inválido")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}

	logs, err := h.store.ListLogs(r.Context(), fdID, limit)
	if err != nil {
		h.logger.Error("failed to list sync logs", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar logs")
		return
	}

	if logs == nil {
		logs = make([]domain.SyncLog, 0)
	}

	respondJSON(w, http.StatusOK, logs)
}
