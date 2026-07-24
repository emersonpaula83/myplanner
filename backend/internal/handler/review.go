package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
)

type ReviewStore interface {
	GetReviewData(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*service.ReviewData, error)
	ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error)
	CreateDestaque(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error)
	UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error)
	DeleteDestaque(ctx context.Context, id uuid.UUID) error
}

type ReviewHandler struct {
	store  ReviewStore
	logger *zap.Logger
}

func NewReviewHandler(store ReviewStore, logger *zap.Logger) *ReviewHandler {
	return &ReviewHandler{store: store, logger: logger}
}

func (h *ReviewHandler) GetReviewData(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
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

	var produtoIDs []uuid.UUID
	if produtosStr := r.URL.Query().Get("produtos"); produtosStr != "" {
		for _, p := range strings.Split(produtosStr, ",") {
			pid, err := uuid.Parse(strings.TrimSpace(p))
			if err != nil {
				respondError(w, http.StatusBadRequest, "invalid produto id: "+p)
				return
			}
			produtoIDs = append(produtoIDs, pid)
		}
	}

	data, err := h.store.GetReviewData(r.Context(), sprintID, equipeID, produtoIDs)
	if err != nil {
		h.logger.Error("getting review data", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "error getting review data")
		return
	}

	respondJSON(w, http.StatusOK, data)
}

func (h *ReviewHandler) ListDestaques(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "sprintId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
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

	destaques, err := h.store.ListDestaques(r.Context(), sprintID, equipeID)
	if err != nil {
		h.logger.Error("listing destaques", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "error listing destaques")
		return
	}

	respondJSON(w, http.StatusOK, destaques)
}

type createDestaqueRequest struct {
	EquipeID  uuid.UUID `json:"equipe_id"`
	ProdutoID uuid.UUID `json:"produto_id"`
	Titulo    string    `json:"titulo"`
	Descricao string    `json:"descricao"`
	Link      *string   `json:"link"`
}

func (h *ReviewHandler) CreateDestaque(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "sprintId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}

	var req createDestaqueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Titulo == "" || req.Descricao == "" {
		respondError(w, http.StatusBadRequest, "titulo and descricao are required")
		return
	}
	if len(req.Titulo) > 200 {
		respondError(w, http.StatusBadRequest, "titulo max 200 characters")
		return
	}
	if req.EquipeID == uuid.Nil || req.ProdutoID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "equipe_id and produto_id are required")
		return
	}
	if req.Link != nil && *req.Link != "" && !strings.HasPrefix(*req.Link, "http://") && !strings.HasPrefix(*req.Link, "https://") {
		respondError(w, http.StatusBadRequest, "link must start with http:// or https://")
		return
	}

	d := repository.ReviewDestaque{
		SprintID:  sprintID,
		EquipeID:  req.EquipeID,
		ProdutoID: req.ProdutoID,
		Titulo:    req.Titulo,
		Descricao: req.Descricao,
		Link:      req.Link,
	}

	created, err := h.store.CreateDestaque(r.Context(), d)
	if err != nil {
		h.logger.Error("creating destaque", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "error creating destaque")
		return
	}

	respondJSON(w, http.StatusCreated, created)
}

type updateDestaqueRequest struct {
	Titulo    string  `json:"titulo"`
	Descricao string  `json:"descricao"`
	Link      *string `json:"link"`
}

func (h *ReviewHandler) UpdateDestaque(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid destaque id")
		return
	}

	var req updateDestaqueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Titulo == "" || req.Descricao == "" {
		respondError(w, http.StatusBadRequest, "titulo and descricao are required")
		return
	}
	if len(req.Titulo) > 200 {
		respondError(w, http.StatusBadRequest, "titulo max 200 characters")
		return
	}
	if req.Link != nil && *req.Link != "" && !strings.HasPrefix(*req.Link, "http://") && !strings.HasPrefix(*req.Link, "https://") {
		respondError(w, http.StatusBadRequest, "link must start with http:// or https://")
		return
	}

	updated, err := h.store.UpdateDestaque(r.Context(), id, req.Titulo, req.Descricao, req.Link)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "destaque not found")
			return
		}
		h.logger.Error("updating destaque", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "error updating destaque")
		return
	}

	respondJSON(w, http.StatusOK, updated)
}

func (h *ReviewHandler) DeleteDestaque(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid destaque id")
		return
	}

	if err := h.store.DeleteDestaque(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "destaque not found")
			return
		}
		h.logger.Error("deleting destaque", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "error deleting destaque")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
