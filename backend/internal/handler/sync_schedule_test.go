package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockSyncScheduleStore struct {
	getByFonteFn func(ctx context.Context, fonteID uuid.UUID) (*domain.SyncSchedule, error)
	upsertFn     func(ctx context.Context, fonteID uuid.UUID, projectKeys []string, horarios []string) (*domain.SyncSchedule, error)
	deleteFn     func(ctx context.Context, fonteID uuid.UUID) error
	setAtivoFn   func(ctx context.Context, id uuid.UUID, ativo bool) error
}

func (m *mockSyncScheduleStore) GetByFonte(ctx context.Context, fonteID uuid.UUID) (*domain.SyncSchedule, error) {
	return m.getByFonteFn(ctx, fonteID)
}

func (m *mockSyncScheduleStore) Upsert(ctx context.Context, fonteID uuid.UUID, projectKeys []string, horarios []string) (*domain.SyncSchedule, error) {
	return m.upsertFn(ctx, fonteID, projectKeys, horarios)
}

func (m *mockSyncScheduleStore) Delete(ctx context.Context, fonteID uuid.UUID) error {
	return m.deleteFn(ctx, fonteID)
}

func (m *mockSyncScheduleStore) SetAtivo(ctx context.Context, id uuid.UUID, ativo bool) error {
	return m.setAtivoFn(ctx, id, ativo)
}

// --- Get ---

func TestSyncScheduleHandler_Get(t *testing.T) {
	fonteID := uuid.New()
	repo := &mockSyncScheduleStore{
		getByFonteFn: func(ctx context.Context, f uuid.UUID) (*domain.SyncSchedule, error) {
			return &domain.SyncSchedule{ID: uuid.New(), FonteDadosID: f}, nil
		},
	}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	req := httptest.NewRequest("GET", "/sync-schedule?fonte_dados_id="+fonteID.String(), nil)
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSyncScheduleHandler_Get_NilSchedule(t *testing.T) {
	fonteID := uuid.New()
	repo := &mockSyncScheduleStore{
		getByFonteFn: func(ctx context.Context, f uuid.UUID) (*domain.SyncSchedule, error) {
			return nil, nil
		},
	}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	req := httptest.NewRequest("GET", "/sync-schedule?fonte_dados_id="+fonteID.String(), nil)
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSyncScheduleHandler_Get_MissingFonteID(t *testing.T) {
	repo := &mockSyncScheduleStore{}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	req := httptest.NewRequest("GET", "/sync-schedule", nil)
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSyncScheduleHandler_Get_InvalidFonteID(t *testing.T) {
	repo := &mockSyncScheduleStore{}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	req := httptest.NewRequest("GET", "/sync-schedule?fonte_dados_id=bad", nil)
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSyncScheduleHandler_Get_Error(t *testing.T) {
	fonteID := uuid.New()
	repo := &mockSyncScheduleStore{
		getByFonteFn: func(ctx context.Context, f uuid.UUID) (*domain.SyncSchedule, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	req := httptest.NewRequest("GET", "/sync-schedule?fonte_dados_id="+fonteID.String(), nil)
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- Upsert ---

func validUpsertBody(fonteID uuid.UUID) []byte {
	body, _ := json.Marshal(upsertScheduleRequest{
		FonteDadosID: fonteID,
		ProjectKeys:  []string{"PRJ"},
		Horarios:     []string{"08:00"},
	})
	return body
}

func TestSyncScheduleHandler_Upsert(t *testing.T) {
	fonteID := uuid.New()
	repo := &mockSyncScheduleStore{
		upsertFn: func(ctx context.Context, f uuid.UUID, projectKeys []string, horarios []string) (*domain.SyncSchedule, error) {
			return &domain.SyncSchedule{ID: uuid.New(), FonteDadosID: f}, nil
		},
	}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	req := httptest.NewRequest("POST", "/sync-schedule", bytes.NewReader(validUpsertBody(fonteID)))
	w := httptest.NewRecorder()
	h.Upsert(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSyncScheduleHandler_Upsert_InvalidBody(t *testing.T) {
	repo := &mockSyncScheduleStore{}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	req := httptest.NewRequest("POST", "/sync-schedule", bytes.NewReader([]byte("not-json")))
	w := httptest.NewRecorder()
	h.Upsert(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSyncScheduleHandler_Upsert_MissingFonteID(t *testing.T) {
	repo := &mockSyncScheduleStore{}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	body, _ := json.Marshal(upsertScheduleRequest{ProjectKeys: []string{"PRJ"}, Horarios: []string{"08:00"}})
	req := httptest.NewRequest("POST", "/sync-schedule", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Upsert(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSyncScheduleHandler_Upsert_MissingProjectKeys(t *testing.T) {
	repo := &mockSyncScheduleStore{}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	body, _ := json.Marshal(upsertScheduleRequest{FonteDadosID: uuid.New(), Horarios: []string{"08:00"}})
	req := httptest.NewRequest("POST", "/sync-schedule", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Upsert(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSyncScheduleHandler_Upsert_MissingHorarios(t *testing.T) {
	repo := &mockSyncScheduleStore{}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	body, _ := json.Marshal(upsertScheduleRequest{FonteDadosID: uuid.New(), ProjectKeys: []string{"PRJ"}})
	req := httptest.NewRequest("POST", "/sync-schedule", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Upsert(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSyncScheduleHandler_Upsert_TooManyHorarios(t *testing.T) {
	repo := &mockSyncScheduleStore{}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	body, _ := json.Marshal(upsertScheduleRequest{
		FonteDadosID: uuid.New(),
		ProjectKeys:  []string{"PRJ"},
		Horarios:     []string{"08:00", "09:00", "10:00", "11:00", "12:00"},
	})
	req := httptest.NewRequest("POST", "/sync-schedule", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Upsert(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSyncScheduleHandler_Upsert_InvalidHorario(t *testing.T) {
	repo := &mockSyncScheduleStore{}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	body, _ := json.Marshal(upsertScheduleRequest{
		FonteDadosID: uuid.New(),
		ProjectKeys:  []string{"PRJ"},
		Horarios:     []string{"25:99"},
	})
	req := httptest.NewRequest("POST", "/sync-schedule", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Upsert(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSyncScheduleHandler_Upsert_Error(t *testing.T) {
	fonteID := uuid.New()
	repo := &mockSyncScheduleStore{
		upsertFn: func(ctx context.Context, f uuid.UUID, projectKeys []string, horarios []string) (*domain.SyncSchedule, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	req := httptest.NewRequest("POST", "/sync-schedule", bytes.NewReader(validUpsertBody(fonteID)))
	w := httptest.NewRecorder()
	h.Upsert(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- Delete ---

func TestSyncScheduleHandler_Delete(t *testing.T) {
	fonteID := uuid.New()
	repo := &mockSyncScheduleStore{
		deleteFn: func(ctx context.Context, f uuid.UUID) error {
			return nil
		},
	}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/sync-schedule?fonte_dados_id="+fonteID.String(), nil)
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSyncScheduleHandler_Delete_MissingFonteID(t *testing.T) {
	repo := &mockSyncScheduleStore{}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/sync-schedule", nil)
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSyncScheduleHandler_Delete_InvalidFonteID(t *testing.T) {
	repo := &mockSyncScheduleStore{}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/sync-schedule?fonte_dados_id=bad", nil)
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSyncScheduleHandler_Delete_Error(t *testing.T) {
	fonteID := uuid.New()
	repo := &mockSyncScheduleStore{
		deleteFn: func(ctx context.Context, f uuid.UUID) error {
			return errors.New("boom")
		},
	}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/sync-schedule?fonte_dados_id="+fonteID.String(), nil)
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- Toggle ---

func TestSyncScheduleHandler_Toggle(t *testing.T) {
	id := uuid.New()
	repo := &mockSyncScheduleStore{
		setAtivoFn: func(ctx context.Context, i uuid.UUID, ativo bool) error {
			return nil
		},
	}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	body, _ := json.Marshal(map[string]bool{"ativo": true})
	req := httptest.NewRequest("PATCH", "/sync-schedule/"+id.String(), bytes.NewReader(body))
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.Toggle(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSyncScheduleHandler_Toggle_InvalidID(t *testing.T) {
	repo := &mockSyncScheduleStore{}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	req := httptest.NewRequest("PATCH", "/sync-schedule/bad", nil)
	req = addChiParam(req, "id", "bad")
	w := httptest.NewRecorder()
	h.Toggle(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSyncScheduleHandler_Toggle_InvalidBody(t *testing.T) {
	id := uuid.New()
	repo := &mockSyncScheduleStore{}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	req := httptest.NewRequest("PATCH", "/sync-schedule/"+id.String(), bytes.NewReader([]byte("not-json")))
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.Toggle(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSyncScheduleHandler_Toggle_Error(t *testing.T) {
	id := uuid.New()
	repo := &mockSyncScheduleStore{
		setAtivoFn: func(ctx context.Context, i uuid.UUID, ativo bool) error {
			return errors.New("boom")
		},
	}
	h := NewSyncScheduleHandler(repo, zap.NewNop())

	body, _ := json.Marshal(map[string]bool{"ativo": true})
	req := httptest.NewRequest("PATCH", "/sync-schedule/"+id.String(), bytes.NewReader(body))
	req = addChiParam(req, "id", id.String())
	w := httptest.NewRecorder()
	h.Toggle(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
