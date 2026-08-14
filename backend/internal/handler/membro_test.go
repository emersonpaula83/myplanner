package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

type mockMembroStore struct {
	searchFn      func(ctx context.Context, query string, incluirInativos bool) ([]domain.Membro, error)
	updateAtivoFn func(ctx context.Context, id uuid.UUID, ativo bool) error
}

func (m *mockMembroStore) List(context.Context) ([]domain.Membro, error) { return nil, nil }
func (m *mockMembroStore) GetByID(context.Context, uuid.UUID) (*domain.Membro, error) {
	return nil, nil
}
func (m *mockMembroStore) Search(ctx context.Context, query string, incluirInativos bool) ([]domain.Membro, error) {
	return m.searchFn(ctx, query, incluirInativos)
}
func (m *mockMembroStore) ListDisponibilidade(context.Context, uuid.UUID) ([]domain.Disponibilidade, error) {
	return nil, nil
}
func (m *mockMembroStore) CreateDisponibilidade(context.Context, *domain.Disponibilidade) error {
	return nil
}
func (m *mockMembroStore) UpdateDisponibilidade(context.Context, uuid.UUID, string, pgtype.Date, pgtype.Date, *string) error {
	return nil
}
func (m *mockMembroStore) DeleteDisponibilidade(context.Context, uuid.UUID) error { return nil }
func (m *mockMembroStore) GetMembroStats(context.Context, uuid.UUID, time.Time, time.Time) (*domain.MembroStats, error) {
	return nil, nil
}
func (m *mockMembroStore) UpdateDataDesligamento(context.Context, uuid.UUID, *time.Time) error {
	return nil
}
func (m *mockMembroStore) UpdateAtivo(ctx context.Context, id uuid.UUID, ativo bool) error {
	return m.updateAtivoFn(ctx, id, ativo)
}

func membroRequestComID(metodo, alvo, corpo, id string) *http.Request {
	req := httptest.NewRequest(metodo, alvo, strings.NewReader(corpo))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestSetAtivoDesativaMembro(t *testing.T) {
	membroID := uuid.New()
	var recebidoID uuid.UUID
	recebidoAtivo := true
	store := &mockMembroStore{updateAtivoFn: func(_ context.Context, id uuid.UUID, ativo bool) error {
		recebidoID, recebidoAtivo = id, ativo
		return nil
	}}
	h := NewMembroHandler(store, zap.NewNop())

	w := httptest.NewRecorder()
	h.SetAtivo(w, membroRequestComID(http.MethodPut, "/membros/x/ativo", `{"ativo":false}`, membroID.String()))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, esperava 200. Corpo: %s", w.Code, w.Body.String())
	}
	if recebidoID != membroID {
		t.Errorf("id repassado = %s, esperava %s", recebidoID, membroID)
	}
	if recebidoAtivo {
		t.Error("esperava ativo=false repassado ao store")
	}
}

func TestSetAtivoReativaMembro(t *testing.T) {
	recebidoAtivo := false
	store := &mockMembroStore{updateAtivoFn: func(_ context.Context, _ uuid.UUID, ativo bool) error {
		recebidoAtivo = ativo
		return nil
	}}
	h := NewMembroHandler(store, zap.NewNop())

	w := httptest.NewRecorder()
	h.SetAtivo(w, membroRequestComID(http.MethodPut, "/membros/x/ativo", `{"ativo":true}`, uuid.NewString()))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, esperava 200", w.Code)
	}
	if !recebidoAtivo {
		t.Error("esperava ativo=true repassado ao store")
	}
}

func TestSetAtivoSemCampoAtivoDa400(t *testing.T) {
	store := &mockMembroStore{updateAtivoFn: func(context.Context, uuid.UUID, bool) error {
		t.Fatal("não deveria chamar o store")
		return nil
	}}
	h := NewMembroHandler(store, zap.NewNop())

	w := httptest.NewRecorder()
	h.SetAtivo(w, membroRequestComID(http.MethodPut, "/membros/x/ativo", `{}`, uuid.NewString()))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, esperava 400", w.Code)
	}
}

// A busca só traz inativos quando quem chama pede: é o caminho de volta para
// reativar alguém desativado por engano.
func TestSearchNaoIncluiInativosPorPadrao(t *testing.T) {
	var recebido bool
	store := &mockMembroStore{searchFn: func(_ context.Context, _ string, incluirInativos bool) ([]domain.Membro, error) {
		recebido = incluirInativos
		return []domain.Membro{}, nil
	}}
	h := NewMembroHandler(store, zap.NewNop())

	w := httptest.NewRecorder()
	h.Search(w, httptest.NewRequest(http.MethodGet, "/membros/search?q=paulo", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, esperava 200", w.Code)
	}
	if recebido {
		t.Error("busca padrão não deveria incluir inativos")
	}
}

func TestSearchIncluiInativosQuandoPedido(t *testing.T) {
	var recebido bool
	store := &mockMembroStore{searchFn: func(_ context.Context, _ string, incluirInativos bool) ([]domain.Membro, error) {
		recebido = incluirInativos
		return []domain.Membro{}, nil
	}}
	h := NewMembroHandler(store, zap.NewNop())

	w := httptest.NewRecorder()
	h.Search(w, httptest.NewRequest(http.MethodGet, "/membros/search?q=paulo&inativos=true", nil))

	if !recebido {
		t.Error("esperava incluirInativos=true com ?inativos=true")
	}
}
