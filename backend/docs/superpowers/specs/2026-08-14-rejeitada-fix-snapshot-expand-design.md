# Fix "Rejeitada" Status + Expand Sprint Snapshot — Design Spec

## Problem Statement

Two bugs and one enhancement:

1. **Bug: "Rejeitada" tasks inflate hours** — Tasks with status "Rejeitada" are not filtered from sprint capacity calculations, burndown charts, equalizer, and timeline detail. Only "Cancelado" is excluded in 5 locations. Since Jira maps "Rejeitada" to `statusCategory = 'done'`, these tasks also inflate `status_categoria`-based metrics (effort report, membro stats).

2. **Enhancement: Sprint snapshot lacks capacity/burndown data** — `captureSprintSnapshot` in `sync.go` only saves task list (`[]snapshotTask`) to `sprint_review_snapshots.snapshot_json`. After a sprint closes, capacity metrics and burndown data are computed from live task data which may have changed (tasks moved, re-estimated, reassigned). The snapshot should freeze capacity and burndown at close time.

3. **Test coverage: Guarantee snapshot before close** — Unit tests must prove that when a sprint transitions active→closed, the snapshot (including capacity + burndown) is captured BEFORE the sprint state is updated.

## Architecture

### Part 1: "Rejeitada" Fix

Add "Rejeitada" to the exclusion filter in 5 locations:

| # | File | Line | Current | Fix |
|---|------|------|---------|-----|
| 1 | `internal/service/sprint.go` | 240 | `if t.Status == "Cancelado"` | `if t.Status == "Cancelado" \|\| t.Status == "Rejeitada"` |
| 2 | `internal/repository/sprint.go` | ~315 | `'Cancelado'` in NOT IN list (GetEqualizerTarefas) | Add `'Rejeitada'` to NOT IN list |
| 3 | `internal/repository/sprint.go` | ~831 | `t.status != 'Cancelado'` (GetHorasAlocadasPorSprint) | `t.status NOT IN ('Cancelado', 'Rejeitada')` |
| 4 | `internal/repository/sprint.go` | ~862 | `t.status != 'Cancelado'` (GetBurndownTarefas) | `t.status NOT IN ('Cancelado', 'Rejeitada')` |
| 5 | `internal/repository/sprint.go` | ~917 | `t.status != 'Cancelado'` (GetTimelineDetailTarefas) | `t.status NOT IN ('Cancelado', 'Rejeitada')` |

No migration needed. Pure code fix.

### Part 2: Expand Sprint Snapshot

#### Snapshot Format v2

The `snapshot_json` JSONB column will store a richer struct:

```json
{
  "version": 2,
  "tarefas": [
    {
      "id": "uuid",
      "numero_ticket": "PROJ-123",
      "resumo": "Task title",
      "tipo": "Story",
      "tipo_demanda": "Evolutiva",
      "status": "Concluído",
      "parent_id": null,
      "relator_nome": "Ana",
      "nao_planejada": false,
      "estimativa_tempo": 14400,
      "produtos": ["Produto A"],
      "produto_ids": ["uuid"]
    }
  ],
  "capacidade": {
    "dias_uteis": 10,
    "horas_total_sprint": 480.0,
    "horas_alocadas": 320.0,
    "horas_executadas": 200.0,
    "horas_pendentes_execucao": 80.0,
    "total_membros_equipe": 5,
    "membros": [
      {
        "membro_id": "uuid",
        "nome": "Ana",
        "horas_estimadas": 48.0,
        "horas_alocadas": 40.0,
        "horas_executadas": 32.0,
        "horas_disponiveis": 48.0,
        "percentual_alocacao": 83.3,
        "percentual_executado": 66.7,
        "overcapacity": false,
        "da_equipe": true
      }
    ]
  },
  "burndown": {
    "horas_total": 320.0,
    "linha_ideal": [{"data": "2026-08-01", "horas": 320.0}, ...],
    "linha_real": [{"data": "2026-08-01", "horas": 320.0}, ...],
    "linha_nao_planejadas": [{"data": "2026-08-01", "horas": 0}, ...]
  }
}
```

#### Backward Compatibility

`GetSprintSnapshot` in `review.go` currently does:
```go
json.Unmarshal(raw, &tasks) // directly into []ReviewTaskRow
```

Must change to:
1. Try to unmarshal as v2 struct (has `version` field)
2. If v2: extract `tarefas` field → return `[]ReviewTaskRow`
3. If unmarshal fails or no `version` field: treat as v1 legacy `[]ReviewTaskRow`

#### captureSprintSnapshot Changes

`captureSprintSnapshot` in `sync.go` currently:
1. Queries tasks → marshals → inserts

New flow:
1. Query tasks (existing logic, unchanged)
2. Compute capacity metrics — call existing repo/service methods or replicate the core calculation inline
3. Compute burndown data — call existing repo/service methods or replicate inline
4. Marshal v2 struct with all three sections
5. Insert (existing ON CONFLICT DO NOTHING)

**Dependency consideration:** `captureSprintSnapshot` is a repository method on `SyncRepository`. Computing capacity/burndown requires data from `SprintRepository` methods and service-layer logic. Two approaches:

- **Option A (recommended):** Pass pre-computed capacity and burndown data into `captureSprintSnapshot` from the caller (`UpsertSprint`). The caller orchestrates: fetch capacity data → fetch burndown data → call captureSprintSnapshot with all data.
- **Option B:** Move snapshot logic to service layer. More invasive refactor.

We go with **Option A**: `captureSprintSnapshot` receives optional capacity/burndown data alongside the sprint ID.

#### No Migration Needed

`snapshot_json` is already JSONB — flexible schema. Only content changes, not column structure.

### Part 3: Unit Tests

#### Tests for "Rejeitada" Fix

In existing test files (`sprint_test.go`, `sprint_generation_test.go`):
- Test GetCapacity: task with status "Rejeitada" must NOT appear in horas_alocadas
- Test GetCapacity: task with status "Cancelado" must NOT appear (regression guard)
- Test burndown: "Rejeitada" tasks excluded from burndown calculation

#### Tests for Snapshot

- Test that v2 snapshot struct serializes/deserializes correctly
- Test backward compat: v1 snapshot (raw `[]ReviewTaskRow`) still works
- Test that snapshot includes capacity data when present
- Test that snapshot includes burndown data when present
- Test that "Rejeitada" tasks are excluded from snapshot task list (already works, test guards it)
- Test snapshot ordering: snapshot captured before sprint state update (verify via mock call ordering)

## Tech Stack

- Go stdlib `testing`
- Function-field mock pattern (consistent with existing test suite)
- No external test dependencies
- No database migration

## Constraints

- No commits until everything passes 100%
- Backward compatible with existing v1 snapshots
- stdlib testing only — no testify/gomock
- Follow existing mock patterns in codebase
