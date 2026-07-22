package handler

import (
	"encoding/json"
	"net/http"

	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type EqualizerHandler struct {
	svc    *service.EqualizerService
	logger *zap.Logger
}

func NewEqualizerHandler(svc *service.EqualizerService, logger *zap.Logger) *EqualizerHandler {
	return &EqualizerHandler{svc: svc, logger: logger}
}

func (h *EqualizerHandler) GetSuggestions(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}
	var equipeID *uuid.UUID
	if e := r.URL.Query().Get("equipe"); e != "" {
		id, err := uuid.Parse(e)
		if err == nil {
			equipeID = &id
		}
	}
	result, err := h.svc.Calculate(r.Context(), sprintID, equipeID)
	if err != nil {
		h.logger.Error("calculating equalizer", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao calcular equalização")
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func (h *EqualizerHandler) ApplyTransfers(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}
	var req service.ApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	req.SprintID = sprintID
	if len(req.Transferencias) == 0 {
		respondError(w, http.StatusBadRequest, "nenhuma transferência selecionada")
		return
	}
	if req.FonteDadosID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "fonte_dados_id obrigatório")
		return
	}
	result, err := h.svc.Apply(r.Context(), req)
	if err != nil {
		h.logger.Error("applying equalizer", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao aplicar transferências")
		return
	}
	respondJSON(w, http.StatusOK, result)
}
