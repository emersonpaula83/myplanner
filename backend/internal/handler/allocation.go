package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AllocationServiceInterface interface {
	ListProjectAllocations(ctx context.Context, equipeID uuid.UUID, produtoNomes []string, statusFilter string) ([]service.ProjectAllocation, error)
	GetProjectDetail(ctx context.Context, epicID, equipeID uuid.UUID) (*service.ProjectDetail, error)
	AllocateTask(ctx context.Context, req service.AllocateTaskRequest) (*service.AllocateTaskResult, error)
	SyncProjectTasks(ctx context.Context, epicID uuid.UUID) (int, error)
	GetAvailableSprints(ctx context.Context, equipeID uuid.UUID) ([]service.SprintOption, error)
	CloseProject(ctx context.Context, epicID uuid.UUID, req service.CloseProjectRequest, encerradoPor string) error
	ReopenProject(ctx context.Context, epicID uuid.UUID) error
	GetFilteredProducts(ctx context.Context) ([]repository.ProdutoRow, error)
}

type AllocationHandler struct {
	svc    AllocationServiceInterface
	logger *zap.Logger
}

func NewAllocationHandler(svc AllocationServiceInterface, logger *zap.Logger) *AllocationHandler {
	return &AllocationHandler{svc: svc, logger: logger}
}

func (h *AllocationHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	var equipeID uuid.UUID
	equipeStr := r.URL.Query().Get("equipe_id")
	if equipeStr != "" {
		id, err := uuid.Parse(equipeStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid equipe_id")
			return
		}
		equipeID = id
	}

	if equipeID != uuid.Nil {
		if err := middleware.ValidateEquipeAccess(r.Context(), []uuid.UUID{equipeID}); err != nil {
			respondError(w, http.StatusForbidden, err.Error())
			return
		}
	}

	produtoNomes := r.URL.Query()["produto_nome"]
	if len(produtoNomes) == 0 {
		respondError(w, http.StatusBadRequest, "produto_nome is required")
		return
	}

	statusFilter := r.URL.Query().Get("status")
	if statusFilter == "" {
		statusFilter = "em_andamento"
	}

	result, err := h.svc.ListProjectAllocations(r.Context(), equipeID, produtoNomes, statusFilter)
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

	var equipeID uuid.UUID
	equipeStr := r.URL.Query().Get("equipe_id")
	if equipeStr != "" {
		equipeID, err = uuid.Parse(equipeStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid equipe_id")
			return
		}
	}

	if equipeID != uuid.Nil {
		if err := middleware.ValidateEquipeAccess(r.Context(), []uuid.UUID{equipeID}); err != nil {
			respondError(w, http.StatusForbidden, err.Error())
			return
		}
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
	if err := middleware.ValidateEquipeAccess(r.Context(), []uuid.UUID{req.EquipeID}); err != nil {
		respondError(w, http.StatusForbidden, err.Error())
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

	if err := middleware.ValidateEquipeAccess(r.Context(), []uuid.UUID{equipeID}); err != nil {
		respondError(w, http.StatusForbidden, err.Error())
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

func (h *AllocationHandler) CloseProject(w http.ResponseWriter, r *http.Request) {
	epicID, err := uuid.Parse(chi.URLParam(r, "epicId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid epicId")
		return
	}

	var req service.CloseProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	if req.Descricao == "" {
		respondError(w, http.StatusBadRequest, "descricao obrigatória")
		return
	}
	if req.DataEncerramento == "" {
		respondError(w, http.StatusBadRequest, "data_encerramento obrigatória")
		return
	}

	if err := h.svc.CloseProject(r.Context(), epicID, req, ""); err != nil {
		h.logger.Error("closing project", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao encerrar projeto")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "closed"})
}

func (h *AllocationHandler) ReopenProject(w http.ResponseWriter, r *http.Request) {
	epicID, err := uuid.Parse(chi.URLParam(r, "epicId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid epicId")
		return
	}

	if err := h.svc.ReopenProject(r.Context(), epicID); err != nil {
		h.logger.Error("reopening project", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao reabrir projeto")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "reopened"})
}

func (h *AllocationHandler) ListFilteredProducts(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.GetFilteredProducts(r.Context())
	if err != nil {
		h.logger.Error("listing filtered products", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar produtos")
		return
	}
	respondJSON(w, http.StatusOK, result)
}
