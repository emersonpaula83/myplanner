package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	mw "github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/google/uuid"
)

// resultadoComSalario simula um preview de import com uma linha casada e uma
// não casada, ambas carregando salário — inclusive no marcador "changes", que
// sozinho já denunciaria que o valor mudou.
func resultadoComSalario(valor float64) *domain.ImportMatchResult {
	return &domain.ImportMatchResult{
		Matched: []domain.ImportMatched{{
			Linha:      1,
			MembroNome: "Fulano",
			Dados:      domain.ImportDados{Salario: &valor},
			Changes:    []string{"salario", "cargo"},
		}},
		UnmatchedMembros: []domain.ImportUnmatchedMembro{{
			Linha:        2,
			NomePlanilha: "Ciclano",
			Dados:        domain.ImportDados{Salario: &valor},
		}},
	}
}

func requestImportComContexto(metodo, alvo, corpo string, destravado bool) *http.Request {
	req := httptest.NewRequest(metodo, alvo, bytes.NewBufferString(corpo))
	ctx := mw.ContextParaTeste(req.Context(), uuid.New(), "chefe@myplanner.local", "gerente", time.Now().Add(time.Hour))
	if destravado {
		ctx = mw.ContextDestravadoParaTeste(ctx)
	}
	return req.WithContext(ctx)
}

func TestSyncTravadoNaoMandaSalario(t *testing.T) {
	store := &mockImportStore{syncFn: func(context.Context) (*domain.ImportMatchResult, error) {
		return resultadoComSalario(8888.88), nil
	}}
	h := newTestImportHandler(store)

	w := httptest.NewRecorder()
	h.Sync(w, requestImportComContexto(http.MethodPost, "/investimentos/import/sync", "", false))

	corpo := w.Body.String()
	if strings.Contains(corpo, "8888.88") || strings.Contains(corpo, "\"salario\"") {
		t.Errorf("sync travado vazou salário: %s", corpo)
	}
	if !strings.Contains(corpo, "Fulano") || !strings.Contains(corpo, "Ciclano") {
		t.Errorf("travar não pode esvaziar o resto do preview: %s", corpo)
	}
	if !strings.Contains(corpo, "cargo") {
		t.Errorf("changes não deveria perder outros marcadores além de salario: %s", corpo)
	}
}

func TestSyncDestravadoMandaSalario(t *testing.T) {
	store := &mockImportStore{syncFn: func(context.Context) (*domain.ImportMatchResult, error) {
		return resultadoComSalario(8888.88), nil
	}}
	h := newTestImportHandler(store)

	w := httptest.NewRecorder()
	h.Sync(w, requestImportComContexto(http.MethodPost, "/investimentos/import/sync", "", true))

	corpo := w.Body.String()
	if !strings.Contains(corpo, "8888.88") {
		t.Errorf("destravado deveria mandar o salário: %s", corpo)
	}
	if !strings.Contains(corpo, "\"salario\"") {
		t.Errorf("destravado deveria manter o marcador salario em changes: %s", corpo)
	}
}

func TestImportTravadoNaoMandaSalario(t *testing.T) {
	store := &mockImportStore{
		fetchGoogleSheetCSVFn: func(context.Context, string) (string, string, string, error) {
			return "Nome\nFulano\n", "abc", "0", nil
		},
		matchPlanilhaFn: func(context.Context, string) (*domain.ImportMatchResult, error) {
			return resultadoComSalario(9999.99), nil
		},
	}
	h := newTestImportHandler(store)

	req := requestImportComContexto(http.MethodPost, "/investimentos/import",
		`{"sheets_url":"https://docs.google.com/spreadsheets/d/abc/edit"}`, false)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.Import(w, req)

	corpo := w.Body.String()
	if strings.Contains(corpo, "9999.99") || strings.Contains(corpo, "\"salario\"") {
		t.Errorf("import travado vazou salário: %s", corpo)
	}
}
