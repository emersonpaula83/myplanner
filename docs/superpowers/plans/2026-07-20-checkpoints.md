# Checkpoints Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add checkpoint markers (single-point or period) to the Sprints Timeline, with backend CRUD, HSL color generation, canvas rendering (dashed lines + striped bands), hover tooltips, and right-click delete.

**Architecture:** PostgreSQL table + Go repository/handler (feriado pattern, no service layer) + vanilla JS frontend. Checkpoints can be per-equipe or global (null equipe_id). Colors generated server-side via HSL→hex. Canvas renders checkpoints as dashed lines (point) or striped bands (period) with low opacity.

**Tech Stack:** Go 1.22+, PostgreSQL (pgx), chi router, vanilla JS Canvas API

## Global Constraints

- `nome`: max 15 characters
- `resumo`: max 50 characters
- `cor`: hex `#RRGGBB`, HSL generated server-side (hue 0-360, sat 50-80%, lum 45-65%)
- Opacity applied on frontend via canvas `globalAlpha`, not stored in color
- Single-point: `globalAlpha = 0.4`; Period stripes: `globalAlpha = 0.15`
- `equipe_id` nullable — NULL = global (visible to all teams)
- `data_fim` nullable — NULL = single point (vertical line), filled = period (striped band)
- Date display format (UI): `dd/MM/yyyy`
- Year filter: checkpoint included if date range intersects the year

---

### Task 1: Database migration + Repository

**Files:**
- Create: `backend/migrations/000010_checkpoints.up.sql`
- Create: `backend/migrations/000010_checkpoints.down.sql`
- Create: `backend/internal/repository/checkpoint.go`

**Interfaces:**
- Consumes: `pgxpool.Pool` from existing infrastructure
- Produces:
  - `Checkpoint` struct with JSON tags
  - `CheckpointRepository` with `List(ctx, equipeID *uuid.UUID, ano int) ([]Checkpoint, error)`, `Create(ctx, equipeID *uuid.UUID, nome, resumo, dataInicio string, dataFim *string, cor string) (*Checkpoint, error)`, `Delete(ctx, id uuid.UUID) error`

- [ ] **Step 1: Create up migration**

Create `backend/migrations/000010_checkpoints.up.sql`:

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

- [ ] **Step 2: Create down migration**

Create `backend/migrations/000010_checkpoints.down.sql`:

```sql
DROP TABLE IF EXISTS checkpoints;
```

- [ ] **Step 3: Implement repository**

Create `backend/internal/repository/checkpoint.go`:

```go
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Checkpoint struct {
	ID         uuid.UUID  `json:"id"`
	EquipeID   *uuid.UUID `json:"equipe_id"`
	Nome       string     `json:"nome"`
	Resumo     string     `json:"resumo"`
	DataInicio string     `json:"data_inicio"`
	DataFim    *string    `json:"data_fim"`
	Cor        string     `json:"cor"`
}

type CheckpointRepository struct {
	pool *pgxpool.Pool
}

func NewCheckpointRepository(pool *pgxpool.Pool) *CheckpointRepository {
	return &CheckpointRepository{pool: pool}
}

func (r *CheckpointRepository) List(ctx context.Context, equipeID *uuid.UUID, ano int) ([]Checkpoint, error) {
	yearStart := time.Date(ano, 1, 1, 0, 0, 0, 0, time.UTC)
	yearEnd := time.Date(ano, 12, 31, 0, 0, 0, 0, time.UTC)

	rows, err := r.pool.Query(ctx, `
		SELECT id, equipe_id, nome, resumo, data_inicio, data_fim, cor
		FROM checkpoints
		WHERE (equipe_id = $1 OR equipe_id IS NULL)
		AND data_inicio <= $3
		AND (data_fim >= $2 OR (data_fim IS NULL AND data_inicio >= $2))
		ORDER BY data_inicio
	`, equipeID, yearStart, yearEnd)
	if err != nil {
		return nil, fmt.Errorf("listing checkpoints: %w", err)
	}
	defer rows.Close()

	result := make([]Checkpoint, 0)
	for rows.Next() {
		var c Checkpoint
		var di time.Time
		var df *time.Time
		if err := rows.Scan(&c.ID, &c.EquipeID, &c.Nome, &c.Resumo, &di, &df, &c.Cor); err != nil {
			return nil, fmt.Errorf("scanning checkpoint: %w", err)
		}
		c.DataInicio = di.Format("2006-01-02")
		if df != nil {
			s := df.Format("2006-01-02")
			c.DataFim = &s
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (r *CheckpointRepository) Create(ctx context.Context, equipeID *uuid.UUID, nome, resumo, dataInicio string, dataFim *string, cor string) (*Checkpoint, error) {
	di, err := time.Parse("2006-01-02", dataInicio)
	if err != nil {
		return nil, fmt.Errorf("parsing data_inicio: %w", err)
	}

	var df *time.Time
	if dataFim != nil && *dataFim != "" {
		t, err := time.Parse("2006-01-02", *dataFim)
		if err != nil {
			return nil, fmt.Errorf("parsing data_fim: %w", err)
		}
		df = &t
	}

	id := uuid.New()
	_, err = r.pool.Exec(ctx, `
		INSERT INTO checkpoints (id, equipe_id, nome, resumo, data_inicio, data_fim, cor)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, equipeID, nome, resumo, di, df, cor)
	if err != nil {
		return nil, fmt.Errorf("creating checkpoint: %w", err)
	}

	return &Checkpoint{
		ID: id, EquipeID: equipeID, Nome: nome, Resumo: resumo,
		DataInicio: dataInicio, DataFim: dataFim, Cor: cor,
	}, nil
}

func (r *CheckpointRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM checkpoints WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting checkpoint: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add backend/migrations/000010_checkpoints.up.sql backend/migrations/000010_checkpoints.down.sql backend/internal/repository/checkpoint.go
git commit -m "feat: add checkpoints table migration and repository"
```

---

### Task 2: Handler with color generation and route wiring

**Files:**
- Create: `backend/internal/handler/checkpoint.go`
- Modify: `backend/cmd/api/main.go`

**Interfaces:**
- Consumes: `CheckpointRepository` from Task 1 — `List(ctx, equipeID *uuid.UUID, ano int)`, `Create(ctx, equipeID *uuid.UUID, nome, resumo, dataInicio string, dataFim *string, cor string)`, `Delete(ctx, id uuid.UUID)`
- Produces: HTTP endpoints `GET /api/v1/checkpoints`, `POST /api/v1/checkpoints`, `DELETE /api/v1/checkpoints/{id}`

- [ ] **Step 1: Implement handler with color generation**

Create `backend/internal/handler/checkpoint.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/emersonpaula83/myplanner/backend/internal/repository"
	"go.uber.org/zap"
)

type CheckpointStore interface {
	List(ctx context.Context, equipeID *uuid.UUID, ano int) ([]repository.Checkpoint, error)
	Create(ctx context.Context, equipeID *uuid.UUID, nome, resumo, dataInicio string, dataFim *string, cor string) (*repository.Checkpoint, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type CheckpointHandler struct {
	store  CheckpointStore
	logger *zap.Logger
}

func NewCheckpointHandler(store CheckpointStore, logger *zap.Logger) *CheckpointHandler {
	return &CheckpointHandler{store: store, logger: logger}
}

func (h *CheckpointHandler) List(w http.ResponseWriter, r *http.Request) {
	equipeStr := r.URL.Query().Get("equipe_id")
	anoStr := r.URL.Query().Get("ano")

	var equipeID *uuid.UUID
	if equipeStr != "" {
		id, err := uuid.Parse(equipeStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "equipe_id inválido")
			return
		}
		equipeID = &id
	}

	ano := time.Now().Year()
	if anoStr != "" {
		var err error
		ano, err = strconv.Atoi(anoStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "ano inválido")
			return
		}
	}

	checkpoints, err := h.store.List(r.Context(), equipeID, ano)
	if err != nil {
		h.logger.Error("listing checkpoints", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar checkpoints")
		return
	}
	respondJSON(w, http.StatusOK, checkpoints)
}

func (h *CheckpointHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EquipeID  *uuid.UUID `json:"equipe_id"`
		Nome      string     `json:"nome"`
		Resumo    string     `json:"resumo"`
		DataInicio string    `json:"data_inicio"`
		DataFim   *string    `json:"data_fim"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	if req.Nome == "" || len(req.Nome) > 15 {
		respondError(w, http.StatusBadRequest, "Nome é obrigatório (máximo 15 caracteres)")
		return
	}
	if req.Resumo == "" || len(req.Resumo) > 50 {
		respondError(w, http.StatusBadRequest, "Resumo é obrigatório (máximo 50 caracteres)")
		return
	}
	if req.DataInicio == "" {
		respondError(w, http.StatusBadRequest, "Data de início é obrigatória")
		return
	}
	di, err := time.Parse("2006-01-02", req.DataInicio)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Data de início inválida")
		return
	}
	if req.DataFim != nil && *req.DataFim != "" {
		df, err := time.Parse("2006-01-02", *req.DataFim)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Data fim inválida")
			return
		}
		if df.Before(di) {
			respondError(w, http.StatusBadRequest, "Data fim deve ser posterior à data de início")
			return
		}
	}

	cor := generateCheckpointColor()

	cp, err := h.store.Create(r.Context(), req.EquipeID, req.Nome, req.Resumo, req.DataInicio, req.DataFim, cor)
	if err != nil {
		h.logger.Error("creating checkpoint", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao criar checkpoint")
		return
	}
	respondJSON(w, http.StatusCreated, cp)
}

func (h *CheckpointHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	if err := h.store.Delete(r.Context(), id); err != nil {
		h.logger.Error("deleting checkpoint", zap.Error(err))
		respondError(w, http.StatusNotFound, "checkpoint não encontrado")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func generateCheckpointColor() string {
	h := float64(rand.Intn(360))
	s := 50.0 + float64(rand.Intn(31))
	l := 45.0 + float64(rand.Intn(21))
	return hslToHex(h, s, l)
}

func hslToHex(h, s, l float64) string {
	s /= 100
	l /= 100
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}

	ri := int(math.Round((r + m) * 255))
	gi := int(math.Round((g + m) * 255))
	bi := int(math.Round((b + m) * 255))
	return fmt.Sprintf("#%02X%02X%02X", ri, gi, bi)
}
```

- [ ] **Step 2: Wire in main.go**

Add after the existing `feriadoRepo`/`feriadoHandler` lines (~line 96-97) in `backend/cmd/api/main.go`:

```go
checkpointRepo := repository.NewCheckpointRepository(pool)
checkpointHandler := handler.NewCheckpointHandler(checkpointRepo, logger)
```

Add routes inside the authenticated group, after feriado routes (~line 198):

```go
r.Get("/checkpoints", checkpointHandler.List)
r.Post("/checkpoints", checkpointHandler.Create)
r.Delete("/checkpoints/{id}", checkpointHandler.Delete)
```

- [ ] **Step 3: Verify compilation**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handler/checkpoint.go backend/cmd/api/main.go
git commit -m "feat: add checkpoint handler with HSL color generation and routes"
```

---

### Task 3: Frontend — button, creation form, data loading

**Files:**
- Modify: `frontend/index.html`

**Interfaces:**
- Consumes: `POST /api/v1/checkpoints` → `{id, equipe_id, nome, resumo, data_inicio, data_fim, cor}`, `GET /api/v1/checkpoints?equipe_id={uuid}&ano={int}` → `[{...}]`
- Produces: `stlCheckpoints` global variable, `openCheckpointForm()`, `criarCheckpoint()`, `cancelCheckpointForm()` functions. Modified `loadSprintsTimeline()` fetches checkpoints. Modified `drawSprintsTimeline()` signature adds `checkpoints` parameter.

- [ ] **Step 1: Add checkpoint button in HTML**

After the `stl-gen-btn` button (line 628), add:

```html
<button class="btn-sm" id="stl-cp-btn" style="display:none;font-size:12px;padding:4px 12px;background:var(--accent);color:#fff" onclick="openCheckpointForm()">+ Checkpoint</button>
```

- [ ] **Step 2: Add global variable and modify loadSprintsTimeline**

After the `let stlData = null;` line (line 2289), add:

```javascript
let stlCheckpoints = [];
```

In `loadSprintsTimeline()`, after the line `stlData = data;` (line 2319), add:

```javascript
try {
  stlCheckpoints = await api('/checkpoints?equipe_id=' + equipe + '&ano=' + stlAno);
  if (!stlCheckpoints) stlCheckpoints = [];
} catch(e) { stlCheckpoints = []; }
```

Update the `drawSprintsTimeline` call (line 2336) to pass checkpoints:

```javascript
drawSprintsTimeline(document.getElementById('stl-canvas'), data || [], stlAno, stlCheckpoints);
```

Show the checkpoint button when equipe is selected — after `genBtn.style.display = '';` (line 2314), add:

```javascript
document.getElementById('stl-cp-btn').style.display = '';
```

Hide the checkpoint button at start — after `genBtn.style.display = 'none';` (line 2307), add:

```javascript
document.getElementById('stl-cp-btn').style.display = 'none';
```

Update the resize handler (line 2296) to pass checkpoints:

```javascript
if (c && stlData !== null) drawSprintsTimeline(c, stlData, stlAno, stlCheckpoints);
```

- [ ] **Step 3: Add checkpoint form functions**

Add after the `cancelGerarSprints()` function (before `drawSprintsTimeline`):

```javascript
function openCheckpointForm() {
  var equipe = document.getElementById('stl-equipe').value;
  if (!equipe) { alert('Selecione uma equipe primeiro.'); return; }
  var el = document.getElementById('stl-content');
  var prevHTML = el.innerHTML;
  el.innerHTML = '<div style="background:var(--card-bg);border:1px solid var(--border-subtle);border-radius:8px;padding:20px;max-width:700px">'
    + '<h3 style="margin:0 0 16px">Criar Checkpoint</h3>'
    + '<div style="display:flex;gap:12px;align-items:flex-end;flex-wrap:wrap">'
    + '<div><label style="font-size:12px;color:var(--text-secondary)">Nome (máx 15)</label><br>'
    + '<input id="cp-nome" type="text" maxlength="15" placeholder="Ex: Deploy v2" style="padding:6px 10px;border:1px solid var(--border-subtle);border-radius:4px;background:var(--card-bg);color:var(--text-primary);font-size:13px;width:140px"></div>'
    + '<div><label style="font-size:12px;color:var(--text-secondary)">Resumo (máx 50)</label><br>'
    + '<input id="cp-resumo" type="text" maxlength="50" placeholder="Descrição breve" style="padding:6px 10px;border:1px solid var(--border-subtle);border-radius:4px;background:var(--card-bg);color:var(--text-primary);font-size:13px;width:220px"></div>'
    + '</div>'
    + '<div style="display:flex;gap:12px;align-items:flex-end;flex-wrap:wrap;margin-top:12px">'
    + '<div><label style="font-size:12px;color:var(--text-secondary)">Data Início</label><br>'
    + '<input id="cp-inicio" type="date" style="padding:6px 10px;border:1px solid var(--border-subtle);border-radius:4px;background:var(--card-bg);color:var(--text-primary);font-size:13px;width:150px"></div>'
    + '<div><label style="font-size:12px;color:var(--text-secondary)">Data Fim (opcional)</label><br>'
    + '<input id="cp-fim" type="date" style="padding:6px 10px;border:1px solid var(--border-subtle);border-radius:4px;background:var(--card-bg);color:var(--text-primary);font-size:13px;width:150px"></div>'
    + '<label style="font-size:12px;color:var(--text-secondary);display:flex;align-items:center;gap:4px"><input id="cp-global" type="checkbox"> Global (todas equipes)</label>'
    + '</div>'
    + '<div style="margin-top:16px;display:flex;gap:8px">'
    + '<button class="btn-sm" style="padding:6px 16px;font-size:13px;background:var(--accent);color:#fff" onclick="criarCheckpoint()">Criar</button>'
    + '<button class="btn-sm" style="padding:6px 16px;font-size:13px;background:var(--border-subtle)" onclick="cancelCheckpointForm()">Cancelar</button>'
    + '</div></div>';
  el.dataset.prevHtml = prevHTML;
}

function cancelCheckpointForm() {
  var el = document.getElementById('stl-content');
  if (el.dataset.prevHtml) {
    el.innerHTML = el.dataset.prevHtml;
    if (stlData !== null) {
      var c = document.getElementById('stl-canvas');
      if (c) drawSprintsTimeline(c, stlData, stlAno, stlCheckpoints);
    }
  } else {
    loadSprintsTimeline();
  }
}

async function criarCheckpoint() {
  var equipe = document.getElementById('stl-equipe').value;
  var nome = document.getElementById('cp-nome').value.trim();
  var resumo = document.getElementById('cp-resumo').value.trim();
  var inicio = document.getElementById('cp-inicio').value;
  var fim = document.getElementById('cp-fim').value || null;
  var global = document.getElementById('cp-global').checked;

  if (!nome) { alert('Informe o nome.'); return; }
  if (!resumo) { alert('Informe o resumo.'); return; }
  if (!inicio) { alert('Informe a data de início.'); return; }

  try {
    await api('/checkpoints', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        equipe_id: global ? null : equipe,
        nome: nome,
        resumo: resumo,
        data_inicio: inicio,
        data_fim: fim
      })
    });
    loadSprintsTimeline();
  } catch(err) {
    alert('Erro: ' + err.message);
  }
}
```

- [ ] **Step 4: Update drawSprintsTimeline signature**

Change the function signature (line 2469) from:

```javascript
function drawSprintsTimeline(canvas, data, ano) {
```

to:

```javascript
function drawSprintsTimeline(canvas, data, ano, checkpoints) {
  checkpoints = checkpoints || [];
```

- [ ] **Step 5: Verify page loads without JS errors**

Open `frontend/index.html` in browser. Select equipe. Verify checkpoint button appears and form opens/closes.

- [ ] **Step 6: Commit**

```bash
git add frontend/index.html
git commit -m "feat: checkpoint creation form, data loading, button wiring"
```

---

### Task 4: Frontend — canvas rendering (lines + striped bands + tooltips + right-click delete)

**Files:**
- Modify: `frontend/index.html`

**Interfaces:**
- Consumes: `checkpoints` array passed to `drawSprintsTimeline()`, each item: `{id, equipe_id, nome, resumo, data_inicio, data_fim, cor}`. `dateToX(date)` function already in scope. `api()` function for DELETE.
- Produces: Checkpoint lines/bands rendered on canvas, tooltip hit-test, right-click context menu for delete.

- [ ] **Step 1: Render checkpoint lines and bands**

Inside `drawSprintsTimeline`, after the "Today line" section (after line 2632, before "Sprint name labels"), add:

```javascript
  // Checkpoint lines and bands
  var cpHitAreas = [];
  checkpoints.forEach(function(cp) {
    var x1 = dateToX(cp.data_inicio);
    ctx.save();

    if (cp.data_fim) {
      // Period: striped band
      var x2 = dateToX(cp.data_fim);
      var bandW = x2 - x1;
      if (bandW < 2) bandW = 2;

      // Create diagonal stripe pattern
      var patCanvas = document.createElement('canvas');
      patCanvas.width = 10;
      patCanvas.height = 10;
      var patCtx = patCanvas.getContext('2d');
      patCtx.strokeStyle = cp.cor;
      patCtx.lineWidth = 3;
      patCtx.beginPath();
      patCtx.moveTo(0, 10);
      patCtx.lineTo(10, 0);
      patCtx.stroke();
      var pattern = ctx.createPattern(patCanvas, 'repeat');

      ctx.globalAlpha = 0.15;
      ctx.fillStyle = pattern;
      ctx.fillRect(x1, pad.top, bandW, ch);
      ctx.globalAlpha = 0.4;
      ctx.strokeStyle = cp.cor;
      ctx.lineWidth = 1;
      ctx.setLineDash([4, 3]);
      ctx.strokeRect(x1, pad.top, bandW, ch);
      ctx.setLineDash([]);
      ctx.globalAlpha = 1;

      // Label centered at top
      ctx.fillStyle = cp.cor;
      ctx.globalAlpha = 0.7;
      ctx.font = 'bold 9px system-ui';
      ctx.textAlign = 'center';
      ctx.fillText(cp.nome, x1 + bandW / 2, pad.top - 6);
      ctx.globalAlpha = 1;

      cpHitAreas.push({ x1: x1, x2: x1 + bandW, cp: cp });
    } else {
      // Single point: dashed vertical line
      ctx.globalAlpha = 0.4;
      ctx.strokeStyle = cp.cor;
      ctx.lineWidth = 2;
      ctx.setLineDash([5, 3]);
      ctx.beginPath();
      ctx.moveTo(x1, pad.top);
      ctx.lineTo(x1, pad.top + ch);
      ctx.stroke();
      ctx.setLineDash([]);

      // Label above
      ctx.fillStyle = cp.cor;
      ctx.globalAlpha = 0.7;
      ctx.font = 'bold 9px system-ui';
      ctx.textAlign = 'center';
      ctx.fillText(cp.nome, x1, pad.top - 6);
      ctx.globalAlpha = 1;

      cpHitAreas.push({ x1: x1 - 6, x2: x1 + 6, cp: cp });
    }

    ctx.restore();
  });
```

- [ ] **Step 2: Add checkpoint tooltip hit-test**

In the `canvas.onmousemove` handler, after the headcount dots check (after the `if (hcHit) { ... return; }` block, before the sprint bars check), add:

```javascript
    // Check checkpoint areas
    var cpHit = null;
    for (var ci = 0; ci < cpHitAreas.length; ci++) {
      if (mx >= cpHitAreas[ci].x1 && mx <= cpHitAreas[ci].x2) {
        cpHit = cpHitAreas[ci].cp;
        break;
      }
    }
    if (cpHit) {
      if (tip) {
        var cpDates = fmtDateBR(cpHit.data_inicio);
        if (cpHit.data_fim) cpDates += ' — ' + fmtDateBR(cpHit.data_fim);
        tip.innerHTML = '<span style="color:' + esc(cpHit.cor) + '">●</span> <b>' + esc(cpHit.nome) + '</b><br>'
          + esc(cpHit.resumo) + '<br>'
          + '<span style="font-size:11px;color:var(--text-secondary)">' + cpDates + '</span>';
        tip.style.display = 'block';
        tip.style.left = (e.clientX + 12) + 'px';
        tip.style.top = (e.clientY - 10) + 'px';
      }
      return;
    }
```

- [ ] **Step 3: Add right-click context menu for delete**

After the `canvas.onmouseleave` handler (after line 2730), add:

```javascript
  canvas.oncontextmenu = function(e) {
    e.preventDefault();
    var r = canvas.getBoundingClientRect();
    var mx = e.clientX - r.left;

    var cpHit = null;
    for (var ci = 0; ci < cpHitAreas.length; ci++) {
      if (mx >= cpHitAreas[ci].x1 && mx <= cpHitAreas[ci].x2) {
        cpHit = cpHitAreas[ci].cp;
        break;
      }
    }
    if (!cpHit) return;

    // Remove existing context menu if any
    var old = document.getElementById('cp-ctx-menu');
    if (old) old.remove();

    var menu = document.createElement('div');
    menu.id = 'cp-ctx-menu';
    menu.style.cssText = 'position:fixed;left:' + e.clientX + 'px;top:' + e.clientY + 'px;background:var(--card-bg);border:1px solid var(--border-subtle);border-radius:6px;padding:4px 0;z-index:300;box-shadow:0 4px 12px rgba(0,0,0,0.3);font-size:13px';
    var item = document.createElement('div');
    item.style.cssText = 'padding:6px 16px;cursor:pointer;color:var(--red)';
    item.textContent = 'Excluir "' + cpHit.nome + '"';
    item.onmouseenter = function() { this.style.background = 'var(--border-subtle)'; };
    item.onmouseleave = function() { this.style.background = 'none'; };
    item.onclick = function() {
      menu.remove();
      if (!confirm('Excluir checkpoint "' + cpHit.nome + '"?')) return;
      api('/checkpoints/' + cpHit.id, { method: 'DELETE' })
        .then(function() { loadSprintsTimeline(); })
        .catch(function(err) { alert('Erro: ' + err.message); });
    };
    menu.appendChild(item);
    document.body.appendChild(menu);

    var closeMenu = function() { menu.remove(); document.removeEventListener('click', closeMenu); };
    setTimeout(function() { document.addEventListener('click', closeMenu); }, 10);
  };
```

- [ ] **Step 4: Test in browser**

1. Create a single-point checkpoint → verify dashed line appears with label
2. Create a period checkpoint → verify striped band appears
3. Hover over checkpoint → verify tooltip with colored dot, name, summary, dates
4. Right-click checkpoint → verify context menu appears
5. Delete checkpoint → verify it disappears after reload

- [ ] **Step 5: Commit**

```bash
git add frontend/index.html
git commit -m "feat: checkpoint canvas rendering, tooltips, and right-click delete"
```

---

### Task 5: Final verification and cleanup

**Files:**
- No new files

**Interfaces:**
- Consumes: all previous tasks
- Produces: verified working feature

- [ ] **Step 1: Run backend build and tests**

Run: `cd /home/emerson/code/myplanner/backend && go build ./...`
Expected: no errors

Run: `cd /home/emerson/code/myplanner/backend && go vet ./...`
Expected: no issues

Run: `cd /home/emerson/code/myplanner/backend && go test ./... -v`
Expected: all PASS

- [ ] **Step 2: Verify migration file numbering**

Run: `ls /home/emerson/code/myplanner/backend/migrations/ | sort`
Expected: 000010 follows 000009 sequentially, no gaps or conflicts

- [ ] **Step 3: Test full flow in browser**

1. Select equipe → checkpoint button visible
2. Create single-point checkpoint → line on timeline
3. Create period checkpoint → striped band on timeline
4. Create global checkpoint → visible after switching equipes
5. Hover all checkpoints → tooltips correct
6. Right-click delete → checkpoint removed
7. Page reload → checkpoints persist

- [ ] **Step 4: Commit if any cleanup was needed**

Only commit if changes were made.
