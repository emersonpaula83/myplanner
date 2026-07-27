package handler

import (
	"encoding/json"
	"net/http"

	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AllocationHandler struct {
	svc    *service.AllocationService
	logger *zap.Logger
}

func NewAllocationHandler(svc *service.AllocationService, logger *zap.Logger) *AllocationHandler {
	return &AllocationHandler{svc: svc, logger: logger}
}

func (h *AllocationHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	equipeStr := r.URL.Query().Get("equipe_id")
	if equipeStr == "" {
		respondError(w, http.StatusBadRequest, "equipe_id is required")
		return
	}
	equipeID, err := uuid.Parse(equipeStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid equipe_id")
		return
	}

	produtoStr := r.URL.Query().Get("produto_id")
	if produtoStr == "" {
		respondError(w, http.StatusBadRequest, "produto_id is required")
		return
	}
	produtoID, err := uuid.Parse(produtoStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid produto_id")
		return
	}

	result, err := h.svc.ListProjectAllocations(r.Context(), equipeID, produtoID)
	if err != nil {
		h.logger.Error("listing project allocations", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar projetos")
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func (h *AllocationHandler) GetProjectDetail(w http.ResponseWriter, r *http.Request) {
	epicID, err := uuid.Parse(chi.URLParam(r, "epicId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid epicId")
		return
	}

	equipeStr := r.URL.Query().Get("equipe_id")
	if equipeStr == "" {
		respondError(w, http.StatusBadRequest, "equipe_id is required")
		return
	}
	equipeID, err := uuid.Parse(equipeStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid equipe_id")
		return
	}

	result, err := h.svc.GetProjectDetail(r.Context(), epicID, equipeID)
	if err != nil {
		h.logger.Error("getting project detail", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao carregar projeto")
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func (h *AllocationHandler) AllocateTask(w http.ResponseWriter, r *http.Request) {
	taskID, err := uuid.Parse(chi.URLParam(r, "taskId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid taskId")
		return
	}

	var req service.AllocateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	req.TaskID = taskID

	if req.SprintID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "sprint_id obrigatório")
		return
	}
	if req.EstimateHours <= 0 {
		respondError(w, http.StatusBadRequest, "estimate_hours deve ser > 0")
		return
	}
	if req.EquipeID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "equipe_id obrigatório")
		return
	}

	result, err := h.svc.AllocateTask(r.Context(), req)
	if err != nil {
		h.logger.Error("allocating task", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao alocar tarefa")
		return
	}

	if result.Conflict {
		respondJSON(w, http.StatusConflict, result)
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (h *AllocationHandler) SyncProject(w http.ResponseWriter, r *http.Request) {
	epicID, err := uuid.Parse(chi.URLParam(r, "epicId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid epicId")
		return
	}

	count, err := h.svc.SyncProjectTasks(r.Context(), epicID)
	if err != nil {
		h.logger.Error("syncing project tasks", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao sincronizar tarefas")
		return
	}

	respondJSON(w, http.StatusOK, map[string]int{"synced": count})
}

func (h *AllocationHandler) ListSprints(w http.ResponseWriter, r *http.Request) {
	equipeStr := r.URL.Query().Get("equipe_id")
	if equipeStr == "" {
		respondError(w, http.StatusBadRequest, "equipe_id is required")
		return
	}
	equipeID, err := uuid.Parse(equipeStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid equipe_id")
		return
	}

	result, err := h.svc.GetAvailableSprints(r.Context(), equipeID)
	if err != nil {
		h.logger.Error("listing sprints", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar sprints")
		return
	}
	respondJSON(w, http.StatusOK, result)
}
