# Design: Links JIRA nos Números de Tarefa

**Data:** 2026-08-10
**Status:** Aprovado

## Objetivo

Transformar todos os números de tarefa (`numero_ticket`) exibidos nas listas do sistema em links clicáveis que abrem a tarefa correspondente no JIRA em nova aba.

## URL Pattern

```
{base_url}/browse/{numero_ticket}
```

Exemplo: `https://totvscloud.atlassian.net/browse/TCDV-3593`

## Abordagem: Helper function + cache de base_url

### 1. Cache de `base_url`

- Variável global `window._jiraBaseUrl`
- Populada quando sprint capacity é carregado (response inclui `fonte_dados_id`)
- Fetch `/fontes/{id}` uma vez para obter `base_url`, cachear resultado
- Se fontes já carregadas na sessão, reusar

### 2. Helper function `jiraTicketLink(numero)`

```javascript
function jiraTicketLink(numero) {
  if (!numero) return '';
  if (!window._jiraBaseUrl) return esc(numero);
  return '<a href="' + esc(window._jiraBaseUrl) + '/browse/' + esc(numero)
    + '" target="_blank" rel="noopener" style="color:inherit;text-decoration:underline">'
    + esc(numero) + '</a>';
}
```

- Fallback gracioso: sem `base_url` configurada retorna texto puro (comportamento atual)
- `target="_blank"` — abre em nova aba, não sai da página
- `rel="noopener"` — segurança
- `color:inherit` — mantém visual existente
- `text-decoration:underline` — indica que é clicável
- `esc()` — sanitização XSS (função já existente no projeto)

### 3. Locais de substituição (16 pontos, 8 telas)

| Linha | Variável | Tela |
|-------|----------|------|
| ~2558 | `p.numero_ticket` | Timeline/Projects — lista de épicos |
| ~2559 | `p.numero_ticket` | Timeline/Projects — linha de tabela |
| ~3153 | `t.numero_ticket` | Sprint Review — tabela de tarefas |
| ~3341 | `t.numero_ticket` | Sprint Review — export/print |
| ~4223 | `t.numero_ticket` | Capacity — tarefas por status |
| ~6067 | `t.numero_ticket` | Timeline — tooltip pie chart |
| ~6165 | `t.numero_ticket` | Equalizer — sugestões de balanceamento |
| ~6640 | `t.numero_ticket` | Tarefas — tabela de gerenciamento |
| ~6657 | `t.numero_ticket` | Tarefas — referência botão delete |
| ~6928 | `p.numero_ticket` | Alocação — cards de épicos |
| ~7091 | `epic.numero_ticket` | Alocação — modal título épico |
| ~7156 | `t.numero_ticket` | Alocação — tarefas planejadas |
| ~7177 | `t.numero_ticket` | Alocação — tarefas concluídas |
| ~7250 | `t.numero_ticket` | Alocação — label gantt |
| ~7262 | `t.numero_ticket` | Alocação — tooltip gantt |
| ~7271 | `t.numero_ticket` | Alocação — label gantt não alocado |
| ~7273 | `t.numero_ticket` | Alocação — tooltip gantt não alocado |
| ~7288 | `t.numero_ticket` | Alocação — linha editável |

### 4. Sem alterações no backend

Base URL já disponível via `GET /api/v1/fontes/{id}`. Nenhuma alteração de API necessária.

## Considerações

- **Performance:** Uma única chamada extra a `/fontes/{id}` por sessão, resultado cacheado
- **Segurança:** XSS prevenido via `esc()`, `rel="noopener"` em links externos
- **Graceful degradation:** Sem base_url = texto puro como hoje
