package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type TarefaStore interface {
	ListTarefas(ctx context.Context, f repository.TarefaListFilter) (*repository.TarefaListResult, error)
	HardDeleteTarefa(ctx context.Context, id uuid.UUID) error
}

type TarefaHandler struct {
	repo   TarefaStore
	logger *zap.Logger
}

func NewTarefaHandler(repo TarefaStore, logger *zap.Logger) *TarefaHandler {
	return &TarefaHandler{repo: repo, logger: logger}
}

func (h *TarefaHandler) ListTarefas(w http.ResponseWriter, r *http.Request) {
	f := repository.TarefaListFilter{
		Removido: r.URL.Query().Get("removido"),
		Busca:    r.URL.Query().Get("busca"),
		Page:     1,
		PerPage:  50,
	}
	if f.Removido == "" {
		f.Removido = "nao"
	}

	if v := r.URL.Query().Get("equipe_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid equipe_id")
			return
		}
		f.EquipeID = &id

		if err := middleware.ValidateEquipeAccess(r.Context(), []uuid.UUID{id}); err != nil {
			respondError(w, http.StatusForbidden, err.Error())
			return
		}
	}

	if v := r.URL.Query().Get("produto_nome"); v != "" {
		f.ProdutoNome = &v
	}

	if v := r.URL.Query().Get("responsavel_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid responsavel_id")
			return
		}
		f.ResponsavelID = &id
	}

	if v := r.URL.Query().Get("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			f.Page = p
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if pp, err := strconv.Atoi(v); err == nil && pp > 0 && pp <= 100 {
			f.PerPage = pp
		}
	}

	result, err := h.repo.ListTarefas(r.Context(), f)
	if err != nil {
		h.logger.Error("listing tarefas", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar tarefas")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"items":    result.Items,
		"total":    result.Total,
		"page":     f.Page,
		"per_page": f.PerPage,
	})
}

func (h *TarefaHandler) DeleteTarefa(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.repo.HardDeleteTarefa(r.Context(), id); err != nil {
		h.logger.Error("hard-deleting tarefa", zap.Error(err))
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
