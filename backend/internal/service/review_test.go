package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func intPtr(i int) *int {
	return &i
}

// --- Pass-through methods ---

func TestReviewService_GetAnalise(t *testing.T) {
	ctx := context.Background()
	sprintID, equipeID := uuid.New(), uuid.New()

	t.Run("error", func(t *testing.T) {
		repo := &mockReviewStore{
			getReviewAnaliseFn: func(ctx context.Context, s, e uuid.UUID, p []uuid.UUID) (*repository.ReviewAnalise, error) {
				return nil, fmt.Errorf("boom")
			},
		}
		svc := NewReviewService(repo, nil, zap.NewNop())
		_, err := svc.GetAnalise(ctx, sprintID, equipeID, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("success", func(t *testing.T) {
		want := &repository.ReviewAnalise{ID: uuid.New(), SprintID: sprintID, EquipeID: equipeID}
		repo := &mockReviewStore{
			getReviewAnaliseFn: func(ctx context.Context, s, e uuid.UUID, p []uuid.UUID) (*repository.ReviewAnalise, error) {
				if s != sprintID || e != equipeID {
					t.Errorf("unexpected args: %v %v", s, e)
				}
				return want, nil
			},
		}
		svc := NewReviewService(repo, nil, zap.NewNop())
		got, err := svc.GetAnalise(ctx, sprintID, equipeID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("expected %v, got %v", want, got)
		}
	})
}

func TestReviewService_ListDestaques(t *testing.T) {
	ctx := context.Background()
	sprintID, equipeID := uuid.New(), uuid.New()

	t.Run("error", func(t *testing.T) {
		repo := &mockReviewStore{
			listDestaquesFn: func(ctx context.Context, s, e uuid.UUID) ([]repository.ReviewDestaque, error) {
				return nil, fmt.Errorf("boom")
			},
		}
		svc := NewReviewService(repo, nil, zap.NewNop())
		_, err := svc.ListDestaques(ctx, sprintID, equipeID)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("success", func(t *testing.T) {
		want := []repository.ReviewDestaque{{ID: uuid.New(), Titulo: "Foo"}}
		repo := &mockReviewStore{
			listDestaquesFn: func(ctx context.Context, s, e uuid.UUID) ([]repository.ReviewDestaque, error) {
				return want, nil
			},
		}
		svc := NewReviewService(repo, nil, zap.NewNop())
		got, err := svc.ListDestaques(ctx, sprintID, equipeID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].Titulo != "Foo" {
			t.Errorf("unexpected result: %+v", got)
		}
	})
}

func TestReviewService_CreateDestaque(t *testing.T) {
	ctx := context.Background()

	t.Run("error", func(t *testing.T) {
		repo := &mockReviewStore{
			createDestaqueFn: func(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error) {
				return repository.ReviewDestaque{}, fmt.Errorf("boom")
			},
		}
		svc := NewReviewService(repo, nil, zap.NewNop())
		_, err := svc.CreateDestaque(ctx, repository.ReviewDestaque{Titulo: "Foo"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("success", func(t *testing.T) {
		in := repository.ReviewDestaque{Titulo: "Foo", Descricao: "Bar"}
		out := in
		out.ID = uuid.New()
		repo := &mockReviewStore{
			createDestaqueFn: func(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error) {
				if d.Titulo != "Foo" {
					t.Errorf("unexpected arg: %+v", d)
				}
				return out, nil
			},
		}
		svc := NewReviewService(repo, nil, zap.NewNop())
		got, err := svc.CreateDestaque(ctx, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != out.ID {
			t.Errorf("expected ID %v, got %v", out.ID, got.ID)
		}
	})
}

func TestReviewService_UpdateDestaque(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()

	t.Run("error", func(t *testing.T) {
		repo := &mockReviewStore{
			updateDestaqueFn: func(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error) {
				return repository.ReviewDestaque{}, fmt.Errorf("boom")
			},
		}
		svc := NewReviewService(repo, nil, zap.NewNop())
		_, err := svc.UpdateDestaque(ctx, id, "t", "d", nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("success", func(t *testing.T) {
		link := "http://example.com"
		repo := &mockReviewStore{
			updateDestaqueFn: func(ctx context.Context, gotID uuid.UUID, titulo, descricao string, gotLink *string) (repository.ReviewDestaque, error) {
				if gotID != id || titulo != "t" || descricao != "d" || gotLink != &link {
					t.Errorf("unexpected args: %v %v %v %v", gotID, titulo, descricao, gotLink)
				}
				return repository.ReviewDestaque{ID: id, Titulo: titulo, Descricao: descricao, Link: gotLink}, nil
			},
		}
		svc := NewReviewService(repo, nil, zap.NewNop())
		got, err := svc.UpdateDestaque(ctx, id, "t", "d", &link)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != id || got.Titulo != "t" {
			t.Errorf("unexpected result: %+v", got)
		}
	})
}

func TestReviewService_DeleteDestaque(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()

	t.Run("error", func(t *testing.T) {
		repo := &mockReviewStore{
			deleteDestaqueFn: func(ctx context.Context, id uuid.UUID) error {
				return fmt.Errorf("boom")
			},
		}
		svc := NewReviewService(repo, nil, zap.NewNop())
		err := svc.DeleteDestaque(ctx, id)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("success", func(t *testing.T) {
		called := false
		repo := &mockReviewStore{
			deleteDestaqueFn: func(ctx context.Context, gotID uuid.UUID) error {
				called = true
				if gotID != id {
					t.Errorf("expected id %v, got %v", id, gotID)
				}
				return nil
			},
		}
		svc := NewReviewService(repo, nil, zap.NewNop())
		if err := svc.DeleteDestaque(ctx, id); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !called {
			t.Error("expected deleteDestaqueFn to be called")
		}
	})
}

// --- GetReviewData ---

func TestReviewService_GetReviewData(t *testing.T) {
	ctx := context.Background()
	sprintID, equipeID := uuid.New(), uuid.New()

	t.Run("GetReviewTasks error", func(t *testing.T) {
		repo := &mockReviewStore{
			getSprintEstadoFn: func(ctx context.Context, s uuid.UUID) (*string, error) {
				return nil, nil
			},
			getReviewTasksFn: func(ctx context.Context, s uuid.UUID, e *uuid.UUID, p []uuid.UUID) ([]repository.ReviewTaskRow, error) {
				return nil, fmt.Errorf("boom")
			},
		}
		svc := NewReviewService(repo, nil, zap.NewNop())
		_, err := svc.GetReviewData(ctx, sprintID, equipeID, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("happy path with varied tasks", func(t *testing.T) {
		produtoID := uuid.New()
		gdptcParentID := uuid.New()
		gdptcTaskID := uuid.New()

		tasks := []repository.ReviewTaskRow{
			// 1. Concluído, planejada, Bug -> Concluidas, PlanejadasConcl, BugsIncidentes, manutencao
			{
				ID:              uuid.New(),
				NumeroTicket:    "T-1",
				Resumo:          "Fix bug",
				Tipo:            "Bug",
				TipoDemanda:     "Compromisso",
				Status:          "Concluído",
				NaoPlanejada:    false,
				EstimativaTempo: intPtr(3600),
				Produtos:        []string{"Produto A"},
				ProdutoIDs:      []uuid.UUID{produtoID},
			},
			// 2. Desenvolvimento (em andamento), não planejada, Incidente -> EmAndamento, BugsIncidentes, manutencao
			{
				ID:           uuid.New(),
				NumeroTicket: "T-2",
				Resumo:       "Handle incident",
				Tipo:         "Incidente Produção",
				TipoDemanda:  "Meta",
				Status:       "Desenvolvimento",
				NaoPlanejada: true,
			},
			// 3. Backlog (não iniciada), planejada, Melhoria -> NaoIniciadas, MelhoriasInovacoes, melhorias
			{
				ID:              uuid.New(),
				NumeroTicket:    "T-3",
				Resumo:          "Improve X",
				Tipo:            "Melhoria",
				TipoDemanda:     "",
				Status:          "Backlog",
				NaoPlanejada:    false,
				EstimativaTempo: intPtr(7200),
				Produtos:        []string{"Produto A", "Produto B"},
				ProdutoIDs:      []uuid.UUID{produtoID, uuid.New()},
			},
			// 4. Concluído, planejada, História, but has GDPTC ancestor parent -> novos_projetos
			{
				ID:           gdptcTaskID,
				NumeroTicket: "T-4",
				Resumo:       "GDPTC linked story",
				Tipo:         "História",
				TipoDemanda:  "",
				Status:       "Concluído",
				NaoPlanejada: false,
				ParentID:     &gdptcParentID,
			},
			// 5. Teste (em andamento), não planejada, Tarefa (outros) -> EmAndamento, Outros, outros
			{
				ID:           uuid.New(),
				NumeroTicket: "T-5",
				Resumo:       "Misc task",
				Tipo:         "Tarefa",
				TipoDemanda:  "",
				Status:       "Teste",
				NaoPlanejada: true,
			},
		}

		repo := &mockReviewStore{
			getSprintEstadoFn: func(ctx context.Context, s uuid.UUID) (*string, error) {
				return nil, nil
			},
			getReviewTasksFn: func(ctx context.Context, s uuid.UUID, e *uuid.UUID, p []uuid.UUID) ([]repository.ReviewTaskRow, error) {
				return tasks, nil
			},
			getGDPTCAncestorIDsFn: func(ctx context.Context, taskIDs []uuid.UUID) ([]uuid.UUID, error) {
				// Only task 4 has a non-bug/incident parent, so should be the only one passed.
				if len(taskIDs) != 1 || taskIDs[0] != gdptcTaskID {
					t.Errorf("unexpected parentTaskIDs passed to GetGDPTCAncestorTaskIDs: %v", taskIDs)
				}
				return []uuid.UUID{gdptcTaskID}, nil
			},
			getReviewPOsFn: func(ctx context.Context, e uuid.UUID, p []uuid.UUID) ([]repository.ReviewPO, error) {
				return []repository.ReviewPO{{Nome: "PO Person", Produtos: []string{"Produto A"}}}, nil
			},
		}

		svc := NewReviewService(repo, nil, zap.NewNop())
		data, err := svc.GetReviewData(ctx, sprintID, equipeID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if data.Stats.Total != 5 {
			t.Errorf("expected Total=5, got %d", data.Stats.Total)
		}
		if data.Stats.Concluidas != 2 {
			t.Errorf("expected Concluidas=2, got %d", data.Stats.Concluidas)
		}
		if data.Stats.EmAndamento != 2 {
			t.Errorf("expected EmAndamento=2, got %d", data.Stats.EmAndamento)
		}
		if data.Stats.NaoIniciadas != 1 {
			t.Errorf("expected NaoIniciadas=1, got %d", data.Stats.NaoIniciadas)
		}
		// Planejadas: tasks 1, 3, 4 are planejada (NaoPlanejada=false) => PlanejadasTotal=3
		if data.Stats.PlanejadasTotal != 3 {
			t.Errorf("expected PlanejadasTotal=3, got %d", data.Stats.PlanejadasTotal)
		}
		// PlanejadasConcl: task 1 (Concluído + planejada) and task 4 (Concluído + planejada) => 2
		if data.Stats.PlanejadasConcl != 2 {
			t.Errorf("expected PlanejadasConcl=2, got %d", data.Stats.PlanejadasConcl)
		}
		// BugsIncidentes: task 1 (Bug), task 2 (Incidente Produção) => 2
		if data.Stats.BugsIncidentes != 2 {
			t.Errorf("expected BugsIncidentes=2, got %d", data.Stats.BugsIncidentes)
		}
		// MelhoriasInovacoes: task 3 (Melhoria), task 4 (GDPTC ancestor) => 2
		if data.Stats.MelhoriasInovacoes != 2 {
			t.Errorf("expected MelhoriasInovacoes=2, got %d", data.Stats.MelhoriasInovacoes)
		}
		// Outros: task 5
		if data.Stats.Outros != 1 {
			t.Errorf("expected Outros=1, got %d", data.Stats.Outros)
		}

		// Category chart
		catTotals := map[string]int{}
		for _, c := range data.GraficoCategorias {
			catTotals[c.Categoria] = c.Total
		}
		if catTotals["manutencao"] != 2 {
			t.Errorf("expected manutencao=2, got %d", catTotals["manutencao"])
		}
		if catTotals["novos_projetos"] != 1 {
			t.Errorf("expected novos_projetos=1, got %d", catTotals["novos_projetos"])
		}
		if catTotals["melhorias"] != 1 {
			t.Errorf("expected melhorias=1, got %d", catTotals["melhorias"])
		}
		if catTotals["outros"] != 1 {
			t.Errorf("expected outros=1, got %d", catTotals["outros"])
		}

		// Planejamento chart: Planejadas=3, NaoPlanejadas=2 (task 2 bug/incident, task 5 outros)
		if data.GraficoPlanejamento.Planejadas != 3 {
			t.Errorf("expected Planejadas=3, got %d", data.GraficoPlanejamento.Planejadas)
		}
		if data.GraficoPlanejamento.NaoPlanejadas != 2 {
			t.Errorf("expected NaoPlanejadas=2, got %d", data.GraficoPlanejamento.NaoPlanejadas)
		}
		if data.GraficoPlanejamento.NaoPlanejadasBugs != 1 {
			t.Errorf("expected NaoPlanejadasBugs=1, got %d", data.GraficoPlanejamento.NaoPlanejadasBugs)
		}
		if data.GraficoPlanejamento.NaoPlanejadasOutras != 1 {
			t.Errorf("expected NaoPlanejadasOutras=1, got %d", data.GraficoPlanejamento.NaoPlanejadasOutras)
		}

		// Task list categorias
		gotCategorias := map[string]string{}
		for _, tr := range data.Tarefas {
			gotCategorias[tr.NumeroTicket] = tr.Categoria
		}
		want := map[string]string{
			"T-1": "manutencao",
			"T-2": "manutencao",
			"T-3": "melhorias",
			"T-4": "novos_projetos",
			"T-5": "outros",
		}
		for ticket, wantCat := range want {
			if gotCategorias[ticket] != wantCat {
				t.Errorf("task %s: expected categoria %q, got %q", ticket, wantCat, gotCategorias[ticket])
			}
		}

		// EstimativaHoras conversion: task 1 had 3600s -> 1h
		for _, tr := range data.Tarefas {
			if tr.NumeroTicket == "T-1" && tr.EstimativaHoras != 1.0 {
				t.Errorf("expected T-1 EstimativaHoras=1.0, got %v", tr.EstimativaHoras)
			}
			if tr.NumeroTicket == "T-2" && tr.EstimativaHoras != 0 {
				t.Errorf("expected T-2 EstimativaHoras=0 (nil estimate), got %v", tr.EstimativaHoras)
			}
		}

		// POs pass-through
		if len(data.POs) != 1 || data.POs[0].Nome != "PO Person" {
			t.Errorf("unexpected POs: %+v", data.POs)
		}

		// Product chart: Produto A appears in tasks 1 and 3 (total 2, 1 concluida); Produto B in task 3 only
		var prodA, prodB *ReviewGraficoProduto
		for i := range data.GraficoProdutos {
			switch data.GraficoProdutos[i].Produto {
			case "Produto A":
				prodA = &data.GraficoProdutos[i]
			case "Produto B":
				prodB = &data.GraficoProdutos[i]
			}
		}
		if prodA == nil || prodA.Total != 2 || prodA.Concluidas != 1 {
			t.Errorf("unexpected Produto A entry: %+v", prodA)
		}
		if prodB == nil || prodB.Total != 1 {
			t.Errorf("unexpected Produto B entry: %+v", prodB)
		}
	})

	t.Run("closed sprint uses snapshot", func(t *testing.T) {
		closed := "closed"
		produtoID := uuid.New()
		snapshotTasks := []repository.ReviewTaskRow{
			{
				ID:           uuid.New(),
				NumeroTicket: "S-1",
				Resumo:       "Snapshot task",
				Tipo:         "Tarefa",
				Status:       "Concluído",
				NaoPlanejada: false,
				Produtos:     []string{"Produto A"},
				ProdutoIDs:   []uuid.UUID{produtoID},
			},
		}

		reviewTasksCalled := false
		repo := &mockReviewStore{
			getSprintEstadoFn: func(ctx context.Context, s uuid.UUID) (*string, error) {
				return &closed, nil
			},
			getSprintSnapshotFn: func(ctx context.Context, s uuid.UUID) ([]repository.ReviewTaskRow, error) {
				return snapshotTasks, nil
			},
			getReviewTasksFn: func(ctx context.Context, s uuid.UUID, e *uuid.UUID, p []uuid.UUID) ([]repository.ReviewTaskRow, error) {
				reviewTasksCalled = true
				return nil, fmt.Errorf("should not be called when snapshot succeeds")
			},
			getGDPTCAncestorIDsFn: func(ctx context.Context, taskIDs []uuid.UUID) ([]uuid.UUID, error) {
				return nil, nil
			},
			getReviewPOsFn: func(ctx context.Context, e uuid.UUID, p []uuid.UUID) ([]repository.ReviewPO, error) {
				return nil, nil
			},
		}

		svc := NewReviewService(repo, nil, zap.NewNop())
		data, err := svc.GetReviewData(ctx, sprintID, equipeID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if reviewTasksCalled {
			t.Error("expected GetReviewTasks to not be called when snapshot is used")
		}
		if data.Stats.Total != 1 {
			t.Errorf("expected Total=1, got %d", data.Stats.Total)
		}
		if len(data.Tarefas) != 1 || data.Tarefas[0].NumeroTicket != "S-1" {
			t.Errorf("unexpected tarefas: %+v", data.Tarefas)
		}
	})

	t.Run("closed sprint falls back to review tasks when snapshot errors", func(t *testing.T) {
		closed := "closed"
		fallbackTasks := []repository.ReviewTaskRow{
			{ID: uuid.New(), NumeroTicket: "F-1", Tipo: "Tarefa", Status: "Backlog"},
		}
		repo := &mockReviewStore{
			getSprintEstadoFn: func(ctx context.Context, s uuid.UUID) (*string, error) {
				return &closed, nil
			},
			getSprintSnapshotFn: func(ctx context.Context, s uuid.UUID) ([]repository.ReviewTaskRow, error) {
				return nil, fmt.Errorf("no snapshot")
			},
			getReviewTasksFn: func(ctx context.Context, s uuid.UUID, e *uuid.UUID, p []uuid.UUID) ([]repository.ReviewTaskRow, error) {
				return fallbackTasks, nil
			},
			getGDPTCAncestorIDsFn: func(ctx context.Context, taskIDs []uuid.UUID) ([]uuid.UUID, error) {
				return nil, nil
			},
			getReviewPOsFn: func(ctx context.Context, e uuid.UUID, p []uuid.UUID) ([]repository.ReviewPO, error) {
				return nil, nil
			},
		}

		svc := NewReviewService(repo, nil, zap.NewNop())
		data, err := svc.GetReviewData(ctx, sprintID, equipeID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(data.Tarefas) != 1 || data.Tarefas[0].NumeroTicket != "F-1" {
			t.Errorf("expected fallback to review tasks, got: %+v", data.Tarefas)
		}
	})

	t.Run("GetGDPTCAncestorTaskIDs error", func(t *testing.T) {
		repo := &mockReviewStore{
			getSprintEstadoFn: func(ctx context.Context, s uuid.UUID) (*string, error) {
				return nil, nil
			},
			getReviewTasksFn: func(ctx context.Context, s uuid.UUID, e *uuid.UUID, p []uuid.UUID) ([]repository.ReviewTaskRow, error) {
				return []repository.ReviewTaskRow{}, nil
			},
			getGDPTCAncestorIDsFn: func(ctx context.Context, taskIDs []uuid.UUID) ([]uuid.UUID, error) {
				return nil, fmt.Errorf("boom")
			},
		}
		svc := NewReviewService(repo, nil, zap.NewNop())
		_, err := svc.GetReviewData(ctx, sprintID, equipeID, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("GetReviewPOs error", func(t *testing.T) {
		repo := &mockReviewStore{
			getSprintEstadoFn: func(ctx context.Context, s uuid.UUID) (*string, error) {
				return nil, nil
			},
			getReviewTasksFn: func(ctx context.Context, s uuid.UUID, e *uuid.UUID, p []uuid.UUID) ([]repository.ReviewTaskRow, error) {
				return []repository.ReviewTaskRow{}, nil
			},
			getGDPTCAncestorIDsFn: func(ctx context.Context, taskIDs []uuid.UUID) ([]uuid.UUID, error) {
				return nil, nil
			},
			getReviewPOsFn: func(ctx context.Context, e uuid.UUID, p []uuid.UUID) ([]repository.ReviewPO, error) {
				return nil, fmt.Errorf("boom")
			},
		}
		svc := NewReviewService(repo, nil, zap.NewNop())
		_, err := svc.GetReviewData(ctx, sprintID, equipeID, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// --- filterSnapshotTasks ---

func TestFilterSnapshotTasks(t *testing.T) {
	pA, pB, pC := uuid.New(), uuid.New(), uuid.New()
	tasks := []repository.ReviewTaskRow{
		{ID: uuid.New(), NumeroTicket: "T-1", ProdutoIDs: []uuid.UUID{pA}},
		{ID: uuid.New(), NumeroTicket: "T-2", ProdutoIDs: []uuid.UUID{pB}},
		{ID: uuid.New(), NumeroTicket: "T-3", ProdutoIDs: []uuid.UUID{pA, pB}},
		{ID: uuid.New(), NumeroTicket: "T-4", ProdutoIDs: []uuid.UUID{pC}},
	}

	t.Run("empty produtoIDs returns all tasks", func(t *testing.T) {
		got := filterSnapshotTasks(tasks, nil)
		if len(got) != len(tasks) {
			t.Errorf("expected %d tasks, got %d", len(tasks), len(got))
		}
	})

	t.Run("filters by intersection", func(t *testing.T) {
		got := filterSnapshotTasks(tasks, []uuid.UUID{pA})
		if len(got) != 2 {
			t.Fatalf("expected 2 tasks, got %d", len(got))
		}
		tickets := map[string]bool{}
		for _, tr := range got {
			tickets[tr.NumeroTicket] = true
		}
		if !tickets["T-1"] || !tickets["T-3"] {
			t.Errorf("unexpected filtered tasks: %+v", got)
		}
	})

	t.Run("no match returns empty", func(t *testing.T) {
		got := filterSnapshotTasks(tasks, []uuid.UUID{uuid.New()})
		if len(got) != 0 {
			t.Errorf("expected 0 tasks, got %d", len(got))
		}
	})
}

// --- buildReviewAnalisePrompt ---

func TestBuildReviewAnalisePrompt(t *testing.T) {
	tarefas := []ReviewTarefa{
		{
			NumeroTicket:    "T-1",
			Produto:         "Produto A",
			Resumo:          "Do something",
			Tipo:            "Bug",
			TipoDemanda:     "Meta",
			Categoria:       "manutencao",
			Status:          "Concluído",
			NaoPlanejada:    true,
			EstimativaHoras: 2.5,
		},
	}

	system, user := buildReviewAnalisePrompt(tarefas)

	if system == "" {
		t.Error("expected non-empty system prompt")
	}
	if user == "" {
		t.Error("expected non-empty user prompt")
	}

	const prefix = "DADOS DA SPRINT:\n"
	if len(user) <= len(prefix) || user[:len(prefix)] != prefix {
		t.Fatalf("expected user prompt to start with %q, got %q", prefix, user)
	}

	var pts []promptTarefa
	if err := json.Unmarshal([]byte(user[len(prefix):]), &pts); err != nil {
		t.Fatalf("failed to unmarshal embedded JSON: %v", err)
	}
	if len(pts) != 1 {
		t.Fatalf("expected 1 tarefa in JSON, got %d", len(pts))
	}
	pt := pts[0]
	if pt.Ticket != "T-1" || pt.Resumo != "Do something" || pt.Tipo != "Bug" ||
		pt.TipoDemanda != "Meta" || pt.Status != "Concluído" || pt.Produto != "Produto A" ||
		!pt.NaoPlanejada || pt.EstimativaHoras != 2.5 {
		t.Errorf("unexpected promptTarefa: %+v", pt)
	}
}
