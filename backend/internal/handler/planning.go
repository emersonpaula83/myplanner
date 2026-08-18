package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type PlanningServiceInterface interface {
	GetNextSprint(ctx context.Context, currentSprintID uuid.UUID, equipeID *uuid.UUID) (*service.NextSprintResult, error)
	Apply(ctx context.Context, req service.PlanningApplyRequest) (string, error)
	GetProgress(jobID string) *service.PlanningJobProgress
	SearchTasks(ctx context.Context, sprintID uuid.UUID, ticketKeys []string) (*service.SearchTasksResult, error)
	IncludeTasks(ctx context.Context, sprintID uuid.UUID, tarefaIDs []uuid.UUID) ([]repository.PlanningTarefa, error)
}

type PlanningHandler struct {
	svc    PlanningServiceInterface
	logger *zap.Logger
}

func NewPlanningHandler(svc PlanningServiceInterface, logger *zap.Logger) *PlanningHandler {
	return &PlanningHandler{svc: svc, logger: logger}
}

func (h *PlanningHandler) GetNextSprint(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}

	var equipeID *uuid.UUID
	if e := r.URL.Query().Get("equipe_id"); e != "" {
		id, err := uuid.Parse(e)
		if err == nil {
			if err := middleware.ValidateEquipeAccess(r.Context(), []uuid.UUID{id}); err != nil {
				respondError(w, http.StatusForbidden, err.Error())
				return
			}
			equipeID = &id
		}
	}

	result, err := h.svc.GetNextSprint(r.Context(), sprintID, equipeID)
	if err != nil {
		h.logger.Error("getting next sprint", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar próxima sprint")
		return
	}
	if result == nil {
		respondError(w, http.StatusNotFound, "nenhuma sprint futura encontrada")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (h *PlanningHandler) Apply(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}

	var req service.PlanningApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	req.SprintID = sprintID

	if req.FonteDadosID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "fonte_dados_id obrigatório")
		return
	}

	jobID, err := h.svc.Apply(r.Context(), req)
	if err != nil {
		h.logger.Error("applying planning", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao aplicar planning")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"job_id": jobID})
}

func (h *PlanningHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		respondError(w, http.StatusBadRequest, "job_id obrigatório")
		return
	}

	progress := h.svc.GetProgress(jobID)
	if progress == nil {
		respondError(w, http.StatusNotFound, "job não encontrado")
		return
	}

	respondJSON(w, http.StatusOK, progress)
}

func (h *PlanningHandler) SearchTasks(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "ID inválido")
		return
	}

	var input struct {
		TicketKeys []string `json:"ticket_keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	normalized := make([]string, 0, len(input.TicketKeys))
	for _, k := range input.TicketKeys {
		k = strings.TrimSpace(strings.ToUpper(k))
		if k != "" {
			normalized = append(normalized, k)
		}
	}
	if len(normalized) == 0 {
		respondError(w, http.StatusBadRequest, "nenhuma chave informada")
		return
	}

	result, err := h.svc.SearchTasks(r.Context(), sprintID, normalized)
	if err != nil {
		h.logger.Error("failed to search tasks", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro ao pesquisar tarefas")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (h *PlanningHandler) IncludeTasks(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "ID inválido")
		return
	}

	var input struct {
		TarefaIDs []uuid.UUID `json:"tarefa_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if len(input.TarefaIDs) == 0 {
		respondError(w, http.StatusBadRequest, "nenhuma tarefa selecionada")
		return
	}

	tarefas, err := h.svc.IncludeTasks(r.Context(), sprintID, input.TarefaIDs)
	if err != nil {
		h.logger.Error("failed to include tasks", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro ao incluir tarefas")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"tarefas": tarefas})
}
