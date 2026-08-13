package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockEquipeStore struct {
	listEquipesFn                func(ctx context.Context) ([]domain.Equipe, error)
	getEquipeByIDFn              func(ctx context.Context, id uuid.UUID) (*domain.Equipe, error)
	createEquipeFn               func(ctx context.Context, nome string) (*domain.Equipe, error)
	updateEquipeFn               func(ctx context.Context, id uuid.UUID, nome string, boardID *int) error
	deleteEquipeFn               func(ctx context.Context, id uuid.UUID) error
	getMembrosEquipeFn           func(ctx context.Context, equipeID uuid.UUID) ([]domain.Membro, error)
	addMembroEquipeFn            func(ctx context.Context, equipeID, membroID uuid.UUID) error
	removeMembroEquipeFn         func(ctx context.Context, equipeID, membroID uuid.UUID) error
	getDiasAusenciaFn            func(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) (map[uuid.UUID]int, error)
	getHorasTarefasEquipeFn      func(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]domain.HorasTarefasMembro, error)
	updateMembroCargoFn          func(ctx context.Context, membroID uuid.UUID, cargo *string) error
	listProdutosFn               func(ctx context.Context) ([]domain.Produto, error)
	getMembroProdutosFn          func(ctx context.Context, membroID uuid.UUID) ([]domain.Produto, error)
	setMembroProdutosFn          func(ctx context.Context, membroID uuid.UUID, produtoIDs []uuid.UUID) error
	getEquipeAtivaMembroFn       func(ctx context.Context, membroID uuid.UUID) (*domain.Equipe, error)
	transferirMembroFn           func(ctx context.Context, equipeOrigemID, equipeDestinoID, membroID uuid.UUID) error
	insertMeritoPromocaoFn       func(ctx context.Context, membroID uuid.UUID, tipo string, cargoAnterior, cargoNovo *string, salarioAnterior *float64, salarioNovo float64, dataVigencia time.Time) (*domain.HistoricoMeritoPromocao, error)
	getMembrosEquipeComEntradaFn func(ctx context.Context, equipeID uuid.UUID) ([]domain.MembroComEntrada, error)
}

func (m *mockEquipeStore) ListEquipes(ctx context.Context) ([]domain.Equipe, error) {
	return m.listEquipesFn(ctx)
}
func (m *mockEquipeStore) GetEquipeByID(ctx context.Context, id uuid.UUID) (*domain.Equipe, error) {
	return m.getEquipeByIDFn(ctx, id)
}
func (m *mockEquipeStore) CreateEquipe(ctx context.Context, nome string) (*domain.Equipe, error) {
	return m.createEquipeFn(ctx, nome)
}
func (m *mockEquipeStore) UpdateEquipe(ctx context.Context, id uuid.UUID, nome string, boardID *int) error {
	return m.updateEquipeFn(ctx, id, nome, boardID)
}
func (m *mockEquipeStore) DeleteEquipe(ctx context.Context, id uuid.UUID) error {
	return m.deleteEquipeFn(ctx, id)
}
func (m *mockEquipeStore) GetMembrosEquipe(ctx context.Context, equipeID uuid.UUID) ([]domain.Membro, error) {
	return m.getMembrosEquipeFn(ctx, equipeID)
}
func (m *mockEquipeStore) AddMembroEquipe(ctx context.Context, equipeID, membroID uuid.UUID) error {
	return m.addMembroEquipeFn(ctx, equipeID, membroID)
}
func (m *mockEquipeStore) RemoveMembroEquipe(ctx context.Context, equipeID, membroID uuid.UUID) error {
	return m.removeMembroEquipeFn(ctx, equipeID, membroID)
}
func (m *mockEquipeStore) GetDiasAusencia(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) (map[uuid.UUID]int, error) {
	return m.getDiasAusenciaFn(ctx, membroIDs, inicio, fim)
}
func (m *mockEquipeStore) GetHorasTarefasEquipe(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]domain.HorasTarefasMembro, error) {
	return m.getHorasTarefasEquipeFn(ctx, membroIDs, inicio, fim)
}
func (m *mockEquipeStore) UpdateMembroCargo(ctx context.Context, membroID uuid.UUID, cargo *string) error {
	return m.updateMembroCargoFn(ctx, membroID, cargo)
}
func (m *mockEquipeStore) ListProdutos(ctx context.Context) ([]domain.Produto, error) {
	return m.listProdutosFn(ctx)
}
func (m *mockEquipeStore) GetMembroProdutos(ctx context.Context, membroID uuid.UUID) ([]domain.Produto, error) {
	return m.getMembroProdutosFn(ctx, membroID)
}
func (m *mockEquipeStore) SetMembroProdutos(ctx context.Context, membroID uuid.UUID, produtoIDs []uuid.UUID) error {
	return m.setMembroProdutosFn(ctx, membroID, produtoIDs)
}
func (m *mockEquipeStore) GetEquipeAtivaMembro(ctx context.Context, membroID uuid.UUID) (*domain.Equipe, error) {
	return m.getEquipeAtivaMembroFn(ctx, membroID)
}
func (m *mockEquipeStore) TransferirMembro(ctx context.Context, equipeOrigemID, equipeDestinoID, membroID uuid.UUID) error {
	return m.transferirMembroFn(ctx, equipeOrigemID, equipeDestinoID, membroID)
}
func (m *mockEquipeStore) InsertMeritoPromocao(ctx context.Context, membroID uuid.UUID, tipo string, cargoAnterior, cargoNovo *string, salarioAnterior *float64, salarioNovo float64, dataVigencia time.Time) (*domain.HistoricoMeritoPromocao, error) {
	return m.insertMeritoPromocaoFn(ctx, membroID, tipo, cargoAnterior, cargoNovo, salarioAnterior, salarioNovo, dataVigencia)
}
func (m *mockEquipeStore) GetMembrosEquipeComEntrada(ctx context.Context, equipeID uuid.UUID) ([]domain.MembroComEntrada, error) {
	return m.getMembrosEquipeComEntradaFn(ctx, equipeID)
}

func newTestEquipeHandler(store *mockEquipeStore) *EquipeHandler {
	return NewEquipeHandler(store, zap.NewNop())
}

func withURLParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestAddMembro_NoActiveEquipe_Success(t *testing.T) {
	equipeID := uuid.New()
	membroID := uuid.New()

	var addedEquipeID, addedMembroID uuid.UUID
	store := &mockEquipeStore{
		getEquipeAtivaMembroFn: func(ctx context.Context, mID uuid.UUID) (*domain.Equipe, error) {
			return nil, nil
		},
		addMembroEquipeFn: func(ctx context.Context, eID, mID uuid.UUID) error {
			addedEquipeID, addedMembroID = eID, mID
			return nil
		},
	}
	h := newTestEquipeHandler(store)

	body := `{"membro_id":"` + membroID.String() + `"}`
	req := httptest.NewRequest("POST", "/equipes/"+equipeID.String()+"/membros", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": equipeID.String()})
	w := httptest.NewRecorder()

	h.AddMembro(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if addedEquipeID != equipeID || addedMembroID != membroID {
		t.Errorf("AddMembroEquipe called with wrong ids: %s/%s", addedEquipeID, addedMembroID)
	}
}

func TestAddMembro_AlreadyInSameEquipe(t *testing.T) {
	equipeID := uuid.New()
	membroID := uuid.New()

	store := &mockEquipeStore{
		getEquipeAtivaMembroFn: func(ctx context.Context, mID uuid.UUID) (*domain.Equipe, error) {
			return &domain.Equipe{ID: equipeID, Nome: "Time A"}, nil
		},
	}
	h := newTestEquipeHandler(store)

	body := `{"membro_id":"` + membroID.String() + `"}`
	req := httptest.NewRequest("POST", "/equipes/"+equipeID.String()+"/membros", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": equipeID.String()})
	w := httptest.NewRecorder()

	h.AddMembro(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestAddMembro_ConflictWithOtherEquipe(t *testing.T) {
	equipeID := uuid.New()
	outraEquipeID := uuid.New()
	membroID := uuid.New()

	addCalled := false
	store := &mockEquipeStore{
		getEquipeAtivaMembroFn: func(ctx context.Context, mID uuid.UUID) (*domain.Equipe, error) {
			return &domain.Equipe{ID: outraEquipeID, Nome: "Time B"}, nil
		},
		addMembroEquipeFn: func(ctx context.Context, eID, mID uuid.UUID) error {
			addCalled = true
			return nil
		},
	}
	h := newTestEquipeHandler(store)

	body := `{"membro_id":"` + membroID.String() + `"}`
	req := httptest.NewRequest("POST", "/equipes/"+equipeID.String()+"/membros", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": equipeID.String()})
	w := httptest.NewRecorder()

	h.AddMembro(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
	if addCalled {
		t.Error("AddMembroEquipe should not be called on conflict")
	}

	var conflict domain.TransferConflict
	if err := json.NewDecoder(w.Body).Decode(&conflict); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !conflict.Conflito {
		t.Error("expected conflito=true")
	}
	if conflict.EquipeAtual.ID != outraEquipeID || conflict.EquipeAtual.Nome != "Time B" {
		t.Errorf("unexpected equipe_atual: %+v", conflict.EquipeAtual)
	}
}

func TestTransferMembro_Success(t *testing.T) {
	origemID := uuid.New()
	destinoID := uuid.New()
	membroID := uuid.New()

	var transferOrigem, transferDestino, transferMembro uuid.UUID
	store := &mockEquipeStore{
		getEquipeByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Equipe, error) {
			if id == origemID {
				return &domain.Equipe{ID: origemID, Nome: "Origem"}, nil
			}
			if id == destinoID {
				return &domain.Equipe{ID: destinoID, Nome: "Destino"}, nil
			}
			return nil, nil
		},
		transferirMembroFn: func(ctx context.Context, eOrigem, eDestino, mID uuid.UUID) error {
			transferOrigem, transferDestino, transferMembro = eOrigem, eDestino, mID
			return nil
		},
	}
	h := newTestEquipeHandler(store)

	body := `{"equipe_destino_id":"` + destinoID.String() + `"}`
	req := httptest.NewRequest("POST", "/equipes/"+origemID.String()+"/membros/"+membroID.String()+"/transferir", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": origemID.String(), "membroId": membroID.String()})
	w := httptest.NewRecorder()

	h.TransferMembro(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if transferOrigem != origemID || transferDestino != destinoID || transferMembro != membroID {
		t.Errorf("TransferirMembro called with wrong ids: %s/%s/%s", transferOrigem, transferDestino, transferMembro)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["equipe_origem"] != "Origem" || resp["equipe_destino"] != "Destino" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestTransferMembro_EquipeOrigemNaoEncontrada(t *testing.T) {
	origemID := uuid.New()
	destinoID := uuid.New()
	membroID := uuid.New()

	store := &mockEquipeStore{
		getEquipeByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Equipe, error) {
			return nil, nil
		},
	}
	h := newTestEquipeHandler(store)

	body := `{"equipe_destino_id":"` + destinoID.String() + `"}`
	req := httptest.NewRequest("POST", "/equipes/"+origemID.String()+"/membros/"+membroID.String()+"/transferir", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": origemID.String(), "membroId": membroID.String()})
	w := httptest.NewRecorder()

	h.TransferMembro(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestTransferMembro_EquipeDestinoNaoEncontrada(t *testing.T) {
	origemID := uuid.New()
	destinoID := uuid.New()
	membroID := uuid.New()

	store := &mockEquipeStore{
		getEquipeByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Equipe, error) {
			if id == origemID {
				return &domain.Equipe{ID: origemID, Nome: "Origem"}, nil
			}
			return nil, nil
		},
	}
	h := newTestEquipeHandler(store)

	body := `{"equipe_destino_id":"` + destinoID.String() + `"}`
	req := httptest.NewRequest("POST", "/equipes/"+origemID.String()+"/membros/"+membroID.String()+"/transferir", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": origemID.String(), "membroId": membroID.String()})
	w := httptest.NewRecorder()

	h.TransferMembro(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestTransferMembro_InvalidBody(t *testing.T) {
	origemID := uuid.New()
	membroID := uuid.New()

	h := newTestEquipeHandler(&mockEquipeStore{})

	req := httptest.NewRequest("POST", "/equipes/"+origemID.String()+"/membros/"+membroID.String()+"/transferir", bytes.NewBufferString(`{"equipe_destino_id":"not-a-uuid"}`))
	req = withURLParams(req, map[string]string{"id": origemID.String(), "membroId": membroID.String()})
	w := httptest.NewRecorder()

	h.TransferMembro(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}
