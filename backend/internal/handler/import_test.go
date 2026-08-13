package handler

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"go.uber.org/zap"
)

type mockImportStore struct {
	matchPlanilhaFn       func(ctx context.Context, csvContent string) (*domain.ImportMatchResult, error)
	fetchGoogleSheetCSVFn func(ctx context.Context, sheetsURL string) (string, string, string, error)
}

func (m *mockImportStore) MatchPlanilha(ctx context.Context, csvContent string) (*domain.ImportMatchResult, error) {
	return m.matchPlanilhaFn(ctx, csvContent)
}
func (m *mockImportStore) FetchGoogleSheetCSV(ctx context.Context, sheetsURL string) (string, string, string, error) {
	return m.fetchGoogleSheetCSVFn(ctx, sheetsURL)
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
