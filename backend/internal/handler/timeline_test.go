package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"go.uber.org/zap"
)

type mockTimelineStore struct {
	epicos          []domain.EpicoEquipe
	membrosCount    int
	ausencias       []domain.AusenciaMensal
	updateErr       error
	epicoPorID      *domain.Tarefa
	epicosList      []domain.ProjetoListItem
	capturedApelido *string
}

func (m *mockTimelineStore) BuscarEpicosEquipe(_ context.Context, _ uuid.UUID, _ int, _ []uuid.UUID) ([]domain.EpicoEquipe, error) {
	return m.epicos, nil
}

func (m *mockTimelineStore) ContarMembrosAtivosEquipe(_ context.Context, _ uuid.UUID) (int, error) {
	return m.membrosCount, nil
}

func (m *mockTimelineStore) BuscarAusenciasMensais(_ context.Context, _ uuid.UUID, _ int) ([]domain.AusenciaMensal, error) {
	return m.ausencias, nil
}

func (m *mockTimelineStore) AtualizarMetadataProjeto(_ context.Context, _ uuid.UUID, apelido *string, _ *time.Time) error {
	m.capturedApelido = apelido
	return m.updateErr
}

func (m *mockTimelineStore) BuscarEpicoPorID(_ context.Context, _ uuid.UUID) (*domain.Tarefa, error) {
	return m.epicoPorID, nil
}

func (m *mockTimelineStore) ListarEpicos(_ context.Context, _ *uuid.UUID, _ []uuid.UUID) ([]domain.ProjetoListItem, error) {
	return m.epicosList, nil
}

type mockAnalyzer struct {
	result string
	err    error
}

func (m *mockAnalyzer) Analisar(_ context.Context, _ domain.AnaliseCapacidadeInput) (string, error) {
	return m.result, m.err
}

func TestListTimeline_MissingEquipe(t *testing.T) {
	h := NewTimelineHandler(&mockTimelineStore{}, nil, zap.NewNop())
	req := httptest.NewRequest("GET", "/api/v1/timeline-capacidade?ano=2026", nil)
	w := httptest.NewRecorder()

	h.ListTimeline(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestListTimeline_MissingAno(t *testing.T) {
	h := NewTimelineHandler(&mockTimelineStore{}, nil, zap.NewNop())
	req := httptest.NewRequest("GET", "/api/v1/timeline-capacidade?equipe="+uuid.New().String(), nil)
	w := httptest.NewRecorder()

	h.ListTimeline(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestListTimeline_Success(t *testing.T) {
	dl := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	exec := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	apelido := "MIGRAÇÃO DB"

	store := &mockTimelineStore{
		epicos: []domain.EpicoEquipe{
			{
				ID: uuid.New(), NumeroTicket: "BACK-100", Resumo: "Migrar DB",
				Status: "Em Andamento", Apelido: &apelido,
				DataInicioExecucao: &exec, DataLimite: &dl,
				TipoDemanda: strPtr("Meta"), TotalSegundosEquipe: 144000,
				ProjetoCI: false,
			},
		},
		membrosCount: 4,
		ausencias:    []domain.AusenciaMensal{},
	}

	testEquipeID := uuid.New()
	h := NewTimelineHandler(store, nil, zap.NewNop())
	req := httptest.NewRequest("GET", "/api/v1/timeline-capacidade?equipe="+testEquipeID.String()+"&ano=2026", nil)
	w := httptest.NewRecorder()

	h.ListTimeline(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp domain.TimelineResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.Equipe != testEquipeID.String() {
		t.Errorf("Equipe = %q, want %s", resp.Equipe, testEquipeID.String())
	}
	if len(resp.Projetos) != 1 {
		t.Fatalf("Projetos count = %d, want 1", len(resp.Projetos))
	}
	if resp.Projetos[0].TotalDiasEstimados != 5 {
		t.Errorf("TotalDiasEstimados = %.2f, want 5 (144000/3600/8)", resp.Projetos[0].TotalDiasEstimados)
	}
	if len(resp.CapacidadeMensal) != 12 {
		t.Errorf("CapacidadeMensal count = %d, want 12", len(resp.CapacidadeMensal))
	}
}

func TestUpdateProjetoMetadata_NotEpic(t *testing.T) {
	store := &mockTimelineStore{
		epicoPorID: &domain.Tarefa{ID: uuid.New(), Tipo: "Bug", NumeroTicket: "BACK-1"},
	}
	h := NewTimelineHandler(store, nil, zap.NewNop())

	body := `{"apelido": "TEST"}`
	req := httptest.NewRequest("PUT", "/api/v1/projetos/"+uuid.New().String()+"/metadata", bytes.NewBufferString(body))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.UpdateProjetoMetadata(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestUpdateProjetoMetadata_ApelidoTooLong(t *testing.T) {
	store := &mockTimelineStore{
		epicoPorID: &domain.Tarefa{ID: uuid.New(), Tipo: "Épico", NumeroTicket: "BACK-1"},
	}
	h := NewTimelineHandler(store, nil, zap.NewNop())

	body := `{"apelido": "APELIDO MUITO LONGO DEMAIS"}`
	req := httptest.NewRequest("PUT", "/", bytes.NewBufferString(body))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.UpdateProjetoMetadata(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestUpdateProjetoMetadata_UppercaseConversion(t *testing.T) {
	store := &mockTimelineStore{
		epicoPorID: &domain.Tarefa{ID: uuid.New(), Tipo: "Épico", NumeroTicket: "BACK-1"},
	}

	h := NewTimelineHandler(store, nil, zap.NewNop())

	body := `{"apelido": "migração db"}`
	id := uuid.New()
	req := httptest.NewRequest("PUT", "/", bytes.NewBufferString(body))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.UpdateProjetoMetadata(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if store.capturedApelido == nil || *store.capturedApelido != "MIGRAÇÃO DB" {
		t.Errorf("capturedApelido = %v, want MIGRAÇÃO DB", store.capturedApelido)
	}
}

func TestAnalisarCapacidade_NoAnalyzer(t *testing.T) {
	h := NewTimelineHandler(&mockTimelineStore{}, nil, zap.NewNop())

	body := `{"equipe":"00000000-0000-0000-0000-000000000001","ano":2026,"mes":7}`
	req := httptest.NewRequest("POST", "/api/v1/timeline-capacidade/analisar", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.AnalisarCapacidade(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestAnalisarCapacidade_InvalidMes(t *testing.T) {
	h := NewTimelineHandler(&mockTimelineStore{}, &mockAnalyzer{}, zap.NewNop())

	body := `{"equipe":"00000000-0000-0000-0000-000000000001","ano":2026,"mes":13}`
	req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.AnalisarCapacidade(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestListProjetos_Success(t *testing.T) {
	store := &mockTimelineStore{
		epicosList: []domain.ProjetoListItem{
			{ID: uuid.New(), NumeroTicket: "BACK-1", Resumo: "Test", Status: "Backlog"},
		},
	}
	h := NewTimelineHandler(store, nil, zap.NewNop())

	req := httptest.NewRequest("GET", "/api/v1/projetos", nil)
	w := httptest.NewRecorder()

	h.ListProjetos(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func strPtr(s string) *string { return &s }
