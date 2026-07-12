package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type mockFetcher struct {
	ids []uuid.UUID
	err error
}

func (m *mockFetcher) BuscarProjetoIDsPorUsuario(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return m.ids, m.err
}

func TestProjetoFilter_InjectsIDs(t *testing.T) {
	projetoIDs := []uuid.UUID{uuid.New(), uuid.New()}
	fetcher := &mockFetcher{ids: projetoIDs}
	userID := uuid.New()

	var capturedIDs []uuid.UUID
	handler := ProjetoFilter(fetcher)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIDs = ProjetoIDsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), userIDKey, userID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if len(capturedIDs) != len(projetoIDs) {
		t.Errorf("got %d projeto IDs, want %d", len(capturedIDs), len(projetoIDs))
	}
}

func TestProjetoFilter_EmptyAlcada(t *testing.T) {
	fetcher := &mockFetcher{ids: nil}
	userID := uuid.New()

	var capturedIDs []uuid.UUID
	handler := ProjetoFilter(fetcher)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIDs = ProjetoIDsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), userIDKey, userID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d — empty alçada must not block", rr.Code, http.StatusOK)
	}
	if capturedIDs != nil {
		t.Errorf("expected nil projeto IDs for empty alçada, got %v", capturedIDs)
	}
}

func TestProjetoFilter_FetcherError(t *testing.T) {
	fetcher := &mockFetcher{err: fmt.Errorf("db connection failed")}
	userID := uuid.New()

	handler := ProjetoFilter(fetcher)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called on fetcher error")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), userIDKey, userID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}
