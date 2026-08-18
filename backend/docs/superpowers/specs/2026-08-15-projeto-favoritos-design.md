# Projeto Favoritos — Design Spec

## Goal

Allow users to mark Jira projects as favorites within a Fonte de Dados, so they appear at the top of the projects modal and can be batch-synced in one click.

## Context

Today, the "Projetos" modal in Fontes de Dados fetches the full project list from Jira live (`GET /sync/projects`). Users must scroll through all projects to find the ones they sync regularly. No persistence of which projects matter to a user — every visit starts fresh.

## Decisions

- **Per-user**: each user has their own favorites, independent of other users
- **Favorites at top**: full Jira project list remains visible, favorites float to the top with a star indicator
- **Batch sync**: "Sincronizar Favoritos" button triggers sync for all favorited projects at once
- **Identified by `project_key`**: favorites reference the Jira project key (e.g., "TCDV"), not the local `projetos.id`, because a project may be favorited before it's ever synced locally

## Database

New migration. Single table:

```sql
CREATE TABLE usuario_projeto_favoritos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id UUID NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    fonte_dados_id UUID NOT NULL REFERENCES fonte_dados(id) ON DELETE CASCADE,
    project_key VARCHAR(20) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(usuario_id, fonte_dados_id, project_key)
);
CREATE INDEX idx_upf_usuario_fonte ON usuario_projeto_favoritos(usuario_id, fonte_dados_id);
```

CASCADE on both FKs — deleting a user or fonte removes their favorites.

## Backend API

### `GET /fontes/{id}/favoritos`

Returns the authenticated user's favorite project keys for a given fonte.

- Auth: JWT — extracts `usuario_id` from context
- Response: `200 OK` with `["TCDV", "PLAT", "DATA"]`
- Empty favorites: `200 OK` with `[]`

### `PUT /fontes/{id}/favoritos`

Replaces all favorites for the authenticated user on a given fonte.

- Auth: JWT
- Request body: `{"project_keys": ["TCDV", "PLAT"]}`
- Behavior: delete-then-insert within a transaction (same pattern as `UsuarioRepository.AtualizarProjetos`)
- Response: `200 OK` with `{"project_keys": ["TCDV", "PLAT"]}`
- Empty array clears all favorites: `{"project_keys": []}`

### `POST /sync/trigger-batch`

Triggers sync for all of the user's favorite projects on a given fonte.

- Auth: JWT
- Request body: `{"fonte_dados_id": "<uuid>"}`
- Behavior:
  1. Fetch user's favorites for that fonte
  2. For each project_key, call existing `SyncService.SyncProject()` (sequential — each starts a goroutine internally)
  3. Return immediately with the list of triggered projects
- Response: `200 OK` with `{"triggered": ["TCDV", "PLAT"], "count": 2}`
- No favorites: `400` with error message
- Frontend polls sync status using existing `GET /sync/status` mechanism

### Go structure

- **`backend/internal/repository/favoritos.go`**: `FavoritosRepository` with methods:
  - `List(ctx, usuarioID, fonteDadosID) ([]string, error)` — returns project_keys
  - `Replace(ctx, usuarioID, fonteDadosID, projectKeys []string) error` — delete + insert in tx
- **`backend/internal/handler/favoritos.go`**: `FavoritosHandler` with `List` and `Replace` methods, plus `TriggerBatch` for batch sync
- Routes registered under existing `/api/fontes` group with auth middleware

## Frontend

### Projects Modal Changes

`openProjectsModal(fonteId, nome)` and `loadJiraProjects(fonteId)` are modified:

1. On modal open, two parallel fetches:
   - `GET /sync/projects?fonte_dados_id=<id>` — full Jira project list (existing)
   - `GET /fontes/<id>/favoritos` — user's favorites (new)

2. Render combined list:
   - **Favorites section** (top): projects whose key is in the favorites list, each with a filled star (★)
   - **Divider**: subtle `1px` line with label "Outros projetos" if there are favorites
   - **Other projects** (below): remaining projects, each with an empty star (☆)
   - If no favorites, no divider — just the full list with empty stars

3. Each row layout: `[★/☆] [ProjectKey] [ProjectName] [Sincronizar button]`

### Star Toggle Behavior

- Click star → toggle locally (add/remove from in-memory favorites set)
- Immediately call `PUT /fontes/{id}/favoritos` with updated full list
- Re-sort the modal list (favorite moves to top / drops to bottom section)
- No full reload needed — DOM manipulation only

### "Sincronizar Favoritos" Button

- Positioned in the modal header, right of the title
- Only visible when ≥1 favorite exists
- Click:
  1. Disable button, show spinner text "Sincronizando..."
  2. Call `POST /sync/trigger-batch`
  3. For each triggered project, start polling via existing `startSyncPolling()`
  4. When all complete, re-enable button, update fonte card stats

### Star SVG

Inline SVG, 16×16:
- **Active** (favorited): filled star, color `#F59E0B` (amber)
- **Inactive**: outline star, color `var(--text-tertiary)`
- `cursor: pointer`, `transition: color 0.15s`

### CSS

New classes added to the existing `<style>` block (no separate file):
- `.project-row` — flex row for each project in modal
- `.project-star` — star icon button (no border/background)
- `.project-star.active` — filled amber star
- `.projects-divider` — subtle divider between favorites and others

### No changes outside the modal

The fonte card itself is unchanged. Favorites are managed exclusively within the projects modal.

## Testing

- **Backend unit tests**: `FavoritosRepository` — test List, Replace (including empty replace), unique constraint
- **Backend handler tests**: test auth extraction, request validation, batch trigger with no favorites
- **Manual testing**: open modal, toggle stars, verify persistence across modal close/reopen, batch sync, theme switching (dark/light)

## Global Constraints

- Monolithic frontend: all changes in `frontend/index.html`
- Go backend: follow existing patterns (handler → service → repository)
- Auth: reuse existing JWT middleware and `usuario_id` extraction from context
- CSS variables: use existing theme variables (`--text-tertiary`, `--border`, `--surface`, etc.)
- No external dependencies
