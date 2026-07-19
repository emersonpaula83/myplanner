package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"go.uber.org/zap"
)

type EquipeStore interface {
	ListEquipes(ctx context.Context) ([]domain.Equipe, error)
	GetEquipeByID(ctx context.Context, id uuid.UUID) (*domain.Equipe, error)
	CreateEquipe(ctx context.Context, nome string) (*domain.Equipe, error)
	UpdateEquipe(ctx context.Context, id uuid.UUID, nome string) error
	DeleteEquipe(ctx context.Context, id uuid.UUID) error
	GetMembrosEquipe(ctx context.Context, equipeID uuid.UUID) ([]domain.Membro, error)
	AddMembroEquipe(ctx context.Context, equipeID uuid.UUID, membroID uuid.UUID) error
	RemoveMembroEquipe(ctx context.Context, equipeID uuid.UUID, membroID uuid.UUID) error
	GetDiasAusencia(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) (map[uuid.UUID]int, error)
	GetHorasTarefasEquipe(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]domain.HorasTarefasMembro, error)
}

type EquipeHandler struct {
	store  EquipeStore
	logger *zap.Logger
}

func NewEquipeHandler(store EquipeStore, logger *zap.Logger) *EquipeHandler {
	return &EquipeHandler{store: store, logger: logger}
}

var periodos = map[string]int{
	"1m": 1,
	"2m": 2,
	"3m": 3,
	"6m": 6,
	"1a": 12,
}

func ParsePeriodo(p string) (time.Time, time.Time, bool) {
	meses, ok := periodos[p]
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	now := time.Now()
	fim := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	inicio := fim.AddDate(0, -meses, 0)
	return inicio, fim, true
}

func ContarDiasUteis(inicio, fim time.Time) int {
	dias := 0
	for d := inicio; !d.After(fim); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			dias++
		}
	}
	return dias
}

func CalcularResumoEquipe(
	equipeID uuid.UUID,
	equipeNome string,
	periodo string,
	membros []domain.Membro,
	ausencias map[uuid.UUID]int,
	tarefas []domain.HorasTarefasMembro,
	diasUteis int,
) domain.ResumoEquipe {
	horasTotalPeriodo := float64(diasUteis) * 8.0

	tarefasMap := make(map[uuid.UUID]domain.HorasTarefasMembro)
	for _, t := range tarefas {
		tarefasMap[t.MembroID] = t
	}

	var somaAtuacao float64
	var totalSegundosEquipe int64
	var totalMetas, totalCompromissos, totalIniciativas int64
	var totalManutencao, totalMelhorias, totalSuporte int64
	var totalEstimadoAbertos int64

	membrosResumo := make([]domain.MembroResumo, len(membros))
	for i, m := range membros {
		diasAusencia := ausencias[m.ID]
		horasReais := horasTotalPeriodo - float64(diasAusencia)*8.0
		if horasReais < 0 {
			horasReais = 0
		}

		t := tarefasMap[m.ID]
		horasCards := float64(t.TotalSegundos) / 3600.0

		var pctAtuacao float64
		if horasReais > 0 {
			pctAtuacao = (horasCards / horasReais) * 100
		}

		membrosResumo[i] = domain.MembroResumo{
			ID:               m.ID,
			Nome:             m.Nome,
			Email:            m.Email,
			AvatarURL:        m.AvatarURL,
			AtuacaoRastreada: pctAtuacao,
		}

		somaAtuacao += pctAtuacao
		totalSegundosEquipe += t.TotalSegundos
		totalMetas += t.SegundosMetas
		totalCompromissos += t.SegundosCompromissos
		totalIniciativas += t.SegundosIniciativas
		totalManutencao += t.SegundosManutencao
		totalMelhorias += t.SegundosMelhorias
		totalSuporte += t.SegundosSuporte
		totalEstimadoAbertos += t.SegundosEstimadoAbertos
	}

	mediaAtuacao := 0.0
	if len(membros) > 0 {
		mediaAtuacao = somaAtuacao / float64(len(membros))
	}

	pctMetas, pctCompromissos, pctIniciativas := 0.0, 0.0, 0.0
	if totalSegundosEquipe > 0 {
		pctMetas = float64(totalMetas) / float64(totalSegundosEquipe) * 100
		pctCompromissos = float64(totalCompromissos) / float64(totalSegundosEquipe) * 100
		pctIniciativas = float64(totalIniciativas) / float64(totalSegundosEquipe) * 100
	}

	detalhes := domain.DetalhesIniciativas{}
	if totalIniciativas > 0 {
		detalhes.PercentualManutencao = float64(totalManutencao) / float64(totalIniciativas) * 100
		detalhes.PercentualMelhorias = float64(totalMelhorias) / float64(totalIniciativas) * 100
		detalhes.PercentualSuporte = float64(totalSuporte) / float64(totalIniciativas) * 100
	}

	return domain.ResumoEquipe{
		EquipeID:               equipeID,
		NomeEquipe:             equipeNome,
		Periodo:                periodo,
		AtuacaoRastreada:       mediaAtuacao,
		TotalHorasEstimadas:    float64(totalEstimadoAbertos) / 3600.0,
		PercentualMetas:        pctMetas,
		PercentualCompromissos: pctCompromissos,
		PercentualIniciativas:  pctIniciativas,
		DetalhesIniciativas:    detalhes,
		Membros:                membrosResumo,
	}
}

func (h *EquipeHandler) List(w http.ResponseWriter, r *http.Request) {
	equipes, err := h.store.ListEquipes(r.Context())
	if err != nil {
		h.logger.Error("failed to list equipes", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list equipes")
		return
	}
	respondJSON(w, http.StatusOK, equipes)
}

func (h *EquipeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nome string `json:"nome"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if req.Nome == "" {
		respondError(w, http.StatusBadRequest, "nome é obrigatório")
		return
	}
	equipe, err := h.store.CreateEquipe(r.Context(), req.Nome)
	if err != nil {
		h.logger.Error("failed to create equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao criar equipe")
		return
	}
	respondJSON(w, http.StatusCreated, equipe)
}

func (h *EquipeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		Nome string `json:"nome"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if req.Nome == "" {
		respondError(w, http.StatusBadRequest, "nome é obrigatório")
		return
	}
	if err := h.store.UpdateEquipe(r.Context(), id, req.Nome); err != nil {
		h.logger.Error("failed to update equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar equipe")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "equipe atualizada"})
}

func (h *EquipeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}
	if err := h.store.DeleteEquipe(r.Context(), id); err != nil {
		h.logger.Error("failed to delete equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao excluir equipe")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "equipe excluída"})
}

func (h *EquipeHandler) GetResumo(w http.ResponseWriter, r *http.Request) {
	equipeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}
	periodo := r.URL.Query().Get("periodo")
	if periodo == "" {
		periodo = "3m"
	}

	inicio, fim, ok := ParsePeriodo(periodo)
	if !ok {
		respondError(w, http.StatusBadRequest, "periodo inválido: use 1m, 2m, 3m, 6m, 1a")
		return
	}

	equipe, err := h.store.GetEquipeByID(r.Context(), equipeID)
	if err != nil {
		h.logger.Error("failed to get equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get equipe")
		return
	}
	if equipe == nil {
		respondError(w, http.StatusNotFound, "equipe não encontrada")
		return
	}

	membros, err := h.store.GetMembrosEquipe(r.Context(), equipeID)
	if err != nil {
		h.logger.Error("failed to get membros equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get team members")
		return
	}
	if len(membros) == 0 {
		respondJSON(w, http.StatusOK, domain.ResumoEquipe{
			EquipeID:   equipeID,
			NomeEquipe: equipe.Nome,
			Periodo:    periodo,
			Membros:    []domain.MembroResumo{},
		})
		return
	}

	membroIDs := make([]uuid.UUID, len(membros))
	for i, m := range membros {
		membroIDs[i] = m.ID
	}

	ausencias, err := h.store.GetDiasAusencia(r.Context(), membroIDs, inicio, fim)
	if err != nil {
		h.logger.Error("failed to get ausencias", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get absences")
		return
	}

	tarefas, err := h.store.GetHorasTarefasEquipe(r.Context(), membroIDs, inicio, fim)
	if err != nil {
		h.logger.Error("failed to get horas tarefas", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get task hours")
		return
	}

	diasUteis := ContarDiasUteis(inicio, fim)
	resumo := CalcularResumoEquipe(equipeID, equipe.Nome, periodo, membros, ausencias, tarefas, diasUteis)
	respondJSON(w, http.StatusOK, resumo)
}

func (h *EquipeHandler) GetMembros(w http.ResponseWriter, r *http.Request) {
	equipeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}
	membros, err := h.store.GetMembrosEquipe(r.Context(), equipeID)
	if err != nil {
		h.logger.Error("failed to get membros equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get team members")
		return
	}
	respondJSON(w, http.StatusOK, membros)
}

func (h *EquipeHandler) AddMembro(w http.ResponseWriter, r *http.Request) {
	equipeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}
	var req struct {
		MembroID string `json:"membro_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	membroID, err := uuid.Parse(req.MembroID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "membro_id inválido")
		return
	}
	if err := h.store.AddMembroEquipe(r.Context(), equipeID, membroID); err != nil {
		h.logger.Error("failed to add membro to equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao adicionar membro")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "membro adicionado"})
}

func (h *EquipeHandler) RemoveMembro(w http.ResponseWriter, r *http.Request) {
	equipeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}
	membroID, err := uuid.Parse(chi.URLParam(r, "membroId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "membro_id inválido")
		return
	}
	if err := h.store.RemoveMembroEquipe(r.Context(), equipeID, membroID); err != nil {
		h.logger.Error("failed to remove membro from equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao remover membro")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "membro removido"})
}
