package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TestListProjectAllocations_ComputesMetrics(t *testing.T) {
	svc := &AllocationService{}
	_, err := svc.ListProjectAllocations(context.Background(), uuid.New(), nil, "ativos")
	if err == nil {
		t.Fatal("expected error from nil repo, got nil")
	}
}

func TestListProjectAllocations_HappyPath(t *testing.T) {
	ctx := context.Background()
	equipeID := uuid.New()
	epicID := uuid.New()

	repo := &mockAllocationStore{
		getEpicsByEquipeAndProdutoFn: func(ctx context.Context, eq uuid.UUID, produtoNomes []string, statusFilter string) ([]repository.EpicAllocationRow, error) {
			return []repository.EpicAllocationRow{
				{
					EpicID:              epicID,
					NumeroTicket:        "PROJ-1",
					Resumo:              "Epic 1",
					TotalFilhas:         10,
					FilhasComEstimativa: 8,
					HorasEstimadas:      100,
					HorasEmSprint:       50,
					FilhasConcluidas:    2,
					FilhasEmAndamento:   3,
				},
			}, nil
		},
		checkGDPTCAncestorsFn: func(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
			return map[uuid.UUID]bool{epicID: true}, nil
		},
		getClosedEpicIDsFn: func(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
			return map[uuid.UUID]bool{}, nil
		},
	}

	svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	result, err := svc.ListProjectAllocations(ctx, equipeID, nil, "ativos")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	pa := result[0]
	if pa.PctEstimado != 80 {
		t.Errorf("expected PctEstimado=80, got %v", pa.PctEstimado)
	}
	if pa.PctPlanejado != 50 {
		t.Errorf("expected PctPlanejado=50, got %v", pa.PctPlanejado)
	}
	if pa.Status != "em_andamento" {
		t.Errorf("expected status=em_andamento, got %q", pa.Status)
	}
	if !pa.IsGDPTC {
		t.Errorf("expected IsGDPTC=true")
	}
	if pa.Encerrado {
		t.Errorf("expected Encerrado=false")
	}
	if pa.TarefasSemEst != 2 {
		t.Errorf("expected TarefasSemEst=2, got %d", pa.TarefasSemEst)
	}
}

func TestListProjectAllocations_Closed(t *testing.T) {
	ctx := context.Background()
	equipeID := uuid.New()
	epicID := uuid.New()
	closureDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	repo := &mockAllocationStore{
		getEpicsByEquipeAndProdutoFn: func(ctx context.Context, eq uuid.UUID, produtoNomes []string, statusFilter string) ([]repository.EpicAllocationRow, error) {
			return []repository.EpicAllocationRow{
				{EpicID: epicID, NumeroTicket: "PROJ-2", TotalFilhas: 5, FilhasComEstimativa: 5, FilhasConcluidas: 5},
			}, nil
		},
		checkGDPTCAncestorsFn: func(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
			return map[uuid.UUID]bool{}, nil
		},
		getClosedEpicIDsFn: func(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
			return map[uuid.UUID]bool{epicID: true}, nil
		},
		getProjectClosureFn: func(ctx context.Context, id uuid.UUID) (*repository.ProjectClosureRow, error) {
			return &repository.ProjectClosureRow{
				Descricao:        "done",
				DataEncerramento: closureDate,
				EncerradoPor:     "user@example.com",
			}, nil
		},
	}

	svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	result, err := svc.ListProjectAllocations(ctx, equipeID, nil, "todos")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	pa := result[0]
	if pa.Status != "desconsiderado" {
		t.Errorf("expected status=desconsiderado, got %q", pa.Status)
	}
	if !pa.Encerrado {
		t.Errorf("expected Encerrado=true")
	}
	if pa.Encerramento == nil {
		t.Fatalf("expected Encerramento to be set")
	}
	if pa.Encerramento.Descricao != "done" || pa.Encerramento.EncerradoPor != "user@example.com" {
		t.Errorf("unexpected closure fields: %+v", pa.Encerramento)
	}
}

func TestGetProjectDetail(t *testing.T) {
	ctx := context.Background()
	equipeID := uuid.New()
	epicID := uuid.New()

	t.Run("GetEpicByID error", func(t *testing.T) {
		repo := &mockAllocationStore{
			getEpicByIDFn: func(ctx context.Context, id uuid.UUID) (*repository.EpicAllocationRow, error) {
				return nil, fmt.Errorf("not found")
			},
		}
		svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.GetProjectDetail(ctx, epicID, equipeID)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("happy path", func(t *testing.T) {
		doneCat := "done"
		notDoneCat := "in_progress"
		sprintID := uuid.New()
		person1 := uuid.New()
		est3600 := 3600 // 1 hour

		tasks := []repository.TaskAllocationRow{
			// naoAlocadas: no estimate, not done
			{TarefaID: uuid.New(), NumeroTicket: "T-1", StatusCategoria: &notDoneCat},
			// concluidas: no estimate, done
			{TarefaID: uuid.New(), NumeroTicket: "T-2", StatusCategoria: &doneCat},
			// naoAlocadas: has estimate but no sprint, not done
			{TarefaID: uuid.New(), NumeroTicket: "T-3", EstimativaTempo: &est3600, StatusCategoria: &notDoneCat},
			// parciais: has estimate+sprint but no person, not done
			{TarefaID: uuid.New(), NumeroTicket: "T-4", EstimativaTempo: &est3600, SprintID: &sprintID, StatusCategoria: &notDoneCat},
			// completas: has estimate+sprint+person, not done
			{TarefaID: uuid.New(), NumeroTicket: "T-5", EstimativaTempo: &est3600, SprintID: &sprintID, ResponsavelID: &person1, StatusCategoria: &notDoneCat},
			// concluidas: has estimate+sprint+person, done
			{TarefaID: uuid.New(), NumeroTicket: "T-6", EstimativaTempo: &est3600, SprintID: &sprintID, ResponsavelID: &person1, StatusCategoria: &doneCat},
		}

		repo := &mockAllocationStore{
			getEpicByIDFn: func(ctx context.Context, id uuid.UUID) (*repository.EpicAllocationRow, error) {
				return &repository.EpicAllocationRow{
					EpicID:              epicID,
					NumeroTicket:        "PROJ-1",
					Resumo:              "Epic",
					TotalFilhas:         6,
					FilhasComEstimativa: 4,
					HorasEstimadas:      10,
					HorasEmSprint:       5,
					FilhasConcluidas:    2,
					FilhasEmAndamento:   1,
				}, nil
			},
			getEpicTasksFn: func(ctx context.Context, id uuid.UUID) ([]repository.TaskAllocationRow, error) {
				return tasks, nil
			},
			getEpicPeopleFn: func(ctx context.Context, id uuid.UUID) ([]repository.PersonAllocationRow, error) {
				return []repository.PersonAllocationRow{
					{MembroID: person1, Nome: "Alice", HorasNoProjeto: 2},
				}, nil
			},
			checkGDPTCAncestorsFn: func(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
				return map[uuid.UUID]bool{epicID: true}, nil
			},
		}

		svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		detail, err := svc.GetProjectDetail(ctx, epicID, equipeID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(detail.NaoAlocadas) != 2 {
			t.Errorf("expected 2 naoAlocadas, got %d", len(detail.NaoAlocadas))
		}
		if len(detail.Parciais) != 1 {
			t.Errorf("expected 1 parciais, got %d", len(detail.Parciais))
		}
		if len(detail.Completas) != 1 {
			t.Errorf("expected 1 completas, got %d", len(detail.Completas))
		}
		if len(detail.Concluidas) != 2 {
			t.Errorf("expected 2 concluidas, got %d", len(detail.Concluidas))
		}
		if !detail.Epic.IsGDPTC {
			t.Errorf("expected epic IsGDPTC=true")
		}
		if len(detail.Pessoas) != 1 {
			t.Fatalf("expected 1 pessoa, got %d", len(detail.Pessoas))
		}
		if detail.Pessoas[0].Nome != "Alice" {
			t.Errorf("expected Alice, got %q", detail.Pessoas[0].Nome)
		}
	})
}

func TestGetAvailableSprints(t *testing.T) {
	ctx := context.Background()
	equipeID := uuid.New()

	t.Run("error", func(t *testing.T) {
		repo := &mockAllocationStore{
			getFutureSprintsByEquipeFn: func(ctx context.Context, eq uuid.UUID) ([]repository.SprintOptionRow, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.GetAvailableSprints(ctx, equipeID)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("happy path", func(t *testing.T) {
		sprintID := uuid.New()
		inicio := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		fim := time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)
		repo := &mockAllocationStore{
			getFutureSprintsByEquipeFn: func(ctx context.Context, eq uuid.UUID) ([]repository.SprintOptionRow, error) {
				return []repository.SprintOptionRow{
					{ID: sprintID, JiraID: 42, Nome: "Sprint 1", Inicio: inicio, Fim: fim, Estado: "future"},
				}, nil
			},
		}
		svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		result, err := svc.GetAvailableSprints(ctx, equipeID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result))
		}
		if result[0].ID != sprintID || result[0].JiraID != 42 || result[0].Nome != "Sprint 1" || result[0].Estado != "future" {
			t.Errorf("unexpected result: %+v", result[0])
		}
		if !result[0].Inicio.Equal(inicio) || !result[0].Fim.Equal(fim) {
			t.Errorf("unexpected dates: %+v", result[0])
		}
	})
}

func TestCloseProject(t *testing.T) {
	ctx := context.Background()
	epicID := uuid.New()

	t.Run("error - bad date", func(t *testing.T) {
		svc := NewAllocationService(&mockAllocationStore{}, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		err := svc.CloseProject(ctx, epicID, CloseProjectRequest{Descricao: "x", DataEncerramento: "not-a-date"}, "user@example.com")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("happy path", func(t *testing.T) {
		var gotEpicID uuid.UUID
		var gotDescricao, gotEncerradoPor string
		var gotDate time.Time
		repo := &mockAllocationStore{
			closeProjectFn: func(ctx context.Context, epicID uuid.UUID, descricao string, dataEncerramento time.Time, encerradoPor string) error {
				gotEpicID = epicID
				gotDescricao = descricao
				gotDate = dataEncerramento
				gotEncerradoPor = encerradoPor
				return nil
			},
		}
		svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		err := svc.CloseProject(ctx, epicID, CloseProjectRequest{Descricao: "finalizado", DataEncerramento: "2026-02-15"}, "user@example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotEpicID != epicID {
			t.Errorf("expected epicID=%v, got %v", epicID, gotEpicID)
		}
		if gotDescricao != "finalizado" {
			t.Errorf("expected descricao=finalizado, got %q", gotDescricao)
		}
		if gotEncerradoPor != "user@example.com" {
			t.Errorf("expected encerradoPor=user@example.com, got %q", gotEncerradoPor)
		}
		expectedDate := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
		if !gotDate.Equal(expectedDate) {
			t.Errorf("expected date=%v, got %v", expectedDate, gotDate)
		}
	})
}

func TestReopenProject(t *testing.T) {
	ctx := context.Background()
	epicID := uuid.New()

	t.Run("error", func(t *testing.T) {
		repo := &mockAllocationStore{
			reopenProjectFn: func(ctx context.Context, id uuid.UUID) error {
				return fmt.Errorf("db error")
			},
		}
		svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		err := svc.ReopenProject(ctx, epicID)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("happy path", func(t *testing.T) {
		var gotID uuid.UUID
		repo := &mockAllocationStore{
			reopenProjectFn: func(ctx context.Context, id uuid.UUID) error {
				gotID = id
				return nil
			},
		}
		svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		err := svc.ReopenProject(ctx, epicID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotID != epicID {
			t.Errorf("expected id=%v, got %v", epicID, gotID)
		}
	})
}

func TestAllocateTask(t *testing.T) {
	ctx := context.Background()
	taskID := uuid.New()
	sprintID := uuid.New()
	equipeID := uuid.New()

	t.Run("estimate must be positive", func(t *testing.T) {
		svc := NewAllocationService(&mockAllocationStore{}, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.AllocateTask(ctx, AllocateTaskRequest{
			TaskID: taskID, SprintID: sprintID, EquipeID: equipeID, EstimateHours: 0,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("GetTaskPreviousState error", func(t *testing.T) {
		repo := &mockAllocationStore{
			getTaskPreviousStateFn: func(ctx context.Context, id uuid.UUID) (*repository.TaskPreviousState, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		// Force=true skips the sprintSvc capacity check (sprintSvc is nil here).
		_, err := svc.AllocateTask(ctx, AllocateTaskRequest{
			TaskID: taskID, SprintID: sprintID, EquipeID: equipeID,
			EstimateHours: 8, Force: true,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("happy path no conflict", func(t *testing.T) {
		var gotSprintID uuid.UUID
		var gotEstimateSeconds int
		repo := &mockAllocationStore{
			getTaskPreviousStateFn: func(ctx context.Context, id uuid.UUID) (*repository.TaskPreviousState, error) {
				return &repository.TaskPreviousState{}, nil
			},
			updateTaskAllocationFn: func(ctx context.Context, tid, sid uuid.UUID, assigneeID *uuid.UUID, estimateSeconds int) error {
				gotSprintID = sid
				gotEstimateSeconds = estimateSeconds
				return nil
			},
			// The goroutine fired by AllocateTask calls GetTaskJiraKey first; make
			// it fail fast so the async writeToJira path exits quickly, and give
			// it a working rollback so it doesn't panic on a nil func field.
			getTaskJiraKeyFn: func(ctx context.Context, id uuid.UUID) (string, error) {
				return "", fmt.Errorf("no jira config")
			},
			rollbackTaskAllocationFn: func(ctx context.Context, id uuid.UUID, prev *repository.TaskPreviousState) error {
				return nil
			},
		}
		svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		// AssigneeID nil skips the capacity conflict check entirely.
		result, err := svc.AllocateTask(ctx, AllocateTaskRequest{
			TaskID: taskID, SprintID: sprintID, EquipeID: equipeID,
			EstimateHours: 8,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil || result.Conflict {
			t.Fatalf("expected non-conflict result, got %+v", result)
		}

		time.Sleep(50 * time.Millisecond) // let the async writeToJira goroutine finish

		if gotSprintID != sprintID {
			t.Errorf("expected sprintID=%v, got %v", sprintID, gotSprintID)
		}
		if gotEstimateSeconds != 8*3600 {
			t.Errorf("expected estimateSeconds=%d, got %d", 8*3600, gotEstimateSeconds)
		}
	})
}

func TestAllocateTask_Conflict(t *testing.T) {
	ctx := context.Background()
	taskID := uuid.New()
	sprintID := uuid.New()
	equipeID := uuid.New()
	membroID := uuid.New()

	// 8 weekdays (Mon 2026-01-05 .. Wed 2026-01-14), no holidays => 48h disponiveis.
	inicio := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	fim := time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)

	sprintRepo := &mockSprintRepoStore{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
			return &domain.Sprint{
				ID: sprintID, Nome: "Sprint 1",
				DataInicio: &inicio, DataFim: &fim,
				FonteDadosID: uuid.New(),
			}, nil
		},
		getFeriadosNoPeriodoFn: func(ctx context.Context, i, f time.Time) ([]repository.FeriadoRecord, error) {
			return nil, nil
		},
		getMembrosEquipeIDsFn: func(ctx context.Context, eid uuid.UUID, dataFim time.Time) (map[uuid.UUID]bool, error) {
			return map[uuid.UUID]bool{membroID: true}, nil
		},
		getMembrosEquipeInfoFn: func(ctx context.Context, eid uuid.UUID, dataFim time.Time) ([]repository.MembroInfo, error) {
			return []repository.MembroInfo{{ID: membroID, Nome: "Dev 1"}}, nil
		},
		getMembrosFromSprintFn: func(ctx context.Context, sid uuid.UUID) ([]repository.MembroInfo, error) {
			return []repository.MembroInfo{{ID: membroID, Nome: "Dev 1"}}, nil
		},
		getTarefasDetailBySprintFn: func(ctx context.Context, sid uuid.UUID) ([]repository.TarefaDetail, error) {
			// 50h already allocated to the member (pending status), against 48h available.
			return []repository.TarefaDetail{
				{
					ID: uuid.New(), NumeroTicket: "PROJ-1", Resumo: "Existing",
					Tipo: "Story", Status: "Desenvolvimento", Segundos: 50 * 3600,
					ResponsavelID: membroID,
				},
			}, nil
		},
		getAusenciasNoPeriodoFn: func(ctx context.Context, mids []uuid.UUID, i, f time.Time) ([]repository.AusenciaRecord, error) {
			return nil, nil
		},
	}
	sprintSvc := NewSprintService(sprintRepo, zap.NewNop())

	svc := NewAllocationService(&mockAllocationStore{}, sprintSvc, sprintRepo, nil, nil, nil, nil, nil, 0, zap.NewNop())

	result, err := svc.AllocateTask(ctx, AllocateTaskRequest{
		TaskID: taskID, SprintID: sprintID, EquipeID: equipeID,
		AssigneeID: &membroID, EstimateHours: 8, Force: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.Conflict {
		t.Fatalf("expected conflict, got %+v", result)
	}
	if result.MembroNome != "Dev 1" {
		t.Errorf("expected membro nome 'Dev 1', got %q", result.MembroNome)
	}
	if result.SprintNome != "Sprint 1" {
		t.Errorf("expected sprint nome 'Sprint 1', got %q", result.SprintNome)
	}
	if result.PctAtual <= 100 {
		t.Errorf("expected PctAtual > 100, got %v", result.PctAtual)
	}
}

func TestSyncProjectTasks(t *testing.T) {
	ctx := context.Background()
	epicID := uuid.New()

	t.Run("GetTaskJiraKey error", func(t *testing.T) {
		repo := &mockAllocationStore{
			getTaskJiraKeyFn: func(ctx context.Context, id uuid.UUID) (string, error) {
				return "", fmt.Errorf("not found")
			},
		}
		svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncProjectTasks(ctx, epicID)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("GetTaskFonteDadosID error", func(t *testing.T) {
		repo := &mockAllocationStore{
			getTaskJiraKeyFn: func(ctx context.Context, id uuid.UUID) (string, error) {
				return "PROJ-1", nil
			},
			getTaskFonteDadosIDFn: func(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
				return uuid.Nil, fmt.Errorf("db error")
			},
		}
		svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncProjectTasks(ctx, epicID)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("syncSvc nil", func(t *testing.T) {
		repo := &mockAllocationStore{
			getTaskJiraKeyFn: func(ctx context.Context, id uuid.UUID) (string, error) {
				return "PROJ-1", nil
			},
			getTaskFonteDadosIDFn: func(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
				return uuid.New(), nil
			},
		}
		svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.SyncProjectTasks(ctx, epicID)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestGetFilteredProducts(t *testing.T) {
	ctx := context.Background()

	t.Run("error", func(t *testing.T) {
		repo := &mockAllocationStore{
			getProdutosComProjetosAtvFn: func(ctx context.Context) ([]repository.ProdutoRow, error) {
				return nil, fmt.Errorf("db error")
			},
		}
		svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		_, err := svc.GetFilteredProducts(ctx)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("happy path", func(t *testing.T) {
		produtoID := uuid.New()
		repo := &mockAllocationStore{
			getProdutosComProjetosAtvFn: func(ctx context.Context) ([]repository.ProdutoRow, error) {
				return []repository.ProdutoRow{{ID: produtoID, Nome: "Produto X"}}, nil
			},
		}
		svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
		result, err := svc.GetFilteredProducts(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 || result[0].ID != produtoID || result[0].Nome != "Produto X" {
			t.Errorf("unexpected result: %+v", result)
		}
	})
}

func TestComputeProjectStatus(t *testing.T) {
	tests := []struct {
		name                                             string
		encerrado                                        bool
		totalFilhas, filhasConcluidas, filhasEmAndamento int
		want                                             string
	}{
		{"encerrado", true, 5, 3, 2, "desconsiderado"},
		{"no tasks", false, 0, 0, 0, "nao_iniciado"},
		{"all done", false, 5, 5, 0, "concluido"},
		{"in progress", false, 5, 2, 3, "em_andamento"},
		{"has tasks but none in progress or done", false, 5, 0, 0, "nao_iniciado"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeProjectStatus(tc.encerrado, tc.totalFilhas, tc.filhasConcluidas, tc.filhasEmAndamento)
			if got != tc.want {
				t.Errorf("computeProjectStatus(%v, %d, %d, %d) = %q, want %q",
					tc.encerrado, tc.totalFilhas, tc.filhasConcluidas, tc.filhasEmAndamento, got, tc.want)
			}
		})
	}
}

func TestListProjectAllocations_PctPlanejadoCap(t *testing.T) {
	epicID := uuid.New()
	equipeID := uuid.New()

	repo := &mockAllocationStore{
		getEpicsByEquipeAndProdutoFn: func(ctx context.Context, eqID uuid.UUID, produtos []string, status string) ([]repository.EpicAllocationRow, error) {
			return []repository.EpicAllocationRow{{
				EpicID:              epicID,
				NumeroTicket:        "PROJ-100",
				Resumo:              "Test epic",
				TotalFilhas:         10,
				FilhasComEstimativa: 8,
				FilhasConcluidas:    3,
				FilhasEmAndamento:   5,
				HorasEstimadas:      50.0,
				HorasEmSprint:       80.0, // > HorasEstimadas → pctPlanejado > 100 → should cap at 100
			}}, nil
		},
		checkGDPTCAncestorsFn: func(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
			return map[uuid.UUID]bool{epicID: true}, nil
		},
		getClosedEpicIDsFn: func(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
			return map[uuid.UUID]bool{}, nil
		},
	}

	svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	result, err := svc.ListProjectAllocations(context.Background(), equipeID, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].PctPlanejado > 100 {
		t.Errorf("pctPlanejado should be capped at 100, got %f", result[0].PctPlanejado)
	}
	if !result[0].IsGDPTC {
		t.Error("expected IsGDPTC = true")
	}
}

func TestListProjectAllocations_GDPTCError(t *testing.T) {
	equipeID := uuid.New()

	repo := &mockAllocationStore{
		getEpicsByEquipeAndProdutoFn: func(ctx context.Context, eqID uuid.UUID, produtos []string, status string) ([]repository.EpicAllocationRow, error) {
			return []repository.EpicAllocationRow{{
				EpicID:            uuid.New(),
				NumeroTicket:      "PROJ-200",
				Resumo:            "Test",
				TotalFilhas:       3,
				FilhasEmAndamento: 1,
			}}, nil
		},
		checkGDPTCAncestorsFn: func(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
			return nil, fmt.Errorf("db error")
		},
		getClosedEpicIDsFn: func(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
			return nil, fmt.Errorf("db error")
		},
	}

	svc := NewAllocationService(repo, nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	result, err := svc.ListProjectAllocations(context.Background(), equipeID, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}
