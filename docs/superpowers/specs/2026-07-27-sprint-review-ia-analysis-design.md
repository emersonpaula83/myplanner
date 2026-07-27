# Sprint Review — Análise IA

## Objetivo

Adicionar análise inteligente via IA (modelo gratuito `openai/gpt-oss-20b:free` no OpenRouter) à aba Review do detalhe da sprint. A análise é separada por produto e cobre: foco da sprint, top 3 entregas, incidentes e tarefas não planejadas.

## Modelo e Provedor

- **Provedor**: OpenRouter (`https://openrouter.ai/api/v1/chat/completions`)
- **Modelo padrão**: `openai/gpt-oss-20b:free` (configurável via tabela `configuracoes` chave `openrouter_model`)
- **Formato API**: OpenAI-compatible (chat completions)
- **Autenticação**: `Authorization: Bearer <key>` header
- **API Key**: configurada pelo usuário no browser, salva no banco de dados (chave `openrouter_api_key`)
- **Modelo**: configurável pelo usuário (chave `openrouter_model`), default `openai/gpt-oss-20b:free`

## Arquitetura

### Fluxo

```
Usuário clica "Gerar Análise IA"
  → Frontend verifica se key existe (GET /api/config/openrouter_api_key)
    → Se não existe: modal pra inserir key → POST /api/config
    → Se existe: POST /sprints/{id}/review/analise
      → Backend monta prompt com dados da sprint (tarefas + estimativas)
      → Backend chama OpenRouter API
      → Backend parseia JSON estruturado da resposta
      → Backend salva em sprint_review_analises
      → Frontend renderiza cards por produto
```

### Componentes

1. **Tabela `configuracoes`** — chave-valor genérica pra settings do app
2. **Tabela `sprint_review_analises`** — cache da análise gerada
3. **Service `openrouter.go`** — client OpenRouter (padrão similar a `gemini.go`)
4. **Service `review.go`** — método `GenerateAnalise` que monta prompt e orquestra
5. **Handler `review.go`** — endpoints GET/POST pra análise
6. **Frontend** — botão, modal key, cards estruturados

## Schema do Banco

### Tabela `configuracoes`

```sql
CREATE TABLE configuracoes (
    chave VARCHAR(100) PRIMARY KEY,
    valor TEXT NOT NULL,
    atualizado_em TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### Tabela `sprint_review_analises`

```sql
CREATE TABLE sprint_review_analises (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sprint_id UUID NOT NULL REFERENCES sprints(id),
    equipe_id UUID NOT NULL REFERENCES equipes(id),
    produto_ids UUID[] NOT NULL DEFAULT '{}',
    analise_json JSONB NOT NULL,
    modelo VARCHAR(100) NOT NULL,
    criado_em TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(sprint_id, equipe_id, produto_ids)
);
```

A constraint UNIQUE garante 1 cache por combinação sprint+equipe+produtos. Re-gerar faz UPDATE.

## Dados de Entrada pra IA

Adicionar `estimativa_tempo` (segundos) ao `ReviewTaskRow` e `ReviewTarefa`. O campo já existe na tabela `tarefas` (`estimativa_tempo INTEGER`).

### Campos enviados no prompt

Para cada tarefa:
- `numero_ticket`
- `resumo`
- `tipo` (Bug, Melhoria, Incidente, etc.)
- `tipo_demanda` (Meta, Compromisso, Iniciativa)
- `status`
- `produto`
- `nao_planejada` (bool)
- `estimativa_horas` (convertido de segundos pra horas: `estimativa_tempo / 3600`)

## Prompt da IA

O prompt instrui a IA a retornar JSON estruturado. Dados da sprint enviados como JSON no prompt. Resposta esperada em JSON puro (sem markdown fences).

### Estrutura do Prompt

```
Você é um analista de sprints de desenvolvimento de software.
Analise os dados da sprint abaixo e retorne um JSON com a análise separada por produto.

DADOS DA SPRINT:
{json com array de tarefas agrupadas por produto}

REGRAS:
1. Foco da Sprint: identifique onde a maior parte das horas estimadas foi gasta
2. Top 3 Entregas: as 3 tarefas com maior estimativa. Se tipo_demanda for "Meta" ou "Compromisso", marque destaque=true
3. Incidentes: avalie todos os incidentes/bugs. Se houver causa raiz similar entre eles, informe
4. Não Planejadas: liste tarefas com nao_planejada=true EXCLUINDO bugs e incidentes. Informe produto, horas e percentual da sprint

Responda APENAS com JSON válido no formato:
{schema do JSON esperado}
```

## Estrutura JSON da Resposta

```json
{
  "analises_por_produto": [
    {
      "produto": "Nome Produto",
      "foco_sprint": {
        "descricao": "Texto descrevendo foco principal da sprint...",
        "categoria_principal": "melhorias",
        "horas_estimadas": 48
      },
      "top3_entregas": [
        {
          "ticket": "PROJ-123",
          "resumo": "Migração de cache distribuído",
          "tipo_demanda": "Meta",
          "destaque": true,
          "horas_estimadas": 16
        }
      ],
      "analise_incidentes": {
        "total": 3,
        "resumo": "Descrição geral dos incidentes...",
        "causa_comum": "2 incidentes relacionados a timeout no serviço X",
        "incidentes": [
          {"ticket": "PROJ-500", "resumo": "Timeout API pagamentos", "horas_estimadas": 4}
        ]
      },
      "nao_planejadas": {
        "total": 2,
        "horas_total": 12,
        "percentual_sprint": 8.5,
        "resumo": "Tarefas que entraram após início da sprint...",
        "tarefas": [
          {"ticket": "PROJ-321", "resumo": "Relatório urgente", "produto": "Core", "horas_estimadas": 6}
        ]
      }
    }
  ]
}
```

## API Endpoints

### Configurações

```
GET  /api/config/:chave        → { "chave": "...", "valor": "..." }
POST /api/config               → body: { "chave": "openrouter_api_key", "valor": "sk-..." }
                                  Retorna 200 se OK
```

Nota: endpoint de config limitado a chaves permitidas (whitelist: `openrouter_api_key`, `openrouter_model`). Não expõe chaves arbitrárias.

### Análise

```
GET  /sprints/{id}/review/analise?equipe_id=...&produtos=...
     → Retorna cache se existir, 404 se não

POST /sprints/{id}/review/analise?equipe_id=...&produtos=...
     → Gera análise via IA, salva cache, retorna resultado
     → Se cache existe, faz UPDATE (regerar)
     → Retorna 503 se key não configurada
```

## Frontend

### Localização

Cards de análise aparecem entre gráficos de pizza e tabela de tarefas na aba Review.

### Botão

```
[🤖 Gerar Análise IA]  [⚙️]
```

- Botão principal: gera ou carrega análise
- Ícone engrenagem: abre modal pra editar API key
- Se análise já existe (cache): mostra cards + botão "🔄 Regerar"

### Modal de API Key

Aparece automaticamente no primeiro clique se key não configurada:
- Input text pra key
- Botão salvar
- Texto explicativo: "Insira sua chave da API OpenRouter para habilitar análise por IA"

### Cards Estruturados

Por produto, com seções visuais distintas:

```
╔═ Produto: Core Banking ═══════════════════════════╗
║ 📊 Foco da Sprint                                  ║
║ "Principal foco em melhorias de performance..."     ║
║ Categoria: Melhorias | Total estimado: 48h          ║
╠════════════════════════════════════════════════════╣
║ 🏆 Top 3 Entregas                                  ║
║ 1. PROJ-123 - Migração de cache [Meta ⭐]    16h   ║
║ 2. PROJ-456 - Endpoint batch                 12h   ║
║ 3. PROJ-789 - Dashboard filtros               8h   ║
╠════════════════════════════════════════════════════╣
║ 🚨 Incidentes (3)                                  ║
║ "2 incidentes com causa comum: timeout no..."       ║
║ • PROJ-500 - Timeout API pagamentos           4h    ║
║ • PROJ-501 - Falha integração                 2h    ║
║ • PROJ-502 - Erro login SSO                   3h    ║
╠════════════════════════════════════════════════════╣
║ 📋 Não Planejadas (excl. bugs/incidentes)           ║
║ Total: 2 tarefas | 12h (8.5% da sprint)            ║
║ • PROJ-321 - Relatório urgente                6h    ║
║ • PROJ-654 - Ajuste integração                6h    ║
╚════════════════════════════════════════════════════╝
```

Estilo:
- Fundo ligeiramente diferente do resto da página
- Borders arredondadas
- Seções com ícones e cor de header distintas
- Badge "Meta ⭐" / "Compromisso ⭐" em destaque (bold, cor diferenciada)
- Responsivo (stack vertical em mobile)

### Export (PDF/Imagem)

Se análise existe no cache, incluir nos exports entre gráficos e tabela. Se não gerada, export segue sem seção de análise.

### Loading State

Spinner + texto "Gerando análise..." enquanto API processa. Timeout de 60s (modelo free pode ser lento).

## Error Handling

| Cenário | Comportamento |
|---------|--------------|
| Key não configurada | Modal pra inserir key |
| Key inválida (401) | Toast: "Chave API inválida. Verifique em ⚙️" |
| API indisponível | Retry 1x, depois toast: "Serviço indisponível, tente novamente" |
| Rate limit (429) | Toast: "Limite de requisições. Aguarde alguns minutos" |
| JSON inválido da IA | Retry 1x com prompt reforçado. Se falhar, toast de erro |
| Timeout (60s) | Toast: "Análise demorou muito. Tente novamente" |

## Segurança

- API key armazenada em texto no banco (single-tenant app, sem multi-user com permissões distintas)
- Endpoint GET config NÃO retorna valor da key — retorna `{"exists": true/false}` pra checagem
- Endpoint POST config aceita whitelist de chaves (`openrouter_api_key`)
- Key enviada ao OpenRouter server-side apenas (nunca exposta ao browser após configuração)

## Arquivos Afetados

### Backend — Novos
- `backend/migrations/000019_configuracoes.up.sql`
- `backend/migrations/000019_configuracoes.down.sql`
- `backend/migrations/000020_sprint_review_analises.up.sql`
- `backend/migrations/000020_sprint_review_analises.down.sql`
- `backend/internal/service/openrouter.go`
- `backend/internal/repository/config.go`

### Backend — Modificados
- `backend/internal/repository/review.go` — adicionar `estimativa_tempo` na query e struct
- `backend/internal/service/review.go` — adicionar `EstimativaHoras` ao `ReviewTarefa`, método `GenerateAnalise`
- `backend/internal/handler/review.go` — endpoints análise e config
- `backend/cmd/api/main.go` — wiring novos repos/handlers
- `backend/internal/config/config.go` — adicionar `OpenRouterModel` default

### Frontend — Modificado
- `frontend/index.html` — botão, modal key, cards análise, export, loading state
