# Burndown Chart — Sprint

**Data:** 2026-07-19
**Escopo:** Backend (novo endpoint) + Frontend (layout side-by-side + canvas chart)

## Contexto

Tela de Sprints tem espaço vazio à direita. Ao selecionar sprint e abrir capacity, gráfico de burndown aparece ao lado direito.

## Layout

Quando abre capacity de uma sprint, `sprints-content` vira grid 2 colunas:
- Esquerda (60%): capacity panel existente
- Direita (40%): painel de gráficos (burndown primeiro)
- Telas <1200px: stacking vertical

## API

`GET /api/v1/sprints/{id}/burndown?equipe={equipeID}`

Response:
```json
{
  "sprint_nome": "Varejo 20/07 - 31/07 [2026]",
  "data_inicio": "2026-07-20",
  "data_fim": "2026-07-31",
  "horas_total": 80.0,
  "linha_ideal": [{"data": "2026-07-20", "horas": 80.0}, ...],
  "linha_real": [{"data": "2026-07-20", "horas": 80.0}, ...]
}
```

### Cálculo

- `horas_total`: soma estimativa_tempo de tarefas da sprint (exceto Cancelado) no dia início
- `linha_ideal`: reta do total até zero, 1 ponto por dia útil
- `linha_real`: para cada dia até hoje — horas_total - soma horas concluídas (data_resolvido <= dia) + soma horas adicionadas (data_entrada_sprint > dia início e <= dia)
- Tarefas Cancelado excluídas de tudo

## Frontend — Canvas 2D

- Background: var(--surface), border-radius 8px, padding 16px
- Título: "Burndown" font-size 14px
- Eixo X: datas (dd/mm)
- Eixo Y: horas restantes
- Linha ideal: tracejada, var(--text-tertiary), opacity 0.5
- Linha real: sólida, var(--accent) se abaixo da ideal, var(--red) se acima
- Pontos: círculos 4px nos dados reais
- Hoje: linha vertical pontilhada var(--blue)
- Grid: horizontais sutis var(--border-subtle)
- Tooltip: gantt-tooltip existente, "dd/mm — Xh restantes"
- Tamanho: 100% largura, 280px altura
- Dark mode via CSS vars

## Arquivos

- `backend/internal/repository/sprint.go` — query burndown
- `backend/internal/service/sprint.go` — cálculo linhas
- `backend/internal/handler/sprint.go` — handler + interface
- `backend/cmd/api/main.go` — rota
- `frontend/index.html` — layout grid + canvas chart
