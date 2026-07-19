# Sync Optimization Design — Approach A+C

**Date:** 2026-07-19  
**Status:** Approved  

## Problem

Current sync fetches all 130 JIRA projects × (users + boards + issues) per cycle = ~400-600 API requests, hitting JIRA rate limits (429).

## Solution

Sync only projects relevant to equipes, using a single cross-project JQL query. Eliminate dedicated Users and Boards API calls. Remove background worker.

## Architecture

### Sync Flow (new `executSync`)

1. **Discover sprint field** — `GET /rest/api/3/field` (1 request)
2. **Discover project keys** — DB query: `SELECT DISTINCT p.chave FROM projetos p INNER JOIN tarefas t ON t.projeto_id = p.id INNER JOIN equipe_membros em ON em.membro_id = t.responsavel_id`. Falls back to fetching all projects if no equipes exist yet (first sync).
3. **Single JQL query** — `POST /rest/api/3/search/jql` with `project IN (KEY1, KEY2, ...) ORDER BY updated DESC`, paginated (100/page). If `ultimo_sync` exists, adds `AND updated >= "date"`.
4. **Process issues** — For each issue: upsert projeto (from issue.fields.project), upsert membro (from assignee/reporter), upsert sprint (from custom field), upsert tarefa.
5. **Resolve parents** — Same as current.

### Eliminated API Calls

| Call | Reason |
|------|--------|
| `GET /project/search` | Projects derived from DB equipe membership or from issue data |
| `GET /user/assignable/search` | Members upserted from issue assignee/reporter |
| `GET /rest/agile/1.0/board` | 401 error anyway; sprints come from issue custom field |
| `GET /rest/agile/1.0/board/{id}/sprint` | Same as above |

### Estimated Requests

- First sync (no equipes): 1 field + N pages issues (~5-15 requests per project)
- Subsequent syncs: 1 field + 2-8 pages (only updated issues from ~6 projects)

### Background Worker

Removed. Sync is manual only via `POST /api/sync/trigger`.

### Member Upsert (from issues)

When processing issue assignee/reporter:
- If accountID already in memberCache → skip
- Else → `UpsertMembro` with displayName, avatarUrl from issue fields
- Team derived from project name (same as current)

### New JIRA Client Method

```go
GetIssuesByProjects(ctx context.Context, projectKeys []string, updatedSince *time.Time) ([]JiraIssue, error)
```

Single JQL: `project IN (KEY1, KEY2, ...) [AND updated >= "date"] ORDER BY updated DESC`

### DB Query: Project Keys for Sync

```sql
SELECT DISTINCT p.chave
FROM projetos p
INNER JOIN tarefas t ON t.projeto_id = p.id
INNER JOIN equipe_membros em ON em.membro_id = t.responsavel_id
WHERE p.fonte_dados_id = $1 AND p.ativo = true
```

Returns keys of projects that have at least one task assigned to an equipe member.

### Fallback (first sync / no equipes)

If no project keys found in DB, sync falls back to `GetProjects` API call to discover projects. This handles first-time setup where no data exists yet.

## Files to Change

1. `backend/internal/jira/client.go` — Add `GetIssuesByProjects` method
2. `backend/internal/service/sync.go` — Rewrite `executSync` to use new flow; update `processIssue` to upsert members inline
3. `backend/internal/repository/sync.go` — Add `GetProjectKeysForSync` query
4. `backend/cmd/api/main.go` — Remove worker startup
5. `backend/internal/worker/sync.go` — Delete file
6. `backend/internal/service/sync_test.go` — Update mock, add tests
