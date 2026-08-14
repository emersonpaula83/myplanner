package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/jira"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockSprintGenService struct {
	getBoardsForEquipeFn func(ctx context.Context, equipeID uuid.UUID) ([]jira.JiraBoard, error)
	previewSprintsFn     func(ctx context.Context, equipeID uuid.UUID, boardID int, startDate time.Time) (*service.PreviewResult, error)
	generateSprintsFn    func(ctx context.Context, equipeID uuid.UUID, boardID int, startDate time.Time) (*service.GenerateResult, error)
}

func (m *mockSprintGenService) GetBoardsForEquipe(ctx context.Context, equipeID uuid.UUID) ([]jira.JiraBoard, error) {
	return m.getBoardsForEquipeFn(ctx, equipeID)
}

func (m *mockSprintGenService) PreviewSprints(ctx context.Context, equipeID uuid.UUID, boardID int, startDate time.Time) (*service.PreviewResult, error) {
	return m.previewSprintsFn(ctx, equipeID, boardID, startDate)
}

func (m *mockSprintGenService) GenerateSprints(ctx context.Context, equipeID uuid.UUID, boardID int, startDate time.Time) (*service.GenerateResult, error) {
	return m.generateSprintsFn(ctx, equipeID, boardID, startDate)
}

const futureStartDate = "2099-01-01"

// --- GetBoards ---

func TestSprintGenerationHandler_GetBoards(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockSprintGenService{
		getBoardsForEquipeFn: func(ctx context.Context, e uuid.UUID) ([]jira.JiraBoard, error) {
			return []jira.JiraBoard{{ID: 1, Name: "Board"}}, nil
		},
	}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprint-generation/boards?equipe_id="+equipeID.String(), nil)
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.GetBoards(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSprintGenerationHandler_GetBoards_MissingEquipeID(t *testing.T) {
	svc := &mockSprintGenService{}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprint-generation/boards", nil)
	w := httptest.NewRecorder()
	h.GetBoards(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSprintGenerationHandler_GetBoards_InvalidEquipeID(t *testing.T) {
	svc := &mockSprintGenService{}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprint-generation/boards?equipe_id=bad", nil)
	w := httptest.NewRecorder()
	h.GetBoards(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSprintGenerationHandler_GetBoards_Forbidden(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockSprintGenService{}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprint-generation/boards?equipe_id="+equipeID.String(), nil)
	w := httptest.NewRecorder()
	h.GetBoards(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSprintGenerationHandler_GetBoards_Error(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockSprintGenService{
		getBoardsForEquipeFn: func(ctx context.Context, e uuid.UUID) ([]jira.JiraBoard, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprint-generation/boards?equipe_id="+equipeID.String(), nil)
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.GetBoards(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- Preview ---

func TestSprintGenerationHandler_Preview(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockSprintGenService{
		previewSprintsFn: func(ctx context.Context, e uuid.UUID, boardID int, startDate time.Time) (*service.PreviewResult, error) {
			return &service.PreviewResult{}, nil
		},
	}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(generateRequest{EquipeID: equipeID, BoardID: 1, StartDate: futureStartDate})
	req := httptest.NewRequest("POST", "/sprint-generation/preview", bytes.NewReader(body))
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.Preview(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSprintGenerationHandler_Preview_InvalidBody(t *testing.T) {
	svc := &mockSprintGenService{}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("POST", "/sprint-generation/preview", bytes.NewReader([]byte("not-json")))
	w := httptest.NewRecorder()
	h.Preview(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSprintGenerationHandler_Preview_ValidationError(t *testing.T) {
	svc := &mockSprintGenService{}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(generateRequest{})
	req := httptest.NewRequest("POST", "/sprint-generation/preview", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Preview(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSprintGenerationHandler_Preview_Forbidden(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockSprintGenService{}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(generateRequest{EquipeID: equipeID, BoardID: 1, StartDate: futureStartDate})
	req := httptest.NewRequest("POST", "/sprint-generation/preview", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Preview(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSprintGenerationHandler_Preview_Error(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockSprintGenService{
		previewSprintsFn: func(ctx context.Context, e uuid.UUID, boardID int, startDate time.Time) (*service.PreviewResult, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(generateRequest{EquipeID: equipeID, BoardID: 1, StartDate: futureStartDate})
	req := httptest.NewRequest("POST", "/sprint-generation/preview", bytes.NewReader(body))
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.Preview(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- Generate ---

func TestSprintGenerationHandler_Generate(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockSprintGenService{
		generateSprintsFn: func(ctx context.Context, e uuid.UUID, boardID int, startDate time.Time) (*service.GenerateResult, error) {
			return &service.GenerateResult{Criadas: 2}, nil
		},
	}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(generateRequest{EquipeID: equipeID, BoardID: 1, StartDate: futureStartDate})
	req := httptest.NewRequest("POST", "/sprint-generation/generate", bytes.NewReader(body))
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.Generate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSprintGenerationHandler_Generate_InvalidBody(t *testing.T) {
	svc := &mockSprintGenService{}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("POST", "/sprint-generation/generate", bytes.NewReader([]byte("not-json")))
	w := httptest.NewRecorder()
	h.Generate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSprintGenerationHandler_Generate_ValidationError(t *testing.T) {
	svc := &mockSprintGenService{}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(generateRequest{EquipeID: uuid.New(), BoardID: 0, StartDate: futureStartDate})
	req := httptest.NewRequest("POST", "/sprint-generation/generate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Generate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSprintGenerationHandler_Generate_Forbidden(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockSprintGenService{}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(generateRequest{EquipeID: equipeID, BoardID: 1, StartDate: futureStartDate})
	req := httptest.NewRequest("POST", "/sprint-generation/generate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Generate(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestSprintGenerationHandler_Generate_Error(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockSprintGenService{
		generateSprintsFn: func(ctx context.Context, e uuid.UUID, boardID int, startDate time.Time) (*service.GenerateResult, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewSprintGenerationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(generateRequest{EquipeID: equipeID, BoardID: 1, StartDate: futureStartDate})
	req := httptest.NewRequest("POST", "/sprint-generation/generate", bytes.NewReader(body))
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.Generate(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- validateRequest (unexported helper, same package) ---

func TestSprintGenerationHandler_validateRequest(t *testing.T) {
	h := &SprintGenerationHandler{}

	if _, err := h.validateRequest(generateRequest{}); err == nil {
		t.Error("expected error for missing equipe_id")
	}
	if _, err := h.validateRequest(generateRequest{EquipeID: uuid.New()}); err == nil {
		t.Error("expected error for missing board_id")
	}
	if _, err := h.validateRequest(generateRequest{EquipeID: uuid.New(), BoardID: 1}); err == nil {
		t.Error("expected error for missing start_date")
	}
	if _, err := h.validateRequest(generateRequest{EquipeID: uuid.New(), BoardID: 1, StartDate: "bad-date"}); err == nil {
		t.Error("expected error for bad start_date format")
	}
	if _, err := h.validateRequest(generateRequest{EquipeID: uuid.New(), BoardID: 1, StartDate: "2000-01-01"}); err == nil {
		t.Error("expected error for past start_date")
	}
	startDate, err := h.validateRequest(generateRequest{EquipeID: uuid.New(), BoardID: 1, StartDate: futureStartDate})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	expected, _ := time.Parse("2006-01-02", futureStartDate)
	if !startDate.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, startDate)
	}
}
