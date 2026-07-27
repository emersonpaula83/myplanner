# AI Provider Flexível — Design Spec

## Objetivo

Substituir o acoplamento direto ao OpenRouter por uma arquitetura de AI provider configurável, permitindo que o administrador da instância escolha qualquer provider OpenAI-compatible (OpenRouter, OpenAI, Azure OpenAI, Groq, Together, Ollama, etc.) via configuração no banco de dados. A API key nunca é exposta ao frontend.

## Contexto

O MyPlanner será SaaS no futuro. Cada instância terá seus próprios usuários e configurações. Por agora, a configuração de AI provider é **global por instância** (tabela `configuracoes`). Quando o módulo IAM (Fluig Identity) for implementado, a granularidade poderá mudar para per-tenant/organização.

A implementação atual usa OpenRouter hardcoded (`baseURL: "https://openrouter.ai/api/v1"`) com config keys `openrouter_api_key` e `openrouter_model`. Este design generaliza para qualquer provider OpenAI-compatible.

## Premissa Técnica

Todos os providers-alvo usam o formato OpenAI Chat Completions (`POST /chat/completions` com `model`, `messages[]`). O client HTTP existente (`openrouter.go`) já implementa este formato — basta tornar `baseURL` configurável e renomear.

Anthropic API (formato Messages, não Chat Completions) **não** está no escopo. Se demanda surgir, será adapter separado em ciclo futuro.

## Arquitetura

### Config Keys (tabela `configuracoes`)

| Chave antiga | Chave nova | Tipo | Default | Descrição |
|---|---|---|---|---|
| `openrouter_api_key` | `ai_api_key` | string | — | API key do provider (nunca exposta ao frontend) |
| `openrouter_model` | `ai_model` | string | `openai/gpt-oss-20b:free` | Identificador do modelo |
| *(novo)* | `ai_base_url` | string | `https://openrouter.ai/api/v1` | Base URL do provider |

### Backend

**Renomear `openrouter.go` → `ai_client.go`:**
- `OpenRouterClient` → `AIClient`
- `NewOpenRouterClient(apiKey, model string)` → `NewAIClient(apiKey, model, baseURL string)`
- `baseURL` passado por parâmetro em vez de hardcoded
- Mensagens de erro: trocar "openrouter" por "ai provider"
- Resto do código (retry, timeout, response parsing) permanece igual

**Atualizar `review.go` (service) — `GenerateAnalise`:**
- Ler `ai_api_key`, `ai_model`, `ai_base_url` da config (em vez de `openrouter_*`)
- Default de `ai_base_url` quando não configurado: `https://openrouter.ai/api/v1`
- Default de `ai_model` quando não configurado: `openai/gpt-oss-20b:free`
- Chamar `NewAIClient(apiKey, model, baseURL)`

**Atualizar `handler/review.go` — config whitelist:**
- Remover: `openrouter_api_key`, `openrouter_model`
- Adicionar: `ai_api_key`, `ai_model`, `ai_base_url`
- `GetConfig`: tratar `ai_api_key` como sensível (retorna `{"exists": bool}`, nunca o valor)
- `ai_model` e `ai_base_url` retornam valor normal

### Migration (000021)

```sql
-- UP
UPDATE configuracoes SET chave = 'ai_api_key' WHERE chave = 'openrouter_api_key';
UPDATE configuracoes SET chave = 'ai_model' WHERE chave = 'openrouter_model';
INSERT INTO configuracoes (chave, valor)
VALUES ('ai_base_url', 'https://openrouter.ai/api/v1')
ON CONFLICT (chave) DO NOTHING;

-- DOWN
UPDATE configuracoes SET chave = 'openrouter_api_key' WHERE chave = 'ai_api_key';
UPDATE configuracoes SET chave = 'openrouter_model' WHERE chave = 'ai_model';
DELETE FROM configuracoes WHERE chave = 'ai_base_url';
```

### Frontend — Modal de Configuração AI

Substituir o modal simples de API key (`#apikey-modal`) por um modal expandido com 4 campos:

**Layout do modal:**

```
┌─ Configurar AI Provider ──────────────────────┐
│                                                │
│  Provider:  [OpenRouter ▾]                     │
│  Base URL:  [https://openrouter.ai/api/v1   ]  │
│  API Key:   [••••••••••••••                 ]  │
│  Modelo:    [openai/gpt-oss-20b:free        ]  │
│                                                │
│                    [Cancelar]  [Salvar]         │
└────────────────────────────────────────────────┘
```

**Comportamento do select Provider:**

| Provider | Base URL (auto-preenchido) | Modelo sugerido |
|---|---|---|
| OpenRouter | `https://openrouter.ai/api/v1` | `openai/gpt-oss-20b:free` |
| OpenAI | `https://api.openai.com/v1` | `gpt-4o-mini` |
| Azure OpenAI | *(campo vazio — usuário preenche)* | *(campo vazio)* |
| Custom | *(campo vazio)* | *(campo vazio)* |

- Provider é apenas UX helper — não é salvo no banco. O que importa são `ai_base_url`, `ai_api_key`, `ai_model`.
- Ao trocar provider, auto-preenche base URL e modelo sugerido (usuário pode editar).
- Ao abrir modal, carrega valores atuais de `ai_base_url` e `ai_model` do backend (GET `/config/{chave}`). O campo API Key mostra placeholder se key existe, vazio se não.

**Salvar:** 3 chamadas POST `/config` (uma por campo). Se API key estiver vazia e já existia, não sobrescreve (mantém a existente).

### Segurança

- `ai_api_key` NUNCA é retornada ao frontend. `GetConfig` retorna apenas `{"exists": true/false}`.
- Frontend nunca armazena key em localStorage/sessionStorage.
- Keys no banco ficam em texto plano (mesma abordagem atual). Encriptação at-rest é escopo futuro (quando IAM for implementado).

## Testes

- Testes unitários existentes em `handler/review_test.go`: atualizar mock e nomes de config keys.
- `service/sync_test.go`: verificar se referencia config keys (não deve).
- Build check: `go build ./...`, `go vet ./...`, `go test ./...`.
- Frontend: `node --check` no JS extraído.
- Manual: abrir modal → selecionar provider → verificar auto-preenchimento → salvar → gerar análise.

## Fora do Escopo

- Adapter para Anthropic API (formato Messages) — ciclo futuro se demanda surgir.
- Config per-equipe/per-tenant — depende de módulo IAM.
- Encriptação de API keys at-rest.
- Validação de conectividade (test connection button) — pode ser adicionado depois.
- Streaming de resposta (SSE) — ciclo futuro.

## Global Constraints

- Frontend: vanilla JS, `var`/`function` only, sem ES6+. Usar `esc()` para todo texto dinâmico.
- Backend: Go com chi router, pgx/v5, zap logger.
- Formato de commit: não commitar automaticamente — deixar unstaged.
- Dark mode: CSS custom properties + `@media (prefers-color-scheme: dark)` + `:root[data-theme="dark"]`.
