package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/service"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockEqualizerService struct {
	calculateFn func(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*service.EqualizerResult, error)
	applyFn     func(ctx context.Context, req service.ApplyRequest) (*service.ApplyResult, error)
}

func (m *mockEqualizerService) Calculate(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID) (*service.EqualizerResult, error) {
	return m.calculateFn(ctx, sprintID, equipeID)
}

func (m *mockEqualizerService) Apply(ctx context.Context, req service.ApplyRequest) (*service.ApplyResult, error) {
	return m.applyFn(ctx, req)
}

// --- GetSuggestions ---

func TestEqualizerHandler_GetSuggestions(t *testing.T) {
	sprintID := uuid.New()
	svc := &mockEqualizerService{
		calculateFn: func(ctx context.Context, s uuid.UUID, e *uuid.UUID) (*service.EqualizerResult, error) {
			return &service.EqualizerResult{}, nil
		},
	}
	h := NewEqualizerHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/equalizer/"+sprintID.String(), nil)
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.GetSuggestions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestEqualizerHandler_GetSuggestions_WithEquipe(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	var gotEquipe *uuid.UUID
	svc := &mockEqualizerService{
		calculateFn: func(ctx context.Context, s uuid.UUID, e *uuid.UUID) (*service.EqualizerResult, error) {
			gotEquipe = e
			return &service.EqualizerResult{}, nil
		},
	}
	h := NewEqualizerHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/equalizer/"+sprintID.String()+"?equipe="+equipeID.String(), nil)
	req = addChiParam(req, "id", sprintID.String())
	req = withEquipeAccess(req, equipeID)
	w := httptest.NewRecorder()
	h.GetSuggestions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotEquipe == nil || *gotEquipe != equipeID {
		t.Errorf("expected equipeID %s passed through, got %v", equipeID, gotEquipe)
	}
}

func TestEqualizerHandler_GetSuggestions_ForbiddenEquipe(t *testing.T) {
	sprintID := uuid.New()
	equipeID := uuid.New()
	svc := &mockEqualizerService{}
	h := NewEqualizerHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/equalizer/"+sprintID.String()+"?equipe="+equipeID.String(), nil)
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.GetSuggestions(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestEqualizerHandler_GetSuggestions_InvalidID(t *testing.T) {
	svc := &mockEqualizerService{}
	h := NewEqualizerHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/equalizer/bad", nil)
	req = addChiParam(req, "id", "bad")
	w := httptest.NewRecorder()
	h.GetSuggestions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestEqualizerHandler_GetSuggestions_Error(t *testing.T) {
	sprintID := uuid.New()
	svc := &mockEqualizerService{
		calculateFn: func(ctx context.Context, s uuid.UUID, e *uuid.UUID) (*service.EqualizerResult, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewEqualizerHandler(svc, zap.NewNop())

	req := httptest.NewRequest("GET", "/equalizer/"+sprintID.String(), nil)
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.GetSuggestions(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// --- ApplyTransfers ---

func TestEqualizerHandler_ApplyTransfers(t *testing.T) {
	sprintID := uuid.New()
	svc := &mockEqualizerService{
		applyFn: func(ctx context.Context, req service.ApplyRequest) (*service.ApplyResult, error) {
			return &service.ApplyResult{Aplicadas: 1}, nil
		},
	}
	h := NewEqualizerHandler(svc, zap.NewNop())

	body, _ := json.Marshal(service.ApplyRequest{
		FonteDadosID:   uuid.New(),
		Transferencias: []service.TransferRequest{{TarefaID: uuid.New(), TarefaKey: "K-1", NovoResponsavelID: uuid.New()}},
	})
	req := httptest.NewRequest("POST", "/equalizer/"+sprintID.String()+"/apply", bytes.NewReader(body))
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.ApplyTransfers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestEqualizerHandler_ApplyTransfers_InvalidID(t *testing.T) {
	svc := &mockEqualizerService{}
	h := NewEqualizerHandler(svc, zap.NewNop())

	req := httptest.NewRequest("POST", "/equalizer/bad/apply", nil)
	req = addChiParam(req, "id", "bad")
	w := httptest.NewRecorder()
	h.ApplyTransfers(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestEqualizerHandler_ApplyTransfers_InvalidBody(t *testing.T) {
	sprintID := uuid.New()
	svc := &mockEqualizerService{}
	h := NewEqualizerHandler(svc, zap.NewNop())

	req := httptest.NewRequest("POST", "/equalizer/"+sprintID.String()+"/apply", bytes.NewReader([]byte("not-json")))
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.ApplyTransfers(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestEqualizerHandler_ApplyTransfers_MissingTransferencias(t *testing.T) {
	sprintID := uuid.New()
	svc := &mockEqualizerService{}
	h := NewEqualizerHandler(svc, zap.NewNop())

	body, _ := json.Marshal(service.ApplyRequest{FonteDadosID: uuid.New()})
	req := httptest.NewRequest("POST", "/equalizer/"+sprintID.String()+"/apply", bytes.NewReader(body))
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.ApplyTransfers(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestEqualizerHandler_ApplyTransfers_MissingFonteDadosID(t *testing.T) {
	sprintID := uuid.New()
	svc := &mockEqualizerService{}
	h := NewEqualizerHandler(svc, zap.NewNop())

	body, _ := json.Marshal(service.ApplyRequest{
		Transferencias: []service.TransferRequest{{TarefaID: uuid.New(), TarefaKey: "K-1", NovoResponsavelID: uuid.New()}},
	})
	req := httptest.NewRequest("POST", "/equalizer/"+sprintID.String()+"/apply", bytes.NewReader(body))
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.ApplyTransfers(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestEqualizerHandler_ApplyTransfers_Error(t *testing.T) {
	sprintID := uuid.New()
	svc := &mockEqualizerService{
		applyFn: func(ctx context.Context, req service.ApplyRequest) (*service.ApplyResult, error) {
			return nil, errors.New("boom")
		},
	}
	h := NewEqualizerHandler(svc, zap.NewNop())

	body, _ := json.Marshal(service.ApplyRequest{
		FonteDadosID:   uuid.New(),
		Transferencias: []service.TransferRequest{{TarefaID: uuid.New(), TarefaKey: "K-1", NovoResponsavelID: uuid.New()}},
	})
	req := httptest.NewRequest("POST", "/equalizer/"+sprintID.String()+"/apply", bytes.NewReader(body))
	req = addChiParam(req, "id", sprintID.String())
	w := httptest.NewRecorder()
	h.ApplyTransfers(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
