package service

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestNewSprintService(t *testing.T) {
	svc := NewSprintService(nil, zap.NewNop())
	if svc == nil {
		t.Fatal("expected non-nil")
	}
}

func TestListProjetosComSprints(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		expected := []repository.ProjetoComSprints{{ID: uuid.New(), Nome: "P1"}}
		repo := &mockSprintRepoStore{
			listProjetosComSprintsFn: func(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error) {
				return expected, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.ListProjetosComSprints(ctx, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("expected 1 result, got %d", len(result))
		}
	})

	t.Run("error", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			listProjetosComSprintsFn: func(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.ListProjetosComSprints(ctx, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestListByProjeto(t *testing.T) {
	ctx := context.Background()
	projetoID := uuid.New()

	t.Run("success", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			listByProjetoFn: func(ctx context.Context, pid uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
				return []repository.SprintListItem{{ID: uuid.New(), Nome: "Sprint 1"}}, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.ListByProjeto(ctx, projetoID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("expected 1, got %d", len(result))
		}
	})

	t.Run("error", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			listByProjetoFn: func(ctx context.Context, pid uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.ListByProjeto(ctx, projetoID, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestListSprints(t *testing.T) {
	ctx := context.Background()

	t.Run("without equipeID", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			listSprintsFn: func(ctx context.Context, eid *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error) {
				if boardID != nil {
					t.Error("expected nil boardID when no equipeID")
				}
				return []repository.SprintListItem{{Nome: "S1"}}, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.ListSprints(ctx, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("expected 1, got %d", len(result))
		}
	})

	t.Run("with equipeID gets boardID", func(t *testing.T) {
		equipeID := uuid.New()
		boardID := 42
		repo := &mockSprintRepoStore{
			getEquipeBoardIDFn: func(ctx context.Context, eid uuid.UUID) (*int, error) {
				return &boardID, nil
			},
			listSprintsFn: func(ctx context.Context, eid *uuid.UUID, estado *string, bid *int) ([]repository.SprintListItem, error) {
				if bid == nil || *bid != 42 {
					t.Errorf("expected boardID 42, got %v", bid)
				}
				return []repository.SprintListItem{{Nome: "S1"}}, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.ListSprints(ctx, &equipeID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("GetEquipeBoardID error", func(t *testing.T) {
		equipeID := uuid.New()
		repo := &mockSprintRepoStore{
			getEquipeBoardIDFn: func(ctx context.Context, eid uuid.UUID) (*int, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.ListSprints(ctx, &equipeID, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func floatsClose(a, b float64) bool {
	return math.Abs(a-b) < 0.05
}

func TestGetCapacity_GetByIDError(t *testing.T) {
	ctx := context.Background()
	sprintID := uuid.New()

	repo := &mockSprintRepoStore{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	svc := NewSprintService(repo, zap.NewNop())
	_, err := svc.GetCapacity(ctx, sprintID, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetCapacity_NoDates(t *testing.T) {
	ctx := context.Background()
	sprintID := uuid.New()

	repo := &mockSprintRepoStore{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
			return &domain.Sprint{ID: sprintID, Nome: "No Dates", FonteDadosID: uuid.New()}, nil
		},
	}
	svc := NewSprintService(repo, zap.NewNop())
	result, err := svc.GetCapacity(ctx, sprintID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Sprint.Nome != "No Dates" {
		t.Errorf("expected sprint nome 'No Dates', got %q", result.Sprint.Nome)
	}
	if len(result.Membros) != 0 {
		t.Errorf("expected 0 membros, got %d", len(result.Membros))
	}
	if len(result.Feriados) != 0 {
		t.Errorf("expected 0 feriados, got %d", len(result.Feriados))
	}
	if result.DiasUteis != 0 {
		t.Errorf("expected 0 dias uteis, got %d", result.DiasUteis)
	}
}

// TestGetCapacity_HappyPath exercises the full GetCapacity flow for a sprint
// running Mon 2026-01-05 through Fri 2026-01-16 (10 weekdays), with:
//   - one weekday holiday (2026-01-07, Wed) reducing dias uteis to 9
//   - one member with tasks spanning the "pendente" (Desenvolvimento),
//     "executado" (Concluído) and "ambos" (Teste) status categories, plus one
//     Cancelado task that must be excluded entirely
//   - one ausencia for that member on 2026-01-05 (1 dia util), reducing the
//     member's dias disponiveis to 9-1=8 (48 horas disponiveis)
func TestGetCapacity_HappyPath(t *testing.T) {
	ctx := context.Background()
	sprintID := uuid.New()
	projetoID := uuid.New()
	fonteDadosID := uuid.New()
	membroID := uuid.New()

	inicio := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)  // Monday
	fim := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)    // Friday (next week)
	feriado := time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC) // Wednesday

	tarefaPendenteID := uuid.New()
	tarefaExecutadaID := uuid.New()
	tarefaAmbosID := uuid.New()
	tarefaCanceladaID := uuid.New()

	repo := &mockSprintRepoStore{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
			if id != sprintID {
				t.Errorf("unexpected sprintID %v", id)
			}
			return &domain.Sprint{
				ID:           sprintID,
				FonteDadosID: fonteDadosID,
				ProjetoID:    &projetoID,
				Nome:         "Sprint 1",
				DataInicio:   &inicio,
				DataFim:      &fim,
			}, nil
		},
		getProjetoChaveFn: func(ctx context.Context, pid uuid.UUID) (string, error) {
			if pid != projetoID {
				t.Errorf("unexpected projetoID %v", pid)
			}
			return "PROJ", nil
		},
		getFeriadosNoPeriodoFn: func(ctx context.Context, i, f time.Time) ([]repository.FeriadoRecord, error) {
			return []repository.FeriadoRecord{
				{Data: feriado, Nome: "Feriado Teste"},
			}, nil
		},
		getMembrosFromSprintFn: func(ctx context.Context, sid uuid.UUID) ([]repository.MembroInfo, error) {
			if sid != sprintID {
				t.Errorf("unexpected sprintID %v", sid)
			}
			return []repository.MembroInfo{
				{ID: membroID, Nome: "Dev 1"},
			}, nil
		},
		getTarefasDetailBySprintFn: func(ctx context.Context, sid uuid.UUID) ([]repository.TarefaDetail, error) {
			return []repository.TarefaDetail{
				{
					ID: tarefaPendenteID, NumeroTicket: "PROJ-1", Resumo: "Pendente",
					Tipo: "Story", Status: "Desenvolvimento", Segundos: 8 * 3600,
					ProjetoID: projetoID, ProjetoChave: "PROJ", ProjetoNome: "Projeto",
					ResponsavelID: membroID,
				},
				{
					ID: tarefaExecutadaID, NumeroTicket: "PROJ-2", Resumo: "Executada",
					Tipo: "Bug", Status: "Concluído", Segundos: 4 * 3600,
					ProjetoID: projetoID, ProjetoChave: "PROJ", ProjetoNome: "Projeto",
					ResponsavelID: membroID,
				},
				{
					ID: tarefaAmbosID, NumeroTicket: "PROJ-3", Resumo: "Ambos",
					Tipo: "Story", Status: "Teste", Segundos: 2 * 3600,
					ProjetoID: projetoID, ProjetoChave: "PROJ", ProjetoNome: "Projeto",
					ResponsavelID: membroID,
				},
				{
					ID: tarefaCanceladaID, NumeroTicket: "PROJ-4", Resumo: "Cancelada",
					Tipo: "Bug", Status: "Cancelado", Segundos: 1 * 3600,
					ProjetoID: projetoID, ProjetoChave: "PROJ", ProjetoNome: "Projeto",
					ResponsavelID: membroID,
				},
			}, nil
		},
		getAusenciasNoPeriodoFn: func(ctx context.Context, mids []uuid.UUID, i, f time.Time) ([]repository.AusenciaRecord, error) {
			if len(mids) != 1 || mids[0] != membroID {
				t.Errorf("unexpected membroIDs %v", mids)
			}
			return []repository.AusenciaRecord{
				{MembroID: membroID, Tipo: "Ferias", DataInicio: inicio, DataFim: inicio},
			}, nil
		},
	}

	svc := NewSprintService(repo, zap.NewNop())
	result, err := svc.GetCapacity(ctx, sprintID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Sprint.Nome != "Sprint 1" {
		t.Errorf("expected sprint nome 'Sprint 1', got %q", result.Sprint.Nome)
	}
	if result.FonteDadosID != fonteDadosID {
		t.Errorf("expected fonteDadosID %v, got %v", fonteDadosID, result.FonteDadosID)
	}
	if result.ProjetoChave != "PROJ" {
		t.Errorf("expected projeto chave 'PROJ', got %q", result.ProjetoChave)
	}
	if result.DiasUteis != 9 {
		t.Errorf("expected 9 dias uteis (10 weekdays - 1 holiday), got %d", result.DiasUteis)
	}
	if len(result.Feriados) != 1 || result.Feriados[0].Data != "2026-01-07" {
		t.Errorf("expected 1 feriado on 2026-01-07, got %+v", result.Feriados)
	}

	if len(result.Membros) != 1 {
		t.Fatalf("expected 1 membro, got %d", len(result.Membros))
	}
	mc := result.Membros[0]
	if mc.Nome != "Dev 1" {
		t.Errorf("expected membro nome 'Dev 1', got %q", mc.Nome)
	}
	if !mc.DaEquipe {
		t.Error("expected DaEquipe true when equipeID is nil")
	}
	if mc.Desligado {
		t.Error("expected Desligado false")
	}

	// horasAlocadasMembro = 8 (pendente) + 2 (ambos) = 10
	// horasExecutadasMembro = 4 (executado) + 2 (ambos) = 6
	// horasAmbosPorMembro = 2
	// horasAlocPura = 10 - 2 = 8; horasEstimadas = 10 + 6 - 2 = 14
	if !floatsClose(mc.HorasAlocadas, 8.0) {
		t.Errorf("expected HorasAlocadas 8.0, got %v", mc.HorasAlocadas)
	}
	if !floatsClose(mc.HorasExecutadas, 6.0) {
		t.Errorf("expected HorasExecutadas 6.0, got %v", mc.HorasExecutadas)
	}
	if !floatsClose(mc.HorasEstimadas, 14.0) {
		t.Errorf("expected HorasEstimadas 14.0, got %v", mc.HorasEstimadas)
	}

	// dias uteis = 9, 1 dia de ausencia (2026-01-05) => 8 dias disponiveis * 6h = 48h
	if !floatsClose(mc.HorasDisponiveis, 48.0) {
		t.Errorf("expected HorasDisponiveis 48.0, got %v", mc.HorasDisponiveis)
	}
	// pct = round((8/48)*1000)/10 = 16.7
	if !floatsClose(mc.PercentualAlocacao, 16.7) {
		t.Errorf("expected PercentualAlocacao ~16.7, got %v", mc.PercentualAlocacao)
	}
	// pctExec = round((6/48)*1000)/10 = 12.5
	if !floatsClose(mc.PercentualExecutado, 12.5) {
		t.Errorf("expected PercentualExecutado ~12.5, got %v", mc.PercentualExecutado)
	}
	if mc.Overcapacity {
		t.Error("expected Overcapacity false")
	}

	if len(mc.Ausencias) != 1 || mc.Ausencias[0].Dias != 1 {
		t.Errorf("expected 1 ausencia of 1 dia, got %+v", mc.Ausencias)
	}

	// Cancelado task must be excluded; only 3 of the 4 tasks should appear.
	if len(mc.Tarefas) != 3 {
		t.Fatalf("expected 3 tarefas (Cancelado excluded), got %d", len(mc.Tarefas))
	}
	for _, td := range mc.Tarefas {
		if td.Status == "Cancelado" {
			t.Errorf("Cancelado task should not appear in Tarefas: %+v", td)
		}
	}

	// Sprint-level aggregates (equipeID nil => all membros count as "da equipe")
	if result.TotalMembrosEquipe != 1 {
		t.Errorf("expected TotalMembrosEquipe 1, got %d", result.TotalMembrosEquipe)
	}
	if !floatsClose(result.HorasTotalSprint, 48.0) {
		t.Errorf("expected HorasTotalSprint 48.0, got %v", result.HorasTotalSprint)
	}
	if !floatsClose(result.HorasAlocadas, 8.0) {
		t.Errorf("expected HorasAlocadas 8.0, got %v", result.HorasAlocadas)
	}
	if !floatsClose(result.HorasExecutadas, 6.0) {
		t.Errorf("expected HorasExecutadas 6.0, got %v", result.HorasExecutadas)
	}
	if !floatsClose(result.HorasPendentesExecucao, 8.0) {
		t.Errorf("expected HorasPendentesExecucao 8.0, got %v", result.HorasPendentesExecucao)
	}

	// Member has an ausencia and tasks => should appear in MembrosAusentesComCards.
	if len(result.MembrosAusentesComCards) != 1 {
		t.Fatalf("expected 1 membro ausente com cards, got %d", len(result.MembrosAusentesComCards))
	}
	ma := result.MembrosAusentesComCards[0]
	if ma.Nome != "Dev 1" || ma.Motivo != "Ferias" || ma.DiasAusente != 1 || ma.SprintInteira {
		t.Errorf("unexpected membro ausente: %+v", ma)
	}
}

func TestGetCapacity_RejeitadaExcluded(t *testing.T) {
	ctx := context.Background()
	sprintID := uuid.New()
	projetoID := uuid.New()
	fonteDadosID := uuid.New()
	membroID := uuid.New()

	inicio := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	fim := time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC)

	repo := &mockSprintRepoStore{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
			return &domain.Sprint{
				ID: sprintID, FonteDadosID: fonteDadosID, ProjetoID: &projetoID,
				Nome: "Sprint R", DataInicio: &inicio, DataFim: &fim,
			}, nil
		},
		getProjetoChaveFn: func(ctx context.Context, pid uuid.UUID) (string, error) {
			return "PROJ", nil
		},
		getFeriadosNoPeriodoFn: func(ctx context.Context, i, f time.Time) ([]repository.FeriadoRecord, error) {
			return nil, nil
		},
		getMembrosFromSprintFn: func(ctx context.Context, sid uuid.UUID) ([]repository.MembroInfo, error) {
			return []repository.MembroInfo{{ID: membroID, Nome: "Dev"}}, nil
		},
		getTarefasDetailBySprintFn: func(ctx context.Context, sid uuid.UUID) ([]repository.TarefaDetail, error) {
			return []repository.TarefaDetail{
				{
					ID: uuid.New(), NumeroTicket: "P-1", Resumo: "Valid",
					Tipo: "Story", Status: "Desenvolvimento", Segundos: 4 * 3600,
					ProjetoID: projetoID, ProjetoChave: "PROJ", ProjetoNome: "P",
					ResponsavelID: membroID,
				},
				{
					ID: uuid.New(), NumeroTicket: "P-2", Resumo: "Rejeitada Task",
					Tipo: "Bug", Status: "Rejeitada", Segundos: 8 * 3600,
					ProjetoID: projetoID, ProjetoChave: "PROJ", ProjetoNome: "P",
					ResponsavelID: membroID,
				},
				{
					ID: uuid.New(), NumeroTicket: "P-3", Resumo: "Cancelada Task",
					Tipo: "Bug", Status: "Cancelado", Segundos: 2 * 3600,
					ProjetoID: projetoID, ProjetoChave: "PROJ", ProjetoNome: "P",
					ResponsavelID: membroID,
				},
			}, nil
		},
		getAusenciasNoPeriodoFn: func(ctx context.Context, mids []uuid.UUID, i, f time.Time) ([]repository.AusenciaRecord, error) {
			return nil, nil
		},
	}

	svc := NewSprintService(repo, zap.NewNop())
	result, err := svc.GetCapacity(ctx, sprintID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Membros) != 1 {
		t.Fatalf("expected 1 membro, got %d", len(result.Membros))
	}
	mc := result.Membros[0]

	// Only the "Desenvolvimento" task (4h) should count. Rejeitada and Cancelado excluded.
	if len(mc.Tarefas) != 1 {
		t.Fatalf("expected 1 tarefa (Rejeitada+Cancelado excluded), got %d", len(mc.Tarefas))
	}
	if mc.Tarefas[0].Status == "Rejeitada" || mc.Tarefas[0].Status == "Cancelado" {
		t.Errorf("excluded status appeared in Tarefas: %s", mc.Tarefas[0].Status)
	}
	if !floatsClose(mc.HorasAlocadas, 4.0) {
		t.Errorf("expected HorasAlocadas 4.0 (only valid task), got %v", mc.HorasAlocadas)
	}
	if !floatsClose(result.HorasAlocadas, 4.0) {
		t.Errorf("expected total HorasAlocadas 4.0, got %v", result.HorasAlocadas)
	}
}

func TestGetUnplannedAnalysis(t *testing.T) {
	ctx := context.Background()
	sprintID := uuid.New()

	t.Run("GetUnplannedStats error", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			getUnplannedStatsFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID) (*repository.UnplannedStats, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.GetUnplannedAnalysis(ctx, sprintID, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("happy path no projetoID", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			getUnplannedStatsFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID) (*repository.UnplannedStats, error) {
				if sid != sprintID {
					t.Errorf("unexpected sprintID %v", sid)
				}
				return &repository.UnplannedStats{
					TotalTarefas:         10,
					TarefasNaoPlanejadas: 3,
					HorasNaoPlanejadas:   12.34,
					HorasTotalSprint:     100.0,
					ManutencaoCount:      1,
					ManutencaoHoras:      4.0,
					OutrasCount:          2,
					OutrasHoras:          6.0,
				}, nil
			},
			getSprintProjetoIDFn: func(ctx context.Context, sid uuid.UUID) (*uuid.UUID, error) {
				return nil, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.GetUnplannedAnalysis(ctx, sprintID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.SprintAtual.TotalTarefas != 10 {
			t.Errorf("expected 10 total, got %d", result.SprintAtual.TotalTarefas)
		}
		if result.SprintAtual.TarefasNaoPlanejadas != 3 {
			t.Errorf("expected 3 nao planejadas, got %d", result.SprintAtual.TarefasNaoPlanejadas)
		}
		// pctNaoPlanejadas = round(3/10*1000)/10 = 30.0
		if !floatsClose(result.SprintAtual.PercentualNaoPlanejadas, 30.0) {
			t.Errorf("expected 30.0 pct, got %v", result.SprintAtual.PercentualNaoPlanejadas)
		}
		// No projetoID => MediaHistorica stays zero value.
		if result.MediaHistorica.SprintsAnalisadas != 0 {
			t.Errorf("expected 0 sprints analisadas, got %d", result.MediaHistorica.SprintsAnalisadas)
		}
		if result.EquipeNome != "" {
			t.Errorf("expected empty equipe nome, got %q", result.EquipeNome)
		}
	})

	t.Run("happy path with historical", func(t *testing.T) {
		projetoID := uuid.New()
		histSprintID := uuid.New()

		// Current sprint: Mon 2026-01-05 through Fri 2026-01-09 (5 weekdays, no holidays).
		curInicio := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
		curFim := time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC)

		// Historical sprint: same 5-weekday window, 2 membros => capacidade = 5*6*2 = 60.
		histInicio := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
		histFim := time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC)

		repo := &mockSprintRepoStore{
			getUnplannedStatsFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID) (*repository.UnplannedStats, error) {
				return &repository.UnplannedStats{TotalTarefas: 10, TarefasNaoPlanejadas: 3}, nil
			},
			getSprintProjetoIDFn: func(ctx context.Context, sid uuid.UUID) (*uuid.UUID, error) {
				return &projetoID, nil
			},
			getHistoricalUnplannedFn: func(ctx context.Context, pid uuid.UUID, eid *uuid.UUID, currentSprintID uuid.UUID, limit int) ([]repository.HistoricalUnplannedItem, error) {
				if pid != projetoID {
					t.Errorf("unexpected projetoID %v", pid)
				}
				if currentSprintID != sprintID {
					t.Errorf("unexpected currentSprintID %v", currentSprintID)
				}
				return []repository.HistoricalUnplannedItem{
					{
						SprintID:           histSprintID,
						SprintNome:         "Hist 1",
						HorasNaoPlanejadas: 10.0,
						HorasTotal:         100.0,
						TotalMembros:       2,
					},
				}, nil
			},
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
				if id == sprintID {
					return &domain.Sprint{ID: sprintID, Nome: "Current", DataInicio: &curInicio, DataFim: &curFim}, nil
				}
				if id == histSprintID {
					return &domain.Sprint{ID: histSprintID, Nome: "Hist 1", DataInicio: &histInicio, DataFim: &histFim}, nil
				}
				t.Errorf("unexpected GetByID call with %v", id)
				return nil, fmt.Errorf("unexpected id")
			},
			getFeriadosNoPeriodoFn: func(ctx context.Context, inicio, fim time.Time) ([]repository.FeriadoRecord, error) {
				return nil, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.GetUnplannedAnalysis(ctx, sprintID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.SprintAtual.TotalTarefas != 10 {
			t.Errorf("expected 10 total, got %d", result.SprintAtual.TotalTarefas)
		}
		if result.MediaHistorica.SprintsAnalisadas != 1 {
			t.Errorf("expected 1 sprint analisada, got %d", result.MediaHistorica.SprintsAnalisadas)
		}
		if !floatsClose(result.MediaHistorica.MediaHorasNaoPlanejadas, 10.0) {
			t.Errorf("expected media horas nao planejadas 10.0, got %v", result.MediaHistorica.MediaHorasNaoPlanejadas)
		}
		if !floatsClose(result.MediaHistorica.MediaPercentualNaoPlanejadas, 10.0) {
			t.Errorf("expected media percentual 10.0, got %v", result.MediaHistorica.MediaPercentualNaoPlanejadas)
		}
		if !floatsClose(result.MediaHistorica.CapacidadeMediaSprint, 60.0) {
			t.Errorf("expected capacidade media 60.0, got %v", result.MediaHistorica.CapacidadeMediaSprint)
		}
		// pctSugerido = round(100 - (10/60*100)) = round(83.33) = 83
		if !floatsClose(result.MediaHistorica.PercentualAlocacaoSugerido, 83.0) {
			t.Errorf("expected pct sugerido 83.0, got %v", result.MediaHistorica.PercentualAlocacaoSugerido)
		}
	})
}

func TestGetDisclaimerTasks(t *testing.T) {
	ctx := context.Background()
	sprintID := uuid.New()

	t.Run("error", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			getDisclaimerTasksFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID, tt string) ([]repository.DisclaimerTarefaRow, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.GetDisclaimerTasks(ctx, sprintID, nil, "unplanned")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("happy path", func(t *testing.T) {
		tarefaID := uuid.New()
		tipoDemanda := "Manutenção"
		relator := "Relator X"
		repo := &mockSprintRepoStore{
			getDisclaimerTasksFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID, tt string) ([]repository.DisclaimerTarefaRow, error) {
				if sid != sprintID {
					t.Errorf("unexpected sprintID %v", sid)
				}
				if tt != "unplanned" {
					t.Errorf("unexpected taskType %q", tt)
				}
				return []repository.DisclaimerTarefaRow{{
					ID:              tarefaID,
					NumeroTicket:    "T-1",
					Resumo:          "Task",
					Tipo:            "Bug",
					TipoDemanda:     &tipoDemanda,
					EstimativaTempo: 3600,
					RelatorNome:     &relator,
				}}, nil
			},
			getDisclaimerTarefaProdFn: func(ctx context.Context, tids []uuid.UUID) (map[uuid.UUID][]string, error) {
				if len(tids) != 1 || tids[0] != tarefaID {
					t.Errorf("unexpected tarefaIDs %v", tids)
				}
				return map[uuid.UUID][]string{tarefaID: {"Produto A"}}, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.GetDisclaimerTasks(ctx, sprintID, nil, "unplanned")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Tarefas) != 1 {
			t.Fatalf("expected 1 tarefa, got %d", len(result.Tarefas))
		}
		task := result.Tarefas[0]
		if task.NumeroTicket != "T-1" || task.Resumo != "Task" || task.Tipo != "Bug" {
			t.Errorf("unexpected task fields: %+v", task)
		}
		if task.TipoDemanda == nil || *task.TipoDemanda != "Manutenção" {
			t.Errorf("unexpected tipo demanda: %v", task.TipoDemanda)
		}
		if len(task.Produtos) != 1 || task.Produtos[0] != "Produto A" {
			t.Errorf("expected produtos [Produto A], got %v", task.Produtos)
		}
	})
}

func TestGetBurndown(t *testing.T) {
	ctx := context.Background()
	sprintID := uuid.New()

	t.Run("GetByID error", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.GetBurndown(ctx, sprintID, nil)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("sprint without dates", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
				return &domain.Sprint{ID: sprintID, Nome: "No Dates"}, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.GetBurndown(ctx, sprintID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.SprintNome != "No Dates" {
			t.Errorf("expected sprint nome 'No Dates', got %q", result.SprintNome)
		}
		if len(result.LinhaIdeal) != 0 || len(result.LinhaReal) != 0 || len(result.LinhaUnplanned) != 0 {
			t.Errorf("expected empty burndown lines, got %+v", result)
		}
	})

	t.Run("happy path", func(t *testing.T) {
		// Mon 2026-01-05 through Wed 2026-01-07 (3 weekdays, no holidays).
		inicio := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
		fim := time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC)

		repo := &mockSprintRepoStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
				return &domain.Sprint{ID: sprintID, Nome: "Sprint 1", DataInicio: &inicio, DataFim: &fim}, nil
			},
			getFeriadosNoPeriodoFn: func(ctx context.Context, i, f time.Time) ([]repository.FeriadoRecord, error) {
				return nil, nil
			},
			getBurndownTarefasFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID) ([]repository.BurndownTarefa, error) {
				if sid != sprintID {
					t.Errorf("unexpected sprintID %v", sid)
				}
				// Planned task (entered before/at sprint start), never resolved,
				// status not in the 80% discount set.
				return []repository.BurndownTarefa{
					{EstimativaSegundos: 8 * 3600, Status: "Desenvolvimento"},
				}, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.GetBurndown(ctx, sprintID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.SprintNome != "Sprint 1" {
			t.Errorf("expected sprint nome 'Sprint 1', got %q", result.SprintNome)
		}
		if result.DataInicio != "2026-01-05" || result.DataFim != "2026-01-07" {
			t.Errorf("unexpected dates: inicio=%q fim=%q", result.DataInicio, result.DataFim)
		}
		if !floatsClose(result.HorasTotal, 8.0) {
			t.Errorf("expected HorasTotal 8.0, got %v", result.HorasTotal)
		}
		if len(result.LinhaIdeal) != 3 {
			t.Fatalf("expected 3 dias uteis in LinhaIdeal, got %d", len(result.LinhaIdeal))
		}
		// decPerDay = 8/2 = 4 => 8, 4, 0
		if !floatsClose(result.LinhaIdeal[0].Horas, 8.0) {
			t.Errorf("expected ideal[0] 8.0, got %v", result.LinhaIdeal[0].Horas)
		}
		if !floatsClose(result.LinhaIdeal[1].Horas, 4.0) {
			t.Errorf("expected ideal[1] 4.0, got %v", result.LinhaIdeal[1].Horas)
		}
		if !floatsClose(result.LinhaIdeal[2].Horas, 0.0) {
			t.Errorf("expected ideal[2] 0.0, got %v", result.LinhaIdeal[2].Horas)
		}
		// Sprint dates are all in the past relative to test execution time, so
		// LinhaReal/LinhaUnplanned should cover all 3 dias uteis.
		if len(result.LinhaReal) != 3 {
			t.Fatalf("expected 3 dias uteis in LinhaReal, got %d", len(result.LinhaReal))
		}
		// Task never resolved and not in the 80%-discount status set => horas
		// restantes stay at 8.0 for every day.
		for i, p := range result.LinhaReal {
			if !floatsClose(p.Horas, 8.0) {
				t.Errorf("expected real[%d] 8.0, got %v", i, p.Horas)
			}
		}
		if len(result.LinhaUnplanned) != 3 {
			t.Fatalf("expected 3 dias uteis in LinhaUnplanned, got %d", len(result.LinhaUnplanned))
		}
		// Task entered at/before sprint start => it never counts as unplanned.
		for i, p := range result.LinhaUnplanned {
			if !floatsClose(p.Horas, 0.0) {
				t.Errorf("expected unplanned[%d] 0.0, got %v", i, p.Horas)
			}
		}
	})
}

func TestGetSprintsTimeline(t *testing.T) {
	ctx := context.Background()
	equipeID := uuid.New()

	t.Run("GetEquipeBoardID error", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			getEquipeBoardIDFn: func(ctx context.Context, eid uuid.UUID) (*int, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.GetSprintsTimeline(ctx, equipeID, 2026)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty sprints", func(t *testing.T) {
		// Sprint dates fall entirely within 2024, outside the requested 2025
		// window, so it must be filtered out before ever reaching membros.
		foraInicio := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		foraFim := time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC)
		repo := &mockSprintRepoStore{
			getEquipeBoardIDFn: func(ctx context.Context, eid uuid.UUID) (*int, error) {
				return nil, nil
			},
			listSprintsIncludeEmptyFn: func(ctx context.Context, eid *uuid.UUID, estado *string, boardID *int) ([]repository.SprintListItem, error) {
				return []repository.SprintListItem{
					{ID: uuid.New(), Nome: "Fora do Ano", DataInicio: &foraInicio, DataFim: &foraFim},
				}, nil
			},
			getAllMembrosEquipeFn: func(ctx context.Context, eid uuid.UUID) ([]repository.MembroInfo, error) {
				return []repository.MembroInfo{{ID: uuid.New(), Nome: "Dev 1"}}, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.GetSprintsTimeline(ctx, equipeID, 2025)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty result, got %d items", len(result))
		}
	})

	t.Run("happy path", func(t *testing.T) {
		sprintID := uuid.New()
		fonteDadosID := uuid.New()
		projetoChave := "PROJ"
		estado := "active"
		membroA := uuid.New()
		membroB := uuid.New()
		membroDesligado := uuid.New()

		// Mon 2026-01-05 through Fri 2026-01-09 (5 weekdays, no holidays).
		inicio := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
		fim := time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC)
		// Member desligado before the sprint ends => excluded entirely.
		desligamento := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		repo := &mockSprintRepoStore{
			getEquipeBoardIDFn: func(ctx context.Context, eid uuid.UUID) (*int, error) {
				boardID := 7
				return &boardID, nil
			},
			listSprintsIncludeEmptyFn: func(ctx context.Context, eid *uuid.UUID, estado2 *string, boardID *int) ([]repository.SprintListItem, error) {
				if boardID == nil || *boardID != 7 {
					t.Errorf("expected boardID 7, got %v", boardID)
				}
				return []repository.SprintListItem{
					{
						ID: sprintID, Nome: "Sprint 1", Estado: &estado,
						DataInicio: &inicio, DataFim: &fim,
						FonteDadosID: &fonteDadosID, ProjetoChave: &projetoChave,
					},
				}, nil
			},
			getAllMembrosEquipeFn: func(ctx context.Context, eid uuid.UUID) ([]repository.MembroInfo, error) {
				return []repository.MembroInfo{
					{ID: membroA, Nome: "Dev A"},
					{ID: membroB, Nome: "Dev B"},
					{ID: membroDesligado, Nome: "Dev Desligado", DataDesligamento: &desligamento},
				}, nil
			},
			getFeriadosNoPeriodoFn: func(ctx context.Context, i, f time.Time) ([]repository.FeriadoRecord, error) {
				return nil, nil
			},
			getAusenciasNoPeriodoFn: func(ctx context.Context, mids []uuid.UUID, i, f time.Time) ([]repository.AusenciaRecord, error) {
				if len(mids) != 3 {
					t.Errorf("expected 3 membroIDs, got %d", len(mids))
				}
				return []repository.AusenciaRecord{
					{MembroID: membroB, Tipo: "Ferias", DataInicio: inicio, DataFim: inicio},
				}, nil
			},
			getHorasAlocadasPorSprintFn: func(ctx context.Context, sids, mids []uuid.UUID) (map[uuid.UUID]float64, error) {
				if len(sids) != 1 || sids[0] != sprintID {
					t.Errorf("unexpected sprintIDs %v", sids)
				}
				return map[uuid.UUID]float64{sprintID: 45.34}, nil
			},
		}

		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.GetSprintsTimeline(ctx, equipeID, 2026)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 item, got %d", len(result))
		}
		item := result[0]
		if item.SprintID != sprintID {
			t.Errorf("expected sprintID %v, got %v", sprintID, item.SprintID)
		}
		if item.SprintNome != "Sprint 1" {
			t.Errorf("expected nome 'Sprint 1', got %q", item.SprintNome)
		}
		if item.DataInicio != "2026-01-05" || item.DataFim != "2026-01-09" {
			t.Errorf("unexpected dates: inicio=%q fim=%q", item.DataInicio, item.DataFim)
		}
		if item.Estado != "active" {
			t.Errorf("expected estado 'active', got %q", item.Estado)
		}
		if item.FonteDadosID != fonteDadosID.String() {
			t.Errorf("expected fonteDadosID %q, got %q", fonteDadosID.String(), item.FonteDadosID)
		}
		if item.ProjetoChave != "PROJ" {
			t.Errorf("expected projetoChave 'PROJ', got %q", item.ProjetoChave)
		}
		// membroDesligado excluded entirely; membroA has full 5 dias uteis
		// (30h capacidade), membroB loses 1 dia to ausencia (4 dias => 24h).
		if item.Headcount != 2 {
			t.Errorf("expected headcount 2, got %d", item.Headcount)
		}
		if !floatsClose(item.HorasCapacidade, 54.0) {
			t.Errorf("expected HorasCapacidade 54.0, got %v", item.HorasCapacidade)
		}
		if !floatsClose(item.HorasMaximoTeorico, 60.0) {
			t.Errorf("expected HorasMaximoTeorico 60.0, got %v", item.HorasMaximoTeorico)
		}
		if !floatsClose(item.HorasAlocadas, 45.3) {
			t.Errorf("expected HorasAlocadas 45.3, got %v", item.HorasAlocadas)
		}
	})
}

func TestGetTimelineDetail(t *testing.T) {
	ctx := context.Background()
	sprintID := uuid.New()
	equipeID := uuid.New()

	t.Run("GetByID error", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.GetTimelineDetail(ctx, sprintID, equipeID)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("sprint without dates", func(t *testing.T) {
		repo := &mockSprintRepoStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
				return &domain.Sprint{ID: sprintID, Nome: "No Dates"}, nil
			},
		}
		svc := NewSprintService(repo, zap.NewNop())
		_, err := svc.GetTimelineDetail(ctx, sprintID, equipeID)
		if err == nil {
			t.Fatal("expected error")
		}
		if err.Error() != "sprint has no start/end date" {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("happy path", func(t *testing.T) {
		membroID := uuid.New()

		// Mon 2026-01-05 through Fri 2026-01-09.
		inicio := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
		fim := time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC)
		// Ausencia starts before the sprint and ends inside it, so it must be
		// clamped to the sprint's DataInicio in the result.
		ausenciaInicio := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
		ausenciaFim := time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)

		epico := "Epico X"
		relator := "Relator Y"

		repo := &mockSprintRepoStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
				if id != sprintID {
					t.Errorf("unexpected sprintID %v", id)
				}
				return &domain.Sprint{ID: sprintID, Nome: "Sprint 1", DataInicio: &inicio, DataFim: &fim}, nil
			},
			getMembrosEquipeInfoFn: func(ctx context.Context, eid uuid.UUID, dataFim time.Time) ([]repository.MembroInfo, error) {
				if eid != equipeID {
					t.Errorf("unexpected equipeID %v", eid)
				}
				if dataFim != fim {
					t.Errorf("unexpected dataFim %v", dataFim)
				}
				return []repository.MembroInfo{{ID: membroID, Nome: "Dev 1"}}, nil
			},
			getAusenciasNoPeriodoFn: func(ctx context.Context, mids []uuid.UUID, i, f time.Time) ([]repository.AusenciaRecord, error) {
				if len(mids) != 1 || mids[0] != membroID {
					t.Errorf("unexpected membroIDs %v", mids)
				}
				// Duplicate record (same membro/tipo/data_inicio) must be deduped.
				dup := repository.AusenciaRecord{MembroID: membroID, Tipo: "Ferias", DataInicio: ausenciaInicio, DataFim: ausenciaFim}
				return []repository.AusenciaRecord{dup, dup}, nil
			},
			getTimelineDetailTarefasFn: func(ctx context.Context, sid, eid uuid.UUID) ([]repository.TimelineDetailTarefa, error) {
				if sid != sprintID || eid != equipeID {
					t.Errorf("unexpected sid=%v eid=%v", sid, eid)
				}
				return []repository.TimelineDetailTarefa{
					{
						NumeroTicket: "PROJ-1", Resumo: "Task 1", TipoDemanda: "Planejada",
						EstimativaTempo: 3600, EpicoApelido: &epico, RelatorNome: &relator,
					},
				}, nil
			},
		}

		svc := NewSprintService(repo, zap.NewNop())
		result, err := svc.GetTimelineDetail(ctx, sprintID, equipeID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.SprintNome != "Sprint 1" {
			t.Errorf("expected sprint nome 'Sprint 1', got %q", result.SprintNome)
		}
		if result.DataInicio != "2026-01-05" || result.DataFim != "2026-01-09" {
			t.Errorf("unexpected dates: inicio=%q fim=%q", result.DataInicio, result.DataFim)
		}

		// Deduped to a single ausente entry, clamped to sprint start.
		if len(result.Ausentes) != 1 {
			t.Fatalf("expected 1 ausente (deduped), got %d", len(result.Ausentes))
		}
		aus := result.Ausentes[0]
		if aus.Nome != "Dev 1" || aus.Tipo != "Ferias" {
			t.Errorf("unexpected ausente fields: %+v", aus)
		}
		if aus.DataInicio != "2026-01-05" {
			t.Errorf("expected ausente DataInicio clamped to '2026-01-05', got %q", aus.DataInicio)
		}
		if aus.DataFim != "2026-01-06" {
			t.Errorf("expected ausente DataFim '2026-01-06', got %q", aus.DataFim)
		}

		if len(result.Tarefas) != 1 {
			t.Fatalf("expected 1 tarefa, got %d", len(result.Tarefas))
		}
		tarefa := result.Tarefas[0]
		if tarefa.NumeroTicket != "PROJ-1" || tarefa.Resumo != "Task 1" || tarefa.TipoDemanda != "Planejada" {
			t.Errorf("unexpected tarefa fields: %+v", tarefa)
		}
		if tarefa.EstimativaTempo != 3600 {
			t.Errorf("expected EstimativaTempo 3600, got %d", tarefa.EstimativaTempo)
		}
		if tarefa.EpicoApelido == nil || *tarefa.EpicoApelido != "Epico X" {
			t.Errorf("unexpected EpicoApelido: %v", tarefa.EpicoApelido)
		}
		if tarefa.RelatorNome == nil || *tarefa.RelatorNome != "Relator Y" {
			t.Errorf("unexpected RelatorNome: %v", tarefa.RelatorNome)
		}
	})
}
