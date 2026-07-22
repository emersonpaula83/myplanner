# Timeline Exclude Closed Sprints — Design Spec

## Objetivo

Excluir sprints concluidas (estado=closed) do relatorio Sprints Timeline, mantendo sprints vazias e futuras visiveis com barras de capacidade.

## Escopo

- Filtrar sprints com `estado = "closed"` no service layer (`GetSprintsTimeline`)
- Manter logica `includeEmpty` existente no repository (sem mudanca)
- Sprints sem datas continuam ignoradas (sem mudanca)
- Frontend sem mudanca — barra normal pra sprints vazias (azul capacidade, 0 alocado)

## Fora de Escopo

- Filtro por projeto no timeline
- Agrupamento visual por projeto
- Estimativa de posicao pra sprints sem datas
- Visual diferenciado pra sprints vazias

## Implementacao

### Service Layer

Arquivo: `backend/internal/service/sprint.go`, funcao `GetSprintsTimeline`

No loop de filtragem por ano (linhas 767-775), adicionar condicao para excluir sprints com estado `closed`:

```go
for _, sp := range allSprints {
    if sp.DataInicio == nil || sp.DataFim == nil {
        continue
    }
    if sp.DataFim.Before(anoInicio) || sp.DataInicio.After(anoFim) {
        continue
    }
    if sp.Estado != nil && *sp.Estado == "closed" {
        continue
    }
    sprints = append(sprints, sp)
}
```

### Frontend

Nenhuma mudanca. Sprint vazia renderiza como barra azul (capacidade) sem barra verde (alocado). Tooltip mostra 0h alocadas.

## Testes

- Verificar visualmente que sprints closed nao aparecem no timeline
- Verificar que sprints vazias (active/future) aparecem com barra de capacidade
- Verificar tooltip mostra dados corretos pra sprints vazias
