# Timeline Sprint Modal — Design Spec

## Goal

When clicking a sprint bar on the Sprints Timeline chart, open a modal showing:
1. A disclaimer listing team members absent (férias/folga) during that sprint
2. A pie chart breaking down hours by **Tipo de Demanda** (Iniciativa / Meta / Compromisso)
3. A pie chart breaking down hours by **Épico** (using the epic's `apelido`), with tasks having no parent epic grouped as "Outras Tarefas"

## Architecture

- **Backend**: new endpoint `GET /api/v1/sprints/{id}/timeline-detail?equipe_id=X` returning absent members and task-level data with classified tipo_demanda and parent epic apelido
- **Frontend**: click handler on timeline canvas → fetch endpoint → render modal with disclaimer + two pie charts using existing `drawPieChart()` and `aggregateTarefasByKey()`

## Backend

### New endpoint

`GET /api/v1/sprints/{id}/timeline-detail?equipe_id={uuid}`

**Response:**

```json
{
  "sprint_nome": "Varejo 20/07-31/07",
  "data_inicio": "2025-07-20",
  "data_fim": "2025-07-31",
  "ausentes": [
    {
      "nome": "Fulano da Silva",
      "tipo": "ferias",
      "data_inicio": "2025-07-20",
      "data_fim": "2025-07-31"
    }
  ],
  "tarefas": [
    {
      "numero_ticket": "TCDV-1234",
      "resumo": "Implementar login SSO",
      "tipo_demanda": "Iniciativa",
      "estimativa_tempo": 14400,
      "epico_apelido": "MIGRAÇÃO API"
    }
  ]
}
```

### Repository: new method `GetTimelineDetail`

Single query joining `tarefas` with parent epic for apelido:

```sql
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
```

- Filters by equipe members (same pattern as existing sprint queries)
- Excludes Épico/Projeto type tasks from the list (they are containers, not work items)
- Uses existing COALESCE classification logic from `equipe.go:185-191`

### Repository: absent members

Reuse existing `GetMembrosEquipeInfo` + `GetAusenciasNoPeriodo` — already available in sprint repository. The service joins them: for each member, check if any ausencia overlaps the sprint period.

### Service method: `GetTimelineDetail`

```go
type TimelineDetailResult struct {
    SprintNome string                    `json:"sprint_nome"`
    DataInicio string                    `json:"data_inicio"`
    DataFim    string                    `json:"data_fim"`
    Ausentes   []TimelineAusente         `json:"ausentes"`
    Tarefas    []TimelineDetailTarefa    `json:"tarefas"`
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
```

Steps:
1. Fetch sprint by ID (get nome, data_inicio, data_fim)
2. Fetch equipe member IDs + info via `GetMembrosEquipeInfo`
3. Fetch ausencias via `GetAusenciasNoPeriodo` overlapping sprint period
4. Fetch tarefas via new `GetTimelineDetailTarefas` query
5. Cross-reference members with ausencias: any member with overlapping ausencia → include in `Ausentes` list (ALL absent members, not just those with cards)

### Handler

Pattern follows existing sprint handlers in `handler/sprint.go`:
- Parse sprint ID from URL path
- Parse equipe_id from query param
- Call service method
- Return JSON response

### Route

Add to `main.go`:
```go
r.Get("/api/v1/sprints/{id}/timeline-detail", sprintHandler.GetTimelineDetail)
```

## Frontend

### Click handler

Add `canvas.onclick` in `drawSprintsTimeline()`, after the existing `onmousemove` handler. Same hit detection logic as tooltip:

```js
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

### Modal HTML

Add new modal element using existing `pie-modal` class (780px width):

```html
<div class="modal-overlay" id="timeline-detail-modal" onclick="if(event.target===this)closeTimelineDetailModal()">
  <div class="modal pie-modal">
    <div class="modal-title" id="timeline-detail-title"></div>
    <div id="timeline-detail-content"></div>
    <div class="modal-actions">
      <button class="btn-cancel" type="button" onclick="closeTimelineDetailModal()">Fechar</button>
    </div>
  </div>
</div>
```

### Modal functions

**`openTimelineDetailModal(sprintId)`**:
1. Fetch `GET /api/v1/sprints/{sprintId}/timeline-detail?equipe_id={currentEquipeId}`
2. Set modal title: sprint name + dates
3. Render disclaimer if `ausentes.length > 0`
4. Render two pie charts side by side

**`closeTimelineDetailModal()`**: standard `classList.remove('open')`.

### Disclaimer rendering

Use existing `.unplanned-disclaimer` style:

```html
<div class="unplanned-disclaimer warn">
  <span class="unplanned-disclaimer-icon">⚠</span>
  <div><b>Fulano</b> (Férias: 20/07 - 31/07), <b>Ciclano</b> (Day Off: 25/07 - 25/07)</div>
</div>
```

Map `tipo` values to display names: `ferias` → "Férias", `dayoff` → "Day Off", `licenca_medica` → "Lic. Médica", `licenca_paternidade` → "Lic. Paternidade", `licenca_maternidade` → "Lic. Maternidade".

### Pie charts

Two charts side by side in a flex container (same layout as `renderDisclaimerPies`):

**Chart 1 — Tipo de Demanda:**
```js
var slicesTipo = aggregateTarefasByKey(data.tarefas, function(t) {
  return [t.tipo_demanda];
});
```

**Chart 2 — Por Épico:**
```js
var slicesEpico = aggregateTarefasByKey(data.tarefas, function(t) {
  return [t.epico_apelido || 'Outras Tarefas'];
});
```

Both use existing `drawPieChart()` — no new charting code needed.

### Hours conversion

`estimativa_tempo` comes in seconds from the backend. Existing `aggregateTarefasByKey` uses `t.estimativa_tempo` directly. The `drawPieChart` function expects `horas` field. Conversion: `estimativa_tempo / 3600` to get hours.

Either convert in aggregation callback or normalize data before passing.

## Scope exclusions

- No task list/table in the modal (just pie charts)
- No click-through from pie slices to task lists
- No editing from this modal
- No caching of the endpoint response
