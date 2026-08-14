package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockFeriadoStore struct {
	listFn   func(ctx context.Context) ([]repository.Feriado, error)
	createFn func(ctx context.Context, data string, nome string) (*repository.Feriado, error)
	deleteFn func(ctx context.Context, id uuid.UUID) error
}

func (m *mockFeriadoStore) List(ctx context.Context) ([]repository.Feriado, error) {
	return m.listFn(ctx)
}

func (m *mockFeriadoStore) Create(ctx context.Context, data string, nome string) (*repository.Feriado, error) {
	return m.createFn(ctx, data, nome)
}

func (m *mockFeriadoStore) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteFn(ctx, id)
}

// --- List ---

func TestFeriadoHandler_List(t *testing.T) {
	store := &mockFeriadoStore{
		listFn: func(ctx context.Context) ([]repository.Feriado, error) {
			return []repository.Feriado{{ID: uuid.New(), Data: "2026-01-01", Nome: "Ano Novo"}}, nil
		},
	}
	h := NewFeriadoHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/feriados", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFeriadoHandler_List_Error(t *testing.T) {
	store := &mockFeriadoStore{
		listFn: func(ctx context.Context) ([]repository.Feriado, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewFeriadoHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/feriados", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- Create ---

func TestFeriadoHandler_Create(t *testing.T) {
	store := &mockFeriadoStore{
		createFn: func(ctx context.Context, data string, nome string) (*repository.Feriado, error) {
			return &repository.Feriado{ID: uuid.New(), Data: data, Nome: nome}, nil
		},
	}
	h := NewFeriadoHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]string{"data": "2026-01-01", "nome": "Ano Novo"})
	req := httptest.NewRequest("POST", "/feriados", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFeriadoHandler_Create_Error(t *testing.T) {
	store := &mockFeriadoStore{
		createFn: func(ctx context.Context, data string, nome string) (*repository.Feriado, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewFeriadoHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]string{"data": "2026-01-01", "nome": "Ano Novo"})
	req := httptest.NewRequest("POST", "/feriados", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestFeriadoHandler_Create_InvalidBody(t *testing.T) {
	store := &mockFeriadoStore{}
	h := NewFeriadoHandler(store, zap.NewNop())

	req := httptest.NewRequest("POST", "/feriados", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestFeriadoHandler_Create_MissingFields(t *testing.T) {
	store := &mockFeriadoStore{}
	h := NewFeriadoHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]string{"data": "", "nome": ""})
	req := httptest.NewRequest("POST", "/feriados", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Delete ---

func TestFeriadoHandler_Delete(t *testing.T) {
	id := uuid.New()
	store := &mockFeriadoStore{
		deleteFn: func(ctx context.Context, deletedID uuid.UUID) error {
			if deletedID != id {
				t.Errorf("unexpected id: %s", deletedID)
			}
			return nil
		},
	}
	h := NewFeriadoHandler(store, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/feriados/"+id.String(), nil)
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFeriadoHandler_Delete_Error(t *testing.T) {
	id := uuid.New()
	store := &mockFeriadoStore{
		deleteFn: func(ctx context.Context, deletedID uuid.UUID) error {
			return errors.New("boom")
		},
	}
	h := NewFeriadoHandler(store, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/feriados/"+id.String(), nil)
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestFeriadoHandler_Delete_InvalidID(t *testing.T) {
	store := &mockFeriadoStore{}
	h := NewFeriadoHandler(store, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/feriados/not-a-uuid", nil)
	req = addChiParam(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
