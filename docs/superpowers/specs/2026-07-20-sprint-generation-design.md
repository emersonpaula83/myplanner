# Sprint Generation v2 — Design Spec

## Summary

Refactor sprint generation to fix overlap detection bug and simplify UX. User provides `board_id` + `start_date`; system auto-detects sprint prefix and duration from existing board sprints, generates all missing sprints until end of year, and creates them in JIRA.

Key changes from v1:
- Overlap check uses JIRA API (source of truth) instead of local DB
- Prefix auto-detected from existing sprint names (no manual input)
- Duration auto-detected from existing sprint patterns (no hardcoded 2 weeks)
- New `start_date` field replaces `prefixo` field
- Sprint times: Monday 08:30 → Friday 18:30 (America/Sao_Paulo)
- Dates displayed as `dd/MM/yyyy HH:mm:ss` in UI

## API Contract

### GET /api/v1/sprints/boards?equipe_id={uuid}

Unchanged from v1. Returns JIRA boards accessible to the team.

```json
[
  {"id": 42, "name": "TCW Dev Board", "type": "scrum"},
  {"id": 424, "name": "RM Board", "type": "scrum"}
]
```

### POST /api/v1/sprints/generate/preview

Calculates missing sprint slots without creating anything.

**Request:**
```json
{
  "equipe_id": "uuid",
  "board_id": 424,
  "start_date": "2026-08-03"
}
```

**Response:**
```json
{
  "prefixo_detectado": "RM Dev",
  "duracao_detectada_dias": 11,
  "sprints": [
    {
      "nome": "RM Dev 03/08 - 14/08 [2026]",
      "data_inicio": "2026-08-03T08:30:00-03:00",
      "data_fim": "2026-08-14T18:30:00-03:00"
    }
  ],
  "existentes_ignoradas": 3
}
```

### POST /api/v1/sprints/generate

Creates sprints in JIRA and upserts locally. Same request body as preview.

**Response:**
```json
{
  "criadas": 10,
  "erros": []
}
```

## Sprint Pattern Detection

New method `detectSprintPattern(sprints []jira.JiraSprint) (prefixo string, duracaoDias int, error)`:

### Prefix Detection
1. Fetch sprints from board via `client.GetBoardSprints(boardID)`
2. Filter sprints with non-nil dates
3. Extract prefix via regex: everything before `DD/MM` pattern in the name
   - Example: `"RM Dev 21/07 - 01/08 [2026]"` → `"RM Dev"`
4. Use most frequent prefix (statistical mode) — protects against outlier names

### Duration Detection
1. Calculate day difference between start and end of each sprint with valid dates
2. Use most frequent duration (statistical mode)
3. Fallback: 11 days if no sprints have dates

### Errors
- Board has no sprints → `"Nenhuma sprint encontrada no board {id} para detectar padrão"`
- No recognizable name pattern → `"Não foi possível detectar prefixo das sprints do board {id}"`

## Sprint Slot Generation

Method `generateSprintSlots(startDate time.Time, duracaoDias int, ano int) []sprintSlot`:

### Logic
1. `startDate` adjusted to next Monday via `nextMonday()` if not already Monday
2. Loop while `start <= Dec 31 of ano`:
   - `end = start + (duracaoDias - 1) days` — lands on Friday
   - Start time: Monday 08:30:00 America/Sao_Paulo
   - End time: Friday 18:30:00 America/Sao_Paulo
   - Append slot
   - `start = end + 3 days` (next Monday)
3. If `end` exceeds Dec 31 → last sprint not included

### Overlap Check
- Fetch existing sprints via `client.GetBoardSprints(boardID)` (JIRA API, source of truth)
- `filterExistingSlots()` uses standard interval intersection: `slot.start < exEnd && slot.end > exStart`
- Sprints without dates in JIRA are skipped

### Changes from v1
- `duracaoDias` dynamic instead of hardcoded 11
- JIRA API instead of local DB for overlap check
- Correct times (08:30/18:30 America/Sao_Paulo) instead of 00:00/18:00 UTC

## Frontend Changes

### Form (`openGerarSprintsForm()`)

**Fields:**
- `Board ID` — input numérico (unchanged)
- `Data Inicial` — input `type="date"` (new, replaces `Prefixo` text input)

**Flow:**
1. User selects equipe → button "+ Gerar Sprints" appears
2. Click → form with Board ID + Data Inicial
3. "Calcular" → `POST /sprints/generate/preview` with `{equipe_id, board_id, start_date}`
4. Shows detected info + sprint table
5. "Criar N Sprints no JIRA" → `POST /sprints/generate`

**Validation:**
- Board ID: number > 0
- Data Inicial: required, cannot be in the past

**Preview display:**
- `"Prefixo detectado: RM Dev"`
- `"Duração detectada: 11 dias (2 semanas)"`
- Table:

| Nome | Início | Fim |
|------|--------|-----|
| RM Dev 03/08 - 14/08 [2026] | 03/08/2026 08:30:00 | 14/08/2026 18:30:00 |

**Date format:** always `dd/MM/yyyy HH:mm:ss` in all UI presentation.

## Error Handling

### Pattern Detection
- Board without sprints → `"Nenhuma sprint encontrada no board {id} para detectar padrão"`
- Unrecognizable name patterns → `"Não foi possível detectar prefixo das sprints do board {id}"`
- Sprints without dates → fallback 11 days for duration, warn in log

### Generation
- All sprints already exist → preview returns empty list + `"Todas as sprints já existem no board"`
- Individual JIRA failure → continues to next sprint, errors collected in response
- JIRA API unavailable → HTTP 502 with clear message

### Validation
- `board_id` = 0 → 400
- `start_date` empty or invalid → 400
- `start_date` in the past → 400 `"Data inicial não pode ser no passado"`
- `equipe_id` nil → 400

No changes to JIRA client retry/rate-limit logic.

## Files to Modify

| File | Action |
|------|--------|
| `backend/internal/service/sprint_generation.go` | Refactor: remove prefixo param, add start_date, add detectSprintPattern(), use JIRA API for overlap, fix sprint times |
| `backend/internal/handler/sprint_generation.go` | Refactor: update generateRequest struct (remove Prefixo, add StartDate), update validation |
| `backend/cmd/api/main.go` | No structural changes — same routes, same wiring |
| `frontend/index.html` | Refactor: replace Prefixo input with Data Inicial date picker, update preview display with detected info, format dates as dd/MM/yyyy HH:mm:ss |

## Dependencies

- JIRA Agile REST API `POST /rest/agile/1.0/sprint` requires board admin permissions
- JIRA Agile REST API `GET /rest/agile/1.0/board/{boardId}/sprint` for pattern detection
- OAuth scopes: `write:sprint:jira-software` (if using OAuth)
- API token: user needs JIRA project admin role
- Timezone: fixed `America/Sao_Paulo` for all sprint times
