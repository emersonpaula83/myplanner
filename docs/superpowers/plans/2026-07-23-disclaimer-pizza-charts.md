# Disclaimer Pizza Charts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add clickable disclaimers in sprint detail that open modals with SVG pie charts showing task distribution by product/type, with hover tooltips showing task details.

**Architecture:** New backend endpoint returns disclaimer tasks with relator and produtos. Frontend renders SVG pie charts in modals, with floating card tooltips on slice hover. Two modal variants: single pie for bugs/incidents, side-by-side pies for unplanned tasks.

**Tech Stack:** Go/pgx/chi (backend), vanilla JS/SVG (frontend)

## Global Constraints

- No external libraries — SVG pie charts drawn inline
- Follow existing modal pattern (`.modal-overlay` / `.modal` CSS classes)
- Follow existing repository → service → handler → route chain
- Reuse existing `naoPlanejadasFilter` and `manutencaoFilter` SQL patterns from `GetUnplannedStats`
- Dark/light theme support via CSS variables and `@media (prefers-color-scheme: dark)` + `:root[data-theme="dark"]`
- Pie proportions by hours (estimativa_tempo), not count
- Tasks with multiple componentes count in each produto's slice
- Tasks without componente grouped as "Sem componente"

---

### Task 1: Backend — Repository method for disclaimer tasks

**Files:**
- Modify: `backend/internal/repository/sprint.go` (add struct + method after line 619, near `GetEquipeNome`)

**Interfaces:**
- Consumes: existing `SprintRepository` struct with `pool *pgxpool.Pool`
- Produces: `DisclaimerTarefaRow` struct and `GetDisclaimerTasks(ctx, sprintID, equipeID, taskType) ([]DisclaimerTarefaRow, error)` method; `GetDisclaimerTarefaProdutos(ctx, tarefaIDs) (map[uuid.UUID][]string, error)` method

- [ ] **Step 1: Add `DisclaimerTarefaRow` struct**

Add after the `UnplannedStats` struct (around line 466) in `backend/internal/repository/sprint.go`:

```go
type DisclaimerTarefaRow struct {
	ID              uuid.UUID  `json:"id"`
	NumeroTicket    string     `json:"numero_ticket"`
	Resumo          string     `json:"resumo"`
	Tipo            string     `json:"tipo"`
	TipoDemanda     *string    `json:"tipo_demanda"`
	EstimativaTempo int64      `json:"estimativa_tempo"`
	RelatorNome     *string    `json:"relator_nome"`
}
```

- [ ] **Step 2: Add `GetDisclaimerTasks` method**

Add after `GetUnplannedStats` method in `backend/internal/repository/sprint.go`:

```go
func (r *SprintRepository) GetDisclaimerTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, taskType string) ([]DisclaimerTarefaRow, error) {
	naoPlanejadasFilter := `
		t.data_entrada_sprint > s.data_inicio
		OR (t.data_entrada_sprint IS NULL AND t.data_criacao > s.data_inicio)
	`
	manutencaoFilter := `LOWER(t.tipo) IN ('bug') OR LOWER(t.tipo) LIKE '%incidente%'`

	var typeFilter string
	if taskType == "manutencao" {
		typeFilter = fmt.Sprintf("AND (%s) AND (%s)", naoPlanejadasFilter, manutencaoFilter)
	} else {
		typeFilter = fmt.Sprintf("AND (%s) AND NOT (%s)", naoPlanejadasFilter, manutencaoFilter)
	}

	var equipeJoin, equipeWhere string
	args := []interface{}{sprintID}
	if equipeID != nil {
		equipeJoin = "INNER JOIN equipe_membros em ON em.membro_id = t.responsavel_id"
		equipeWhere = fmt.Sprintf("AND em.equipe_id = $%d", len(args)+1)
		args = append(args, *equipeID)
	}

	query := fmt.Sprintf(`
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.tipo_demanda,
		       COALESCE(t.estimativa_tempo, 0), m.nome
		FROM tarefas t
		INNER JOIN sprints s ON s.id = t.sprint_id
		LEFT JOIN membros m ON m.id = t.relator_id
		%s
		WHERE t.sprint_id = $1 AND t.responsavel_id IS NOT NULL
		  AND s.data_inicio IS NOT NULL
		  %s %s
		ORDER BY t.numero_ticket
	`, equipeJoin, typeFilter, equipeWhere)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("getting disclaimer tasks: %w", err)
	}
	defer rows.Close()

	var result []DisclaimerTarefaRow
	for rows.Next() {
		var t DisclaimerTarefaRow
		if err := rows.Scan(&t.ID, &t.NumeroTicket, &t.Resumo, &t.Tipo, &t.TipoDemanda,
			&t.EstimativaTempo, &t.RelatorNome); err != nil {
			return nil, fmt.Errorf("scanning disclaimer task: %w", err)
		}
		result = append(result, t)
	}
	return result, nil
}
```

- [ ] **Step 3: Add `GetDisclaimerTarefaProdutos` method**

Add right after `GetDisclaimerTasks` in `backend/internal/repository/sprint.go`:

```go
func (r *SprintRepository) GetDisclaimerTarefaProdutos(ctx context.Context, tarefaIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	if len(tarefaIDs) == 0 {
		return map[uuid.UUID][]string{}, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT tp.tarefa_id, p.nome
		FROM tarefa_produtos tp
		INNER JOIN produtos p ON p.id = tp.produto_id
		WHERE tp.tarefa_id = ANY($1)
		ORDER BY p.nome
	`, tarefaIDs)
	if err != nil {
		return nil, fmt.Errorf("getting disclaimer tarefa produtos: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]string)
	for rows.Next() {
		var tarefaID uuid.UUID
		var produtoNome string
		if err := rows.Scan(&tarefaID, &produtoNome); err != nil {
			return nil, fmt.Errorf("scanning tarefa produto: %w", err)
		}
		result[tarefaID] = append(result[tarefaID], produtoNome)
	}
	return result, nil
}
```

- [ ] **Step 4: Build and verify compilation**

Run:
```bash
cd /home/emerson/code/myplanner/backend && go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/repository/sprint.go
git commit -m "feat: add disclaimer tasks repository methods with relator and produtos"
```

---

### Task 2: Backend — Service + Handler + Route

**Files:**
- Modify: `backend/internal/service/sprint.go` (add response struct + method)
- Modify: `backend/internal/handler/sprint.go` (update `SprintStore` interface + add handler method)
- Modify: `backend/cmd/api/main.go` (add route)

**Interfaces:**
- Consumes: `SprintRepository.GetDisclaimerTasks(ctx, sprintID, equipeID, taskType) ([]DisclaimerTarefaRow, error)` and `SprintRepository.GetDisclaimerTarefaProdutos(ctx, tarefaIDs) (map[uuid.UUID][]string, error)` from Task 1
- Produces: `GET /sprints/{id}/disclaimer-tasks?type=manutencao|outras` endpoint returning `DisclaimerTasksResult`

- [ ] **Step 1: Add response struct in service**

Add in `backend/internal/service/sprint.go`, after the `UnplannedAnalysisResult` struct (around line 439):

```go
type DisclaimerTask struct {
	ID              uuid.UUID `json:"id"`
	NumeroTicket    string    `json:"numero_ticket"`
	Resumo          string    `json:"resumo"`
	Tipo            string    `json:"tipo"`
	TipoDemanda     *string   `json:"tipo_demanda"`
	EstimativaTempo int64     `json:"estimativa_tempo"`
	RelatorNome     *string   `json:"relator_nome"`
	Produtos        []string  `json:"produtos"`
}

type DisclaimerTasksResult struct {
	Tarefas []DisclaimerTask `json:"tarefas"`
}
```

- [ ] **Step 2: Add service method**

Add in `backend/internal/service/sprint.go`, after `GetUnplannedAnalysis`:

```go
func (s *SprintService) GetDisclaimerTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, taskType string) (*DisclaimerTasksResult, error) {
	rows, err := s.repo.GetDisclaimerTasks(ctx, sprintID, equipeID, taskType)
	if err != nil {
		return nil, err
	}

	tarefaIDs := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		tarefaIDs[i] = r.ID
	}

	produtosMap, err := s.repo.GetDisclaimerTarefaProdutos(ctx, tarefaIDs)
	if err != nil {
		s.logger.Warn("could not get disclaimer tarefa produtos", zap.Error(err))
		produtosMap = map[uuid.UUID][]string{}
	}

	tarefas := make([]DisclaimerTask, len(rows))
	for i, r := range rows {
		produtos := produtosMap[r.ID]
		if produtos == nil {
			produtos = []string{}
		}
		tarefas[i] = DisclaimerTask{
			ID:              r.ID,
			NumeroTicket:    r.NumeroTicket,
			Resumo:          r.Resumo,
			Tipo:            r.Tipo,
			TipoDemanda:     r.TipoDemanda,
			EstimativaTempo: r.EstimativaTempo,
			RelatorNome:     r.RelatorNome,
			Produtos:        produtos,
		}
	}

	return &DisclaimerTasksResult{Tarefas: tarefas}, nil
}
```

- [ ] **Step 3: Update handler `SprintStore` interface**

In `backend/internal/handler/sprint.go`, add to the `SprintStore` interface (line ~23, before closing `}`):

```go
GetDisclaimerTasks(ctx context.Context, sprintID uuid.UUID, equipeID *uuid.UUID, taskType string) (*service.DisclaimerTasksResult, error)
```

- [ ] **Step 4: Add handler method**

Add in `backend/internal/handler/sprint.go`, after `GetUnplanned`:

```go
func (h *SprintHandler) GetDisclaimerTasks(w http.ResponseWriter, r *http.Request) {
	sprintID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint id")
		return
	}

	taskType := r.URL.Query().Get("type")
	if taskType != "manutencao" && taskType != "outras" {
		respondError(w, http.StatusBadRequest, "type must be 'manutencao' or 'outras'")
		return
	}

	var equipeID *uuid.UUID
	if e := r.URL.Query().Get("equipe"); e != "" {
		id, err := uuid.Parse(e)
		if err == nil {
			equipeID = &id
		}
	}

	result, err := h.store.GetDisclaimerTasks(r.Context(), sprintID, equipeID, taskType)
	if err != nil {
		h.logger.Error("getting disclaimer tasks", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get disclaimer tasks")
		return
	}

	respondJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 5: Register route**

In `backend/cmd/api/main.go`, add after the burndown route (line ~242, after `r.Get("/sprints/{id}/burndown", ...)`):

```go
r.Get("/sprints/{id}/disclaimer-tasks", sprintHandler.GetDisclaimerTasks)
```

- [ ] **Step 6: Build and verify compilation**

Run:
```bash
cd /home/emerson/code/myplanner/backend && go build ./...
```

Expected: no errors.

- [ ] **Step 7: Manual test with curl**

Start server and test:
```bash
# Get a sprint ID from the database
# Then test both types:
curl -s "http://localhost:8080/api/v1/sprints/<SPRINT_ID>/disclaimer-tasks?type=manutencao" | jq .
curl -s "http://localhost:8080/api/v1/sprints/<SPRINT_ID>/disclaimer-tasks?type=outras" | jq .
# Verify: response contains tarefas array with numero_ticket, resumo, relator_nome, produtos fields
# Test validation:
curl -s "http://localhost:8080/api/v1/sprints/<SPRINT_ID>/disclaimer-tasks?type=invalid"
# Expected: 400 error
```

- [ ] **Step 8: Commit**

```bash
git add backend/internal/service/sprint.go backend/internal/handler/sprint.go backend/cmd/api/main.go
git commit -m "feat: add disclaimer tasks endpoint with relator and produtos"
```

---

### Task 3: Frontend — Modal structure, CSS, disclaimer onclick, API call

**Files:**
- Modify: `frontend/index.html` (CSS around line 460, modal HTML after line 930, JS functions)

**Interfaces:**
- Consumes: `GET /sprints/{id}/disclaimer-tasks?type=manutencao|outras` from Task 2
- Produces: `openDisclaimerModal(sprintID, type)` function, `closeDisclaimerModal()` function, modal HTML with `#disclaimer-modal`, pie container divs

- [ ] **Step 1: Add CSS for pie chart components**

In `frontend/index.html`, add after the `.unplanned-disclaimer.danger` dark theme rules (after line 460, before the next unrelated CSS block):

```css
.pie-modal { max-width: 780px; }
.pie-modal-content { display: flex; gap: 32px; justify-content: center; flex-wrap: wrap; margin-top: 16px; }
.pie-modal-content.single { justify-content: center; }
.pie-chart-wrap { display: flex; flex-direction: column; align-items: center; min-width: 300px; flex: 1; }
.pie-chart-wrap h4 { font-size: 13px; color: var(--text-secondary); margin: 0 0 12px 0; font-weight: 600; }
.pie-chart-svg { cursor: default; }
.pie-chart-svg path { transition: opacity .15s; }
.pie-chart-svg path:hover { opacity: .85; }
.pie-legend { margin-top: 12px; width: 100%; }
.pie-legend-item { display: flex; align-items: center; gap: 8px; padding: 3px 0; font-size: 12px; color: var(--text-primary); }
.pie-legend-color { width: 12px; height: 12px; border-radius: 3px; flex-shrink: 0; }
.pie-legend-label { flex: 1; }
.pie-legend-value { font-weight: 600; white-space: nowrap; }
.pie-tooltip-card { position: fixed; z-index: 1000; background: var(--surface); border: 1px solid var(--border); border-radius: 8px; padding: 10px 14px; box-shadow: 0 4px 16px rgba(0,0,0,.15); pointer-events: none; max-width: 420px; max-height: 300px; overflow-y: auto; }
.pie-tooltip-card table { border-collapse: collapse; font-size: 11px; width: 100%; }
.pie-tooltip-card th { text-align: left; padding: 2px 8px 4px 0; color: var(--text-secondary); font-weight: 600; border-bottom: 1px solid var(--border); }
.pie-tooltip-card td { padding: 3px 8px 3px 0; color: var(--text-primary); white-space: nowrap; }
.pie-tooltip-card td.resumo-col { white-space: normal; max-width: 200px; word-break: break-word; }
.pie-no-data { text-align: center; color: var(--text-secondary); padding: 40px 0; font-size: 13px; }
```

- [ ] **Step 2: Add modal HTML**

In `frontend/index.html`, add after the equalizer modal (after line 930, before the `<div class="gantt-tooltip">`):

```html
<div class="modal-overlay" id="disclaimer-modal" onclick="if(event.target===this)closeDisclaimerModal()">
  <div class="modal pie-modal">
    <div class="modal-title" id="disclaimer-modal-title"></div>
    <div id="disclaimer-modal-content"></div>
    <div class="modal-actions"><button class="btn-cancel" onclick="closeDisclaimerModal()">Fechar</button></div>
  </div>
</div>
<div class="pie-tooltip-card" id="pie-tooltip" style="display:none"></div>
```

- [ ] **Step 3: Add `openDisclaimerModal` and `closeDisclaimerModal` JS functions**

Add in the `<script>` section, near the other modal functions (after `closeEqualizerModal`):

```javascript
async function openDisclaimerModal(sprintID, taskType) {
  var modal = document.getElementById('disclaimer-modal');
  var titleEl = document.getElementById('disclaimer-modal-title');
  var content = document.getElementById('disclaimer-modal-content');

  titleEl.textContent = taskType === 'manutencao' ? '🔥 Bugs e Incidentes por Produto' : '⚡ Tarefas Não Planejadas';
  content.innerHTML = '<div class="pie-no-data">Carregando...</div>';
  modal.classList.add('open');

  var equipeVal = document.getElementById('sprints-equipe') ? document.getElementById('sprints-equipe').value : '';
  var qs = equipeVal ? '&equipe=' + equipeVal : '';
  try {
    var data = await api('/sprints/' + sprintID + '/disclaimer-tasks?type=' + taskType + qs);
    renderDisclaimerPies(content, data.tarefas, taskType);
  } catch (e) {
    content.innerHTML = '<div class="pie-no-data">Erro ao carregar dados</div>';
  }
}

function closeDisclaimerModal() {
  document.getElementById('disclaimer-modal').classList.remove('open');
  document.getElementById('pie-tooltip').style.display = 'none';
}
```

- [ ] **Step 4: Make disclaimers clickable**

Modify the disclaimer rendering in `openSprintCapacity` (lines ~1731-1748). Change the two disclaimer `<div>` elements to include `onclick`, `style="cursor:pointer"`, and store the sprintID.

The 🔥 disclaimer line (around line 1734) — change from:
```javascript
html += '<div class="unplanned-disclaimer danger"><span class="unplanned-disclaimer-icon">🔥</span><div><b>' + pctInc + '%</b> da capacity dedicado em <b>' + sa.manutencao_count + '</b> incidente(s)/bug(s) (' + sa.manutencao_horas.toFixed(1) + 'h)</div></div>';
```
to:
```javascript
html += '<div class="unplanned-disclaimer danger" style="cursor:pointer" onclick="openDisclaimerModal(\'' + sprintID + '\',\'manutencao\')"><span class="unplanned-disclaimer-icon">🔥</span><div><b>' + pctInc + '%</b> da capacity dedicado em <b>' + sa.manutencao_count + '</b> incidente(s)/bug(s) (' + sa.manutencao_horas.toFixed(1) + 'h)</div></div>';
```

The ⚡ disclaimer line (around line 1748) — change from:
```javascript
html += '<div class="unplanned-disclaimer warn"><span class="unplanned-disclaimer-icon">⚡</span><div><b>' + pctOutras + '%</b> da capacity dedicado em <b>' + sa.outras_count + '</b> tarefa(s) não planejada(s) (' + sa.outras_horas.toFixed(1) + 'h)</div></div>';
```
to:
```javascript
html += '<div class="unplanned-disclaimer warn" style="cursor:pointer" onclick="openDisclaimerModal(\'' + sprintID + '\',\'outras\')"><span class="unplanned-disclaimer-icon">⚡</span><div><b>' + pctOutras + '%</b> da capacity dedicado em <b>' + sa.outras_count + '</b> tarefa(s) não planejada(s) (' + sa.outras_horas.toFixed(1) + 'h)</div></div>';
```

- [ ] **Step 5: Add stub `renderDisclaimerPies` function**

Add after `openDisclaimerModal`:

```javascript
function renderDisclaimerPies(container, tarefas, taskType) {
  if (!tarefas || tarefas.length === 0) {
    container.innerHTML = '<div class="pie-no-data">Nenhuma tarefa encontrada</div>';
    return;
  }
  container.innerHTML = '<div class="pie-no-data">Gráfico será implementado na próxima etapa (' + tarefas.length + ' tarefas carregadas)</div>';
}
```

- [ ] **Step 6: Manual test**

Start server, open sprint detail in browser. Verify:
1. Disclaimers show pointer cursor on hover
2. Clicking 🔥 disclaimer opens modal titled "🔥 Bugs e Incidentes por Produto"
3. Clicking ⚡ disclaimer opens modal titled "⚡ Tarefas Não Planejadas"
4. Modal shows "X tarefas carregadas" message
5. Clicking outside modal or "Fechar" closes it
6. Escape key closes modal

- [ ] **Step 7: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add disclaimer modal structure with clickable disclaimers and API integration"
```

---

### Task 4: Frontend — SVG pie chart rendering with legend

**Files:**
- Modify: `frontend/index.html` (replace stub `renderDisclaimerPies`, add `drawPieChart` helper)

**Interfaces:**
- Consumes: `renderDisclaimerPies(container, tarefas, taskType)` stub from Task 3, `tarefas` array with `{estimativa_tempo, produtos, tipo_demanda}` fields
- Produces: `drawPieChart(container, slices, tarefas)` generic SVG pie rendering function, fully implemented `renderDisclaimerPies`

- [ ] **Step 1: Define color palette and aggregation helpers**

Add before `renderDisclaimerPies` in the `<script>` section:

```javascript
var PIE_COLORS = ['#0D7C66', '#5B7FBF', '#D4483B', '#C78D2E', '#8B5CF6', '#EC4899', '#06B6D4', '#84CC16', '#F97316', '#6366F1'];

function aggregateTarefasByKey(tarefas, keyFn) {
  var groups = {};
  tarefas.forEach(function(t) {
    var keys = keyFn(t);
    if (!keys || keys.length === 0) keys = ['Sem componente'];
    var horas = (t.estimativa_tempo || 0) / 3600;
    keys.forEach(function(k) {
      if (!groups[k]) groups[k] = { label: k, horas: 0, tarefas: [] };
      groups[k].horas += horas;
      if (groups[k].tarefas.indexOf(t) === -1) groups[k].tarefas.push(t);
    });
  });
  var arr = Object.values(groups).sort(function(a, b) { return b.horas - a.horas; });
  return arr;
}
```

- [ ] **Step 2: Implement `drawPieChart` SVG rendering**

Add after `aggregateTarefasByKey`:

```javascript
function drawPieChart(wrapEl, slices) {
  if (!slices || slices.length === 0) {
    wrapEl.innerHTML = '<div class="pie-no-data">Sem dados</div>';
    return;
  }

  var total = slices.reduce(function(s, d) { return s + d.horas; }, 0);
  if (total === 0) {
    wrapEl.innerHTML = '<div class="pie-no-data">Sem horas estimadas</div>';
    return;
  }

  var size = 200, cx = size / 2, cy = size / 2, r = 85;
  var svgNS = 'http://www.w3.org/2000/svg';
  var svg = document.createElementNS(svgNS, 'svg');
  svg.setAttribute('width', size);
  svg.setAttribute('height', size);
  svg.setAttribute('viewBox', '0 0 ' + size + ' ' + size);
  svg.classList.add('pie-chart-svg');

  var startAngle = -Math.PI / 2;

  slices.forEach(function(slice, i) {
    var pct = slice.horas / total;
    var angle = pct * 2 * Math.PI;
    var endAngle = startAngle + angle;

    var path = document.createElementNS(svgNS, 'path');
    var d;
    if (pct >= 0.999) {
      d = 'M ' + cx + ' ' + (cy - r) +
          ' A ' + r + ' ' + r + ' 0 1 1 ' + cx + ' ' + (cy + r) +
          ' A ' + r + ' ' + r + ' 0 1 1 ' + cx + ' ' + (cy - r) + ' Z';
    } else {
      var x1 = cx + r * Math.cos(startAngle);
      var y1 = cy + r * Math.sin(startAngle);
      var x2 = cx + r * Math.cos(endAngle);
      var y2 = cy + r * Math.sin(endAngle);
      var largeArc = angle > Math.PI ? 1 : 0;
      d = 'M ' + cx + ' ' + cy +
          ' L ' + x1.toFixed(2) + ' ' + y1.toFixed(2) +
          ' A ' + r + ' ' + r + ' 0 ' + largeArc + ' 1 ' + x2.toFixed(2) + ' ' + y2.toFixed(2) +
          ' Z';
    }

    path.setAttribute('d', d);
    path.setAttribute('fill', PIE_COLORS[i % PIE_COLORS.length]);
    path.setAttribute('data-slice-index', i);

    path.addEventListener('mousemove', function(e) {
      showPieTooltip(e, slice.tarefas, slice.label, slice.horas);
    });
    path.addEventListener('mouseleave', function() {
      document.getElementById('pie-tooltip').style.display = 'none';
    });

    svg.appendChild(path);
    startAngle = endAngle;
  });

  var centerText = document.createElementNS(svgNS, 'text');
  centerText.setAttribute('x', cx);
  centerText.setAttribute('y', cy - 6);
  centerText.setAttribute('text-anchor', 'middle');
  centerText.setAttribute('font-size', '16');
  centerText.setAttribute('font-weight', '700');
  centerText.setAttribute('fill', 'var(--text-primary)');
  centerText.textContent = total.toFixed(1) + 'h';
  svg.appendChild(centerText);

  var centerLabel = document.createElementNS(svgNS, 'text');
  centerLabel.setAttribute('x', cx);
  centerLabel.setAttribute('y', cy + 12);
  centerLabel.setAttribute('text-anchor', 'middle');
  centerLabel.setAttribute('font-size', '10');
  centerLabel.setAttribute('fill', 'var(--text-secondary)');
  centerLabel.textContent = 'total';
  svg.appendChild(centerLabel);

  wrapEl.appendChild(svg);

  var legend = document.createElement('div');
  legend.className = 'pie-legend';
  slices.forEach(function(slice, i) {
    var pct = (slice.horas / total * 100).toFixed(1);
    legend.innerHTML += '<div class="pie-legend-item">' +
      '<span class="pie-legend-color" style="background:' + PIE_COLORS[i % PIE_COLORS.length] + '"></span>' +
      '<span class="pie-legend-label">' + esc(slice.label) + '</span>' +
      '<span class="pie-legend-value">' + slice.horas.toFixed(1) + 'h (' + pct + '%)</span>' +
      '</div>';
  });
  wrapEl.appendChild(legend);
}
```

- [ ] **Step 3: Replace `renderDisclaimerPies` stub with full implementation**

Replace the stub `renderDisclaimerPies` function with:

```javascript
function renderDisclaimerPies(container, tarefas, taskType) {
  if (!tarefas || tarefas.length === 0) {
    container.innerHTML = '<div class="pie-no-data">Nenhuma tarefa encontrada</div>';
    return;
  }

  var contentDiv = document.createElement('div');
  contentDiv.className = 'pie-modal-content';

  if (taskType === 'manutencao') {
    contentDiv.classList.add('single');
    var wrap = document.createElement('div');
    wrap.className = 'pie-chart-wrap';
    wrap.innerHTML = '<h4>Por Produto</h4>';
    var slices = aggregateTarefasByKey(tarefas, function(t) { return t.produtos && t.produtos.length > 0 ? t.produtos : null; });
    drawPieChart(wrap, slices);
    contentDiv.appendChild(wrap);
  } else {
    var wrap1 = document.createElement('div');
    wrap1.className = 'pie-chart-wrap';
    wrap1.innerHTML = '<h4>Por Tipo de Demanda</h4>';
    var slicesTipo = aggregateTarefasByKey(tarefas, function(t) { return [t.tipo_demanda || 'Não classificado']; });
    drawPieChart(wrap1, slicesTipo);
    contentDiv.appendChild(wrap1);

    var wrap2 = document.createElement('div');
    wrap2.className = 'pie-chart-wrap';
    wrap2.innerHTML = '<h4>Por Produto</h4>';
    var slicesProd = aggregateTarefasByKey(tarefas, function(t) { return t.produtos && t.produtos.length > 0 ? t.produtos : null; });
    drawPieChart(wrap2, slicesProd);
    contentDiv.appendChild(wrap2);
  }

  container.innerHTML = '';
  container.appendChild(contentDiv);
}
```

- [ ] **Step 4: Add stub `showPieTooltip` function**

Add after `drawPieChart` (will be fully implemented in Task 5):

```javascript
function showPieTooltip(event, tarefas, label, horas) {
  var tip = document.getElementById('pie-tooltip');
  tip.textContent = label + ': ' + horas.toFixed(1) + 'h (' + tarefas.length + ' tarefas)';
  tip.style.display = 'block';
  tip.style.left = (event.clientX + 12) + 'px';
  tip.style.top = (event.clientY + 12) + 'px';
}
```

- [ ] **Step 5: Manual test**

Open sprint detail in browser. Click on disclaimers. Verify:
1. 🔥 modal shows single pie chart grouped by produto
2. ⚡ modal shows two pie charts side by side (Tipo de Demanda + Produto)
3. Slices are colored and proportional to hours
4. Legend shows product names with hours and percentages
5. Center shows total hours
6. Basic tooltip shows on hover (text only for now)
7. Works in both light and dark theme

- [ ] **Step 6: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add SVG pie chart rendering with legend for disclaimer modals"
```

---

### Task 5: Frontend — Rich tooltip card on slice hover

**Files:**
- Modify: `frontend/index.html` (replace stub `showPieTooltip` with full card rendering)

**Interfaces:**
- Consumes: `showPieTooltip(event, tarefas, label, horas)` called from `drawPieChart` slice mousemove event, `tarefas` array items with `{numero_ticket, resumo, relator_nome}` fields, `#pie-tooltip` div from Task 3
- Produces: Rich floating card with task table on pie slice hover

- [ ] **Step 1: Replace `showPieTooltip` with rich card implementation**

Replace the stub `showPieTooltip` function:

```javascript
function showPieTooltip(event, tarefas, label, horas) {
  var tip = document.getElementById('pie-tooltip');

  var html = '<table><thead><tr><th>Ticket</th><th>Descrição</th><th>Relator</th></tr></thead><tbody>';
  tarefas.forEach(function(t) {
    var resumo = t.resumo || '';
    if (resumo.length > 60) resumo = resumo.substring(0, 57) + '...';
    var relator = t.relator_nome || '—';
    html += '<tr><td>' + esc(t.numero_ticket) + '</td><td class="resumo-col">' + esc(resumo) + '</td><td>' + esc(relator) + '</td></tr>';
  });
  html += '</tbody></table>';

  tip.innerHTML = html;
  tip.style.display = 'block';

  var x = event.clientX + 16;
  var y = event.clientY + 16;
  var rect = tip.getBoundingClientRect();
  if (x + rect.width > window.innerWidth - 8) x = event.clientX - rect.width - 16;
  if (y + rect.height > window.innerHeight - 8) y = event.clientY - rect.height - 16;
  tip.style.left = x + 'px';
  tip.style.top = y + 'px';
}
```

- [ ] **Step 2: Ensure tooltip hides on modal close**

The `closeDisclaimerModal` function from Task 3 already hides the tooltip:
```javascript
document.getElementById('pie-tooltip').style.display = 'none';
```

Verify this is present. No additional code needed.

- [ ] **Step 3: Manual test**

Open sprint detail, click on a disclaimer. Verify:
1. Hovering on a pie slice shows a rich card near the cursor
2. Card has a table with columns: Ticket | Descrição | Relator
3. Long descriptions are truncated at ~60 chars
4. Card repositions if it would overflow the viewport
5. Card disappears when mouse leaves the slice
6. Card works in dark mode (background uses `--surface`, text uses `--text-primary`)
7. Card disappears when modal is closed

- [ ] **Step 4: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add rich tooltip card on pie chart slice hover"
```
