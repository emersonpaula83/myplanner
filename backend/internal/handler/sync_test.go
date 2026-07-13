package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"go.uber.org/zap"
)

type mockSyncStore struct {
	syncAllResult []domain.SyncLog
	syncOneResult *domain.SyncLog
	statusResult  *domain.SyncLog
	logsResult    []domain.SyncLog
	syncAllErr    error
	syncOneErr    error
}

func (m *mockSyncStore) SyncFonteDados(ctx context.Context, id uuid.UUID) (*domain.SyncLog, error) {
	return m.syncOneResult, m.syncOneErr
}
func (m *mockSyncStore) SyncAll(ctx context.Context) ([]domain.SyncLog, error) {
	return m.syncAllResult, m.syncAllErr
}
func (m *mockSyncStore) GetStatus(ctx context.Context, id uuid.UUID) (*domain.SyncLog, error) {
	return m.statusResult, nil
}
func (m *mockSyncStore) ListLogs(ctx context.Context, id uuid.UUID, limit int) ([]domain.SyncLog, error) {
	return m.logsResult, nil
}

func TestTriggerSync_All(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := &mockSyncStore{
		syncAllResult: []domain.SyncLog{
			{ID: uuid.New(), Status: "success"},
		},
	}
	h := NewSyncHandler(store, logger)

	req := httptest.NewRequest("POST", "/sync/trigger", nil)
	w := httptest.NewRecorder()
	h.TriggerSync(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	if result["message"] != "sincronização concluída" {
		t.Errorf("unexpected message: %v", result["message"])
	}
}

func TestTriggerSync_All_Error(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := &mockSyncStore{
		syncAllErr: context.DeadlineExceeded,
	}
	h := NewSyncHandler(store, logger)

	req := httptest.NewRequest("POST", "/sync/trigger", nil)
	w := httptest.NewRecorder()
	h.TriggerSync(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestTriggerSync_Single(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	fdID := uuid.New()
	store := &mockSyncStore{
		syncOneResult: &domain.SyncLog{ID: uuid.New(), FonteDadosID: fdID, Status: "success"},
	}
	h := NewSyncHandler(store, logger)

	req := httptest.NewRequest("POST", "/sync/trigger?fonte_dados_id="+fdID.String(), nil)
	w := httptest.NewRecorder()
	h.TriggerSync(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result domain.SyncLog
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.FonteDadosID != fdID {
		t.Errorf("expected fonte_dados_id %s, got %s", fdID, result.FonteDadosID)
	}
}

func TestTriggerSync_Single_InvalidID(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	h := NewSyncHandler(&mockSyncStore{}, logger)

	req := httptest.NewRequest("POST", "/sync/trigger?fonte_dados_id=not-a-uuid", nil)
	w := httptest.NewRecorder()
	h.TriggerSync(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTriggerSync_Single_Error(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := &mockSyncStore{
		syncOneErr: context.DeadlineExceeded,
	}
	h := NewSyncHandler(store, logger)

	req := httptest.NewRequest("POST", "/sync/trigger?fonte_dados_id="+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	h.TriggerSync(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestGetSyncStatus_NeverSynced(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := &mockSyncStore{statusResult: nil}
	h := NewSyncHandler(store, logger)

	req := httptest.NewRequest("GET", "/sync/status?fonte_dados_id="+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	h.GetSyncStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]string
	json.NewDecoder(w.Body).Decode(&result)
	if result["status"] != "never_synced" {
		t.Errorf("expected never_synced, got %s", result["status"])
	}
}

func TestGetSyncStatus_WithLog(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	now := time.Now()
	store := &mockSyncStore{
		statusResult: &domain.SyncLog{
			ID:           uuid.New(),
			Status:       "success",
			IniciadoEm:   now,
			FinalizadoEm: &now,
			TotalTarefas: 42,
		},
	}
	h := NewSyncHandler(store, logger)

	req := httptest.NewRequest("GET", "/sync/status?fonte_dados_id="+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	h.GetSyncStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result domain.SyncLog
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.TotalTarefas != 42 {
		t.Errorf("expected total_tarefas 42, got %d", result.TotalTarefas)
	}
}

func TestGetSyncStatus_MissingID(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	h := NewSyncHandler(&mockSyncStore{}, logger)

	req := httptest.NewRequest("GET", "/sync/status", nil)
	w := httptest.NewRecorder()
	h.GetSyncStatus(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGetSyncStatus_InvalidID(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	h := NewSyncHandler(&mockSyncStore{}, logger)

	req := httptest.NewRequest("GET", "/sync/status?fonte_dados_id=not-a-uuid", nil)
	w := httptest.NewRecorder()
	h.GetSyncStatus(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListSyncLogs_Success(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := &mockSyncStore{
		logsResult: []domain.SyncLog{
			{ID: uuid.New(), Status: "success"},
			{ID: uuid.New(), Status: "partial"},
		},
	}
	h := NewSyncHandler(store, logger)

	req := httptest.NewRequest("GET", "/sync/logs?fonte_dados_id="+uuid.New().String()+"&limit=5", nil)
	w := httptest.NewRecorder()
	h.ListSyncLogs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result []domain.SyncLog
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 logs, got %d", len(result))
	}
}

func TestListSyncLogs_MissingID(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	h := NewSyncHandler(&mockSyncStore{}, logger)

	req := httptest.NewRequest("GET", "/sync/logs", nil)
	w := httptest.NewRecorder()
	h.ListSyncLogs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListSyncLogs_EmptyResult(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := &mockSyncStore{logsResult: nil}
	h := NewSyncHandler(store, logger)

	req := httptest.NewRequest("GET", "/sync/logs?fonte_dados_id="+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	h.ListSyncLogs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result []domain.SyncLog
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result == nil {
		t.Errorf("expected empty slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 logs, got %d", len(result))
	}
}
