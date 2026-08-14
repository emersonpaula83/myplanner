package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockInvestimentoStore struct {
	getDashboardFn         func(ctx context.Context, equipeID uuid.UUID) (*domain.InvestimentoDashboard, error)
	getGastosMensaisFn     func(ctx context.Context, equipeID uuid.UUID, ano int) (*domain.GastosMensaisResponse, error)
	getAlocacoesProjetosFn func(ctx context.Context, membroID uuid.UUID) (*domain.AlocacoesProjetosResponse, error)
}

// Os métodos toleram campo nil: cada teste preenche só o que exercita, e os
// testes da cortina de salários passam o store vazio quando o handler sob teste
// nem toca nele.
func (m *mockInvestimentoStore) GetDashboard(ctx context.Context, equipeID uuid.UUID) (*domain.InvestimentoDashboard, error) {
	if m.getDashboardFn == nil {
		return nil, nil
	}
	return m.getDashboardFn(ctx, equipeID)
}

func (m *mockInvestimentoStore) GetGastosMensais(ctx context.Context, equipeID uuid.UUID, ano int) (*domain.GastosMensaisResponse, error) {
	if m.getGastosMensaisFn == nil {
		return nil, nil
	}
	return m.getGastosMensaisFn(ctx, equipeID, ano)
}

func (m *mockInvestimentoStore) GetAlocacoesProjetos(ctx context.Context, membroID uuid.UUID) (*domain.AlocacoesProjetosResponse, error) {
	if m.getAlocacoesProjetosFn == nil {
		return nil, nil
	}
	return m.getAlocacoesProjetosFn(ctx, membroID)
}

type mockMembroFinanceiroStore struct {
	updateSalarioFn          func(ctx context.Context, id uuid.UUID, valor float64) error
	updateBancoHorasFn       func(ctx context.Context, id uuid.UUID, valor float64) error
	updateDataAdmissaoFn     func(ctx context.Context, id uuid.UUID, data *time.Time) error
	getHistoricoSalarioFn    func(ctx context.Context, membroID uuid.UUID) ([]domain.SalarioHistorico, error)
	getHistoricoBancoHorasFn func(ctx context.Context, membroID uuid.UUID) ([]domain.BancoHorasHistorico, error)
}

func (m *mockMembroFinanceiroStore) UpdateSalario(ctx context.Context, id uuid.UUID, valor float64) error {
	if m.updateSalarioFn == nil {
		return nil
	}
	return m.updateSalarioFn(ctx, id, valor)
}

func (m *mockMembroFinanceiroStore) UpdateBancoHoras(ctx context.Context, id uuid.UUID, valor float64) error {
	if m.updateBancoHorasFn == nil {
		return nil
	}
	return m.updateBancoHorasFn(ctx, id, valor)
}

func (m *mockMembroFinanceiroStore) UpdateDataAdmissao(ctx context.Context, id uuid.UUID, data *time.Time) error {
	if m.updateDataAdmissaoFn == nil {
		return nil
	}
	return m.updateDataAdmissaoFn(ctx, id, data)
}

func (m *mockMembroFinanceiroStore) GetHistoricoSalario(ctx context.Context, membroID uuid.UUID) ([]domain.SalarioHistorico, error) {
	if m.getHistoricoSalarioFn == nil {
		return nil, nil
	}
	return m.getHistoricoSalarioFn(ctx, membroID)
}

func (m *mockMembroFinanceiroStore) GetHistoricoBancoHoras(ctx context.Context, membroID uuid.UUID) ([]domain.BancoHorasHistorico, error) {
	if m.getHistoricoBancoHorasFn == nil {
		return nil, nil
	}
	return m.getHistoricoBancoHorasFn(ctx, membroID)
}

// --- GetDashboard ---

func TestInvestimento_GetDashboard_Success(t *testing.T) {
	equipeID := uuid.New()
	store := &mockInvestimentoStore{
		getDashboardFn: func(ctx context.Context, id uuid.UUID) (*domain.InvestimentoDashboard, error) {
			return &domain.InvestimentoDashboard{Equipe: domain.EquipeInfo{ID: id, Nome: "Time A"}}, nil
		},
	}
	h := NewInvestimentoHandler(store, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/equipe/"+equipeID.String(), nil)
	req = addChiParam(req, "id", equipeID.String())
	w := httptest.NewRecorder()

	h.GetDashboard(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestInvestimento_GetDashboard_NotFound(t *testing.T) {
	equipeID := uuid.New()
	store := &mockInvestimentoStore{
		getDashboardFn: func(ctx context.Context, id uuid.UUID) (*domain.InvestimentoDashboard, error) {
			return nil, errors.New("equipe not found")
		},
	}
	h := NewInvestimentoHandler(store, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/equipe/"+equipeID.String(), nil)
	req = addChiParam(req, "id", equipeID.String())
	w := httptest.NewRecorder()

	h.GetDashboard(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestInvestimento_GetDashboard_Error(t *testing.T) {
	equipeID := uuid.New()
	store := &mockInvestimentoStore{
		getDashboardFn: func(ctx context.Context, id uuid.UUID) (*domain.InvestimentoDashboard, error) {
			return nil, errors.New("db error")
		},
	}
	h := NewInvestimentoHandler(store, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/equipe/"+equipeID.String(), nil)
	req = addChiParam(req, "id", equipeID.String())
	w := httptest.NewRecorder()

	h.GetDashboard(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestInvestimento_GetDashboard_InvalidID(t *testing.T) {
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/equipe/not-a-uuid", nil)
	req = addChiParam(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	h.GetDashboard(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- GetGastosMensais ---

func TestInvestimento_GetGastosMensais_Success(t *testing.T) {
	equipeID := uuid.New()
	store := &mockInvestimentoStore{
		getGastosMensaisFn: func(ctx context.Context, id uuid.UUID, ano int) (*domain.GastosMensaisResponse, error) {
			if ano != 2025 {
				t.Errorf("expected ano 2025, got %d", ano)
			}
			return &domain.GastosMensaisResponse{Ano: ano}, nil
		},
	}
	h := NewInvestimentoHandler(store, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/equipe/"+equipeID.String()+"/gastos-mensais?ano=2025", nil)
	req = addChiParam(req, "id", equipeID.String())
	w := httptest.NewRecorder()

	h.GetGastosMensais(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestInvestimento_GetGastosMensais_Error(t *testing.T) {
	equipeID := uuid.New()
	store := &mockInvestimentoStore{
		getGastosMensaisFn: func(ctx context.Context, id uuid.UUID, ano int) (*domain.GastosMensaisResponse, error) {
			return nil, errors.New("db error")
		},
	}
	h := NewInvestimentoHandler(store, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/equipe/"+equipeID.String()+"/gastos-mensais", nil)
	req = addChiParam(req, "id", equipeID.String())
	w := httptest.NewRecorder()

	h.GetGastosMensais(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestInvestimento_GetGastosMensais_InvalidAno(t *testing.T) {
	equipeID := uuid.New()
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/equipe/"+equipeID.String()+"/gastos-mensais?ano=abc", nil)
	req = addChiParam(req, "id", equipeID.String())
	w := httptest.NewRecorder()

	h.GetGastosMensais(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestInvestimento_GetGastosMensais_InvalidID(t *testing.T) {
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/equipe/not-a-uuid/gastos-mensais", nil)
	req = addChiParam(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	h.GetGastosMensais(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- UpdateSalario ---

func TestInvestimento_UpdateSalario_Success(t *testing.T) {
	membroID := uuid.New()
	membroStore := &mockMembroFinanceiroStore{
		updateSalarioFn: func(ctx context.Context, id uuid.UUID, valor float64) error {
			if id != membroID {
				t.Errorf("expected id %s, got %s", membroID, id)
			}
			if valor != 5000 {
				t.Errorf("expected valor 5000, got %v", valor)
			}
			return nil
		},
	}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())
	body, _ := json.Marshal(map[string]any{"valor": 5000})
	req := httptest.NewRequest(http.MethodPut, "/investimento/membro/"+membroID.String()+"/salario", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.UpdateSalario(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestInvestimento_UpdateSalario_NotFound(t *testing.T) {
	membroID := uuid.New()
	membroStore := &mockMembroFinanceiroStore{
		updateSalarioFn: func(ctx context.Context, id uuid.UUID, valor float64) error {
			return errors.New("membro not found")
		},
	}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())
	body, _ := json.Marshal(map[string]any{"valor": 5000})
	req := httptest.NewRequest(http.MethodPut, "/investimento/membro/"+membroID.String()+"/salario", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.UpdateSalario(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestInvestimento_UpdateSalario_Error(t *testing.T) {
	membroID := uuid.New()
	membroStore := &mockMembroFinanceiroStore{
		updateSalarioFn: func(ctx context.Context, id uuid.UUID, valor float64) error {
			return errors.New("db error")
		},
	}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())
	body, _ := json.Marshal(map[string]any{"valor": 5000})
	req := httptest.NewRequest(http.MethodPut, "/investimento/membro/"+membroID.String()+"/salario", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.UpdateSalario(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestInvestimento_UpdateSalario_InvalidBody(t *testing.T) {
	membroID := uuid.New()
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodPut, "/investimento/membro/"+membroID.String()+"/salario", bytes.NewReader([]byte("{invalid")))
	req = addChiParam(req, "id", membroID.String())
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.UpdateSalario(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestInvestimento_UpdateSalario_NegativeValue(t *testing.T) {
	membroID := uuid.New()
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, &mockMembroFinanceiroStore{}, zap.NewNop())
	body, _ := json.Marshal(map[string]any{"valor": -100})
	req := httptest.NewRequest(http.MethodPut, "/investimento/membro/"+membroID.String()+"/salario", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.UpdateSalario(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestInvestimento_UpdateSalario_InvalidID(t *testing.T) {
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodPut, "/investimento/membro/not-a-uuid/salario", nil)
	req = addChiParam(req, "id", "not-a-uuid")
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.UpdateSalario(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- UpdateBancoHoras ---

func TestInvestimento_UpdateBancoHoras_Success(t *testing.T) {
	membroID := uuid.New()
	membroStore := &mockMembroFinanceiroStore{
		updateBancoHorasFn: func(ctx context.Context, id uuid.UUID, valor float64) error {
			if id != membroID {
				t.Errorf("expected id %s, got %s", membroID, id)
			}
			return nil
		},
	}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())
	body, _ := json.Marshal(map[string]any{"valor": 10})
	req := httptest.NewRequest(http.MethodPut, "/investimento/membro/"+membroID.String()+"/banco-horas", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.UpdateBancoHoras(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestInvestimento_UpdateBancoHoras_Error(t *testing.T) {
	membroID := uuid.New()
	membroStore := &mockMembroFinanceiroStore{
		updateBancoHorasFn: func(ctx context.Context, id uuid.UUID, valor float64) error {
			return errors.New("db error")
		},
	}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())
	body, _ := json.Marshal(map[string]any{"valor": 10})
	req := httptest.NewRequest(http.MethodPut, "/investimento/membro/"+membroID.String()+"/banco-horas", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.UpdateBancoHoras(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestInvestimento_UpdateBancoHoras_NotFound(t *testing.T) {
	membroID := uuid.New()
	membroStore := &mockMembroFinanceiroStore{
		updateBancoHorasFn: func(ctx context.Context, id uuid.UUID, valor float64) error {
			return errors.New("membro not found")
		},
	}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())
	body, _ := json.Marshal(map[string]any{"valor": 10})
	req := httptest.NewRequest(http.MethodPut, "/investimento/membro/"+membroID.String()+"/banco-horas", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.UpdateBancoHoras(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestInvestimento_UpdateBancoHoras_InvalidBody(t *testing.T) {
	membroID := uuid.New()
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodPut, "/investimento/membro/"+membroID.String()+"/banco-horas", bytes.NewReader([]byte("{invalid")))
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.UpdateBancoHoras(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- UpdateDataAdmissao ---

func TestInvestimento_UpdateDataAdmissao_Success(t *testing.T) {
	membroID := uuid.New()
	membroStore := &mockMembroFinanceiroStore{
		updateDataAdmissaoFn: func(ctx context.Context, id uuid.UUID, data *time.Time) error {
			if id != membroID {
				t.Errorf("expected id %s, got %s", membroID, id)
			}
			if data == nil {
				t.Errorf("expected non-nil data")
			}
			return nil
		},
	}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())
	data := "2024-05-01"
	body, _ := json.Marshal(map[string]any{"data_admissao": &data})
	req := httptest.NewRequest(http.MethodPut, "/investimento/membro/"+membroID.String()+"/data-admissao", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.UpdateDataAdmissao(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestInvestimento_UpdateDataAdmissao_Error(t *testing.T) {
	membroID := uuid.New()
	membroStore := &mockMembroFinanceiroStore{
		updateDataAdmissaoFn: func(ctx context.Context, id uuid.UUID, data *time.Time) error {
			return errors.New("db error")
		},
	}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())
	body, _ := json.Marshal(map[string]any{"data_admissao": nil})
	req := httptest.NewRequest(http.MethodPut, "/investimento/membro/"+membroID.String()+"/data-admissao", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.UpdateDataAdmissao(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestInvestimento_UpdateDataAdmissao_InvalidDate(t *testing.T) {
	membroID := uuid.New()
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, &mockMembroFinanceiroStore{}, zap.NewNop())
	data := "not-a-date"
	body, _ := json.Marshal(map[string]any{"data_admissao": &data})
	req := httptest.NewRequest(http.MethodPut, "/investimento/membro/"+membroID.String()+"/data-admissao", bytes.NewReader(body))
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.UpdateDataAdmissao(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- GetHistoricoSalario ---

func TestInvestimento_GetHistoricoSalario_Success(t *testing.T) {
	membroID := uuid.New()
	// Valor virou *float64 com omitempty na cortina de salários: travado, a
	// chave some do JSON em vez de virar zero.
	valor := 5000.0
	membroStore := &mockMembroFinanceiroStore{
		getHistoricoSalarioFn: func(ctx context.Context, id uuid.UUID) ([]domain.SalarioHistorico, error) {
			return []domain.SalarioHistorico{{ID: uuid.New(), MembroID: id, Valor: &valor}}, nil
		},
	}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/membro/"+membroID.String()+"/historico-salario", nil)
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.GetHistoricoSalario(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestInvestimento_GetHistoricoSalario_Error(t *testing.T) {
	membroID := uuid.New()
	membroStore := &mockMembroFinanceiroStore{
		getHistoricoSalarioFn: func(ctx context.Context, id uuid.UUID) ([]domain.SalarioHistorico, error) {
			return nil, errors.New("db error")
		},
	}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/membro/"+membroID.String()+"/historico-salario", nil)
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.GetHistoricoSalario(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestInvestimento_GetHistoricoSalario_InvalidID(t *testing.T) {
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/membro/not-a-uuid/historico-salario", nil)
	req = addChiParam(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	h.GetHistoricoSalario(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- GetHistoricoBancoHoras ---

func TestInvestimento_GetHistoricoBancoHoras_Success(t *testing.T) {
	membroID := uuid.New()
	membroStore := &mockMembroFinanceiroStore{
		getHistoricoBancoHorasFn: func(ctx context.Context, id uuid.UUID) ([]domain.BancoHorasHistorico, error) {
			return []domain.BancoHorasHistorico{{ID: uuid.New(), MembroID: id, Valor: 10}}, nil
		},
	}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/membro/"+membroID.String()+"/historico-banco-horas", nil)
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.GetHistoricoBancoHoras(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestInvestimento_GetHistoricoBancoHoras_Error(t *testing.T) {
	membroID := uuid.New()
	membroStore := &mockMembroFinanceiroStore{
		getHistoricoBancoHorasFn: func(ctx context.Context, id uuid.UUID) ([]domain.BancoHorasHistorico, error) {
			return nil, errors.New("db error")
		},
	}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/membro/"+membroID.String()+"/historico-banco-horas", nil)
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.GetHistoricoBancoHoras(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestInvestimento_GetHistoricoBancoHoras_InvalidID(t *testing.T) {
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/membro/not-a-uuid/historico-banco-horas", nil)
	req = addChiParam(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	h.GetHistoricoBancoHoras(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- GetAlocacoesProjetos ---

func TestInvestimento_GetAlocacoesProjetos_Success(t *testing.T) {
	membroID := uuid.New()
	store := &mockInvestimentoStore{
		getAlocacoesProjetosFn: func(ctx context.Context, id uuid.UUID) (*domain.AlocacoesProjetosResponse, error) {
			return &domain.AlocacoesProjetosResponse{Projetos: []domain.ProjetoAlocacao{{Apelido: "Proj A"}}}, nil
		},
	}
	h := NewInvestimentoHandler(store, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/membro/"+membroID.String()+"/alocacoes", nil)
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.GetAlocacoesProjetos(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestInvestimento_GetAlocacoesProjetos_Error(t *testing.T) {
	membroID := uuid.New()
	store := &mockInvestimentoStore{
		getAlocacoesProjetosFn: func(ctx context.Context, id uuid.UUID) (*domain.AlocacoesProjetosResponse, error) {
			return nil, errors.New("db error")
		},
	}
	h := NewInvestimentoHandler(store, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/membro/"+membroID.String()+"/alocacoes", nil)
	req = addChiParam(req, "id", membroID.String())
	w := httptest.NewRecorder()

	h.GetAlocacoesProjetos(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestInvestimento_GetAlocacoesProjetos_InvalidID(t *testing.T) {
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, &mockMembroFinanceiroStore{}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/investimento/membro/not-a-uuid/alocacoes", nil)
	req = addChiParam(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	h.GetAlocacoesProjetos(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
