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
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
)

func newTestReviewHandlerWithConfig(store *mockReviewStore, cfgStore *mockConfigStore) *ReviewHandler {
	return NewReviewHandler(store, cfgStore, zap.NewNop())
}

// --- GetConfig ---

func TestGetConfig_NotWhitelisted(t *testing.T) {
	h := newTestReviewHandler(&mockReviewStore{})
	req := httptest.NewRequest("GET", "/config/some_random_key", nil)
	req = addChiParam(req, "chave", "some_random_key")
	w := httptest.NewRecorder()

	h.GetConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetConfig_SecretKeyExists(t *testing.T) {
	cfgStore := &mockConfigStore{
		configExistsFn: func(ctx context.Context, chave string) (bool, error) {
			if chave != "openrouter_api_key" {
				t.Errorf("unexpected chave: %s", chave)
			}
			return true, nil
		},
	}
	h := newTestReviewHandlerWithConfig(&mockReviewStore{}, cfgStore)
	req := httptest.NewRequest("GET", "/config/openrouter_api_key", nil)
	req = addChiParam(req, "chave", "openrouter_api_key")
	w := httptest.NewRecorder()

	h.GetConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp map[string]bool
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp["exists"] {
		t.Errorf("expected exists=true")
	}
}

func TestGetConfig_SecretKeyError(t *testing.T) {
	cfgStore := &mockConfigStore{
		configExistsFn: func(ctx context.Context, chave string) (bool, error) {
			return false, errors.New("db error")
		},
	}
	h := newTestReviewHandlerWithConfig(&mockReviewStore{}, cfgStore)
	req := httptest.NewRequest("GET", "/config/smtp_password", nil)
	req = addChiParam(req, "chave", "smtp_password")
	w := httptest.NewRecorder()

	h.GetConfig(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetConfig_PlainValueSuccess(t *testing.T) {
	cfgStore := &mockConfigStore{
		getConfigFn: func(ctx context.Context, chave string) (string, error) {
			if chave != "smtp_host" {
				t.Errorf("unexpected chave: %s", chave)
			}
			return "smtp.example.com", nil
		},
	}
	h := newTestReviewHandlerWithConfig(&mockReviewStore{}, cfgStore)
	req := httptest.NewRequest("GET", "/config/smtp_host", nil)
	req = addChiParam(req, "chave", "smtp_host")
	w := httptest.NewRecorder()

	h.GetConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["valor"] != "smtp.example.com" {
		t.Errorf("valor = %s, want smtp.example.com", resp["valor"])
	}
}

func TestGetConfig_NotFound(t *testing.T) {
	cfgStore := &mockConfigStore{
		getConfigFn: func(ctx context.Context, chave string) (string, error) {
			return "", pgx.ErrNoRows
		},
	}
	h := newTestReviewHandlerWithConfig(&mockReviewStore{}, cfgStore)
	req := httptest.NewRequest("GET", "/config/smtp_host", nil)
	req = addChiParam(req, "chave", "smtp_host")
	w := httptest.NewRecorder()

	h.GetConfig(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetConfig_Error(t *testing.T) {
	cfgStore := &mockConfigStore{
		getConfigFn: func(ctx context.Context, chave string) (string, error) {
			return "", errors.New("db error")
		},
	}
	h := newTestReviewHandlerWithConfig(&mockReviewStore{}, cfgStore)
	req := httptest.NewRequest("GET", "/config/smtp_host", nil)
	req = addChiParam(req, "chave", "smtp_host")
	w := httptest.NewRecorder()

	h.GetConfig(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- SetConfig ---

func TestSetConfig_InvalidBody(t *testing.T) {
	h := newTestReviewHandler(&mockReviewStore{})
	req := httptest.NewRequest("POST", "/config", bytes.NewBufferString(`not-json`))
	w := httptest.NewRecorder()

	h.SetConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSetConfig_NotWhitelisted(t *testing.T) {
	h := newTestReviewHandler(&mockReviewStore{})
	body, _ := json.Marshal(map[string]string{"chave": "random", "valor": "x"})
	req := httptest.NewRequest("POST", "/config", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.SetConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSetConfig_MissingValor(t *testing.T) {
	h := newTestReviewHandler(&mockReviewStore{})
	body, _ := json.Marshal(map[string]string{"chave": "smtp_host", "valor": ""})
	req := httptest.NewRequest("POST", "/config", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.SetConfig(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSetConfig_Success(t *testing.T) {
	var capturedChave, capturedValor string
	cfgStore := &mockConfigStore{
		setConfigFn: func(ctx context.Context, chave, valor string) error {
			capturedChave, capturedValor = chave, valor
			return nil
		},
	}
	h := newTestReviewHandlerWithConfig(&mockReviewStore{}, cfgStore)
	body, _ := json.Marshal(map[string]string{"chave": "smtp_host", "valor": "smtp.example.com"})
	req := httptest.NewRequest("POST", "/config", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.SetConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if capturedChave != "smtp_host" || capturedValor != "smtp.example.com" {
		t.Errorf("unexpected captured: %s/%s", capturedChave, capturedValor)
	}
}

func TestSetConfig_Error(t *testing.T) {
	cfgStore := &mockConfigStore{
		setConfigFn: func(ctx context.Context, chave, valor string) error {
			return errors.New("db error")
		},
	}
	h := newTestReviewHandlerWithConfig(&mockReviewStore{}, cfgStore)
	body, _ := json.Marshal(map[string]string{"chave": "smtp_host", "valor": "x"})
	req := httptest.NewRequest("POST", "/config", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.SetConfig(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- GetReviewAnalise ---

func TestGetReviewAnalise_MissingEquipe(t *testing.T) {
	h := newTestReviewHandler(&mockReviewStore{})
	sprintID := uuid.New()
	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/review/analise", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.GetReviewAnalise(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetReviewAnalise_InvalidSprintID(t *testing.T) {
	h := newTestReviewHandler(&mockReviewStore{})
	req := httptest.NewRequest("GET", "/sprints/xxx/review/analise", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "xxx")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.GetReviewAnalise(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetReviewAnalise_Forbidden(t *testing.T) {
	h := newTestReviewHandler(&mockReviewStore{})
	sprintID := uuid.New()
	equipeID := uuid.New()
	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/review/analise?equipe_id="+equipeID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.GetReviewAnalise(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestGetReviewAnalise_InvalidProdutos(t *testing.T) {
	h := newTestReviewHandler(&mockReviewStore{})
	sprintID := uuid.New()
	equipeID := uuid.New()
	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/review/analise?equipe_id="+equipeID.String()+"&produtos=not-a-uuid", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	ctx := middleware.ContextWithEquipeIDs(context.WithValue(req.Context(), chi.RouteCtxKey, rctx), []uuid.UUID{equipeID})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.GetReviewAnalise(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetReviewAnalise_NotFound(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	store := &mockReviewStore{
		getAnaliseFn: func(ctx context.Context, sid, eid uuid.UUID, pids []uuid.UUID) (*repository.ReviewAnalise, error) {
			return nil, pgx.ErrNoRows
		},
	}
	h := newTestReviewHandler(store)
	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/review/analise?equipe_id="+equipeID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	ctx := middleware.ContextWithEquipeIDs(context.WithValue(req.Context(), chi.RouteCtxKey, rctx), []uuid.UUID{equipeID})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.GetReviewAnalise(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetReviewAnalise_Error(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	store := &mockReviewStore{
		getAnaliseFn: func(ctx context.Context, sid, eid uuid.UUID, pids []uuid.UUID) (*repository.ReviewAnalise, error) {
			return nil, errors.New("db error")
		},
	}
	h := newTestReviewHandler(store)
	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/review/analise?equipe_id="+equipeID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	ctx := middleware.ContextWithEquipeIDs(context.WithValue(req.Context(), chi.RouteCtxKey, rctx), []uuid.UUID{equipeID})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.GetReviewAnalise(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetReviewAnalise_Success(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	produtoID := uuid.New()
	store := &mockReviewStore{
		getAnaliseFn: func(ctx context.Context, sid, eid uuid.UUID, pids []uuid.UUID) (*repository.ReviewAnalise, error) {
			if sid != sprintID || eid != equipeID || len(pids) != 1 || pids[0] != produtoID {
				t.Errorf("unexpected args: sid=%s eid=%s pids=%v", sid, eid, pids)
			}
			return &repository.ReviewAnalise{SprintID: sprintID, EquipeID: equipeID}, nil
		},
	}
	h := newTestReviewHandler(store)
	req := httptest.NewRequest("GET", "/sprints/"+sprintID.String()+"/review/analise?equipe_id="+equipeID.String()+"&produtos="+produtoID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	ctx := middleware.ContextWithEquipeIDs(context.WithValue(req.Context(), chi.RouteCtxKey, rctx), []uuid.UUID{equipeID})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.GetReviewAnalise(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// --- PostReviewAnalise ---

func TestPostReviewAnalise_MissingEquipe(t *testing.T) {
	h := newTestReviewHandler(&mockReviewStore{})
	sprintID := uuid.New()
	req := httptest.NewRequest("POST", "/sprints/"+sprintID.String()+"/review/analise", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.PostReviewAnalise(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestPostReviewAnalise_InvalidSprintID(t *testing.T) {
	h := newTestReviewHandler(&mockReviewStore{})
	req := httptest.NewRequest("POST", "/sprints/xxx/review/analise", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "xxx")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.PostReviewAnalise(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestPostReviewAnalise_Forbidden(t *testing.T) {
	h := newTestReviewHandler(&mockReviewStore{})
	sprintID := uuid.New()
	equipeID := uuid.New()
	req := httptest.NewRequest("POST", "/sprints/"+sprintID.String()+"/review/analise?equipe_id="+equipeID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	h.PostReviewAnalise(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestPostReviewAnalise_InvalidProdutos(t *testing.T) {
	h := newTestReviewHandler(&mockReviewStore{})
	sprintID := uuid.New()
	equipeID := uuid.New()
	req := httptest.NewRequest("POST", "/sprints/"+sprintID.String()+"/review/analise?equipe_id="+equipeID.String()+"&produtos=not-a-uuid", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	ctx := middleware.ContextWithEquipeIDs(context.WithValue(req.Context(), chi.RouteCtxKey, rctx), []uuid.UUID{equipeID})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.PostReviewAnalise(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestPostReviewAnalise_NotConfigured(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	store := &mockReviewStore{
		generateAnaliseFn: func(ctx context.Context, sid, eid uuid.UUID, pids []uuid.UUID) (*repository.ReviewAnalise, error) {
			return nil, errors.New("openrouter api key not configured")
		},
	}
	h := newTestReviewHandler(store)
	req := httptest.NewRequest("POST", "/sprints/"+sprintID.String()+"/review/analise?equipe_id="+equipeID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	ctx := middleware.ContextWithEquipeIDs(context.WithValue(req.Context(), chi.RouteCtxKey, rctx), []uuid.UUID{equipeID})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.PostReviewAnalise(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestPostReviewAnalise_Error(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	store := &mockReviewStore{
		generateAnaliseFn: func(ctx context.Context, sid, eid uuid.UUID, pids []uuid.UUID) (*repository.ReviewAnalise, error) {
			return nil, errors.New("some other error")
		},
	}
	h := newTestReviewHandler(store)
	req := httptest.NewRequest("POST", "/sprints/"+sprintID.String()+"/review/analise?equipe_id="+equipeID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	ctx := middleware.ContextWithEquipeIDs(context.WithValue(req.Context(), chi.RouteCtxKey, rctx), []uuid.UUID{equipeID})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.PostReviewAnalise(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestPostReviewAnalise_Success(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	store := &mockReviewStore{
		generateAnaliseFn: func(ctx context.Context, sid, eid uuid.UUID, pids []uuid.UUID) (*repository.ReviewAnalise, error) {
			return &repository.ReviewAnalise{SprintID: sid, EquipeID: eid}, nil
		},
	}
	h := newTestReviewHandler(store)
	req := httptest.NewRequest("POST", "/sprints/"+sprintID.String()+"/review/analise?equipe_id="+equipeID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sprintID.String())
	ctx := middleware.ContextWithEquipeIDs(context.WithValue(req.Context(), chi.RouteCtxKey, rctx), []uuid.UUID{equipeID})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.PostReviewAnalise(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}
