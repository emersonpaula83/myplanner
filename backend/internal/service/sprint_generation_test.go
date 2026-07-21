package service

import (
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/jira"
)

func TestDetectSprintPattern_PrefixAndDuration(t *testing.T) {
	sd1 := "2026-07-07T11:30:00.000-0300"
	ed1 := "2026-07-18T21:30:00.000-0300"
	sd2 := "2026-07-21T11:30:00.000-0300"
	ed2 := "2026-08-01T21:30:00.000-0300"
	sprints := []jira.JiraSprint{
		{ID: 1, Name: "RM Dev 07/07 - 18/07 [2026]", StartDate: &sd1, EndDate: &ed1},
		{ID: 2, Name: "RM Dev 21/07 - 01/08 [2026]", StartDate: &sd2, EndDate: &ed2},
	}
	prefix, days, err := detectSprintPattern(sprints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prefix != "RM Dev" {
		t.Errorf("prefix = %q, want %q", prefix, "RM Dev")
	}
	if days != 11 {
		t.Errorf("days = %d, want %d", days, 11)
	}
}

func TestDetectSprintPattern_NoSprints(t *testing.T) {
	_, _, err := detectSprintPattern(nil)
	if err == nil {
		t.Fatal("expected error for empty sprints")
	}
}

func TestDetectSprintPattern_NoRecognizablePattern(t *testing.T) {
	sd := "2026-07-07T11:30:00.000-0300"
	ed := "2026-07-18T21:30:00.000-0300"
	sprints := []jira.JiraSprint{
		{ID: 1, Name: "Random Sprint Name", StartDate: &sd, EndDate: &ed},
	}
	_, _, err := detectSprintPattern(sprints)
	if err == nil {
		t.Fatal("expected error for unrecognizable pattern")
	}
}

func TestDetectSprintPattern_FallbackDuration(t *testing.T) {
	sprints := []jira.JiraSprint{
		{ID: 1, Name: "RM Dev 07/07 - 18/07 [2026]"},
	}
	prefix, days, err := detectSprintPattern(sprints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prefix != "RM Dev" {
		t.Errorf("prefix = %q, want %q", prefix, "RM Dev")
	}
	if days != 11 {
		t.Errorf("days = %d, want 11 (fallback)", days)
	}
}

func TestDetectSprintPattern_MostFrequentPrefix(t *testing.T) {
	sd1 := "2026-07-07T11:30:00.000-0300"
	ed1 := "2026-07-18T21:30:00.000-0300"
	sd2 := "2026-07-21T11:30:00.000-0300"
	ed2 := "2026-08-01T21:30:00.000-0300"
	sd3 := "2026-08-04T11:30:00.000-0300"
	ed3 := "2026-08-15T21:30:00.000-0300"
	sprints := []jira.JiraSprint{
		{ID: 1, Name: "RM Dev 07/07 - 18/07 [2026]", StartDate: &sd1, EndDate: &ed1},
		{ID: 2, Name: "RM Dev 21/07 - 01/08 [2026]", StartDate: &sd2, EndDate: &ed2},
		{ID: 3, Name: "Outlier 04/08 - 15/08 [2026]", StartDate: &sd3, EndDate: &ed3},
	}
	prefix, _, err := detectSprintPattern(sprints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prefix != "RM Dev" {
		t.Errorf("prefix = %q, want %q (most frequent)", prefix, "RM Dev")
	}
}
