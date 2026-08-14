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

// São dois stores: InvestimentoStore (dashboard, gastos, alocações) e
// MembroFinanceiroStore (escrita e históricos). NewInvestimentoHandler recebe
// os dois, nesta ordem, mais o logger.
type mockInvestimentoStore struct {
	getDashboardFn     func(ctx context.Context, equipeID uuid.UUID) (*domain.InvestimentoDashboard, error)
	getGastosMensaisFn func(ctx context.Context, equipeID uuid.UUID, ano int) (*domain.GastosMensaisResponse, error)
}

func (m *mockInvestimentoStore) GetDashboard(ctx context.Context, equipeID uuid.UUID) (*domain.InvestimentoDashboard, error) {
	return m.getDashboardFn(ctx, equipeID)
}
func (m *mockInvestimentoStore) GetGastosMensais(ctx context.Context, equipeID uuid.UUID, ano int) (*domain.GastosMensaisResponse, error) {
	return m.getGastosMensaisFn(ctx, equipeID, ano)
}
func (m *mockInvestimentoStore) GetAlocacoesProjetos(context.Context, uuid.UUID) (*domain.AlocacoesProjetosResponse, error) {
	return nil, nil
}

type mockMembroFinanceiroStore struct {
	updateSalarioFn       func(ctx context.Context, id uuid.UUID, valor float64) error
	getHistoricoSalarioFn func(ctx context.Context, membroID uuid.UUID) ([]domain.SalarioHistorico, error)
}

func (m *mockMembroFinanceiroStore) UpdateSalario(ctx context.Context, id uuid.UUID, valor float64) error {
	return m.updateSalarioFn(ctx, id, valor)
}
func (m *mockMembroFinanceiroStore) UpdateBancoHoras(context.Context, uuid.UUID, float64) error {
	return nil
}
func (m *mockMembroFinanceiroStore) UpdateDataAdmissao(context.Context, uuid.UUID, *time.Time) error {
	return nil
}
func (m *mockMembroFinanceiroStore) GetHistoricoSalario(ctx context.Context, membroID uuid.UUID) ([]domain.SalarioHistorico, error) {
	return m.getHistoricoSalarioFn(ctx, membroID)
}
func (m *mockMembroFinanceiroStore) GetHistoricoBancoHoras(context.Context, uuid.UUID) ([]domain.BancoHorasHistorico, error) {
	return nil, nil
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
