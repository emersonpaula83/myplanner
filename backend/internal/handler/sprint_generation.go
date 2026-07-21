package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"go.uber.org/zap"
)

type SprintGenerationHandler struct {
	svc    *service.SprintGenerationService
	logger *zap.Logger
}

func NewSprintGenerationHandler(svc *service.SprintGenerationService, logger *zap.Logger) *SprintGenerationHandler {
	return &SprintGenerationHandler{svc: svc, logger: logger}
}

func (h *SprintGenerationHandler) GetBoards(w http.ResponseWriter, r *http.Request) {
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

	boards, err := h.svc.GetBoardsForEquipe(r.Context(), equipeID)
	if err != nil {
		h.logger.Error("getting boards for equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get boards")
		return
	}
	respondJSON(w, http.StatusOK, boards)
}

type generateRequest struct {
	EquipeID  uuid.UUID `json:"equipe_id"`
	BoardID   int       `json:"board_id"`
	StartDate string    `json:"start_date"`
}

func (h *SprintGenerationHandler) Preview(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	startDate, err := h.validateRequest(req)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.svc.PreviewSprints(r.Context(), req.EquipeID, req.BoardID, startDate)
	if err != nil {
		h.logger.Error("previewing sprints", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Erro ao comunicar com JIRA")
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func (h *SprintGenerationHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	startDate, err := h.validateRequest(req)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.svc.GenerateSprints(r.Context(), req.EquipeID, req.BoardID, startDate)
	if err != nil {
		h.logger.Error("generating sprints", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Erro ao comunicar com JIRA")
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func (h *SprintGenerationHandler) validateRequest(req generateRequest) (time.Time, error) {
	if req.EquipeID == uuid.Nil {
		return time.Time{}, fmt.Errorf("equipe_id is required")
	}
	if req.BoardID == 0 {
		return time.Time{}, fmt.Errorf("board_id is required")
	}
	if req.StartDate == "" {
		return time.Time{}, fmt.Errorf("start_date is required")
	}
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("start_date must be in YYYY-MM-DD format")
	}
	today := time.Now().Truncate(24 * time.Hour)
	if startDate.Before(today) {
		return time.Time{}, fmt.Errorf("Data inicial não pode ser no passado")
	}
	return startDate, nil
}
