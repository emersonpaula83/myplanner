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
)

// --- List ---

func TestEquipeList_Success(t *testing.T) {
	store := &mockEquipeStore{
		listEquipesFn: func(ctx context.Context) ([]domain.Equipe, error) {
			return []domain.Equipe{{ID: uuid.New(), Nome: "Time A"}}, nil
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/equipes", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestEquipeList_Error(t *testing.T) {
	store := &mockEquipeStore{
		listEquipesFn: func(ctx context.Context) ([]domain.Equipe, error) {
			return nil, errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/equipes", nil)
	w := httptest.NewRecorder()

	h.List(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- Create ---

func TestEquipeCreate_Success(t *testing.T) {
	store := &mockEquipeStore{
		createEquipeFn: func(ctx context.Context, nome string) (*domain.Equipe, error) {
			return &domain.Equipe{ID: uuid.New(), Nome: nome}, nil
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("POST", "/equipes", bytes.NewBufferString(`{"nome":"Time A"}`))
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
}

func TestEquipeCreate_InvalidBody(t *testing.T) {
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("POST", "/equipes", bytes.NewBufferString(`not-json`))
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestEquipeCreate_MissingNome(t *testing.T) {
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("POST", "/equipes", bytes.NewBufferString(`{"nome":""}`))
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestEquipeCreate_Error(t *testing.T) {
	store := &mockEquipeStore{
		createEquipeFn: func(ctx context.Context, nome string) (*domain.Equipe, error) {
			return nil, errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("POST", "/equipes", bytes.NewBufferString(`{"nome":"Time A"}`))
	w := httptest.NewRecorder()

	h.Create(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- Update ---

func TestEquipeUpdate_Success(t *testing.T) {
	id := uuid.New()
	store := &mockEquipeStore{
		updateEquipeFn: func(ctx context.Context, eid uuid.UUID, nome string, boardID *int) error {
			if eid != id || nome != "Novo Nome" {
				t.Errorf("unexpected args: id=%s nome=%s", eid, nome)
			}
			return nil
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("PUT", "/equipes/"+id.String(), bytes.NewBufferString(`{"nome":"Novo Nome"}`))
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.Update(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestEquipeUpdate_InvalidID(t *testing.T) {
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("PUT", "/equipes/xxx", bytes.NewBufferString(`{"nome":"a"}`))
	req = withURLParams(req, map[string]string{"id": "xxx"})
	w := httptest.NewRecorder()

	h.Update(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestEquipeUpdate_InvalidBody(t *testing.T) {
	id := uuid.New()
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("PUT", "/equipes/"+id.String(), bytes.NewBufferString(`not-json`))
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.Update(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestEquipeUpdate_MissingNome(t *testing.T) {
	id := uuid.New()
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("PUT", "/equipes/"+id.String(), bytes.NewBufferString(`{"nome":""}`))
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.Update(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestEquipeUpdate_Error(t *testing.T) {
	id := uuid.New()
	store := &mockEquipeStore{
		updateEquipeFn: func(ctx context.Context, eid uuid.UUID, nome string, boardID *int) error {
			return errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("PUT", "/equipes/"+id.String(), bytes.NewBufferString(`{"nome":"x"}`))
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.Update(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- Delete ---

func TestEquipeDelete_Success(t *testing.T) {
	id := uuid.New()
	store := &mockEquipeStore{
		deleteEquipeFn: func(ctx context.Context, eid uuid.UUID) error {
			if eid != id {
				t.Errorf("unexpected id: %s", eid)
			}
			return nil
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("DELETE", "/equipes/"+id.String(), nil)
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.Delete(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestEquipeDelete_InvalidID(t *testing.T) {
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("DELETE", "/equipes/xxx", nil)
	req = withURLParams(req, map[string]string{"id": "xxx"})
	w := httptest.NewRecorder()

	h.Delete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestEquipeDelete_Error(t *testing.T) {
	id := uuid.New()
	store := &mockEquipeStore{
		deleteEquipeFn: func(ctx context.Context, eid uuid.UUID) error {
			return errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("DELETE", "/equipes/"+id.String(), nil)
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.Delete(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- GetResumo ---

func TestGetResumo_InvalidID(t *testing.T) {
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("GET", "/equipes/xxx/resumo", nil)
	req = withURLParams(req, map[string]string{"id": "xxx"})
	w := httptest.NewRecorder()

	h.GetResumo(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetResumo_InvalidPeriodo(t *testing.T) {
	id := uuid.New()
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("GET", "/equipes/"+id.String()+"/resumo?periodo=99z", nil)
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.GetResumo(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetResumo_ErrorGetEquipe(t *testing.T) {
	id := uuid.New()
	store := &mockEquipeStore{
		getEquipeByIDFn: func(ctx context.Context, eid uuid.UUID) (*domain.Equipe, error) {
			return nil, errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/equipes/"+id.String()+"/resumo", nil)
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.GetResumo(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetResumo_NotFound(t *testing.T) {
	id := uuid.New()
	store := &mockEquipeStore{
		getEquipeByIDFn: func(ctx context.Context, eid uuid.UUID) (*domain.Equipe, error) {
			return nil, nil
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/equipes/"+id.String()+"/resumo", nil)
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.GetResumo(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetResumo_ErrorGetMembros(t *testing.T) {
	id := uuid.New()
	store := &mockEquipeStore{
		getEquipeByIDFn: func(ctx context.Context, eid uuid.UUID) (*domain.Equipe, error) {
			return &domain.Equipe{ID: id, Nome: "Time A"}, nil
		},
		getMembrosEquipeFn: func(ctx context.Context, eid uuid.UUID) ([]domain.Membro, error) {
			return nil, errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/equipes/"+id.String()+"/resumo", nil)
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.GetResumo(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetResumo_NoMembros(t *testing.T) {
	id := uuid.New()
	store := &mockEquipeStore{
		getEquipeByIDFn: func(ctx context.Context, eid uuid.UUID) (*domain.Equipe, error) {
			return &domain.Equipe{ID: id, Nome: "Time A"}, nil
		},
		getMembrosEquipeFn: func(ctx context.Context, eid uuid.UUID) ([]domain.Membro, error) {
			return []domain.Membro{}, nil
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/equipes/"+id.String()+"/resumo", nil)
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.GetResumo(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resumo domain.ResumoEquipe
	if err := json.NewDecoder(w.Body).Decode(&resumo); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resumo.Membros) != 0 {
		t.Errorf("expected empty membros, got %d", len(resumo.Membros))
	}
}

func TestGetResumo_ErrorAusencias(t *testing.T) {
	id := uuid.New()
	membroID := uuid.New()
	store := &mockEquipeStore{
		getEquipeByIDFn: func(ctx context.Context, eid uuid.UUID) (*domain.Equipe, error) {
			return &domain.Equipe{ID: id, Nome: "Time A"}, nil
		},
		getMembrosEquipeFn: func(ctx context.Context, eid uuid.UUID) ([]domain.Membro, error) {
			return []domain.Membro{{ID: membroID, Nome: "Alice"}}, nil
		},
		getDiasAusenciaFn: func(ctx context.Context, ids []uuid.UUID, inicio, fim time.Time) (map[uuid.UUID]int, error) {
			return nil, errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/equipes/"+id.String()+"/resumo", nil)
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.GetResumo(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetResumo_ErrorTarefas(t *testing.T) {
	id := uuid.New()
	membroID := uuid.New()
	store := &mockEquipeStore{
		getEquipeByIDFn: func(ctx context.Context, eid uuid.UUID) (*domain.Equipe, error) {
			return &domain.Equipe{ID: id, Nome: "Time A"}, nil
		},
		getMembrosEquipeFn: func(ctx context.Context, eid uuid.UUID) ([]domain.Membro, error) {
			return []domain.Membro{{ID: membroID, Nome: "Alice"}}, nil
		},
		getDiasAusenciaFn: func(ctx context.Context, ids []uuid.UUID, inicio, fim time.Time) (map[uuid.UUID]int, error) {
			return map[uuid.UUID]int{}, nil
		},
		getHorasTarefasEquipeFn: func(ctx context.Context, ids []uuid.UUID, inicio, fim time.Time) ([]domain.HorasTarefasMembro, error) {
			return nil, errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/equipes/"+id.String()+"/resumo", nil)
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.GetResumo(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestGetResumo_Success(t *testing.T) {
	id := uuid.New()
	membroID := uuid.New()
	store := &mockEquipeStore{
		getEquipeByIDFn: func(ctx context.Context, eid uuid.UUID) (*domain.Equipe, error) {
			return &domain.Equipe{ID: id, Nome: "Time A"}, nil
		},
		getMembrosEquipeFn: func(ctx context.Context, eid uuid.UUID) ([]domain.Membro, error) {
			return []domain.Membro{{ID: membroID, Nome: "Alice"}}, nil
		},
		getDiasAusenciaFn: func(ctx context.Context, ids []uuid.UUID, inicio, fim time.Time) (map[uuid.UUID]int, error) {
			return map[uuid.UUID]int{}, nil
		},
		getHorasTarefasEquipeFn: func(ctx context.Context, ids []uuid.UUID, inicio, fim time.Time) ([]domain.HorasTarefasMembro, error) {
			return []domain.HorasTarefasMembro{}, nil
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/equipes/"+id.String()+"/resumo?periodo=1m", nil)
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.GetResumo(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// --- GetMembros ---

func TestGetMembros_Success(t *testing.T) {
	id := uuid.New()
	store := &mockEquipeStore{
		getMembrosEquipeFn: func(ctx context.Context, eid uuid.UUID) ([]domain.Membro, error) {
			return []domain.Membro{{ID: uuid.New(), Nome: "Alice"}}, nil
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/equipes/"+id.String()+"/membros", nil)
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.GetMembros(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestGetMembros_InvalidID(t *testing.T) {
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("GET", "/equipes/xxx/membros", nil)
	req = withURLParams(req, map[string]string{"id": "xxx"})
	w := httptest.NewRecorder()

	h.GetMembros(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetMembros_Error(t *testing.T) {
	id := uuid.New()
	store := &mockEquipeStore{
		getMembrosEquipeFn: func(ctx context.Context, eid uuid.UUID) ([]domain.Membro, error) {
			return nil, errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/equipes/"+id.String()+"/membros", nil)
	req = withURLParams(req, map[string]string{"id": id.String()})
	w := httptest.NewRecorder()

	h.GetMembros(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- RemoveMembro ---

func TestRemoveMembro_Success(t *testing.T) {
	equipeID := uuid.New()
	membroID := uuid.New()
	store := &mockEquipeStore{
		removeMembroEquipeFn: func(ctx context.Context, eid, mid uuid.UUID) error {
			if eid != equipeID || mid != membroID {
				t.Errorf("unexpected ids: %s/%s", eid, mid)
			}
			return nil
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("DELETE", "/equipes/"+equipeID.String()+"/membros/"+membroID.String(), nil)
	req = withURLParams(req, map[string]string{"id": equipeID.String(), "membroId": membroID.String()})
	w := httptest.NewRecorder()

	h.RemoveMembro(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestRemoveMembro_InvalidEquipeID(t *testing.T) {
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("DELETE", "/equipes/xxx/membros/"+uuid.New().String(), nil)
	req = withURLParams(req, map[string]string{"id": "xxx", "membroId": uuid.New().String()})
	w := httptest.NewRecorder()

	h.RemoveMembro(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestRemoveMembro_InvalidMembroID(t *testing.T) {
	equipeID := uuid.New()
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("DELETE", "/equipes/"+equipeID.String()+"/membros/xxx", nil)
	req = withURLParams(req, map[string]string{"id": equipeID.String(), "membroId": "xxx"})
	w := httptest.NewRecorder()

	h.RemoveMembro(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestRemoveMembro_Error(t *testing.T) {
	equipeID := uuid.New()
	membroID := uuid.New()
	store := &mockEquipeStore{
		removeMembroEquipeFn: func(ctx context.Context, eid, mid uuid.UUID) error {
			return errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("DELETE", "/equipes/"+equipeID.String()+"/membros/"+membroID.String(), nil)
	req = withURLParams(req, map[string]string{"id": equipeID.String(), "membroId": membroID.String()})
	w := httptest.NewRecorder()

	h.RemoveMembro(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- UpdateCargo ---

func TestUpdateCargo_Success(t *testing.T) {
	membroID := uuid.New()
	var capturedCargo *string
	store := &mockEquipeStore{
		updateMembroCargoFn: func(ctx context.Context, mid uuid.UUID, cargo *string) error {
			capturedCargo = cargo
			return nil
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("PUT", "/membros/"+membroID.String()+"/cargo", bytes.NewBufferString(`{"cargo":"analista_i"}`))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	w := httptest.NewRecorder()

	h.UpdateCargo(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if capturedCargo == nil || *capturedCargo != "analista_i" {
		t.Errorf("capturedCargo = %v, want analista_i", capturedCargo)
	}
}

func TestUpdateCargo_EmptyCargoClearsField(t *testing.T) {
	membroID := uuid.New()
	var capturedCargo *string
	called := false
	store := &mockEquipeStore{
		updateMembroCargoFn: func(ctx context.Context, mid uuid.UUID, cargo *string) error {
			capturedCargo = cargo
			called = true
			return nil
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("PUT", "/membros/"+membroID.String()+"/cargo", bytes.NewBufferString(`{"cargo":""}`))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	w := httptest.NewRecorder()

	h.UpdateCargo(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !called || capturedCargo != nil {
		t.Errorf("expected nil cargo, got %v (called=%v)", capturedCargo, called)
	}
}

func TestUpdateCargo_InvalidID(t *testing.T) {
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("PUT", "/membros/xxx/cargo", bytes.NewBufferString(`{"cargo":"analista_i"}`))
	req = withURLParams(req, map[string]string{"id": "xxx"})
	w := httptest.NewRecorder()

	h.UpdateCargo(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpdateCargo_InvalidBody(t *testing.T) {
	membroID := uuid.New()
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("PUT", "/membros/"+membroID.String()+"/cargo", bytes.NewBufferString(`not-json`))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	w := httptest.NewRecorder()

	h.UpdateCargo(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpdateCargo_InvalidCargo(t *testing.T) {
	membroID := uuid.New()
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("PUT", "/membros/"+membroID.String()+"/cargo", bytes.NewBufferString(`{"cargo":"invalido"}`))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	w := httptest.NewRecorder()

	h.UpdateCargo(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpdateCargo_Error(t *testing.T) {
	membroID := uuid.New()
	store := &mockEquipeStore{
		updateMembroCargoFn: func(ctx context.Context, mid uuid.UUID, cargo *string) error {
			return errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("PUT", "/membros/"+membroID.String()+"/cargo", bytes.NewBufferString(`{"cargo":"analista_i"}`))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	w := httptest.NewRecorder()

	h.UpdateCargo(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- ListCargos ---

func TestListCargos_Success(t *testing.T) {
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("GET", "/cargos", nil)
	w := httptest.NewRecorder()

	h.ListCargos(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var result []map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != len(domain.CargosValidos) {
		t.Errorf("got %d cargos, want %d", len(result), len(domain.CargosValidos))
	}
}

// --- ListProdutos ---

func TestListProdutos_Success(t *testing.T) {
	store := &mockEquipeStore{
		listProdutosFn: func(ctx context.Context) ([]domain.Produto, error) {
			return []domain.Produto{{ID: uuid.New(), Nome: "Produto A"}}, nil
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/produtos", nil)
	w := httptest.NewRecorder()

	h.ListProdutos(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestListProdutos_Error(t *testing.T) {
	store := &mockEquipeStore{
		listProdutosFn: func(ctx context.Context) ([]domain.Produto, error) {
			return nil, errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/produtos", nil)
	w := httptest.NewRecorder()

	h.ListProdutos(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- GetMembroProdutos ---

func TestGetMembroProdutos_Success(t *testing.T) {
	membroID := uuid.New()
	store := &mockEquipeStore{
		getMembroProdutosFn: func(ctx context.Context, mid uuid.UUID) ([]domain.Produto, error) {
			return []domain.Produto{{ID: uuid.New(), Nome: "Produto A"}}, nil
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/membros/"+membroID.String()+"/produtos", nil)
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	w := httptest.NewRecorder()

	h.GetMembroProdutos(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestGetMembroProdutos_InvalidID(t *testing.T) {
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("GET", "/membros/xxx/produtos", nil)
	req = withURLParams(req, map[string]string{"id": "xxx"})
	w := httptest.NewRecorder()

	h.GetMembroProdutos(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetMembroProdutos_Error(t *testing.T) {
	membroID := uuid.New()
	store := &mockEquipeStore{
		getMembroProdutosFn: func(ctx context.Context, mid uuid.UUID) ([]domain.Produto, error) {
			return nil, errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	req := httptest.NewRequest("GET", "/membros/"+membroID.String()+"/produtos", nil)
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	w := httptest.NewRecorder()

	h.GetMembroProdutos(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- SetMembroProdutos ---

func TestSetMembroProdutos_Success(t *testing.T) {
	membroID := uuid.New()
	produtoID := uuid.New()
	var captured []uuid.UUID
	store := &mockEquipeStore{
		setMembroProdutosFn: func(ctx context.Context, mid uuid.UUID, produtoIDs []uuid.UUID) error {
			captured = produtoIDs
			return nil
		},
	}
	h := newTestEquipeHandler(store)
	body := `{"produto_ids":["` + produtoID.String() + `","` + produtoID.String() + `"]}`
	req := httptest.NewRequest("PUT", "/membros/"+membroID.String()+"/produtos", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	w := httptest.NewRecorder()

	h.SetMembroProdutos(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if len(captured) != 1 || captured[0] != produtoID {
		t.Errorf("expected deduped [%s], got %v", produtoID, captured)
	}
}

func TestSetMembroProdutos_InvalidID(t *testing.T) {
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("PUT", "/membros/xxx/produtos", bytes.NewBufferString(`{"produto_ids":[]}`))
	req = withURLParams(req, map[string]string{"id": "xxx"})
	w := httptest.NewRecorder()

	h.SetMembroProdutos(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSetMembroProdutos_InvalidBody(t *testing.T) {
	membroID := uuid.New()
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("PUT", "/membros/"+membroID.String()+"/produtos", bytes.NewBufferString(`not-json`))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	w := httptest.NewRecorder()

	h.SetMembroProdutos(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSetMembroProdutos_InvalidProdutoID(t *testing.T) {
	membroID := uuid.New()
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("PUT", "/membros/"+membroID.String()+"/produtos", bytes.NewBufferString(`{"produto_ids":["xxx"]}`))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	w := httptest.NewRecorder()

	h.SetMembroProdutos(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSetMembroProdutos_Error(t *testing.T) {
	membroID := uuid.New()
	produtoID := uuid.New()
	store := &mockEquipeStore{
		setMembroProdutosFn: func(ctx context.Context, mid uuid.UUID, produtoIDs []uuid.UUID) error {
			return errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	body := `{"produto_ids":["` + produtoID.String() + `"]}`
	req := httptest.NewRequest("PUT", "/membros/"+membroID.String()+"/produtos", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	w := httptest.NewRecorder()

	h.SetMembroProdutos(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// --- MeritoPromocao ---

func TestMeritoPromocao_InvalidID(t *testing.T) {
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("POST", "/membros/xxx/merito", bytes.NewBufferString(`{}`))
	req = withURLParams(req, map[string]string{"id": "xxx"})
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.MeritoPromocao(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestMeritoPromocao_InvalidBody(t *testing.T) {
	membroID := uuid.New()
	h := newTestEquipeHandler(&mockEquipeStore{})
	req := httptest.NewRequest("POST", "/membros/"+membroID.String()+"/merito", bytes.NewBufferString(`not-json`))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.MeritoPromocao(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestMeritoPromocao_InvalidTipo(t *testing.T) {
	membroID := uuid.New()
	h := newTestEquipeHandler(&mockEquipeStore{})
	body := `{"tipo":"outro","salario_novo":5000,"data_vigencia":"2026-08-01"}`
	req := httptest.NewRequest("POST", "/membros/"+membroID.String()+"/merito", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.MeritoPromocao(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestMeritoPromocao_InvalidData(t *testing.T) {
	membroID := uuid.New()
	h := newTestEquipeHandler(&mockEquipeStore{})
	body := `{"tipo":"merito","salario_novo":5000,"data_vigencia":"01-08-2026"}`
	req := httptest.NewRequest("POST", "/membros/"+membroID.String()+"/merito", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.MeritoPromocao(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestMeritoPromocao_ErrorGetMembro(t *testing.T) {
	membroID := uuid.New()
	store := &mockEquipeStore{
		getMembroByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
			return nil, errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	body := `{"tipo":"merito","salario_novo":5000,"data_vigencia":"2026-08-01"}`
	req := httptest.NewRequest("POST", "/membros/"+membroID.String()+"/merito", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.MeritoPromocao(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestMeritoPromocao_MembroNotFound(t *testing.T) {
	membroID := uuid.New()
	store := &mockEquipeStore{
		getMembroByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
			return nil, nil
		},
	}
	h := newTestEquipeHandler(store)
	body := `{"tipo":"merito","salario_novo":5000,"data_vigencia":"2026-08-01"}`
	req := httptest.NewRequest("POST", "/membros/"+membroID.String()+"/merito", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.MeritoPromocao(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestMeritoPromocao_SalarioMenor(t *testing.T) {
	membroID := uuid.New()
	salarioAtual := 6000.0
	store := &mockEquipeStore{
		getMembroByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
			return &domain.Membro{ID: membroID, Salario: &salarioAtual}, nil
		},
	}
	h := newTestEquipeHandler(store)
	body := `{"tipo":"merito","salario_novo":5000,"data_vigencia":"2026-08-01"}`
	req := httptest.NewRequest("POST", "/membros/"+membroID.String()+"/merito", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.MeritoPromocao(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestMeritoPromocao_PromocaoMissingCargoNovo(t *testing.T) {
	membroID := uuid.New()
	cargoAtual := domain.CargoAnalistaI
	store := &mockEquipeStore{
		getMembroByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
			return &domain.Membro{ID: membroID, Cargo: &cargoAtual}, nil
		},
	}
	h := newTestEquipeHandler(store)
	body := `{"tipo":"promocao","salario_novo":5000,"data_vigencia":"2026-08-01"}`
	req := httptest.NewRequest("POST", "/membros/"+membroID.String()+"/merito", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.MeritoPromocao(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestMeritoPromocao_PromocaoInvalidCargoNovo(t *testing.T) {
	membroID := uuid.New()
	cargoAtual := domain.CargoAnalistaI
	store := &mockEquipeStore{
		getMembroByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
			return &domain.Membro{ID: membroID, Cargo: &cargoAtual}, nil
		},
	}
	h := newTestEquipeHandler(store)
	body := `{"tipo":"promocao","cargo_novo":"invalido","salario_novo":5000,"data_vigencia":"2026-08-01"}`
	req := httptest.NewRequest("POST", "/membros/"+membroID.String()+"/merito", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.MeritoPromocao(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestMeritoPromocao_PromocaoInvalidTransition(t *testing.T) {
	membroID := uuid.New()
	cargoAtual := domain.CargoAnalistaI
	store := &mockEquipeStore{
		getMembroByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
			return &domain.Membro{ID: membroID, Cargo: &cargoAtual}, nil
		},
	}
	h := newTestEquipeHandler(store)
	// analista_i cannot be promoted directly to master
	body := `{"tipo":"promocao","cargo_novo":"master","salario_novo":5000,"data_vigencia":"2026-08-01"}`
	req := httptest.NewRequest("POST", "/membros/"+membroID.String()+"/merito", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.MeritoPromocao(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestMeritoPromocao_PromocaoSuccess(t *testing.T) {
	membroID := uuid.New()
	cargoAtual := domain.CargoAnalistaI
	historicoID := uuid.New()
	store := &mockEquipeStore{
		getMembroByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
			return &domain.Membro{ID: membroID, Cargo: &cargoAtual}, nil
		},
		insertMeritoPromocaoFn: func(ctx context.Context, mid uuid.UUID, tipo string, cargoAnterior, cargoNovo *string, salarioAnterior *float64, salarioNovo float64, dataVigencia time.Time) (*domain.HistoricoMeritoPromocao, error) {
			if tipo != "promocao" || cargoNovo == nil || *cargoNovo != domain.CargoAnalistaII {
				t.Errorf("unexpected args: tipo=%s cargoNovo=%v", tipo, cargoNovo)
			}
			return &domain.HistoricoMeritoPromocao{ID: historicoID}, nil
		},
	}
	h := newTestEquipeHandler(store)
	body := `{"tipo":"promocao","cargo_novo":"analista_ii","salario_novo":5000,"data_vigencia":"2026-08-01"}`
	req := httptest.NewRequest("POST", "/membros/"+membroID.String()+"/merito", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.MeritoPromocao(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp domain.MeritoPromocaoResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.HistoricoID != historicoID {
		t.Errorf("HistoricoID = %s, want %s", resp.HistoricoID, historicoID)
	}
	if resp.Depois.Cargo == nil || *resp.Depois.Cargo != domain.CargoAnalistaII {
		t.Errorf("Depois.Cargo = %v, want analista_ii", resp.Depois.Cargo)
	}
}

func TestMeritoPromocao_MeritoCargoMismatch(t *testing.T) {
	membroID := uuid.New()
	cargoAtual := domain.CargoAnalistaI
	store := &mockEquipeStore{
		getMembroByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
			return &domain.Membro{ID: membroID, Cargo: &cargoAtual}, nil
		},
	}
	h := newTestEquipeHandler(store)
	body := `{"tipo":"merito","cargo_novo":"analista_ii","salario_novo":5000,"data_vigencia":"2026-08-01"}`
	req := httptest.NewRequest("POST", "/membros/"+membroID.String()+"/merito", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.MeritoPromocao(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestMeritoPromocao_MeritoSuccess(t *testing.T) {
	membroID := uuid.New()
	cargoAtual := domain.CargoAnalistaI
	historicoID := uuid.New()
	store := &mockEquipeStore{
		getMembroByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
			return &domain.Membro{ID: membroID, Cargo: &cargoAtual}, nil
		},
		insertMeritoPromocaoFn: func(ctx context.Context, mid uuid.UUID, tipo string, cargoAnterior, cargoNovo *string, salarioAnterior *float64, salarioNovo float64, dataVigencia time.Time) (*domain.HistoricoMeritoPromocao, error) {
			return &domain.HistoricoMeritoPromocao{ID: historicoID}, nil
		},
	}
	h := newTestEquipeHandler(store)
	body := `{"tipo":"merito","salario_novo":5000,"data_vigencia":"2026-08-01"}`
	req := httptest.NewRequest("POST", "/membros/"+membroID.String()+"/merito", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.MeritoPromocao(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestMeritoPromocao_ErrorInsert(t *testing.T) {
	membroID := uuid.New()
	store := &mockEquipeStore{
		getMembroByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
			return &domain.Membro{ID: membroID}, nil
		},
		insertMeritoPromocaoFn: func(ctx context.Context, mid uuid.UUID, tipo string, cargoAnterior, cargoNovo *string, salarioAnterior *float64, salarioNovo float64, dataVigencia time.Time) (*domain.HistoricoMeritoPromocao, error) {
			return nil, errors.New("db error")
		},
	}
	h := newTestEquipeHandler(store)
	body := `{"tipo":"merito","salario_novo":5000,"data_vigencia":"2026-08-01"}`
	req := httptest.NewRequest("POST", "/membros/"+membroID.String()+"/merito", bytes.NewBufferString(body))
	req = withURLParams(req, map[string]string{"id": membroID.String()})
	req = comCortinaAberta(req)
	w := httptest.NewRecorder()

	h.MeritoPromocao(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}
