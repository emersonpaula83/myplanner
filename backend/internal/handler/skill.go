package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type SkillStore interface {
	List(ctx context.Context, query string) ([]domain.Skill, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Skill, error)
	Create(ctx context.Context, nome string) (*domain.Skill, error)
	Delete(ctx context.Context, id uuid.UUID) error
	GetMembroSkills(ctx context.Context, membroID uuid.UUID) ([]domain.Skill, error)
	AddMembroSkill(ctx context.Context, membroID, skillID uuid.UUID) error
	RemoveMembroSkill(ctx context.Context, membroID, skillID uuid.UUID) error
}

type SkillHandler struct {
	store  SkillStore
	logger *zap.Logger
}

func NewSkillHandler(store SkillStore, logger *zap.Logger) *SkillHandler {
	return &SkillHandler{store: store, logger: logger}
}

func (h *SkillHandler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	skills, err := h.store.List(r.Context(), query)
	if err != nil {
		h.logger.Error("failed to list skills", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar skills")
		return
	}
	respondJSON(w, http.StatusOK, skills)
}

func (h *SkillHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nome string `json:"nome"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo invalido")
		return
	}
	req.Nome = strings.TrimSpace(req.Nome)
	if req.Nome == "" {
		respondError(w, http.StatusBadRequest, "nome e obrigatorio")
		return
	}
	if len(req.Nome) > 100 {
		respondError(w, http.StatusBadRequest, "nome deve ter no maximo 100 caracteres")
		return
	}
	skill, err := h.store.Create(r.Context(), req.Nome)
	if err != nil {
		h.logger.Error("failed to create skill", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao criar skill")
		return
	}
	respondJSON(w, http.StatusOK, skill)
}

func (h *SkillHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id invalido")
		return
	}
	if err := h.store.Delete(r.Context(), id); err != nil {
		h.logger.Error("failed to delete skill", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao excluir skill")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "skill excluida"})
}

func (h *SkillHandler) GetMembroSkills(w http.ResponseWriter, r *http.Request) {
	membroID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id invalido")
		return
	}
	skills, err := h.store.GetMembroSkills(r.Context(), membroID)
	if err != nil {
		h.logger.Error("failed to get membro skills", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar skills do membro")
		return
	}
	respondJSON(w, http.StatusOK, skills)
}

func (h *SkillHandler) AddMembroSkill(w http.ResponseWriter, r *http.Request) {
	membroID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id invalido")
		return
	}
	var req struct {
		SkillID string `json:"skill_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo invalido")
		return
	}
	skillID, err := uuid.Parse(req.SkillID)
	if err != nil || skillID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "skill_id invalido")
		return
	}
	if err := h.store.AddMembroSkill(r.Context(), membroID, skillID); err != nil {
		h.logger.Error("failed to add skill to membro", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao associar skill")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "skill associada"})
}

func (h *SkillHandler) RemoveMembroSkill(w http.ResponseWriter, r *http.Request) {
	membroID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id invalido")
		return
	}
	skillID, err := uuid.Parse(chi.URLParam(r, "skillId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "skill_id invalido")
		return
	}
	if err := h.store.RemoveMembroSkill(r.Context(), membroID, skillID); err != nil {
		h.logger.Error("failed to remove skill from membro", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao desassociar skill")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "skill desassociada"})
}
