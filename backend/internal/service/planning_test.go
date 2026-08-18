package service

import (
	"context"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockPlanningRepoStore struct {
	getNextSprintFn           func(ctx context.Context, boardID int, currentDataInicio time.Time) (*domain.Sprint, error)
	getAllTarefasBySprintFn   func(ctx context.Context, sprintID uuid.UUID) ([]repository.PlanningTarefa, error)
	updateTarefaEstimativaFn  func(ctx context.Context, tarefaID uuid.UUID, segundos int) error
	updateTarefaTipoDemandaFn func(ctx context.Context, tarefaID uuid.UUID, valor string) error
	updateTarefaResponsavelFn func(ctx context.Context, tarefaID uuid.UUID, responsavelID *uuid.UUID) error
	moveTarefaToSprintFn      func(ctx context.Context, tarefaID uuid.UUID, sprintID uuid.UUID) error
	removeTarefaFromSprintFn  func(ctx context.Context, tarefaID uuid.UUID) error
	getSprintJiraIDFn         func(ctx context.Context, sprintID uuid.UUID) (int, error)
}

func (m *mockPlanningRepoStore) GetNextSprint(ctx context.Context, boardID int, currentDataInicio time.Time) (*domain.Sprint, error) {
	return m.getNextSprintFn(ctx, boardID, currentDataInicio)
}
func (m *mockPlanningRepoStore) GetAllTarefasBySprint(ctx context.Context, sprintID uuid.UUID) ([]repository.PlanningTarefa, error) {
	return m.getAllTarefasBySprintFn(ctx, sprintID)
}
func (m *mockPlanningRepoStore) UpdateTarefaEstimativa(ctx context.Context, tarefaID uuid.UUID, segundos int) error {
	if m.updateTarefaEstimativaFn != nil {
		return m.updateTarefaEstimativaFn(ctx, tarefaID, segundos)
	}
	return nil
}
func (m *mockPlanningRepoStore) UpdateTarefaTipoDemanda(ctx context.Context, tarefaID uuid.UUID, valor string) error {
	if m.updateTarefaTipoDemandaFn != nil {
		return m.updateTarefaTipoDemandaFn(ctx, tarefaID, valor)
	}
	return nil
}
func (m *mockPlanningRepoStore) UpdateTarefaResponsavel(ctx context.Context, tarefaID uuid.UUID, responsavelID *uuid.UUID) error {
	if m.updateTarefaResponsavelFn != nil {
		return m.updateTarefaResponsavelFn(ctx, tarefaID, responsavelID)
	}
	return nil
}
func (m *mockPlanningRepoStore) MoveTarefaToSprint(ctx context.Context, tarefaID uuid.UUID, sprintID uuid.UUID) error {
	if m.moveTarefaToSprintFn != nil {
		return m.moveTarefaToSprintFn(ctx, tarefaID, sprintID)
	}
	return nil
}
func (m *mockPlanningRepoStore) RemoveTarefaFromSprint(ctx context.Context, tarefaID uuid.UUID) error {
	if m.removeTarefaFromSprintFn != nil {
		return m.removeTarefaFromSprintFn(ctx, tarefaID)
	}
	return nil
}
func (m *mockPlanningRepoStore) GetSprintJiraID(ctx context.Context, sprintID uuid.UUID) (int, error) {
	if m.getSprintJiraIDFn != nil {
		return m.getSprintJiraIDFn(ctx, sprintID)
	}
	return 0, nil
}
func (m *mockPlanningRepoStore) SearchTarefasByKeys(ctx context.Context, projetoID uuid.UUID, keys []string) ([]repository.SearchTarefaResult, error) {
	return nil, nil
}
func (m *mockPlanningRepoStore) UpsertTarefaFromJira(ctx context.Context, t *repository.UpsertTarefaParams) (uuid.UUID, error) {
	return uuid.Nil, nil
}
func (m *mockPlanningRepoStore) MoveTarefasToSprint(ctx context.Context, sprintID uuid.UUID, tarefaIDs []uuid.UUID) error {
	return nil
}
func (m *mockPlanningRepoStore) GetTarefasByIDs(ctx context.Context, ids []uuid.UUID) ([]repository.PlanningTarefa, error) {
	return nil, nil
}
func (m *mockPlanningRepoStore) GetProjetoChaveByID(ctx context.Context, projetoID uuid.UUID) (string, uuid.UUID, error) {
	return "", uuid.Nil, nil
}

func TestPlanningService_GetNextSprint(t *testing.T) {
	boardID := 42
	now := time.Now()
	currentSprintID := uuid.New()
	nextSprintID := uuid.New()
	estado := "active"
	estadoFuture := "future"
	tarefaID := uuid.New()

	sprintRepo := &mockSprintRepoStore{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
			return &domain.Sprint{
				ID:         currentSprintID,
				BoardID:    &boardID,
				Estado:     &estado,
				DataInicio: &now,
				DataFim:    func() *time.Time { t := now.Add(14 * 24 * time.Hour); return &t }(),
			}, nil
		},
	}

	planRepo := &mockPlanningRepoStore{
		getNextSprintFn: func(ctx context.Context, bID int, di time.Time) (*domain.Sprint, error) {
			if bID != boardID {
				t.Errorf("expected boardID %d, got %d", boardID, bID)
			}
			nextStart := now.Add(15 * 24 * time.Hour)
			nextEnd := now.Add(29 * 24 * time.Hour)
			return &domain.Sprint{
				ID:         nextSprintID,
				BoardID:    &boardID,
				Estado:     &estadoFuture,
				Nome:       "Sprint 11",
				DataInicio: &nextStart,
				DataFim:    &nextEnd,
			}, nil
		},
		getAllTarefasBySprintFn: func(ctx context.Context, sprintID uuid.UUID) ([]repository.PlanningTarefa, error) {
			if sprintID != nextSprintID {
				t.Errorf("expected nextSprintID, got %s", sprintID)
			}
			return []repository.PlanningTarefa{
				{ID: tarefaID, NumeroTicket: "PROJ-101", Resumo: "Test task", Tipo: "Story", Status: "Backlog"},
			}, nil
		},
	}

	svc := NewPlanningService(planRepo, sprintRepo, nil, nil, nil, nil, 5, zap.NewNop())
	result, err := svc.GetNextSprint(context.Background(), currentSprintID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Sprint.ID != nextSprintID {
		t.Errorf("expected sprint ID %s, got %s", nextSprintID, result.Sprint.ID)
	}
	if len(result.Tarefas) != 1 {
		t.Fatalf("expected 1 tarefa, got %d", len(result.Tarefas))
	}
	if result.Tarefas[0].NumeroTicket != "PROJ-101" {
		t.Errorf("expected PROJ-101, got %s", result.Tarefas[0].NumeroTicket)
	}
}

func TestPlanningService_GetNextSprint_NoNext(t *testing.T) {
	boardID := 42
	now := time.Now()
	currentSprintID := uuid.New()
	estado := "active"

	sprintRepo := &mockSprintRepoStore{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
			return &domain.Sprint{
				ID:         currentSprintID,
				BoardID:    &boardID,
				Estado:     &estado,
				DataInicio: &now,
			}, nil
		},
	}

	planRepo := &mockPlanningRepoStore{
		getNextSprintFn: func(ctx context.Context, bID int, di time.Time) (*domain.Sprint, error) {
			return nil, nil
		},
	}

	svc := NewPlanningService(planRepo, sprintRepo, nil, nil, nil, nil, 5, zap.NewNop())
	result, err := svc.GetNextSprint(context.Background(), currentSprintID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result when no next sprint, got %+v", result)
	}
}

func TestPlanningService_BuildOperations(t *testing.T) {
	svc := &PlanningService{}
	changes := PlanningChanges{
		Reassigned:      []ReassignChange{{TarefaKey: "P-1"}},
		Estimated:       []EstimateChange{{TarefaKey: "P-2"}},
		TipoDemanda:     []TipoDemandaChange{{TarefaKey: "P-3"}},
		MovedNextSprint: []MoveSprintChange{{TarefaKey: "P-4"}},
		MovedBacklog:    []MoveBacklogChange{{TarefaKey: "P-5"}},
	}
	ops := svc.buildOperations(changes)
	if len(ops) != 5 {
		t.Fatalf("expected 5 operations, got %d", len(ops))
	}
	expected := []struct{ key, action string }{
		{"P-1", "assign"}, {"P-2", "estimate"}, {"P-3", "tipo_demanda"},
		{"P-4", "move_sprint"}, {"P-5", "move_backlog"},
	}
	for i, e := range expected {
		if ops[i].Key != e.key || ops[i].Action != e.action {
			t.Errorf("op[%d]: expected %s/%s, got %s/%s", i, e.key, e.action, ops[i].Key, ops[i].Action)
		}
		if ops[i].Status != "pending" {
			t.Errorf("op[%d]: expected pending, got %s", i, ops[i].Status)
		}
	}
}
