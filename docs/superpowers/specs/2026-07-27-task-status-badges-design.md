# Status Badges nas Tarefas do Modal de Alocação

## Objetivo

Exibir badge colorido de status em cada tarefa dentro do modal de detalhes do projeto (Alocação de Projetos). Importar campo "flagged" do Jira para identificar tarefas bloqueadas.

## Mapeamento de Status

| Badge | Regra | Cor |
|-------|-------|-----|
| Bloqueada | `marcacao = true` (prioridade sobre outros) | Vermelho `#dc3545` / `rgba(220,53,69,0.15)` |
| Concluído | `status_categoria = 'done'` | Verde `var(--accent)` / `var(--accent-soft)` |
| Em Andamento | `status_categoria = 'indeterminate'` | Azul `var(--blue)` / `var(--blue-soft)` |
| Backlog | qualquer outro valor | Cinza `var(--text-tertiary)` / `var(--chip-bg)` |

Bloqueada sobrepõe qualquer status — se `marcacao = true`, badge é vermelho independente de `status_categoria`.

## Escopo

### 1. Migration 000023

```sql
ALTER TABLE tarefas ADD COLUMN marcacao BOOLEAN NOT NULL DEFAULT false;
```

### 2. Jira Sync — importar campo `flagged`

- Na struct `JiraIssue` (ou no mapeamento de fields), ler `fields.flagged` (booleano).
- No `upsertTarefas` do sync, gravar valor em `tarefas.marcacao`.
- Campo é `bool` simples na API REST do Jira: `true` quando tarefa está flagged, `false` ou ausente quando não.

### 3. Backend — `GetEpicTasks`

- Adicionar `marcacao` ao `TaskAllocationRow` struct com json tag `"marcacao"`.
- Adicionar `t.marcacao` ao SELECT da query em `GetEpicTasks`.
- Adicionar `t.marcacao` ao Scan.

### 4. Frontend — CSS

Reusar pattern `.capacity-tarefa-status` existente (lines 638-640). Adicionar variante `.blocked`:

```css
.capacity-tarefa-status.blocked {
  background: rgba(220,53,69,0.15);
  color: #dc3545;
}
```

Dark mode: mesmas cores (vermelho funciona em ambos temas).

### 5. Frontend — Helper JS

```javascript
function taskStatusBadgeHtml(task) {
  if (task.marcacao) return '<span class="capacity-tarefa-status blocked">Bloqueada</span>';
  var cat = task.status_categoria || '';
  if (cat === 'Done') return '<span class="capacity-tarefa-status done">Concluído</span>';
  if (cat === 'In Progress') return '<span class="capacity-tarefa-status inprogress">Em Andamento</span>';
  return '<span class="capacity-tarefa-status todo">Backlog</span>';
}
```

### 6. Frontend — Renderização

Badge aparece nas 3 seções do modal:

- **Tarefas Não Alocadas** — em `renderAllocTaskEditable`, ao lado do `numero_ticket`
- **Estimadas sem Pessoa** — mesmo `renderAllocTaskEditable`
- **Tarefas Planejadas** — na renderização inline, ao lado do `numero_ticket`

### 7. Backend — `GetEpicTasks` precisa retornar `status_categoria`

Verificar se `status_categoria` já é retornado na query. Se não, adicionar ao SELECT e ao struct.

### 8. Frontend — Emojis nos títulos de seção (tipo_demanda)

Em `renderAllocationBoxes`, adicionar emojis aos títulos das seções de tipo_demanda:

| Seção | Título |
|-------|--------|
| Metas | 🎯 Metas |
| Compromissos | 🤝 Compromissos |
| Iniciativas | ⬆️ Iniciativas |

Aplicar no texto do `<h3>` ou equivalente que renderiza o nome da seção.

### 9. Bugfix — Cálculo `pct_no_projeto` incorreto

**Problema:** `service/allocation.go:297` calcula `HorasNoProjeto / GetPersonTotalAllocatedHours * 100` — divide pelas horas totais da pessoa em TODOS os projetos. Resultado não faz sentido pro contexto do modal.

**Correto:** `HorasNoProjeto / TotalHorasDoProjeto * 100` — onde `TotalHorasDoProjeto` é a soma de estimativas de todas as tarefas do épico (não canceladas/rejeitadas).

**Fix:** No `GetProjectDetail` do service, calcular o total de horas do projeto (já disponível via tasks) e usar como denominador. Remover chamada a `GetPersonTotalAllocatedHours` se não for mais necessária.

### 10. Bugfix — Sprints do modal carregam só da primeira equipe

**Problema:** `frontend/index.html:6174` — `allocSprints` carrega apenas 1x (`if (allocSprints.length === 0 && allocEquipeId)`). Se trocar equipe, sprints permanecem da equipe anterior. Se "Todas as Equipes" selecionada, `allocEquipeId` é vazio e sprints nunca carregam.

**Fix:** Resetar `allocSprints = []` em `onAllocFilterChange` quando equipe muda. No `openProjectModal`, sempre recarregar sprints se equipe mudou (guardar `lastSprintEquipeId` e comparar). Se `allocEquipeId` vazio (Todas), passar equipe_id vazio pro backend — backend deve retornar sprints de todas as equipes (ou tratar uuid.Nil como bypass).

## Fora de Escopo

- Filtro por status no modal
- Contadores de status no header do modal
- Edição de status/marcação pelo frontend
