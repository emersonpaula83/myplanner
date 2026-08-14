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
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

// --- Update ---

func TestUsuarioHandler_Update_Success(t *testing.T) {
	id := uuid.New()
	store := &mockUsuarioStore{
		atualizarFn: func(_ context.Context, uid uuid.UUID, req *domain.AtualizarUsuarioRequest) (*domain.Usuario, error) {
			if uid != id {
				t.Errorf("unexpected id: %s", uid)
			}
			return &domain.Usuario{ID: uid, NomeCompleto: "Updated"}, nil
		},
	}
	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")

	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}", h.Update)

	body := `{"apelido":"novo"}`
	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String(), bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestUsuarioHandler_Update_InvalidID(t *testing.T) {
	h := NewUsuarioHandler(&mockUsuarioStore{}, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}", h.Update)

	req := httptest.NewRequest("PUT", "/api/v1/usuarios/xxx", bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUsuarioHandler_Update_InvalidBody(t *testing.T) {
	id := uuid.New()
	h := NewUsuarioHandler(&mockUsuarioStore{}, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}", h.Update)

	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String(), bytes.NewBufferString(`not-json`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUsuarioHandler_Update_InvalidApelido(t *testing.T) {
	id := uuid.New()
	h := NewUsuarioHandler(&mockUsuarioStore{}, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}", h.Update)

	apelidoVazio := ""
	body, _ := json.Marshal(domain.AtualizarUsuarioRequest{Apelido: &apelidoVazio})
	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String(), bytes.NewBuffer(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUsuarioHandler_Update_InvalidEmail(t *testing.T) {
	id := uuid.New()
	h := NewUsuarioHandler(&mockUsuarioStore{}, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}", h.Update)

	emailInvalido := "not-an-email"
	body, _ := json.Marshal(domain.AtualizarUsuarioRequest{Email: &emailInvalido})
	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String(), bytes.NewBuffer(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUsuarioHandler_Update_InvalidCargo(t *testing.T) {
	id := uuid.New()
	h := NewUsuarioHandler(&mockUsuarioStore{}, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}", h.Update)

	// "diretor" virou cargo válido na cortina de salários; o inválido agora é
	// qualquer coisa fora de coordenador/gerente/gerente_projetos/diretor.
	cargoInvalido := "presidente"
	body, _ := json.Marshal(domain.AtualizarUsuarioRequest{Cargo: &cargoInvalido})
	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String(), bytes.NewBuffer(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUsuarioHandler_Update_UniqueViolation(t *testing.T) {
	id := uuid.New()
	store := &mockUsuarioStore{
		atualizarFn: func(_ context.Context, _ uuid.UUID, _ *domain.AtualizarUsuarioRequest) (*domain.Usuario, error) {
			return nil, &pgconn.PgError{Code: "23505"}
		},
	}
	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}", h.Update)

	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String(), bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusConflict)
	}
}

func TestUsuarioHandler_Update_Error(t *testing.T) {
	id := uuid.New()
	store := &mockUsuarioStore{
		atualizarFn: func(_ context.Context, _ uuid.UUID, _ *domain.AtualizarUsuarioRequest) (*domain.Usuario, error) {
			return nil, errors.New("db error")
		},
	}
	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}", h.Update)

	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String(), bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestUsuarioHandler_Update_NotFound(t *testing.T) {
	id := uuid.New()
	store := &mockUsuarioStore{
		atualizarFn: func(_ context.Context, _ uuid.UUID, _ *domain.AtualizarUsuarioRequest) (*domain.Usuario, error) {
			return nil, nil
		},
	}
	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}", h.Update)

	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String(), bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// --- ListProjetos ---

func TestUsuarioHandler_ListProjetos_Success(t *testing.T) {
	id := uuid.New()
	projetoID := uuid.New()
	store := &mockUsuarioStore{
		listarProjetosFn: func(_ context.Context, uid uuid.UUID) ([]domain.ProjetoResumo, error) {
			if uid != id {
				t.Errorf("unexpected id: %s", uid)
			}
			return []domain.ProjetoResumo{{ID: projetoID, Chave: "BACK", Nome: "Backend"}}, nil
		},
	}
	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Get("/api/v1/usuarios/{id}/projetos", h.ListProjetos)

	req := httptest.NewRequest("GET", "/api/v1/usuarios/"+id.String()+"/projetos", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestUsuarioHandler_ListProjetos_InvalidID(t *testing.T) {
	h := NewUsuarioHandler(&mockUsuarioStore{}, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Get("/api/v1/usuarios/{id}/projetos", h.ListProjetos)

	req := httptest.NewRequest("GET", "/api/v1/usuarios/xxx/projetos", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUsuarioHandler_ListProjetos_Error(t *testing.T) {
	id := uuid.New()
	store := &mockUsuarioStore{
		listarProjetosFn: func(_ context.Context, _ uuid.UUID) ([]domain.ProjetoResumo, error) {
			return nil, errors.New("db error")
		},
	}
	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Get("/api/v1/usuarios/{id}/projetos", h.ListProjetos)

	req := httptest.NewRequest("GET", "/api/v1/usuarios/"+id.String()+"/projetos", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// --- AlterarSenha additional paths ---

func TestUsuarioHandler_AlterarSenha_InvalidID(t *testing.T) {
	h := NewUsuarioHandler(&mockUsuarioStore{}, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}/senha", h.AlterarSenha)

	req := httptest.NewRequest("PUT", "/api/v1/usuarios/xxx/senha", bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUsuarioHandler_AlterarSenha_InvalidBody(t *testing.T) {
	id := uuid.New()
	h := NewUsuarioHandler(&mockUsuarioStore{}, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}/senha", h.AlterarSenha)

	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String()+"/senha", bytes.NewBufferString(`not-json`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUsuarioHandler_AlterarSenha_ShortPassword(t *testing.T) {
	id := uuid.New()
	h := NewUsuarioHandler(&mockUsuarioStore{}, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}/senha", h.AlterarSenha)

	body := `{"senha_atual":"whatever","nova_senha":"123"}`
	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String()+"/senha", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUsuarioHandler_AlterarSenha_ErrorGet(t *testing.T) {
	id := uuid.New()
	store := &mockUsuarioStore{
		buscarPorIDFn: func(_ context.Context, _ uuid.UUID) (*domain.Usuario, error) {
			return nil, errors.New("db error")
		},
	}
	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}/senha", h.AlterarSenha)

	body := `{"senha_atual":"whatever","nova_senha":"NewPass123"}`
	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String()+"/senha", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestUsuarioHandler_AlterarSenha_NotFound(t *testing.T) {
	id := uuid.New()
	store := &mockUsuarioStore{
		buscarPorIDFn: func(_ context.Context, _ uuid.UUID) (*domain.Usuario, error) {
			return nil, nil
		},
	}
	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}/senha", h.AlterarSenha)

	body := `{"senha_atual":"whatever","nova_senha":"NewPass123"}`
	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String()+"/senha", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestUsuarioHandler_AlterarSenha_NoLocalPassword(t *testing.T) {
	id := uuid.New()
	store := &mockUsuarioStore{
		buscarPorIDFn: func(_ context.Context, _ uuid.UUID) (*domain.Usuario, error) {
			return &domain.Usuario{ID: id, SenhaHash: nil}, nil
		},
	}
	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}/senha", h.AlterarSenha)

	body := `{"senha_atual":"whatever","nova_senha":"NewPass123"}`
	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String()+"/senha", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestUsuarioHandler_AlterarSenha_Success(t *testing.T) {
	id := uuid.New()
	hashBytes, err := bcrypt.GenerateFromPassword([]byte("correctpass"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	senhaHash := string(hashBytes)

	var atualizadoID uuid.UUID
	var novaHash string
	store := &mockUsuarioStore{
		buscarPorIDFn: func(_ context.Context, _ uuid.UUID) (*domain.Usuario, error) {
			return &domain.Usuario{ID: id, SenhaHash: &senhaHash}, nil
		},
		atualizarSenhaFn: func(_ context.Context, uid uuid.UUID, hash string) error {
			atualizadoID = uid
			novaHash = hash
			return nil
		},
	}
	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}/senha", h.AlterarSenha)

	body := `{"senha_atual":"correctpass","nova_senha":"NewPass123"}`
	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String()+"/senha", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if atualizadoID != id {
		t.Errorf("atualizadoID = %s, want %s", atualizadoID, id)
	}
	if novaHash == "" {
		t.Error("expected non-empty new hash")
	}
}

func TestUsuarioHandler_AlterarSenha_ErrorUpdate(t *testing.T) {
	id := uuid.New()
	hashBytes, err := bcrypt.GenerateFromPassword([]byte("correctpass"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	senhaHash := string(hashBytes)
	store := &mockUsuarioStore{
		buscarPorIDFn: func(_ context.Context, _ uuid.UUID) (*domain.Usuario, error) {
			return &domain.Usuario{ID: id, SenhaHash: &senhaHash}, nil
		},
		atualizarSenhaFn: func(_ context.Context, _ uuid.UUID, _ string) error {
			return errors.New("db error")
		},
	}
	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")
	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}/senha", h.AlterarSenha)

	body := `{"senha_atual":"correctpass","nova_senha":"NewPass123"}`
	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+id.String()+"/senha", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// --- isUniqueViolation ---

func TestIsUniqueViolation_True(t *testing.T) {
	err := &pgconn.PgError{Code: "23505"}
	if !isUniqueViolation(err) {
		t.Error("expected true for unique_violation code")
	}
}

func TestIsUniqueViolation_FalseOtherCode(t *testing.T) {
	err := &pgconn.PgError{Code: "23503"}
	if isUniqueViolation(err) {
		t.Error("expected false for foreign_key_violation code")
	}
}

func TestIsUniqueViolation_FalseNonPgError(t *testing.T) {
	if isUniqueViolation(errors.New("generic error")) {
		t.Error("expected false for non-pg error")
	}
}

func TestIsUniqueViolation_Nil(t *testing.T) {
	if isUniqueViolation(nil) {
		t.Error("expected false for nil error")
	}
}
