package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/auth"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	mw "github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type mockSalarioLockStore struct {
	buscarPorEmailFn func(ctx context.Context, email string) (*domain.Usuario, error)
}

func (m *mockSalarioLockStore) BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error) {
	return m.buscarPorEmailFn(ctx, email)
}

func usuarioDeTeste(t *testing.T, email, cargo, senha string) *domain.Usuario {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(senha), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("gerando hash: %v", err)
	}
	h := string(hash)
	return &domain.Usuario{ID: uuid.New(), Email: email, Cargo: cargo, SenhaHash: &h, AuthProvider: "local"}
}

// requestDestravar monta a requisição já com o que o middleware de auth injeta.
func requestDestravar(corpo, email, cargo string, expiraEm time.Time) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/auth/desbloquear-salarios", strings.NewReader(corpo))
	ctx := mw.ContextParaTeste(req.Context(), uuid.New(), email, cargo, expiraEm)
	return req.WithContext(ctx)
}

func TestDesbloquearRecusaCargoSemPermissao(t *testing.T) {
	chamou := false
	store := &mockSalarioLockStore{buscarPorEmailFn: func(context.Context, string) (*domain.Usuario, error) {
		chamou = true
		return nil, nil
	}}
	h := NewSalarioLockHandler(store, auth.NewTokenService("s", 24), "admin@myplanner.local", zap.NewNop())

	w := httptest.NewRecorder()
	h.Desbloquear(w, requestDestravar(`{"senha":"seja-la-qual"}`, "dev@myplanner.local", "gerente_projetos", time.Now().Add(time.Hour)))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, esperava 403. Corpo: %s", w.Code, w.Body.String())
	}
	if chamou {
		t.Error("cargo sem permissão não deveria nem consultar o usuário")
	}
}

func TestDesbloquearRecusaSenhaErrada(t *testing.T) {
	u := usuarioDeTeste(t, "chefe@myplanner.local", "gerente", "senha-certa")
	store := &mockSalarioLockStore{buscarPorEmailFn: func(context.Context, string) (*domain.Usuario, error) {
		return u, nil
	}}
	h := NewSalarioLockHandler(store, auth.NewTokenService("s", 24), "admin@myplanner.local", zap.NewNop())

	w := httptest.NewRecorder()
	h.Desbloquear(w, requestDestravar(`{"senha":"senha-errada"}`, u.Email, u.Cargo, time.Now().Add(time.Hour)))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, esperava 401", w.Code)
	}
}

func TestDesbloquearDevolveTokenComClaimEMesmaExpiracao(t *testing.T) {
	u := usuarioDeTeste(t, "chefe@myplanner.local", "diretor", "senha-certa")
	store := &mockSalarioLockStore{buscarPorEmailFn: func(context.Context, string) (*domain.Usuario, error) {
		return u, nil
	}}
	ts := auth.NewTokenService("s", 24)
	h := NewSalarioLockHandler(store, ts, "admin@myplanner.local", zap.NewNop())
	expiraEm := time.Now().Add(90 * time.Minute).Truncate(time.Second)

	w := httptest.NewRecorder()
	h.Desbloquear(w, requestDestravar(`{"senha":"senha-certa"}`, u.Email, u.Cargo, expiraEm))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, esperava 200. Corpo: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta inválida: %v", err)
	}
	claims, err := ts.ValidateToken(resp.Token)
	if err != nil {
		t.Fatalf("token devolvido não valida: %v", err)
	}
	if !claims.Salarios {
		t.Error("token devolvido veio sem a claim de salários")
	}
	if !claims.ExpiresAt.Time.Equal(expiraEm) {
		t.Errorf("expiração = %v, esperava %v — destravar não pode estender a sessão", claims.ExpiresAt.Time, expiraEm)
	}
}

// A conta admin passa mesmo com cargo fora da lista.
func TestDesbloquearAceitaContaAdmin(t *testing.T) {
	u := usuarioDeTeste(t, "admin@myplanner.local", "gerente_projetos", "senha-certa")
	store := &mockSalarioLockStore{buscarPorEmailFn: func(context.Context, string) (*domain.Usuario, error) {
		return u, nil
	}}
	h := NewSalarioLockHandler(store, auth.NewTokenService("s", 24), "admin@myplanner.local", zap.NewNop())

	w := httptest.NewRecorder()
	h.Desbloquear(w, requestDestravar(`{"senha":"senha-certa"}`, u.Email, u.Cargo, time.Now().Add(time.Hour)))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, esperava 200 para a conta admin. Corpo: %s", w.Code, w.Body.String())
	}
}

func TestTravarDevolveTokenSemClaim(t *testing.T) {
	ts := auth.NewTokenService("s", 24)
	h := NewSalarioLockHandler(&mockSalarioLockStore{}, ts, "admin@myplanner.local", zap.NewNop())
	expiraEm := time.Now().Add(time.Hour).Truncate(time.Second)

	w := httptest.NewRecorder()
	h.Travar(w, requestDestravar("", "chefe@myplanner.local", "gerente", expiraEm))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, esperava 200. Corpo: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta inválida: %v", err)
	}
	claims, err := ts.ValidateToken(resp.Token)
	if err != nil {
		t.Fatalf("token devolvido não valida: %v", err)
	}
	if claims.Salarios {
		t.Error("travar devolveu token ainda destravado")
	}
	if !claims.ExpiresAt.Time.Equal(expiraEm) {
		t.Errorf("travar mexeu na expiração: %v, esperava %v", claims.ExpiresAt.Time, expiraEm)
	}
}

// Achado da revisão: TokenExpiraEm devolve time.Time{} quando o contexto não
// carrega expiração. Hoje isso é inalcançável em produção (AuthJWT sempre
// grava a expiração de todo token emitido), mas se algum dia deixar de ser
// verdade, um zero aqui geraria um token que já nasce expirado — deslogando o
// usuário sem explicação nenhuma. O handler recusa explicitamente em vez de
// deixar esse logout silencioso acontecer.
func TestTravarRecusaContextoSemExpiracao(t *testing.T) {
	h := NewSalarioLockHandler(&mockSalarioLockStore{}, auth.NewTokenService("s", 24), "admin@myplanner.local", zap.NewNop())

	w := httptest.NewRecorder()
	h.Travar(w, requestDestravar("", "chefe@myplanner.local", "gerente", time.Time{}))

	if w.Code == http.StatusOK {
		t.Fatalf("status = %d: token com expiração zero nasceria expirado", w.Code)
	}
	if strings.Contains(w.Body.String(), `"token"`) {
		t.Errorf("não devia devolver token para contexto sem expiração: %s", w.Body.String())
	}
}
