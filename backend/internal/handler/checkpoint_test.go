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
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

type mockCheckpointStore struct {
	listFn   func(ctx context.Context, equipeID *uuid.UUID, ano int) ([]repository.Checkpoint, error)
	createFn func(ctx context.Context, equipeID *uuid.UUID, nome, resumo, dataInicio string, dataFim *string, cor string) (*repository.Checkpoint, error)
	deleteFn func(ctx context.Context, id uuid.UUID) error
}

func (m *mockCheckpointStore) List(ctx context.Context, equipeID *uuid.UUID, ano int) ([]repository.Checkpoint, error) {
	return m.listFn(ctx, equipeID, ano)
}

func (m *mockCheckpointStore) Create(ctx context.Context, equipeID *uuid.UUID, nome, resumo, dataInicio string, dataFim *string, cor string) (*repository.Checkpoint, error) {
	return m.createFn(ctx, equipeID, nome, resumo, dataInicio, dataFim, cor)
}

func (m *mockCheckpointStore) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteFn(ctx, id)
}

// --- List ---

func TestCheckpointHandler_List(t *testing.T) {
	store := &mockCheckpointStore{
		listFn: func(ctx context.Context, equipeID *uuid.UUID, ano int) ([]repository.Checkpoint, error) {
			return []repository.Checkpoint{{ID: uuid.New(), Nome: "CP1"}}, nil
		},
	}
	h := NewCheckpointHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/checkpoints", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCheckpointHandler_List_Error(t *testing.T) {
	store := &mockCheckpointStore{
		listFn: func(ctx context.Context, equipeID *uuid.UUID, ano int) ([]repository.Checkpoint, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewCheckpointHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/checkpoints", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestCheckpointHandler_List_InvalidEquipeID(t *testing.T) {
	store := &mockCheckpointStore{}
	h := NewCheckpointHandler(store, zap.NewNop())

	req := httptest.NewRequest("GET", "/checkpoints?equipe_id=not-a-uuid", nil)
	w := httptest.NewRecorder()
	h.List(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Create ---

func TestCheckpointHandler_Create(t *testing.T) {
	store := &mockCheckpointStore{
		createFn: func(ctx context.Context, equipeID *uuid.UUID, nome, resumo, dataInicio string, dataFim *string, cor string) (*repository.Checkpoint, error) {
			if cor == "" {
				t.Errorf("expected generated color, got empty")
			}
			return &repository.Checkpoint{ID: uuid.New(), Nome: nome, Resumo: resumo, DataInicio: dataInicio, Cor: cor}, nil
		},
	}
	h := NewCheckpointHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]any{
		"nome":        "CP1",
		"resumo":      "Resumo do checkpoint",
		"data_inicio": "2026-01-01",
	})
	req := httptest.NewRequest("POST", "/checkpoints", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCheckpointHandler_Create_Error(t *testing.T) {
	store := &mockCheckpointStore{
		createFn: func(ctx context.Context, equipeID *uuid.UUID, nome, resumo, dataInicio string, dataFim *string, cor string) (*repository.Checkpoint, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewCheckpointHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]any{
		"nome":        "CP1",
		"resumo":      "Resumo do checkpoint",
		"data_inicio": "2026-01-01",
	})
	req := httptest.NewRequest("POST", "/checkpoints", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestCheckpointHandler_Create_InvalidBody(t *testing.T) {
	store := &mockCheckpointStore{}
	h := NewCheckpointHandler(store, zap.NewNop())

	req := httptest.NewRequest("POST", "/checkpoints", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCheckpointHandler_Create_MissingNome(t *testing.T) {
	store := &mockCheckpointStore{}
	h := NewCheckpointHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]any{
		"resumo":      "Resumo do checkpoint",
		"data_inicio": "2026-01-01",
	})
	req := httptest.NewRequest("POST", "/checkpoints", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCheckpointHandler_Create_InvalidDataInicio(t *testing.T) {
	store := &mockCheckpointStore{}
	h := NewCheckpointHandler(store, zap.NewNop())

	body, _ := json.Marshal(map[string]any{
		"nome":        "CP1",
		"resumo":      "Resumo do checkpoint",
		"data_inicio": "not-a-date",
	})
	req := httptest.NewRequest("POST", "/checkpoints", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- Delete ---

func TestCheckpointHandler_Delete(t *testing.T) {
	id := uuid.New()
	store := &mockCheckpointStore{
		deleteFn: func(ctx context.Context, deletedID uuid.UUID) error {
			if deletedID != id {
				t.Errorf("unexpected id: %s", deletedID)
			}
			return nil
		},
	}
	h := NewCheckpointHandler(store, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/checkpoints/"+id.String(), nil)
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCheckpointHandler_Delete_Error(t *testing.T) {
	id := uuid.New()
	store := &mockCheckpointStore{
		deleteFn: func(ctx context.Context, deletedID uuid.UUID) error {
			return errors.New("boom")
		},
	}
	h := NewCheckpointHandler(store, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/checkpoints/"+id.String(), nil)
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestCheckpointHandler_Delete_NotFound(t *testing.T) {
	id := uuid.New()
	store := &mockCheckpointStore{
		deleteFn: func(ctx context.Context, deletedID uuid.UUID) error {
			return pgx.ErrNoRows
		},
	}
	h := NewCheckpointHandler(store, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/checkpoints/"+id.String(), nil)
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCheckpointHandler_Delete_InvalidID(t *testing.T) {
	store := &mockCheckpointStore{}
	h := NewCheckpointHandler(store, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/checkpoints/not-a-uuid", nil)
	req = addChiParam(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- generateCheckpointColor / hslToHex ---

func TestGenerateCheckpointColor(t *testing.T) {
	for i := 0; i < 20; i++ {
		cor := generateCheckpointColor()
		if len(cor) != 7 || cor[0] != '#' {
			t.Fatalf("unexpected color format: %q", cor)
		}
	}
}

func TestHslToHex(t *testing.T) {
	tests := []struct {
		h, s, l float64
		want    string
	}{
		{0, 100, 50, "#FF0000"},   // red
		{120, 100, 50, "#00FF00"}, // green
		{240, 100, 50, "#0000FF"}, // blue
		{0, 0, 0, "#000000"},      // black
		{0, 0, 100, "#FFFFFF"},    // white
	}
	for _, tt := range tests {
		got := hslToHex(tt.h, tt.s, tt.l)
		if got != tt.want {
			t.Errorf("hslToHex(%v, %v, %v) = %s, want %s", tt.h, tt.s, tt.l, got, tt.want)
		}
	}
}
