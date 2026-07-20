# Filtro Global de Membros Desligados

## Contexto

O campo `data_desligamento` existe na tabela `membros` (migration 000009) mas só é filtrado em 3 das 9 funções que usam membros para cálculos. Membros desligados continuam contando em sprint capacity, equipe resumo, e timeline single-equipe.

## Regra de Negócio

Membro com `data_desligamento` preenchida é excluído de cálculos quando `data_desligamento <= data_referencia`.

Sprints passadas (encerradas antes do desligamento) mantêm o membro no histórico.

### Data de Referência por Módulo

| Módulo | Data referência |
|--------|----------------|
| Sprint capacity | `data_fim` da sprint |
| Timeline capacidade mensal | início do mês (`YYYY-MM-01`) |
| Timeline Gantt membros | `inicio_ano` (Jan 1) — já implementado |
| Equipe resumo | `data_fim` do período selecionado |

## Alterações

### Backend — Queries SQL (Abordagem A: filtro direto no SQL)

Padrão de filtro: `AND (m.data_desligamento IS NULL OR m.data_desligamento > $N)`

#### sprint.go

1. **`GetMembrosEquipeIDs(equipeID, dataFim)`** — adicionar JOIN com `membros`, receber `dataFim`, filtrar desligados
2. **`GetMembrosEquipeInfo(equipeID, dataFim)`** — receber `dataFim`, adicionar filtro
3. **`GetMembrosFromSprint(sprintID)`** — sem alteração (preserva histórico de tarefas atribuídas)

#### timeline.go

4. **`ContarMembrosAtivosEquipe(equipeID, inicioAno)`** — adicionar filtro (alinhar com `ContarMembrosAtivosEquipes` que já filtra)
5. **`BuscarAusenciasMensais(equipeID, ano)`** — adicionar filtro (alinhar com `BuscarAusenciasMensaisEquipes` que já filtra)

#### equipe.go

6. **`GetMembrosEquipe(equipeID)`** — incluir `data_desligamento` no SELECT, adicionar filtro com data referência

### Backend — Service Layer

#### sprint.go — `GetCapacity()`

- Passar `sprint.DataFim` para `GetMembrosEquipeIDs()` e `GetMembrosEquipeInfo()`
- Membros vindos de `GetMembrosFromSprint()` que estão desligados antes de `sprint.DataFim`: excluir de `HorasTotalSprint` mas manter visíveis com flag `desligado`

### Frontend

- Sprint capacity: membro com flag `desligado` renderiza com opacidade reduzida + label "(Desligado)"
- Não entra nos totais de capacidade da equipe

### Sem Alteração

- `membro.go` `List()` / `Search()` — listagem geral, não é cálculo
- `GetMembrosFromSprint()` — preserva histórico
- Funções multi-equipe de timeline — já filtram corretamente
