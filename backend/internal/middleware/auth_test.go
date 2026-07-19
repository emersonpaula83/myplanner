package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/auth"
)

func TestAuthJWT_ValidToken(t *testing.T) {
	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	userID := uuid.New()
	token, _ := ts.GenerateToken(userID, "test@example.com", "coordenador")

	var capturedUserID uuid.UUID
	handler := AuthJWT(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if capturedUserID != userID {
		t.Errorf("userID = %s, want %s", capturedUserID, userID)
	}
}

func TestAuthJWT_MissingToken(t *testing.T) {
	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	handler := AuthJWT(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAuthJWT_InvalidToken(t *testing.T) {
	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	handler := AuthJWT(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAuthJWT_WrongScheme(t *testing.T) {
	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	token, _ := ts.GenerateToken(uuid.New(), "test@example.com", "gerente")

	handler := AuthJWT(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}
