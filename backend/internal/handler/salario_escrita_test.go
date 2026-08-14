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

func requestEscrita(metodo, alvo, corpo, paramValor string) *http.Request {
	req := httptest.NewRequest(metodo, alvo, strings.NewReader(corpo))
	ctx := mw.ContextParaTeste(req.Context(), uuid.New(), "chefe@myplanner.local", "gerente", time.Now().Add(time.Hour))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", paramValor)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	return req.WithContext(ctx)
}

// Travar só a leitura seria teatro: quem abre o F12 monta o PUT na mão.
func TestUpdateSalarioTravadoDa403(t *testing.T) {
	chamou := false
	membroStore := &mockMembroFinanceiroStore{updateSalarioFn: func(context.Context, uuid.UUID, float64) error {
		chamou = true
		return nil
	}}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())

	w := httptest.NewRecorder()
	h.UpdateSalario(w, requestEscrita(http.MethodPut, "/membros/x/salario", `{"salario":9000}`, uuid.NewString()))

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, esperava 403", w.Code)
	}
	if chamou {
		t.Error("store foi chamado com a cortina fechada")
	}
}

func TestImportConfirmarTravadoDa403(t *testing.T) {
	chamou := false
	store := &mockImportStore{confirmImportFn: func(context.Context, domain.ConfirmImportRequest) (*domain.ConfirmImportResponse, error) {
		chamou = true
		return &domain.ConfirmImportResponse{}, nil
	}}
	h := newTestImportHandler(store)

	w := httptest.NewRecorder()
	h.Confirmar(w, requestEscrita(http.MethodPost, "/investimentos/import/confirmar", `{"linhas":[{"linha":2}]}`, ""))

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, esperava 403", w.Code)
	}
	if chamou {
		t.Error("import confirmou com a cortina fechada")
	}
}

// MeritoPromocao não está nos exemplos do brief, mas grava salário igual a
// UpdateSalario e Confirmar — a mesma regra "escrita sem desbloqueio -> 403"
// se aplica, e o store não pode ser tocado a caminho da resposta.
func TestMeritoPromocaoTravadoDa403(t *testing.T) {
	chamou := false
	store := &mockEquipeStore{
		getMembroByIDFn: func(context.Context, uuid.UUID) (*domain.Membro, error) {
			t.Fatal("não deveria chegar a buscar o membro com a cortina fechada")
			return nil, nil
		},
		insertMeritoPromocaoFn: func(context.Context, uuid.UUID, string, *string, *string, *float64, float64, time.Time) (*domain.HistoricoMeritoPromocao, error) {
			chamou = true
			return &domain.HistoricoMeritoPromocao{}, nil
		},
	}
	h := newTestEquipeHandler(store)

	corpo := `{"tipo":"merito","data_vigencia":"2026-01-01","salario_novo":9000}`
	w := httptest.NewRecorder()
	h.MeritoPromocao(w, requestEscrita(http.MethodPost, "/membros/x/merito-promocao", corpo, uuid.NewString()))

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, esperava 403. Corpo: %s", w.Code, w.Body.String())
	}
	if chamou {
		t.Error("store foi chamado com a cortina fechada")
	}
}
