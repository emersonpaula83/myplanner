# Alocação de Projetos — Design Spec

## Objetivo

Módulo para visualizar e alocar tarefas de projetos (épicos) para sprints e pessoas, considerando capacity individual, férias, folgas, ausências e feriados. Permite ao coordenador/gestor ver o panorama de alocação de cada projeto e agir diretamente — atribuindo sprint, estimativa e pessoa — com write-back otimista para o Jira.

## Contexto

O MyPlanner já possui:
- Cálculo de capacity por sprint (`SprintService.GetCapacity`) — 6h/dia útil, descontando feriados e ausências
- Épicos tratados como "projetos" no timeline (`tarefas WHERE tipo = 'Epico'`)
- Detecção GDPTC recursiva (portfólio unificado) via CTE em `parent_id`
- Jira write: `AssignIssue`, `AddComment`, `CreateSprint`
- Sync de tarefas via JQL por project key com `processIssue()` para upsert

Faltam: `MoveToSprint`, `UpdateTimeEstimate` no Jira client, e toda a camada de alocação.

## Premissas

- "Projeto" = Épico (`tarefas` com `tipo = 'Epico'`), não `projetos` (Jira projects)
- Config global por instância (sem multi-tenant por ora)
- Sprints disponíveis para alocação: futuras/ativas do board vinculado à equipe
- Estimativa obrigatória para alocar tarefa (horas, convertidas em segundos internamente)
- Assignee opcional — tarefa pode ser alocada a sprint sem pessoa
- Write-back otimista: update local imediato, Jira em background, rollback se falha

## Arquitetura

### Filtragem: Equipe + Produto

**Equipe** → busca épicos pela `fonte_dados_id` dos membros da equipe (mesmo pattern do timeline). Não filtra por assignee — épicos podem não ter ninguém alocado ainda.

**Produto** → filtra épicos que possuem pelo menos uma tarefa filha com o produto selecionado (via `tarefa_produtos`).

Ambos obrigatórios. Sem seleção, página mostra mensagem "Selecione equipe e produto".

### Métricas por Épico

| Métrica | Fórmula | Exemplo |
|---------|---------|---------|
| **% Estimado** | `count(filhas com estimativa) / count(total filhas) × 100` | 8/10 = 80% |
| **% Planejado** | `sum(horas filhas com estimativa E em sprint futura/ativa) / sum(horas de TODAS filhas estimadas) × 100` | 150h/200h = 75% |
| **Alerta** | `count(filhas sem estimativa)` | "2 tarefas sem estimativa" |

**Status derivado do % Planejado:**
- `100%` → **Planejado** (badge verde)
- `> 0%` → **Em Planejamento** (badge amarelo)
- `0%` → **Não Planejado** (badge cinza)

### Ordenação dos Boxes

1. Prioridade do épico: Highest(1) > High(2) > Medium(3) > Low(4) > Lowest(5) > null(6)
2. Tipo de demanda: Meta > Compromisso > Iniciativa > outros

### GDPTC (Portfólio)

CTE recursivo (até 10 níveis) na cadeia `parent_id`, buscando ancestral com `numero_ticket LIKE 'GDPTC-%'`. Épicos com ancestral GDPTC recebem estrela amarela + tooltip "Projeto de Portfólio Unificado".

### Cor da Caixa

Baseada no `% Planejado` — gradiente vermelho→amarelo→verde usando CSS custom properties (mesmo approach do Timeline Capacity):
- 0-30%: vermelho
- 31-70%: amarelo/laranja  
- 71-100%: verde

## Jira Client Extensions

### Novos métodos na interface `Client`

**`MoveToSprint(ctx context.Context, sprintJiraID int, issueKey string) error`**
- REST: `POST /rest/agile/1.0/sprint/{sprintId}/issue`
- Body: `{"issues": ["issueKey"]}`

**`UpdateTimeEstimate(ctx context.Context, issueKey string, seconds int) error`**
- REST: `PUT /rest/api/3/issue/{key}`
- Body: `{"fields": {"timetracking": {"originalEstimate": "Xh"}}}`
- Conversão: segundos → formato Jira (`3600 → "1h"`, `28800 → "1d"`)

### Fluxo de write no Jira (3 calls sequenciais)

```
1. UpdateTimeEstimate(ctx, "PROJ-123", 28800)   // obrigatório
2. MoveToSprint(ctx, 42, "PROJ-123")            // obrigatório
3. AssignIssue(ctx, "PROJ-123", "accountId")     // opcional
```

### Error handling

Chamadas são independentes. Se uma falha após outra ter sucesso:
- Campos já escritos no Jira ficam (aceitável — estimativa sem sprint é melhor que nada)
- Rollback do update local DB
- Toast de erro específico: "Falha ao mover para sprint" ou "Falha ao atribuir responsável"

## Backend Service & API

### AllocationService

Arquivo: `backend/internal/service/allocation.go`

```go
type AllocationService struct {
    repo           *repository.AllocationRepository
    sprintService  *SprintService
    syncService    *SyncService
    jiraFactory    ClientFactory
    oauthFactory   OAuthClientFactory
    oauthSvc       *jira.OAuthService
    rateLimit      int
    logger         *zap.Logger
}

func (s *AllocationService) ListProjectAllocations(ctx, equipeID, produtoID uuid.UUID) ([]ProjectAllocation, error)
func (s *AllocationService) GetProjectDetail(ctx, epicID, equipeID uuid.UUID) (*ProjectDetail, error)
func (s *AllocationService) AllocateTask(ctx, req AllocateTaskRequest) error
func (s *AllocationService) SyncProjectTasks(ctx, epicID uuid.UUID) error
func (s *AllocationService) GetAvailableSprints(ctx, equipeID uuid.UUID) ([]SprintOption, error)
```

### AllocationRepository

Arquivo: `backend/internal/repository/allocation.go`

```go
func (r *AllocationRepository) GetEpicsByEquipeAndProduto(ctx, equipeID, produtoID uuid.UUID) ([]EpicAllocationRow, error)
func (r *AllocationRepository) GetEpicTasks(ctx, epicID uuid.UUID) ([]TaskAllocationRow, error)
func (r *AllocationRepository) GetEpicPeople(ctx, epicID uuid.UUID) ([]PersonAllocationRow, error)
func (r *AllocationRepository) UpdateTaskAllocation(ctx, taskID uuid.UUID, sprintID uuid.UUID, assigneeID *uuid.UUID, estimateSeconds int) error
func (r *AllocationRepository) RollbackTaskAllocation(ctx, taskID uuid.UUID, prevSprintID *uuid.UUID, prevAssigneeID *uuid.UUID, prevEstimate *int) error
func (r *AllocationRepository) GetFutureSprintsByEquipe(ctx, equipeID uuid.UUID) ([]SprintOptionRow, error)
func (r *AllocationRepository) CheckGDPTCAncestors(ctx, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error)
```

### DTOs

```go
type ProjectAllocation struct {
    EpicID        uuid.UUID  `json:"epic_id"`
    NumeroTicket  string     `json:"numero_ticket"`
    Resumo        string     `json:"resumo"`
    Apelido       *string    `json:"apelido"`
    DataLimite    *time.Time `json:"data_limite"`
    Prioridade    *string    `json:"prioridade"`
    TipoDemanda   *string    `json:"tipo_demanda"`
    Produtos      []string   `json:"produtos"`
    PctEstimado   float64    `json:"pct_estimado"`
    PctPlanejado  float64    `json:"pct_planejado"`
    TarefasSemEst int        `json:"tarefas_sem_estimativa"`
    TotalTarefas  int        `json:"total_tarefas"`
    IsGDPTC       bool       `json:"is_gdptc"`
    Status        string     `json:"status"`
}

type ProjectDetail struct {
    Epic        ProjectAllocation    `json:"epic"`
    Pessoas     []PersonAllocation   `json:"pessoas"`
    NaoAlocadas []TaskAllocation     `json:"nao_alocadas"`
    Parciais    []TaskAllocation     `json:"parciais"`
    Completas   []TaskAllocation     `json:"completas"`
}

type PersonAllocation struct {
    MembroID       uuid.UUID `json:"membro_id"`
    Nome           string    `json:"nome"`
    HorasNoProjeto float64   `json:"horas_no_projeto"`
    HorasCapTotal  float64   `json:"horas_cap_total"`
    PctNoProjeto   float64   `json:"pct_no_projeto"`
}
// PctNoProjeto = HorasNoProjeto / HorasCapTotal × 100
// HorasNoProjeto = sum(estimativa_tempo) das tarefas desta pessoa neste épico
// HorasCapTotal = sum(HorasDisponiveis) nas sprints onde a pessoa tem tarefas deste épico

type TaskAllocation struct {
    TarefaID       uuid.UUID  `json:"tarefa_id"`
    NumeroTicket   string     `json:"numero_ticket"`
    Resumo         string     `json:"resumo"`
    Tipo           string     `json:"tipo"`
    TipoDemanda    *string    `json:"tipo_demanda"`
    Status         string     `json:"status"`
    EstimativaHoras *float64  `json:"estimativa_horas"`
    SprintID       *uuid.UUID `json:"sprint_id"`
    SprintNome     *string    `json:"sprint_nome"`
    SprintInicio   *time.Time `json:"sprint_inicio"`
    SprintFim      *time.Time `json:"sprint_fim"`
    ResponsavelID  *uuid.UUID `json:"responsavel_id"`
    ResponsavelNome *string   `json:"responsavel_nome"`
}

type SprintOption struct {
    ID     uuid.UUID `json:"id"`
    JiraID int       `json:"jira_id"`
    Nome   string    `json:"nome"`
    Inicio time.Time `json:"inicio"`
    Fim    time.Time `json:"fim"`
    Estado string    `json:"estado"`
}

type AllocateTaskRequest struct {
    TaskID        uuid.UUID  `json:"task_id"`
    SprintID      uuid.UUID  `json:"sprint_id"`
    AssigneeID    *uuid.UUID `json:"assignee_id"`
    EstimateHours float64    `json:"estimate_hours"`
    Force         bool       `json:"force"`
}
```

### API Endpoints

| Método | Path | Descrição |
|--------|------|-----------|
| GET | `/api/v1/allocation/projects?equipe_id=X&produto_id=Y` | Lista épicos com métricas |
| GET | `/api/v1/allocation/projects/{epicId}?equipe_id=X` | Detalhe do épico (pessoas + tarefas) |
| POST | `/api/v1/allocation/tasks/{taskId}/allocate` | Alocar tarefa (sprint + estimativa + pessoa) |
| POST | `/api/v1/allocation/projects/{epicId}/sync` | Sync tarefas do épico via Jira |
| GET | `/api/v1/allocation/sprints?equipe_id=X` | Sprints futuras/ativas do board |

### Fluxo AllocateTask (otimista)

1. Validar: `estimate_hours > 0` obrigatório, `sprint_id` obrigatório
2. Se `assignee_id` fornecido → buscar capacity da pessoa na sprint-alvo via `SprintService.GetCapacity`
   - Se alocação resultante > 100% e `force != true` → return `409 Conflict` com payload:
     ```json
     {"conflict": true, "membro_nome": "João", "sprint_nome": "Sprint 22", "pct_atual": 105}
     ```
   - Frontend mostra confirmação, re-envia com `force: true`
3. Salvar estado anterior da tarefa (para rollback)
4. UPDATE local DB: `estimativa_tempo`, `sprint_id`, `responsavel_id`
5. Goroutine em background:
   - Construir Jira client via `fonteDadosID` da tarefa
   - `UpdateTimeEstimate(ctx, issueKey, seconds)`
   - `MoveToSprint(ctx, sprintJiraID, issueKey)`
   - Se `assigneeID` → `AssignIssue(ctx, issueKey, accountID)` + `AddComment`
   - Se qualquer call falha → rollback local DB, log warning
6. Return `200 OK` com tarefa atualizada

### SyncProjectTasks

1. Buscar épico no DB → `numero_ticket` (ex: "PROJ-123")
2. Buscar `fonte_dados_id` → construir Jira client
3. JQL: `"Epic Link" = PROJ-123 OR parent = PROJ-123`
4. Para cada issue retornada: reutilizar `processIssue()` do sync service
5. Return count de tarefas processadas

## Frontend

### Página de Alocação

**Sidebar:** novo item "Alocação" no grupo "Relatórios".

**Filtros:** dois selects obrigatórios — Equipe e Produto. Equipes carregadas de `/api/v1/equipes`, produtos de `/api/v1/produtos` filtrados pela equipe.

**Grid de boxes:** CSS grid responsivo (`grid-template-columns: repeat(auto-fill, minmax(280px, 1fr))`). Cada box é um card clicável.

**Conteúdo do box:**
- Estrela amarela GDPTC (se aplicável) + `numero_ticket`
- `apelido` ou `resumo` (apelido tem prioridade)
- Data limite (se existir)
- Chips de produtos
- Barras visuais: `% Estimado` e `% Planejado`
- Alerta: "⚠ X tarefas sem estimativa" (se > 0)
- Badge de status: Planejado (verde) / Em Planejamento (amarelo) / Não Planejado (cinza)
- Background: cor gradiente baseada em `% Planejado`

### Modal de Detalhe

Abre ao clicar no box. Modal overlay com `max-width: 900px; max-height: 85vh; overflow-y: auto`.

**Header:**
- `numero_ticket`: "Resumo" + badge status
- Botão "Sincronizar Tarefas" (spinner durante sync)

**Seção 1 — Equipe Envolvida:**
Tabela com membros que têm tarefas no épico. Colunas: Nome, Horas no Projeto, % no Projeto.

**Seção 2 — Tarefas Não Alocadas:**
Tarefas sem estimativa OU sem sprint. Cada linha tem:
- Ticket + resumo + tipo
- Select "Sprint" (sprints futuras do board)
- Select "Pessoa" (membros da equipe, mostrando `nome (X% na sprint)`)
- Input "Estimativa" (horas, numérico)
- Botão ✓ alocar

**Seção 3 — Tarefas Parciais:**
Tarefas com estimativa + sprint, sem pessoa. Mesmos controles mas sprint/estimativa preenchidos.

**Seção 4 — Tarefas Completas:**
Tarefas com estimativa + sprint + pessoa. Somente leitura. Colunas: Ticket, Resumo, Horas, Sprint, Responsável.

**Seção 5 — Timeline (Gantt):**
- X-axis: meses do ano corrente (Jan-Dez), colunas proporcionais
- Y-axis: uma linha por tarefa alocada
- Barra: início da sprint → fim da sprint, cor do produto ou azul default
- Separador visual
- Tarefas não alocadas: barras vermelhas abaixo, ocupam ano todo
- Hover tooltip: `numero_ticket` + resumo + "Não Alocada" (se for o caso)
- Implementação: CSS grid/flexbox com divs posicionados, sem lib externa

### Modal de Confirmação de Capacity

Ao alocar tarefa com pessoa que ficará >100%:

```
┌─ Atenção ─────────────────────────────────────────┐
│                                                    │
│  O colaborador João Silva já está com 105% do     │
│  tempo alocado na Sprint 22.                       │
│  Deseja continuar mesmo assim?                     │
│                                                    │
│                      [Cancelar]  [Sim, continuar]  │
└────────────────────────────────────────────────────┘
```

### Fluxo de interação

```
navigate('alocacao')
  → loadAlocacao() → mostra filtros
    → user seleciona equipe + produto
      → loadProjectAllocations(equipeId, produtoId)
        → GET /allocation/projects → renderiza boxes
          → user clica box
            → openProjectModal(epicId, equipeId)
              → GET /allocation/projects/{epicId}
              → renderiza modal com 5 seções
                → user preenche sprint + estimativa + pessoa em tarefa
                  → click ✓
                    → se capacity check falha → modal confirmação
                    → POST /allocation/tasks/{taskId}/allocate
                    → refresh modal (re-fetch detail)
                → user clica "Sincronizar Tarefas"
                  → POST /allocation/projects/{epicId}/sync
                  → refresh modal
```

## Testes

- **Jira client**: unit tests para `MoveToSprint` e `UpdateTimeEstimate` (mock HTTP)
- **AllocationRepository**: integration tests com DB real (pattern existente)
- **AllocationService**: unit tests com mocks para capacity check, allocate flow, rollback
- **Handler**: unit tests para validation, 409 conflict response
- **Frontend**: `node --check` para syntax. Manual: filtros → boxes → modal → alocar → Gantt

## Segurança

- Todos endpoints protegidos por JWT (`middleware.AuthJWT`)
- `ProjetoFilter` middleware garante que usuário só vê equipes/projetos dos seus projetos
- Jira credentials nunca expostas ao frontend (client construído server-side)
- Input validation: `estimate_hours > 0`, UUIDs válidos, sprint pertence ao board da equipe

## Fora do Escopo

- Drag-and-drop de tarefas entre sprints no Gantt
- Alocação em batch (múltiplas tarefas de uma vez)
- Notificações push quando capacity excede
- Config per-equipe/per-tenant (depende de IAM futuro)
- Filtro por período/data no Gantt (sempre ano corrente)
- Edição de tarefas já alocadas (reatribuir sprint/pessoa) — ciclo futuro

## Global Constraints

- Frontend: vanilla JS, `var`/`function` only, sem ES6+. Usar `esc()` para texto dinâmico (XSS prevention)
- Backend: Go com chi router, pgx/v5, zap logger
- Não commitar automaticamente — deixar unstaged
- Dark mode: CSS custom properties + `@media (prefers-color-scheme: dark)` + `:root[data-theme="dark"]`
- Capacity: 6h por dia útil, feriados e ausências descontados (reutilizar `SprintService.GetCapacity`)
