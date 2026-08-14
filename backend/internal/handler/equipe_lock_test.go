package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
)

// GetMembros não está nos exemplos do brief (só GetByID/List/Search de membro.go
// e o preview do import), mas o brief pede para aplicar a mesma regra ali —
// então a mesma cobertura de teste se aplica.
func TestGetMembrosEquipeTravadoNaoMandaSalario(t *testing.T) {
	equipeID := uuid.New()
	store := &mockEquipeStore{getMembrosEquipeFn: func(context.Context, uuid.UUID) ([]domain.Membro, error) {
		return []domain.Membro{membroComSalario()}, nil
	}}
	h := newTestEquipeHandler(store)

	w := httptest.NewRecorder()
	h.GetMembros(w, requestComContexto(http.MethodGet, "/equipes/x/membros", "", "id", equipeID.String(), false))

	if strings.Contains(w.Body.String(), "7777.77") || strings.Contains(w.Body.String(), "\"salario\"") {
		t.Errorf("membros da equipe travados vazaram salário: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Fulano") {
		t.Errorf("travar não pode esvaziar a lista de membros da equipe: %s", w.Body.String())
	}
}

func TestGetMembrosEquipeDestravadoMandaSalario(t *testing.T) {
	equipeID := uuid.New()
	store := &mockEquipeStore{getMembrosEquipeFn: func(context.Context, uuid.UUID) ([]domain.Membro, error) {
		return []domain.Membro{membroComSalario()}, nil
	}}
	h := newTestEquipeHandler(store)

	w := httptest.NewRecorder()
	h.GetMembros(w, requestComContexto(http.MethodGet, "/equipes/x/membros", "", "id", equipeID.String(), true))

	if !strings.Contains(w.Body.String(), "7777.77") {
		t.Errorf("destravado deveria mandar o salário: %s", w.Body.String())
	}
}
