package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/google/uuid"
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
	// Dec 8 (Tue) → nextMonday = Dec 14. End = Dec 14 + 11 = Dec 25 (Fri). Fits.
	// Next: Dec 28 (Mon) + 11 = Jan 8, 2027 > yearEnd. Stops.
	start := time.Date(2026, 12, 8, 0, 0, 0, 0, loc)
	slots := generateSprintSlots(start, 12, 2026)

	if len(slots) != 1 {
		t.Errorf("expected 1 slot, got %d", len(slots))
	}
}

func TestPreviewSprints_UsesJiraAPI(t *testing.T) {
	sd1 := "2026-07-07T11:30:00.000-0300"
	ed1 := "2026-07-18T21:30:00.000-0300"
	sd2 := "2026-07-21T11:30:00.000-0300"
	ed2 := "2026-08-01T21:30:00.000-0300"
	mockClient := newDefaultMockJiraClient()
	mockClient.getBoardSprintsFn = func(ctx context.Context, boardID int) ([]jira.JiraSprint, error) {
		return []jira.JiraSprint{
			{ID: 1, Name: "RM Dev 07/07 - 18/07 [2026]", StartDate: &sd1, EndDate: &ed1},
			{ID: 2, Name: "RM Dev 21/07 - 01/08 [2026]", StartDate: &sd2, EndDate: &ed2},
		}, nil
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

func TestGenerateSprintSlots_AlwaysMonToFri(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	start := time.Date(2026, 8, 17, 0, 0, 0, 0, loc)
	slots := generateSprintSlots(start, 14, 2026)

	if len(slots) == 0 {
		t.Fatal("expected at least one slot")
	}
	for i, s := range slots {
		if s.start.Weekday() != time.Monday {
			t.Errorf("slot %d: start = %v (%s), want Monday", i, s.start, s.start.Weekday())
		}
		if s.end.Weekday() != time.Friday {
			t.Errorf("slot %d: end = %v (%s), want Friday", i, s.end, s.end.Weekday())
		}
		diff := int(s.end.Sub(s.start).Hours()/24) + 1
		if diff != 12 {
			t.Errorf("slot %d: duration = %d, want 12", i, diff)
		}
	}
	// Verify consecutive sprints: next starts Monday after previous Friday (3 calendar days)
	for i := 1; i < len(slots); i++ {
		prevEnd := slots[i-1].end
		nextStart := slots[i].start
		gapDays := nextStart.Day() - prevEnd.Day()
		if prevEnd.Month() != nextStart.Month() {
			lastDay := time.Date(prevEnd.Year(), prevEnd.Month()+1, 0, 0, 0, 0, 0, loc).Day()
			gapDays = (lastDay - prevEnd.Day()) + nextStart.Day()
		}
		if gapDays != 3 {
			t.Errorf("gap between slot %d and %d = %d calendar days, want 3 (Fri→Mon)", i-1, i, gapDays)
		}
	}
}

func TestNextMonday(t *testing.T) {
	tests := []struct {
		name  string
		input time.Time
		want  time.Time
	}{
		{
			name:  "already Monday",
			input: time.Date(2026, 8, 3, 0, 0, 0, 0, saoPaulo),
			want:  time.Date(2026, 8, 3, 0, 0, 0, 0, saoPaulo),
		},
		{
			name:  "Wednesday to next Monday",
			input: time.Date(2026, 8, 5, 0, 0, 0, 0, saoPaulo),
			want:  time.Date(2026, 8, 10, 0, 0, 0, 0, saoPaulo),
		},
		{
			name:  "Sunday to next Monday",
			input: time.Date(2026, 8, 2, 0, 0, 0, 0, saoPaulo),
			want:  time.Date(2026, 8, 3, 0, 0, 0, 0, saoPaulo),
		},
		{
			name:  "Saturday to next Monday",
			input: time.Date(2026, 8, 1, 0, 0, 0, 0, saoPaulo),
			want:  time.Date(2026, 8, 3, 0, 0, 0, 0, saoPaulo),
		},
		{
			name:  "Friday to next Monday",
			input: time.Date(2026, 8, 7, 0, 0, 0, 0, saoPaulo),
			want:  time.Date(2026, 8, 10, 0, 0, 0, 0, saoPaulo),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nextMonday(tc.input)
			if !got.Equal(tc.want) {
				t.Errorf("nextMonday(%v) = %v, want %v", tc.input, got, tc.want)
			}
			if got.Weekday() != time.Monday {
				t.Errorf("nextMonday(%v) weekday = %v, want Monday", tc.input, got.Weekday())
			}
		})
	}
}

func TestFormatSprintName(t *testing.T) {
	start := time.Date(2026, 8, 3, 8, 30, 0, 0, saoPaulo)
	end := time.Date(2026, 8, 14, 18, 30, 0, 0, saoPaulo)

	got := formatSprintName("RM Dev", start, end, 2026)
	want := "RM Dev 03/08 - 14/08 [2026]"
	if got != want {
		t.Errorf("formatSprintName() = %q, want %q", got, want)
	}
}

func TestParseJiraDate(t *testing.T) {
	rfc3339 := "2026-08-03T08:30:00Z"
	custom := "2026-08-03T08:30:00.000-0300"
	dateOnly := "2026-08-03"
	invalid := "not-a-date"
	empty := ""

	tests := []struct {
		name    string
		input   *string
		wantNil bool
	}{
		{"nil input", nil, true},
		{"empty string", &empty, true},
		{"RFC3339", &rfc3339, false},
		{"custom Jira format", &custom, false},
		{"date only", &dateOnly, false},
		{"invalid", &invalid, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseJiraDate(tc.input)
			if tc.wantNil {
				if got != nil {
					t.Errorf("parseJiraDate(%v) = %v, want nil", tc.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseJiraDate(%v) = nil, want non-nil", tc.input)
			}
			if got.Year() != 2026 || got.Month() != time.August || got.Day() != 3 {
				t.Errorf("parseJiraDate(%v) = %v, want date 2026-08-03", *tc.input, got)
			}
		})
	}
}

func TestModeString(t *testing.T) {
	t.Run("single entry", func(t *testing.T) {
		counts := map[string]int{"a": 1}
		got := modeString(counts)
		if got != "a" {
			t.Errorf("modeString() = %q, want %q", got, "a")
		}
	})

	t.Run("multiple entries, one dominant", func(t *testing.T) {
		counts := map[string]int{"a": 1, "b": 3, "c": 2}
		got := modeString(counts)
		if got != "b" {
			t.Errorf("modeString() = %q, want %q", got, "b")
		}
	})
}

func TestModeInt(t *testing.T) {
	t.Run("single entry", func(t *testing.T) {
		counts := map[int]int{12: 1}
		got := modeInt(counts)
		if got != 12 {
			t.Errorf("modeInt() = %d, want %d", got, 12)
		}
	})

	t.Run("multiple entries, one dominant", func(t *testing.T) {
		counts := map[int]int{10: 1, 12: 4, 14: 2}
		got := modeInt(counts)
		if got != 12 {
			t.Errorf("modeInt() = %d, want %d", got, 12)
		}
	})
}

func TestFilterExistingSlots(t *testing.T) {
	slots := []sprintSlot{
		{
			start: time.Date(2026, 8, 3, 8, 30, 0, 0, saoPaulo),
			end:   time.Date(2026, 8, 14, 18, 30, 0, 0, saoPaulo),
		},
		{
			start: time.Date(2026, 8, 17, 8, 30, 0, 0, saoPaulo),
			end:   time.Date(2026, 8, 28, 18, 30, 0, 0, saoPaulo),
		},
	}

	t.Run("no overlaps", func(t *testing.T) {
		sd := "2026-01-05T08:30:00.000-0300"
		ed := "2026-01-16T18:30:00.000-0300"
		existing := []jira.JiraSprint{
			{ID: 1, Name: "RM Dev 05/01 - 16/01 [2026]", StartDate: &sd, EndDate: &ed},
		}

		missing, ignored := filterExistingSlots(slots, existing)
		if ignored != 0 {
			t.Errorf("ignored = %d, want 0", ignored)
		}
		if len(missing) != len(slots) {
			t.Errorf("missing = %d slots, want %d (all)", len(missing), len(slots))
		}
	})

	t.Run("some overlaps are filtered", func(t *testing.T) {
		// Overlaps the first slot (2026-08-03 .. 2026-08-14).
		sd := "2026-08-05T08:30:00.000-0300"
		ed := "2026-08-16T18:30:00.000-0300"
		existing := []jira.JiraSprint{
			{ID: 1, Name: "RM Dev 05/08 - 16/08 [2026]", StartDate: &sd, EndDate: &ed},
		}

		missing, ignored := filterExistingSlots(slots, existing)
		if ignored != 1 {
			t.Errorf("ignored = %d, want 1", ignored)
		}
		if len(missing) != 1 {
			t.Fatalf("missing = %d slots, want 1", len(missing))
		}
		if !missing[0].start.Equal(slots[1].start) {
			t.Errorf("missing slot start = %v, want %v (second slot survives)", missing[0].start, slots[1].start)
		}
	})

	t.Run("existing sprint with unparseable dates is ignored, not treated as overlap", func(t *testing.T) {
		existing := []jira.JiraSprint{
			{ID: 1, Name: "No dates", StartDate: nil, EndDate: nil},
		}

		missing, ignored := filterExistingSlots(slots, existing)
		if ignored != 0 {
			t.Errorf("ignored = %d, want 0", ignored)
		}
		if len(missing) != len(slots) {
			t.Errorf("missing = %d slots, want %d (all)", len(missing), len(slots))
		}
	})
}

func TestGetFonteDadosForEquipe(t *testing.T) {
	equipeID := uuid.New()

	t.Run("GetMembrosEquipe error", func(t *testing.T) {
		svc := &SprintGenerationService{
			equipeRepo: &mockEquipeStore{
				getMembrosEquipeFn: func(ctx context.Context, id uuid.UUID) ([]domain.Membro, error) {
					return nil, errors.New("db error")
				},
			},
			logger: zap.NewNop(),
		}

		_, err := svc.getFonteDadosForEquipe(context.Background(), equipeID)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("empty members returns error", func(t *testing.T) {
		svc := &SprintGenerationService{
			equipeRepo: &mockEquipeStore{
				getMembrosEquipeFn: func(ctx context.Context, id uuid.UUID) ([]domain.Membro, error) {
					return []domain.Membro{}, nil
				},
			},
			logger: zap.NewNop(),
		}

		_, err := svc.getFonteDadosForEquipe(context.Background(), equipeID)
		if err == nil {
			t.Fatal("expected error for empty members, got nil")
		}
	})

	t.Run("happy path returns first member's FonteDadosID", func(t *testing.T) {
		wantFonteID := uuid.New()
		svc := &SprintGenerationService{
			equipeRepo: &mockEquipeStore{
				getMembrosEquipeFn: func(ctx context.Context, id uuid.UUID) ([]domain.Membro, error) {
					if id != equipeID {
						t.Errorf("equipeID passed = %v, want %v", id, equipeID)
					}
					return []domain.Membro{
						{ID: uuid.New(), FonteDadosID: wantFonteID},
						{ID: uuid.New(), FonteDadosID: uuid.New()},
					}, nil
				},
			},
			logger: zap.NewNop(),
		}

		got, err := svc.getFonteDadosForEquipe(context.Background(), equipeID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != wantFonteID {
			t.Errorf("getFonteDadosForEquipe() = %v, want %v", got, wantFonteID)
		}
	})
}

func TestDetectSprintPattern_IgnoresNonMondayStarts(t *testing.T) {
	// Mix of correct (Mon start) and manually edited (non-Mon start) sprints
	sdMon := "2026-07-07T08:30:00.000-0300"  // Monday
	edFri := "2026-07-18T18:30:00.000-0300"  // Friday, 12 days
	sdSat := "2026-07-11T11:30:00.000-0300"  // Saturday (manual edit)
	edFriB := "2026-07-24T21:30:00.000-0300" // Friday, 14 days
	sprints := []jira.JiraSprint{
		{ID: 1, Name: "RM Dev 07/07 - 18/07 [2026]", StartDate: &sdMon, EndDate: &edFri},
		{ID: 2, Name: "RM Dev 11/07 - 24/07 [2026]", StartDate: &sdSat, EndDate: &edFriB},
	}
	_, days, err := detectSprintPattern(sprints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if days != 12 {
		t.Errorf("days = %d, want 12 (should ignore non-Monday sprint)", days)
	}
}

func TestNewSprintGenerationService(t *testing.T) {
	svc := NewSprintGenerationService(nil, nil, nil, nil, nil, nil, nil, 10, zap.NewNop())
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}
