# AI Configuration Flexível + Destaques Automáticos

## Objetivo

Tornar a configuração de IA do sistema provider-agnostic (qualquer provedor OpenAI-compatible), remover edição manual de destaques (IA popula), e corrigir tema do modal de configuração.

## Escopo

Três mudanças coordenadas:

1. **Remover edição manual de destaques** — esconder botões adicionar/editar/excluir, manter backend intacto
2. **Novo modal de configuração de IA** — provider-agnostic, theme-aware, com instruções
3. **Backend generalizado** — renomear OpenRouterClient → AIClient, config keys generalizadas

## 1. Remover Edição Manual de Destaques

### Frontend (`frontend/index.html`)

Em `renderDestaques()` (linha ~4973):
- Remover botão "Adicionar destaque" (`review-add-destaque`)
- Remover botões editar (✎) e excluir (🗑) de cada destaque item
- Remover `<div id="destaque-form-{prodID}">` containers
- Manter exibição read-only de título, descrição e link
- Manter seção de produtos sem destaques (mas sem botão adicionar)

Funções que podem ser removidas do frontend:
- `showDestaqueForm()` — formulário de inclusão/edição
- `saveDestaque()` — POST/PUT para backend
- `deleteDestaque()` — DELETE para backend

### Backend

Manter todos os endpoints intactos — a IA chamará `POST /sprints/{sprintId}/review/destaques` para popular destaques automaticamente via análise.

## 2. Novo Modal de Configuração de IA

### Substituir modal atual

O modal atual (`#apikey-modal`, linha ~1848) tem problemas:
- Hardcoded para OpenRouter
- Background `#fff` hardcoded — não respeita tema escuro
- Sem instruções de uso
- Só campo de API key

### Novo modal — estrutura HTML

```html
<div id="ai-config-modal" class="planning-modal-overlay" style="display:none">
  <div class="ai-config-dialog">
    <h3>Configurar Inteligência Artificial</h3>
    
    <div class="ai-config-instructions">
      Configure qualquer provedor de IA compatível com a API OpenAI.
      Você precisa de: uma Base URL do provedor, sua API Key e o nome do modelo.
      Selecione um provedor abaixo para preencher automaticamente, ou escolha "Personalizado".
    </div>

    <label>Provedor</label>
    <select id="ai-provider-select" onchange="onAIProviderChange()">
      <option value="openrouter" data-url="https://openrouter.ai/api/v1" data-placeholder="sk-or-..." data-model="openai/gpt-4o-mini">OpenRouter</option>
      <option value="openai" data-url="https://api.openai.com/v1" data-placeholder="sk-..." data-model="gpt-4o-mini">OpenAI</option>
      <option value="groq" data-url="https://api.groq.com/openai/v1" data-placeholder="gsk_..." data-model="llama-3.1-70b-versatile">Groq</option>
      <option value="together" data-url="https://api.together.xyz/v1" data-placeholder="..." data-model="meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo">Together AI</option>
      <option value="ollama" data-url="http://localhost:11434/v1" data-placeholder="(não necessário)" data-model="llama3.1">Ollama (local)</option>
      <option value="custom" data-url="" data-placeholder="..." data-model="">Personalizado</option>
    </select>

    <label>Base URL</label>
    <input id="ai-base-url" type="text" placeholder="https://api.provider.com/v1">

    <label>API Key</label>
    <input id="ai-api-key" type="password" placeholder="sk-...">

    <label>Modelo</label>
    <input id="ai-model" type="text" placeholder="nome-do-modelo">

    <div id="ai-config-test-result"></div>

    <div class="ai-config-actions">
      <button class="btn-cancel" onclick="closeAIConfigModal()">Cancelar</button>
      <button class="btn-secondary" onclick="testAIConfig()">Testar Conexão</button>
      <button class="btn-save" onclick="saveAIConfig()">Salvar</button>
    </div>
  </div>
</div>
```

### CSS — theme-aware

```css
.ai-config-dialog {
  background: var(--surface);
  color: var(--text-primary);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 24px;
  max-width: 480px;
  width: 90%;
}
.ai-config-dialog h3 {
  margin: 0 0 8px;
  font-size: 18px;
  color: var(--text-primary);
}
.ai-config-instructions {
  font-size: 13px;
  color: var(--text-secondary);
  margin-bottom: 20px;
  line-height: 1.5;
  padding: 12px;
  background: var(--surface-secondary);
  border-radius: 8px;
  border: 1px solid var(--border-subtle);
}
.ai-config-dialog label {
  display: block;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  margin: 12px 0 4px;
  text-transform: uppercase;
}
.ai-config-dialog input,
.ai-config-dialog select {
  width: 100%;
  padding: 8px 12px;
  border: 1px solid var(--border);
  border-radius: 6px;
  font-size: 13px;
  background: var(--surface);
  color: var(--text-primary);
  box-sizing: border-box;
}
.ai-config-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  margin-top: 20px;
}
```

### JavaScript — funções

**`onAIProviderChange()`** — Ao selecionar provedor, preenche automaticamente Base URL, placeholder da API Key e modelo recomendado.

**`showAIConfigModal()`** — Abre modal. Carrega valores salvos do backend:
- `GET /config/ai_base_url` → preenche Base URL
- `GET /config/ai_api_key` → mostra "(configurada)" se existe
- `GET /config/ai_model` → preenche modelo
- Seleciona provedor correto no dropdown baseado na base URL

**`closeAIConfigModal()`** — Fecha modal.

**`saveAIConfig()`** — Salva três config keys via `POST /config`:
- `ai_base_url`
- `ai_api_key` (só se preenchido — campo vazio não sobrescreve)
- `ai_model`

**`testAIConfig()`** — Chama `POST /config/ai/test` com valores do formulário. Mostra resultado (sucesso verde / erro vermelho) em `#ai-config-test-result`.

### Trigger — onde abre o modal

- Botão ⚙️ na seção de análise review (já existe, trocar `showApiKeyModal()` → `showAIConfigModal()`)
- Quando API retorna 503 (sem config), abrir modal ao invés de só pedir key

## 3. Backend — Generalizar AI Client

### Renomear arquivo e structs

`internal/service/openrouter.go` → `internal/service/ai.go`

```go
type AIClient struct {
    apiKey  string
    model   string
    baseURL string
}

func NewAIClient(apiKey, model, baseURL string) *AIClient {
    return &AIClient{
        apiKey:  apiKey,
        model:   model,
        baseURL: baseURL,
    }
}
```

`ChatCompletion()` e `callAPI()` — mesma lógica, só trocar nomes de tipos:
- `openRouterRequest` → `aiRequest`
- `openRouterMessage` → `aiMessage`
- `openRouterResponse` → `aiResponse`
- Error messages: "openrouter" → "AI provider"

### Config keys

Novas keys no whitelist:
- `ai_api_key` (secret — retorna só `exists: bool`)
- `ai_model` (retorna valor)
- `ai_base_url` (retorna valor)

Manter keys antigas `openrouter_*` no whitelist para backward compatibility — não quebrar configs existentes.

### `GenerateAnalise()` — atualizar

```go
func (s *ReviewService) GenerateAnalise(...) {
    // Try new keys first, fall back to legacy
    apiKey, err := s.configRepo.GetConfig(ctx, "ai_api_key")
    if err != nil {
        apiKey, err = s.configRepo.GetConfig(ctx, "openrouter_api_key")
        if err != nil {
            return nil, fmt.Errorf("AI API key not configured")
        }
    }
    
    baseURL := "https://openrouter.ai/api/v1"
    if u, err := s.configRepo.GetConfig(ctx, "ai_base_url"); err == nil && u != "" {
        baseURL = u
    }
    
    model := "openai/gpt-4o-mini"
    if m, err := s.configRepo.GetConfig(ctx, "ai_model"); err == nil && m != "" {
        model = m
    }
    
    client := NewAIClient(apiKey, model, baseURL)
    // ... rest unchanged
}
```

### Novo endpoint — Test Connection

`POST /config/ai/test`

Request body:
```json
{
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-...",
  "model": "gpt-4o-mini"
}
```

Handler cria `AIClient` temporário, faz chamada simples (`"Say OK"`) e retorna sucesso/erro. Não persiste nada — só valida.

Response sucesso: `{ "status": "ok", "model": "gpt-4o-mini" }`
Response erro: `{ "error": "connection refused" }`

Rota: `r.Post("/config/ai/test", reviewHandler.TestAIConnection)`

## Migração de Dados

Sem migration SQL necessária — tabela `configuracoes` é key-value genérica. Novas keys são inseridas via `POST /config` (upsert).

Backend faz fallback: se `ai_api_key` não existe, tenta `openrouter_api_key`. Configs antigas continuam funcionando até user reconfigurar via novo modal.

## Provedores Suportados (documentação no modal)

| Provedor | Base URL | Key Format | Modelo Recomendado |
|----------|----------|------------|-------------------|
| OpenRouter | `https://openrouter.ai/api/v1` | `sk-or-...` | `openai/gpt-4o-mini` |
| OpenAI | `https://api.openai.com/v1` | `sk-...` | `gpt-4o-mini` |
| Groq | `https://api.groq.com/openai/v1` | `gsk_...` | `llama-3.1-70b-versatile` |
| Together | `https://api.together.xyz/v1` | `...` | `meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo` |
| Ollama | `http://localhost:11434/v1` | (nenhuma) | `llama3.1` |
| Qualquer OpenAI-compatible | URL customizada | Depende | Depende |

## Testes

- Trocar provider no dropdown → campos preenchem automaticamente
- Salvar config → `POST /config` chamado 3x (url, key, model)
- Testar Conexão → feedback visual verde/vermelho
- Gerar análise → usa config salva, fallback para openrouter_* se ai_* não existir
- Modal respeita tema escuro/claro (usar CSS vars)
- Destaques aparecem read-only (sem botões editar/adicionar/excluir)
