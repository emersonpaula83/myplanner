package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockFonteDadosStore struct {
	listFn    func(ctx context.Context) ([]domain.FonteDados, error)
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error)
	createFn  func(ctx context.Context, req *repository.CreateFonteDadosRequest) (*domain.FonteDados, error)
	updateFn  func(ctx context.Context, id uuid.UUID, req *repository.UpdateFonteDadosRequest) (*domain.FonteDados, error)
	deleteFn  func(ctx context.Context, id uuid.UUID) error
}

func (m *mockFonteDadosStore) List(ctx context.Context) ([]domain.FonteDados, error) {
	return m.listFn(ctx)
}

func (m *mockFonteDadosStore) GetByID(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) {
	return m.getByIDFn(ctx, id)
}

func (m *mockFonteDadosStore) Create(ctx context.Context, req *repository.CreateFonteDadosRequest) (*domain.FonteDados, error) {
	return m.createFn(ctx, req)
}

func (m *mockFonteDadosStore) Update(ctx context.Context, id uuid.UUID, req *repository.UpdateFonteDadosRequest) (*domain.FonteDados, error) {
	return m.updateFn(ctx, id, req)
}

func (m *mockFonteDadosStore) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteFn(ctx, id)
}

// --- List ---

func TestFonteDadosHandler_List(t *testing.T) {
	store := &mockFonteDadosStore{
		listFn: func(ctx context.Context) ([]domain.FonteDados, error) {
			return []domain.FonteDados{{ID: uuid.New(), Nome: "Fonte 1"}}, nil
		},
	}
	h := NewFonteDadosHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/fontes-dados", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFonteDadosHandler_List_Error(t *testing.T) {
	store := &mockFonteDadosStore{
		listFn: func(ctx context.Context) ([]domain.FonteDados, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewFonteDadosHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/fontes-dados", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- GetByID ---

func TestFonteDadosHandler_GetByID(t *testing.T) {
	id := uuid.New()
	store := &mockFonteDadosStore{
		getByIDFn: func(ctx context.Context, gotID uuid.UUID) (*domain.FonteDados, error) {
			if gotID != id {
				t.Errorf("unexpected id: %s", gotID)
			}
			return &domain.FonteDados{ID: id, Nome: "Fonte 1"}, nil
		},
	}
	h := NewFonteDadosHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/fontes-dados/"+id.String(), nil)
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.GetByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFonteDadosHandler_GetByID_Error(t *testing.T) {
	id := uuid.New()
	store := &mockFonteDadosStore{
		getByIDFn: func(ctx context.Context, gotID uuid.UUID) (*domain.FonteDados, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewFonteDadosHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/fontes-dados/"+id.String(), nil)
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.GetByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestFonteDadosHandler_GetByID_NotFound(t *testing.T) {
	id := uuid.New()
	store := &mockFonteDadosStore{
		getByIDFn: func(ctx context.Context, gotID uuid.UUID) (*domain.FonteDados, error) {
			return nil, nil
		},
	}
	h := NewFonteDadosHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/fontes-dados/"+id.String(), nil)
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.GetByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFonteDadosHandler_GetByID_InvalidID(t *testing.T) {
	store := &mockFonteDadosStore{}
	h := NewFonteDadosHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/fontes-dados/not-a-uuid", nil)
	req = addChiParam(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()
	h.GetByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Create ---

func TestFonteDadosHandler_Create(t *testing.T) {
	store := &mockFonteDadosStore{
		createFn: func(ctx context.Context, req *repository.CreateFonteDadosRequest) (*domain.FonteDados, error) {
			return &domain.FonteDados{ID: uuid.New(), Nome: req.Nome, Tipo: req.Tipo, BaseURL: req.BaseURL, AuthType: req.AuthType}, nil
		},
	}
	h := NewFonteDadosHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]string{
		"nome":      "Fonte 1",
		"base_url":  "https://example.com",
		"auth_type": "basic",
	})
	req := httptest.NewRequest("POST", "/fontes-dados", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFonteDadosHandler_Create_DefaultsTipoToJira(t *testing.T) {
	var capturedTipo string
	store := &mockFonteDadosStore{
		createFn: func(ctx context.Context, req *repository.CreateFonteDadosRequest) (*domain.FonteDados, error) {
			capturedTipo = req.Tipo
			return &domain.FonteDados{ID: uuid.New(), Nome: req.Nome}, nil
		},
	}
	h := NewFonteDadosHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]string{
		"nome":      "Fonte 1",
		"base_url":  "https://example.com",
		"auth_type": "basic",
	})
	req := httptest.NewRequest("POST", "/fontes-dados", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if capturedTipo != "jira" {
		t.Errorf("expected tipo defaulted to 'jira', got %q", capturedTipo)
	}
}

func TestFonteDadosHandler_Create_Error(t *testing.T) {
	store := &mockFonteDadosStore{
		createFn: func(ctx context.Context, req *repository.CreateFonteDadosRequest) (*domain.FonteDados, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewFonteDadosHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]string{
		"nome":      "Fonte 1",
		"base_url":  "https://example.com",
		"auth_type": "basic",
	})
	req := httptest.NewRequest("POST", "/fontes-dados", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestFonteDadosHandler_Create_InvalidBody(t *testing.T) {
	store := &mockFonteDadosStore{}
	h := NewFonteDadosHandler(store, zap.NewNop())

	req := httptest.NewRequest("POST", "/fontes-dados", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestFonteDadosHandler_Create_MissingNome(t *testing.T) {
	store := &mockFonteDadosStore{}
	h := NewFonteDadosHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]string{
		"base_url":  "https://example.com",
		"auth_type": "basic",
	})
	req := httptest.NewRequest("POST", "/fontes-dados", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestFonteDadosHandler_Create_MissingBaseURL(t *testing.T) {
	store := &mockFonteDadosStore{}
	h := NewFonteDadosHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]string{
		"nome":      "Fonte 1",
		"auth_type": "basic",
	})
	req := httptest.NewRequest("POST", "/fontes-dados", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestFonteDadosHandler_Create_MissingAuthType(t *testing.T) {
	store := &mockFonteDadosStore{}
	h := NewFonteDadosHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]string{
		"nome":     "Fonte 1",
		"base_url": "https://example.com",
	})
	req := httptest.NewRequest("POST", "/fontes-dados", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Update ---

func TestFonteDadosHandler_Update(t *testing.T) {
	id := uuid.New()
	store := &mockFonteDadosStore{
		updateFn: func(ctx context.Context, gotID uuid.UUID, req *repository.UpdateFonteDadosRequest) (*domain.FonteDados, error) {
			if gotID != id {
				t.Errorf("unexpected id: %s", gotID)
			}
			return &domain.FonteDados{ID: id, Nome: *req.Nome}, nil
		},
	}
	h := NewFonteDadosHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]string{"nome": "Fonte Atualizada"})
	req := httptest.NewRequest("PUT", "/fontes-dados/"+id.String(), strings.NewReader(string(body)))
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.Update(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFonteDadosHandler_Update_Error(t *testing.T) {
	id := uuid.New()
	store := &mockFonteDadosStore{
		updateFn: func(ctx context.Context, gotID uuid.UUID, req *repository.UpdateFonteDadosRequest) (*domain.FonteDados, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewFonteDadosHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]string{"nome": "Fonte Atualizada"})
	req := httptest.NewRequest("PUT", "/fontes-dados/"+id.String(), strings.NewReader(string(body)))
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.Update(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestFonteDadosHandler_Update_NotFound(t *testing.T) {
	id := uuid.New()
	store := &mockFonteDadosStore{
		updateFn: func(ctx context.Context, gotID uuid.UUID, req *repository.UpdateFonteDadosRequest) (*domain.FonteDados, error) {
			return nil, nil
		},
	}
	h := NewFonteDadosHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]string{"nome": "Fonte Atualizada"})
	req := httptest.NewRequest("PUT", "/fontes-dados/"+id.String(), strings.NewReader(string(body)))
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.Update(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFonteDadosHandler_Update_InvalidID(t *testing.T) {
	store := &mockFonteDadosStore{}
	h := NewFonteDadosHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]string{"nome": "Fonte Atualizada"})
	req := httptest.NewRequest("PUT", "/fontes-dados/not-a-uuid", strings.NewReader(string(body)))
	req = addChiParam(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()
	h.Update(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestFonteDadosHandler_Update_InvalidBody(t *testing.T) {
	id := uuid.New()
	store := &mockFonteDadosStore{}
	h := NewFonteDadosHandler(store, zap.NewNop())

	req := httptest.NewRequest("PUT", "/fontes-dados/"+id.String(), strings.NewReader("not-json"))
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.Update(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Delete ---

func TestFonteDadosHandler_Delete(t *testing.T) {
	id := uuid.New()
	store := &mockFonteDadosStore{
		deleteFn: func(ctx context.Context, gotID uuid.UUID) error {
			if gotID != id {
				t.Errorf("unexpected id: %s", gotID)
			}
			return nil
		},
	}
	h := NewFonteDadosHandler(store, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/fontes-dados/"+id.String(), nil)
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFonteDadosHandler_Delete_Error(t *testing.T) {
	id := uuid.New()
	store := &mockFonteDadosStore{
		deleteFn: func(ctx context.Context, gotID uuid.UUID) error {
			return errors.New("boom")
		},
	}
	h := NewFonteDadosHandler(store, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/fontes-dados/"+id.String(), nil)
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestFonteDadosHandler_Delete_InvalidID(t *testing.T) {
	store := &mockFonteDadosStore{}
	h := NewFonteDadosHandler(store, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/fontes-dados/not-a-uuid", nil)
	req = addChiParam(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
