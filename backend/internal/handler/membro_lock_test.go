package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	mw "github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func membroComSalario() domain.Membro {
	salario := 7777.77
	return domain.Membro{ID: uuid.New(), Nome: "Fulano", Salario: &salario}
}

func requestSimples(alvo string, destravado bool) *http.Request {
	req := httptest.NewRequest(http.MethodGet, alvo, nil)
	ctx := mw.ContextParaTeste(req.Context(), uuid.New(), "chefe@myplanner.local", "gerente", time.Now().Add(time.Hour))
	if destravado {
		ctx = mw.ContextDestravadoParaTeste(ctx)
	}
	return req.WithContext(ctx)
}

func TestListMembrosTravadoNaoMandaSalario(t *testing.T) {
	store := &mockMembroStore{listFn: func(context.Context) ([]domain.Membro, error) {
		return []domain.Membro{membroComSalario()}, nil
	}}
	h := NewMembroHandler(store, zap.NewNop())

	w := httptest.NewRecorder()
	h.List(w, requestSimples("/membros", false))

	if strings.Contains(w.Body.String(), "7777.77") || strings.Contains(w.Body.String(), "\"salario\"") {
		t.Errorf("lista travada vazou salário: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Fulano") {
		t.Errorf("travar não pode esvaziar a lista: %s", w.Body.String())
	}
}

func TestListMembrosDestravadoMandaSalario(t *testing.T) {
	store := &mockMembroStore{listFn: func(context.Context) ([]domain.Membro, error) {
		return []domain.Membro{membroComSalario()}, nil
	}}
	h := NewMembroHandler(store, zap.NewNop())

	w := httptest.NewRecorder()
	h.List(w, requestSimples("/membros", true))

	if !strings.Contains(w.Body.String(), "7777.77") {
		t.Errorf("destravado deveria mandar o salário: %s", w.Body.String())
	}
}

func TestSearchMembrosTravadoNaoMandaSalario(t *testing.T) {
	store := &mockMembroStore{searchFn: func(context.Context, string, bool) ([]domain.Membro, error) {
		return []domain.Membro{membroComSalario()}, nil
	}}
	h := NewMembroHandler(store, zap.NewNop())

	w := httptest.NewRecorder()
	h.Search(w, requestSimples("/membros/search?q=ful", false))

	if strings.Contains(w.Body.String(), "7777.77") {
		t.Errorf("busca travada vazou salário: %s", w.Body.String())
	}
}

// GetByID não está nos exemplos do brief, mas usa o mesmo helper limparSalarios
// e carrega o mesmo risco: o mapa "membro" pode vazar o valor.
func TestGetByIDTravadoNaoMandaSalario(t *testing.T) {
	membro := membroComSalario()
	store := &mockMembroStore{getByIDFn: func(context.Context, uuid.UUID) (*domain.Membro, error) {
		return &membro, nil
	}}
	h := NewMembroHandler(store, zap.NewNop())

	req := requestSimples("/membros/"+membro.ID.String(), false)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", membro.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetByID(w, req)

	if strings.Contains(w.Body.String(), "7777.77") {
		t.Errorf("GetByID travado vazou salário: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Fulano") {
		t.Errorf("travar não pode esconder o resto do membro: %s", w.Body.String())
	}
}

func TestGetByIDDestravadoMandaSalario(t *testing.T) {
	membro := membroComSalario()
	store := &mockMembroStore{getByIDFn: func(context.Context, uuid.UUID) (*domain.Membro, error) {
		return &membro, nil
	}}
	h := NewMembroHandler(store, zap.NewNop())

	req := requestSimples("/membros/"+membro.ID.String(), true)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", membro.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.GetByID(w, req)

	if !strings.Contains(w.Body.String(), "7777.77") {
		t.Errorf("destravado deveria mandar o salário: %s", w.Body.String())
	}
}
