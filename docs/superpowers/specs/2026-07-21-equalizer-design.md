# Equalizador de Capacidade — Design Spec

## Objetivo

Ferramenta no detalhamento da sprint que analisa a distribuição de carga entre membros da equipe e sugere transferências de tarefas não-iniciadas de membros sobrecarregados para membros com capacidade disponível, com opção de aplicação automática via JIRA API.

## Regras de Negócio

### Escopo de membros
- Apenas membros com `da_equipe = true`. Externos excluídos.
- Membros desligados (`desligado = true`) excluídos.

### Classificação
- **Doador:** `percentual_alocacao > 100%` E possui tarefas não-iniciadas.
- **Receptor:** `percentual_alocacao < 70%`.
- **Neutro:** entre 70-100% — não participa das transferências.

### Tarefas candidatas
- Status não-iniciado: `status_categoria` diferente de `'indeterminate'` (in progress) e `'done'`.
- Pertence ao doador (`responsavel_id`).
- Pertence à sprint atual (`sprint_id`).

### Algoritmo (Greedy por maior desvio)
1. Filtra membros da equipe do resultado de `GetCapacity`.
2. Classifica em doadores e receptores.
3. Ordena doadores por `percentual_alocacao` descendente, receptores por ascendente.
4. Para cada doador, obtém tarefas não-iniciadas ordenadas por horas descendente.
5. Para cada tarefa, tenta alocar no receptor com mais `horas_disponiveis`.
6. Só sugere se a transferência representar ≥10% da capacity do doador (`horas_tarefa / horas_estimadas_doador * 100 >= 10`).
7. Máximo 10 sugestões por execução.

### Limiar "nada a sugerir"
- Nenhum doador encontrado (todos ≤100%).
- Nenhum receptor encontrado (todos ≥70%).
- Nenhuma transferência viável atinge limiar de 10%.

### Cálculo antes/depois
- `pct_antes = horas_alocadas / horas_estimadas * 100` (valor atual de cada membro).
- `pct_depois = (horas_alocadas ∓ horas_tarefa) / horas_estimadas * 100` (projeção com transferências).
- Array `membros_antes_depois` inclui todos os membros da equipe (não só doadores/receptores), para visualização completa.

## API

### GET /api/v1/sprints/{id}/equalizer?equipe={equipeID}

Calcula sugestões de equalização server-side. Parâmetro `equipe` opcional (filtra por equipe, como nos endpoints existentes).

**Response 200:**
```json
{
  "sugestoes": [
    {
      "de": {
        "membro_id": "uuid",
        "nome": "Davi Portella",
        "avatar_url": "https://...",
        "pct_antes": 130.5,
        "pct_depois": 108.2
      },
      "para": {
        "membro_id": "uuid",
        "nome": "Paulo César",
        "avatar_url": "https://...",
        "pct_antes": 60.0,
        "pct_depois": 82.3
      },
      "tarefas": [
        {
          "id": "uuid",
          "numero_ticket": "PROJ-123",
          "resumo": "Implementar feature X",
          "horas": 4.0,
          "tipo": "Story",
          "prioridade": "Medium"
        }
      ],
      "horas_transferidas": 4.0,
      "pct_transferido": 22.3
    }
  ],
  "membros_antes_depois": [
    {
      "membro_id": "uuid",
      "nome": "Davi Portella",
      "avatar_url": "https://...",
      "pct_antes": 130.5,
      "pct_depois": 108.2,
      "horas_antes": 47.0,
      "horas_depois": 39.0
    }
  ],
  "nada_a_sugerir": false,
  "motivo": ""
}
```

Quando `nada_a_sugerir = true`, `motivo` contém a razão (ex: "Todos os membros estão com alocação entre 70-100%").

### POST /api/v1/sprints/{id}/equalizer/apply

Aplica transferências selecionadas pelo usuário.

**Request:**
```json
{
  "fonte_dados_id": "uuid",
  "transferencias": [
    {
      "tarefa_id": "uuid",
      "tarefa_key": "PROJ-123",
      "novo_responsavel_id": "uuid",
      "novo_responsavel_jira_account_id": "jira-account-id"
    }
  ]
}
```

**Ações por transferência:**
1. `PUT /rest/api/3/issue/{issueKey}` — altera assignee para `novo_responsavel_jira_account_id`.
2. `POST /rest/api/3/issue/{issueKey}/comment` — adiciona comentário: "Tarefa transferida de {antigo} para {novo} via Equalizador de Capacidade".
3. `UPDATE tarefas SET responsavel_id = $novo WHERE id = $tarefa_id` — atualiza base local.

**Response 200:**
```json
{
  "aplicadas": 3,
  "erros": [
    { "tarefa_key": "PROJ-456", "erro": "JIRA API: 403 Forbidden" }
  ]
}
```

## Arquitetura de Código

### Arquivos novos
- `backend/internal/service/equalizer.go` — `EqualizerService` com métodos `Calculate(ctx, sprintID, equipeID)` e `Apply(ctx, sprintID, transferencias)`.
- `backend/internal/handler/equalizer.go` — `EqualizerHandler` com métodos `GetSuggestions` e `ApplyTransfers`.

### Arquivos modificados
- `backend/internal/jira/client.go` — novos métodos na interface `Client`:
  - `UpdateIssueAssignee(ctx, issueKey, accountID string) error`
  - `AddComment(ctx, issueKey, body string) error`
- `backend/cmd/api/main.go` — registrar rotas e wiring.
- `frontend/index.html` — CSS do modal, botão, funções JS.

### Dependências do EqualizerService
- `SprintService` — reutiliza `GetCapacity()` para dados de capacity.
- `SprintRepository` — query tarefas não-iniciadas com `jira_account_id` do membro.
- `FonteDadosRepository` — credenciais JIRA para API calls.
- `jira.Client` (via factory) — re-atribuir + comentar.
- `OAuthService` — refresh token se OAuth.

## Frontend — Modal

### Botão trigger
Na seção Capacity Summary da sprint detail. Botão "⚖️ Equalizador" com estilo secundário. Ao clicar: loading → GET endpoint → abre modal.

### Badge de notificação
Ao carregar a sprint detail, uma chamada em background ao `GET /equalizer` verifica se há sugestões. Se `nada_a_sugerir = false`, exibe um badge "!" vermelho sobre o botão Equalizador indicando que há recomendações de transferência.

### Modal — "nada a sugerir"
- Ícone ✅ centralizado.
- Texto: "Nada a sugerir de transferência de tarefas".
- Motivo exibido abaixo.
- Botão Fechar.

### Modal — com sugestões

**Área 1 — Antes/Depois (topo):**
Tabela compacta com todos membros da equipe:

| Avatar | Membro | % Antes | → | % Depois |

Barras de progresso coloridas: verde <80%, amarelo 80-100%, vermelho >100%. Mostra visualmente o impacto da equalização.

**Área 2 — Sugestões de transferência:**
Para cada par doador→receptor:
- Card com avatar do doador à esquerda, seta `→` central com "Xh (Y%)", avatar do receptor à direita.
- `% antes → % depois` embaixo de cada avatar com cor (vermelho→amarelo para doador, verde→amarelo para receptor).
- Lista de tarefas candidatas com checkbox (pré-selecionadas):
  - Ticket, resumo, tipo badge, horas, prioridade.
  - Usuário pode desmarcar tarefas que não quer transferir.

**Footer:**
- Botão "Aplicar Selecionadas (N)" — primário, desabilitado se 0 selecionadas.
- Botão "Cancelar" — secundário.

**Pós-aplicação:**
- Loading com progresso "Aplicando 1/N..."
- Sucesso: alert "N tarefas transferidas com sucesso", fecha modal, recarrega capacity.
- Erro parcial: mostra quais falharam, mantém modal aberto.
