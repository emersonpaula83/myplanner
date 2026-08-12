# Estrutura — Investimentos SP2 (Frontend) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Investimentos frontend page inside the new "Estrutura" sidebar group — team financial dashboard with summary cards, cost line chart, member table, and person detail modal.

**Architecture:** All changes in a single monolithic file `frontend/index.html` (~7500 lines). Vanilla JS/HTML/CSS with SVG custom charts. Uses existing `api()` fetch wrapper, `esc()`, `initials()`, `stringColor()` utilities. Backend SP1 endpoints already deployed and tested.

**Tech Stack:** Vanilla JavaScript, HTML5, CSS3, SVG for charts. No frameworks, no external dependencies.

## Global Constraints

- Frontend: vanilla JS/HTML/CSS — no frameworks, no external dependencies
- Gráficos: SVG custom (consistent with existing `drawPieChart` pattern at line 5836)
- Same access control as equipes — coordenador vê 1:N equipes
- Dark/light theme support via CSS variables (`var(--bg)`, `var(--surface)`, `var(--text-primary)`, etc.)
- Follow existing patterns: `.sidebar-group` for menu groups, `.modal-overlay.open` for modals, `.review-stat-card` for stat cards, `api()` for HTTP calls
- CSS insertion point: before `</style>` at line 945
- Page div insertion point: after `page-membro-detail` div (line 1142)
- Modal insertion point: after existing modals (around line 1390)
- JS insertion point: after existing functions at bottom of script

## Existing Patterns Reference

**Sidebar group** (example: Projetos group at line 987):
```html
<div class="sidebar-group" id="sidebar-group-projetos">
  <button class="sidebar-group-header" title="Projetos" onclick="toggleSidebarGroup('projetos')">
    <svg viewBox="0 0 24 24">...</svg>
    <span class="sidebar-item-label">Projetos</span>
    <span class="sidebar-arrow">▶</span>
  </button>
  <div class="sidebar-group-items">
    <button class="sidebar-item" data-page="projetos" title="Lista de Projetos" onclick="navigate('projetos')">
      <svg viewBox="0 0 24 24">...</svg>
      <span class="sidebar-item-label">Lista de Projetos</span>
    </button>
  </div>
</div>
```

**navigate() function** (line 2075): switches `.page.active`, highlights `.sidebar-item[data-page]`, opens sidebar groups.

**loadEquipes() fills array** (line 2107): populates multiple equipe `<select>` elements. New selects added here.

**Stat card pattern** (`.review-stat-card` at line 483):
```css
.review-stat-card { background: #fff; border: 1px solid #e0e0e0; border-radius: 10px; padding: 16px; text-align: center; }
.review-stat-card .stat-pct { font-size: 28px; font-weight: 700; color: #1976d2; }
.review-stat-card .stat-label { font-size: 11px; color: #666; text-transform: uppercase; }
```

**Modal pattern**: `<div class="modal-overlay" id="xxx-modal" onclick="if(event.target===this)closeXxxModal()">` + toggle `.open` class.

**Avatar pattern**: `stringColor(name)` for background hsl, `initials(name)` for fallback text, `<img>` for `avatar_url`.

**API endpoints** (all tested working):
- `GET /equipes/{id}/investimentos` → `{equipe, sumario, membros[]}`
- `GET /equipes/{id}/investimentos/gastos-mensais` → `{ano, meses[{mes, custo_total}]}`
- `GET /membros/{id}/salario/historico` → `[{valor, data_vigencia, created_at}]`
- `GET /membros/{id}/banco-horas/historico` → `[{valor, data_registro, created_at}]`
- `GET /membros/{id}/alocacoes-projetos` → `{projetos[{apelido, chave_jira, percentual_alocacao}]}`

---

### Task 1: Sidebar Restructuring + Page Scaffold + Navigation

**Files:**
- Modify: `frontend/index.html:983-986` (sidebar Equipes button → Estrutura group)
- Modify: `frontend/index.html:1142` (add page-investimentos div after page-membro-detail)
- Modify: `frontend/index.html:2075-2092` (navigate function)
- Modify: `frontend/index.html:2107` (loadEquipes fills array)

**Interfaces:**
- Consumes: existing `navigate()`, `toggleSidebarGroup()`, `loadEquipes()`
- Produces: `page-investimentos` div, `inv-equipe` select element, sidebar group `sidebar-group-estrutura`, navigation wiring for `investimentos` page

- [ ] **Step 1: Replace Equipes sidebar-item with Estrutura group**

Find the standalone Equipes button (line 983-986):
```html
    <button class="sidebar-item active" data-page="equipes" title="Equipes" onclick="navigate('equipes')">
      <svg viewBox="0 0 24 24"><circle cx="9" cy="7" r="3"/><circle cx="17" cy="7" r="2.5"/><path d="M2 21v-2a5 5 0 0 1 5-5h4a5 5 0 0 1 5 5v2"/><path d="M17 14a4 4 0 0 1 4 4v3"/></svg>
      <span class="sidebar-item-label">Equipes</span>
    </button>
```

Replace with this Estrutura group (starts `open` since Equipes is default page):
```html
    <div class="sidebar-group open" id="sidebar-group-estrutura">
      <button class="sidebar-group-header" title="Estrutura" onclick="toggleSidebarGroup('estrutura')">
        <svg viewBox="0 0 24 24"><rect x="9" y="2" width="6" height="4" rx="1"/><rect x="2" y="14" width="6" height="4" rx="1"/><rect x="16" y="14" width="6" height="4" rx="1"/><line x1="12" y1="6" x2="12" y2="10"/><line x1="5" y1="14" x2="5" y2="10"/><line x1="19" y1="14" x2="19" y2="10"/><line x1="5" y1="10" x2="19" y2="10"/></svg>
        <span class="sidebar-item-label">Estrutura</span>
        <span class="sidebar-arrow">▶</span>
      </button>
      <div class="sidebar-group-items">
        <button class="sidebar-item active" data-page="equipes" title="Equipes" onclick="navigate('equipes')">
          <svg viewBox="0 0 24 24"><circle cx="9" cy="7" r="3"/><circle cx="17" cy="7" r="2.5"/><path d="M2 21v-2a5 5 0 0 1 5-5h4a5 5 0 0 1 5 5v2"/><path d="M17 14a4 4 0 0 1 4 4v3"/></svg>
          <span class="sidebar-item-label">Equipes</span>
        </button>
        <button class="sidebar-item" data-page="investimentos" title="Investimentos" onclick="navigate('investimentos')">
          <svg viewBox="0 0 24 24"><line x1="12" y1="2" x2="12" y2="22"/><path d="M17 5H9.5a3.5 3.5 0 000 7h5a3.5 3.5 0 010 7H6"/></svg>
          <span class="sidebar-item-label">Investimentos</span>
        </button>
      </div>
    </div>
```

- [ ] **Step 2: Add page-investimentos div**

After the `page-membro-detail` closing `</div>` (line 1142), add:
```html
    <!-- INVESTIMENTOS -->
    <div class="page" id="page-investimentos">
      <div class="page-header-row">
        <h1 class="page-title">Investimentos</h1>
      </div>
      <div class="inv-filter-row">
        <select class="filter-select" id="inv-equipe" onchange="onInvEquipeChange()">
          <option value="">Selecione a equipe</option>
        </select>
        <div class="inv-avatar-row" id="inv-avatars"></div>
      </div>
      <div id="inv-content">
        <div class="empty-state"><div class="empty-state-text">Selecione uma equipe para visualizar os investimentos.</div></div>
      </div>
    </div>
```

- [ ] **Step 3: Add investimento modal overlay**

After the last existing modal overlay div (find the last `</div>` for a `.modal-overlay`, around line 1390), add:
```html
<div class="modal-overlay" id="investimento-modal" onclick="if(event.target===this)closeInvestimentoModal()">
  <div class="inv-modal-box">
    <div id="investimento-modal-content"><div class="loading"><div class="spinner"></div></div></div>
  </div>
</div>
```

- [ ] **Step 4: Update navigate() function**

In the `navigate()` function (line 2075), add the Estrutura group handling. After the existing `projGroup` block (line 2084), add:
```javascript
  var estruturaPages = ['equipes', 'investimentos'];
  var estruturaGroup = document.getElementById('sidebar-group-estrutura');
  if (estruturaGroup && estruturaPages.indexOf(page) >= 0) { estruturaGroup.classList.add('open'); }
```

And at the end of the function (before the closing `}`), add the load trigger:
```javascript
  if (page === 'investimentos') { /* page shown, data loads on equipe select */ }
```

Note: no data-load needed on navigate — data loads when equipe is selected via `onInvEquipeChange()`.

- [ ] **Step 5: Add inv-equipe to loadEquipes() fills**

In `loadEquipes()` (line 2107), add to the `fills` array:
```javascript
      {sel: document.getElementById('inv-equipe'), def: 'Selecione a equipe'},
```

The full `fills` array becomes:
```javascript
    const fills = [
      {sel: document.getElementById('equipe-select'), def: 'Todos os membros'},
      {sel: document.getElementById('sprints-equipe'), def: 'Todas as equipes'},
      {sel: document.getElementById('stl-equipe'), def: 'Selecione uma equipe'},
      {sel: document.getElementById('inv-equipe'), def: 'Selecione a equipe'}
    ];
```

- [ ] **Step 6: Verify in browser**

Open `http://localhost:3000` (or the static file URL). Verify:
1. Sidebar shows "Estrutura" group with org-chart icon
2. Estrutura group is open by default, showing "Equipes" (active) and "Investimentos"
3. Clicking "Investimentos" shows the page with equipe dropdown and empty state message
4. Equipe dropdown is populated with teams
5. Clicking "Equipes" still works — opens the existing equipes page
6. Sidebar collapsed state: Estrutura icon visible, children hidden

- [ ] **Step 7: Commit**

```bash
git add frontend/index.html
git commit -m "feat(frontend): restructure sidebar with Estrutura group, add investimentos page scaffold"
```

---

### Task 2: Investimentos CSS + Dashboard (Summary Cards + Avatars)

**Files:**
- Modify: `frontend/index.html:944` (add CSS before `</style>` at line 945)
- Modify: `frontend/index.html` (add JS functions at bottom of script)

**Interfaces:**
- Consumes: `api()`, `esc()`, `initials()`, `stringColor()`, `inv-equipe` select (Task 1), `inv-content` div (Task 1), `inv-avatars` div (Task 1)
- Produces: `onInvEquipeChange()`, `loadInvestimentos(equipeId)`, `formatSalarioBR(valor)`, `formatTempoCasa(meses)`, CSS classes `.inv-*`

- [ ] **Step 1: Add all investimentos CSS**

Before the closing `</style>` tag (line 945), insert this complete CSS block:
```css
/* === INVESTIMENTOS === */
.inv-filter-row { display: flex; align-items: center; gap: 16px; margin-bottom: 20px; flex-wrap: wrap; }
.inv-filter-row .filter-select { min-width: 220px; }
.inv-avatar-row { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
.inv-avatar { width: 34px; height: 34px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-weight: 600; font-size: 11px; color: #fff; background: var(--accent); cursor: pointer; overflow: hidden; transition: transform .15s, box-shadow .15s; border: 2px solid transparent; flex-shrink: 0; }
.inv-avatar:hover { transform: scale(1.15); box-shadow: 0 2px 8px rgba(0,0,0,.15); }
.inv-avatar img { width: 100%; height: 100%; object-fit: cover; }
.inv-team-header { margin-bottom: 20px; }
.inv-team-name { font-size: 26px; font-weight: 700; color: var(--accent); letter-spacing: -.5px; margin: 0; }
.inv-dashboard-grid { display: grid; grid-template-columns: 1fr 1.5fr; gap: 24px; margin-bottom: 28px; align-items: start; }
@media (max-width: 900px) { .inv-dashboard-grid { grid-template-columns: 1fr; } }
.inv-stat-cards { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.inv-stat-card { background: var(--surface); border: 1px solid var(--border); border-radius: 10px; padding: 16px; text-align: center; }
.inv-stat-card .inv-stat-value { font-size: 24px; font-weight: 700; color: var(--accent); font-variant-numeric: tabular-nums; margin-bottom: 4px; }
.inv-stat-card .inv-stat-label { font-size: 11px; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .4px; }
.inv-chart-wrap { background: var(--surface); border: 1px solid var(--border); border-radius: 10px; padding: 16px; min-height: 220px; }
.inv-chart-wrap h4 { font-size: 13px; font-weight: 600; color: var(--text-secondary); margin: 0 0 12px; text-transform: uppercase; letter-spacing: .3px; }
.inv-members-section { margin-top: 4px; }
.inv-members-section h3 { font-size: 16px; font-weight: 650; margin: 0 0 12px; color: var(--text-primary); }
.inv-table { width: 100%; border-collapse: collapse; }
.inv-table th { font-size: 11px; font-weight: 600; color: var(--text-tertiary); text-transform: uppercase; letter-spacing: .4px; padding: 8px 12px; text-align: left; border-bottom: 1px solid var(--border); }
.inv-table td { padding: 10px 12px; border-bottom: 1px solid var(--border-subtle); font-size: 13px; color: var(--text-primary); vertical-align: middle; }
.inv-table tr { cursor: pointer; transition: background .12s; }
.inv-table tbody tr:hover { background: var(--accent-soft); }
.inv-table .inv-member-cell { display: flex; align-items: center; gap: 10px; }
.inv-table .inv-member-avatar { width: 32px; height: 32px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-weight: 600; font-size: 11px; color: #fff; overflow: hidden; flex-shrink: 0; }
.inv-table .inv-member-avatar img { width: 100%; height: 100%; object-fit: cover; }
.inv-chip { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: 500; background: var(--chip-bg); color: var(--chip-text); margin: 1px 2px; }
.inv-salary-null { color: var(--text-tertiary); font-style: italic; }
.inv-modal-box { background: var(--modal-bg); border-radius: 14px; padding: 28px; width: 100%; max-width: 720px; max-height: 90vh; overflow-y: auto; box-shadow: 0 8px 32px rgba(0,0,0,.2); }
.inv-modal-header { display: flex; align-items: center; gap: 16px; margin-bottom: 24px; }
.inv-modal-avatar { width: 56px; height: 56px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-weight: 700; font-size: 20px; color: #fff; overflow: hidden; flex-shrink: 0; }
.inv-modal-avatar img { width: 100%; height: 100%; object-fit: cover; }
.inv-modal-info h2 { font-size: 20px; font-weight: 650; color: var(--text-primary); margin: 0 0 4px; }
.inv-modal-info p { font-size: 13px; color: var(--text-secondary); margin: 0; }
.inv-modal-close { margin-left: auto; background: none; border: none; font-size: 22px; cursor: pointer; color: var(--text-secondary); padding: 4px 8px; border-radius: 6px; }
.inv-modal-close:hover { background: var(--chip-bg); }
.inv-modal-section { margin-bottom: 24px; }
.inv-modal-section h3 { font-size: 14px; font-weight: 650; color: var(--text-primary); margin: 0 0 12px; }
.inv-alloc-item { display: flex; align-items: center; gap: 12px; margin-bottom: 10px; }
.inv-alloc-info { min-width: 140px; flex-shrink: 0; }
.inv-alloc-name { font-size: 13px; font-weight: 600; color: var(--text-primary); }
.inv-alloc-key { font-size: 11px; color: var(--text-tertiary); opacity: .6; }
.inv-alloc-bar-wrap { flex: 1; height: 20px; background: var(--member-bar-bg); border-radius: 4px; overflow: hidden; position: relative; }
.inv-alloc-bar { height: 100%; background: var(--accent); border-radius: 4px; transition: width .3s ease; min-width: 2px; }
.inv-alloc-pct { font-size: 12px; font-weight: 600; color: var(--text-secondary); min-width: 45px; text-align: right; font-variant-numeric: tabular-nums; }
.inv-modal-charts { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
@media (max-width: 600px) { .inv-modal-charts { grid-template-columns: 1fr; } }
.inv-modal-chart-box { background: var(--surface); border: 1px solid var(--border); border-radius: 8px; padding: 12px; }
.inv-modal-chart-box h4 { font-size: 12px; font-weight: 600; color: var(--text-secondary); margin: 0 0 8px; text-transform: uppercase; letter-spacing: .3px; }
```

- [ ] **Step 2: Add helper functions**

At the end of the `<script>` section (before `</script>`), add these helper functions:
```javascript
// === INVESTIMENTOS ===
let invCurrentEquipeId = null;
let invDashboardData = null;

function formatSalarioBR(valor) {
  if (valor == null) return '—';
  return 'R$ ' + valor.toLocaleString('pt-BR', {minimumFractionDigits: 0, maximumFractionDigits: 0});
}

function formatTempoCasa(meses) {
  if (!meses || meses <= 0) return '—';
  var anos = Math.floor(meses / 12);
  var m = meses % 12;
  if (anos > 0 && m > 0) return anos + 'a ' + m + 'm';
  if (anos > 0) return anos + 'a';
  return m + 'm';
}

function onInvEquipeChange() {
  var equipeId = document.getElementById('inv-equipe').value;
  if (!equipeId) {
    document.getElementById('inv-avatars').innerHTML = '';
    document.getElementById('inv-content').innerHTML = '<div class="empty-state"><div class="empty-state-text">Selecione uma equipe para visualizar os investimentos.</div></div>';
    return;
  }
  invCurrentEquipeId = equipeId;
  loadInvestimentos(equipeId);
}

async function loadInvestimentos(equipeId) {
  var content = document.getElementById('inv-content');
  content.innerHTML = '<div class="loading"><div class="spinner"></div></div>';
  try {
    var data = await api('/equipes/' + equipeId + '/investimentos');
    invDashboardData = data;
    renderInvestimentosDashboard(data);
  } catch (err) {
    content.innerHTML = '<div class="empty-state"><div class="empty-state-icon">⚠️</div><div class="empty-state-text">' + esc(err.message) + '</div></div>';
  }
}
```

- [ ] **Step 3: Add renderInvestimentosDashboard function**

Immediately after `loadInvestimentos`, add:
```javascript
function renderInvestimentosDashboard(data) {
  var eq = data.equipe;
  var sum = data.sumario;
  var membros = data.membros || [];

  // Avatars row
  var avatarsHtml = '';
  membros.forEach(function(m) {
    var av = m.avatar_url
      ? '<img src="' + esc(m.avatar_url) + '" alt="">'
      : initials(m.nome);
    avatarsHtml += '<div class="inv-avatar" title="' + esc(m.nome) + '" style="background:' + stringColor(m.nome) + '" onclick="scrollToInvMembro(\'' + m.id + '\')">' + av + '</div>';
  });
  document.getElementById('inv-avatars').innerHTML = avatarsHtml;

  // Build page content
  var html = '<div class="fade-in">';

  // Team header
  html += '<div class="inv-team-header"><h2 class="inv-team-name">' + esc(eq.nome) + '</h2></div>';

  // Dashboard grid: stat cards (left) + chart (right)
  html += '<div class="inv-dashboard-grid">';

  // Stat cards
  html += '<div class="inv-stat-cards">';
  html += '<div class="inv-stat-card"><div class="inv-stat-value">' + formatSalarioBR(sum.custo_mensal_total) + '</div><div class="inv-stat-label">Investimento / Mês</div></div>';
  html += '<div class="inv-stat-card"><div class="inv-stat-value">' + sum.total_membros + '</div><div class="inv-stat-label">Membros Ativos</div></div>';
  html += '<div class="inv-stat-card"><div class="inv-stat-value">' + formatTempoCasa(sum.tempo_casa_medio_meses) + '</div><div class="inv-stat-label">Tempo Médio de Casa</div></div>';
  html += '<div class="inv-stat-card"><div class="inv-stat-value">' + (sum.banco_horas_total || 0).toLocaleString('pt-BR') + 'h</div><div class="inv-stat-label">Banco de Horas Total</div></div>';
  html += '</div>';

  // Chart placeholder (filled by Task 3)
  html += '<div class="inv-chart-wrap"><h4>Gastos Mensais ' + new Date().getFullYear() + '</h4><div id="inv-gastos-chart"></div></div>';

  html += '</div>'; // close dashboard-grid

  // Members table placeholder (filled by Task 4)
  html += '<div class="inv-members-section"><h3>Equipe</h3><div id="inv-members-table"></div></div>';

  html += '</div>'; // close fade-in

  document.getElementById('inv-content').innerHTML = html;

  // Load chart data (Task 3 will implement loadInvGastosMensais)
  if (typeof loadInvGastosMensais === 'function') loadInvGastosMensais(invCurrentEquipeId, sum.custo_mensal_total);

  // Render members table (Task 4 will implement renderInvMembrosTable)
  if (typeof renderInvMembrosTable === 'function') renderInvMembrosTable(membros);
}

function scrollToInvMembro(membroId) {
  var row = document.querySelector('.inv-table tr[data-membro-id="' + membroId + '"]');
  if (row) {
    row.scrollIntoView({ behavior: 'smooth', block: 'center' });
    row.style.background = 'var(--accent-soft)';
    setTimeout(function() { row.style.background = ''; }, 1500);
  }
}
```

- [ ] **Step 4: Verify in browser**

1. Navigate to Investimentos page
2. Select an equipe from the dropdown
3. Verify: team name appears in accent color, 4 stat cards show (Investimento/Mês, Membros, Tempo Médio, Banco de Horas), avatar circles appear in filter row
4. Hover on avatar → tooltip shows member name
5. Click avatar → (table not yet rendered, but no error)
6. Check dark mode: cards use `var(--surface)`, text uses `var(--text-primary)`

- [ ] **Step 5: Commit**

```bash
git add frontend/index.html
git commit -m "feat(frontend): investimentos dashboard with equipe filter, summary cards, and avatars"
```

---

### Task 3: SVG Line Chart (Gastos Mensais)

**Files:**
- Modify: `frontend/index.html` (add JS functions at bottom of script)

**Interfaces:**
- Consumes: `api()`, `formatSalarioBR()`, `invCurrentEquipeId` (Task 2), `inv-gastos-chart` div (Task 2)
- Produces: `drawInvLineChart(container, points, opts)` — reusable line chart function, `loadInvGastosMensais(equipeId, custoAtual)` — fetches and draws monthly cost chart

- [ ] **Step 1: Add drawInvLineChart function**

After the Task 2 functions in the script section, add this reusable SVG line chart:
```javascript
function drawInvLineChart(container, points, opts) {
  if (!container || !points || points.length === 0) {
    if (container) container.innerHTML = '<div style="text-align:center;color:var(--text-tertiary);padding:20px">Sem dados</div>';
    return;
  }
  opts = opts || {};
  var W = opts.w || 460, H = opts.h || 180;
  var pad = { t: 15, r: 20, b: 30, l: opts.leftPad || 65 };
  var plotW = W - pad.l - pad.r;
  var plotH = H - pad.t - pad.b;

  var values = points.map(function(p) { return p.value; });
  var maxVal = Math.max.apply(null, values);
  var minVal = Math.min.apply(null, values.filter(function(v) { return v > 0; }));
  if (maxVal === 0) maxVal = 1;
  if (minVal === maxVal) { minVal = 0; }
  else { minVal = Math.max(0, minVal * 0.85); }
  var range = maxVal - minVal || 1;

  var svgNS = 'http://www.w3.org/2000/svg';
  var svg = document.createElementNS(svgNS, 'svg');
  svg.setAttribute('viewBox', '0 0 ' + W + ' ' + H);
  svg.setAttribute('width', '100%');
  svg.style.maxWidth = W + 'px';
  svg.style.display = 'block';

  var xScale = function(i) { return pad.l + (points.length > 1 ? (i / (points.length - 1)) * plotW : plotW / 2); };
  var yScale = function(v) { return pad.t + plotH - ((v - minVal) / range) * plotH; };

  // Horizontal grid lines (4 lines)
  for (var g = 0; g <= 3; g++) {
    var gy = pad.t + (g / 3) * plotH;
    var gv = maxVal - (g / 3) * range;
    var gridLine = document.createElementNS(svgNS, 'line');
    gridLine.setAttribute('x1', pad.l);
    gridLine.setAttribute('x2', W - pad.r);
    gridLine.setAttribute('y1', gy);
    gridLine.setAttribute('y2', gy);
    gridLine.setAttribute('stroke', 'var(--border-subtle)');
    gridLine.setAttribute('stroke-width', '1');
    svg.appendChild(gridLine);

    var yLabel = document.createElementNS(svgNS, 'text');
    yLabel.setAttribute('x', pad.l - 6);
    yLabel.setAttribute('y', gy + 4);
    yLabel.setAttribute('text-anchor', 'end');
    yLabel.setAttribute('font-size', '10');
    yLabel.setAttribute('fill', 'var(--text-tertiary)');
    yLabel.textContent = opts.formatY ? opts.formatY(gv) : Math.round(gv).toLocaleString('pt-BR');
    svg.appendChild(yLabel);
  }

  // X axis labels
  var labelStep = points.length > 12 ? Math.ceil(points.length / 12) : 1;
  points.forEach(function(p, i) {
    if (i % labelStep !== 0 && i !== points.length - 1) return;
    var xLabel = document.createElementNS(svgNS, 'text');
    xLabel.setAttribute('x', xScale(i));
    xLabel.setAttribute('y', H - 6);
    xLabel.setAttribute('text-anchor', 'middle');
    xLabel.setAttribute('font-size', '10');
    xLabel.setAttribute('fill', 'var(--text-tertiary)');
    xLabel.textContent = p.label;
    svg.appendChild(xLabel);
  });

  // Build solid and dashed paths
  var solidPath = '', dashedPath = '';
  var lastSolidIdx = -1;

  points.forEach(function(p, i) {
    var x = xScale(i).toFixed(1), y = yScale(p.value).toFixed(1);
    if (p.projected) {
      if (!dashedPath && lastSolidIdx >= 0) {
        dashedPath = 'M' + xScale(lastSolidIdx).toFixed(1) + ',' + yScale(points[lastSolidIdx].value).toFixed(1);
      }
      dashedPath += ' L' + x + ',' + y;
    } else {
      solidPath += (solidPath === '' ? 'M' : ' L') + x + ',' + y;
      lastSolidIdx = i;
    }
  });

  // Draw solid line
  if (solidPath) {
    var sPath = document.createElementNS(svgNS, 'path');
    sPath.setAttribute('d', solidPath);
    sPath.setAttribute('fill', 'none');
    sPath.setAttribute('stroke', opts.color || 'var(--accent)');
    sPath.setAttribute('stroke-width', '2.5');
    sPath.setAttribute('stroke-linejoin', 'round');
    svg.appendChild(sPath);
  }

  // Draw dashed line (projected)
  if (dashedPath) {
    var dPath = document.createElementNS(svgNS, 'path');
    dPath.setAttribute('d', dashedPath);
    dPath.setAttribute('fill', 'none');
    dPath.setAttribute('stroke', opts.color || 'var(--accent)');
    dPath.setAttribute('stroke-width', '2');
    dPath.setAttribute('stroke-dasharray', '6,4');
    dPath.setAttribute('opacity', '0.5');
    dPath.setAttribute('stroke-linejoin', 'round');
    svg.appendChild(dPath);
  }

  // Data point circles + tooltips
  points.forEach(function(p, i) {
    var cx = xScale(i), cy = yScale(p.value);
    var circle = document.createElementNS(svgNS, 'circle');
    circle.setAttribute('cx', cx.toFixed(1));
    circle.setAttribute('cy', cy.toFixed(1));
    circle.setAttribute('r', '4');
    circle.setAttribute('fill', p.projected ? 'var(--surface)' : (opts.color || 'var(--accent)'));
    circle.setAttribute('stroke', opts.color || 'var(--accent)');
    circle.setAttribute('stroke-width', '2');
    circle.style.cursor = 'pointer';

    var title = document.createElementNS(svgNS, 'title');
    title.textContent = p.label + ': ' + (opts.formatTooltip ? opts.formatTooltip(p.value) : p.value.toLocaleString('pt-BR'));
    circle.appendChild(title);
    svg.appendChild(circle);
  });

  container.innerHTML = '';
  container.appendChild(svg);
}
```

- [ ] **Step 2: Add loadInvGastosMensais function**

```javascript
async function loadInvGastosMensais(equipeId, custoAtual) {
  var chartEl = document.getElementById('inv-gastos-chart');
  if (!chartEl) return;
  try {
    var data = await api('/equipes/' + equipeId + '/investimentos/gastos-mensais');
    var meses = data.meses || [];
    var mesLabels = ['Jan','Fev','Mar','Abr','Mai','Jun','Jul','Ago','Set','Out','Nov','Dez'];
    var currentMonth = new Date().getMonth() + 1; // 1-based

    var points = meses.map(function(m) {
      return {
        label: mesLabels[m.mes - 1] || m.mes,
        value: m.custo_total || 0,
        projected: m.mes > currentMonth
      };
    });

    // For future months with zero cost, project using current monthly cost
    points.forEach(function(p) {
      if (p.projected && p.value === 0 && custoAtual > 0) {
        p.value = custoAtual;
      }
    });

    drawInvLineChart(chartEl, points, {
      formatY: function(v) { return 'R$ ' + (v / 1000).toFixed(0) + 'k'; },
      formatTooltip: function(v) { return formatSalarioBR(v); }
    });
  } catch (err) {
    chartEl.innerHTML = '<div style="color:var(--text-tertiary);text-align:center;padding:20px">Erro ao carregar gráfico</div>';
  }
}
```

- [ ] **Step 3: Remove the typeof guard in renderInvestimentosDashboard**

In `renderInvestimentosDashboard()`, replace:
```javascript
  if (typeof loadInvGastosMensais === 'function') loadInvGastosMensais(invCurrentEquipeId, sum.custo_mensal_total);
```
with:
```javascript
  loadInvGastosMensais(invCurrentEquipeId, sum.custo_mensal_total);
```

- [ ] **Step 4: Verify in browser**

1. Select equipe on Investimentos page
2. Line chart appears on the right side alongside stat cards
3. X axis shows Jan-Dez, Y axis shows R$ values
4. Current and past months show solid line, future months dashed
5. Hover on data points → tooltip with formatted value
6. Test with equipe that has salary data (Bruno with R$12.500)

- [ ] **Step 5: Commit**

```bash
git add frontend/index.html
git commit -m "feat(frontend): add SVG line chart for monthly investment costs"
```

---

### Task 4: Members Table

**Files:**
- Modify: `frontend/index.html` (add JS function at bottom of script)

**Interfaces:**
- Consumes: `esc()`, `initials()`, `stringColor()`, `formatSalarioBR()`, `formatTempoCasa()` (Task 2), `inv-members-table` div (Task 2)
- Produces: `renderInvMembrosTable(membros)` — renders sorted members table, `openInvestimentoModal(membroId)` call on row click (implemented in Task 5)

- [ ] **Step 1: Add renderInvMembrosTable function**

```javascript
function renderInvMembrosTable(membros) {
  var el = document.getElementById('inv-members-table');
  if (!el) return;
  if (!membros || membros.length === 0) {
    el.innerHTML = '<div class="empty-state"><div class="empty-state-text">Nenhum membro encontrado.</div></div>';
    return;
  }

  var html = '<table class="inv-table"><thead><tr>';
  html += '<th>Nome</th><th>R$/Mês</th><th>Tempo de Casa</th><th>Banco de Horas</th><th>Atuação</th>';
  html += '</tr></thead><tbody>';

  membros.forEach(function(m) {
    var avatarHtml = m.avatar_url
      ? '<img src="' + esc(m.avatar_url) + '" alt="">'
      : initials(m.nome);

    var salarioHtml = m.salario != null
      ? formatSalarioBR(m.salario)
      : '<span class="inv-salary-null">Não definido</span>';

    var tempoHtml = m.data_admissao ? formatTempoCasa(m.tempo_casa_meses) : '<span class="inv-salary-null">—</span>';

    var bancoHtml = (m.banco_horas || 0) > 0
      ? m.banco_horas.toLocaleString('pt-BR') + 'h'
      : '0h';

    var produtosHtml = '';
    (m.top_produtos || []).forEach(function(p) {
      produtosHtml += '<span class="inv-chip">' + esc(p) + '</span>';
    });
    if (!produtosHtml) produtosHtml = '<span class="inv-salary-null">—</span>';

    html += '<tr data-membro-id="' + m.id + '" onclick="openInvestimentoModal(\'' + m.id + '\')">';
    html += '<td><div class="inv-member-cell"><div class="inv-member-avatar" style="background:' + stringColor(m.nome) + '">' + avatarHtml + '</div>' + esc(m.nome) + '</div></td>';
    html += '<td>' + salarioHtml + '</td>';
    html += '<td>' + tempoHtml + '</td>';
    html += '<td>' + bancoHtml + '</td>';
    html += '<td>' + produtosHtml + '</td>';
    html += '</tr>';
  });

  html += '</tbody></table>';
  el.innerHTML = html;
}
```

- [ ] **Step 2: Remove the typeof guard in renderInvestimentosDashboard**

In `renderInvestimentosDashboard()`, replace:
```javascript
  if (typeof renderInvMembrosTable === 'function') renderInvMembrosTable(membros);
```
with:
```javascript
  renderInvMembrosTable(membros);
```

- [ ] **Step 3: Verify in browser**

1. Select equipe on Investimentos page
2. Members table shows below the dashboard grid
3. Rows show: avatar + name, salary (formatted), tempo de casa, banco de horas, top 3 produtos as chips
4. Members sorted by salary desc (Bruno R$12.500 first, others with "Não definido")
5. Hover on row → row highlights with accent background
6. Click on row → calls `openInvestimentoModal` (not yet implemented, will show error in console — that's ok)
7. Click avatar in filter row → table scrolls to that member, row flashes

- [ ] **Step 4: Commit**

```bash
git add frontend/index.html
git commit -m "feat(frontend): add investimentos members table with salary, tenure, and products"
```

---

### Task 5: Person Detail Modal

**Files:**
- Modify: `frontend/index.html` (add JS functions at bottom of script)

**Interfaces:**
- Consumes: `api()`, `esc()`, `initials()`, `stringColor()`, `formatSalarioBR()`, `drawInvLineChart()` (Task 3), `invDashboardData` (Task 2), `investimento-modal` overlay div (Task 1)
- Produces: `openInvestimentoModal(membroId)`, `closeInvestimentoModal()`

- [ ] **Step 1: Add openInvestimentoModal function**

```javascript
async function openInvestimentoModal(membroId) {
  var modal = document.getElementById('investimento-modal');
  var content = document.getElementById('investimento-modal-content');
  modal.classList.add('open');
  content.innerHTML = '<div class="loading"><div class="spinner"></div></div>';

  try {
    // Find member data from cached dashboard
    var membro = null;
    if (invDashboardData && invDashboardData.membros) {
      membro = invDashboardData.membros.find(function(m) { return m.id === membroId; });
    }
    if (!membro) {
      content.innerHTML = '<div class="empty-state"><div class="empty-state-text">Membro não encontrado</div></div>';
      return;
    }

    // Fetch all 3 endpoints in parallel
    var results = await Promise.all([
      api('/membros/' + membroId + '/alocacoes-projetos').catch(function() { return { projetos: [] }; }),
      api('/membros/' + membroId + '/salario/historico').catch(function() { return []; }),
      api('/membros/' + membroId + '/banco-horas/historico').catch(function() { return []; })
    ]);

    var alocacoes = results[0].projetos || [];
    var historicoSalario = results[1] || [];
    var historicoBanco = results[2] || [];

    var avatarHtml = membro.avatar_url
      ? '<img src="' + esc(membro.avatar_url) + '" alt="">'
      : initials(membro.nome);

    var html = '';

    // Header
    html += '<div class="inv-modal-header">';
    html += '<div class="inv-modal-avatar" style="background:' + stringColor(membro.nome) + '">' + avatarHtml + '</div>';
    html += '<div class="inv-modal-info"><h2>' + esc(membro.nome) + '</h2><p>' + esc(membro.cargo || '—') + '</p></div>';
    html += '<button class="inv-modal-close" onclick="closeInvestimentoModal()" title="Fechar">&times;</button>';
    html += '</div>';

    // Allocations section
    html += '<div class="inv-modal-section"><h3>Projetos & Alocações</h3>';
    if (alocacoes.length > 0) {
      alocacoes.forEach(function(a) {
        html += '<div class="inv-alloc-item">';
        html += '<div class="inv-alloc-info"><div class="inv-alloc-name">' + esc(a.apelido) + '</div><div class="inv-alloc-key">' + esc(a.chave_jira) + '</div></div>';
        html += '<div class="inv-alloc-bar-wrap"><div class="inv-alloc-bar" style="width:' + Math.min(a.percentual_alocacao, 100).toFixed(1) + '%"></div></div>';
        html += '<div class="inv-alloc-pct">' + a.percentual_alocacao.toFixed(1) + '%</div>';
        html += '</div>';
      });
    } else {
      html += '<div style="color:var(--text-tertiary);font-size:13px">Nenhuma alocação registrada.</div>';
    }
    html += '</div>';

    // History charts
    html += '<div class="inv-modal-section"><h3>Histórico</h3>';
    html += '<div class="inv-modal-charts">';
    html += '<div class="inv-modal-chart-box"><h4>Salário</h4><div id="inv-hist-salario"></div></div>';
    html += '<div class="inv-modal-chart-box"><h4>Banco de Horas</h4><div id="inv-hist-banco"></div></div>';
    html += '</div></div>';

    content.innerHTML = html;

    // Draw salary history chart
    if (historicoSalario.length > 0) {
      var salarioPoints = historicoSalario.map(function(h) {
        var d = h.data_vigencia ? h.data_vigencia.substring(0, 10) : '';
        return { label: fmtDateBR(d), value: h.valor || 0, projected: false };
      });
      drawInvLineChart(document.getElementById('inv-hist-salario'), salarioPoints, {
        w: 300, h: 150, leftPad: 60,
        formatY: function(v) { return 'R$ ' + (v / 1000).toFixed(1) + 'k'; },
        formatTooltip: function(v) { return formatSalarioBR(v); }
      });
    } else {
      document.getElementById('inv-hist-salario').innerHTML = '<div style="text-align:center;color:var(--text-tertiary);padding:16px;font-size:12px">Sem histórico</div>';
    }

    // Draw banco de horas history chart
    if (historicoBanco.length > 0) {
      var bancoPoints = historicoBanco.map(function(h) {
        var d = h.data_registro ? h.data_registro.substring(0, 10) : '';
        return { label: fmtDateBR(d), value: h.valor || 0, projected: false };
      });
      drawInvLineChart(document.getElementById('inv-hist-banco'), bancoPoints, {
        w: 300, h: 150, leftPad: 45,
        color: 'var(--blue)',
        formatY: function(v) { return v.toFixed(0) + 'h'; },
        formatTooltip: function(v) { return v.toLocaleString('pt-BR') + 'h'; }
      });
    } else {
      document.getElementById('inv-hist-banco').innerHTML = '<div style="text-align:center;color:var(--text-tertiary);padding:16px;font-size:12px">Sem histórico</div>';
    }
  } catch (err) {
    content.innerHTML = '<div class="empty-state"><div class="empty-state-icon">⚠️</div><div class="empty-state-text">' + esc(err.message) + '</div></div>';
  }
}

function closeInvestimentoModal() {
  document.getElementById('investimento-modal').classList.remove('open');
}
```

- [ ] **Step 2: Verify in browser**

1. Select equipe, table loads
2. Click on a member row → modal opens
3. Modal shows: avatar, name, cargo, close button
4. Projects & allocations section: project names with JIRA keys below (smaller, transparent), colored bars proportional to %, percentage value on right
5. History section: two charts side by side — salary chart (green/accent) and banco de horas chart (blue)
6. For members with no salary history → shows "Sem histórico"
7. For member with data (Bruno) → shows salary and banco horas charts with actual data points
8. Click close button or click outside modal → modal closes
9. Check dark mode: modal uses `var(--modal-bg)`, text and borders follow theme

- [ ] **Step 3: Commit**

```bash
git add frontend/index.html
git commit -m "feat(frontend): add investimentos person detail modal with allocations and history charts"
```
