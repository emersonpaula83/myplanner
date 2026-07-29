# Soft-delete de Cards Ausentes + Tela de Tarefas + Filtros em Projetos

## Objetivo

Detectar cards removidos do Jira durante sync geral, marcando-os como removidos (soft-delete). Criar tela de Tarefas como guardrail para verificar sync. Adicionar filtros à tela de Projetos.

## 1. Migration

```sql
ALTER TABLE tarefas ADD COLUMN removido_em TIMESTAMPTZ NULL;
ALTER TABLE tarefas ADD COLUMN motivo_remocao TEXT NULL;
```

Mesma tabela cobre épicos e tarefas (ambos são registros em `tarefas`).

## 2. Sync Geral — Soft-delete por ausência

**Escopo:** Apenas `executSync` (sync geral via menu Fontes de Dados). `executSyncProject` e `SyncProjectTasks` (sync pontual do modal de alocação) **não** fazem remoção.

**Lógica por projeto:**
1. Após processar todas issues retornadas do Jira para um projeto (sem erro de fetch), coletar set de `jira_id` que vieram no response
2. Buscar no banco todos `jira_id` existentes para aquele `fonte_dados_id` + `projeto_id` onde `removido_em IS NULL`
3. IDs existentes no banco mas ausentes do response → `UPDATE SET removido_em = NOW(), motivo_remocao = 'removido do jira'`
4. IDs que estavam marcados como removidos mas reapareceram no response → `UPDATE SET removido_em = NULL, motivo_remocao = NULL` (ressurgiu)

**Condição de segurança:** Só executa soft-delete se `GetIssuesByProjects` retornou sem erro para aquele projeto. Se houve erro de fetch, pula a verificação de ausência (evita falso positivo por falha de rede/API).

**Log:** Registrar quantidade de soft-deletes e re-aparições via zap logger.

## 3. Nova Tela: Tarefas

### Sidebar
Novo item no menu lateral, após "Lista de Projetos":
- Label: "Tarefas"
- Ícone: usar pattern existente do sidebar

### Layout
Tabela com colunas:
| Coluna | Fonte |
|--------|-------|
| Ticket | `numero_ticket` |
| Resumo | `resumo` (truncado) |
| Tipo | `tipo` |
| Status | `status` |
| Equipe | via `equipe_membros` → `equipes.nome` (do responsável) |
| Produto | via `tarefa_produtos` → `produtos.nome` |
| Responsável | `membros.nome` via `responsavel_id` |
| Removido | badge Sim/Não baseado em `removido_em IS NOT NULL` |
| Data Remoção | `removido_em` formatado pt-BR, ou `--` |
| Última Sync | `updated_at` formatado pt-BR |

### Filtros
- **Equipe**: dropdown com equipes (via `/equipes`)
- **Produto**: dropdown com produtos (via `/allocation/products`)
- **Pessoa**: dropdown com membros (filtrado por equipe se equipe selecionada)
- **Removido**: dropdown com "Não" (default), "Sim", "Todos"
- **Busca**: input text para pesquisar por ticket ou resumo

### Paginação
Backend paginado: `?page=1&per_page=50`. Frontend com botões Anterior/Próximo e indicador "Mostrando X-Y de Z".

### Ação: Excluir Definitivamente
- Botão vermelho de lixeira **apenas** em tarefas com `removido_em` preenchido
- Confirmação: "Tem certeza? Esta ação é irreversível."
- Backend: `DELETE FROM tarefas WHERE id = $1 AND removido_em IS NOT NULL` (condição dupla de segurança)
- Cascade: `ON DELETE CASCADE` em `tarefa_produtos` e demais FKs já deve existir; verificar

### Endpoint Backend
- `GET /api/v1/tarefas` — listagem paginada com filtros query params:
  - `equipe_id` (UUID, opcional)
  - `produto_nome` (string, opcional)
  - `responsavel_id` (UUID, opcional)
  - `removido` (`sim`/`nao`/`todos`, default `nao`)
  - `busca` (string, opcional — ILIKE em numero_ticket e resumo)
  - `page` (int, default 1)
  - `per_page` (int, default 50, max 100)
- `DELETE /api/v1/tarefas/{id}` — hard delete (só se `removido_em IS NOT NULL`)

## 4. Tela Projetos (existente) — Novos Filtros

Adicionar aos filtros existentes da tela "Lista de Projetos":
- **Equipe**: dropdown (já existe parcialmente)
- **Produto**: dropdown com produtos
- **Responsável**: dropdown com membros
- **Removido**: dropdown "Não" (default) / "Sim" / "Todos"

Quando Removido = "Sim" ou "Todos", mostrar coluna extra "Data Remoção".

Default "Removido = Não" preserva comportamento atual — projetos removidos ficam ocultos sem mudança pra usuário.

## 5. Queries Existentes — Filtrar Removidos

Todas queries de UI que listam tarefas/épicos devem adicionar `AND removido_em IS NULL`:
- `GetEpicsByEquipeAndProduto` (Alocação de Projetos)
- `GetEpicTasks` (Modal de projeto)
- `GetEpicPeople` (Equipe do projeto)
- Capacidade/Sprint review (se aplicável)

## 6. Fora de Escopo

- Soft-delete de membros ou sprints
- Notificações de remoção
- Restore de tarefas removidas via UI (re-aparição automática na sync já cobre isso)
- Sync incremental (updatedSince) — hoje é always full-fetch
