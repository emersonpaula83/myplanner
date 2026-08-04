# Redesign de Status de Projetos

## Resumo

Substituir o sistema de status atual (Planejado/Em Planejamento/Não Planejado/Encerrado) por 4 status semânticos calculados automaticamente, com filtros atualizados e melhorias visuais nos cards.

## Status do Projeto

Calculados top-down por prioridade:

| Status | Valor | Condição | Cor Badge |
|--------|-------|----------|-----------|
| Desconsiderado | `desconsiderado` | Existe em `projeto_encerramentos` (manual) | Cinza escuro, card opacity 0.5 |
| Concluído | `concluido` | 100% tarefas ativas com `status_categoria = 'done'` OU status cancelado/rejeitada | Verde |
| Em Andamento | `em_andamento` | Pelo menos 1 tarefa com `status_categoria` = `indeterminate` ou `done` | Azul |
| Não Iniciado | `nao_iniciado` | Todas tarefas em backlog (nenhuma em andamento/done) | Cinza neutro |

### Lógica de cálculo

**Detail-level** (`GetProjectDetail`): já possui lista de tarefas com `status_categoria` — iterar e classificar.

**List-level** (`GetEpicsByEquipeAndProduto`): adicionar subqueries ao SQL:
- `filhas_ativas`: COUNT tarefas não-canceladas/não-rejeitadas
- `filhas_concluidas`: COUNT tarefas com `status_categoria = 'done'` OU status cancelado/rejeitada
- `filhas_em_andamento`: COUNT tarefas com `status_categoria IN ('indeterminate', 'done')`
- Lógica: se `desconsiderado` (via closedMap) → desconsiderado; se `filhas_ativas == 0` → nao_iniciado; se `filhas_concluidas == filhas_ativas` → concluido; se `filhas_em_andamento > 0` → em_andamento; senão → nao_iniciado

## Filtro (Dropdown)

Substitui o atual `<select id="alloc-status">` com opções: Ativos / Encerrados / Todos.

| Opção | Valor | Resultado |
|-------|-------|-----------|
| Em Andamento | `em_andamento` | Status `em_andamento` + `nao_iniciado` (tudo não concluído/não desconsiderado) |
| Em Atraso | `em_atraso` | `data_limite < hoje` AND status != `concluido` AND != `desconsiderado` |
| Concluídos | `concluidos` | Só status `concluido` |
| Desconsiderados | `desconsiderados` | Só status `desconsiderado` |
| Todos | `todos` | Tudo. Desconsiderados renderizam com `opacity: 0.5` |

**Default**: "Em Andamento".

**Backend**: filtro `em_atraso` = WHERE NOT EXISTS em `projeto_encerramentos` AND não-concluido AND `data_limite < CURRENT_DATE`.

## Card do Projeto

### Mudanças visuais em `renderAllocationBoxes`:

1. **Texto**: "Limite: " → "Data Limite: "
2. **Tipo de Demanda**: adicionar abaixo do título com ícone — `🎯 Meta` / `🤝 Compromisso` / `⬆️ Iniciativa` (font-size 12px, cor secondary)
3. **Badge de status**: substituir badges atuais pelos 4 novos status
4. **Tooltip cadeado**: "Encerrar Projeto" → "Desconsiderar projeto"
5. **Form**: título "Encerrar Projeto" → "Desconsiderar Projeto", placeholder "Descrição do encerramento..." → "Motivo..."
6. **Opacity**: cards com status `desconsiderado` no filtro "Todos" recebem `opacity: 0.5`
7. **Barras de progresso**: mantidas como estão (% estimado, % planejado)

## Modal de Detalhes

1. **Badge de status no header**: mesmos 4 novos status
2. **Accordion das seções de tarefas**: já implementado — Não Alocadas, Estimadas sem Pessoa, Tarefas Planejadas, Concluídas (collapsed por default)

## Impacto em Arquivos

### Backend
- `backend/internal/repository/allocation.go` — novas subqueries para contadores de status por épico; ajustar filtro `statusFilter`
- `backend/internal/service/allocation.go` — nova lógica de status (substituir planejado/em_planejamento/nao_planejado); campos novos no `ProjectAllocation`

### Frontend
- `frontend/index.html` — dropdown de filtro, renderização de cards, badges, tooltip, form, modal header
