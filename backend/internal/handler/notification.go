package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
)

type DestinatarioStore interface {
	ListByEquipe(ctx context.Context, equipeID uuid.UUID) ([]repository.Destinatario, error)
	Create(ctx context.Context, equipeID uuid.UUID, tipo, valor string, nome *string) (*repository.Destinatario, error)
	Delete(ctx context.Context, id uuid.UUID, equipeID uuid.UUID) error
}

type NotifServiceInterface interface {
	EnviarReview(ctx context.Context, sprintID, equipeID uuid.UUID, destIDs []uuid.UUID) ([]service.EnvioResultado, error)
}

type NotificationHandler struct {
	destRepo DestinatarioStore
	notifSvc NotifServiceInterface
	logger   *zap.Logger
}

func NewNotificationHandler(destRepo DestinatarioStore, notifSvc NotifServiceInterface, logger *zap.Logger) *NotificationHandler {
	return &NotificationHandler{destRepo: destRepo, notifSvc: notifSvc, logger: logger}
}

func (h *NotificationHandler) ListDestinatarios(w http.ResponseWriter, r *http.Request) {
	equipeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "equipe id inválido")
		return
	}
	if err := middleware.ValidateEquipeAccess(r.Context(), []uuid.UUID{equipeID}); err != nil {
		respondError(w, http.StatusForbidden, err.Error())
		return
	}
	dests, err := h.destRepo.ListByEquipe(r.Context(), equipeID)
	if err != nil {
		h.logger.Error("listing destinatarios", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar destinatários")
		return
	}
	if dests == nil {
		dests = []repository.Destinatario{}
	}
	respondJSON(w, http.StatusOK, dests)
}

func (h *NotificationHandler) CreateDestinatario(w http.ResponseWriter, r *http.Request) {
	equipeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "equipe id inválido")
		return
	}
	if err := middleware.ValidateEquipeAccess(r.Context(), []uuid.UUID{equipeID}); err != nil {
		respondError(w, http.StatusForbidden, err.Error())
		return
	}

	var body struct {
		Tipo  string  `json:"tipo"`
		Valor string  `json:"valor"`
		Nome  *string `json:"nome"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "body inválido")
		return
	}
	if body.Tipo != "email" && body.Tipo != "whatsapp" {
		respondError(w, http.StatusBadRequest, "tipo deve ser 'email' ou 'whatsapp'")
		return
	}
	if body.Valor == "" {
		respondError(w, http.StatusBadRequest, "valor é obrigatório")
		return
	}

	dest, err := h.destRepo.Create(r.Context(), equipeID, body.Tipo, body.Valor, body.Nome)
	if err != nil {
		h.logger.Error("creating destinatario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao criar destinatário")
		return
	}
	respondJSON(w, http.StatusCreated, dest)
}

func (h *NotificationHandler) DeleteDestinatario(w http.ResponseWriter, r *http.Request) {
	equipeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "equipe id inválido")
		return
	}
	if err := middleware.ValidateEquipeAccess(r.Context(), []uuid.UUID{equipeID}); err != nil {
		respondError(w, http.StatusForbidden, err.Error())
		return
	}
	destID, err := uuid.Parse(chi.URLParam(r, "destId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "destinatario id inválido")
		return
	}
	if err := h.destRepo.Delete(r.Context(), destID, equipeID); err != nil {
		h.logger.Error("deleting destinatario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao remover destinatário")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *NotificationHandler) EnviarReview(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "sprint id inválido")
		return
	}

	var body struct {
		EquipeID        uuid.UUID   `json:"equipe_id"`
		DestinatarioIDs []uuid.UUID `json:"destinatario_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "body inválido")
		return
	}
	if len(body.DestinatarioIDs) == 0 {
		respondError(w, http.StatusBadRequest, "nenhum destinatário selecionado")
		return
	}
	if err := middleware.ValidateEquipeAccess(r.Context(), []uuid.UUID{body.EquipeID}); err != nil {
		respondError(w, http.StatusForbidden, err.Error())
		return
	}

	resultados, err := h.notifSvc.EnviarReview(r.Context(), sprintID, body.EquipeID, body.DestinatarioIDs)
	if err != nil {
		h.logger.Error("sending review", zap.Error(err))
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"resultados": resultados})
}
