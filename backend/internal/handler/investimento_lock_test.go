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

// Os mocks mockInvestimentoStore e mockMembroFinanceiroStore vivem em
// investimento_test.go — mesmo pacote, uma definição só.

// comCortinaAberta marca a requisição como podendo ver salários. As rotas que
// escrevem salário recusam com 403 sem isso, então os testes que exercitam a
// regra de negócio dessas rotas precisam passar por aqui.
func comCortinaAberta(r *http.Request) *http.Request {
	return r.WithContext(mw.ContextDestravadoParaTeste(r.Context()))
}

func requestComContexto(metodo, alvo, corpo, paramNome, paramValor string, destravado bool) *http.Request {
	req := httptest.NewRequest(metodo, alvo, strings.NewReader(corpo))
	ctx := mw.ContextParaTeste(req.Context(), uuid.New(), "chefe@myplanner.local", "gerente", time.Now().Add(time.Hour))
	if destravado {
		ctx = mw.ContextDestravadoParaTeste(ctx)
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(paramNome, paramValor)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	return req.WithContext(ctx)
}

func dashboardDeTeste() *domain.InvestimentoDashboard {
	custo := 12345.67
	salario := 5000.00
	return &domain.InvestimentoDashboard{
		Equipe:  domain.EquipeInfo{ID: uuid.New(), Nome: "Time"},
		Sumario: domain.InvestimentoSumario{CustoMensalTotal: &custo, TotalMembros: 3},
		Membros: []domain.MembroInvestimento{{ID: uuid.New(), Nome: "Fulano", Salario: &salario}},
	}
}

// O requisito do F12: travado, o número não pode existir no corpo da resposta.
func TestDashboardTravadoNaoMandaValores(t *testing.T) {
	store := &mockInvestimentoStore{getDashboardFn: func(context.Context, uuid.UUID) (*domain.InvestimentoDashboard, error) {
		return dashboardDeTeste(), nil
	}}
	h := NewInvestimentoHandler(store, &mockMembroFinanceiroStore{}, zap.NewNop())

	w := httptest.NewRecorder()
	h.GetDashboard(w, requestComContexto(http.MethodGet, "/equipes/x/investimentos", "", "id", uuid.NewString(), false))

	corpo := w.Body.String()
	for _, proibido := range []string{"12345.67", "5000", "custo_mensal_total", "\"salario\""} {
		if strings.Contains(corpo, proibido) {
			t.Errorf("corpo travado contém %q: %s", proibido, corpo)
		}
	}
	if !strings.Contains(corpo, "total_membros") {
		t.Errorf("travar não pode esvaziar o resto do dashboard: %s", corpo)
	}
}

func TestDashboardDestravadoMandaValores(t *testing.T) {
	store := &mockInvestimentoStore{getDashboardFn: func(context.Context, uuid.UUID) (*domain.InvestimentoDashboard, error) {
		return dashboardDeTeste(), nil
	}}
	h := NewInvestimentoHandler(store, &mockMembroFinanceiroStore{}, zap.NewNop())

	w := httptest.NewRecorder()
	h.GetDashboard(w, requestComContexto(http.MethodGet, "/equipes/x/investimentos", "", "id", uuid.NewString(), true))

	if !strings.Contains(w.Body.String(), "12345.67") {
		t.Errorf("destravado deveria mandar o custo: %s", w.Body.String())
	}
}

func TestGastosMensaisTravadoNaoMandaValores(t *testing.T) {
	custo := 9876.54
	store := &mockInvestimentoStore{getGastosMensaisFn: func(context.Context, uuid.UUID, int) (*domain.GastosMensaisResponse, error) {
		return &domain.GastosMensaisResponse{Ano: 2026, Meses: []domain.GastoMensal{{Mes: 1, CustoTotal: &custo}}}, nil
	}}
	h := NewInvestimentoHandler(store, &mockMembroFinanceiroStore{}, zap.NewNop())

	w := httptest.NewRecorder()
	h.GetGastosMensais(w, requestComContexto(http.MethodGet, "/equipes/x/investimentos/gastos-mensais", "", "id", uuid.NewString(), false))

	if strings.Contains(w.Body.String(), "9876.54") || strings.Contains(w.Body.String(), "custo_total") {
		t.Errorf("corpo travado contém o gasto: %s", w.Body.String())
	}
}

func TestHistoricoSalarioTravadoVemVazio(t *testing.T) {
	valor := 4321.00
	membroStore := &mockMembroFinanceiroStore{getHistoricoSalarioFn: func(context.Context, uuid.UUID) ([]domain.SalarioHistorico, error) {
		return []domain.SalarioHistorico{{ID: uuid.New(), Valor: &valor}}, nil
	}}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())

	w := httptest.NewRecorder()
	h.GetHistoricoSalario(w, requestComContexto(http.MethodGet, "/membros/x/salario/historico", "", "id", uuid.NewString(), false))

	if strings.Contains(w.Body.String(), "4321") {
		t.Errorf("histórico travado vazou valor: %s", w.Body.String())
	}
}
