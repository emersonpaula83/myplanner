package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/auth"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

func TestUsuarioHandler_List(t *testing.T) {
	store := &mockUsuarioStore{
		listarTodosFn: func(_ context.Context) ([]domain.Usuario, error) {
			return []domain.Usuario{
				{ID: uuid.New(), NomeCompleto: "Admin", Apelido: "admin", Email: "admin@myplanner.local", Cargo: "coordenador", Ativo: true},
			}, nil
		},
	}

	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")
	req := httptest.NewRequest("GET", "/api/v1/usuarios", nil)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp map[string][]domain.Usuario
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp["usuarios"]) != 1 {
		t.Errorf("got %d usuarios, want 1", len(resp["usuarios"]))
	}
}

func TestUsuarioHandler_Create(t *testing.T) {
	createdID := uuid.New()
	store := &mockUsuarioStore{
		criarFn: func(_ context.Context, req *domain.CriarUsuarioRequest, senhaHash string) (*domain.Usuario, error) {
			return &domain.Usuario{
				ID:           createdID,
				NomeCompleto: req.NomeCompleto,
				Apelido:      req.Apelido,
				Email:        req.Email,
				Cargo:        req.Cargo,
				Ativo:        true,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}, nil
		},
	}

	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")
	body := `{"nome_completo":"João Silva","apelido":"joao","email":"joao@totvs.com","senha":"MinhaS3nh@","cargo":"gerente"}`
	req := httptest.NewRequest("POST", "/api/v1/usuarios", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}
}

func TestUsuarioHandler_Create_InvalidCargo(t *testing.T) {
	store := &mockUsuarioStore{}
	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")

	body := `{"nome_completo":"João","apelido":"joao","email":"joao@totvs.com","senha":"MinhaS3nh@","cargo":"presidente"}`
	req := httptest.NewRequest("POST", "/api/v1/usuarios", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUsuarioHandler_Create_ShortPassword(t *testing.T) {
	store := &mockUsuarioStore{}
	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")

	body := `{"nome_completo":"João","apelido":"joao","email":"joao@totvs.com","senha":"123","cargo":"gerente"}`
	req := httptest.NewRequest("POST", "/api/v1/usuarios", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUsuarioHandler_GetByID(t *testing.T) {
	userID := uuid.New()
	store := &mockUsuarioStore{
		buscarPorIDFn: func(_ context.Context, id uuid.UUID) (*domain.Usuario, error) {
			return &domain.Usuario{
				ID:           id,
				NomeCompleto: "Admin",
				Apelido:      "admin",
				Email:        "admin@myplanner.local",
				Cargo:        "coordenador",
				Ativo:        true,
			}, nil
		},
	}

	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")

	r := chi.NewRouter()
	r.Get("/api/v1/usuarios/{id}", h.GetByID)

	req := httptest.NewRequest("GET", "/api/v1/usuarios/"+userID.String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestUsuarioHandler_AlterarSenha_WrongCurrent(t *testing.T) {
	senhaHash := "$2a$12$YD27E7brWZvrrq0lVpbsouDUIi3UiwgjT6NsiIOQGPzwDBlvC5DYK"
	store := &mockUsuarioStore{
		buscarPorIDFn: func(_ context.Context, _ uuid.UUID) (*domain.Usuario, error) {
			return &domain.Usuario{
				ID:        uuid.New(),
				SenhaHash: &senhaHash,
			}, nil
		},
	}

	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")

	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}/senha", h.AlterarSenha)

	body := `{"senha_atual":"wrong","nova_senha":"NewPass123"}`
	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+uuid.New().String()+"/senha", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestUsuarioHandler_UpdateProjetos(t *testing.T) {
	projetoID := uuid.New()
	store := &mockUsuarioStore{
		atualizarProjetosFn: func(_ context.Context, _ uuid.UUID, ids []uuid.UUID) ([]domain.ProjetoResumo, error) {
			return []domain.ProjetoResumo{
				{ID: ids[0], Chave: "BACK", Nome: "Backend"},
			}, nil
		},
	}

	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")

	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}/projetos", h.UpdateProjetos)

	body, _ := json.Marshal(domain.AlcadaProjetosRequest{ProjetoIDs: []uuid.UUID{projetoID}})
	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+uuid.New().String()+"/projetos", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestUsuarioHandler_ListEquipes(t *testing.T) {
	equipeID := uuid.New()
	store := &mockUsuarioStore{
		listarEquipesFn: func(_ context.Context, _ uuid.UUID) ([]repository.EquipeResumo, error) {
			return []repository.EquipeResumo{
				{ID: equipeID, Nome: "Squad Alpha"},
			}, nil
		},
	}

	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")

	r := chi.NewRouter()
	r.Get("/api/v1/usuarios/{id}/equipes", h.ListEquipes)

	req := httptest.NewRequest("GET", "/api/v1/usuarios/"+uuid.New().String()+"/equipes", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func newAuthedRequest(t *testing.T, method, target, email string, body []byte) (*httptest.ResponseRecorder, *http.Request, *auth.TokenService) {
	t.Helper()
	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	token, err := ts.GenerateToken(uuid.New(), email, "coordenador")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, target, bytes.NewBuffer(body))
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return httptest.NewRecorder(), req, ts
}

func TestUsuarioHandler_UpdateEquipes_Success(t *testing.T) {
	equipeID := uuid.New()
	var atualizadoIDs []uuid.UUID
	store := &mockUsuarioStore{
		atualizarEquipesFn: func(_ context.Context, _ uuid.UUID, ids []uuid.UUID) error {
			atualizadoIDs = ids
			return nil
		},
		listarEquipesFn: func(_ context.Context, _ uuid.UUID) ([]repository.EquipeResumo, error) {
			return []repository.EquipeResumo{{ID: equipeID, Nome: "Squad Alpha"}}, nil
		},
	}

	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")

	body, _ := json.Marshal(domain.AlcadaEquipesRequest{EquipeIDs: []uuid.UUID{equipeID}})
	rr, req, ts := newAuthedRequest(t, "PUT", "/api/v1/usuarios/"+uuid.New().String()+"/equipes", "admin@myplanner.local", body)

	r := chi.NewRouter()
	r.With(middleware.AuthJWT(ts)).Put("/api/v1/usuarios/{id}/equipes", h.UpdateEquipes)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if len(atualizadoIDs) != 1 || atualizadoIDs[0] != equipeID {
		t.Errorf("atualizadoIDs = %v, want [%s]", atualizadoIDs, equipeID)
	}
}

func TestUsuarioHandler_UpdateEquipes_Forbidden(t *testing.T) {
	store := &mockUsuarioStore{}
	h := NewUsuarioHandler(store, zap.NewNop(), "admin@myplanner.local")

	body, _ := json.Marshal(domain.AlcadaEquipesRequest{EquipeIDs: []uuid.UUID{uuid.New()}})
	rr, req, ts := newAuthedRequest(t, "PUT", "/api/v1/usuarios/"+uuid.New().String()+"/equipes", "naoadmin@myplanner.local", body)

	r := chi.NewRouter()
	r.With(middleware.AuthJWT(ts)).Put("/api/v1/usuarios/{id}/equipes", h.UpdateEquipes)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}
