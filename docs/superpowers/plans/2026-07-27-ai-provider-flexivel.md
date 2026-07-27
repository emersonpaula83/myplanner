# AI Provider Flexível Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace hardcoded OpenRouter coupling with a configurable AI provider supporting any OpenAI-compatible API.

**Architecture:** Rename `openrouter.go` → `ai_client.go` and add `baseURL` as a constructor parameter. Rename config keys from `openrouter_*` to `ai_*` and add `ai_base_url`. Expand frontend modal from single API key input to 4-field provider configuration (provider select, base URL, API key, model).

**Tech Stack:** Go (chi, pgx/v5, zap), PostgreSQL, vanilla JS

## Global Constraints

- Frontend: vanilla JS, `var`/`function` only, no ES6+. Use `esc()` for all dynamic text in HTML.
- Backend: Go with chi router, pgx/v5, zap logger.
- Do NOT commit automatically — leave all changes unstaged.
- Dark mode: CSS custom properties + `@media (prefers-color-scheme: dark)` + `:root[data-theme="dark"]`.
- `ai_api_key` is NEVER returned to the frontend. `GetConfig` returns only `{"exists": true/false}` for it.
- Provider select in the modal is UX-only — not saved to the database. Only `ai_base_url`, `ai_api_key`, `ai_model` are persisted.

---

### Task 1: Migration + Backend Rename (ai_client.go, service, handler)

**Files:**
- Create: `backend/migrations/000021_ai_provider_config.up.sql`
- Create: `backend/migrations/000021_ai_provider_config.down.sql`
- Create: `backend/internal/service/ai_client.go` (replaces `openrouter.go`)
- Delete: `backend/internal/service/openrouter.go`
- Modify: `backend/internal/service/review.go:360-382` (GenerateAnalise config key names + NewAIClient call)
- Modify: `backend/internal/handler/review.go:35-37,248` (whitelist + sensitive key name)
- Modify: `backend/internal/handler/review_test.go` (no code changes expected, but verify compilation)

**Interfaces:**
- Produces: `NewAIClient(apiKey, model, baseURL string) *AIClient` — used by `GenerateAnalise` in `review.go`
- Produces: updated `configWhitelist` with keys `ai_api_key`, `ai_model`, `ai_base_url`
- Produces: migration 000021 renaming config rows

- [ ] **Step 1: Create migration UP file**

Create `backend/migrations/000021_ai_provider_config.up.sql`:

```sql
UPDATE configuracoes SET chave = 'ai_api_key' WHERE chave = 'openrouter_api_key';
UPDATE configuracoes SET chave = 'ai_model' WHERE chave = 'openrouter_model';
INSERT INTO configuracoes (chave, valor)
VALUES ('ai_base_url', 'https://openrouter.ai/api/v1')
ON CONFLICT (chave) DO NOTHING;
```

- [ ] **Step 2: Create migration DOWN file**

Create `backend/migrations/000021_ai_provider_config.down.sql`:

```sql
UPDATE configuracoes SET chave = 'openrouter_api_key' WHERE chave = 'ai_api_key';
UPDATE configuracoes SET chave = 'openrouter_model' WHERE chave = 'ai_model';
DELETE FROM configuracoes WHERE chave = 'ai_base_url';
```

- [ ] **Step 3: Create `ai_client.go`**

Create `backend/internal/service/ai_client.go` with this exact content:

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
			return "", fmt.Errorf("ai provider failed after retry: %w", err)
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
		return "", fmt.Errorf("calling ai provider: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ai provider returned %d: %s", resp.StatusCode, string(respBody))
	}

	var aiResp aiResponse
	if err := json.Unmarshal(respBody, &aiResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if aiResp.Error != nil {
		return "", fmt.Errorf("ai provider error: %s", aiResp.Error.Message)
	}

	if len(aiResp.Choices) == 0 || aiResp.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("empty response from ai provider")
	}

	return aiResp.Choices[0].Message.Content, nil
}
```

- [ ] **Step 4: Delete `openrouter.go`**

```bash
rm backend/internal/service/openrouter.go
```

- [ ] **Step 5: Update `GenerateAnalise` in `review.go`**

In `backend/internal/service/review.go`, replace lines 360-382 of `GenerateAnalise`. The changes are:

1. Config key `"openrouter_api_key"` → `"ai_api_key"`
2. Error message `"openrouter API key not configured"` → `"AI API key not configured"`
3. Config key `"openrouter_model"` → `"ai_model"`
4. Add `ai_base_url` read with default `"https://openrouter.ai/api/v1"`
5. `NewOpenRouterClient(apiKey, model)` → `NewAIClient(apiKey, model, baseURL)`

Replace the block from `apiKey, err := s.configRepo.GetConfig(ctx, "openrouter_api_key")` through `client := NewOpenRouterClient(apiKey, model)` with:

```go
	apiKey, err := s.configRepo.GetConfig(ctx, "ai_api_key")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("AI API key not configured: %w", err)
		}
		return nil, err
	}

	model := "openai/gpt-oss-20b:free"
	if m, err := s.configRepo.GetConfig(ctx, "ai_model"); err == nil && m != "" {
		model = m
	}

	baseURL := "https://openrouter.ai/api/v1"
	if u, err := s.configRepo.GetConfig(ctx, "ai_base_url"); err == nil && u != "" {
		baseURL = u
	}

	reviewData, err := s.GetReviewData(ctx, sprintID, equipeID, produtoIDs)
	if err != nil {
		return nil, fmt.Errorf("getting review data for analysis: %w", err)
	}

	systemPrompt, userPrompt := buildReviewAnalisePrompt(reviewData.Tarefas)

	client := NewAIClient(apiKey, model, baseURL)
```

- [ ] **Step 6: Update handler config whitelist and sensitive key check**

In `backend/internal/handler/review.go`:

Replace the whitelist (lines 35-38):
```go
var configWhitelist = map[string]bool{
	"ai_api_key":  true,
	"ai_model":    true,
	"ai_base_url": true,
}
```

Replace the sensitive key check in `GetConfig` (line 248):
```go
	if chave == "ai_api_key" {
```

- [ ] **Step 7: Update handler error string check in `PostReviewAnalise`**

In `backend/internal/handler/review.go`, the `PostReviewAnalise` handler checks error message for `"not configured"` (line 379). The new error message `"AI API key not configured"` still contains `"not configured"`, so no change needed. Verify this by reading line 379:

```go
		if strings.Contains(err.Error(), "not configured") {
```

Confirm: matches new message `"AI API key not configured"`. No change needed.

- [ ] **Step 8: Build and test**

```bash
cd backend
go build ./...
go vet ./...
go test ./...
```

All must pass. The test file (`review_test.go`) does not reference config key names directly — the `mockConfigStore` uses function fields, so no changes needed there.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/service/ai_client.go \
       backend/internal/service/review.go \
       backend/internal/handler/review.go \
       backend/migrations/000021_ai_provider_config.up.sql \
       backend/migrations/000021_ai_provider_config.down.sql
git rm backend/internal/service/openrouter.go
git commit -m "refactor: rename OpenRouter client to generic AI client

Rename openrouter.go to ai_client.go, generalize types and error
messages, accept baseURL as constructor parameter. Rename config
keys from openrouter_* to ai_* and add ai_base_url. Migration
000021 renames existing config rows."
```

---

### Task 2: Frontend — Expand API Key Modal to AI Provider Config

**Files:**
- Modify: `frontend/index.html` — HTML modal (~line 1112-1122), JS functions `showApiKeyModal` (~line 3003), `saveApiKey` (~line 3010), settings button title (~line 2625)

**Interfaces:**
- Consumes: Backend endpoints unchanged — `GET /config/{chave}`, `POST /config` with `{chave, valor}`. New config keys: `ai_api_key`, `ai_model`, `ai_base_url`.
- Produces: Updated modal UI and JS functions. No new functions consumed by other code — `showApiKeyModal()` is already called from the settings button and from `generateReviewAnalise` on 503.

- [ ] **Step 1: Replace the `#apikey-modal` HTML**

In `frontend/index.html`, replace the entire `<div id="apikey-modal" ...>` block (lines 1112-1122) with:

```html
<div id="apikey-modal" style="display:none;position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.5);z-index:9999;justify-content:center;align-items:center">
  <div style="background:var(--bg-primary,#fff);border-radius:10px;padding:24px;max-width:480px;width:90%">
    <h3 style="margin:0 0 16px;font-size:16px;color:var(--text-primary,#1a1a2e)">Configurar AI Provider</h3>
    <div style="margin-bottom:12px">
      <label style="display:block;font-size:12px;font-weight:600;color:var(--text-secondary,#666);margin-bottom:4px">Provider</label>
      <select id="ai-provider-select" onchange="onAiProviderChange()" style="width:100%;padding:8px 12px;border:1px solid var(--border-color,#ddd);border-radius:6px;font-size:13px;box-sizing:border-box;background:var(--bg-primary,#fff);color:var(--text-primary,#1a1a2e)">
        <option value="openrouter">OpenRouter</option>
        <option value="openai">OpenAI</option>
        <option value="azure">Azure OpenAI</option>
        <option value="custom">Custom</option>
      </select>
    </div>
    <div style="margin-bottom:12px">
      <label style="display:block;font-size:12px;font-weight:600;color:var(--text-secondary,#666);margin-bottom:4px">Base URL</label>
      <input id="ai-baseurl-input" type="text" placeholder="https://openrouter.ai/api/v1" style="width:100%;padding:8px 12px;border:1px solid var(--border-color,#ddd);border-radius:6px;font-size:13px;box-sizing:border-box;background:var(--bg-primary,#fff);color:var(--text-primary,#1a1a2e)">
    </div>
    <div style="margin-bottom:12px">
      <label style="display:block;font-size:12px;font-weight:600;color:var(--text-secondary,#666);margin-bottom:4px">API Key</label>
      <input id="ai-apikey-input" type="password" placeholder="" style="width:100%;padding:8px 12px;border:1px solid var(--border-color,#ddd);border-radius:6px;font-size:13px;box-sizing:border-box;background:var(--bg-primary,#fff);color:var(--text-primary,#1a1a2e)">
    </div>
    <div style="margin-bottom:16px">
      <label style="display:block;font-size:12px;font-weight:600;color:var(--text-secondary,#666);margin-bottom:4px">Modelo</label>
      <input id="ai-model-input" type="text" placeholder="openai/gpt-oss-20b:free" style="width:100%;padding:8px 12px;border:1px solid var(--border-color,#ddd);border-radius:6px;font-size:13px;box-sizing:border-box;background:var(--bg-primary,#fff);color:var(--text-primary,#1a1a2e)">
    </div>
    <div style="display:flex;gap:8px;justify-content:flex-end">
      <button onclick="document.getElementById('apikey-modal').style.display='none'" style="padding:8px 16px;border:1px solid var(--border-color,#ddd);border-radius:6px;background:var(--bg-primary,#fff);cursor:pointer;font-size:13px;color:var(--text-primary,#1a1a2e)">Cancelar</button>
      <button onclick="saveAiConfig()" style="padding:8px 16px;border:none;border-radius:6px;background:#7C3AED;color:#fff;cursor:pointer;font-size:13px;font-weight:600">Salvar</button>
    </div>
  </div>
</div>
```

- [ ] **Step 2: Update settings button title**

In `renderReviewContent` (~line 2625), change the title attribute:

```js
  html += '<button class="review-analise-btn-settings" onclick="showApiKeyModal()" title="Configurar AI Provider">⚙️</button>';
```

- [ ] **Step 3: Replace `showApiKeyModal` and `saveApiKey` JS functions**

Replace the existing `showApiKeyModal` function (lines 3003-3008) and `saveApiKey` function (lines 3010-3019) with these new functions:

```js
var _aiProviderPresets = {
  openrouter: { url: 'https://openrouter.ai/api/v1', model: 'openai/gpt-oss-20b:free' },
  openai: { url: 'https://api.openai.com/v1', model: 'gpt-4o-mini' },
  azure: { url: '', model: '' },
  custom: { url: '', model: '' }
};

function onAiProviderChange() {
  var sel = document.getElementById('ai-provider-select');
  var preset = _aiProviderPresets[sel.value];
  if (!preset) return;
  document.getElementById('ai-baseurl-input').value = preset.url;
  document.getElementById('ai-model-input').value = preset.model;
}

function showApiKeyModal() {
  var modal = document.getElementById('apikey-modal');
  modal.style.display = 'flex';
  document.getElementById('ai-apikey-input').value = '';

  fetch(API + '/config/ai_base_url', { headers: { 'Authorization': 'Bearer ' + token } })
    .then(function(r) { return r.ok ? r.json() : null; })
    .then(function(d) {
      if (d && d.valor) {
        document.getElementById('ai-baseurl-input').value = d.valor;
        var sel = document.getElementById('ai-provider-select');
        var found = false;
        for (var key in _aiProviderPresets) {
          if (_aiProviderPresets[key].url === d.valor) {
            sel.value = key;
            found = true;
            break;
          }
        }
        if (!found) sel.value = 'custom';
      }
    }).catch(function() {});

  fetch(API + '/config/ai_model', { headers: { 'Authorization': 'Bearer ' + token } })
    .then(function(r) { return r.ok ? r.json() : null; })
    .then(function(d) {
      if (d && d.valor) document.getElementById('ai-model-input').value = d.valor;
    }).catch(function() {});

  fetch(API + '/config/ai_api_key', { headers: { 'Authorization': 'Bearer ' + token } })
    .then(function(r) { return r.ok ? r.json() : null; })
    .then(function(d) {
      var inp = document.getElementById('ai-apikey-input');
      if (d && d.exists) {
        inp.placeholder = '••••••••••••••••  (salva, deixe vazio para manter)';
      } else {
        inp.placeholder = 'Cole sua API key aqui';
      }
    }).catch(function() {});
}

function saveAiConfig() {
  var baseUrl = document.getElementById('ai-baseurl-input').value.trim();
  var apiKey = document.getElementById('ai-apikey-input').value.trim();
  var model = document.getElementById('ai-model-input').value.trim();

  if (!baseUrl) { alert('Base URL é obrigatória'); return; }
  if (!model) { alert('Modelo é obrigatório'); return; }

  var promises = [
    api('/config', { method: 'POST', body: JSON.stringify({ chave: 'ai_base_url', valor: baseUrl }) }),
    api('/config', { method: 'POST', body: JSON.stringify({ chave: 'ai_model', valor: model }) })
  ];
  if (apiKey) {
    promises.push(api('/config', { method: 'POST', body: JSON.stringify({ chave: 'ai_api_key', valor: apiKey }) }));
  }

  Promise.all(promises).then(function() {
    document.getElementById('apikey-modal').style.display = 'none';
  }).catch(function(e) { alert('Erro ao salvar: ' + e.message); });
}
```

- [ ] **Step 4: Verify no remaining references to old function/element names**

Search for stale references. These should NOT exist after the changes:
- `saveApiKey` — renamed to `saveAiConfig`
- `apikey-input` — renamed to `ai-apikey-input`
- `openrouter_api_key` — renamed to `ai_api_key` in JS

Check in `generateReviewAnalise` (~line 3039): the 503 handler calls `showApiKeyModal()` — this function name is unchanged, so no update needed there.

```bash
grep -n "saveApiKey\|apikey-input\|openrouter_api_key" frontend/index.html
```

The only match should be zero. If any remain, update them.

- [ ] **Step 5: Verify JS syntax**

Extract the main `<script>` block and check syntax:

```bash
sed -n '/<script>/,/<\/script>/p' frontend/index.html | sed '1d;$d' > /tmp/check.js
node --check /tmp/check.js
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add frontend/index.html
git commit -m "feat: expand AI config modal with provider selector

Replace single API key input with 4-field modal: provider select
(OpenRouter/OpenAI/Azure/Custom), base URL (auto-filled from
provider), API key, and model. Provider select is UX-only helper.
Loads current values on open. Saves ai_base_url, ai_model, and
ai_api_key (only if changed) via POST /config."
```

---

## Self-Review Checklist

**Spec coverage:**
- [x] Migration renaming config keys — Task 1 Step 1-2
- [x] `openrouter.go` → `ai_client.go` rename — Task 1 Step 3-4
- [x] `AIClient` with `baseURL` parameter — Task 1 Step 3
- [x] `GenerateAnalise` reads `ai_*` keys with defaults — Task 1 Step 5
- [x] Handler whitelist updated — Task 1 Step 6
- [x] `ai_api_key` sensitive (exists-only) — Task 1 Step 6 (check remains `== "ai_api_key"`)
- [x] Error messages changed from "openrouter" to "ai provider" — Task 1 Step 3
- [x] Frontend modal with 4 fields — Task 2 Step 1
- [x] Provider select auto-fills base URL and model — Task 2 Step 3
- [x] Modal loads current values on open — Task 2 Step 3
- [x] API key not overwritten if empty — Task 2 Step 3 (saveAiConfig skips if empty)
- [x] Security: ai_api_key never exposed — handler check in Task 1 Step 6
- [x] Dark mode: modal uses CSS custom properties — Task 2 Step 1
- [x] Tests compile — Task 1 Step 8

**Placeholder scan:** No TBD/TODO found.

**Type consistency:** `NewAIClient(apiKey, model, baseURL string)` — same signature in Task 1 Step 3 (definition) and Step 5 (call site).
