package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
)

func zapNopLoggerTL() *zap.Logger { return zap.NewNop() }

// --- GetProjetoEquipes ---

func TestGetProjetoEquipes_Success(t *testing.T) {
	epicoID := uuid.New()
	equipeID := uuid.New()
	store := &mockTimelineStore{
		equipeIDs: []uuid.UUID{equipeID},
	}
	h := NewTimelineHandler(store, nil, zapNopLoggerTL())

	req := httptest.NewRequest("GET", "/api/v1/projetos/"+epicoID.String()+"/equipes", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", epicoID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.GetProjetoEquipes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var result []uuid.UUID
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 || result[0] != equipeID {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestGetProjetoEquipes_NilBecomesEmptyArray(t *testing.T) {
	epicoID := uuid.New()
	store := &mockTimelineStore{
		equipeIDs: nil,
	}
	h := NewTimelineHandler(store, nil, zapNopLoggerTL())

	req := httptest.NewRequest("GET", "/api/v1/projetos/"+epicoID.String()+"/equipes", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", epicoID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.GetProjetoEquipes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if w.Body.String() != "[]\n" && w.Body.String() != "[]" {
		t.Errorf("expected empty array body, got %q", w.Body.String())
	}
}

func TestGetProjetoEquipes_InvalidID(t *testing.T) {
	h := NewTimelineHandler(&mockTimelineStore{}, nil, zapNopLoggerTL())

	req := httptest.NewRequest("GET", "/api/v1/projetos/xxx/equipes", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "xxx")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.GetProjetoEquipes(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

type errTimelineStore struct {
	mockTimelineStore
}

func (m *errTimelineStore) BuscarEpicoEquipes(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return nil, errors.New("db error")
}

func TestGetProjetoEquipes_Error(t *testing.T) {
	epicoID := uuid.New()
	store := &errTimelineStore{}
	h := NewTimelineHandler(store, nil, zapNopLoggerTL())

	req := httptest.NewRequest("GET", "/api/v1/projetos/"+epicoID.String()+"/equipes", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", epicoID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.GetProjetoEquipes(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- AnalisarCapacidade additional paths ---

func TestAnalisarCapacidade_InvalidBody(t *testing.T) {
	h := NewTimelineHandler(&mockTimelineStore{}, &mockAnalyzer{}, zapNopLoggerTL())

	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(`not-json`))
	w := httptest.NewRecorder()

	h.AnalisarCapacidade(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAnalisarCapacidade_MissingEquipe(t *testing.T) {
	h := NewTimelineHandler(&mockTimelineStore{}, &mockAnalyzer{}, zapNopLoggerTL())

	body := `{"equipe":"","ano":2026,"mes":7}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.AnalisarCapacidade(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAnalisarCapacidade_InvalidEquipeID(t *testing.T) {
	h := NewTimelineHandler(&mockTimelineStore{}, &mockAnalyzer{}, zapNopLoggerTL())

	body := `{"equipe":"not-a-uuid","ano":2026,"mes":7}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.AnalisarCapacidade(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAnalisarCapacidade_InvalidAno(t *testing.T) {
	equipeID := uuid.New()
	h := NewTimelineHandler(&mockTimelineStore{}, &mockAnalyzer{}, zapNopLoggerTL())

	body := `{"equipe":"` + equipeID.String() + `","ano":0,"mes":7}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.AnalisarCapacidade(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAnalisarCapacidade_Forbidden(t *testing.T) {
	equipeID := uuid.New()
	h := NewTimelineHandler(&mockTimelineStore{}, &mockAnalyzer{}, zapNopLoggerTL())

	body := `{"equipe":"` + equipeID.String() + `","ano":2026,"mes":7}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.AnalisarCapacidade(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestAnalisarCapacidade_ZeroMembrosProceedsToSuccess(t *testing.T) {
	equipeID := uuid.New()
	store := &mockTimelineStore{}
	h := NewTimelineHandler(store, &mockAnalyzer{}, zapNopLoggerTL())

	body := `{"equipe":"` + equipeID.String() + `","ano":2026,"mes":7}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	req = req.WithContext(middleware.ContextWithEquipeIDs(req.Context(), []uuid.UUID{equipeID}))
	w := httptest.NewRecorder()

	h.AnalisarCapacidade(w, req)

	// membrosCount defaults to 0, no error from mock — expect success flow to proceed.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

type errCountTimelineStore struct {
	mockTimelineStore
}

func (m *errCountTimelineStore) ContarMembrosAtivosEquipes(_ context.Context, _ []uuid.UUID, _ int) (int, error) {
	return 0, errors.New("db error")
}

func TestAnalisarCapacidade_ErrorContarMembros(t *testing.T) {
	equipeID := uuid.New()
	store := &errCountTimelineStore{}
	h := NewTimelineHandler(store, &mockAnalyzer{}, zapNopLoggerTL())

	body := `{"equipe":"` + equipeID.String() + `","ano":2026,"mes":7}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	req = req.WithContext(middleware.ContextWithEquipeIDs(req.Context(), []uuid.UUID{equipeID}))
	w := httptest.NewRecorder()

	h.AnalisarCapacidade(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestAnalisarCapacidade_AnalyzerError(t *testing.T) {
	equipeID := uuid.New()
	store := &mockTimelineStore{}
	analyzer := &mockAnalyzer{err: errors.New("gemini failed")}
	h := NewTimelineHandler(store, analyzer, zapNopLoggerTL())

	body := `{"equipe":"` + equipeID.String() + `","ano":2026,"mes":7}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	req = req.WithContext(middleware.ContextWithEquipeIDs(req.Context(), []uuid.UUID{equipeID}))
	w := httptest.NewRecorder()

	h.AnalisarCapacidade(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestAnalisarCapacidade_Success(t *testing.T) {
	equipeID := uuid.New()
	store := &mockTimelineStore{membrosCount: 3}
	analyzer := &mockAnalyzer{result: "analise ok"}
	h := NewTimelineHandler(store, analyzer, zapNopLoggerTL())

	body := `{"equipe":"` + equipeID.String() + `","ano":2026,"mes":7}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	req = req.WithContext(middleware.ContextWithEquipeIDs(req.Context(), []uuid.UUID{equipeID}))
	w := httptest.NewRecorder()

	h.AnalisarCapacidade(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp domain.AnaliseResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Analise != "analise ok" {
		t.Errorf("Analise = %q, want %q", resp.Analise, "analise ok")
	}
}

// --- UpdateProjetoMetadata additional paths ---

func TestUpdateProjetoMetadata_InvalidID(t *testing.T) {
	h := NewTimelineHandler(&mockTimelineStore{}, nil, zapNopLoggerTL())

	req := httptest.NewRequest("PUT", "/api/v1/projetos/xxx/metadata", bytes.NewBufferString(`{}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "xxx")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.UpdateProjetoMetadata(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpdateProjetoMetadata_InvalidBody(t *testing.T) {
	id := uuid.New()
	h := NewTimelineHandler(&mockTimelineStore{}, nil, zapNopLoggerTL())

	req := httptest.NewRequest("PUT", "/", bytes.NewBufferString(`not-json`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.UpdateProjetoMetadata(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

type errFetchTimelineStore struct {
	mockTimelineStore
	callCount int
}

func (m *errFetchTimelineStore) BuscarEpicoPorID(_ context.Context, _ uuid.UUID) (*domain.Tarefa, error) {
	m.callCount++
	return nil, errors.New("db error")
}

func TestUpdateProjetoMetadata_ErrorFetchEpico(t *testing.T) {
	id := uuid.New()
	store := &errFetchTimelineStore{}
	h := NewTimelineHandler(store, nil, zapNopLoggerTL())

	req := httptest.NewRequest("PUT", "/", bytes.NewBufferString(`{}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.UpdateProjetoMetadata(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestUpdateProjetoMetadata_NotFound(t *testing.T) {
	id := uuid.New()
	store := &mockTimelineStore{epicoPorID: nil}
	h := NewTimelineHandler(store, nil, zapNopLoggerTL())

	req := httptest.NewRequest("PUT", "/", bytes.NewBufferString(`{}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.UpdateProjetoMetadata(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestUpdateProjetoMetadata_InvalidDataLimite(t *testing.T) {
	id := uuid.New()
	store := &mockTimelineStore{
		epicoPorID: &domain.Tarefa{ID: id, Tipo: "Épico", NumeroTicket: "BACK-1"},
	}
	h := NewTimelineHandler(store, nil, zapNopLoggerTL())

	body := `{"data_limite": "31-12-2026"}`
	req := httptest.NewRequest("PUT", "/", bytes.NewBufferString(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.UpdateProjetoMetadata(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpdateProjetoMetadata_ErrorUpdate(t *testing.T) {
	id := uuid.New()
	store := &mockTimelineStore{
		epicoPorID: &domain.Tarefa{ID: id, Tipo: "Épico", NumeroTicket: "BACK-1"},
		updateErr:  errors.New("db error"),
	}
	h := NewTimelineHandler(store, nil, zapNopLoggerTL())

	req := httptest.NewRequest("PUT", "/", bytes.NewBufferString(`{}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.UpdateProjetoMetadata(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestUpdateProjetoMetadata_EquipeAccessForbidden(t *testing.T) {
	id := uuid.New()
	otherEquipeID := uuid.New()
	store := &mockTimelineStore{
		epicoPorID: &domain.Tarefa{ID: id, Tipo: "Épico", NumeroTicket: "BACK-1"},
	}
	h := NewTimelineHandler(store, nil, zapNopLoggerTL())

	body := `{"equipe_ids":["` + otherEquipeID.String() + `"]}`
	req := httptest.NewRequest("PUT", "/", bytes.NewBufferString(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.UpdateProjetoMetadata(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestUpdateProjetoMetadata_SaveEquipesError(t *testing.T) {
	id := uuid.New()
	equipeID := uuid.New()
	store := &mockTimelineStore{
		epicoPorID:     &domain.Tarefa{ID: id, Tipo: "Épico", NumeroTicket: "BACK-1"},
		saveEquipesErr: errors.New("db error"),
	}
	h := NewTimelineHandler(store, nil, zapNopLoggerTL())

	body := `{"equipe_ids":["` + equipeID.String() + `"]}`
	req := httptest.NewRequest("PUT", "/", bytes.NewBufferString(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id.String())
	ctx := middleware.ContextWithEquipeIDs(req.Context(), []uuid.UUID{equipeID})
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.UpdateProjetoMetadata(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestUpdateProjetoMetadata_SuccessWithEquipes(t *testing.T) {
	id := uuid.New()
	equipeID := uuid.New()
	store := &mockTimelineStore{
		epicoPorID: &domain.Tarefa{ID: id, Tipo: "Épico", NumeroTicket: "BACK-1"},
	}
	h := NewTimelineHandler(store, nil, zapNopLoggerTL())

	body := `{"equipe_ids":["` + equipeID.String() + `"]}`
	req := httptest.NewRequest("PUT", "/", bytes.NewBufferString(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id.String())
	ctx := middleware.ContextWithEquipeIDs(req.Context(), []uuid.UUID{equipeID})
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.UpdateProjetoMetadata(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if len(store.capturedEquipeIDs) != 1 || store.capturedEquipeIDs[0] != equipeID {
		t.Errorf("capturedEquipeIDs = %v, want [%s]", store.capturedEquipeIDs, equipeID)
	}
}
