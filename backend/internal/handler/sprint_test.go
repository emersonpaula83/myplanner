package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockSprintStore struct {
	listProjetosFn       func(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error)
	listByProjetoFn      func(ctx context.Context, projetoID uuid.UUID, estado *string) ([]repository.SprintListItem, error)
	listSprintsFn        func(ctx context.Context, equipeID *uuid.UUID, estado *string) ([]repository.SprintListItem, error)
	getCapacityFn        func(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*service.SprintCapacityResult, error)
	getUnplannedFn       func(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*service.UnplannedAnalysisResult, error)
	getBurndownFn        func(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*service.BurndownResult, error)
	getSprintsTimelineFn func(ctx context.Context, equipeID uuid.UUID, ano int) ([]service.SprintTimelineItem, error)
	getDisclaimerTasksFn func(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, taskType string) (*service.DisclaimerTasksResult, error)
	getTimelineDetailFn  func(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID) (*service.TimelineDetailResult, error)
}

func (m *mockSprintStore) ListProjetosComSprints(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error) {
	return m.listProjetosFn(ctx, equipeID)
}

func (m *mockSprintStore) ListByProjeto(ctx context.Context, projetoID uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
	return m.listByProjetoFn(ctx, projetoID, estado)
}

func (m *mockSprintStore) ListSprints(ctx context.Context, equipeID *uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
	return m.listSprintsFn(ctx, equipeID, estado)
}

func (m *mockSprintStore) GetCapacity(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*service.SprintCapacityResult, error) {
	return m.getCapacityFn(ctx, sprintID, equipeID)
}

func (m *mockSprintStore) GetUnplannedAnalysis(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*service.UnplannedAnalysisResult, error) {
	return m.getUnplannedFn(ctx, sprintID, equipeID)
}

func (m *mockSprintStore) GetBurndown(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*service.BurndownResult, error) {
	return m.getBurndownFn(ctx, sprintID, equipeID)
}

func (m *mockSprintStore) GetSprintsTimeline(ctx context.Context, equipeID uuid.UUID, ano int) ([]service.SprintTimelineItem, error) {
	return m.getSprintsTimelineFn(ctx, equipeID, ano)
}

func (m *mockSprintStore) GetDisclaimerTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, taskType string) (*service.DisclaimerTasksResult, error) {
	return m.getDisclaimerTasksFn(ctx, sprintID, equipeID, taskType)
}

func (m *mockSprintStore) GetTimelineDetail(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID) (*service.TimelineDetailResult, error) {
	return m.getTimelineDetailFn(ctx, sprintID, equipeID)
}

func addChiParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		rctx = chi.NewRouteContext()
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	}
	rctx.URLParams.Add(key, value)
	return r
}

func withEquipeAccess(r *http.Request, equipeIDs ...uuid.UUID) *http.Request {
	return r.WithContext(middleware.ContextWithEquipeIDs(r.Context(), equipeIDs))
}

// --- ListProjetos ---

func TestSprintHandler_ListProjetos(t *testing.T) {
	store := &mockSprintStore{
		listProjetosFn: func(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error) {
			return []repository.ProjetoComSprints{{ID: uuid.New(), Chave: "ABC", Nome: "Projeto"}}, nil
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/projetos", nil)
	w := httptest.NewRecorder()
	h.ListProjetos(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSprintHandler_ListProjetos_Error(t *testing.T) {
	store := &mockSprintStore{
		listProjetosFn: func(ctx context.Context, equipeID *uuid.UUID) ([]repository.ProjetoComSprints, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/projetos", nil)
	w := httptest.NewRecorder()
	h.ListProjetos(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- ListSprints ---

func TestSprintHandler_ListSprints(t *testing.T) {
	store := &mockSprintStore{
		listSprintsFn: func(ctx context.Context, equipeID *uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
			return []repository.SprintListItem{{ID: uuid.New(), Nome: "Sprint 1"}}, nil
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprints", nil)
	w := httptest.NewRecorder()
	h.ListSprints(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSprintHandler_ListSprints_Error(t *testing.T) {
	store := &mockSprintStore{
		listSprintsFn: func(ctx context.Context, equipeID *uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprints", nil)
	w := httptest.NewRecorder()
	h.ListSprints(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- ListByProjeto ---

func TestSprintHandler_ListByProjeto(t *testing.T) {
	projetoID := uuid.New()
	store := &mockSprintStore{
		listByProjetoFn: func(ctx context.Context, id uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
			if id != projetoID {
				t.Errorf("unexpected projeto id: %s", id)
			}
			return []repository.SprintListItem{{ID: uuid.New(), Nome: "Sprint 1"}}, nil
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/projetos/"+projetoID.String()+"/sprints", nil)
	req = addChiParam(req, "id", projetoID.String())
	w := httptest.NewRecorder()
	h.ListByProjeto(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSprintHandler_ListByProjeto_Error(t *testing.T) {
	projetoID := uuid.New()
	store := &mockSprintStore{
		listByProjetoFn: func(ctx context.Context, id uuid.UUID, estado *string) ([]repository.SprintListItem, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/projetos/"+projetoID.String()+"/sprints", nil)
	req = addChiParam(req, "id", projetoID.String())
	w := httptest.NewRecorder()
	h.ListByProjeto(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestSprintHandler_ListByProjeto_InvalidID(t *testing.T) {
	store := &mockSprintStore{}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/projetos/not-a-uuid/sprints", nil)
	req = addChiParam(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()
	h.ListByProjeto(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- GetCapacity ---

func TestSprintHandler_GetCapacity(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	store := &mockSprintStore{
		getCapacityFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID) (*service.SprintCapacityResult, error) {
			return &service.SprintCapacityResult{}, nil
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/capacity?equipe="+equipeID.String(), nil)
	req = addChiParam(req, "id", sprintID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.GetCapacity(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSprintHandler_GetCapacity_Error(t *testing.T) {
	sprintID := uuid.New()
	store := &mockSprintStore{
		getCapacityFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID) (*service.SprintCapacityResult, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/capacity", nil)
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.GetCapacity(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- GetUnplanned ---

func TestSprintHandler_GetUnplanned(t *testing.T) {
	sprintID := uuid.New()
	store := &mockSprintStore{
		getUnplannedFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID) (*service.UnplannedAnalysisResult, error) {
			return &service.UnplannedAnalysisResult{EquipeNome: "Equipe A"}, nil
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/unplanned", nil)
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.GetUnplanned(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSprintHandler_GetUnplanned_Error(t *testing.T) {
	sprintID := uuid.New()
	store := &mockSprintStore{
		getUnplannedFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID) (*service.UnplannedAnalysisResult, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/unplanned", nil)
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.GetUnplanned(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- GetDisclaimerTasks ---

func TestSprintHandler_GetDisclaimerTasks(t *testing.T) {
	sprintID := uuid.New()
	store := &mockSprintStore{
		getDisclaimerTasksFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID, taskType string) (*service.DisclaimerTasksResult, error) {
			if taskType != "manutencao" {
				t.Errorf("unexpected task type: %s", taskType)
			}
			return &service.DisclaimerTasksResult{}, nil
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/disclaimer?type=manutencao", nil)
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.GetDisclaimerTasks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSprintHandler_GetDisclaimerTasks_Error(t *testing.T) {
	sprintID := uuid.New()
	store := &mockSprintStore{
		getDisclaimerTasksFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID, taskType string) (*service.DisclaimerTasksResult, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/disclaimer?type=outras", nil)
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.GetDisclaimerTasks(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestSprintHandler_GetDisclaimerTasks_InvalidType(t *testing.T) {
	sprintID := uuid.New()
	store := &mockSprintStore{}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/disclaimer?type=bogus", nil)
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.GetDisclaimerTasks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- GetBurndown ---

func TestSprintHandler_GetBurndown(t *testing.T) {
	sprintID := uuid.New()
	store := &mockSprintStore{
		getBurndownFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID) (*service.BurndownResult, error) {
			return &service.BurndownResult{SprintNome: "Sprint 1"}, nil
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/burndown", nil)
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.GetBurndown(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSprintHandler_GetBurndown_Error(t *testing.T) {
	sprintID := uuid.New()
	store := &mockSprintStore{
		getBurndownFn: func(ctx context.Context, sid uuid.UUID, eid *uuid.UUID) (*service.BurndownResult, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/burndown", nil)
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.GetBurndown(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- GetSprintsTimeline ---

func TestSprintHandler_GetSprintsTimeline(t *testing.T) {
	equipeID := uuid.New()
	store := &mockSprintStore{
		getSprintsTimelineFn: func(ctx context.Context, eid uuid.UUID, ano int) ([]service.SprintTimelineItem, error) {
			if eid != equipeID {
				t.Errorf("unexpected equipe id: %s", eid)
			}
			return []service.SprintTimelineItem{{SprintID: uuid.New()}}, nil
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/timeline?equipe="+equipeID.String()+"&ano=2026", nil)
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.GetSprintsTimeline(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSprintHandler_GetSprintsTimeline_Error(t *testing.T) {
	equipeID := uuid.New()
	store := &mockSprintStore{
		getSprintsTimelineFn: func(ctx context.Context, eid uuid.UUID, ano int) ([]service.SprintTimelineItem, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/timeline?equipe="+equipeID.String(), nil)
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.GetSprintsTimeline(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestSprintHandler_GetSprintsTimeline_MissingEquipe(t *testing.T) {
	store := &mockSprintStore{}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/timeline", nil)
	w := httptest.NewRecorder()
	h.GetSprintsTimeline(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- GetTimelineDetail ---

func TestSprintHandler_GetTimelineDetail(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	store := &mockSprintStore{
		getTimelineDetailFn: func(ctx context.Context, sid, eid uuid.UUID) (*service.TimelineDetailResult, error) {
			if sid != sprintID || eid != equipeID {
				t.Errorf("unexpected ids: sprint=%s equipe=%s", sid, eid)
			}
			return &service.TimelineDetailResult{SprintNome: "Sprint 1"}, nil
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/timeline-detail?equipe="+equipeID.String(), nil)
	req = addChiParam(req, "id", sprintID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.GetTimelineDetail(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSprintHandler_GetTimelineDetail_Error(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	store := &mockSprintStore{
		getTimelineDetailFn: func(ctx context.Context, sid, eid uuid.UUID) (*service.TimelineDetailResult, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/timeline-detail?equipe="+equipeID.String(), nil)
	req = addChiParam(req, "id", sprintID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.GetTimelineDetail(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestSprintHandler_GetTimelineDetail_MissingEquipe(t *testing.T) {
	sprintID := uuid.New()
	store := &mockSprintStore{}
	h := NewSprintHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/timeline-detail", nil)
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.GetTimelineDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
