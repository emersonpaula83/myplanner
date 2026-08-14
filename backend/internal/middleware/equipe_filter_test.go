package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

const testAdminEmail = "admin@myplanner.local"

type mockEquipeFetcher struct {
	ids []uuid.UUID
	err error
}

func (m *mockEquipeFetcher) BuscarEquipeIDsPorUsuario(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return m.ids, m.err
}

type mockAllEquipesFetcher struct {
	ids []uuid.UUID
	err error
}

func (m *mockAllEquipesFetcher) ListarTodosIDs(_ context.Context) ([]uuid.UUID, error) {
	return m.ids, m.err
}

func TestEquipeFilter_InjectsIDs(t *testing.T) {
	equipeIDs := []uuid.UUID{uuid.New(), uuid.New()}
	fetcher := &mockEquipeFetcher{ids: equipeIDs}
	allFetcher := &mockAllEquipesFetcher{}
	userID := uuid.New()

	var capturedIDs []uuid.UUID
	handler := EquipeFilter(fetcher, allFetcher, testAdminEmail)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIDs = EquipeIDsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), userIDKey, userID)
	ctx = context.WithValue(ctx, userEmailKey, "user@example.com")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if len(capturedIDs) != len(equipeIDs) {
		t.Errorf("got %d equipe IDs, want %d", len(capturedIDs), len(equipeIDs))
	}
}

func TestEquipeFilter_EmptyAlcada(t *testing.T) {
	fetcher := &mockEquipeFetcher{ids: nil}
	allFetcher := &mockAllEquipesFetcher{}
	userID := uuid.New()

	var capturedIDs []uuid.UUID
	handler := EquipeFilter(fetcher, allFetcher, testAdminEmail)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIDs = EquipeIDsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), userIDKey, userID)
	ctx = context.WithValue(ctx, userEmailKey, "user@example.com")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d — empty alçada must not block", rr.Code, http.StatusOK)
	}
	if capturedIDs != nil {
		t.Errorf("expected nil equipe IDs for empty alçada, got %v", capturedIDs)
	}
}

func TestEquipeFilter_FetcherError(t *testing.T) {
	fetcher := &mockEquipeFetcher{err: fmt.Errorf("db connection failed")}
	allFetcher := &mockAllEquipesFetcher{}
	userID := uuid.New()

	handler := EquipeFilter(fetcher, allFetcher, testAdminEmail)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called on fetcher error")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), userIDKey, userID)
	ctx = context.WithValue(ctx, userEmailKey, "user@example.com")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestEquipeFilter_Unauthenticated(t *testing.T) {
	fetcher := &mockEquipeFetcher{}
	allFetcher := &mockAllEquipesFetcher{}

	handler := EquipeFilter(fetcher, allFetcher, testAdminEmail)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called when unauthenticated")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestEquipeFilter_AdminGetsAllEquipes(t *testing.T) {
	allIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	fetcher := &mockEquipeFetcher{ids: []uuid.UUID{uuid.New()}} // should NOT be used
	allFetcher := &mockAllEquipesFetcher{ids: allIDs}
	userID := uuid.New()

	var capturedIDs []uuid.UUID
	handler := EquipeFilter(fetcher, allFetcher, testAdminEmail)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIDs = EquipeIDsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), userIDKey, userID)
	ctx = context.WithValue(ctx, userEmailKey, testAdminEmail)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if len(capturedIDs) != len(allIDs) {
		t.Errorf("got %d equipe IDs, want %d (all equipes)", len(capturedIDs), len(allIDs))
	}
}

func TestEquipeFilter_AdminFetcherError(t *testing.T) {
	fetcher := &mockEquipeFetcher{}
	allFetcher := &mockAllEquipesFetcher{err: fmt.Errorf("db connection failed")}
	userID := uuid.New()

	handler := EquipeFilter(fetcher, allFetcher, testAdminEmail)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called on allFetcher error")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), userIDKey, userID)
	ctx = context.WithValue(ctx, userEmailKey, testAdminEmail)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestContextWithEquipeIDs(t *testing.T) {
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	ctx := ContextWithEquipeIDs(context.Background(), ids)

	got := EquipeIDsFromContext(ctx)
	if len(got) != len(ids) {
		t.Fatalf("expected %d equipe IDs, got %d", len(ids), len(got))
	}
	for i, id := range ids {
		if got[i] != id {
			t.Errorf("index %d: expected %s, got %s", i, id, got[i])
		}
	}
}

func TestValidateEquipeAccess(t *testing.T) {
	allowedID := uuid.New()
	otherID := uuid.New()

	t.Run("no alcada configured", func(t *testing.T) {
		ctx := context.Background()
		err := ValidateEquipeAccess(ctx, []uuid.UUID{allowedID})
		if err == nil {
			t.Fatal("expected error when no alçada configured")
		}
	})

	t.Run("requested ID allowed", func(t *testing.T) {
		ctx := ContextWithEquipeIDs(context.Background(), []uuid.UUID{allowedID, otherID})
		err := ValidateEquipeAccess(ctx, []uuid.UUID{allowedID})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("requested ID not allowed", func(t *testing.T) {
		ctx := ContextWithEquipeIDs(context.Background(), []uuid.UUID{allowedID})
		err := ValidateEquipeAccess(ctx, []uuid.UUID{otherID})
		if err == nil {
			t.Fatal("expected error for disallowed equipe ID")
		}
	})

	t.Run("empty requested IDs always pass when alcada exists", func(t *testing.T) {
		ctx := ContextWithEquipeIDs(context.Background(), []uuid.UUID{allowedID})
		err := ValidateEquipeAccess(ctx, nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
