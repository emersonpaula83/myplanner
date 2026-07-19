package service

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

const horasPorDia = 6.0

type SprintStore interface {
	ListByProjeto(ctx context.Context, projetoID uuid.UUID, estado *string) ([]repository.SprintListItem, error)
	GetByID(ctx context.Context, id uuid.UUID) (*SprintInfo, error)
	GetCapacity(ctx context.Context, sprintID uuid.UUID) (*SprintCapacityResult, error)
}

type SprintInfo struct {
	ID         uuid.UUID  `json:"id"`
	Nome       string     `json:"nome"`
	Estado     *string    `json:"estado"`
	DataInicio *time.Time `json:"data_inicio"`
	DataFim    *time.Time `json:"data_fim"`
}

type AusenciaInfo struct {
	Tipo       string `json:"tipo"`
	DataInicio string `json:"data_inicio"`
	DataFim    string `json:"data_fim"`
	Dias       int    `json:"dias"`
}

type MembroCapacity struct {
	MembroID            uuid.UUID      `json:"membro_id"`
	Nome                string         `json:"nome"`
	AvatarURL           *string        `json:"avatar_url"`
	HorasEstimadas      float64        `json:"horas_estimadas"`
	HorasDisponiveis    float64        `json:"horas_disponiveis"`
	PercentualAlocacao  float64        `json:"percentual_alocacao"`
	Overcapacity        bool           `json:"overcapacity"`
	Ausencias           []AusenciaInfo `json:"ausencias"`
}

type SprintCapacityResult struct {
	Sprint    SprintInfo       `json:"sprint"`
	DiasUteis int              `json:"dias_uteis"`
	Membros   []MembroCapacity `json:"membros"`
}

type SprintService struct {
	repo   *repository.SprintRepository
	logger *zap.Logger
}

func NewSprintService(repo *repository.SprintRepository, logger *zap.Logger) *SprintService {
	return &SprintService{repo: repo, logger: logger}
}

func (s *SprintService) ListProjetosComSprints(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error) {
	return s.repo.ListProjetosComSprints(ctx, equipeID)
}

func (s *SprintService) ListByProjeto(ctx context.Context, projetoID uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
	return s.repo.ListByProjeto(ctx, projetoID, estado)
}

func (s *SprintService) ListSprints(ctx context.Context, equipeID *uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
	return s.repo.ListSprints(ctx, equipeID, estado)
}

func (s *SprintService) GetCapacity(ctx context.Context, sprintID uuid.UUID) (*SprintCapacityResult, error) {
	sprint, err := s.repo.GetByID(ctx, sprintID)
	if err != nil {
		return nil, err
	}

	info := SprintInfo{
		ID:         sprint.ID,
		Nome:       sprint.Nome,
		Estado:     sprint.Estado,
		DataInicio: sprint.DataInicio,
		DataFim:    sprint.DataFim,
	}

	if sprint.DataInicio == nil || sprint.DataFim == nil {
		return &SprintCapacityResult{Sprint: info, DiasUteis: 0, Membros: []MembroCapacity{}}, nil
	}

	diasUteis := contarDiasUteis(*sprint.DataInicio, *sprint.DataFim)

	membros, err := s.repo.GetMembrosFromSprint(ctx, sprintID)
	if err != nil {
		return nil, err
	}

	if len(membros) == 0 {
		return &SprintCapacityResult{Sprint: info, DiasUteis: diasUteis, Membros: []MembroCapacity{}}, nil
	}

	tarefas, err := s.repo.GetTarefasCapacityBySprint(ctx, sprintID)
	if err != nil {
		return nil, err
	}

	horasPorMembro := make(map[uuid.UUID]float64)
	for _, t := range tarefas {
		horasPorMembro[t.ResponsavelID] += float64(t.Segundos) / 3600.0
	}

	membroIDs := make([]uuid.UUID, len(membros))
	for i, m := range membros {
		membroIDs[i] = m.ID
	}

	ausencias, err := s.repo.GetAusenciasNoPeriodo(ctx, membroIDs, *sprint.DataInicio, *sprint.DataFim)
	if err != nil {
		return nil, err
	}

	ausenciasPorMembro := make(map[uuid.UUID][]repository.AusenciaRecord)
	for _, a := range ausencias {
		ausenciasPorMembro[a.MembroID] = append(ausenciasPorMembro[a.MembroID], a)
	}

	result := make([]MembroCapacity, 0, len(membros))
	for _, m := range membros {
		diasAusencia := 0
		var ausenciasInfo []AusenciaInfo

		for _, a := range ausenciasPorMembro[m.ID] {
			inicio := a.DataInicio
			if inicio.Before(*sprint.DataInicio) {
				inicio = *sprint.DataInicio
			}
			fim := a.DataFim
			if fim.After(*sprint.DataFim) {
				fim = *sprint.DataFim
			}
			dias := contarDiasUteis(inicio, fim)
			diasAusencia += dias
			ausenciasInfo = append(ausenciasInfo, AusenciaInfo{
				Tipo:       a.Tipo,
				DataInicio: a.DataInicio.Format("2006-01-02"),
				DataFim:    a.DataFim.Format("2006-01-02"),
				Dias:       dias,
			})
		}

		diasDisponiveis := diasUteis - diasAusencia
		if diasDisponiveis < 0 {
			diasDisponiveis = 0
		}
		horasDisponiveis := float64(diasDisponiveis) * horasPorDia
		horasEstimadas := horasPorMembro[m.ID]

		var pct float64
		if horasDisponiveis > 0 {
			pct = math.Round((horasEstimadas/horasDisponiveis)*1000) / 10
		} else if horasEstimadas > 0 {
			pct = 999.9
		}

		if ausenciasInfo == nil {
			ausenciasInfo = []AusenciaInfo{}
		}

		result = append(result, MembroCapacity{
			MembroID:           m.ID,
			Nome:               m.Nome,
			AvatarURL:          m.AvatarURL,
			HorasEstimadas:     math.Round(horasEstimadas*10) / 10,
			HorasDisponiveis:   math.Round(horasDisponiveis*10) / 10,
			PercentualAlocacao: pct,
			Overcapacity:       pct > 100,
			Ausencias:          ausenciasInfo,
		})
	}

	return &SprintCapacityResult{Sprint: info, DiasUteis: diasUteis, Membros: result}, nil
}

func contarDiasUteis(inicio, fim time.Time) int {
	startDate := time.Date(inicio.Year(), inicio.Month(), inicio.Day(), 0, 0, 0, 0, time.UTC)
	endDate := time.Date(fim.Year(), fim.Month(), fim.Day(), 0, 0, 0, 0, time.UTC)

	dias := 0
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			dias++
		}
	}
	return dias
}
