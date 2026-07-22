package handler

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

var timeRegex = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

type SyncScheduleHandler struct {
	repo   *repository.SyncScheduleRepository
	logger *zap.Logger
}

func NewSyncScheduleHandler(repo *repository.SyncScheduleRepository, logger *zap.Logger) *SyncScheduleHandler {
	return &SyncScheduleHandler{repo: repo, logger: logger}
}

func (h *SyncScheduleHandler) Get(w http.ResponseWriter, r *http.Request) {
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
	schedule, err := h.repo.GetByFonte(r.Context(), fdID)
	if err != nil {
		h.logger.Error("failed to get schedule", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar agendamento")
		return
	}
	if schedule == nil {
		respondJSON(w, http.StatusOK, nil)
		return
	}
	respondJSON(w, http.StatusOK, schedule)
}

type upsertScheduleRequest struct {
	FonteDadosID uuid.UUID `json:"fonte_dados_id"`
	ProjectKeys  []string  `json:"project_keys"`
	Horarios     []string  `json:"horarios"`
}

func (h *SyncScheduleHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	var req upsertScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	if req.FonteDadosID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "fonte_dados_id é obrigatório")
		return
	}
	if len(req.ProjectKeys) == 0 {
		respondError(w, http.StatusBadRequest, "selecione ao menos um projeto")
		return
	}
	if len(req.Horarios) == 0 {
		respondError(w, http.StatusBadRequest, "selecione ao menos um horário")
		return
	}
	if len(req.Horarios) > 4 {
		respondError(w, http.StatusBadRequest, "máximo de 4 horários permitidos")
		return
	}
	for _, hr := range req.Horarios {
		if !timeRegex.MatchString(hr) {
			respondError(w, http.StatusBadRequest, "horário inválido: "+hr+". Use formato HH:MM")
			return
		}
	}

	schedule, err := h.repo.Upsert(r.Context(), req.FonteDadosID, req.ProjectKeys, req.Horarios)
	if err != nil {
		h.logger.Error("failed to upsert schedule", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao salvar agendamento")
		return
	}
	respondJSON(w, http.StatusOK, schedule)
}

func (h *SyncScheduleHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

	if err := h.repo.Delete(r.Context(), fdID); err != nil {
		h.logger.Error("failed to delete schedule", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao remover agendamento")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SyncScheduleHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var body struct {
		Ativo bool `json:"ativo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	if err := h.repo.SetAtivo(r.Context(), id, body.Ativo); err != nil {
		h.logger.Error("failed to toggle schedule", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao alterar agendamento")
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"ativo": body.Ativo})
}
