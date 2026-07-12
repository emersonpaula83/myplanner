package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/auth"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"go.uber.org/zap"
)

type mockUsuarioStore struct {
	buscarPorEmailFn    func(ctx context.Context, email string) (*domain.Usuario, error)
	buscarPorIDFn       func(ctx context.Context, id uuid.UUID) (*domain.Usuario, error)
	listarTodosFn       func(ctx context.Context) ([]domain.Usuario, error)
	criarFn             func(ctx context.Context, req *domain.CriarUsuarioRequest, senhaHash string) (*domain.Usuario, error)
	atualizarFn         func(ctx context.Context, id uuid.UUID, req *domain.AtualizarUsuarioRequest) (*domain.Usuario, error)
	atualizarSenhaFn    func(ctx context.Context, id uuid.UUID, senhaHash string) error
	listarProjetosFn    func(ctx context.Context, usuarioID uuid.UUID) ([]domain.ProjetoResumo, error)
	atualizarProjetosFn func(ctx context.Context, usuarioID uuid.UUID, projetoIDs []uuid.UUID) ([]domain.ProjetoResumo, error)
	buscarProjetoIDsFn  func(ctx context.Context, usuarioID uuid.UUID) ([]uuid.UUID, error)
}

func (m *mockUsuarioStore) BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error) {
	return m.buscarPorEmailFn(ctx, email)
}
func (m *mockUsuarioStore) BuscarPorID(ctx context.Context, id uuid.UUID) (*domain.Usuario, error) {
	return m.buscarPorIDFn(ctx, id)
}
func (m *mockUsuarioStore) ListarTodos(ctx context.Context) ([]domain.Usuario, error) {
	return m.listarTodosFn(ctx)
}
func (m *mockUsuarioStore) Criar(ctx context.Context, req *domain.CriarUsuarioRequest, senhaHash string) (*domain.Usuario, error) {
	return m.criarFn(ctx, req, senhaHash)
}
func (m *mockUsuarioStore) Atualizar(ctx context.Context, id uuid.UUID, req *domain.AtualizarUsuarioRequest) (*domain.Usuario, error) {
	return m.atualizarFn(ctx, id, req)
}
func (m *mockUsuarioStore) AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error {
	return m.atualizarSenhaFn(ctx, id, senhaHash)
}
func (m *mockUsuarioStore) ListarProjetos(ctx context.Context, usuarioID uuid.UUID) ([]domain.ProjetoResumo, error) {
	return m.listarProjetosFn(ctx, usuarioID)
}
func (m *mockUsuarioStore) AtualizarProjetos(ctx context.Context, usuarioID uuid.UUID, projetoIDs []uuid.UUID) ([]domain.ProjetoResumo, error) {
	return m.atualizarProjetosFn(ctx, usuarioID, projetoIDs)
}
func (m *mockUsuarioStore) BuscarProjetoIDsPorUsuario(ctx context.Context, usuarioID uuid.UUID) ([]uuid.UUID, error) {
	return m.buscarProjetoIDsFn(ctx, usuarioID)
}

func TestLogin_Success(t *testing.T) {
	userID := uuid.New()
	// Hash of "Totvs@123" with cost 12
	senhaHash := "$2a$12$YD27E7brWZvrrq0lVpbsouDUIi3UiwgjT6NsiIOQGPzwDBlvC5DYK"

	store := &mockUsuarioStore{
		buscarPorEmailFn: func(_ context.Context, email string) (*domain.Usuario, error) {
			return &domain.Usuario{
				ID:           userID,
				NomeCompleto: "Administrador",
				Apelido:      "admin",
				Email:        email,
				SenhaHash:    senhaHash,
				Cargo:        "coordenador",
				Ativo:        true,
			}, nil
		},
	}

	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	logger := zap.NewNop()
	h := NewAuthHandler(store, ts, logger)

	body := `{"email":"admin@tcloud.local","senha":"Totvs@123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp domain.LoginResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.Usuario.ID != userID {
		t.Errorf("usuario.id = %s, want %s", resp.Usuario.ID, userID)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	senhaHash := "$2a$12$YD27E7brWZvrrq0lVpbsouDUIi3UiwgjT6NsiIOQGPzwDBlvC5DYK"

	store := &mockUsuarioStore{
		buscarPorEmailFn: func(_ context.Context, _ string) (*domain.Usuario, error) {
			return &domain.Usuario{SenhaHash: senhaHash, Ativo: true}, nil
		},
	}

	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	h := NewAuthHandler(store, ts, zap.NewNop())

	body := `{"email":"admin@tcloud.local","senha":"wrong-password"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	store := &mockUsuarioStore{
		buscarPorEmailFn: func(_ context.Context, _ string) (*domain.Usuario, error) {
			return nil, nil
		},
	}

	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	h := NewAuthHandler(store, ts, zap.NewNop())

	body := `{"email":"nobody@example.com","senha":"whatever"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestLogin_EmptyBody(t *testing.T) {
	store := &mockUsuarioStore{}
	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	h := NewAuthHandler(store, ts, zap.NewNop())

	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString("{}"))
	rr := httptest.NewRecorder()

	h.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}
