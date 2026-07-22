package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type mockSkillStore struct {
	listFn              func(ctx context.Context, query string) ([]domain.Skill, error)
	getByIDFn           func(ctx context.Context, id uuid.UUID) (*domain.Skill, error)
	createFn            func(ctx context.Context, nome string) (*domain.Skill, error)
	deleteFn            func(ctx context.Context, id uuid.UUID) error
	getMembroSkillsFn   func(ctx context.Context, membroID uuid.UUID) ([]domain.Skill, error)
	addMembroSkillFn    func(ctx context.Context, membroID, skillID uuid.UUID) error
	removeMembroSkillFn func(ctx context.Context, membroID, skillID uuid.UUID) error
}

func (m *mockSkillStore) List(ctx context.Context, query string) ([]domain.Skill, error) {
	return m.listFn(ctx, query)
}
func (m *mockSkillStore) GetByID(ctx context.Context, id uuid.UUID) (*domain.Skill, error) {
	return m.getByIDFn(ctx, id)
}
func (m *mockSkillStore) Create(ctx context.Context, nome string) (*domain.Skill, error) {
	return m.createFn(ctx, nome)
}
func (m *mockSkillStore) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteFn(ctx, id)
}
func (m *mockSkillStore) GetMembroSkills(ctx context.Context, membroID uuid.UUID) ([]domain.Skill, error) {
	return m.getMembroSkillsFn(ctx, membroID)
}
func (m *mockSkillStore) AddMembroSkill(ctx context.Context, membroID, skillID uuid.UUID) error {
	return m.addMembroSkillFn(ctx, membroID, skillID)
}
func (m *mockSkillStore) RemoveMembroSkill(ctx context.Context, membroID, skillID uuid.UUID) error {
	return m.removeMembroSkillFn(ctx, membroID, skillID)
}

func newTestSkillHandler(store *mockSkillStore) *SkillHandler {
	return NewSkillHandler(store, zap.NewNop())
}

func TestSkillList_NoQuery(t *testing.T) {
	skills := []domain.Skill{{ID: uuid.New(), Nome: "golang"}, {ID: uuid.New(), Nome: "python"}}
	store := &mockSkillStore{
		listFn: func(_ context.Context, q string) ([]domain.Skill, error) {
			if q != "" {
				t.Errorf("expected empty query, got %q", q)
			}
			return skills, nil
		},
	}
	h := newTestSkillHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/skills", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var got []domain.Skill
	json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 2 {
		t.Errorf("got %d skills, want 2", len(got))
	}
}

func TestSkillList_WithQuery(t *testing.T) {
	store := &mockSkillStore{
		listFn: func(_ context.Context, q string) ([]domain.Skill, error) {
			if q != "go" {
				t.Errorf("expected query 'go', got %q", q)
			}
			return []domain.Skill{{ID: uuid.New(), Nome: "golang"}}, nil
		},
	}
	h := newTestSkillHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/skills?q=go", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var got []domain.Skill
	json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 1 {
		t.Errorf("got %d skills, want 1", len(got))
	}
}

func TestSkillCreate_Valid(t *testing.T) {
	created := domain.Skill{ID: uuid.New(), Nome: "golang"}
	store := &mockSkillStore{
		createFn: func(_ context.Context, nome string) (*domain.Skill, error) {
			if nome != "golang" {
				t.Errorf("expected nome 'golang', got %q", nome)
			}
			return &created, nil
		},
	}
	h := newTestSkillHandler(store)

	body := `{"nome":"  golang  "}`
	req := httptest.NewRequest("POST", "/api/v1/skills", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestSkillCreate_EmptyName(t *testing.T) {
	store := &mockSkillStore{}
	h := newTestSkillHandler(store)

	body := `{"nome":"   "}`
	req := httptest.NewRequest("POST", "/api/v1/skills", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestSkillCreate_TooLong(t *testing.T) {
	store := &mockSkillStore{}
	h := newTestSkillHandler(store)

	longName := make([]byte, 101)
	for i := range longName {
		longName[i] = 'a'
	}
	body := `{"nome":"` + string(longName) + `"}`
	req := httptest.NewRequest("POST", "/api/v1/skills", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestSkillDelete_Valid(t *testing.T) {
	skillID := uuid.New()
	store := &mockSkillStore{
		deleteFn: func(_ context.Context, id uuid.UUID) error {
			if id != skillID {
				t.Errorf("expected id %s, got %s", skillID, id)
			}
			return nil
		},
	}
	h := newTestSkillHandler(store)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", skillID.String())
	req := httptest.NewRequest("DELETE", "/api/v1/skills/"+skillID.String(), nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestSkillDelete_InvalidID(t *testing.T) {
	store := &mockSkillStore{}
	h := newTestSkillHandler(store)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "not-a-uuid")
	req := httptest.NewRequest("DELETE", "/api/v1/skills/not-a-uuid", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestSkillDelete_NotFound(t *testing.T) {
	store := &mockSkillStore{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return fmt.Errorf("skill not found")
		},
	}
	h := newTestSkillHandler(store)

	skillID := uuid.New()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", skillID.String())
	req := httptest.NewRequest("DELETE", "/api/v1/skills/"+skillID.String(), nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestGetMembroSkills(t *testing.T) {
	membroID := uuid.New()
	skills := []domain.Skill{{ID: uuid.New(), Nome: "golang"}}
	store := &mockSkillStore{
		getMembroSkillsFn: func(_ context.Context, id uuid.UUID) ([]domain.Skill, error) {
			if id != membroID {
				t.Errorf("expected membroID %s, got %s", membroID, id)
			}
			return skills, nil
		},
	}
	h := newTestSkillHandler(store)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", membroID.String())
	req := httptest.NewRequest("GET", "/api/v1/membros/"+membroID.String()+"/skills", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.GetMembroSkills(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var got []domain.Skill
	json.NewDecoder(rr.Body).Decode(&got)
	if len(got) != 1 || got[0].Nome != "golang" {
		t.Errorf("unexpected skills: %+v", got)
	}
}

func TestAddMembroSkill_Valid(t *testing.T) {
	membroID := uuid.New()
	skillID := uuid.New()
	store := &mockSkillStore{
		addMembroSkillFn: func(_ context.Context, mID, sID uuid.UUID) error {
			if mID != membroID || sID != skillID {
				t.Errorf("unexpected IDs: membro=%s skill=%s", mID, sID)
			}
			return nil
		},
	}
	h := newTestSkillHandler(store)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", membroID.String())
	body := `{"skill_id":"` + skillID.String() + `"}`
	req := httptest.NewRequest("POST", "/api/v1/membros/"+membroID.String()+"/skills", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.AddMembroSkill(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestAddMembroSkill_EmptySkillID(t *testing.T) {
	store := &mockSkillStore{}
	h := newTestSkillHandler(store)

	membroID := uuid.New()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", membroID.String())
	body := `{"skill_id":""}`
	req := httptest.NewRequest("POST", "/api/v1/membros/"+membroID.String()+"/skills", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.AddMembroSkill(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestRemoveMembroSkill_Valid(t *testing.T) {
	membroID := uuid.New()
	skillID := uuid.New()
	store := &mockSkillStore{
		removeMembroSkillFn: func(_ context.Context, mID, sID uuid.UUID) error {
			if mID != membroID || sID != skillID {
				t.Errorf("unexpected IDs")
			}
			return nil
		},
	}
	h := newTestSkillHandler(store)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", membroID.String())
	rctx.URLParams.Add("skillId", skillID.String())
	req := httptest.NewRequest("DELETE", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	h.RemoveMembroSkill(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}
