# Sincronização Agendada — Design Spec

## Objetivo

Permitir que o usuário configure horários do dia para sincronização automática de projetos específicos do JIRA. Configuração por fonte de dados, com seleção de projetos e até 4 horários (hora:minuto).

## Database

### Nova tabela `sync_schedules`

```sql
CREATE TABLE sync_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fonte_dados_id UUID NOT NULL REFERENCES fonte_dados(id) ON DELETE CASCADE,
    project_key VARCHAR(50) NOT NULL,
    horarios JSONB NOT NULL DEFAULT '[]',
    ativo BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(fonte_dados_id, project_key)
);
```

- `horarios`: array JSON de strings `"HH:MM"` (ex: `["08:30","14:00","18:00"]`), máximo 4 elementos
- Constraint UNIQUE garante 1 schedule por projeto por fonte
- `ativo` permite pausar sem deletar
- CASCADE no DELETE da fonte remove schedules automaticamente

### Sync logs

Reutilizar tabela `sync_logs` existente. Adicionar valor `'scheduled'` ao campo `tipo` para distinguir syncs agendados de manuais (`'full'`, `'project'`).

## Backend

### Domain — `SyncSchedule`

```go
type SyncSchedule struct {
    ID           uuid.UUID `json:"id"`
    FonteDadosID uuid.UUID `json:"fonte_dados_id"`
    ProjectKey   string    `json:"project_key"`
    Horarios     []string  `json:"horarios"`
    Ativo        bool      `json:"ativo"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}
```

### Repository — `sync_schedule.go`

| Método | Descrição |
|--------|-----------|
| `Upsert(ctx, fonteID, projectKey, horarios []string) (*SyncSchedule, error)` | INSERT ON CONFLICT UPDATE horarios e updated_at |
| `Delete(ctx, fonteID, projectKey) error` | Remove schedule |
| `ListByFonte(ctx, fonteID) ([]SyncSchedule, error)` | Lista todos schedules da fonte |
| `GetDueSchedules(ctx, horaMinuto string) ([]SyncSchedule, error)` | Retorna schedules ativos cujo array `horarios` contém o horário informado |
| `SetAtivo(ctx, id uuid.UUID, ativo bool) error` | Toggle ativo/inativo |

Query para `GetDueSchedules`:
```sql
SELECT * FROM sync_schedules
WHERE ativo = true AND horarios @> $1::jsonb
```
Onde `$1` é `'"08:30"'` (string JSON quoted).

### Service — `scheduler.go`

```go
type SchedulerService struct {
    syncSvc      *SyncService
    scheduleRepo *repository.SyncScheduleRepository
    logger       *zap.Logger
    lastFired    map[uuid.UUID]string  // dedup: scheduleID → último horário disparado
}
```

**`Start(ctx context.Context)`** — goroutine com `time.NewTicker(1 * time.Minute)`:

1. Cada tick: `hora := time.Now().Format("15:04")`
2. `schedules := GetDueSchedules(ctx, hora)`
3. Para cada schedule:
   - Checa `lastFired[schedule.ID] != hora` (dedup de minuto)
   - Se novo: `go syncService.SyncProject(ctx, schedule.FonteDadosID, schedule.ProjectKey)`
   - Atualiza `lastFired[schedule.ID] = hora`
4. Limpa entries antigas de `lastFired` periodicamente (a cada hora)

**Guard contra duplicatas:** `SyncProject` já checa `HasRunningSync` e retorna `ErrSyncAlreadyRunning`. Scheduler ignora esse erro silenciosamente (log warn).

**Graceful shutdown:** Context cancellation para ticker e goroutine.

### Handler — `sync_schedule.go`

| Rota | Método | Descrição |
|------|--------|-----------|
| `GET /api/v1/sync/schedules` | `ListSchedules` | Query param `fonte_dados_id` obrigatório. Retorna `[]SyncSchedule` |
| `PUT /api/v1/sync/schedules` | `UpsertSchedule` | Body JSON: `{fonte_dados_id, project_key, horarios}`. Upsert schedule |
| `DELETE /api/v1/sync/schedules` | `DeleteSchedule` | Query params `fonte_dados_id` + `project_key`. Remove schedule |
| `PATCH /api/v1/sync/schedules/:id/toggle` | `ToggleSchedule` | Toggle campo `ativo` |

**Validação no handler:**
- `horarios`: máximo 4 itens, cada um regex `^([01]\d|2[0-3]):[0-5]\d$`
- `project_key` e `fonte_dados_id` obrigatórios
- `fonte_dados_id` deve ser UUID válido

### Tipo no sync_log

`SyncProject` recebe parâmetro adicional `tipo string` ou scheduler chama variante que seta `tipo = 'scheduled'` no log. Alternativa mais simples: adicionar campo `origem` ao `SyncLog` (`"manual"` | `"scheduled"`), sem alterar `tipo` existente.

Decisão: adicionar coluna `origem VARCHAR(20) DEFAULT 'manual'` à tabela `sync_logs`. Valores: `'manual'`, `'scheduled'`.

## Frontend

### UI no card da fonte (aba "Fontes de Dados")

Seção "Sync Agendado" dentro de cada card, após botões existentes:

1. **Botão "Agendar Sync"** (ícone relógio) — expande/colapsa seção inline
2. **Seção expandida:**
   - Lista projetos da fonte com checkbox (carregados via `GET /sync/projects`)
   - Para cada projeto selecionado/já agendado: linha com nome + inputs `type="time"` (até 4)
   - Botão "+" adiciona horário (máx 4), "×" remove
   - Toggle on/off por projeto (campo `ativo`)
   - Botão "Salvar" — PUT para cada projeto com horários configurados
3. **Indicador no card:** Badge "Agendado" com ícone relógio quando fonte tem schedules ativos
4. **Próximo sync:** Texto "Próximo sync: HH:MM" calculado no frontend a partir dos horários configurados

### Fluxo do usuário

1. Acessa aba "Fontes de Dados"
2. No card da fonte, clica "Agendar Sync"
3. Seção expande mostrando projetos disponíveis
4. Seleciona projetos, configura horários para cada
5. Clica "Salvar"
6. Card mostra badge "Agendado" e próximo horário
7. Syncs disparam automaticamente nos horários configurados
8. Resultados aparecem nos logs de sync (com origem "scheduled")

## Scheduler Lifecycle

### Startup (`main.go`)

```
scheduleRepo = repository.NewSyncScheduleRepository(pool)
schedulerSvc = service.NewSchedulerService(syncService, scheduleRepo, logger)
go schedulerSvc.Start(ctx)  // antes do HTTP server
```

### Shutdown

```
ctx cancel → ticker.Stop() → goroutine encerra
```

### Timezone

Usa timezone do servidor. Horários salvos como `"HH:MM"` sem info de timezone. Servidor e usuários assumidos na mesma timezone.

## Arquivos a criar/modificar

### Novos
- `backend/migrations/000011_sync_schedules.up.sql`
- `backend/migrations/000011_sync_schedules.down.sql`
- `backend/internal/domain/sync_schedule.go`
- `backend/internal/repository/sync_schedule.go`
- `backend/internal/service/scheduler.go`
- `backend/internal/handler/sync_schedule.go`

### Modificados
- `backend/cmd/api/main.go` — inicializar scheduler, registrar rotas
- `backend/internal/service/sync.go` — aceitar `origem` param no SyncProject
- `backend/internal/repository/sync.go` — passar `origem` ao criar sync_log
- `frontend/index.html` — UI de agendamento no card da fonte
- `backend/migrations/000011_sync_schedules.up.sql` — também adiciona coluna `origem` em `sync_logs`
