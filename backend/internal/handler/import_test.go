package handler

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	mw "github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// requestConfirmarDestravada monta o contexto que Confirmar exige desde a
// Task 6: sem a claim de salários destravada, a rota nem chega a ler o corpo.
func requestConfirmarDestravada(body string) *http.Request {
	req := httptest.NewRequest("POST", "/api/v1/investimentos/import/confirmar", bytes.NewBufferString(body))
	ctx := mw.ContextParaTeste(req.Context(), uuid.New(), "chefe@myplanner.local", "gerente", time.Now().Add(time.Hour))
	ctx = mw.ContextDestravadoParaTeste(ctx)
	return req.WithContext(ctx)
}

type mockImportStore struct {
	matchPlanilhaFn       func(ctx context.Context, csvContent string) (*domain.ImportMatchResult, error)
	fetchGoogleSheetCSVFn func(ctx context.Context, sheetsURL string) (string, string, string, error)
	confirmImportFn       func(ctx context.Context, req domain.ConfirmImportRequest) (*domain.ConfirmImportResponse, error)
	getSyncConfigFn       func(ctx context.Context) (*domain.ImportConfigResponse, error)
	syncFn                func(ctx context.Context) (*domain.ImportMatchResult, error)
}

func (m *mockImportStore) MatchPlanilha(ctx context.Context, csvContent string) (*domain.ImportMatchResult, error) {
	return m.matchPlanilhaFn(ctx, csvContent)
}
func (m *mockImportStore) FetchGoogleSheetCSV(ctx context.Context, sheetsURL string) (string, string, string, error) {
	return m.fetchGoogleSheetCSVFn(ctx, sheetsURL)
}
func (m *mockImportStore) ConfirmImport(ctx context.Context, req domain.ConfirmImportRequest) (*domain.ConfirmImportResponse, error) {
	return m.confirmImportFn(ctx, req)
}
func (m *mockImportStore) GetSyncConfig(ctx context.Context) (*domain.ImportConfigResponse, error) {
	return m.getSyncConfigFn(ctx)
}
func (m *mockImportStore) Sync(ctx context.Context) (*domain.ImportMatchResult, error) {
	return m.syncFn(ctx)
}

func newTestImportHandler(store *mockImportStore) *ImportHandler {
	return NewImportHandler(store, zap.NewNop())
}

func TestImport_Multipart_Success(t *testing.T) {
	var receivedCSV string
	store := &mockImportStore{
		matchPlanilhaFn: func(_ context.Context, csvContent string) (*domain.ImportMatchResult, error) {
			receivedCSV = csvContent
			return &domain.ImportMatchResult{Matched: []domain.ImportMatched{}}, nil
		},
	}
	h := newTestImportHandler(store)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "planilha.csv")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte("Nome,Gestão\nRICARDO,Chefe\n"))
	mw.Close()

	req := httptest.NewRequest("POST", "/api/v1/investimentos/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !strings.Contains(receivedCSV, "RICARDO") {
		t.Errorf("expected csv content passed to service to contain RICARDO, got %q", receivedCSV)
	}
}

func TestImport_SheetsURL_Success(t *testing.T) {
	store := &mockImportStore{
		fetchGoogleSheetCSVFn: func(_ context.Context, url string) (string, string, string, error) {
			if url != "https://docs.google.com/spreadsheets/d/abc/edit" {
				t.Errorf("unexpected url: %q", url)
			}
			return "Nome\nRICARDO\n", "abc", "0", nil
		},
		matchPlanilhaFn: func(_ context.Context, csvContent string) (*domain.ImportMatchResult, error) {
			return &domain.ImportMatchResult{Matched: []domain.ImportMatched{}}, nil
		},
	}
	h := newTestImportHandler(store)

	body := `{"sheets_url":"https://docs.google.com/spreadsheets/d/abc/edit"}`
	req := httptest.NewRequest("POST", "/api/v1/investimentos/import", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestImport_SheetsURL_MissingURL_BadRequest(t *testing.T) {
	store := &mockImportStore{}
	h := newTestImportHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/investimentos/import", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestImport_FetchError_BadRequest(t *testing.T) {
	store := &mockImportStore{
		fetchGoogleSheetCSVFn: func(_ context.Context, _ string) (string, string, string, error) {
			return "", "", "", errPlanilhaPrivada
		},
	}
	h := newTestImportHandler(store)

	body := `{"sheets_url":"https://docs.google.com/spreadsheets/d/abc/edit"}`
	req := httptest.NewRequest("POST", "/api/v1/investimentos/import", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

var errPlanilhaPrivada = &testError{"planilha não está pública"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestConfirmar_Success(t *testing.T) {
	store := &mockImportStore{
		confirmImportFn: func(_ context.Context, req domain.ConfirmImportRequest) (*domain.ConfirmImportResponse, error) {
			if len(req.Linhas) != 1 {
				t.Errorf("got %d linhas, want 1", len(req.Linhas))
			}
			return &domain.ConfirmImportResponse{Atualizados: 1, Ignorados: 0}, nil
		},
	}
	h := newTestImportHandler(store)

	body := `{"linhas":[{"linha":1,"membro_id":"11111111-1111-1111-1111-111111111111","ignorar":false,"dados":{"salario":6480.00}}],"tipo":"csv"}`
	req := requestConfirmarDestravada(body)
	rr := httptest.NewRecorder()
	h.Confirmar(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestConfirmar_EmptyLinhas_BadRequest(t *testing.T) {
	store := &mockImportStore{}
	h := newTestImportHandler(store)

	req := requestConfirmarDestravada(`{"linhas":[]}`)
	rr := httptest.NewRecorder()
	h.Confirmar(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetConfig_Success(t *testing.T) {
	url := "https://docs.google.com/spreadsheets/d/abc/edit"
	store := &mockImportStore{
		getSyncConfigFn: func(_ context.Context) (*domain.ImportConfigResponse, error) {
			return &domain.ImportConfigResponse{Tipo: "sheets_url", URL: &url}, nil
		},
	}
	h := newTestImportHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/investimentos/import/config", nil)
	rr := httptest.NewRecorder()
	h.GetConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestGetConfig_NilConfig(t *testing.T) {
	store := &mockImportStore{
		getSyncConfigFn: func(_ context.Context) (*domain.ImportConfigResponse, error) {
			return nil, nil
		},
	}
	h := newTestImportHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/investimentos/import/config", nil)
	rr := httptest.NewRecorder()
	h.GetConfig(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if strings.TrimSpace(rr.Body.String()) != "null" {
		t.Errorf("body = %q, want null", rr.Body.String())
	}
}

func TestSync_Success(t *testing.T) {
	store := &mockImportStore{
		syncFn: func(_ context.Context) (*domain.ImportMatchResult, error) {
			return &domain.ImportMatchResult{Matched: []domain.ImportMatched{}}, nil
		},
	}
	h := newTestImportHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/investimentos/import/sync", nil)
	rr := httptest.NewRecorder()
	h.Sync(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestSync_NoConfig_BadRequest(t *testing.T) {
	store := &mockImportStore{
		syncFn: func(_ context.Context) (*domain.ImportMatchResult, error) {
			return nil, errPlanilhaPrivada
		},
	}
	h := newTestImportHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/investimentos/import/sync", nil)
	rr := httptest.NewRecorder()
	h.Sync(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}
