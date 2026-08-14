package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockAllocationService struct {
	listProjectAllocationsFn func(ctx context.Context, equipeID uuid.UUID, produtoNomes []string, statusFilter string) ([]service.ProjectAllocation, error)
	getProjectDetailFn       func(ctx context.Context, epicID, equipeID uuid.UUID) (*service.ProjectDetail, error)
	allocateTaskFn           func(ctx context.Context, req service.AllocateTaskRequest) (*service.AllocateTaskResult, error)
	syncProjectTasksFn       func(ctx context.Context, epicID uuid.UUID) (int, error)
	getAvailableSprintsFn    func(ctx context.Context, equipeID uuid.UUID) ([]service.SprintOption, error)
	closeProjectFn           func(ctx context.Context, epicID uuid.UUID, req service.CloseProjectRequest, encerradoPor string) error
	reopenProjectFn          func(ctx context.Context, epicID uuid.UUID) error
	getFilteredProductsFn    func(ctx context.Context) ([]repository.ProdutoRow, error)
}

func (m *mockAllocationService) ListProjectAllocations(ctx context.Context, equipeID uuid.UUID, produtoNomes []string, statusFilter string) ([]service.ProjectAllocation, error) {
	return m.listProjectAllocationsFn(ctx, equipeID, produtoNomes, statusFilter)
}

func (m *mockAllocationService) GetProjectDetail(ctx context.Context, epicID, equipeID uuid.UUID) (*service.ProjectDetail, error) {
	return m.getProjectDetailFn(ctx, epicID, equipeID)
}

func (m *mockAllocationService) AllocateTask(ctx context.Context, req service.AllocateTaskRequest) (*service.AllocateTaskResult, error) {
	return m.allocateTaskFn(ctx, req)
}

func (m *mockAllocationService) SyncProjectTasks(ctx context.Context, epicID uuid.UUID) (int, error) {
	return m.syncProjectTasksFn(ctx, epicID)
}

func (m *mockAllocationService) GetAvailableSprints(ctx context.Context, equipeID uuid.UUID) ([]service.SprintOption, error) {
	return m.getAvailableSprintsFn(ctx, equipeID)
}

func (m *mockAllocationService) CloseProject(ctx context.Context, epicID uuid.UUID, req service.CloseProjectRequest, encerradoPor string) error {
	return m.closeProjectFn(ctx, epicID, req, encerradoPor)
}

func (m *mockAllocationService) ReopenProject(ctx context.Context, epicID uuid.UUID) error {
	return m.reopenProjectFn(ctx, epicID)
}

func (m *mockAllocationService) GetFilteredProducts(ctx context.Context) ([]repository.ProdutoRow, error) {
	return m.getFilteredProductsFn(ctx)
}

// --- ListProjects ---

func TestAllocationHandler_ListProjects(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockAllocationService{
		listProjectAllocationsFn: func(ctx context.Context, e uuid.UUID, produtoNomes []string, statusFilter string) ([]service.ProjectAllocation, error) {
			return []service.ProjectAllocation{{EpicID: uuid.New()}}, nil
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/allocation/projects?equipe_id="+equipeID.String()+"&produto_nome=X", nil)
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.ListProjects(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAllocationHandler_ListProjects_MissingProdutoNome(t *testing.T) {
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/allocation/projects", nil)
	w := httptest.NewRecorder()
	h.ListProjects(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocationHandler_ListProjects_InvalidEquipeID(t *testing.T) {
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/allocation/projects?equipe_id=not-a-uuid&produto_nome=X", nil)
	w := httptest.NewRecorder()
	h.ListProjects(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocationHandler_ListProjects_Forbidden(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/allocation/projects?equipe_id="+equipeID.String()+"&produto_nome=X", nil)
	w := httptest.NewRecorder()
	h.ListProjects(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAllocationHandler_ListProjects_Error(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockAllocationService{
		listProjectAllocationsFn: func(ctx context.Context, e uuid.UUID, produtoNomes []string, statusFilter string) ([]service.ProjectAllocation, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/allocation/projects?equipe_id="+equipeID.String()+"&produto_nome=X", nil)
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.ListProjects(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- GetProjectDetail ---

func TestAllocationHandler_GetProjectDetail(t *testing.T) {
	epicID := uuid.New()
	svc := &mockAllocationService{
		getProjectDetailFn: func(ctx context.Context, e, eq uuid.UUID) (*service.ProjectDetail, error) {
			return &service.ProjectDetail{}, nil
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/allocation/projects/"+epicID.String(), nil)
	req = addChiParam(req, "epicId", epicID.String())
	w := httptest.NewRecorder()
	h.GetProjectDetail(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAllocationHandler_GetProjectDetail_InvalidEpicID(t *testing.T) {
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/allocation/projects/bad", nil)
	req = addChiParam(req, "epicId", "bad")
	w := httptest.NewRecorder()
	h.GetProjectDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocationHandler_GetProjectDetail_Error(t *testing.T) {
	epicID := uuid.New()
	svc := &mockAllocationService{
		getProjectDetailFn: func(ctx context.Context, e, eq uuid.UUID) (*service.ProjectDetail, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/allocation/projects/"+epicID.String(), nil)
	req = addChiParam(req, "epicId", epicID.String())
	w := httptest.NewRecorder()
	h.GetProjectDetail(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- AllocateTask ---

func TestAllocationHandler_AllocateTask(t *testing.T) {
	taskID := uuid.New()
	equipeID := uuid.New()
	svc := &mockAllocationService{
		allocateTaskFn: func(ctx context.Context, req service.AllocateTaskRequest) (*service.AllocateTaskResult, error) {
			return &service.AllocateTaskResult{}, nil
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(service.AllocateTaskRequest{
		SprintID:      uuid.New(),
		EstimateHours: 4,
		EquipeID:      equipeID,
	})
	req := httptest.NewRequest("POST", "/allocation/tasks/"+taskID.String()+"/allocate", bytes.NewReader(body))
	req = addChiParam(req, "taskId", taskID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.AllocateTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAllocationHandler_AllocateTask_Conflict(t *testing.T) {
	taskID := uuid.New()
	equipeID := uuid.New()
	svc := &mockAllocationService{
		allocateTaskFn: func(ctx context.Context, req service.AllocateTaskRequest) (*service.AllocateTaskResult, error) {
			return &service.AllocateTaskResult{Conflict: true}, nil
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(service.AllocateTaskRequest{
		SprintID:      uuid.New(),
		EstimateHours: 4,
		EquipeID:      equipeID,
	})
	req := httptest.NewRequest("POST", "/allocation/tasks/"+taskID.String()+"/allocate", bytes.NewReader(body))
	req = addChiParam(req, "taskId", taskID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.AllocateTask(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAllocationHandler_AllocateTask_InvalidTaskID(t *testing.T) {
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("POST", "/allocation/tasks/bad/allocate", nil)
	req = addChiParam(req, "taskId", "bad")
	w := httptest.NewRecorder()
	h.AllocateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocationHandler_AllocateTask_InvalidBody(t *testing.T) {
	taskID := uuid.New()
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("POST", "/allocation/tasks/"+taskID.String()+"/allocate", bytes.NewReader([]byte("not-json")))
	req = addChiParam(req, "taskId", taskID.String())
	w := httptest.NewRecorder()
	h.AllocateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocationHandler_AllocateTask_MissingSprintID(t *testing.T) {
	taskID := uuid.New()
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(service.AllocateTaskRequest{EstimateHours: 4, EquipeID: uuid.New()})
	req := httptest.NewRequest("POST", "/allocation/tasks/"+taskID.String()+"/allocate", bytes.NewReader(body))
	req = addChiParam(req, "taskId", taskID.String())
	w := httptest.NewRecorder()
	h.AllocateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocationHandler_AllocateTask_InvalidEstimate(t *testing.T) {
	taskID := uuid.New()
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(service.AllocateTaskRequest{SprintID: uuid.New(), EstimateHours: 0, EquipeID: uuid.New()})
	req := httptest.NewRequest("POST", "/allocation/tasks/"+taskID.String()+"/allocate", bytes.NewReader(body))
	req = addChiParam(req, "taskId", taskID.String())
	w := httptest.NewRecorder()
	h.AllocateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocationHandler_AllocateTask_MissingEquipeID(t *testing.T) {
	taskID := uuid.New()
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(service.AllocateTaskRequest{SprintID: uuid.New(), EstimateHours: 4})
	req := httptest.NewRequest("POST", "/allocation/tasks/"+taskID.String()+"/allocate", bytes.NewReader(body))
	req = addChiParam(req, "taskId", taskID.String())
	w := httptest.NewRecorder()
	h.AllocateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocationHandler_AllocateTask_Forbidden(t *testing.T) {
	taskID := uuid.New()
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(service.AllocateTaskRequest{SprintID: uuid.New(), EstimateHours: 4, EquipeID: uuid.New()})
	req := httptest.NewRequest("POST", "/allocation/tasks/"+taskID.String()+"/allocate", bytes.NewReader(body))
	req = addChiParam(req, "taskId", taskID.String())
	w := httptest.NewRecorder()
	h.AllocateTask(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAllocationHandler_AllocateTask_Error(t *testing.T) {
	taskID := uuid.New()
	equipeID := uuid.New()
	svc := &mockAllocationService{
		allocateTaskFn: func(ctx context.Context, req service.AllocateTaskRequest) (*service.AllocateTaskResult, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(service.AllocateTaskRequest{SprintID: uuid.New(), EstimateHours: 4, EquipeID: equipeID})
	req := httptest.NewRequest("POST", "/allocation/tasks/"+taskID.String()+"/allocate", bytes.NewReader(body))
	req = addChiParam(req, "taskId", taskID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.AllocateTask(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- SyncProject ---

func TestAllocationHandler_SyncProject(t *testing.T) {
	epicID := uuid.New()
	svc := &mockAllocationService{
		syncProjectTasksFn: func(ctx context.Context, e uuid.UUID) (int, error) {
			return 3, nil
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("POST", "/allocation/projects/"+epicID.String()+"/sync", nil)
	req = addChiParam(req, "epicId", epicID.String())
	w := httptest.NewRecorder()
	h.SyncProject(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAllocationHandler_SyncProject_InvalidEpicID(t *testing.T) {
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("POST", "/allocation/projects/bad/sync", nil)
	req = addChiParam(req, "epicId", "bad")
	w := httptest.NewRecorder()
	h.SyncProject(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocationHandler_SyncProject_Error(t *testing.T) {
	epicID := uuid.New()
	svc := &mockAllocationService{
		syncProjectTasksFn: func(ctx context.Context, e uuid.UUID) (int, error) {
			return 0, errors.New("boom")
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("POST", "/allocation/projects/"+epicID.String()+"/sync", nil)
	req = addChiParam(req, "epicId", epicID.String())
	w := httptest.NewRecorder()
	h.SyncProject(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- ListSprints ---

func TestAllocationHandler_ListSprints(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockAllocationService{
		getAvailableSprintsFn: func(ctx context.Context, e uuid.UUID) ([]service.SprintOption, error) {
			return []service.SprintOption{{ID: uuid.New()}}, nil
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/allocation/sprints?equipe_id="+equipeID.String(), nil)
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.ListSprints(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAllocationHandler_ListSprints_MissingEquipeID(t *testing.T) {
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/allocation/sprints", nil)
	w := httptest.NewRecorder()
	h.ListSprints(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocationHandler_ListSprints_Forbidden(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/allocation/sprints?equipe_id="+equipeID.String(), nil)
	w := httptest.NewRecorder()
	h.ListSprints(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAllocationHandler_ListSprints_Error(t *testing.T) {
	equipeID := uuid.New()
	svc := &mockAllocationService{
		getAvailableSprintsFn: func(ctx context.Context, e uuid.UUID) ([]service.SprintOption, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/allocation/sprints?equipe_id="+equipeID.String(), nil)
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.ListSprints(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- CloseProject ---

func TestAllocationHandler_CloseProject(t *testing.T) {
	epicID := uuid.New()
	svc := &mockAllocationService{
		closeProjectFn: func(ctx context.Context, e uuid.UUID, req service.CloseProjectRequest, encerradoPor string) error {
			return nil
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(service.CloseProjectRequest{Descricao: "d", DataEncerramento: "2026-01-01"})
	req := httptest.NewRequest("POST", "/allocation/projects/"+epicID.String()+"/close", bytes.NewReader(body))
	req = addChiParam(req, "epicId", epicID.String())
	w := httptest.NewRecorder()
	h.CloseProject(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAllocationHandler_CloseProject_InvalidEpicID(t *testing.T) {
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("POST", "/allocation/projects/bad/close", nil)
	req = addChiParam(req, "epicId", "bad")
	w := httptest.NewRecorder()
	h.CloseProject(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocationHandler_CloseProject_InvalidBody(t *testing.T) {
	epicID := uuid.New()
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("POST", "/allocation/projects/"+epicID.String()+"/close", bytes.NewReader([]byte("not-json")))
	req = addChiParam(req, "epicId", epicID.String())
	w := httptest.NewRecorder()
	h.CloseProject(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocationHandler_CloseProject_MissingFields(t *testing.T) {
	epicID := uuid.New()
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(service.CloseProjectRequest{})
	req := httptest.NewRequest("POST", "/allocation/projects/"+epicID.String()+"/close", bytes.NewReader(body))
	req = addChiParam(req, "epicId", epicID.String())
	w := httptest.NewRecorder()
	h.CloseProject(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocationHandler_CloseProject_Error(t *testing.T) {
	epicID := uuid.New()
	svc := &mockAllocationService{
		closeProjectFn: func(ctx context.Context, e uuid.UUID, req service.CloseProjectRequest, encerradoPor string) error {
			return errors.New("boom")
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	body, _ := json.Marshal(service.CloseProjectRequest{Descricao: "d", DataEncerramento: "2026-01-01"})
	req := httptest.NewRequest("POST", "/allocation/projects/"+epicID.String()+"/close", bytes.NewReader(body))
	req = addChiParam(req, "epicId", epicID.String())
	w := httptest.NewRecorder()
	h.CloseProject(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- ReopenProject ---

func TestAllocationHandler_ReopenProject(t *testing.T) {
	epicID := uuid.New()
	svc := &mockAllocationService{
		reopenProjectFn: func(ctx context.Context, e uuid.UUID) error {
			return nil
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("POST", "/allocation/projects/"+epicID.String()+"/reopen", nil)
	req = addChiParam(req, "epicId", epicID.String())
	w := httptest.NewRecorder()
	h.ReopenProject(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAllocationHandler_ReopenProject_InvalidEpicID(t *testing.T) {
	svc := &mockAllocationService{}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("POST", "/allocation/projects/bad/reopen", nil)
	req = addChiParam(req, "epicId", "bad")
	w := httptest.NewRecorder()
	h.ReopenProject(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAllocationHandler_ReopenProject_Error(t *testing.T) {
	epicID := uuid.New()
	svc := &mockAllocationService{
		reopenProjectFn: func(ctx context.Context, e uuid.UUID) error {
			return errors.New("boom")
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("POST", "/allocation/projects/"+epicID.String()+"/reopen", nil)
	req = addChiParam(req, "epicId", epicID.String())
	w := httptest.NewRecorder()
	h.ReopenProject(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- ListFilteredProducts ---

func TestAllocationHandler_ListFilteredProducts(t *testing.T) {
	svc := &mockAllocationService{
		getFilteredProductsFn: func(ctx context.Context) ([]repository.ProdutoRow, error) {
			return []repository.ProdutoRow{{ID: uuid.New(), Nome: "X"}}, nil
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/allocation/products", nil)
	w := httptest.NewRecorder()
	h.ListFilteredProducts(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAllocationHandler_ListFilteredProducts_Error(t *testing.T) {
	svc := &mockAllocationService{
		getFilteredProductsFn: func(ctx context.Context) ([]repository.ProdutoRow, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewAllocationHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/allocation/products", nil)
	w := httptest.NewRecorder()
	h.ListFilteredProducts(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
