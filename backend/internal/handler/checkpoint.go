package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

type CheckpointStore interface {
	List(ctx context.Context, equipeID *uuid.UUID, ano int) ([]repository.Checkpoint, error)
	Create(ctx context.Context, equipeID *uuid.UUID, nome, resumo, dataInicio string, dataFim *string, cor string) (*repository.Checkpoint, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type CheckpointHandler struct {
	store  CheckpointStore
	logger *zap.Logger
}

func NewCheckpointHandler(store CheckpointStore, logger *zap.Logger) *CheckpointHandler {
	return &CheckpointHandler{store: store, logger: logger}
}

func (h *CheckpointHandler) List(w http.ResponseWriter, r *http.Request) {
	equipeStr := r.URL.Query().Get("equipe_id")
	anoStr := r.URL.Query().Get("ano")

	var equipeID *uuid.UUID
	if equipeStr != "" {
		id, err := uuid.Parse(equipeStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "equipe_id inválido")
			return
		}
		equipeID = &id
	}

	ano := time.Now().Year()
	if anoStr != "" {
		var err error
		ano, err = strconv.Atoi(anoStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "ano inválido")
			return
		}
	}

	checkpoints, err := h.store.List(r.Context(), equipeID, ano)
	if err != nil {
		h.logger.Error("listing checkpoints", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar checkpoints")
		return
	}
	respondJSON(w, http.StatusOK, checkpoints)
}

func (h *CheckpointHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EquipeID   *uuid.UUID `json:"equipe_id"`
		Nome       string     `json:"nome"`
		Resumo     string     `json:"resumo"`
		DataInicio string     `json:"data_inicio"`
		DataFim    *string    `json:"data_fim"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	if req.Nome == "" || len(req.Nome) > 15 {
		respondError(w, http.StatusBadRequest, "Nome é obrigatório (máximo 15 caracteres)")
		return
	}
	if req.Resumo == "" || len(req.Resumo) > 50 {
		respondError(w, http.StatusBadRequest, "Resumo é obrigatório (máximo 50 caracteres)")
		return
	}
	if req.DataInicio == "" {
		respondError(w, http.StatusBadRequest, "Data de início é obrigatória")
		return
	}
	di, err := time.Parse("2006-01-02", req.DataInicio)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Data de início inválida")
		return
	}
	if req.DataFim != nil && *req.DataFim != "" {
		df, err := time.Parse("2006-01-02", *req.DataFim)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Data fim inválida")
			return
		}
		if df.Before(di) {
			respondError(w, http.StatusBadRequest, "Data fim deve ser posterior à data de início")
			return
		}
	}

	cor := generateCheckpointColor()

	cp, err := h.store.Create(r.Context(), req.EquipeID, req.Nome, req.Resumo, req.DataInicio, req.DataFim, cor)
	if err != nil {
		h.logger.Error("creating checkpoint", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao criar checkpoint")
		return
	}
	respondJSON(w, http.StatusCreated, cp)
}

func (h *CheckpointHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	if err := h.store.Delete(r.Context(), id); err != nil {
		h.logger.Error("deleting checkpoint", zap.Error(err))
		respondError(w, http.StatusNotFound, "checkpoint não encontrado")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func generateCheckpointColor() string {
	h := float64(rand.Intn(360))
	s := 50.0 + float64(rand.Intn(31))
	l := 45.0 + float64(rand.Intn(21))
	return hslToHex(h, s, l)
}

func hslToHex(h, s, l float64) string {
	s /= 100
	l /= 100
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}

	ri := int(math.Round((r + m) * 255))
	gi := int(math.Round((g + m) * 255))
	bi := int(math.Round((b + m) * 255))
	return fmt.Sprintf("#%02X%02X%02X", ri, gi, bi)
}
