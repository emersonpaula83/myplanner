# Planning Tab UX Improvements — Design Spec

## Goal

Three visual improvements to the Planning tab for better feedback and space efficiency.

## Tech Stack

Monolithic vanilla JS frontend (`frontend/index.html`). No backend changes.

## Features

### 1. Stat Flash on Change

When any action (alocar, desalocar, mover backlog, mover sprint, incluir tarefas) changes stats numbers, the affected stat elements flash briefly with a gold glow to highlight what changed.

**CSS:** `@keyframes stat-flash` — gold box-shadow pulse, 0.8s ease-out. Class `planning-stat-flash` added to changed elements, removed on `animationend`.

**JS:** `updatePlanningStats()` function — compares previous values to new values for each stat (`ps-total`, `ps-alocadas`, `ps-nao-alocadas`, `ps-horas-disp`, `ps-horas-pend`, `ps-horas-aloc`). Elements whose values changed receive the flash class. Also applies to member chips in the header strip (Feature 3).

### 2. Three-Column Member Card Grid

Member capacity cards switch from vertical stack to a 3-column CSS grid. "Não Alocadas" card stays full-width above the grid.

**CSS:**
- `.planning-members-grid` — `display:grid; grid-template-columns: repeat(3, 1fr); gap:12px`
- Responsive: 2 columns at `max-width:1200px`, 1 column at `max-width:768px`

**JS:** `renderPlanningTab()` wraps member cards in a `<div class="planning-members-grid">` container.

### 3. Team Member Strip in Sticky Header

New row below the stats bar, inside `.planning-sticky-header`. Shows each team member as a compact chip: 24px avatar + first name only + allocation % colored by status (green ≤80%, amber 80-100%, red >100%).

**Layout:** Horizontal flex with wrap. Chips are ~120px wide, showing avatar circle, first name truncated, and bold percentage.

**CSS:**
- `.planning-team-strip` — `display:flex; flex-wrap:wrap; gap:6px; margin-bottom:8px; padding:8px 12px; background:var(--surface-secondary); border-radius:8px`
- `.planning-team-chip` — compact flex row with avatar, name, percentage
- Color classes reuse existing `green/amber/red` pattern from `.planning-card-pct`

**JS:** Generated inside `renderPlanningTab()`, placed after stats div. Uses same `recalcMemberCapacity()` data. Flash animation applies to chips when their % changes.

## Constraints

- `var` for globals, `function` declarations (no ES6 modules)
- CSS custom properties for theming (light/dark)
- All user-facing text in Portuguese
- No commits without explicit user consent
- Changes applied directly to main at `/home/emerson/code/myplanner/`
