# Alocação de Projetos — Melhorias e Correções

## Objetivo

Corrigir bugs existentes e adicionar funcionalidades ao módulo de Alocação de Projetos: encerramento de projetos, filtros por status e tipo_demanda, deduplicação de produtos, e fix de métricas quebradas.

## Bugs Identificados

### Bug 1: Status sempre "Não Planejado" no modal

`GetProjectDetail` chama `GetEpicsByEquipeAndProduto(ctx, equipeID, uuid.Nil)` — passando `uuid.Nil` como produtoID. Query exige match em `tarefa_produtos.produto_id = $2`, logo nunca encontra o épico. Fallback cai em `ProjectAllocation{Status: "nao_planejado"}`.

**Fix:** Novo método `GetEpicByID(ctx, epicID)` que busca diretamente por ID do épico, sem depender de produtoID.

### Bug 2: `pct_no_projeto` sempre 0%

`GetProjectDetail` popula `PersonAllocation` com `HorasNoProjeto` mas nunca calcula `HorasCapTotal` nem `PctNoProjeto`. Frontend exibe `0%`.

**Fix:** Calcular capacidade total da pessoa (via sprints ativas/futuras da equipe + `GetCapacity`) e dividir horas no projeto por capacidade.

## Banco de Dados

### Nova tabela `projeto_encerramentos`

```sql
CREATE TABLE projeto_encerramentos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    epic_id UUID NOT NULL REFERENCES tarefas(id),
    descricao TEXT NOT NULL,
    data_encerramento DATE NOT NULL,
    encerrado_por TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(epic_id)
);
```

- Constraint UNIQUE em `epic_id` — um encerramento por projeto
- Reabrir projeto = DELETE da row
- `encerrado_por` — string livre (email ou nome)
- `data_encerramento` — DATE (dia, não timestamp)
- Migration: `000017_projeto_encerramentos.up.sql` / `down.sql`

## Backend — Repository

### Novos métodos em `AllocationRepository`

- `CloseProject(ctx, epicID uuid.UUID, descricao string, dataEncerramento time.Time, encerradoPor string) error` — INSERT em `projeto_encerramentos`
- `ReopenProject(ctx, epicID uuid.UUID) error` — DELETE de `projeto_encerramentos WHERE epic_id = $1`
- `GetClosedEpicIDs(ctx, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error)` — retorna set dos encerrados
- `GetProjectClosure(ctx, epicID uuid.UUID) (*ProjectClosureRow, error)` — retorna dados do encerramento
- `GetEpicByID(ctx, epicID uuid.UUID) (*EpicAllocationRow, error)` — busca épico direto por ID, sem filtro de produto
- `GetProdutosComProjetosAtivos(ctx, equipeID uuid.UUID) ([]ProdutoRow, error)` — retorna produtos com épicos ativos vinculados à equipe. "Ativo" = épico com tipo IN ('Épico', 'Epico'), status NOT IN ('Cancelado', 'Rejeitada', 'Concluído'), E que NÃO existe em `projeto_encerramentos`. JOIN via `tarefas` (filhas do épico) → `tarefa_produtos` → `produtos`
- `GetPersonTotalAllocatedHours(ctx, membroIDs []uuid.UUID) (map[uuid.UUID]float64, error)` — para cada membro, soma `estimativa_tempo` de todas tarefas com `sprint_id IS NOT NULL` e status NOT IN ('Cancelado', 'Rejeitada'). Retorna horas (/ 3600)

### Tipos novos

```go
type ProjectClosureRow struct {
    Descricao        string
    DataEncerramento time.Time
    EncerradoPor     string
    CreatedAt        time.Time
}

type ProdutoRow struct {
    ID   uuid.UUID
    Nome string
}
```

### Modificação em `GetEpicsByEquipeAndProduto`

Novo parâmetro `statusFilter string` com valores `"ativos"` (default), `"encerrados"`, `"todos"`:

- `"ativos"`: adiciona `AND NOT EXISTS (SELECT 1 FROM projeto_encerramentos pe WHERE pe.epic_id = e.id)`
- `"encerrados"`: adiciona `AND EXISTS (SELECT 1 FROM projeto_encerramentos pe WHERE pe.epic_id = e.id)`
- `"todos"`: sem filtro adicional

## Backend — Service

### Novos structs

```go
type CloseProjectRequest struct {
    Descricao        string `json:"descricao"`
    DataEncerramento string `json:"data_encerramento"` // formato "2026-07-27"
}

type ProjectClosure struct {
    Descricao        string    `json:"descricao"`
    DataEncerramento time.Time `json:"data_encerramento"`
    EncerradoPor     string    `json:"encerrado_por"`
}
```

### Novos métodos em `AllocationService`

- `CloseProject(ctx, epicID uuid.UUID, req CloseProjectRequest, encerradoPor string) error` — parse da data, chama repo.CloseProject
- `ReopenProject(ctx, epicID uuid.UUID) error` — chama repo.ReopenProject
- `GetFilteredProducts(ctx, equipeID uuid.UUID) ([]ProdutoRow, error)` — chama repo.GetProdutosComProjetosAtivos

### Modificações

- `ListProjectAllocations(ctx, equipeID, produtoID uuid.UUID, statusFilter string)` — passa statusFilter para repo. Adiciona campo `Encerrado bool` e `Encerramento *ProjectClosure` em `ProjectAllocation` para projetos encerrados
- `GetProjectDetail` — usa `repo.GetEpicByID(epicID)` em vez de buscar por equipe+produto. Calcula `pct_no_projeto` para cada pessoa: novo repo method `GetPersonTotalAllocatedHours(ctx, membroIDs []uuid.UUID)` retorna soma de todas estimativas em tarefas com sprint atribuída (não canceladas). `pct = horas_neste_projeto / horas_totais_todos_projetos * 100`. Se total = 0, pct = 0

### Campos adicionais em `ProjectAllocation`

```go
Encerrado    bool             `json:"encerrado"`
Encerramento *ProjectClosure  `json:"encerramento,omitempty"`
```

## Backend — Handler

### Novas rotas

- `POST /allocation/projects/{epicId}/close` — handler `CloseProject`. Body: `CloseProjectRequest`. Responde 200 em sucesso, 409 se já encerrado
- `DELETE /allocation/projects/{epicId}/close` — handler `ReopenProject`. Responde 200 em sucesso, 404 se não encerrado
- `GET /allocation/products?equipe_id=X` — handler `ListFilteredProducts`. Retorna `[{id, nome}]`

### Modificações

- `ListProjects` — aceita query param `status=ativos|encerrados|todos` (default: `ativos`)

## Frontend

### Filtros

- Novo select `#alloc-status` com opções: "Ativos" (default), "Encerrados", "Todos". Posição: após equipe, antes de produto
- `onAllocFilterChange()`:
  - Envia `&status=X` na query de projetos
  - Troca `api('/produtos')` por `api('/allocation/products?equipe_id=' + allocEquipeId)` — só produtos com projetos ativos na equipe

### Boxes — Agrupamento por tipo_demanda

- `renderAllocationBoxes` agrupa projetos em 3 seções verticais: **Metas**, **Compromissos**, **Iniciativas**
- Cada seção: header `<h3>` com título + grid de boxes abaixo
- Seção vazia: omitida (não renderiza)
- Boxes encerrados (quando filtro = "Encerrados" ou "Todos"): opacidade reduzida (0.6), badge "Encerrado" com cor distinta

### Botão "Encerrar Projeto"

- Botão no canto superior direito de cada box ativo (ícone 🔒)
- `event.stopPropagation()` para não abrir modal do projeto
- Click abre mini-modal inline com:
  - Textarea para descrição do encerramento
  - Input date para data de encerramento (default: hoje)
  - Botões "Cancelar" / "Confirmar Encerramento"
- Após confirmar: `POST /allocation/projects/{epicId}/close`, recarrega lista
- Para projetos encerrados: botão "Reabrir" (ícone 🔓), chama `DELETE .../close`, recarrega

### Modal — Correções

- Renomear "Tarefas Completas" → "Tarefas Planejadas"
- Status badge correto (fix via `GetEpicByID`)
- `pct_no_projeto` exibe valor real calculado pelo backend

## Associação Épico ↔ Equipe (N:N)

### Problema
Alguns épicos não possuem membros de nenhuma equipe atribuídos como responsáveis, tornando-os invisíveis nos filtros de equipe tanto na tela de Projetos quanto na Alocação.

### Solução
Nova tabela `epico_equipes` (N:N) permite associação manual de épicos a equipes. No modal de metadados do projeto (onde se edita apelido), checkboxes permitem selecionar 1 ou N equipes.

### Banco de Dados

```sql
CREATE TABLE epico_equipes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    epico_id UUID NOT NULL REFERENCES tarefas(id) ON DELETE CASCADE,
    equipe_id UUID NOT NULL REFERENCES equipes(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(epico_id, equipe_id)
);
```

Migration: `000021_epico_equipes`

### Backend
- `MetadataProjetoRequest` ganha `EquipeIDs []uuid.UUID`
- `UpdateProjetoMetadata` handler salva associações em `epico_equipes` (DELETE all + INSERT)
- `GET /projetos/{id}/equipes` retorna `[]uuid.UUID` das equipes associadas
- Queries de equipe usam lógica OR: épico aparece se `fonte_dados_id` match (implícito via membros) **OU** `epico_equipes` match (explícito manual)
- Filtro "Todas as Equipes": `equipeID = uuid.Nil` → bypass do filtro de equipe

### Frontend
- Modal projmeta: seção de checkboxes com todas as equipes, pré-marcadas se associadas
- Dropdown de equipe na alocação: opção "Todas as Equipes" no topo
- Tela de Projetos (ListarEpicos): query também considera `epico_equipes`

## Restrições Globais

- Frontend: `var`/`function` only, NO ES6+. XSS: `esc()` para texto, `escAttr()` para atributos
- CSS custom properties: `--surface`, `--text-primary`, `--accent`, `--border`, `--text-secondary`
- Dark mode: `@media (prefers-color-scheme: dark)` + `:root[data-theme="dark"]` + `:root[data-theme="light"]`
- Go: chi router, pgx/v5, zap logger
- Sem commits automáticos — mudanças ficam unstaged
