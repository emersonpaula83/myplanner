package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockDestinatarioStore struct {
	listByEquipeFn func(ctx context.Context, equipeID uuid.UUID) ([]repository.Destinatario, error)
	createFn       func(ctx context.Context, equipeID uuid.UUID, tipo, valor string, nome *string) (*repository.Destinatario, error)
	deleteFn       func(ctx context.Context, id uuid.UUID, equipeID uuid.UUID) error
}

func (m *mockDestinatarioStore) ListByEquipe(ctx context.Context, equipeID uuid.UUID) ([]repository.Destinatario, error) {
	return m.listByEquipeFn(ctx, equipeID)
}

func (m *mockDestinatarioStore) Create(ctx context.Context, equipeID uuid.UUID, tipo, valor string, nome *string) (*repository.Destinatario, error) {
	return m.createFn(ctx, equipeID, tipo, valor, nome)
}

func (m *mockDestinatarioStore) Delete(ctx context.Context, id uuid.UUID, equipeID uuid.UUID) error {
	return m.deleteFn(ctx, id, equipeID)
}

type mockNotifService struct {
	enviarReviewFn func(ctx context.Context, sprintID, equipeID uuid.UUID, destIDs []uuid.UUID) ([]service.EnvioResultado, error)
}

func (m *mockNotifService) EnviarReview(ctx context.Context, sprintID, equipeID uuid.UUID, destIDs []uuid.UUID) ([]service.EnvioResultado, error) {
	return m.enviarReviewFn(ctx, sprintID, equipeID, destIDs)
}

// --- ListDestinatarios ---

func TestNotificationHandler_ListDestinatarios(t *testing.T) {
	equipeID := uuid.New()
	destRepo := &mockDestinatarioStore{
		listByEquipeFn: func(ctx context.Context, e uuid.UUID) ([]repository.Destinatario, error) {
			return []repository.Destinatario{{ID: uuid.New(), EquipeID: e}}, nil
		},
	}
	h := NewNotificationHandler(destRepo, &mockNotifService{}, zap.NewNop())

	req := httptest.NewRequest("GET", "/notification/equipes/"+equipeID.String()+"/destinatarios", nil)
	req = addChiParam(req, "id", equipeID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.ListDestinatarios(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNotificationHandler_ListDestinatarios_InvalidID(t *testing.T) {
	h := NewNotificationHandler(&mockDestinatarioStore{}, &mockNotifService{}, zap.NewNop())

	req := httptest.NewRequest("GET", "/notification/equipes/bad/destinatarios", nil)
	req = addChiParam(req, "id", "bad")
	w := httptest.NewRecorder()
	h.ListDestinatarios(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNotificationHandler_ListDestinatarios_Forbidden(t *testing.T) {
	equipeID := uuid.New()
	h := NewNotificationHandler(&mockDestinatarioStore{}, &mockNotifService{}, zap.NewNop())

	req := httptest.NewRequest("GET", "/notification/equipes/"+equipeID.String()+"/destinatarios", nil)
	req = addChiParam(req, "id", equipeID.String())
	w := httptest.NewRecorder()
	h.ListDestinatarios(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestNotificationHandler_ListDestinatarios_Error(t *testing.T) {
	equipeID := uuid.New()
	destRepo := &mockDestinatarioStore{
		listByEquipeFn: func(ctx context.Context, e uuid.UUID) ([]repository.Destinatario, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewNotificationHandler(destRepo, &mockNotifService{}, zap.NewNop())

	req := httptest.NewRequest("GET", "/notification/equipes/"+equipeID.String()+"/destinatarios", nil)
	req = addChiParam(req, "id", equipeID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.ListDestinatarios(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- CreateDestinatario ---

func TestNotificationHandler_CreateDestinatario(t *testing.T) {
	equipeID := uuid.New()
	destRepo := &mockDestinatarioStore{
		createFn: func(ctx context.Context, e uuid.UUID, tipo, valor string, nome *string) (*repository.Destinatario, error) {
			return &repository.Destinatario{ID: uuid.New(), EquipeID: e, Tipo: tipo, Valor: valor}, nil
		},
	}
	h := NewNotificationHandler(destRepo, &mockNotifService{}, zap.NewNop())

	body, _ := json.Marshal(map[string]string{"tipo": "email", "valor": "a@b.com"})
	req := httptest.NewRequest("POST", "/notification/equipes/"+equipeID.String()+"/destinatarios", bytes.NewReader(body))
	req = addChiParam(req, "id", equipeID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.CreateDestinatario(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNotificationHandler_CreateDestinatario_InvalidID(t *testing.T) {
	h := NewNotificationHandler(&mockDestinatarioStore{}, &mockNotifService{}, zap.NewNop())

	req := httptest.NewRequest("POST", "/notification/equipes/bad/destinatarios", nil)
	req = addChiParam(req, "id", "bad")
	w := httptest.NewRecorder()
	h.CreateDestinatario(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNotificationHandler_CreateDestinatario_Forbidden(t *testing.T) {
	equipeID := uuid.New()
	h := NewNotificationHandler(&mockDestinatarioStore{}, &mockNotifService{}, zap.NewNop())

	req := httptest.NewRequest("POST", "/notification/equipes/"+equipeID.String()+"/destinatarios", nil)
	req = addChiParam(req, "id", equipeID.String())
	w := httptest.NewRecorder()
	h.CreateDestinatario(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestNotificationHandler_CreateDestinatario_InvalidBody(t *testing.T) {
	equipeID := uuid.New()
	h := NewNotificationHandler(&mockDestinatarioStore{}, &mockNotifService{}, zap.NewNop())

	req := httptest.NewRequest("POST", "/notification/equipes/"+equipeID.String()+"/destinatarios", bytes.NewReader([]byte("not-json")))
	req = addChiParam(req, "id", equipeID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.CreateDestinatario(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNotificationHandler_CreateDestinatario_InvalidTipo(t *testing.T) {
	equipeID := uuid.New()
	h := NewNotificationHandler(&mockDestinatarioStore{}, &mockNotifService{}, zap.NewNop())

	body, _ := json.Marshal(map[string]string{"tipo": "sms", "valor": "123"})
	req := httptest.NewRequest("POST", "/notification/equipes/"+equipeID.String()+"/destinatarios", bytes.NewReader(body))
	req = addChiParam(req, "id", equipeID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.CreateDestinatario(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNotificationHandler_CreateDestinatario_MissingValor(t *testing.T) {
	equipeID := uuid.New()
	h := NewNotificationHandler(&mockDestinatarioStore{}, &mockNotifService{}, zap.NewNop())

	body, _ := json.Marshal(map[string]string{"tipo": "email", "valor": ""})
	req := httptest.NewRequest("POST", "/notification/equipes/"+equipeID.String()+"/destinatarios", bytes.NewReader(body))
	req = addChiParam(req, "id", equipeID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.CreateDestinatario(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNotificationHandler_CreateDestinatario_Error(t *testing.T) {
	equipeID := uuid.New()
	destRepo := &mockDestinatarioStore{
		createFn: func(ctx context.Context, e uuid.UUID, tipo, valor string, nome *string) (*repository.Destinatario, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewNotificationHandler(destRepo, &mockNotifService{}, zap.NewNop())

	body, _ := json.Marshal(map[string]string{"tipo": "email", "valor": "a@b.com"})
	req := httptest.NewRequest("POST", "/notification/equipes/"+equipeID.String()+"/destinatarios", bytes.NewReader(body))
	req = addChiParam(req, "id", equipeID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.CreateDestinatario(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- DeleteDestinatario ---

func TestNotificationHandler_DeleteDestinatario(t *testing.T) {
	equipeID := uuid.New()
	destID := uuid.New()
	destRepo := &mockDestinatarioStore{
		deleteFn: func(ctx context.Context, id uuid.UUID, e uuid.UUID) error {
			return nil
		},
	}
	h := NewNotificationHandler(destRepo, &mockNotifService{}, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/notification/equipes/"+equipeID.String()+"/destinatarios/"+destID.String(), nil)
	req = addChiParam(req, "id", equipeID.String())
	req = addChiParam(req, "destId", destID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.DeleteDestinatario(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNotificationHandler_DeleteDestinatario_InvalidEquipeID(t *testing.T) {
	h := NewNotificationHandler(&mockDestinatarioStore{}, &mockNotifService{}, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/notification/equipes/bad/destinatarios/"+uuid.New().String(), nil)
	req = addChiParam(req, "id", "bad")
	w := httptest.NewRecorder()
	h.DeleteDestinatario(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNotificationHandler_DeleteDestinatario_Forbidden(t *testing.T) {
	equipeID := uuid.New()
	destID := uuid.New()
	h := NewNotificationHandler(&mockDestinatarioStore{}, &mockNotifService{}, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/notification/equipes/"+equipeID.String()+"/destinatarios/"+destID.String(), nil)
	req = addChiParam(req, "id", equipeID.String())
	req = addChiParam(req, "destId", destID.String())
	w := httptest.NewRecorder()
	h.DeleteDestinatario(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestNotificationHandler_DeleteDestinatario_InvalidDestID(t *testing.T) {
	equipeID := uuid.New()
	h := NewNotificationHandler(&mockDestinatarioStore{}, &mockNotifService{}, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/notification/equipes/"+equipeID.String()+"/destinatarios/bad", nil)
	req = addChiParam(req, "id", equipeID.String())
	req = addChiParam(req, "destId", "bad")
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.DeleteDestinatario(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNotificationHandler_DeleteDestinatario_Error(t *testing.T) {
	equipeID := uuid.New()
	destID := uuid.New()
	destRepo := &mockDestinatarioStore{
		deleteFn: func(ctx context.Context, id uuid.UUID, e uuid.UUID) error {
			return errors.New("boom")
		},
	}
	h := NewNotificationHandler(destRepo, &mockNotifService{}, zap.NewNop())

	req := httptest.NewRequest("DELETE", "/notification/equipes/"+equipeID.String()+"/destinatarios/"+destID.String(), nil)
	req = addChiParam(req, "id", equipeID.String())
	req = addChiParam(req, "destId", destID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.DeleteDestinatario(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- EnviarReview ---

func TestNotificationHandler_EnviarReview(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	destID := uuid.New()
	notifSvc := &mockNotifService{
		enviarReviewFn: func(ctx context.Context, s, e uuid.UUID, destIDs []uuid.UUID) ([]service.EnvioResultado, error) {
			return []service.EnvioResultado{{DestinatarioID: destID, Tipo: "email", Status: "enviado"}}, nil
		},
	}
	h := NewNotificationHandler(&mockDestinatarioStore{}, notifSvc, zap.NewNop())

	body, _ := json.Marshal(map[string]any{"equipe_id": equipeID, "destinatario_ids": []uuid.UUID{destID}})
	req := httptest.NewRequest("POST", "/notification/sprints/"+sprintID.String()+"/review", bytes.NewReader(body))
	req = addChiParam(req, "id", sprintID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.EnviarReview(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNotificationHandler_EnviarReview_InvalidSprintID(t *testing.T) {
	h := NewNotificationHandler(&mockDestinatarioStore{}, &mockNotifService{}, zap.NewNop())

	req := httptest.NewRequest("POST", "/notification/sprints/bad/review", nil)
	req = addChiParam(req, "id", "bad")
	w := httptest.NewRecorder()
	h.EnviarReview(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNotificationHandler_EnviarReview_InvalidBody(t *testing.T) {
	sprintID := uuid.New()
	h := NewNotificationHandler(&mockDestinatarioStore{}, &mockNotifService{}, zap.NewNop())

	req := httptest.NewRequest("POST", "/notification/sprints/"+sprintID.String()+"/review", bytes.NewReader([]byte("not-json")))
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.EnviarReview(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNotificationHandler_EnviarReview_MissingDestinatarios(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	h := NewNotificationHandler(&mockDestinatarioStore{}, &mockNotifService{}, zap.NewNop())

	body, _ := json.Marshal(map[string]any{"equipe_id": equipeID, "destinatario_ids": []uuid.UUID{}})
	req := httptest.NewRequest("POST", "/notification/sprints/"+sprintID.String()+"/review", bytes.NewReader(body))
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.EnviarReview(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNotificationHandler_EnviarReview_Forbidden(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	destID := uuid.New()
	h := NewNotificationHandler(&mockDestinatarioStore{}, &mockNotifService{}, zap.NewNop())

	body, _ := json.Marshal(map[string]any{"equipe_id": equipeID, "destinatario_ids": []uuid.UUID{destID}})
	req := httptest.NewRequest("POST", "/notification/sprints/"+sprintID.String()+"/review", bytes.NewReader(body))
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.EnviarReview(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestNotificationHandler_EnviarReview_Error(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	destID := uuid.New()
	notifSvc := &mockNotifService{
		enviarReviewFn: func(ctx context.Context, s, e uuid.UUID, destIDs []uuid.UUID) ([]service.EnvioResultado, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewNotificationHandler(&mockDestinatarioStore{}, notifSvc, zap.NewNop())

	body, _ := json.Marshal(map[string]any{"equipe_id": equipeID, "destinatario_ids": []uuid.UUID{destID}})
	req := httptest.NewRequest("POST", "/notification/sprints/"+sprintID.String()+"/review", bytes.NewReader(body))
	req = addChiParam(req, "id", sprintID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.EnviarReview(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
