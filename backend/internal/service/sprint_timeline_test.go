package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func sprintTimelineItemDeTeste(nome, estado string, inicio, fim time.Time) repository.SprintListItem {
	return repository.SprintListItem{
		ID:         uuid.New(),
		Nome:       nome,
		Estado:     &estado,
		DataInicio: &inicio,
		DataFim:    &fim,
	}
}

func TestGetSprintsTimeline_IncluiSprintsConcluidas(t *testing.T) {
	dia := func(mes, d int) time.Time { return time.Date(2026, time.Month(mes), d, 0, 0, 0, 0, time.UTC) }
	sprints := []repository.SprintListItem{
		sprintTimelineItemDeTeste("Sprint fechada", "closed", dia(1, 5), dia(1, 19)),
		sprintTimelineItemDeTeste("Sprint ativa", "active", dia(2, 2), dia(2, 16)),
		sprintTimelineItemDeTeste("Sprint futura", "future", dia(3, 2), dia(3, 16)),
	}
	membro := repository.MembroInfo{ID: uuid.New(), Nome: "Fulano"}

	repo := &mockSprintRepoStore{
		getEquipeBoardIDFn: func(context.Context, uuid.UUID) (*int, error) { return nil, nil },
		listSprintsIncludeEmptyFn: func(context.Context, *uuid.UUID, *string, *int) ([]repository.SprintListItem, error) {
			return sprints, nil
		},
		getAllMembrosEquipeFn: func(context.Context, uuid.UUID) ([]repository.MembroInfo, error) {
			return []repository.MembroInfo{membro}, nil
		},
		getFeriadosNoPeriodoFn: func(context.Context, time.Time, time.Time) ([]repository.FeriadoRecord, error) {
			return nil, nil
		},
		getAusenciasNoPeriodoFn: func(context.Context, []uuid.UUID, time.Time, time.Time) ([]repository.AusenciaRecord, error) {
			return nil, nil
		},
		getHorasAlocadasPorSprintFn: func(context.Context, []uuid.UUID, []uuid.UUID) (map[uuid.UUID]float64, error) {
			return map[uuid.UUID]float64{}, nil
		},
	}

	itens, err := NewSprintService(repo, zap.NewNop()).GetSprintsTimeline(context.Background(), uuid.New(), 2026)
	if err != nil {
		t.Fatalf("GetSprintsTimeline: %v", err)
	}

	if len(itens) != 3 {
		nomes := make([]string, len(itens))
		for i, it := range itens {
			nomes[i] = it.SprintNome
		}
		t.Fatalf("esperava 3 sprints na timeline (incluindo closed), veio %d: %v", len(itens), nomes)
	}
}

func TestGetSprintsTimeline_NoDominantProjectHeuristic(t *testing.T) {
	// Verify the dominant project heuristic was removed from the source code.
	// This is a guardrail test — if someone re-adds projetoCount/dominantProjeto,
	// this test fails and forces them to reconsider.
	data, err := os.ReadFile("sprint.go")
	if err != nil {
		t.Fatalf("reading sprint.go: %v", err)
	}
	src := string(data)

	// These patterns were part of the removed heuristic
	forbidden := []string{
		"dominantProjeto",
		"projetoCount",
	}
	for _, pattern := range forbidden {
		if strings.Contains(src, pattern) {
			t.Errorf("sprint.go still contains '%s' — the dominant project heuristic should be removed. "+
				"Sprint filtering must use board_id from equipes table, not heuristics.", pattern)
		}
	}
}

func TestGetSprintsTimeline_UsesBoardIDFilter(t *testing.T) {
	// Verify that GetSprintsTimeline calls GetEquipeBoardID.
	// This is a guardrail test — if someone removes the board_id fetch,
	// this test fails.
	data, err := os.ReadFile("sprint.go")
	if err != nil {
		t.Fatalf("reading sprint.go: %v", err)
	}
	src := string(data)

	if !strings.Contains(src, "GetEquipeBoardID") {
		t.Error("GetSprintsTimeline must call GetEquipeBoardID to fetch the equipe's board_id. " +
			"This ensures sprint filtering is structural (by board), not heuristic-based.")
	}
}

func TestListSprints_AcceptsBoardIDParam(t *testing.T) {
	// Verify that listSprints/ListSprintsIncludeEmpty accept a boardID parameter.
	// This is a guardrail test — the board_id filter must remain in the query builder.
	data, err := os.ReadFile("../repository/sprint.go")
	if err != nil {
		t.Fatalf("reading sprint.go: %v", err)
	}
	src := string(data)

	if !strings.Contains(src, "boardID *int") {
		t.Error("listSprints must accept a boardID *int parameter. " +
			"This structural filter prevents sprints from other boards leaking into timeline.")
	}

	if !strings.Contains(src, `s.board_id = `) {
		t.Error("listSprints must filter by s.board_id in the SQL query. " +
			"Without this, sprints from other Jira boards can leak into the timeline.")
	}
}
