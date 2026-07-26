# Sprint Timeline Board Guardrail — Design Spec

## Problem

The Sprints Timeline report repeatedly leaks sprints from other teams' boards into a given equipe's chart. Root causes:

1. **`includeEmpty` OR-clause** in `listSprints()` (sprint.go:144-160) includes empty sprints from ANY board in a project if that project has any sprint with tasks from equipe members
2. **"Dominant project" heuristic** in `GetSprintsTimeline()` (sprint.go:813-840) tries to compensate but fails when empty sprints inflate the wrong project's count

Every time `listSprints()` or `GetSprintsTimeline()` is modified, the bug risks reintroduction. No test catches it.

## Solution

Hybrid guardrail: structural filter (equipe → board_id) + regression test.

## Schema

Add `board_id INTEGER` column to `equipes` table:

```sql
ALTER TABLE equipes ADD COLUMN board_id INTEGER;
```

- Nullable (equipes without board_id fall back to current equipe_membros filtering)
- No FK — board_id is a logical reference to Jira's board ID integer
- Relationship: 1 equipe = 1 board

## Query Changes

### `listSprints()` — add board_id filter

When equipe has `board_id`, add direct filter:

```go
if boardID != nil {
    query += fmt.Sprintf(" AND s.board_id = $%d", argN)
    args = append(args, *boardID)
    argN++
}
```

This replaces the need for the `includeEmpty` OR-clause (empty sprints from the correct board are included naturally; empty sprints from other boards are excluded).

The existing `equipe_membros` EXISTS filter remains as defense-in-depth.

### `GetSprintsTimeline()` — remove "dominant project" heuristic

Remove lines 813-840 (projetoCount / dominantProjeto logic). With board_id filtering at the query level, this heuristic is unnecessary.

**Fallback:** When equipe has no board_id (NULL), current behavior is preserved (equipe_membros filter only, no board_id filter). No breaking change.

### Signature change

`listSprints()` receives additional `boardID *int` parameter:

```go
func (r *SprintRepository) listSprints(ctx context.Context, equipeID *uuid.UUID, estado *string, includeEmpty bool, boardID *int) ([]SprintListItem, error)
```

`ListSprintsIncludeEmpty()` and `ListSprints()` propagate the parameter.

`GetSprintsTimeline()` fetches equipe's board_id before calling `ListSprintsIncludeEmpty()`.

## Auto-detection in Sync

After `syncSprints()`, for each equipe in the fonte_dados with `board_id IS NULL`:

```sql
SELECT s.board_id, COUNT(*) as cnt
FROM sprints s
JOIN tarefas t ON t.sprint_id = s.id
JOIN equipe_membros em ON em.membro_id = t.responsavel_id
WHERE em.equipe_id = $1 AND s.board_id IS NOT NULL
GROUP BY s.board_id
ORDER BY cnt DESC LIMIT 1
```

If found → `UPDATE equipes SET board_id = $1 WHERE id = $2`. Only when `board_id IS NULL` (never overwrites manual value).

Log: `"Auto-detected board_id=%d for equipe %s"`

## UI

Add "Board ID (Jira)" field to equipe create/edit form:
- Numeric input, optional
- Shows current value (auto-detected or manual)
- Editing saves directly to `equipes.board_id`

## Regression Tests

### Test 1: sprints from other boards do not leak

```
Setup:
- Equipe "DevOps Varejo" with board_id=100
- Project X with sprints on board 100 (correct board)
- Project X with sprints on board 200 (different team, same project)
- Project Y with sprints on board 300 (different project)

Assert:
- Only board 100 sprints returned
- No board 200 or 300 sprints
- Empty sprints from board 100 included
- Empty sprints from board 200 NOT included
```

### Test 2: fallback without board_id

```
Setup:
- Equipe with board_id = NULL
- Sprints with tasks from equipe members

Assert:
- Filters by equipe_membros normally (current behavior)
```

### Test 3: auto-detection does not overwrite manual value

```
Setup:
- Equipe with board_id = 100 (manually set)
- Sync runs with sprints predominantly from board 200

Assert:
- board_id remains 100 after sync
```

## Scope Exclusions

- No multi-board per equipe support (1:1 only)
- No migration to backfill board_id for existing equipes (auto-detection handles it on next sync)
- No board_id validation against Jira API (trust the integer value)
