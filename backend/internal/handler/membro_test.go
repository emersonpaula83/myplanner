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

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

type mockMembroStore struct {
	listFn                   func(ctx context.Context) ([]domain.Membro, error)
	getByIDFn                func(ctx context.Context, id uuid.UUID) (*domain.Membro, error)
	searchFn                 func(ctx context.Context, query string, incluirInativos bool) ([]domain.Membro, error)
	listDisponibilidadeFn    func(ctx context.Context, membroID uuid.UUID) ([]domain.Disponibilidade, error)
	createDisponibilidadeFn  func(ctx context.Context, d *domain.Disponibilidade) error
	updateDisponibilidadeFn  func(ctx context.Context, id uuid.UUID, tipo string, dataInicio, dataFim pgtype.Date, descricao *string) error
	deleteDisponibilidadeFn  func(ctx context.Context, id uuid.UUID) error
	getMembroStatsFn         func(ctx context.Context, membroID uuid.UUID, inicio, fim time.Time) (*domain.MembroStats, error)
	updateDataDesligamentoFn func(ctx context.Context, id uuid.UUID, dataDesligamento *time.Time) error
	updateAtivoFn            func(ctx context.Context, id uuid.UUID, ativo bool) error
}

// List e GetByID toleram o campo nil: os testes da cortina de salários montam o
// mock com só o método que exercitam.
func (m *mockMembroStore) List(ctx context.Context) ([]domain.Membro, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func (m *mockMembroStore) GetByID(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockMembroStore) Search(ctx context.Context, query string, incluirInativos bool) ([]domain.Membro, error) {
	if m.searchFn == nil {
		return nil, nil
	}
	return m.searchFn(ctx, query, incluirInativos)
}

func (m *mockMembroStore) ListDisponibilidade(ctx context.Context, membroID uuid.UUID) ([]domain.Disponibilidade, error) {
	if m.listDisponibilidadeFn == nil {
		return nil, nil
	}
	return m.listDisponibilidadeFn(ctx, membroID)
}

func (m *mockMembroStore) CreateDisponibilidade(ctx context.Context, d *domain.Disponibilidade) error {
	return m.createDisponibilidadeFn(ctx, d)
}

func (m *mockMembroStore) UpdateDisponibilidade(ctx context.Context, id uuid.UUID, tipo string, dataInicio, dataFim pgtype.Date, descricao *string) error {
	return m.updateDisponibilidadeFn(ctx, id, tipo, dataInicio, dataFim, descricao)
}

func (m *mockMembroStore) DeleteDisponibilidade(ctx context.Context, id uuid.UUID) error {
	return m.deleteDisponibilidadeFn(ctx, id)
}

func (m *mockMembroStore) GetMembroStats(ctx context.Context, membroID uuid.UUID, inicio, fim time.Time) (*domain.MembroStats, error) {
	if m.getMembroStatsFn == nil {
		return &domain.MembroStats{}, nil
	}
	return m.getMembroStatsFn(ctx, membroID, inicio, fim)
}

func (m *mockMembroStore) UpdateDataDesligamento(ctx context.Context, id uuid.UUID, dataDesligamento *time.Time) error {
	return m.updateDataDesligamentoFn(ctx, id, dataDesligamento)
}

func (m *mockMembroStore) UpdateAtivo(ctx context.Context, id uuid.UUID, ativo bool) error {
	return m.updateAtivoFn(ctx, id, ativo)
}

// --- List ---

func TestMembro_List_Success(t *testing.T) {
	store := &mockMembroStore{
		listFn: func(ctx context.Context) ([]domain.Membro, error) {
			return []domain.Membro{{ID: uuid.New(), Nome: "Ana"}}, nil
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/membros", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMembro_List_Error(t *testing.T) {
	store := &mockMembroStore{
		listFn: func(ctx context.Context) ([]domain.Membro, error) {
			return nil, errors.New("db error")
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/membros", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// --- Search ---

func TestMembro_Search_Success(t *testing.T) {
	store := &mockMembroStore{
		searchFn: func(ctx context.Context, query string, incluirInativos bool) ([]domain.Membro, error) {
			if query != "ana" {
				t.Errorf("expected query 'ana', got %q", query)
			}
			return []domain.Membro{{ID: uuid.New(), Nome: "Ana"}}, nil
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/membros/search?q=ana", nil)
	w := httptest.NewRecorder()

	h.Search(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMembro_Search_Error(t *testing.T) {
	store := &mockMembroStore{
		searchFn: func(ctx context.Context, query string, incluirInativos bool) ([]domain.Membro, error) {
			return nil, errors.New("db error")
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/membros/search?q=ana", nil)
	w := httptest.NewRecorder()

	h.Search(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestMembro_Search_EmptyQuery(t *testing.T) {
	store := &mockMembroStore{
		searchFn: func(ctx context.Context, query string, incluirInativos bool) ([]domain.Membro, error) {
			t.Fatal("searchFn should not be called for empty query")
			return nil, nil
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/membros/search", nil)
	w := httptest.NewRecorder()

	h.Search(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// --- GetByID ---

func TestMembro_GetByID_Success(t *testing.T) {
	membroID := uuid.New()
	store := &mockMembroStore{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
			return &domain.Membro{ID: id, Nome: "Ana"}, nil
		},
		getMembroStatsFn: func(ctx context.Context, membroID uuid.UUID, inicio, fim time.Time) (*domain.MembroStats, error) {
			return &domain.MembroStats{TotalTarefas: 5}, nil
		},
		listDisponibilidadeFn: func(ctx context.Context, membroID uuid.UUID) ([]domain.Disponibilidade, error) {
			return []domain.Disponibilidade{}, nil
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/membros/"+membroID.String(), nil)
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.GetByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMembro_GetByID_NotFound(t *testing.T) {
	membroID := uuid.New()
	store := &mockMembroStore{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
			return nil, nil
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/membros/"+membroID.String(), nil)
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.GetByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestMembro_GetByID_Error(t *testing.T) {
	membroID := uuid.New()
	store := &mockMembroStore{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
			return nil, errors.New("db error")
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/membros/"+membroID.String(), nil)
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.GetByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestMembro_GetByID_InvalidID(t *testing.T) {
	store := &mockMembroStore{}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/membros/not-a-uuid", nil)
	req = addChiParam(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	h.GetByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestMembro_GetByID_InvalidPeriodo(t *testing.T) {
	membroID := uuid.New()
	store := &mockMembroStore{}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/membros/"+membroID.String()+"?periodo=bogus", nil)
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.GetByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- CreateDisponibilidade ---

func TestMembro_CreateDisponibilidade_Success(t *testing.T) {
	membroID := uuid.New()
	store := &mockMembroStore{
		createDisponibilidadeFn: func(ctx context.Context, d *domain.Disponibilidade) error {
			if d.MembroID != membroID {
				t.Errorf("expected membroID %s, got %s", membroID, d.MembroID)
			}
			return nil
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	body, _ := json.Marshal(map[string]any{
		"tipo":        "ferias",
		"data_inicio": "2026-01-01",
		"data_fim":    "2026-01-10",
	})
	req := httptest.NewRequest(http.MethodPost, "/membros/"+membroID.String()+"/disponibilidade", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.CreateDisponibilidade(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestMembro_CreateDisponibilidade_Error(t *testing.T) {
	membroID := uuid.New()
	store := &mockMembroStore{
		createDisponibilidadeFn: func(ctx context.Context, d *domain.Disponibilidade) error {
			return errors.New("db error")
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	body, _ := json.Marshal(map[string]any{
		"tipo":        "ferias",
		"data_inicio": "2026-01-01",
		"data_fim":    "2026-01-10",
	})
	req := httptest.NewRequest(http.MethodPost, "/membros/"+membroID.String()+"/disponibilidade", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.CreateDisponibilidade(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestMembro_CreateDisponibilidade_InvalidBody(t *testing.T) {
	membroID := uuid.New()
	store := &mockMembroStore{}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/membros/"+membroID.String()+"/disponibilidade", bytes.NewReader([]byte("{invalid")))
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.CreateDisponibilidade(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestMembro_CreateDisponibilidade_MissingFields(t *testing.T) {
	membroID := uuid.New()
	store := &mockMembroStore{}
	h := NewMembroHandler(store, zap.NewNop())
	body, _ := json.Marshal(map[string]any{"tipo": "ferias"})
	req := httptest.NewRequest(http.MethodPost, "/membros/"+membroID.String()+"/disponibilidade", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.CreateDisponibilidade(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestMembro_CreateDisponibilidade_InvalidID(t *testing.T) {
	store := &mockMembroStore{}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/membros/not-a-uuid/disponibilidade", nil)
	req = addChiParam(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	h.CreateDisponibilidade(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- UpdateDisponibilidade ---

func TestMembro_UpdateDisponibilidade_Success(t *testing.T) {
	dispID := uuid.New()
	store := &mockMembroStore{
		updateDisponibilidadeFn: func(ctx context.Context, id uuid.UUID, tipo string, dataInicio, dataFim pgtype.Date, descricao *string) error {
			if id != dispID {
				t.Errorf("expected dispID %s, got %s", dispID, id)
			}
			return nil
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	body, _ := json.Marshal(map[string]any{
		"tipo":        "ferias",
		"data_inicio": "2026-01-01",
		"data_fim":    "2026-01-10",
	})
	req := httptest.NewRequest(http.MethodPut, "/membros/disponibilidade/"+dispID.String(), bytes.NewReader(body))
	req = addChiParam(req, "dispId", dispID.String())
	w := httptest.NewRecorder()

	h.UpdateDisponibilidade(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestMembro_UpdateDisponibilidade_Error(t *testing.T) {
	dispID := uuid.New()
	store := &mockMembroStore{
		updateDisponibilidadeFn: func(ctx context.Context, id uuid.UUID, tipo string, dataInicio, dataFim pgtype.Date, descricao *string) error {
			return errors.New("db error")
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	body, _ := json.Marshal(map[string]any{
		"tipo":        "ferias",
		"data_inicio": "2026-01-01",
		"data_fim":    "2026-01-10",
	})
	req := httptest.NewRequest(http.MethodPut, "/membros/disponibilidade/"+dispID.String(), bytes.NewReader(body))
	req = addChiParam(req, "dispId", dispID.String())
	w := httptest.NewRecorder()

	h.UpdateDisponibilidade(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestMembro_UpdateDisponibilidade_InvalidID(t *testing.T) {
	store := &mockMembroStore{}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodPut, "/membros/disponibilidade/not-a-uuid", nil)
	req = addChiParam(req, "dispId", "not-a-uuid")
	w := httptest.NewRecorder()

	h.UpdateDisponibilidade(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- DeleteDisponibilidade ---

func TestMembro_DeleteDisponibilidade_Success(t *testing.T) {
	dispID := uuid.New()
	store := &mockMembroStore{
		deleteDisponibilidadeFn: func(ctx context.Context, id uuid.UUID) error {
			if id != dispID {
				t.Errorf("expected dispID %s, got %s", dispID, id)
			}
			return nil
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodDelete, "/membros/disponibilidade/"+dispID.String(), nil)
	req = addChiParam(req, "dispId", dispID.String())
	w := httptest.NewRecorder()

	h.DeleteDisponibilidade(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMembro_DeleteDisponibilidade_Error(t *testing.T) {
	dispID := uuid.New()
	store := &mockMembroStore{
		deleteDisponibilidadeFn: func(ctx context.Context, id uuid.UUID) error {
			return errors.New("db error")
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodDelete, "/membros/disponibilidade/"+dispID.String(), nil)
	req = addChiParam(req, "dispId", dispID.String())
	w := httptest.NewRecorder()

	h.DeleteDisponibilidade(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestMembro_DeleteDisponibilidade_InvalidID(t *testing.T) {
	store := &mockMembroStore{}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodDelete, "/membros/disponibilidade/not-a-uuid", nil)
	req = addChiParam(req, "dispId", "not-a-uuid")
	w := httptest.NewRecorder()

	h.DeleteDisponibilidade(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- UpdateDataDesligamento ---

func TestMembro_UpdateDataDesligamento_Success(t *testing.T) {
	membroID := uuid.New()
	store := &mockMembroStore{
		updateDataDesligamentoFn: func(ctx context.Context, id uuid.UUID, dataDesligamento *time.Time) error {
			if id != membroID {
				t.Errorf("expected membroID %s, got %s", membroID, id)
			}
			if dataDesligamento == nil {
				t.Errorf("expected non-nil dataDesligamento")
			}
			return nil
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	data := "2026-02-01"
	body, _ := json.Marshal(map[string]any{"data_desligamento": &data})
	req := httptest.NewRequest(http.MethodPut, "/membros/"+membroID.String()+"/desligamento", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.UpdateDataDesligamento(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestMembro_UpdateDataDesligamento_Error(t *testing.T) {
	membroID := uuid.New()
	store := &mockMembroStore{
		updateDataDesligamentoFn: func(ctx context.Context, id uuid.UUID, dataDesligamento *time.Time) error {
			return errors.New("db error")
		},
	}
	h := NewMembroHandler(store, zap.NewNop())
	body, _ := json.Marshal(map[string]any{"data_desligamento": nil})
	req := httptest.NewRequest(http.MethodPut, "/membros/"+membroID.String()+"/desligamento", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.UpdateDataDesligamento(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestMembro_UpdateDataDesligamento_InvalidID(t *testing.T) {
	store := &mockMembroStore{}
	h := NewMembroHandler(store, zap.NewNop())
	req := httptest.NewRequest(http.MethodPut, "/membros/not-a-uuid/desligamento", nil)
	req = addChiParam(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	h.UpdateDataDesligamento(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestMembro_UpdateDataDesligamento_InvalidDate(t *testing.T) {
	membroID := uuid.New()
	store := &mockMembroStore{}
	h := NewMembroHandler(store, zap.NewNop())
	data := "not-a-date"
	body, _ := json.Marshal(map[string]any{"data_desligamento": &data})
	req := httptest.NewRequest(http.MethodPut, "/membros/"+membroID.String()+"/desligamento", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.UpdateDataDesligamento(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
