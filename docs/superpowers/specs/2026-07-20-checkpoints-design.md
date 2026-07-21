# Checkpoints na Timeline de Sprints — Design Spec

## Summary

Add "Checkpoints" feature to the Sprints Timeline page. Users create named markers with a date (or date range) that appear as vertical lines or striped bands on the timeline. Each checkpoint gets a random HSL-generated color with low opacity. Hover shows a tooltip with a colored dot and the checkpoint summary. Right-click on a checkpoint in the timeline to delete it.

## Data Model

### Table: `checkpoints` (migration `000010_checkpoints.up.sql`)

```sql
CREATE TABLE checkpoints (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    equipe_id   UUID REFERENCES equipes(id) ON DELETE CASCADE,
    nome        VARCHAR(15) NOT NULL,
    resumo      VARCHAR(50) NOT NULL,
    data_inicio DATE NOT NULL,
    data_fim    DATE,
    cor         VARCHAR(7) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_checkpoints_equipe ON checkpoints(equipe_id);
CREATE INDEX idx_checkpoints_data ON checkpoints(data_inicio);
```

- `equipe_id` nullable — `NULL` = global (visible to all teams)
- `data_fim` nullable — `NULL` = single point (vertical line), filled = period (striped band)
- `cor` = hex `#RRGGBB` generated server-side via HSL

## API

### POST /api/v1/checkpoints

**Request:**
```json
{
  "equipe_id": "uuid-or-null",
  "nome": "Deploy v2",
  "resumo": "Release principal do trimestre",
  "data_inicio": "2026-08-15",
  "data_fim": "2026-08-17"
}
```

**Response:** `201`
```json
{
  "id": "uuid",
  "equipe_id": "uuid-or-null",
  "nome": "Deploy v2",
  "resumo": "Release principal do trimestre",
  "data_inicio": "2026-08-15",
  "data_fim": "2026-08-17",
  "cor": "#4A90D9"
}
```

**Validation:**
- `nome`: required, max 15 chars → 400 `"Nome é obrigatório (máximo 15 caracteres)"`
- `resumo`: required, max 50 chars → 400 `"Resumo é obrigatório (máximo 50 caracteres)"`
- `data_inicio`: required, `YYYY-MM-DD` format → 400 `"Data de início é obrigatória"`
- `data_fim`: optional, must be >= `data_inicio` → 400 `"Data fim deve ser posterior à data de início"`
- `equipe_id`: optional (null = global)

### GET /api/v1/checkpoints?equipe_id={uuid}&ano={int}

Returns checkpoints for the team + global checkpoints, filtered by year. A checkpoint is included if its date range intersects the year (`data_inicio <= Dec 31` AND `data_fim >= Jan 1` or `data_fim IS NULL` and `data_inicio` falls within the year). If `ano` not provided, uses current year.

**Response:** `200`
```json
[
  {"id": "uuid", "equipe_id": null, "nome": "Code Freeze", "resumo": "Freeze para release", "data_inicio": "2026-09-01", "data_fim": null, "cor": "#D94A4A"},
  {"id": "uuid", "equipe_id": "uuid", "nome": "Deploy v2", "resumo": "Release principal", "data_inicio": "2026-08-15", "data_fim": "2026-08-17", "cor": "#4A90D9"}
]
```

No checkpoints → returns `[]`.

### DELETE /api/v1/checkpoints/{id}

**Response:** `204` no body.
- Invalid ID → 400
- Not found → 404

## Color Generation

Server-side HSL → hex conversion on INSERT:

```
hue:        0-360 (full spectrum)
saturation: 50-80% (vivid but not neon)
luminosity: 45-65% (readable in dark and light mode)
```

Opacity applied on the frontend via canvas `globalAlpha`, not stored in the color value.

## Frontend

### Button: "Criar Checkpoint"
- New button in the filter bar, next to "+ Gerar Sprints"
- Visible when an equipe is selected
- Opens inline popup (same pattern as Gerar Sprints form)
- Fields:
  - Nome: `input type="text"` maxlength=15
  - Resumo: `input type="text"` maxlength=50
  - Data Início: `input type="date"` (required)
  - Data Fim: `input type="date"` (optional)
  - Checkbox: "Global (todas equipes)"

### Timeline Rendering

**Single-point checkpoint (`data_fim` null):**
- Dashed vertical line from `pad.top` to `pad.top + ch` at `dateToX(data_inicio)`
- Checkpoint color with `globalAlpha = 0.4` (lower than HOJE line which uses full opacity)
- Label with name above the line: `ctx.fillText(nome, x, pad.top - 6)`

**Period checkpoint (`data_fim` filled):**
- Rectangular band from `dateToX(data_inicio)` to `dateToX(data_fim)`, full chart height
- Diagonal striped pattern (45°) via `createPattern` with auxiliary canvas
- Checkpoint color with `globalAlpha = 0.15` for the stripes
- Label with name centered at top of the band

### Tooltip (hover)
- Hit-test in `onmousemove`: `|mx - checkpoint.x| < 6px` (point) or `mx within range` (period)
- Shows: colored dot `●` with checkpoint color + name + summary + dates (dd/MM/yyyy)
- Hit-test priority: headcount dots > checkpoints > sprint bars

### Context Menu (right-click delete)
- `canvas.addEventListener('contextmenu', ...)` — detects click on checkpoint
- Shows custom div menu with "Excluir checkpoint"
- Confirms with `confirm('Excluir checkpoint "nome"?')`
- `DELETE /api/v1/checkpoints/{id}` → reload timeline

### Data Loading
- `loadSprintsTimeline()` fetches checkpoints alongside sprint data
- `GET /api/v1/checkpoints?equipe_id={id}&ano={ano}`
- Passes checkpoints to `drawSprintsTimeline(canvas, data, ano, checkpoints)`

## Backend Architecture

Follows the `feriado` pattern (handler + repository, no service layer):

| File | Action |
|------|--------|
| `backend/migrations/000010_checkpoints.up.sql` | Create table + indexes |
| `backend/migrations/000010_checkpoints.down.sql` | Drop table |
| `backend/internal/repository/checkpoint.go` | Repository: List, Create, Delete |
| `backend/internal/handler/checkpoint.go` | Handler: List, Create, Delete + validation + color generation |
| `backend/cmd/api/main.go` | Wire repo + handler, register routes |
| `frontend/index.html` | Button, form, rendering, tooltip, context menu |

## Files to Modify

| File | Action |
|------|--------|
| `backend/migrations/000010_checkpoints.up.sql` | Create — table definition |
| `backend/migrations/000010_checkpoints.down.sql` | Create — `DROP TABLE checkpoints` |
| `backend/internal/repository/checkpoint.go` | Create — CRUD repository |
| `backend/internal/handler/checkpoint.go` | Create — HTTP handlers + validation + color gen |
| `backend/cmd/api/main.go` | Modify — wire checkpoint repo/handler, register routes |
| `frontend/index.html` | Modify — button, form, canvas rendering, tooltip, context menu |
