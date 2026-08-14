package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockTarefaStore struct {
	listTarefasFn     func(ctx context.Context, f repository.TarefaListFilter) (*repository.TarefaListResult, error)
	hardDeleteTarefaFn func(ctx context.Context, id uuid.UUID) error
}

func (m *mockTarefaStore) ListTarefas(ctx context.Context, f repository.TarefaListFilter) (*repository.TarefaListResult, error) {
	return m.listTarefasFn(ctx, f)
}

func (m *mockTarefaStore) HardDeleteTarefa(ctx context.Context, id uuid.UUID) error {
	return m.hardDeleteTarefaFn(ctx, id)
}

// --- ListTarefas ---

func TestTarefaHandler_ListTarefas(t *testing.T) {
	repo := &mockTarefaStore{
		listTarefasFn: func(ctx context.Context, f repository.TarefaListFilter) (*repository.TarefaListResult, error) {
			return &repository.TarefaListResult{Items: []repository.TarefaListRow{{ID: uuid.New()}}, Total: 1}, nil
		},
	}
	h := NewTarefaHandler(repo, zap.NewNop())

	req := httptest.NewRequest("GET", "/tarefas", nil)
	w := httptest.NewRecorder()
	h.ListTarefas(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTarefaHandler_ListTarefas_WithEquipeID(t *testing.T) {
	equipeID := uuid.New()
	repo := &mockTarefaStore{
		listTarefasFn: func(ctx context.Context, f repository.TarefaListFilter) (*repository.TarefaListResult, error) {
			if f.EquipeID == nil || *f.EquipeID != equipeID {
				t.Errorf("expected equipeID %s in filter, got %v", equipeID, f.EquipeID)
			}
			return &repository.TarefaListResult{}, nil
		},
	}
	h := NewTarefaHandler(repo, zap.NewNop())

	req := httptest.NewRequest("GET", "/tarefas?equipe_id="+equipeID.String(), nil)
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.ListTarefas(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTarefaHandler_ListTarefas_InvalidEquipeID(t *testing.T) {
	repo := &mockTarefaStore{}
	h := NewTarefaHandler(repo, zap.NewNop())

	req := httptest.NewRequest("GET", "/tarefas?equipe_id=bad", nil)
	w := httptest.NewRecorder()
	h.ListTarefas(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTarefaHandler_ListTarefas_ForbiddenEquipe(t *testing.T) {
	equipeID := uuid.New()
	repo := &mockTarefaStore{}
	h := NewTarefaHandler(repo, zap.NewNop())

	req := httptest.NewRequest("GET", "/tarefas?equipe_id="+equipeID.String(), nil)
	w := httptest.NewRecorder()
	h.ListTarefas(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestTarefaHandler_ListTarefas_InvalidResponsavelID(t *testing.T) {
	repo := &mockTarefaStore{}
	h := NewTarefaHandler(repo, zap.NewNop())

	req := httptest.NewRequest("GET", "/tarefas?responsavel_id=bad", nil)
	w := httptest.NewRecorder()
	h.ListTarefas(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTarefaHandler_ListTarefas_Error(t *testing.T) {
	repo := &mockTarefaStore{
		listTarefasFn: func(ctx context.Context, f repository.TarefaListFilter) (*repository.TarefaListResult, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewTarefaHandler(repo, zap.NewNop())

	req := httptest.NewRequest("GET", "/tarefas", nil)
	w := httptest.NewRecorder()
	h.ListTarefas(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- DeleteTarefa ---

func TestTarefaHandler_DeleteTarefa(t *testing.T) {
	id := uuid.New()
	repo := &mockTarefaStore{
		hardDeleteTarefaFn: func(ctx context.Context, deletedID uuid.UUID) error {
			if deletedID != id {
				t.Errorf("unexpected id: %s", deletedID)
			}
			return nil
		},
	}
	h := NewTarefaHandler(repo, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/tarefas/"+id.String(), nil)
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.DeleteTarefa(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTarefaHandler_DeleteTarefa_InvalidID(t *testing.T) {
	repo := &mockTarefaStore{}
	h := NewTarefaHandler(repo, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/tarefas/bad", nil)
	req = addChiParam(req, "id", "bad")
	w := httptest.NewRecorder()
	h.DeleteTarefa(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTarefaHandler_DeleteTarefa_Error(t *testing.T) {
	id := uuid.New()
	repo := &mockTarefaStore{
		hardDeleteTarefaFn: func(ctx context.Context, deletedID uuid.UUID) error {
			return errors.New("boom")
		},
	}
	h := NewTarefaHandler(repo, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/tarefas/"+id.String(), nil)
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.DeleteTarefa(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
