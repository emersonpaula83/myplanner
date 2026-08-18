# AI Config Flexível + Destaques Automáticos — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make AI configuration provider-agnostic (any OpenAI-compatible provider), remove manual destaque editing from frontend, and fix dark-theme support in the AI config modal.

**Architecture:** Backend renames `OpenRouterClient` → `AIClient` with configurable base URL. Frontend replaces single-field API key modal with multi-field provider-agnostic config dialog using CSS variables for theme support. Manual destaque CRUD removed from frontend (backend endpoints kept for AI population).

**Tech Stack:** Go (chi router, pgx), vanilla JS monolith (`frontend/index.html`), CSS variables for theming

## Global Constraints

- All frontend changes go in `frontend/index.html` (monolithic vanilla JS, `var` globals, `function` declarations)
- All backend changes applied directly to local main branch
- No new npm/go dependencies
- CSS must use existing `var(--surface)`, `var(--text-primary)`, `var(--border)`, etc. for theme support
- Backend keeps `openrouter_*` config keys for backward compatibility alongside new `ai_*` keys
- No database migration needed (uses existing `configuracoes` key-value table)

---

### Task 1: Backend — Rename OpenRouterClient to AIClient

**Files:**
- Delete: `backend/internal/service/openrouter.go`
- Create: `backend/internal/service/ai.go`
- Modify: `backend/internal/service/review.go:377-398`
- Modify: `backend/internal/handler/review.go:36-47,271,412-413`

**Interfaces:**
- Consumes: `ConfigStore.GetConfig(ctx, chave)` — existing interface
- Produces: `func NewAIClient(apiKey, model, baseURL string) *AIClient` — used by `GenerateAnalise()` and Task 2's `TestAIConnection` handler

- [ ] **Step 1: Create `backend/internal/service/ai.go`**

Copy the full logic from `openrouter.go`, renaming all types and error messages:

```go
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

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

type aiRequest struct {
	Model    string      `json:"model"`
	Messages []aiMessage `json:"messages"`
}

type aiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func (c *AIClient) ChatCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	result, err := c.callAPI(ctx, systemPrompt, userPrompt)
	if err != nil {
		result, err = c.callAPI(ctx, systemPrompt, userPrompt)
		if err != nil {
			return "", fmt.Errorf("AI provider failed after retry: %w", err)
		}
	}
	return result, nil
}

func (c *AIClient) callAPI(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := aiRequest{
		Model: c.model,
		Messages: []aiMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling AI provider: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI provider returned %d: %s", resp.StatusCode, string(respBody))
	}

	var aiResp aiResponse
	if err := json.Unmarshal(respBody, &aiResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if aiResp.Error != nil {
		return "", fmt.Errorf("AI provider error: %s", aiResp.Error.Message)
	}

	if len(aiResp.Choices) == 0 || aiResp.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("empty response from AI provider")
	}

	return aiResp.Choices[0].Message.Content, nil
}
```

- [ ] **Step 2: Delete `backend/internal/service/openrouter.go`**

```bash
rm backend/internal/service/openrouter.go
```

- [ ] **Step 3: Update `GenerateAnalise()` in `backend/internal/service/review.go`**

Replace lines 377-398 (the config-fetching and client-creation block). The function signature and everything after `client.ChatCompletion()` stays the same.

Replace:
```go
	apiKey, err := s.configRepo.GetConfig(ctx, "openrouter_api_key")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("openrouter API key not configured: %w", err)
		}
		return nil, err
	}

	model := "openai/gpt-oss-20b:free"
	if m, err := s.configRepo.GetConfig(ctx, "openrouter_model"); err == nil && m != "" {
		model = m
	}

	reviewData, err := s.GetReviewData(ctx, sprintID, equipeID, produtoIDs)
	if err != nil {
		return nil, fmt.Errorf("getting review data for analysis: %w", err)
	}

	systemPrompt, userPrompt := buildReviewAnalisePrompt(reviewData.Tarefas)

	client := NewOpenRouterClient(apiKey, model)
```

With:
```go
	apiKey, err := s.configRepo.GetConfig(ctx, "ai_api_key")
	if err != nil {
		apiKey, err = s.configRepo.GetConfig(ctx, "openrouter_api_key")
		if err != nil {
			return nil, fmt.Errorf("AI API key not configured: %w", err)
		}
	}

	baseURL := "https://openrouter.ai/api/v1"
	if u, err := s.configRepo.GetConfig(ctx, "ai_base_url"); err == nil && u != "" {
		baseURL = u
	}

	model := "openai/gpt-4o-mini"
	if m, err := s.configRepo.GetConfig(ctx, "ai_model"); err == nil && m != "" {
		model = m
	} else if m, err := s.configRepo.GetConfig(ctx, "openrouter_model"); err == nil && m != "" {
		model = m
	}

	reviewData, err := s.GetReviewData(ctx, sprintID, equipeID, produtoIDs)
	if err != nil {
		return nil, fmt.Errorf("getting review data for analysis: %w", err)
	}

	systemPrompt, userPrompt := buildReviewAnalisePrompt(reviewData.Tarefas)

	client := NewAIClient(apiKey, model, baseURL)
```

- [ ] **Step 4: Update config whitelist in `backend/internal/handler/review.go`**

Replace the `configWhitelist` map (lines 36-47):

```go
var configWhitelist = map[string]bool{
	"openrouter_api_key": true,
	"openrouter_model":   true,
	"ai_api_key":         true,
	"ai_model":           true,
	"ai_base_url":        true,
	"smtp_host":          true,
	"smtp_port":          true,
	"smtp_user":          true,
	"smtp_password":      true,
	"smtp_from":          true,
	"evolution_api_url":  true,
	"evolution_api_key":  true,
	"evolution_instance": true,
}
```

- [ ] **Step 5: Update secret-key check in `GetConfig` handler**

In `backend/internal/handler/review.go`, line 271, the `GetConfig` method checks for secret keys that should only return `exists: bool`. Add `"ai_api_key"` to that check.

Replace:
```go
	if chave == "openrouter_api_key" || chave == "smtp_password" || chave == "evolution_api_key" {
```

With:
```go
	if chave == "openrouter_api_key" || chave == "ai_api_key" || chave == "smtp_password" || chave == "evolution_api_key" {
```

- [ ] **Step 6: Update 503 error message in `PostReviewAnalise` handler**

In `backend/internal/handler/review.go`, line 413, update the error message:

Replace:
```go
			respondError(w, http.StatusServiceUnavailable, "OpenRouter API key not configured")
```

With:
```go
			respondError(w, http.StatusServiceUnavailable, "AI API key not configured")
```

- [ ] **Step 7: Verify build**

```bash
cd backend && go build ./...
```

Expected: BUILD SUCCESS with no errors.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/service/ai.go backend/internal/service/review.go backend/internal/handler/review.go
git rm backend/internal/service/openrouter.go
git commit -m "refactor: rename OpenRouterClient to AIClient, generalize config keys

Support any OpenAI-compatible AI provider via configurable base_url.
New config keys: ai_api_key, ai_model, ai_base_url.
Legacy openrouter_* keys kept for backward compatibility."
```

---

### Task 2: Backend — Add TestAIConnection Endpoint

**Files:**
- Modify: `backend/internal/handler/review.go` (add handler method)
- Modify: `backend/cmd/api/main.go:366` (add route)

**Interfaces:**
- Consumes: `func NewAIClient(apiKey, model, baseURL string) *AIClient` from Task 1
- Produces: `POST /config/ai/test` endpoint — called by frontend `testAIConfig()` in Task 4

- [ ] **Step 1: Add request type and handler in `backend/internal/handler/review.go`**

Add after the `SetConfig` method (after line 323):

```go
type testAIRequest struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

func (h *ReviewHandler) TestAIConnection(w http.ResponseWriter, r *http.Request) {
	var req testAIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.BaseURL == "" || req.Model == "" {
		respondError(w, http.StatusBadRequest, "base_url and model are required")
		return
	}

	client := service.NewAIClient(req.APIKey, req.Model, req.BaseURL)
	_, err := client.ChatCompletion(r.Context(), "You are a test assistant.", "Say OK")
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]string{"status": "error", "error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok", "model": req.Model})
}
```

- [ ] **Step 2: Register route in `backend/cmd/api/main.go`**

Add after line 366 (`r.Post("/config", reviewHandler.SetConfig)`):

```go
			r.Post("/config/ai/test", reviewHandler.TestAIConnection)
```

- [ ] **Step 3: Verify build**

```bash
cd backend && go build ./...
```

Expected: BUILD SUCCESS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handler/review.go backend/cmd/api/main.go
git commit -m "feat: add POST /config/ai/test endpoint for connection testing

Creates temporary AIClient with provided credentials, sends a test
prompt, returns ok/error without persisting anything."
```

---

### Task 3: Frontend — Remove Manual Destaque Editing

**Files:**
- Modify: `frontend/index.html` — `renderDestaques()` function (~line 4973-5019), delete `showDestaqueForm()` (~5022-5048), `saveDestaque()` (~5050-5079), `deleteDestaque()` (~5082-5087)

**Interfaces:**
- Consumes: nothing from other tasks
- Produces: read-only destaques display (no buttons, no forms)

- [ ] **Step 1: Rewrite `renderDestaques()` to read-only**

Replace the entire `renderDestaques` function (lines 4973-5019) with:

```javascript
function renderDestaques(container, destaques) {
  var byProduto = {};
  var produtoNames = {};
  destaques.forEach(function(d) {
    if (!byProduto[d.produto_id]) {
      byProduto[d.produto_id] = [];
      produtoNames[d.produto_id] = d.produto_nome;
    }
    byProduto[d.produto_id].push(d);
  });

  var html = '';
  Object.keys(byProduto).forEach(function(prodID) {
    var items = byProduto[prodID];
    var prodName = produtoNames[prodID] || 'Produto';
    html += '<div class="review-produto-destaques">';
    html += '<h4>' + esc(prodName) + '</h4>';
    items.forEach(function(d) {
      html += '<div class="review-destaque-item">';
      html += '<div class="destaque-title">' + esc(d.titulo) + '</div>';
      html += '<div class="destaque-desc">' + esc(d.descricao) + '</div>';
      if (d.link) html += '<a class="destaque-link" href="' + escAttr(d.link) + '" target="_blank">' + esc(d.link) + '</a>';
      html += '</div>';
    });
    html += '</div>';
  });

  if (destaques.length === 0) {
    html = '<div style="color:var(--text-tertiary);font-size:13px;padding:12px">Nenhum destaque gerado. Use a análise de IA para gerar destaques automaticamente.</div>';
  }

  container.innerHTML = html;
}
```

- [ ] **Step 2: Delete `showDestaqueForm()`, `saveDestaque()`, and `deleteDestaque()` functions**

Remove these three functions entirely (lines ~5022-5087):

- `showDestaqueForm(produtoID, produtoNome, destaque)` — lines 5022-5048
- `saveDestaque(produtoID, destaqueID)` — lines 5050-5079
- `deleteDestaque(id)` — lines 5082-5087

- [ ] **Step 3: Verify in browser**

Run `./dev.sh`, open review tab, confirm:
- Destaques appear read-only (title, description, link)
- No "Adicionar destaque" buttons
- No edit/delete buttons on each destaque item
- Empty state shows message about AI generation

- [ ] **Step 4: Commit**

```bash
git add frontend/index.html
git commit -m "feat: remove manual destaque editing from review tab

Destaques are now read-only in the UI. AI will populate them
via the existing backend endpoints."
```

---

### Task 4: Frontend — New AI Config Modal (HTML + CSS + JS)

**Files:**
- Modify: `frontend/index.html` — replace `#apikey-modal` HTML (~line 1848-1858), add CSS (~after line 710), replace `showApiKeyModal()`/`saveApiKey()` JS (~lines 4748-4764), update trigger (~line 4161), update 503 handler (~line 4784)

**Interfaces:**
- Consumes: `POST /config` endpoint (existing), `GET /config/{chave}` endpoint (existing), `POST /config/ai/test` endpoint (from Task 2)
- Produces: `showAIConfigModal()`, `closeAIConfigModal()`, `saveAIConfig()`, `testAIConfig()`, `onAIProviderChange()` — all global functions

- [ ] **Step 1: Replace the old modal HTML**

Replace the `#apikey-modal` div (lines 1848-1858) with the new AI config modal:

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
    <div id="ai-config-test-result" style="margin-top:12px;font-size:13px"></div>
    <div class="ai-config-actions">
      <button onclick="closeAIConfigModal()" style="padding:8px 16px;border:1px solid var(--border);border-radius:6px;background:var(--surface);color:var(--text-primary);cursor:pointer;font-size:13px">Cancelar</button>
      <button onclick="testAIConfig()" style="padding:8px 16px;border:1px solid var(--border);border-radius:6px;background:var(--surface-secondary);color:var(--text-primary);cursor:pointer;font-size:13px;font-weight:600">Testar Conexão</button>
      <button onclick="saveAIConfig()" style="padding:8px 16px;border:none;border-radius:6px;background:#7C3AED;color:#fff;cursor:pointer;font-size:13px;font-weight:600">Salvar</button>
    </div>
  </div>
</div>
```

- [ ] **Step 2: Add CSS for the AI config dialog**

Insert after line 710 (after the dark-theme `.review-produto-chip.selected` rule, before the `:root[data-theme="light"]` block):

```css
.ai-config-dialog { background:var(--surface); color:var(--text-primary); border:1px solid var(--border); border-radius:12px; padding:24px; max-width:480px; width:90%; }
.ai-config-dialog h3 { margin:0 0 8px; font-size:18px; color:var(--text-primary); }
.ai-config-instructions { font-size:13px; color:var(--text-secondary); margin-bottom:20px; line-height:1.5; padding:12px; background:var(--surface-secondary); border-radius:8px; border:1px solid var(--border-subtle); }
.ai-config-dialog label { display:block; font-size:12px; font-weight:600; color:var(--text-secondary); margin:12px 0 4px; text-transform:uppercase; }
.ai-config-dialog input, .ai-config-dialog select { width:100%; padding:8px 12px; border:1px solid var(--border); border-radius:6px; font-size:13px; background:var(--surface); color:var(--text-primary); box-sizing:border-box; }
.ai-config-actions { display:flex; gap:8px; justify-content:flex-end; margin-top:20px; }
```

- [ ] **Step 3: Replace JavaScript functions**

Replace `showApiKeyModal()` and `saveApiKey()` (lines ~4748-4764) with these five functions:

```javascript
function showAIConfigModal() {
  var modal = document.getElementById('ai-config-modal');
  modal.style.display = 'flex';
  document.getElementById('ai-config-test-result').innerHTML = '';
  document.getElementById('ai-api-key').value = '';

  api('/config/ai_base_url').then(function(r) {
    if (r && r.valor) {
      document.getElementById('ai-base-url').value = r.valor;
      var sel = document.getElementById('ai-provider-select');
      var matched = false;
      for (var i = 0; i < sel.options.length; i++) {
        if (sel.options[i].dataset.url === r.valor) {
          sel.selectedIndex = i;
          matched = true;
          break;
        }
      }
      if (!matched) sel.value = 'custom';
    }
  }).catch(function() {});

  api('/config/ai_model').then(function(r) {
    if (r && r.valor) document.getElementById('ai-model').value = r.valor;
  }).catch(function() {});

  api('/config/ai_api_key').then(function(r) {
    if (r && r.exists) document.getElementById('ai-api-key').placeholder = '(configurada — deixe vazio para manter)';
  }).catch(function() {});
}

function closeAIConfigModal() {
  document.getElementById('ai-config-modal').style.display = 'none';
}

function onAIProviderChange() {
  var sel = document.getElementById('ai-provider-select');
  var opt = sel.options[sel.selectedIndex];
  document.getElementById('ai-base-url').value = opt.dataset.url || '';
  document.getElementById('ai-api-key').placeholder = opt.dataset.placeholder || '...';
  document.getElementById('ai-model').value = opt.dataset.model || '';
  document.getElementById('ai-config-test-result').innerHTML = '';
}

function saveAIConfig() {
  var baseUrl = document.getElementById('ai-base-url').value.trim();
  var apiKey = document.getElementById('ai-api-key').value.trim();
  var model = document.getElementById('ai-model').value.trim();

  if (!baseUrl || !model) {
    alert('Base URL e Modelo são obrigatórios');
    return;
  }

  var promises = [
    api('/config', { method: 'POST', body: JSON.stringify({ chave: 'ai_base_url', valor: baseUrl }) }),
    api('/config', { method: 'POST', body: JSON.stringify({ chave: 'ai_model', valor: model }) })
  ];
  if (apiKey) {
    promises.push(api('/config', { method: 'POST', body: JSON.stringify({ chave: 'ai_api_key', valor: apiKey }) }));
  }

  Promise.all(promises).then(function() {
    closeAIConfigModal();
  }).catch(function(e) { alert('Erro ao salvar: ' + e.message); });
}

function testAIConfig() {
  var baseUrl = document.getElementById('ai-base-url').value.trim();
  var apiKey = document.getElementById('ai-api-key').value.trim();
  var model = document.getElementById('ai-model').value.trim();
  var resultEl = document.getElementById('ai-config-test-result');

  if (!baseUrl || !model) {
    resultEl.innerHTML = '<span style="color:#EF4444">Base URL e Modelo são obrigatórios</span>';
    return;
  }

  resultEl.innerHTML = '<span class="spinner" style="display:inline-block;width:14px;height:14px;margin-right:6px;vertical-align:middle"></span>Testando conexão...';

  api('/config/ai/test', {
    method: 'POST',
    body: JSON.stringify({ base_url: baseUrl, api_key: apiKey, model: model })
  }).then(function(r) {
    if (r.status === 'ok') {
      resultEl.innerHTML = '<span style="color:#10B981;font-weight:600">✓ Conexão OK — modelo: ' + esc(r.model) + '</span>';
    } else {
      resultEl.innerHTML = '<span style="color:#EF4444;font-weight:600">✗ Erro: ' + esc(r.error) + '</span>';
    }
  }).catch(function(e) {
    resultEl.innerHTML = '<span style="color:#EF4444;font-weight:600">✗ ' + esc(e.message) + '</span>';
  });
}
```

- [ ] **Step 4: Update the ⚙️ button trigger**

In `renderReviewContent()` (~line 4161), replace:

```javascript
html += '<button class="review-analise-btn-settings" onclick="showApiKeyModal()" title="Configurar API Key">⚙️</button>';
```

With:

```javascript
html += '<button class="review-analise-btn-settings" onclick="showAIConfigModal()" title="Configurar IA">⚙️</button>';
```

- [ ] **Step 5: Update the 503 handler**

In `generateReviewAnalise()` (~line 4784), replace:

```javascript
      showApiKeyModal();
```

With:

```javascript
      showAIConfigModal();
```

- [ ] **Step 6: Verify in browser**

Run `./dev.sh`, open review tab:
- Click ⚙️ → new modal opens with provider dropdown, base URL, API key, model fields
- Modal respects dark/light theme (no hardcoded `#fff`)
- Select "Groq" → fields auto-fill with Groq URL and model
- Select "Personalizado" → fields clear for manual entry
- "Testar Conexão" → shows green/red result
- "Salvar" → saves config, closes modal
- Generate AI analysis → uses saved config (test with real API key)

- [ ] **Step 7: Commit**

```bash
git add frontend/index.html
git commit -m "feat: replace OpenRouter modal with provider-agnostic AI config

New modal supports any OpenAI-compatible provider with dropdown
templates for OpenRouter, OpenAI, Groq, Together, Ollama.
Includes test connection button and theme-aware CSS."
```
