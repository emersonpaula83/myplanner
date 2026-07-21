# Sprint Generation v2 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor sprint generation to auto-detect prefix/duration from JIRA board sprints, use JIRA API as source of truth for overlap checks, and accept `start_date` instead of `prefixo`.

**Architecture:** Modify existing handler/service layers. Handler receives `{equipe_id, board_id, start_date}`. Service fetches board sprints from JIRA API, detects naming pattern and duration, generates slots from `start_date` to Dec 31, filters overlaps, creates in JIRA. Frontend replaces `Prefixo` input with `Data Inicial` date picker.

**Tech Stack:** Go 1.22+, chi router, JIRA Agile REST API, vanilla JS frontend

## Global Constraints

- Timezone: fixed `America/Sao_Paulo` for all sprint times
- Sprint start: Monday 08:30:00 America/Sao_Paulo
- Sprint end: Friday 18:30:00 America/Sao_Paulo
- Date display format (UI): `dd/MM/yyyy HH:mm:ss`
- JIRA dates sent as RFC3339 with `-03:00` offset
- `parseOptionalDate` in `sync.go` parses `"2006-01-02"` format only — JIRA API returns RFC3339 dates, so a new parser is needed for direct JIRA sprint dates

---

### Task 1: Add `detectSprintPattern` function with tests

**Files:**
- Modify: `backend/internal/service/sprint_generation.go` (add function at bottom)
- Create: `backend/internal/service/sprint_generation_test.go`

**Interfaces:**
- Consumes: `jira.JiraSprint` type from `backend/internal/jira/types.go`
- Produces: `detectSprintPattern(sprints []jira.JiraSprint) (prefix string, durationDays int, err error)` — used by Tasks 3 and 4

- [ ] **Step 1: Write failing tests for `detectSprintPattern`**

Create `backend/internal/service/sprint_generation_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/service/ -run TestDetectSprintPattern -v`
Expected: FAIL — `detectSprintPattern` not defined

- [ ] **Step 3: Implement `detectSprintPattern` and `parseJiraDate`**

Add to `backend/internal/service/sprint_generation.go` after the existing `formatSprintName` function:

```go
import "regexp"

var sprintNamePattern = regexp.MustCompile(`^(.+?)\s*\d{2}/\d{2}\s*-\s*\d{2}/\d{2}`)

func detectSprintPattern(sprints []jira.JiraSprint) (string, int, error) {
	if len(sprints) == 0 {
		return "", 0, fmt.Errorf("nenhuma sprint encontrada no board para detectar padrão")
	}

	prefixCounts := make(map[string]int)
	durationCounts := make(map[int]int)

	for _, s := range sprints {
		matches := sprintNamePattern.FindStringSubmatch(s.Name)
		if matches != nil {
			prefix := strings.TrimSpace(matches[1])
			prefixCounts[prefix]++
		}

		start := parseJiraDate(s.StartDate)
		end := parseJiraDate(s.EndDate)
		if start != nil && end != nil {
			days := int(end.Sub(*start).Hours()/24) + 1
			if days > 0 {
				durationCounts[days]++
			}
		}
	}

	if len(prefixCounts) == 0 {
		return "", 0, fmt.Errorf("não foi possível detectar prefixo das sprints do board")
	}

	prefix := modeString(prefixCounts)
	duration := 11
	if len(durationCounts) > 0 {
		duration = modeInt(durationCounts)
	}

	return prefix, duration, nil
}

func parseJiraDate(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02",
	} {
		t, err := time.Parse(layout, *s)
		if err == nil {
			return &t
		}
	}
	return nil
}

func modeString(counts map[string]int) string {
	var best string
	var bestCount int
	for k, v := range counts {
		if v > bestCount {
			best = k
			bestCount = v
		}
	}
	return best
}

func modeInt(counts map[int]int) int {
	var best, bestCount int
	for k, v := range counts {
		if v > bestCount {
			best = k
			bestCount = v
		}
	}
	return best
}
```

Add `"regexp"` and `"strings"` to the import block if not already present.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/service/ -run TestDetectSprintPattern -v`
Expected: all 5 tests PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/sprint_generation.go backend/internal/service/sprint_generation_test.go
git commit -m "feat: add detectSprintPattern with prefix/duration auto-detection"
```

---

### Task 2: Refactor `generateSprintSlots` and `nextMonday` for dynamic duration and timezone

**Files:**
- Modify: `backend/internal/service/sprint_generation.go` (refactor existing functions)
- Modify: `backend/internal/service/sprint_generation_test.go` (add tests)

**Interfaces:**
- Consumes: nothing new
- Produces: `generateSprintSlots(startDate time.Time, durationDays int, year int) []sprintSlot` — signature changes from `(ano int, after time.Time)` to `(startDate time.Time, durationDays int, year int)`. Used by Tasks 3 and 4.

- [ ] **Step 1: Write failing tests for refactored `generateSprintSlots`**

Add to `backend/internal/service/sprint_generation_test.go`:

```go
import "time"

func TestGenerateSprintSlots_DynamicDuration(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	start := time.Date(2026, 8, 3, 0, 0, 0, 0, loc)
	slots := generateSprintSlots(start, 11, 2026)

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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/service/ -run TestGenerateSprintSlots -v`
Expected: FAIL — signature mismatch (old takes `(int, time.Time)`, new expects `(time.Time, int, int)`)

- [ ] **Step 3: Refactor `generateSprintSlots` and `nextMonday`**

Replace the existing `generateSprintSlots` and `nextMonday` functions in `backend/internal/service/sprint_generation.go`:

```go
var saoPaulo *time.Location

func init() {
	var err error
	saoPaulo, err = time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		panic("failed to load America/Sao_Paulo timezone: " + err.Error())
	}
}

func generateSprintSlots(startDate time.Time, durationDays int, year int) []sprintSlot {
	start := nextMonday(startDate)
	yearEnd := time.Date(year, 12, 31, 23, 59, 59, 0, saoPaulo)

	var slots []sprintSlot
	for !start.After(yearEnd) {
		end := start.AddDate(0, 0, durationDays-1)
		if end.After(yearEnd) {
			break
		}
		slotStart := time.Date(start.Year(), start.Month(), start.Day(), 8, 30, 0, 0, saoPaulo)
		slotEnd := time.Date(end.Year(), end.Month(), end.Day(), 18, 30, 0, 0, saoPaulo)
		slots = append(slots, sprintSlot{start: slotStart, end: slotEnd})
		start = end.AddDate(0, 0, 3)
	}
	return slots
}

func nextMonday(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, saoPaulo)
	wd := d.Weekday()
	if wd == time.Monday {
		return d
	}
	daysUntilMonday := (8 - int(wd)) % 7
	if daysUntilMonday == 0 {
		daysUntilMonday = 7
	}
	return d.AddDate(0, 0, daysUntilMonday)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/service/ -run TestGenerateSprintSlots -v`
Expected: all 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/sprint_generation.go backend/internal/service/sprint_generation_test.go
git commit -m "refactor: generateSprintSlots with dynamic duration and America/Sao_Paulo timezone"
```

---

### Task 3: Refactor `filterExistingSlots` to use JIRA dates and update `PreviewSprints`/`GenerateSprints`

**Files:**
- Modify: `backend/internal/service/sprint_generation.go` (refactor PreviewSprints, GenerateSprints, filterExistingSlots, update PreviewResult)
- Modify: `backend/internal/service/sprint_generation_test.go` (add tests)

**Interfaces:**
- Consumes: `detectSprintPattern(sprints []jira.JiraSprint) (string, int, error)` from Task 1, `generateSprintSlots(startDate time.Time, durationDays int, year int) []sprintSlot` from Task 2, `parseJiraDate(s *string) *time.Time` from Task 1, `jira.Client.GetBoardSprints(ctx, boardID) ([]JiraSprint, error)` existing
- Produces: `PreviewSprints(ctx, equipeID, boardID, startDate time.Time) (*PreviewResult, error)` and `GenerateSprints(ctx, equipeID, boardID, startDate time.Time) (*GenerateResult, error)` — new signatures without `prefixo`, with `startDate`. `PreviewResult` gains `PrefixoDetectado string` and `DuracaoDetectadaDias int` fields.

- [ ] **Step 1: Write failing tests for refactored `PreviewSprints`**

Add to `backend/internal/service/sprint_generation_test.go`:

```go
import (
	"context"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

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
	if result.DuracaoDetectadaDias != 11 {
		t.Errorf("duracao = %d, want 11", result.DuracaoDetectadaDias)
	}
	if result.ExistentesIgnoradas != 2 {
		t.Errorf("ignoradas = %d, want 2", result.ExistentesIgnoradas)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/service/ -run TestPreviewSprints_UsesJiraAPI -v`
Expected: FAIL — `previewSprintsWithClient` not defined, `PrefixoDetectado` field not found

- [ ] **Step 3: Update `PreviewResult` struct and refactor `filterExistingSlots`**

In `backend/internal/service/sprint_generation.go`, update `PreviewResult`:

```go
type PreviewResult struct {
	PrefixoDetectado    string       `json:"prefixo_detectado"`
	DuracaoDetectadaDias int         `json:"duracao_detectada_dias"`
	Sprints             []SprintSlot `json:"sprints"`
	ExistentesIgnoradas int          `json:"existentes_ignoradas"`
}
```

Update `filterExistingSlots` to use `parseJiraDate` instead of `parseOptionalDate`:

```go
func filterExistingSlots(slots []sprintSlot, existing []jira.JiraSprint) ([]sprintSlot, int) {
	ignored := 0
	var missing []sprintSlot
	for _, slot := range slots {
		overlaps := false
		for _, ex := range existing {
			exStart := parseJiraDate(ex.StartDate)
			exEnd := parseJiraDate(ex.EndDate)
			if exStart == nil || exEnd == nil {
				continue
			}
			if slot.start.Before(*exEnd) && slot.end.After(*exStart) {
				overlaps = true
				break
			}
		}
		if overlaps {
			ignored++
		} else {
			missing = append(missing, slot)
		}
	}
	return missing, ignored
}
```

- [ ] **Step 4: Implement `previewSprintsWithClient` and refactor `PreviewSprints`**

Replace `PreviewSprints` and add `previewSprintsWithClient` in `backend/internal/service/sprint_generation.go`:

```go
func (s *SprintGenerationService) PreviewSprints(ctx context.Context, equipeID uuid.UUID, boardID int, startDate time.Time) (*PreviewResult, error) {
	fdID, err := s.getFonteDadosForEquipe(ctx, equipeID)
	if err != nil {
		return nil, err
	}
	client, err := s.buildClient(ctx, fdID)
	if err != nil {
		return nil, err
	}
	return s.previewSprintsWithClient(ctx, client, boardID, startDate)
}

func (s *SprintGenerationService) previewSprintsWithClient(ctx context.Context, client jira.Client, boardID int, startDate time.Time) (*PreviewResult, error) {
	existing, err := client.GetBoardSprints(ctx, boardID)
	if err != nil {
		return nil, fmt.Errorf("fetching board sprints from JIRA: %w", err)
	}

	prefix, durationDays, err := detectSprintPattern(existing)
	if err != nil {
		return nil, err
	}

	year := startDate.Year()
	slots := generateSprintSlots(startDate, durationDays, year)
	missing, ignored := filterExistingSlots(slots, existing)

	result := &PreviewResult{
		PrefixoDetectado:    prefix,
		DuracaoDetectadaDias: durationDays,
		Sprints:             make([]SprintSlot, 0, len(missing)),
		ExistentesIgnoradas: ignored,
	}
	for _, slot := range missing {
		result.Sprints = append(result.Sprints, SprintSlot{
			Nome:       formatSprintName(prefix, slot.start, slot.end, year),
			DataInicio: slot.start.Format(time.RFC3339),
			DataFim:    slot.end.Format(time.RFC3339),
		})
	}
	return result, nil
}
```

- [ ] **Step 5: Refactor `GenerateSprints`**

Replace `GenerateSprints` in `backend/internal/service/sprint_generation.go`:

```go
func (s *SprintGenerationService) GenerateSprints(ctx context.Context, equipeID uuid.UUID, boardID int, startDate time.Time) (*GenerateResult, error) {
	fdID, err := s.getFonteDadosForEquipe(ctx, equipeID)
	if err != nil {
		return nil, err
	}
	client, err := s.buildClient(ctx, fdID)
	if err != nil {
		return nil, err
	}

	existing, err := client.GetBoardSprints(ctx, boardID)
	if err != nil {
		return nil, fmt.Errorf("fetching board sprints from JIRA: %w", err)
	}

	prefix, durationDays, err := detectSprintPattern(existing)
	if err != nil {
		return nil, err
	}

	year := startDate.Year()
	slots := generateSprintSlots(startDate, durationDays, year)
	missing, _ := filterExistingSlots(slots, existing)

	result := &GenerateResult{Erros: make([]string, 0)}
	for _, slot := range missing {
		name := formatSprintName(prefix, slot.start, slot.end, year)
		created, err := client.CreateSprint(ctx, boardID, name, slot.start, slot.end)
		if err != nil {
			result.Erros = append(result.Erros, fmt.Sprintf("%s: %v", name, err))
			s.logger.Warn("failed to create sprint", zap.String("name", name), zap.Error(err))
			continue
		}

		_, upsertErr := s.syncRepo.UpsertSprint(ctx, fdID, created.ID, name, nil, &slot.start, &slot.end, nil, &boardID, nil)
		if upsertErr != nil {
			s.logger.Warn("sprint created in jira but failed local upsert", zap.String("name", name), zap.Error(upsertErr))
		}
		result.Criadas++
	}
	return result, nil
}
```

- [ ] **Step 6: Run all tests**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/service/ -v`
Expected: all tests PASS

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/sprint_generation.go backend/internal/service/sprint_generation_test.go
git commit -m "refactor: PreviewSprints/GenerateSprints use JIRA API for overlap and auto-detect pattern"
```

---

### Task 4: Refactor handler to accept `start_date` instead of `prefixo`

**Files:**
- Modify: `backend/internal/handler/sprint_generation.go`

**Interfaces:**
- Consumes: `SprintGenerationService.PreviewSprints(ctx, equipeID, boardID, startDate time.Time)` and `GenerateSprints(ctx, equipeID, boardID, startDate time.Time)` from Task 3
- Produces: HTTP handlers with new request format `{equipe_id, board_id, start_date}` — consumed by frontend (Task 5)

- [ ] **Step 1: Update `generateRequest` struct and handler methods**

Replace the full content of `backend/internal/handler/sprint_generation.go`:

```go
package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"go.uber.org/zap"
)

type SprintGenerationHandler struct {
	svc    *service.SprintGenerationService
	logger *zap.Logger
}

func NewSprintGenerationHandler(svc *service.SprintGenerationService, logger *zap.Logger) *SprintGenerationHandler {
	return &SprintGenerationHandler{svc: svc, logger: logger}
}

func (h *SprintGenerationHandler) GetBoards(w http.ResponseWriter, r *http.Request) {
	equipeStr := r.URL.Query().Get("equipe_id")
	if equipeStr == "" {
		respondError(w, http.StatusBadRequest, "equipe_id is required")
		return
	}
	equipeID, err := uuid.Parse(equipeStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid equipe_id")
		return
	}

	boards, err := h.svc.GetBoardsForEquipe(r.Context(), equipeID)
	if err != nil {
		h.logger.Error("getting boards for equipe", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get boards")
		return
	}
	respondJSON(w, http.StatusOK, boards)
}

type generateRequest struct {
	EquipeID  uuid.UUID `json:"equipe_id"`
	BoardID   int       `json:"board_id"`
	StartDate string    `json:"start_date"`
}

func (h *SprintGenerationHandler) Preview(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	startDate, err := h.validateRequest(req)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.svc.PreviewSprints(r.Context(), req.EquipeID, req.BoardID, startDate)
	if err != nil {
		h.logger.Error("previewing sprints", zap.Error(err))
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func (h *SprintGenerationHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	startDate, err := h.validateRequest(req)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.svc.GenerateSprints(r.Context(), req.EquipeID, req.BoardID, startDate)
	if err != nil {
		h.logger.Error("generating sprints", zap.Error(err))
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func (h *SprintGenerationHandler) validateRequest(req generateRequest) (time.Time, error) {
	if req.EquipeID == uuid.Nil {
		return time.Time{}, fmt.Errorf("equipe_id is required")
	}
	if req.BoardID == 0 {
		return time.Time{}, fmt.Errorf("board_id is required")
	}
	if req.StartDate == "" {
		return time.Time{}, fmt.Errorf("start_date is required")
	}
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("start_date must be in YYYY-MM-DD format")
	}
	today := time.Now().Truncate(24 * time.Hour)
	if startDate.Before(today) {
		return time.Time{}, fmt.Errorf("Data inicial não pode ser no passado")
	}
	return startDate, nil
}
```

Add `"fmt"` to the import block.

- [ ] **Step 2: Verify compilation**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: no errors

- [ ] **Step 3: Run all tests**

Run: `cd /home/emerson/code/myplanner/backend && go test ./... -v`
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handler/sprint_generation.go
git commit -m "refactor: handler accepts start_date instead of prefixo"
```

---

### Task 5: Refactor frontend form and preview display

**Files:**
- Modify: `frontend/index.html` (lines 2374-2467: `openGerarSprintsForm`, `previewGerarSprints`, `executarGerarSprints`)

**Interfaces:**
- Consumes: `POST /sprints/generate/preview` returns `{prefixo_detectado, duracao_detectada_dias, sprints: [{nome, data_inicio, data_fim}], existentes_ignoradas}`. `POST /sprints/generate` returns `{criadas, erros}`. Dates in `sprints` array are RFC3339 strings.
- Produces: Updated UI with date picker instead of prefixo field, dates displayed as `dd/MM/yyyy HH:mm:ss`

- [ ] **Step 1: Add `fmtDateTimeBR` helper function**

Find the existing `fmtDateBR` function (line 818) and add `fmtDateTimeBR` right after it:

```javascript
function fmtDateTimeBR(s) {
  if (!s) return '';
  var d = new Date(s);
  if (isNaN(d)) return s;
  var dd = String(d.getDate()).padStart(2, '0');
  var mm = String(d.getMonth() + 1).padStart(2, '0');
  var yyyy = d.getFullYear();
  var hh = String(d.getHours()).padStart(2, '0');
  var mi = String(d.getMinutes()).padStart(2, '0');
  var ss = String(d.getSeconds()).padStart(2, '0');
  return dd + '/' + mm + '/' + yyyy + ' ' + hh + ':' + mi + ':' + ss;
}
```

- [ ] **Step 2: Replace `openGerarSprintsForm`**

Replace the `openGerarSprintsForm` function (line 2374-2393) with:

```javascript
async function openGerarSprintsForm() {
  var equipe = document.getElementById('stl-equipe').value;
  if (!equipe) { alert('Selecione uma equipe primeiro.'); return; }
  var el = document.getElementById('stl-content');
  var prevHTML = el.innerHTML;
  var today = new Date().toISOString().split('T')[0];
  el.innerHTML = '<div style="background:var(--card-bg);border:1px solid var(--border-subtle);border-radius:8px;padding:20px;max-width:700px">'
    + '<h3 style="margin:0 0 16px">Gerar Sprints</h3>'
    + '<div style="display:flex;gap:12px;align-items:flex-end;flex-wrap:wrap">'
    + '<div><label style="font-size:12px;color:var(--text-secondary)">Board ID</label><br>'
    + '<input id="gen-board" type="number" placeholder="Ex: 42" style="padding:6px 10px;border:1px solid var(--border-subtle);border-radius:4px;background:var(--card-bg);color:var(--text-primary);font-size:13px;width:100px">'
    + '<div style="font-size:10px;color:var(--text-tertiary);margin-top:2px">Da URL do board JIRA: /boards/<b>ID</b></div></div>'
    + '<div><label style="font-size:12px;color:var(--text-secondary)">Data Inicial</label><br>'
    + '<input id="gen-start-date" type="date" min="' + today + '" style="padding:6px 10px;border:1px solid var(--border-subtle);border-radius:4px;background:var(--card-bg);color:var(--text-primary);font-size:13px;width:160px"></div>'
    + '<button class="btn-sm" style="padding:6px 16px;font-size:13px" onclick="previewGerarSprints()">Calcular</button>'
    + '<button class="btn-sm" style="padding:6px 16px;font-size:13px;background:var(--border-subtle)" onclick="cancelGerarSprints()">Cancelar</button>'
    + '</div>'
    + '<div id="gen-preview" style="margin-top:16px"></div>'
    + '</div>';
  el.dataset.prevHtml = prevHTML;
}
```

- [ ] **Step 3: Replace `previewGerarSprints`**

Replace the `previewGerarSprints` function (line 2408-2443) with:

```javascript
async function previewGerarSprints() {
  var equipe = document.getElementById('stl-equipe').value;
  var boardId = document.getElementById('gen-board').value.trim();
  var startDate = document.getElementById('gen-start-date').value;
  if (!boardId || isNaN(parseInt(boardId))) { alert('Informe o Board ID (número).'); return; }
  if (!startDate) { alert('Informe a Data Inicial.'); return; }
  var previewEl = document.getElementById('gen-preview');
  previewEl.innerHTML = '<div class="loading"><div class="spinner"></div></div>';
  try {
    var result = await api('/sprints/generate/preview', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ equipe_id: equipe, board_id: parseInt(boardId), start_date: startDate })
    });
    if (!result.sprints || result.sprints.length === 0) {
      previewEl.innerHTML = '<p style="color:var(--text-secondary);font-size:13px">Todas as sprints até o final do ano já existem neste board.'
        + (result.existentes_ignoradas > 0 ? ' (' + result.existentes_ignoradas + ' existentes ignoradas)' : '') + '</p>';
      return;
    }
    var html = '<div style="font-size:13px;color:var(--text-secondary);margin-bottom:12px">'
      + '<div>Prefixo detectado: <strong>' + esc(result.prefixo_detectado) + '</strong></div>'
      + '<div>Duração detectada: <strong>' + result.duracao_detectada_dias + ' dias</strong></div>'
      + '<div style="margin-top:4px">' + result.sprints.length + ' sprints a criar'
      + (result.existentes_ignoradas > 0 ? ' (' + result.existentes_ignoradas + ' existentes ignoradas)' : '') + '</div></div>';
    html += '<div style="max-height:300px;overflow-y:auto"><table style="width:100%;font-size:13px;border-collapse:collapse">';
    html += '<tr style="text-align:left;border-bottom:1px solid var(--border-subtle)"><th style="padding:4px 8px">Nome</th><th style="padding:4px 8px">Início</th><th style="padding:4px 8px">Fim</th></tr>';
    result.sprints.forEach(function(s) {
      html += '<tr style="border-bottom:1px solid var(--border-subtle)">'
        + '<td style="padding:4px 8px">' + esc(s.nome) + '</td>'
        + '<td style="padding:4px 8px">' + fmtDateTimeBR(s.data_inicio) + '</td>'
        + '<td style="padding:4px 8px">' + fmtDateTimeBR(s.data_fim) + '</td></tr>';
    });
    html += '</table></div>';
    html += '<button class="btn-sm" style="margin-top:12px;padding:6px 20px;font-size:13px;background:var(--blue);color:#fff" onclick="executarGerarSprints()">Criar ' + result.sprints.length + ' Sprints no JIRA</button>';
    previewEl.innerHTML = html;
  } catch(err) {
    previewEl.innerHTML = '<p style="color:var(--red)">Erro: ' + esc(err.message) + '</p>';
  }
}
```

- [ ] **Step 4: Replace `executarGerarSprints`**

Replace the `executarGerarSprints` function (line 2446-2467) with:

```javascript
async function executarGerarSprints() {
  var equipe = document.getElementById('stl-equipe').value;
  var boardId = document.getElementById('gen-board').value.trim();
  var startDate = document.getElementById('gen-start-date').value;
  var previewEl = document.getElementById('gen-preview');
  previewEl.innerHTML = '<div class="loading"><div class="spinner"></div></div><p style="font-size:13px;color:var(--text-secondary)">Criando sprints no JIRA...</p>';
  try {
    var result = await api('/sprints/generate', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ equipe_id: equipe, board_id: parseInt(boardId), start_date: startDate })
    });
    var msg = result.criadas + ' sprints criadas com sucesso.';
    if (result.erros && result.erros.length > 0) {
      msg += '\n\nErros:\n' + result.erros.join('\n');
    }
    alert(msg);
    loadSprintsTimeline();
  } catch(err) {
    previewEl.innerHTML = '<p style="color:var(--red)">Erro: ' + esc(err.message) + '</p>';
  }
}
```

- [ ] **Step 5: Verify no compilation errors**

Open `frontend/index.html` in browser, check no JS errors in console on page load.

- [ ] **Step 6: Commit**

```bash
git add frontend/index.html
git commit -m "refactor: frontend uses start_date picker, shows detected pattern, dates as dd/MM/yyyy HH:mm:ss"
```

---

### Task 6: Remove `sprintRepo` dependency from `SprintGenerationService`

**Files:**
- Modify: `backend/internal/service/sprint_generation.go` (remove `sprintRepo` field, keep it only for `GetBoardsForEquipe`)
- Modify: `backend/cmd/api/main.go` (no change needed — `sprintRepo` is still needed for `GetBoardsForEquipe`)

**Interfaces:**
- Consumes: nothing new
- Produces: cleaner service struct (no behavioral change)

- [ ] **Step 1: Verify `sprintRepo` usage**

`sprintRepo` is still used by `GetBoardsForEquipe` (calls `sprintRepo.ListProjetosComSprints`). It is NO longer used by `PreviewSprints` or `GenerateSprints` after Task 3 refactor. The field stays — no code change needed here.

- [ ] **Step 2: Run full test suite**

Run: `cd /home/emerson/code/myplanner/backend && go test ./... -v`
Expected: all PASS

Run: `cd /home/emerson/code/myplanner/backend && go vet ./...`
Expected: no issues

- [ ] **Step 3: Commit if any cleanup was needed**

Only commit if changes were made. Otherwise skip.
