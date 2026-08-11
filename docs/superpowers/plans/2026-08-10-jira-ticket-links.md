# JIRA Ticket Links Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform all `numero_ticket` displays across 8 screens into clickable JIRA links that open in a new tab.

**Architecture:** Add a `jiraTicketLink()` helper function that generates `<a>` tags using a cached JIRA base URL. Cache the URL from the existing `/fontes/{id}` API when sprint capacity loads. Replace 16 `esc(*.numero_ticket)` calls with `jiraTicketLink(*.numero_ticket)`.

**Tech Stack:** Vanilla JS (frontend/index.html)

## Global Constraints

- All links open `target="_blank"` with `rel="noopener"`
- `color:inherit; text-decoration:underline` to preserve existing visual style
- Graceful fallback: no base_url = plain text (current behavior)
- XSS prevention via existing `esc()` function
- No backend changes

---

### Task 1: Add `jiraTicketLink()` helper and base_url cache

**Files:**
- Modify: `frontend/index.html:1944` (after `esc()` function)
- Modify: `frontend/index.html:2743-2748` (sprint capacity load, cache base_url)

**Interfaces:**
- Consumes: `esc()` (line 1944), `api()` for fetching fonte data
- Produces: `window._jiraBaseUrl` (string), `jiraTicketLink(numero)` (returns HTML string)

- [ ] **Step 1: Add `jiraTicketLink()` helper function after `esc()` on line 1944**

```javascript
function jiraTicketLink(numero) {
  if (!numero) return '';
  if (!window._jiraBaseUrl) return esc(numero);
  return '<a href="' + esc(window._jiraBaseUrl) + '/browse/' + esc(numero) + '" target="_blank" rel="noopener" style="color:inherit;text-decoration:underline">' + esc(numero) + '</a>';
}
```

- [ ] **Step 2: Cache `_jiraBaseUrl` when sprint capacity loads**

At line ~2747, after the `syncStatus` fetch inside the `if (fonteDadosId)` block, add the base_url fetch:

```javascript
if (fonteDadosId) {
  syncStatus = await api('/sync/status?fonte_dados_id=' + fonteDadosId).catch(() => null);
  if (!window._jiraBaseUrl) {
    try {
      const fonte = await api('/fontes/' + fonteDadosId);
      if (fonte && fonte.base_url) window._jiraBaseUrl = fonte.base_url.replace(/\/+$/, '');
    } catch(e) {}
  }
}
```

- [ ] **Step 3: Test manually**

Open sprint capacity view. Open browser DevTools console. Verify `window._jiraBaseUrl` is set to `https://totvscloud.atlassian.net` (no trailing slash).

---

### Task 2: Replace `esc(*.numero_ticket)` with `jiraTicketLink()` — Timeline & Sprint Review (4 locations)

**Files:**
- Modify: `frontend/index.html:2559` (Timeline epic table)
- Modify: `frontend/index.html:3153` (Sprint Review task table)
- Modify: `frontend/index.html:3341` (Sprint Review export/print)
- Modify: `frontend/index.html:6067` (Timeline pie chart tooltip)

**Interfaces:**
- Consumes: `jiraTicketLink()` from Task 1

- [ ] **Step 1: Timeline epic table (line ~2559)**

Change:
```javascript
'<td style="font-weight:550;color:var(--accent)">' + esc(p.numero_ticket) + '</td>'
```
To:
```javascript
'<td style="font-weight:550;color:var(--accent)">' + jiraTicketLink(p.numero_ticket) + '</td>'
```

- [ ] **Step 2: Sprint Review task table — `buildReviewTaskTable` (line ~3153)**

Change:
```javascript
h += '<tr><td>' + esc(t.numero_ticket) + '</td>'
```
To:
```javascript
h += '<tr><td>' + jiraTicketLink(t.numero_ticket) + '</td>'
```

- [ ] **Step 3: Sprint Review export/print — `buildReviewExportHTML` (line ~3341)**

Change:
```javascript
html += '<tr><td style="padding:4px 8px;border-bottom:1px solid #f0f0f0">' + esc(t.numero_ticket) + '</td>';
```
To:
```javascript
html += '<tr><td style="padding:4px 8px;border-bottom:1px solid #f0f0f0">' + jiraTicketLink(t.numero_ticket) + '</td>';
```

- [ ] **Step 4: Timeline pie chart tooltip — `showPieTooltip` (line ~6067)**

Change:
```javascript
html += '<tr><td>' + esc(t.numero_ticket) + '</td>'
```
To:
```javascript
html += '<tr><td>' + jiraTicketLink(t.numero_ticket) + '</td>'
```

- [ ] **Step 5: Test manually**

Open Timeline view — click epic ticket number, verify JIRA opens in new tab. Open Sprint Review — verify ticket links work. Export review — verify links in export. Click pie chart slice — verify tooltip ticket links work.

---

### Task 3: Replace `esc(*.numero_ticket)` — Capacity, Equalizer, Tarefas (4 locations)

**Files:**
- Modify: `frontend/index.html:4223` (Capacity task rows)
- Modify: `frontend/index.html:6165` (Equalizer suggestions)
- Modify: `frontend/index.html:6640` (Tarefas management table)
- Modify: `frontend/index.html:6657` (Tarefas delete button — no change, this is a JS argument not display)

**Interfaces:**
- Consumes: `jiraTicketLink()` from Task 1

- [ ] **Step 1: Capacity task rows — `buildCapacityHtml` (line ~4223)**

Change:
```javascript
html += '<span class="capacity-tarefa-ticket">' + esc(t.numero_ticket) + '</span>';
```
To:
```javascript
html += '<span class="capacity-tarefa-ticket">' + jiraTicketLink(t.numero_ticket) + '</span>';
```

- [ ] **Step 2: Equalizer suggestions (line ~6165)**

Change:
```javascript
'<span class="eq-task-key">' + esc(t.numero_ticket) + '</span>'
```
To:
```javascript
'<span class="eq-task-key">' + jiraTicketLink(t.numero_ticket) + '</span>'
```

- [ ] **Step 3: Tarefas management table (line ~6640)**

Change:
```javascript
html += '<td><strong>' + esc(t.numero_ticket) + '</strong></td>';
```
To:
```javascript
html += '<td><strong>' + jiraTicketLink(t.numero_ticket) + '</strong></td>';
```

- [ ] **Step 4: Line ~6657 — skip**

Line 6657 passes `esc(t.numero_ticket)` as a JS function argument to `hardDeleteTarefa()`, not as display text. No change needed here.

- [ ] **Step 5: Test manually**

Open Capacity view — verify ticket links. Open Equalizer — verify ticket links in suggestions. Open Tarefas page — verify ticket links in management table.

---

### Task 4: Replace `esc(*.numero_ticket)` — Alocação (8 locations)

**Files:**
- Modify: `frontend/index.html:6928` (Alloc epic cards)
- Modify: `frontend/index.html:7091` (Alloc modal header)
- Modify: `frontend/index.html:7156` (Alloc planned tasks)
- Modify: `frontend/index.html:7177` (Alloc completed tasks)
- Modify: `frontend/index.html:7250` (Alloc gantt label)
- Modify: `frontend/index.html:7262` (Alloc gantt tooltip — no change, tooltip title attribute)
- Modify: `frontend/index.html:7271` (Alloc unallocated gantt label)
- Modify: `frontend/index.html:7273` (Alloc unallocated gantt tooltip — no change, tooltip title attribute)
- Modify: `frontend/index.html:7288` (Alloc editable task row)

**Interfaces:**
- Consumes: `jiraTicketLink()` from Task 1

- [ ] **Step 1: Alloc epic cards (line ~6928)**

Change:
```javascript
html += '<span class="alloc-box-ticket">' + esc(p.numero_ticket) + '</span>';
```
To:
```javascript
html += '<span class="alloc-box-ticket">' + jiraTicketLink(p.numero_ticket) + '</span>';
```

- [ ] **Step 2: Alloc modal header (line ~7091)**

Change:
```javascript
html += '<h2>' + esc(epic.numero_ticket) + ': '
```
To:
```javascript
html += '<h2>' + jiraTicketLink(epic.numero_ticket) + ': '
```

- [ ] **Step 3: Alloc planned tasks (line ~7156)**

Change:
```javascript
html += '<span class="alloc-task-ticket">' + esc(t.numero_ticket) + '</span>';
```
To:
```javascript
html += '<span class="alloc-task-ticket">' + jiraTicketLink(t.numero_ticket) + '</span>';
```

- [ ] **Step 4: Alloc completed tasks (line ~7177)**

Change:
```javascript
html += '<span class="alloc-task-ticket">' + esc(t.numero_ticket) + '</span>';
```
To:
```javascript
html += '<span class="alloc-task-ticket">' + jiraTicketLink(t.numero_ticket) + '</span>';
```

- [ ] **Step 5: Alloc gantt label (line ~7250)**

Change:
```javascript
html += '<div class="alloc-gantt-label" title="' + escAttr(t.resumo) + '">' + esc(t.numero_ticket) + '</div>';
```
To:
```javascript
html += '<div class="alloc-gantt-label" title="' + escAttr(t.resumo) + '">' + jiraTicketLink(t.numero_ticket) + '</div>';
```

- [ ] **Step 6: Lines ~7262, ~7273 — skip**

These pass `t.numero_ticket` inside `title` attributes (tooltip text for gantt bars). Links don't render in `title` attributes — keep as `escAttr()`. No change.

- [ ] **Step 7: Alloc unallocated gantt label (line ~7271)**

Change:
```javascript
html += '<div class="alloc-gantt-label" title="' + escAttr(t.resumo) + '">' + esc(t.numero_ticket) + '</div>';
```
To:
```javascript
html += '<div class="alloc-gantt-label" title="' + escAttr(t.resumo) + '">' + jiraTicketLink(t.numero_ticket) + '</div>';
```

- [ ] **Step 8: Alloc editable task row (line ~7288)**

Change:
```javascript
html += '<span class="alloc-task-ticket">' + esc(t.numero_ticket) + '</span>';
```
To:
```javascript
html += '<span class="alloc-task-ticket">' + jiraTicketLink(t.numero_ticket) + '</span>';
```

- [ ] **Step 9: Test manually**

Open Alocação view — verify epic card ticket links. Open project detail modal — verify header link, planned task links, completed task links. Check gantt labels — verify links. Check editable task rows — verify links. All should open JIRA in new tab.
