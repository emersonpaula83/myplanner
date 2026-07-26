# Timeline Sprint Modal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Clicking a sprint bar on the Sprints Timeline opens a modal showing absent members disclaimer + two pie charts (by Tipo de Demanda and by Épico apelido).

**Architecture:** New backend endpoint `GET /sprints/{id}/timeline-detail?equipe=X` returns absent members and tasks with classified tipo_demanda + parent epic apelido. Frontend adds click handler on timeline canvas, opens modal reusing existing `drawPieChart()` and disclaimer patterns.

**Tech Stack:** Go (pgx/v5, chi, zap), vanilla JS frontend (single-file SPA)

## Global Constraints

- Follow existing handler → service → repository pattern in `backend/internal/`
- Reuse existing `drawPieChart()`, `aggregateTarefasByKey()` in frontend — no new chart code
- Reuse existing `.pie-modal`, `.unplanned-disclaimer` CSS classes
- COALESCE logic for tipo_demanda classification must match `equipe.go:185-191` exactly:
  ```sql
  COALESCE(tipo_demanda,
      CASE
          WHEN tipo IN ('Épico', 'Projeto') THEN 'Meta'
          WHEN tipo IN ('Spike', 'Implantação', 'Aditivo - Delivery') THEN 'Compromisso'
          ELSE 'Iniciativa'
      END
  )
  ```
- Absence types display mapping: `ferias` → "Férias", `dayoff` → "Day Off", `licenca_medica` → "Lic. Médica", `licenca_paternidade` → "Lic. Paternidade", `licenca_maternidade` → "Lic. Maternidade"
- Tasks of type `Épico` or `Projeto` are excluded from pie charts (they are containers, not work items)
- Tasks without parent epic → group as "Outras Tarefas" in épico pie chart
- `estimativa_tempo` is stored in seconds; frontend `aggregateTarefasByKey` divides by 3600 internally
- Equipe ID comes from `document.getElementById('stl-equipe').value` in frontend

---

### Task 1: Repository — GetTimelineDetailTarefas query

**Files:**
- Modify: `backend/internal/repository/sprint.go` (add struct + method after line 858)

**Interfaces:**
- Consumes: `SprintRepository.pool` (pgxpool), existing table schema (`tarefas`, `equipe_membros`)
- Produces: `TimelineDetailTarefa` struct and `GetTimelineDetailTarefas(ctx, sprintID uuid.UUID, equipeID uuid.UUID) ([]TimelineDetailTarefa, error)` — used by Task 2 service method

- [ ] **Step 1: Write the failing test**

Create test file:

```go
// backend/internal/repository/sprint_timeline_detail_test.go
package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestGetTimelineDetailTarefas_NoRows(t *testing.T) {
	pool := getTestPool(t)
	repo := NewSprintRepository(pool)

	result, err := repo.GetTimelineDetailTarefas(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/repository/ -run TestGetTimelineDetailTarefas -v -count=1`
Expected: FAIL — `GetTimelineDetailTarefas` not defined

- [ ] **Step 3: Add TimelineDetailTarefa struct and GetTimelineDetailTarefas method**

Add after the `GetBurndownTarefas` method (after line 858) in `backend/internal/repository/sprint.go`:

```go
type TimelineDetailTarefa struct {
	NumeroTicket    string  `json:"numero_ticket"`
	Resumo          string  `json:"resumo"`
	TipoDemanda     string  `json:"tipo_demanda"`
	EstimativaTempo int64   `json:"estimativa_tempo"`
	EpicoApelido    *string `json:"epico_apelido"`
}

func (r *SprintRepository) GetTimelineDetailTarefas(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID) ([]TimelineDetailTarefa, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			t.numero_ticket,
			t.resumo,
			COALESCE(t.tipo_demanda,
				CASE
					WHEN t.tipo IN ('Épico', 'Projeto') THEN 'Meta'
					WHEN t.tipo IN ('Spike', 'Implantação', 'Aditivo - Delivery') THEN 'Compromisso'
					ELSE 'Iniciativa'
				END
			) AS tipo_demanda,
			COALESCE(t.estimativa_tempo, 0) AS estimativa_tempo,
			parent.apelido AS epico_apelido
		FROM tarefas t
		LEFT JOIN tarefas parent ON t.parent_id = parent.id
		WHERE t.sprint_id = $1
		  AND t.responsavel_id IN (SELECT membro_id FROM equipe_membros WHERE equipe_id = $2)
		  AND t.tipo NOT IN ('Épico', 'Projeto')
		  AND t.status != 'Cancelado'
		ORDER BY t.numero_ticket
	`, sprintID, equipeID)
	if err != nil {
		return nil, fmt.Errorf("getting timeline detail tarefas: %w", err)
	}
	defer rows.Close()

	var result []TimelineDetailTarefa
	for rows.Next() {
		var td TimelineDetailTarefa
		if err := rows.Scan(&td.NumeroTicket, &td.Resumo, &td.TipoDemanda, &td.EstimativaTempo, &td.EpicoApelido); err != nil {
			return nil, fmt.Errorf("scanning timeline detail tarefa: %w", err)
		}
		result = append(result, td)
	}
	return result, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/repository/ -run TestGetTimelineDetailTarefas -v -count=1`
Expected: PASS

- [ ] **Step 5: Verify build**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: clean build

- [ ] **Step 6: Commit**

```bash
git add backend/internal/repository/sprint.go backend/internal/repository/sprint_timeline_detail_test.go
git commit -m "feat: add GetTimelineDetailTarefas repository method for timeline modal"
```

---

### Task 2: Service — GetTimelineDetail method

**Files:**
- Modify: `backend/internal/service/sprint.go` (add structs + method after `GetSprintsTimeline`)

**Interfaces:**
- Consumes: `SprintRepository.GetByID(ctx, sprintID) (*domain.Sprint, error)`, `SprintRepository.GetMembrosEquipeInfo(ctx, equipeID, dataFim) ([]MembroInfo, error)`, `SprintRepository.GetAusenciasNoPeriodo(ctx, membroIDs, inicio, fim) ([]AusenciaRecord, error)`, `SprintRepository.GetTimelineDetailTarefas(ctx, sprintID, equipeID) ([]TimelineDetailTarefa, error)` (from Task 1)
- Produces: `TimelineDetailResult` struct and `GetTimelineDetail(ctx, sprintID uuid.UUID, equipeID uuid.UUID) (*TimelineDetailResult, error)` — used by Task 3 handler

- [ ] **Step 1: Write the failing test**

```go
// backend/internal/service/sprint_timeline_detail_test.go
package service

import (
	"testing"
)

func TestTimelineDetailResult_Structure(t *testing.T) {
	r := TimelineDetailResult{
		SprintNome: "Sprint 1",
		DataInicio: "2025-07-20",
		DataFim:    "2025-07-31",
		Ausentes:   []TimelineAusente{},
		Tarefas:    []TimelineDetailTarefa{},
	}
	if r.SprintNome != "Sprint 1" {
		t.Error("unexpected sprint nome")
	}
	if r.DataInicio != "2025-07-20" {
		t.Error("unexpected data inicio")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/service/ -run TestTimelineDetailResult -v -count=1`
Expected: FAIL — `TimelineDetailResult` not defined

- [ ] **Step 3: Add TimelineDetailResult structs and GetTimelineDetail method**

Add after the `GetSprintsTimeline` method (around line 970) in `backend/internal/service/sprint.go`:

```go
type TimelineDetailResult struct {
	SprintNome string                      `json:"sprint_nome"`
	DataInicio string                      `json:"data_inicio"`
	DataFim    string                      `json:"data_fim"`
	Ausentes   []TimelineAusente           `json:"ausentes"`
	Tarefas    []TimelineDetailTarefa      `json:"tarefas"`
}

type TimelineAusente struct {
	Nome       string `json:"nome"`
	Tipo       string `json:"tipo"`
	DataInicio string `json:"data_inicio"`
	DataFim    string `json:"data_fim"`
}

type TimelineDetailTarefa struct {
	NumeroTicket    string  `json:"numero_ticket"`
	Resumo          string  `json:"resumo"`
	TipoDemanda     string  `json:"tipo_demanda"`
	EstimativaTempo int64   `json:"estimativa_tempo"`
	EpicoApelido    *string `json:"epico_apelido"`
}

func (s *SprintService) GetTimelineDetail(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID) (*TimelineDetailResult, error) {
	sprint, err := s.repo.GetByID(ctx, sprintID)
	if err != nil {
		return nil, fmt.Errorf("getting sprint: %w", err)
	}
	if sprint.DataInicio == nil || sprint.DataFim == nil {
		return nil, fmt.Errorf("sprint has no start/end date")
	}

	membros, err := s.repo.GetMembrosEquipeInfo(ctx, equipeID, *sprint.DataFim)
	if err != nil {
		return nil, fmt.Errorf("getting membros: %w", err)
	}

	membroIDs := make([]uuid.UUID, len(membros))
	membroNomes := make(map[uuid.UUID]string, len(membros))
	for i, m := range membros {
		membroIDs[i] = m.ID
		membroNomes[m.ID] = m.Nome
	}

	ausencias, err := s.repo.GetAusenciasNoPeriodo(ctx, membroIDs, *sprint.DataInicio, *sprint.DataFim)
	if err != nil {
		return nil, fmt.Errorf("getting ausencias: %w", err)
	}

	ausentes := make([]TimelineAusente, 0)
	seen := make(map[string]bool)
	for _, a := range ausencias {
		key := a.MembroID.String() + "|" + a.Tipo + "|" + a.DataInicio.Format("2006-01-02")
		if seen[key] {
			continue
		}
		seen[key] = true

		inicio := a.DataInicio
		if inicio.Before(*sprint.DataInicio) {
			inicio = *sprint.DataInicio
		}
		fim := a.DataFim
		if fim.After(*sprint.DataFim) {
			fim = *sprint.DataFim
		}

		ausentes = append(ausentes, TimelineAusente{
			Nome:       membroNomes[a.MembroID],
			Tipo:       a.Tipo,
			DataInicio: inicio.Format("2006-01-02"),
			DataFim:    fim.Format("2006-01-02"),
		})
	}

	repoTarefas, err := s.repo.GetTimelineDetailTarefas(ctx, sprintID, equipeID)
	if err != nil {
		return nil, fmt.Errorf("getting tarefas: %w", err)
	}

	tarefas := make([]TimelineDetailTarefa, len(repoTarefas))
	for i, t := range repoTarefas {
		tarefas[i] = TimelineDetailTarefa{
			NumeroTicket:    t.NumeroTicket,
			Resumo:          t.Resumo,
			TipoDemanda:     t.TipoDemanda,
			EstimativaTempo: t.EstimativaTempo,
			EpicoApelido:    t.EpicoApelido,
		}
	}

	return &TimelineDetailResult{
		SprintNome: sprint.Nome,
		DataInicio: sprint.DataInicio.Format("2006-01-02"),
		DataFim:    sprint.DataFim.Format("2006-01-02"),
		Ausentes:   ausentes,
		Tarefas:    tarefas,
	}, nil
}
```

- [ ] **Step 4: Add missing import**

The service file already imports `fmt` — but verify. If not present, add `"fmt"` to the import block at `backend/internal/service/sprint.go:3-11`.

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /home/emerson/code/myplanner/backend && go test ./internal/service/ -run TestTimelineDetailResult -v -count=1`
Expected: PASS

- [ ] **Step 6: Verify build**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: clean build

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/sprint.go backend/internal/service/sprint_timeline_detail_test.go
git commit -m "feat: add GetTimelineDetail service method"
```

---

### Task 3: Handler + route wiring

**Files:**
- Modify: `backend/internal/handler/sprint.go` (add `GetTimelineDetail` to SprintStore interface + handler method)
- Modify: `backend/cmd/api/main.go` (add route)

**Interfaces:**
- Consumes: `service.TimelineDetailResult` (from Task 2), `service.SprintService.GetTimelineDetail(ctx, sprintID, equipeID) (*TimelineDetailResult, error)`
- Produces: HTTP endpoint `GET /api/v1/sprints/{id}/timeline-detail?equipe=X` returning `TimelineDetailResult` JSON

- [ ] **Step 1: Add GetTimelineDetail to SprintStore interface**

In `backend/internal/handler/sprint.go`, add to the `SprintStore` interface (after line 24, before the closing `}`):

```go
	GetTimelineDetail(ctx context.Context, sprintID uuid.UUID, equipeID uuid.UUID) (*service.TimelineDetailResult, error)
```

- [ ] **Step 2: Add GetTimelineDetail handler method**

Add after the last handler method in `backend/internal/handler/sprint.go`:

```go
func (h *SprintHandler) GetTimelineDetail(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}

	equipeStr := r.URL.Query().Get("equipe")
	if equipeStr == "" {
		respondError(w, http.StatusBadRequest, "equipe parameter required")
		return
	}
	equipeID, err := uuid.Parse(equipeStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid equipe id")
		return
	}

	result, err := h.store.GetTimelineDetail(r.Context(), sprintID, equipeID)
	if err != nil {
		h.logger.Error("getting timeline detail", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get timeline detail")
		return
	}

	respondJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 3: Add route in main.go**

In `backend/cmd/api/main.go`, add after line 247 (after the `disclaimer-tasks` route):

```go
			r.Get("/sprints/{id}/timeline-detail", sprintHandler.GetTimelineDetail)
```

- [ ] **Step 4: Verify build**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: clean build

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/sprint.go backend/cmd/api/main.go
git commit -m "feat: add timeline-detail endpoint and route"
```

---

### Task 4: Frontend — modal HTML, click handler, and pie chart rendering

**Files:**
- Modify: `frontend/index.html` (add modal HTML, click handler on timeline canvas, modal open/close functions)

**Interfaces:**
- Consumes: `GET /api/v1/sprints/{id}/timeline-detail?equipe=X` (from Task 3), existing `drawPieChart(wrapEl, slices)`, existing `aggregateTarefasByKey(tarefas, keyFn)`, existing `esc(str)`, existing `fmtDateBR(dateStr)`, existing `api(url)` helper
- Produces: Clickable sprint bars in timeline → modal with disclaimer + two pie charts

- [ ] **Step 1: Add modal HTML**

In `frontend/index.html`, add after the `disclaimer-modal` block (after line 1069, before line 1070 `<div class="pie-tooltip-card"`):

```html
<div class="modal-overlay" id="timeline-detail-modal" onclick="if(event.target===this)closeTimelineDetailModal()">
  <div class="modal pie-modal">
    <div class="modal-title" id="timeline-detail-title"></div>
    <div id="timeline-detail-content"></div>
    <div class="modal-actions"><button class="btn-cancel" onclick="closeTimelineDetailModal()">Fechar</button></div>
  </div>
</div>
```

- [ ] **Step 2: Add click handler on timeline canvas**

In `frontend/index.html`, inside `drawSprintsTimeline()`, add after the `canvas.onmouseleave` handler (after line 4207, before the `canvas.oncontextmenu` handler):

```javascript
  canvas.onclick = function(e) {
    var r = canvas.getBoundingClientRect();
    var mx = e.clientX - r.left;
    var found = null;
    for (var i = 0; i < sprintRects.length; i++) {
      var sr = sprintRects[i];
      if (mx >= sr.x && mx <= sr.x + sr.w) { found = sr; break; }
    }
    if (!found) return;
    openTimelineDetailModal(found.d.sprint_id);
  };
```

- [ ] **Step 3: Add openTimelineDetailModal and closeTimelineDetailModal functions**

Add after the `renderDisclaimerPies` function (after line 4741, before `buildPieTooltipContent`):

```javascript
var AUSENCIA_LABELS = {
  'ferias': 'Férias', 'dayoff': 'Day Off', 'licenca_medica': 'Lic. Médica',
  'licenca_paternidade': 'Lic. Paternidade', 'licenca_maternidade': 'Lic. Maternidade',
  'desligamento': 'Desligamento'
};

async function openTimelineDetailModal(sprintId) {
  var modal = document.getElementById('timeline-detail-modal');
  var titleEl = document.getElementById('timeline-detail-title');
  var content = document.getElementById('timeline-detail-content');

  titleEl.textContent = 'Carregando...';
  content.innerHTML = '<div class="pie-no-data">Carregando...</div>';
  modal.classList.add('open');

  var equipe = document.getElementById('stl-equipe').value;
  if (!equipe) { content.innerHTML = '<div class="pie-no-data">Nenhuma equipe selecionada</div>'; return; }

  try {
    var data = await api('/sprints/' + sprintId + '/timeline-detail?equipe=' + equipe);

    titleEl.textContent = data.sprint_nome + ' (' + fmtDateBR(data.data_inicio) + ' — ' + fmtDateBR(data.data_fim) + ')';

    var html = '';

    if (data.ausentes && data.ausentes.length > 0) {
      var parts = data.ausentes.map(function(a) {
        var label = AUSENCIA_LABELS[a.tipo] || a.tipo;
        return '<b>' + esc(a.nome) + '</b> (' + label + ': ' + fmtDateBR(a.data_inicio) + ' — ' + fmtDateBR(a.data_fim) + ')';
      });
      html += '<div class="unplanned-disclaimer warn" style="margin-bottom:16px">';
      html += '<span class="unplanned-disclaimer-icon">⚠</span>';
      html += '<div>' + parts.join(', ') + ' — ausente(s) neste período.</div>';
      html += '</div>';
    }

    content.innerHTML = html;

    if (!data.tarefas || data.tarefas.length === 0) {
      content.innerHTML += '<div class="pie-no-data">Nenhuma tarefa encontrada nesta sprint</div>';
      return;
    }

    var contentDiv = document.createElement('div');
    contentDiv.className = 'pie-modal-content';

    var wrap1 = document.createElement('div');
    wrap1.className = 'pie-chart-wrap';
    wrap1.innerHTML = '<h4>Por Tipo de Demanda</h4>';
    var slicesTipo = aggregateTarefasByKey(data.tarefas, function(t) { return [t.tipo_demanda]; });
    drawPieChart(wrap1, slicesTipo);
    contentDiv.appendChild(wrap1);

    var wrap2 = document.createElement('div');
    wrap2.className = 'pie-chart-wrap';
    wrap2.innerHTML = '<h4>Por Épico</h4>';
    var slicesEpico = aggregateTarefasByKey(data.tarefas, function(t) { return [t.epico_apelido || 'Outras Tarefas']; });
    drawPieChart(wrap2, slicesEpico);
    contentDiv.appendChild(wrap2);

    content.appendChild(contentDiv);
  } catch (e) {
    content.innerHTML = '<div class="pie-no-data">Erro ao carregar dados: ' + esc(e.message || 'erro desconhecido') + '</div>';
  }
}

function closeTimelineDetailModal() {
  document.getElementById('timeline-detail-modal').classList.remove('open');
}
```

- [ ] **Step 4: Verify by running dev server**

Run: `cd /home/emerson/code/myplanner && ./dev.sh restart`

Test in browser:
1. Navigate to Sprints Timeline tab
2. Select an equipe
3. Click on a sprint bar → modal should open
4. Verify disclaimer shows absent members (if any)
5. Verify two pie charts render (Tipo de Demanda + Por Épico)
6. Click outside modal or "Fechar" → modal closes

- [ ] **Step 5: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add timeline sprint modal with disclaimer and pie charts"
```
