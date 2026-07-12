package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"go.uber.org/zap"
)

type EquipeStore interface {
	ListEquipes(ctx context.Context) ([]string, error)
	GetMembrosEquipe(ctx context.Context, team string) ([]domain.Membro, error)
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
	fim := time.Now().Truncate(24 * time.Hour)
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
	team string,
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
	var totalManutencao, totalMelhorias, totalEvolucao, totalSuporte int64

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
		totalEvolucao += t.SegundosEvolucao
		totalSuporte += t.SegundosSuporte
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
		detalhes.PercentualEvolucao = float64(totalEvolucao) / float64(totalIniciativas) * 100
		detalhes.PercentualSuporte = float64(totalSuporte) / float64(totalIniciativas) * 100
	}

	return domain.ResumoEquipe{
		NomeEquipe:             team,
		Periodo:                periodo,
		AtuacaoRastreada:       mediaAtuacao,
		PercentualMetas:        pctMetas,
		PercentualCompromissos: pctCompromissos,
		PercentualIniciativas:  pctIniciativas,
		DetalhesIniciativas:    detalhes,
		Membros:                membrosResumo,
	}
}

func (h *EquipeHandler) List(w http.ResponseWriter, r *http.Request) {
	teams, err := h.store.ListEquipes(r.Context())
	if err != nil {
		h.logger.Error("failed to list equipes", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list equipes")
		return
	}
	respondJSON(w, http.StatusOK, teams)
}

func (h *EquipeHandler) GetResumo(w http.ResponseWriter, r *http.Request) {
	team := chi.URLParam(r, "team")
	periodo := r.URL.Query().Get("periodo")
	if periodo == "" {
		periodo = "3m"
	}

	inicio, fim, ok := ParsePeriodo(periodo)
	if !ok {
		respondError(w, http.StatusBadRequest, "periodo inválido: use 1m, 2m, 3m, 6m, 1a")
		return
	}

	membros, err := h.store.GetMembrosEquipe(r.Context(), team)
	if err != nil {
		h.logger.Error("failed to get membros equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get team members")
		return
	}
	if len(membros) == 0 {
		respondError(w, http.StatusNotFound, "equipe não encontrada")
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
	resumo := CalcularResumoEquipe(team, periodo, membros, ausencias, tarefas, diasUteis)
	respondJSON(w, http.StatusOK, resumo)
}

func (h *EquipeHandler) GetMembros(w http.ResponseWriter, r *http.Request) {
	team := chi.URLParam(r, "team")
	membros, err := h.store.GetMembrosEquipe(r.Context(), team)
	if err != nil {
		h.logger.Error("failed to get membros equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get team members")
		return
	}
	respondJSON(w, http.StatusOK, membros)
}
