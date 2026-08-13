package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type RelatorioEsforcoStore interface {
	Get(ctx context.Context, f repository.RelatorioEsforcoFiltro) (*repository.RelatorioEsforco, error)
}

type RelatorioEsforcoHandler struct {
	store  RelatorioEsforcoStore
	logger *zap.Logger
}

func NewRelatorioEsforcoHandler(store RelatorioEsforcoStore, logger *zap.Logger) *RelatorioEsforcoHandler {
	return &RelatorioEsforcoHandler{store: store, logger: logger}
}

// ParseFiltroEsforco lê ano, trimestres e equipe_ids da query string. Sem ano ou
// trimestre, assume o trimestre corrente. equipe_ids vazio significa todas as
// equipes.
//
// Aceita `trimestres=1,3` (vários) e `trimestre=2` (um só, formato antigo).
func ParseFiltroEsforco(q url.Values, agora time.Time) (repository.RelatorioEsforcoFiltro, error) {
	f := repository.RelatorioEsforcoFiltro{
		Ano:        agora.Year(),
		Trimestres: []int{int(agora.Month()-1)/3 + 1},
	}

	if raw := strings.TrimSpace(q.Get("ano")); raw != "" {
		ano, err := strconv.Atoi(raw)
		if err != nil {
			return f, fmt.Errorf("ano inválido: %q", raw)
		}
		if ano < 2000 || ano > agora.Year()+1 {
			return f, fmt.Errorf("ano fora do intervalo suportado: %d", ano)
		}
		f.Ano = ano
	}

	bruto := q.Get("trimestres")
	if strings.TrimSpace(bruto) == "" {
		bruto = q.Get("trimestre")
	}
	if strings.TrimSpace(bruto) != "" {
		vistos := map[int]bool{}
		trimestres := []int{}
		for _, item := range strings.Split(bruto, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			tri, err := strconv.Atoi(item)
			if err != nil {
				return f, fmt.Errorf("trimestre inválido: %q", item)
			}
			if tri < 1 || tri > 4 {
				return f, fmt.Errorf("trimestre deve estar entre 1 e 4, recebido %d", tri)
			}
			if vistos[tri] {
				continue
			}
			vistos[tri] = true
			trimestres = append(trimestres, tri)
		}
		if len(trimestres) == 0 {
			return f, fmt.Errorf("informe ao menos um trimestre")
		}
		sort.Ints(trimestres)
		f.Trimestres = trimestres
	}

	f.EquipeIDs = []uuid.UUID{}
	for _, raw := range strings.Split(q.Get("equipe_ids"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return f, fmt.Errorf("equipe_id inválido: %q", raw)
		}
		f.EquipeIDs = append(f.EquipeIDs, id)
	}

	return f, nil
}

func (h *RelatorioEsforcoHandler) Get(w http.ResponseWriter, r *http.Request) {
	filtro, err := ParseFiltroEsforco(r.URL.Query(), time.Now())
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	rel, err := h.store.Get(r.Context(), filtro)
	if err != nil {
		h.logger.Error("failed to build relatorio de esforço", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to build relatorio de esforço")
		return
	}

	respondJSON(w, http.StatusOK, rel)
}
