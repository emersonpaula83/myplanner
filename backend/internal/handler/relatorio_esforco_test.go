package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// 15/08/2026 cai no terceiro trimestre — usado como "agora" nos casos que
// exercitam o padrão.
var agoraQ3 = time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)

func TestParseFiltroEsforcoPadraoUsaTrimestreCorrente(t *testing.T) {
	casos := []struct {
		mes       time.Month
		trimestre int
	}{
		{time.January, 1}, {time.March, 1},
		{time.April, 2}, {time.June, 2},
		{time.July, 3}, {time.September, 3},
		{time.October, 4}, {time.December, 4},
	}

	for _, c := range casos {
		agora := time.Date(2026, c.mes, 10, 0, 0, 0, 0, time.UTC)
		f, err := ParseFiltroEsforco(url.Values{}, agora)
		if err != nil {
			t.Fatalf("mês %v: erro inesperado: %v", c.mes, err)
		}
		if len(f.Trimestres) != 1 || f.Trimestres[0] != c.trimestre {
			t.Errorf("mês %v: trimestres = %v, esperado [%d]", c.mes, f.Trimestres, c.trimestre)
		}
		if f.Ano != 2026 {
			t.Errorf("mês %v: ano = %d, esperado 2026", c.mes, f.Ano)
		}
	}
}

func TestParseFiltroEsforcoTrimestres(t *testing.T) {
	casos := []struct {
		nome     string
		query    url.Values
		esperado []int
	}{
		{"singular, formato antigo", url.Values{"trimestre": {"1"}}, []int{1}},
		{"lista", url.Values{"trimestres": {"1,3"}}, []int{1, 3}},
		{"ordena", url.Values{"trimestres": {"4,2,1"}}, []int{1, 2, 4}},
		{"remove duplicata", url.Values{"trimestres": {"2,2,3"}}, []int{2, 3}},
		{"ignora espaço e vírgula solta", url.Values{"trimestres": {" 2 , ,3,"}}, []int{2, 3}},
		{"plural vence singular", url.Values{"trimestres": {"1,2"}, "trimestre": {"4"}}, []int{1, 2}},
		{"plural vazio cai no singular", url.Values{"trimestres": {""}, "trimestre": {"4"}}, []int{4}},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			f, err := ParseFiltroEsforco(c.query, agoraQ3)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(f.Trimestres) != len(c.esperado) {
				t.Fatalf("trimestres = %v, esperado %v", f.Trimestres, c.esperado)
			}
			for i := range c.esperado {
				if f.Trimestres[i] != c.esperado[i] {
					t.Fatalf("trimestres = %v, esperado %v", f.Trimestres, c.esperado)
				}
			}
		})
	}
}

func TestParseFiltroEsforcoRejeitaValoresInvalidos(t *testing.T) {
	casos := []struct {
		nome  string
		query url.Values
	}{
		{"trimestre zero", url.Values{"trimestre": {"0"}}},
		{"trimestre cinco", url.Values{"trimestre": {"5"}}},
		{"trimestre texto", url.Values{"trimestre": {"Q1"}}},
		{"lista com valor fora de faixa", url.Values{"trimestres": {"1,9"}}},
		{"lista com texto", url.Values{"trimestres": {"1,dois"}}},
		{"lista só de vírgulas", url.Values{"trimestres": {",,"}}},
		{"ano texto", url.Values{"ano": {"ontem"}}},
		{"ano antigo demais", url.Values{"ano": {"1999"}}},
		{"ano futuro demais", url.Values{"ano": {"2030"}}},
		{"equipe_id não é uuid", url.Values{"equipe_ids": {"time-devops"}}},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if _, err := ParseFiltroEsforco(c.query, agoraQ3); err == nil {
				t.Errorf("esperava erro para %v, não veio nenhum", c.query)
			}
		})
	}
}

func TestParseFiltroEsforcoEquipeIDs(t *testing.T) {
	a := uuid.New()
	b := uuid.New()

	t.Run("vazio significa todas as equipes", func(t *testing.T) {
		f, err := ParseFiltroEsforco(url.Values{}, agoraQ3)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if f.EquipeIDs == nil {
			t.Fatal("EquipeIDs = nil, esperado slice vazio (o SQL usa cardinality)")
		}
		if len(f.EquipeIDs) != 0 {
			t.Errorf("EquipeIDs = %v, esperado vazio", f.EquipeIDs)
		}
	})

	t.Run("lista separada por vírgula", func(t *testing.T) {
		q := url.Values{"equipe_ids": {a.String() + "," + b.String()}}
		f, err := ParseFiltroEsforco(q, agoraQ3)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if len(f.EquipeIDs) != 2 || f.EquipeIDs[0] != a || f.EquipeIDs[1] != b {
			t.Errorf("EquipeIDs = %v, esperado [%v %v]", f.EquipeIDs, a, b)
		}
	})

	t.Run("ignora entradas vazias", func(t *testing.T) {
		q := url.Values{"equipe_ids": {"," + a.String() + ", ,"}}
		f, err := ParseFiltroEsforco(q, agoraQ3)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if len(f.EquipeIDs) != 1 || f.EquipeIDs[0] != a {
			t.Errorf("EquipeIDs = %v, esperado [%v]", f.EquipeIDs, a)
		}
	})
}

type fakeRelatorioEsforcoStore struct {
	recebido repository.RelatorioEsforcoFiltro
	rel      *repository.RelatorioEsforco
	err      error
}

func (f *fakeRelatorioEsforcoStore) Get(_ context.Context, filtro repository.RelatorioEsforcoFiltro) (*repository.RelatorioEsforco, error) {
	f.recebido = filtro
	return f.rel, f.err
}

func TestRelatorioEsforcoHandlerRepassaFiltro(t *testing.T) {
	equipe := uuid.New()
	store := &fakeRelatorioEsforcoStore{rel: &repository.RelatorioEsforco{Ano: 2026, Trimestres: []int{1, 2}}}
	h := NewRelatorioEsforcoHandler(store, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/relatorios/esforco?ano=2026&trimestres=2,1&equipe_ids="+equipe.String(), nil)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200. corpo: %s", rec.Code, rec.Body.String())
	}
	if store.recebido.Ano != 2026 {
		t.Errorf("ano = %d, esperado 2026", store.recebido.Ano)
	}
	if len(store.recebido.Trimestres) != 2 || store.recebido.Trimestres[0] != 1 || store.recebido.Trimestres[1] != 2 {
		t.Errorf("trimestres = %v, esperado [1 2]", store.recebido.Trimestres)
	}
	if len(store.recebido.EquipeIDs) != 1 || store.recebido.EquipeIDs[0] != equipe {
		t.Errorf("EquipeIDs = %v, esperado [%v]", store.recebido.EquipeIDs, equipe)
	}

	var body repository.RelatorioEsforco
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("resposta não é JSON válido: %v", err)
	}
	if len(body.Trimestres) != 2 {
		t.Errorf("trimestres na resposta = %v, esperado 2 itens", body.Trimestres)
	}
}

func TestRelatorioEsforcoHandlerFiltroInvalidoDa400(t *testing.T) {
	store := &fakeRelatorioEsforcoStore{}
	h := NewRelatorioEsforcoHandler(store, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/relatorios/esforco?trimestre=9", nil)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", rec.Code)
	}
}
