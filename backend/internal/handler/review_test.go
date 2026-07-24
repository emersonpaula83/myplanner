package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
)

type mockReviewStore struct {
	getReviewDataFn  func(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*service.ReviewData, error)
	listDestaquesFn  func(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error)
	createDestaqueFn func(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error)
	updateDestaqueFn func(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error)
	deleteDestaqueFn func(ctx context.Context, id uuid.UUID) error
}

func (m *mockReviewStore) GetReviewData(ctx context.Context, sprintID, equipeID uuid.UUID, produtoIDs []uuid.UUID) (*service.ReviewData, error) {
	return m.getReviewDataFn(ctx, sprintID, equipeID, produtoIDs)
}
func (m *mockReviewStore) ListDestaques(ctx context.Context, sprintID, equipeID uuid.UUID) ([]repository.ReviewDestaque, error) {
	return m.listDestaquesFn(ctx, sprintID, equipeID)
}
func (m *mockReviewStore) CreateDestaque(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error) {
	return m.createDestaqueFn(ctx, d)
}
func (m *mockReviewStore) UpdateDestaque(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error) {
	return m.updateDestaqueFn(ctx, id, titulo, descricao, link)
}
func (m *mockReviewStore) DeleteDestaque(ctx context.Context, id uuid.UUID) error {
	return m.deleteDestaqueFn(ctx, id)
}

func newTestReviewHandler(store *mockReviewStore) *ReviewHandler {
	return NewReviewHandler(store, zap.NewNop())
}

func TestGetReviewData(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()

	store := &mockReviewStore{
		getReviewDataFn: func(ctx context.Context, sid, eid uuid.UUID, pids []uuid.UUID) (*service.ReviewData, error) {
			if sid != sprintID || eid != equipeID {
				t.Errorf("unexpected IDs: sprint=%s equipe=%s", sid, eid)
			}
			return &service.ReviewData{
				Stats: service.ReviewStats{Total: 10, Concluidas: 5},
			}, nil
		},
	}
	h := newTestReviewHandler(store)

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/review?equipe_id="+equipeID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetReviewData(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result service.ReviewData
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Stats.Total != 10 {
		t.Errorf("expected total=10, got %d", result.Stats.Total)
	}
}

func TestGetReviewDataMissingEquipe(t *testing.T) {
	h := newTestReviewHandler(&mockReviewStore{})
	sprintID := uuid.New()

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/review", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetReviewData(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateDestaque(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	produtoID := uuid.New()
	destaqueID := uuid.New()

	store := &mockReviewStore{
		createDestaqueFn: func(ctx context.Context, d repository.ReviewDestaque) (repository.ReviewDestaque, error) {
			d.ID = destaqueID
			return d, nil
		},
	}
	h := newTestReviewHandler(store)

	body, _ := json.Marshal(map[string]string{
		"equipe_id":  equipeID.String(),
		"produto_id": produtoID.String(),
		"titulo":     "Test Title",
		"descricao":  "Test Description",
	})
	req := httptest.NewRequest("POST", "/sprints/"+sprintID.String()+"/review/destaques", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("sprintId", sprintID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.CreateDestaque(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetReviewDataProdutosValid(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	produtoID1 := uuid.New()
	produtoID2 := uuid.New()

	store := &mockReviewStore{
		getReviewDataFn: func(ctx context.Context, sid, eid uuid.UUID, pids []uuid.UUID) (*service.ReviewData, error) {
			if len(pids) != 2 || pids[0] != produtoID1 || pids[1] != produtoID2 {
				t.Errorf("unexpected produto ids: %v", pids)
			}
			return &service.ReviewData{}, nil
		},
	}
	h := newTestReviewHandler(store)

	url := "/sprints/" + sprintID.String() + "/review?equipe_id=" + equipeID.String() +
		"&produtos=" + produtoID1.String() + "," + produtoID2.String()
	req := httptest.NewRequest("GET", url, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetReviewData(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetReviewDataProdutosInvalid(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	produtoID1 := uuid.New()

	h := newTestReviewHandler(&mockReviewStore{})

	url := "/sprints/" + sprintID.String() + "/review?equipe_id=" + equipeID.String() +
		"&produtos=" + produtoID1.String() + ",not-a-uuid"
	req := httptest.NewRequest("GET", url, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetReviewData(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateDestaqueNotFound(t *testing.T) {
	destaqueID := uuid.New()
	store := &mockReviewStore{
		updateDestaqueFn: func(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error) {
			return repository.ReviewDestaque{}, pgx.ErrNoRows
		},
	}
	h := newTestReviewHandler(store)

	body, _ := json.Marshal(map[string]string{
		"titulo":    "Test Title",
		"descricao": "Test Description",
	})
	req := httptest.NewRequest("PUT", "/destaques/"+destaqueID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", destaqueID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.UpdateDestaque(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteDestaqueNotFound(t *testing.T) {
	destaqueID := uuid.New()
	store := &mockReviewStore{
		deleteDestaqueFn: func(ctx context.Context, id uuid.UUID) error {
			return pgx.ErrNoRows
		},
	}
	h := newTestReviewHandler(store)

	req := httptest.NewRequest("DELETE", "/destaques/"+destaqueID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", destaqueID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.DeleteDestaque(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListDestaquesSuccess(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	destaqueID := uuid.New()

	store := &mockReviewStore{
		listDestaquesFn: func(ctx context.Context, sid, eid uuid.UUID) ([]repository.ReviewDestaque, error) {
			if sid != sprintID || eid != equipeID {
				t.Errorf("unexpected IDs: sprint=%s equipe=%s", sid, eid)
			}
			return []repository.ReviewDestaque{
				{ID: destaqueID, SprintID: sprintID, EquipeID: equipeID, Titulo: "Test Title", Descricao: "Test Description"},
			}, nil
		},
	}
	h := newTestReviewHandler(store)

	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/review/destaques?equipe_id="+equipeID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("sprintId", sprintID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.ListDestaques(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result []repository.ReviewDestaque
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 || result[0].ID != destaqueID {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestUpdateDestaqueSuccess(t *testing.T) {
	destaqueID := uuid.New()

	store := &mockReviewStore{
		updateDestaqueFn: func(ctx context.Context, id uuid.UUID, titulo, descricao string, link *string) (repository.ReviewDestaque, error) {
			if id != destaqueID {
				t.Errorf("unexpected ID: %s", id)
			}
			return repository.ReviewDestaque{ID: id, Titulo: titulo, Descricao: descricao, Link: link}, nil
		},
	}
	h := newTestReviewHandler(store)

	body, _ := json.Marshal(map[string]string{
		"titulo":    "Updated Title",
		"descricao": "Updated Description",
	})
	req := httptest.NewRequest("PUT", "/destaques/"+destaqueID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", destaqueID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.UpdateDestaque(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result repository.ReviewDestaque
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.ID != destaqueID || result.Titulo != "Updated Title" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestCreateDestaqueInvalidLink(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	produtoID := uuid.New()

	h := newTestReviewHandler(&mockReviewStore{})

	body, _ := json.Marshal(map[string]string{
		"equipe_id":  equipeID.String(),
		"produto_id": produtoID.String(),
		"titulo":     "Test Title",
		"descricao":  "Test Description",
		"link":       "javascript:alert(1)",
	})
	req := httptest.NewRequest("POST", "/sprints/"+sprintID.String()+"/review/destaques", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("sprintId", sprintID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.CreateDestaque(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateDestaqueInvalidLink(t *testing.T) {
	destaqueID := uuid.New()

	h := newTestReviewHandler(&mockReviewStore{})

	body, _ := json.Marshal(map[string]string{
		"titulo":    "Test Title",
		"descricao": "Test Description",
		"link":      "javascript:alert(1)",
	})
	req := httptest.NewRequest("PUT", "/destaques/"+destaqueID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", destaqueID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.UpdateDestaque(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteDestaque(t *testing.T) {
	destaqueID := uuid.New()
	store := &mockReviewStore{
		deleteDestaqueFn: func(ctx context.Context, id uuid.UUID) error {
			if id != destaqueID {
				t.Errorf("unexpected ID: %s", id)
			}
			return nil
		},
	}
	h := newTestReviewHandler(store)

	req := httptest.NewRequest("DELETE", "/destaques/"+destaqueID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", destaqueID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.DeleteDestaque(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
