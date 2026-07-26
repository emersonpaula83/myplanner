package service

import (
	"os"
	"strings"
	"testing"
)

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
