package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestUserCargoFromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), userCargoKey, "admin")
	if got := UserCargoFromContext(ctx); got != "admin" {
		t.Errorf("expected 'admin', got %q", got)
	}

	if got := UserCargoFromContext(context.Background()); got != "" {
		t.Errorf("expected empty, got %q", got)
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

func TestAuthJWTPropagaClaimDeSalarios(t *testing.T) {
	ts := auth.NewTokenService("segredo-de-teste", 24)
	expiraEm := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	token, err := ts.GenerateTokenComExpiracao(uuid.New(), "user@myplanner.local", "gerente", true, expiraEm)
	if err != nil {
		t.Fatalf("gerando token: %v", err)
	}

	var pode bool
	var expira time.Time
	handler := AuthJWT(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pode = PodeVerSalarios(r.Context())
		expira = TokenExpiraEm(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/qualquer", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !pode {
		t.Error("PodeVerSalarios = false para token destravado")
	}
	if !expira.Equal(expiraEm) {
		t.Errorf("TokenExpiraEm = %v, esperava %v", expira, expiraEm)
	}
}

func TestAuthJWTTokenDeLoginNaoDestravaSalarios(t *testing.T) {
	ts := auth.NewTokenService("segredo-de-teste", 24)
	token, err := ts.GenerateToken(uuid.New(), "user@myplanner.local", "coordenador")
	if err != nil {
		t.Fatalf("gerando token: %v", err)
	}

	pode := true
	handler := AuthJWT(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pode = PodeVerSalarios(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/qualquer", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if pode {
		t.Error("token de login destravou salários")
	}
}

// Contexto sem middleware (ex.: chamada interna) nunca pode liberar valor.
func TestPodeVerSalariosPadraoEhFalso(t *testing.T) {
	if PodeVerSalarios(context.Background()) {
		t.Error("contexto vazio liberou salários")
	}
}
