package service

import (
	"context"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"go.uber.org/zap"
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
	if days != 12 {
		t.Errorf("days = %d, want %d", days, 12)
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
	if days != 12 {
		t.Errorf("days = %d, want 12 (fallback)", days)
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

func TestGenerateSprintSlots_DynamicDuration(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	start := time.Date(2026, 8, 3, 0, 0, 0, 0, loc)
	// durationDays represents an inclusive calendar-day count (end = start + durationDays - 1),
	// so a Monday-to-Friday-of-next-week sprint (12 calendar days: Mon wk1 .. Fri wk2) needs 12,
	// not 11 (11 lands on Thursday). See task-2-report.md concerns for details.
	slots := generateSprintSlots(start, 12, 2026)

	if len(slots) == 0 {
		t.Fatal("expected at least one slot")
	}

	first := slots[0]
	if first.start.Weekday() != time.Monday {
		t.Errorf("first slot start = %v, want Monday", first.start.Weekday())
	}
	if first.start.Hour() != 8 || first.start.Minute() != 30 {
		t.Errorf("first slot start time = %02d:%02d, want 08:30", first.start.Hour(), first.start.Minute())
	}
	if first.end.Weekday() != time.Friday {
		t.Errorf("first slot end = %v, want Friday", first.end.Weekday())
	}
	if first.end.Hour() != 18 || first.end.Minute() != 30 {
		t.Errorf("first slot end time = %02d:%02d, want 18:30", first.end.Hour(), first.end.Minute())
	}
	if first.end.Location().String() != "America/Sao_Paulo" {
		t.Errorf("timezone = %s, want America/Sao_Paulo", first.end.Location())
	}
}

func TestGenerateSprintSlots_AdjustsToMonday(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	// Wednesday August 5
	start := time.Date(2026, 8, 5, 0, 0, 0, 0, loc)
	slots := generateSprintSlots(start, 11, 2026)

	if len(slots) == 0 {
		t.Fatal("expected at least one slot")
	}
	// Should adjust to next Monday (August 10)
	if slots[0].start.Day() != 10 {
		t.Errorf("first slot start day = %d, want 10 (next Monday)", slots[0].start.Day())
	}
}

func TestGenerateSprintSlots_StopsAtYearEnd(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	start := time.Date(2026, 12, 20, 0, 0, 0, 0, loc)
	slots := generateSprintSlots(start, 11, 2026)

	if len(slots) != 1 {
		t.Errorf("expected 1 slot (Dec 21 is Monday, end Dec 31), got %d", len(slots))
	}
}

func TestPreviewSprints_UsesJiraAPI(t *testing.T) {
	sd1 := "2026-07-07T11:30:00.000-0300"
	ed1 := "2026-07-18T21:30:00.000-0300"
	sd2 := "2026-07-21T11:30:00.000-0300"
	ed2 := "2026-08-01T21:30:00.000-0300"
	mockClient := &mockJiraClient{
		sprints: []jira.JiraSprint{
			{ID: 1, Name: "RM Dev 07/07 - 18/07 [2026]", StartDate: &sd1, EndDate: &ed1},
			{ID: 2, Name: "RM Dev 21/07 - 01/08 [2026]", StartDate: &sd2, EndDate: &ed2},
		},
	}

	svc := &SprintGenerationService{
		logger: zap.NewNop(),
	}

	loc, _ := time.LoadLocation("America/Sao_Paulo")
	startDate := time.Date(2026, 8, 3, 0, 0, 0, 0, loc)

	result, err := svc.previewSprintsWithClient(context.Background(), mockClient, 424, startDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PrefixoDetectado != "RM Dev" {
		t.Errorf("prefixo = %q, want %q", result.PrefixoDetectado, "RM Dev")
	}
	if result.DuracaoDetectadaDias != 12 {
		t.Errorf("duracao = %d, want 12", result.DuracaoDetectadaDias)
	}
	// The two existing JIRA sprints end 2026-08-01, while generated slots start
	// from startDate (2026-08-03) forward, so they can never overlap: ignored is 0.
	if result.ExistentesIgnoradas != 0 {
		t.Errorf("ignoradas = %d, want 0", result.ExistentesIgnoradas)
	}
	if len(result.Sprints) == 0 {
		t.Fatal("expected at least one sprint to create")
	}
	for _, s := range result.Sprints {
		if s.Nome == "" {
			t.Error("sprint nome is empty")
		}
		if s.DataInicio == "" || s.DataFim == "" {
			t.Error("sprint dates are empty")
		}
	}
}

func TestGenerateSprintSlots_7DaySprints(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	start := time.Date(2026, 8, 3, 0, 0, 0, 0, loc)
	slots := generateSprintSlots(start, 5, 2026)

	if len(slots) == 0 {
		t.Fatal("expected at least one slot")
	}
	first := slots[0]
	diff := int(first.end.Sub(first.start).Hours()/24) + 1
	if diff != 5 {
		t.Errorf("duration = %d days, want 5", diff)
	}
}
