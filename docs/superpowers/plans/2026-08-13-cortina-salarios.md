# Cortina de Salários Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Esconder todo valor salarial por padrão e só liberá-lo depois que o usuário digitar a própria senha, sem que o número saia do servidor enquanto estiver travado.

**Architecture:** O desbloqueio vive numa claim `salarios` do JWT. Uma rota nova valida cargo + senha e devolve um token novo com a claim, preservando a expiração restante. O middleware de auth injeta a claim no contexto; cada handler que toca dinheiro consulta `middleware.PodeVerSalarios(ctx)` e limpa os campos antes de responder. Campos de dinheiro que hoje são `float64` viram `*float64` com `omitempty`, para sumirem do JSON em vez de virarem zero.

**Tech Stack:** Go 1.x, chi v5, golang-jwt/v5, bcrypt (`golang.org/x/crypto/bcrypt`), pgx/pgxpool, zap; frontend HTML/JS puro num arquivo só (`frontend/index.html`).

**Spec:** `docs/superpowers/specs/2026-08-13-cortina-salarios-design.md`

## Global Constraints

- Cargos que podem destravar: `coordenador`, `gerente`, `diretor`. A conta admin (`cfg.Auth.AdminEmail`) passa independente do cargo.
- Cargo `diretor` é criado por esta feature. Sem migration: `usuarios.cargo` é `VARCHAR(50)` sem CHECK.
- Duração do desbloqueio: até o token expirar ou o usuário deslogar. **A rota de desbloqueio preserva a expiração restante do token atual** — nunca estende a sessão.
- Enquanto travado, o valor **não pode existir no corpo da resposta**. Campo ausente, não zero.
- Erros: cargo sem permissão → **403**; senha errada → **401**. Mensagens em português, via `respondError(w, status, "texto")`.
- Comentários de código em português, explicando o porquê.
- TDD: teste primeiro, rodar e ver falhar, implementar, rodar e ver passar, commitar.
- Não incluir `.claude/settings.json` nos commits — está modificado na árvore e não pertence a esta feature.
- Rodar a suíte com `cd backend && go test ./...` antes de cada commit.

---

### Task 1: Claim no token e cargo `diretor`

**Files:**
- Modify: `backend/internal/auth/jwt.go`
- Modify: `backend/internal/auth/jwt_test.go`
- Modify: `backend/internal/handler/usuario.go:18-22`
- Modify: `frontend/index.html` (select de cargo da tela de usuários)

**Interfaces:**
- Consumes: nada.
- Produces:
  - `auth.Claims` com campo `Salarios bool \`json:"salarios,omitempty"\``
  - `func (ts *TokenService) GenerateTokenComExpiracao(userID uuid.UUID, email, cargo string, salarios bool, expiraEm time.Time) (string, error)`
  - `GenerateToken` mantém a assinatura atual e emite `salarios: false`.

  Consumidos pelas Tasks 2 e 3.

- [ ] **Step 1: Escrever os testes que falham**

Acrescentar ao fim de `backend/internal/auth/jwt_test.go`:

```go
func TestTokenSemClaimDeSalariosValidaComoFalso(t *testing.T) {
	ts := NewTokenService("segredo-de-teste", 24)
	token, err := ts.GenerateToken(uuid.New(), "user@myplanner.local", "coordenador")
	if err != nil {
		t.Fatalf("gerando token: %v", err)
	}

	claims, err := ts.ValidateToken(token)
	if err != nil {
		t.Fatalf("validando token: %v", err)
	}
	// omitempty não pode virar true por acidente: login nunca destrava salário.
	if claims.Salarios {
		t.Error("token de login veio com salarios=true")
	}
}

func TestGenerateTokenComExpiracaoPreservaClaimEExpiracao(t *testing.T) {
	ts := NewTokenService("segredo-de-teste", 24)
	expiraEm := time.Now().Add(37 * time.Minute).Truncate(time.Second)

	token, err := ts.GenerateTokenComExpiracao(uuid.New(), "user@myplanner.local", "gerente", true, expiraEm)
	if err != nil {
		t.Fatalf("gerando token: %v", err)
	}

	claims, err := ts.ValidateToken(token)
	if err != nil {
		t.Fatalf("validando token: %v", err)
	}
	if !claims.Salarios {
		t.Error("esperava salarios=true no token destravado")
	}
	if !claims.ExpiresAt.Time.Equal(expiraEm) {
		t.Errorf("expiração = %v, esperava %v — destravar não pode estender a sessão", claims.ExpiresAt.Time, expiraEm)
	}
	if claims.Cargo != "gerente" || claims.Email != "user@myplanner.local" {
		t.Errorf("claims perderam identidade: %+v", claims)
	}
}
```

Conferir que o arquivo importa `time` e `github.com/google/uuid`; acrescentar ao bloco de import se faltar.

- [ ] **Step 2: Rodar os testes para ver falhar**

Run: `cd backend && go test ./internal/auth/ -run "Salarios|Expiracao" -v`
Expected: FAIL na compilação — `claims.Salarios undefined` e `ts.GenerateTokenComExpiracao undefined`.

- [ ] **Step 3: Implementar a claim e o gerador com expiração explícita**

Em `backend/internal/auth/jwt.go`, substituir o struct `Claims` e acrescentar o método:

```go
type Claims struct {
	Email string `json:"email"`
	Cargo string `json:"cargo"`
	// Salarios diz se este token pode ver valores salariais. Fica no token
	// porque o desbloqueio dura o que a sessão durar — sem estado no servidor.
	Salarios bool `json:"salarios,omitempty"`
	jwt.RegisteredClaims
}
```

E, logo depois de `GenerateToken`:

```go
// GenerateTokenComExpiracao emite um token com expiração ditada por quem chama.
// É o que permite destravar salários sem renovar a sessão: o token novo herda o
// prazo que restava do antigo.
func (ts *TokenService) GenerateTokenComExpiracao(userID uuid.UUID, email, cargo string, salarios bool, expiraEm time.Time) (string, error) {
	claims := Claims{
		Email:    email,
		Cargo:    cargo,
		Salarios: salarios,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiraEm),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(ts.secret)
}
```

`GenerateToken` fica como está — sem a claim, ele emite `salarios` ausente, que valida como `false`.

- [ ] **Step 4: Rodar os testes para ver passar**

Run: `cd backend && go test ./internal/auth/ -v`
Expected: PASS em todos.

- [ ] **Step 5: Criar o cargo `diretor`**

Em `backend/internal/handler/usuario.go`, o mapa `cargosValidos` (linha 18) passa a:

```go
var cargosValidos = map[string]bool{
	"coordenador":      true,
	"gerente":          true,
	"gerente_projetos": true,
	"diretor":          true,
}
```

E as duas mensagens de erro (linhas 69 e 139) passam a `"cargo inválido: deve ser coordenador, gerente, gerente_projetos ou diretor"`.

- [ ] **Step 6: Acrescentar `diretor` ao select de cargo no frontend**

Localizar o select de cargo da tela de usuários:

Run: `grep -n 'value="gerente_projetos"' frontend/index.html`

Acrescentar, logo depois da opção `gerente_projetos` encontrada, no mesmo formato usado ali:

```html
<option value="diretor">Diretor</option>
```

- [ ] **Step 7: Rodar a suíte inteira**

Run: `cd backend && go build ./... && go test ./...`
Expected: build limpo e `ok` em todos os pacotes.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/auth/jwt.go backend/internal/auth/jwt_test.go backend/internal/handler/usuario.go frontend/index.html
git commit -m "feat: claim de salários no token e cargo diretor"
```

---

### Task 2: Middleware expõe a claim e a expiração

**Files:**
- Modify: `backend/internal/middleware/auth.go`
- Create: `backend/internal/middleware/auth_test.go`

**Interfaces:**
- Consumes: `auth.Claims.Salarios` (Task 1).
- Produces:
  - `func PodeVerSalarios(ctx context.Context) bool`
  - `func TokenExpiraEm(ctx context.Context) time.Time`

  Consumidos pelas Tasks 3 a 6.

- [ ] **Step 1: Escrever os testes que falham**

`backend/internal/middleware/auth_test.go`:

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/auth"
	"github.com/google/uuid"
)

func TestAuthJWTPropagaClaimDeSalarios(t *testing.T) {
	ts := auth.NewTokenService("segredo-de-teste", 24)
	expiraEm := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	token, err := ts.GenerateTokenComExpiracao(uuid.New(), "user@myplanner.local", "gerente", true, expiraEm)
	if err != nil {
		t.Fatalf("gerando token: %v", err)
	}

	var pode bool
	var expira time.Time
	handler := AuthJWT(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pode = PodeVerSalarios(r.Context())
		expira = TokenExpiraEm(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/qualquer", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !pode {
		t.Error("PodeVerSalarios = false para token destravado")
	}
	if !expira.Equal(expiraEm) {
		t.Errorf("TokenExpiraEm = %v, esperava %v", expira, expiraEm)
	}
}

func TestAuthJWTTokenDeLoginNaoDestravaSalarios(t *testing.T) {
	ts := auth.NewTokenService("segredo-de-teste", 24)
	token, err := ts.GenerateToken(uuid.New(), "user@myplanner.local", "coordenador")
	if err != nil {
		t.Fatalf("gerando token: %v", err)
	}

	pode := true
	handler := AuthJWT(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pode = PodeVerSalarios(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/qualquer", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if pode {
		t.Error("token de login destravou salários")
	}
}

// Contexto sem middleware (ex.: chamada interna) nunca pode liberar valor.
func TestPodeVerSalariosPadraoEhFalso(t *testing.T) {
	if PodeVerSalarios(context.Background()) {
		t.Error("contexto vazio liberou salários")
	}
}
```

Acrescentar `"context"` ao bloco de import do arquivo de teste.

- [ ] **Step 2: Rodar os testes para ver falhar**

Run: `cd backend && go test ./internal/middleware/ -run Salarios -v`
Expected: FAIL na compilação — `PodeVerSalarios` e `TokenExpiraEm` não existem.

- [ ] **Step 3: Injetar a claim e a expiração no contexto**

Em `backend/internal/middleware/auth.go`, acrescentar as duas chaves ao bloco `const`:

```go
const (
	userIDKey    contextKey = "user_id"
	userEmailKey contextKey = "user_email"
	userCargoKey contextKey = "user_cargo"
	salariosKey  contextKey = "salarios_desbloqueados"
	tokenExpKey  contextKey = "token_expira_em"
)
```

Dentro de `AuthJWT`, logo depois da linha que injeta `userCargoKey`:

```go
			ctx = context.WithValue(ctx, salariosKey, claims.Salarios)
			if claims.ExpiresAt != nil {
				ctx = context.WithValue(ctx, tokenExpKey, claims.ExpiresAt.Time)
			}
```

E, junto das outras funções de leitura de contexto:

```go
// PodeVerSalarios diz se a requisição pode receber valores salariais. Falso por
// padrão: contexto sem a claim (chamada interna, token de login) nunca libera.
func PodeVerSalarios(ctx context.Context) bool {
	pode, _ := ctx.Value(salariosKey).(bool)
	return pode
}

// TokenExpiraEm devolve o fim da sessão atual. Destravar salários reemite o
// token com esta mesma expiração, para não renovar a sessão de brinde.
func TokenExpiraEm(ctx context.Context) time.Time {
	exp, _ := ctx.Value(tokenExpKey).(time.Time)
	return exp
}
```

Acrescentar `"time"` ao bloco de import do arquivo.

- [ ] **Step 4: Rodar os testes para ver passar**

Run: `cd backend && go test ./internal/middleware/ -v`
Expected: PASS em todos.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/middleware/auth.go backend/internal/middleware/auth_test.go
git commit -m "feat: middleware expõe claim de salários e expiração do token"
```

---

### Task 3: Rotas de desbloquear e travar

**Files:**
- Create: `backend/internal/handler/salario_lock.go`
- Create: `backend/internal/handler/salario_lock_test.go`
- Modify: `backend/cmd/api/main.go` (wiring perto da linha 87 e rotas perto da linha 227)

**Interfaces:**
- Consumes: `auth.TokenService.GenerateTokenComExpiracao` (Task 1); `middleware.TokenExpiraEm` (Task 2).
- Produces:
  - `POST /api/v1/auth/desbloquear-salarios` — corpo `{"senha":"..."}`, resposta `{"token":"..."}`
  - `POST /api/v1/auth/travar-salarios` — sem corpo, resposta `{"token":"..."}`

  Consumidos pela Task 7 (frontend).

- [ ] **Step 1: Escrever os testes que falham**

`backend/internal/handler/salario_lock_test.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/auth"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	mw "github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type mockSalarioLockStore struct {
	buscarPorEmailFn func(ctx context.Context, email string) (*domain.Usuario, error)
}

func (m *mockSalarioLockStore) BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error) {
	return m.buscarPorEmailFn(ctx, email)
}

func usuarioDeTeste(t *testing.T, email, cargo, senha string) *domain.Usuario {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(senha), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("gerando hash: %v", err)
	}
	h := string(hash)
	return &domain.Usuario{ID: uuid.New(), Email: email, Cargo: cargo, SenhaHash: &h, AuthProvider: "local"}
}

// requestDestravar monta a requisição já com o que o middleware de auth injeta.
func requestDestravar(corpo, email, cargo string, expiraEm time.Time) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/auth/desbloquear-salarios", strings.NewReader(corpo))
	ctx := mw.ContextParaTeste(req.Context(), uuid.New(), email, cargo, expiraEm)
	return req.WithContext(ctx)
}

func TestDesbloquearRecusaCargoSemPermissao(t *testing.T) {
	chamou := false
	store := &mockSalarioLockStore{buscarPorEmailFn: func(context.Context, string) (*domain.Usuario, error) {
		chamou = true
		return nil, nil
	}}
	h := NewSalarioLockHandler(store, auth.NewTokenService("s", 24), "admin@myplanner.local", zap.NewNop())

	w := httptest.NewRecorder()
	h.Desbloquear(w, requestDestravar(`{"senha":"seja-la-qual"}`, "dev@myplanner.local", "gerente_projetos", time.Now().Add(time.Hour)))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, esperava 403. Corpo: %s", w.Code, w.Body.String())
	}
	if chamou {
		t.Error("cargo sem permissão não deveria nem consultar o usuário")
	}
}

func TestDesbloquearRecusaSenhaErrada(t *testing.T) {
	u := usuarioDeTeste(t, "chefe@myplanner.local", "gerente", "senha-certa")
	store := &mockSalarioLockStore{buscarPorEmailFn: func(context.Context, string) (*domain.Usuario, error) {
		return u, nil
	}}
	h := NewSalarioLockHandler(store, auth.NewTokenService("s", 24), "admin@myplanner.local", zap.NewNop())

	w := httptest.NewRecorder()
	h.Desbloquear(w, requestDestravar(`{"senha":"senha-errada"}`, u.Email, u.Cargo, time.Now().Add(time.Hour)))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, esperava 401", w.Code)
	}
}

func TestDesbloquearDevolveTokenComClaimEMesmaExpiracao(t *testing.T) {
	u := usuarioDeTeste(t, "chefe@myplanner.local", "diretor", "senha-certa")
	store := &mockSalarioLockStore{buscarPorEmailFn: func(context.Context, string) (*domain.Usuario, error) {
		return u, nil
	}}
	ts := auth.NewTokenService("s", 24)
	h := NewSalarioLockHandler(store, ts, "admin@myplanner.local", zap.NewNop())
	expiraEm := time.Now().Add(90 * time.Minute).Truncate(time.Second)

	w := httptest.NewRecorder()
	h.Desbloquear(w, requestDestravar(`{"senha":"senha-certa"}`, u.Email, u.Cargo, expiraEm))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, esperava 200. Corpo: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta inválida: %v", err)
	}
	claims, err := ts.ValidateToken(resp.Token)
	if err != nil {
		t.Fatalf("token devolvido não valida: %v", err)
	}
	if !claims.Salarios {
		t.Error("token devolvido veio sem a claim de salários")
	}
	if !claims.ExpiresAt.Time.Equal(expiraEm) {
		t.Errorf("expiração = %v, esperava %v — destravar não pode estender a sessão", claims.ExpiresAt.Time, expiraEm)
	}
}

// A conta admin passa mesmo com cargo fora da lista.
func TestDesbloquearAceitaContaAdmin(t *testing.T) {
	u := usuarioDeTeste(t, "admin@myplanner.local", "gerente_projetos", "senha-certa")
	store := &mockSalarioLockStore{buscarPorEmailFn: func(context.Context, string) (*domain.Usuario, error) {
		return u, nil
	}}
	h := NewSalarioLockHandler(store, auth.NewTokenService("s", 24), "admin@myplanner.local", zap.NewNop())

	w := httptest.NewRecorder()
	h.Desbloquear(w, requestDestravar(`{"senha":"senha-certa"}`, u.Email, u.Cargo, time.Now().Add(time.Hour)))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, esperava 200 para a conta admin. Corpo: %s", w.Code, w.Body.String())
	}
}

func TestTravarDevolveTokenSemClaim(t *testing.T) {
	ts := auth.NewTokenService("s", 24)
	h := NewSalarioLockHandler(&mockSalarioLockStore{}, ts, "admin@myplanner.local", zap.NewNop())
	expiraEm := time.Now().Add(time.Hour).Truncate(time.Second)

	w := httptest.NewRecorder()
	h.Travar(w, requestDestravar("", "chefe@myplanner.local", "gerente", expiraEm))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, esperava 200. Corpo: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta inválida: %v", err)
	}
	claims, err := ts.ValidateToken(resp.Token)
	if err != nil {
		t.Fatalf("token devolvido não valida: %v", err)
	}
	if claims.Salarios {
		t.Error("travar devolveu token ainda destravado")
	}
	if !claims.ExpiresAt.Time.Equal(expiraEm) {
		t.Errorf("travar mexeu na expiração: %v, esperava %v", claims.ExpiresAt.Time, expiraEm)
	}
}
```

- [ ] **Step 2: Rodar os testes para ver falhar**

Run: `cd backend && go test ./internal/handler/ -run "Desbloquear|Travar" -v`
Expected: FAIL na compilação — `NewSalarioLockHandler` e `mw.ContextParaTeste` não existem.

- [ ] **Step 3: Expor um construtor de contexto para teste no middleware**

Os handlers leem identidade do contexto, que só o middleware sabe montar. Em `backend/internal/middleware/auth.go`, acrescentar:

```go
// ContextParaTeste monta o contexto que AuthJWT injetaria. Existe para os
// testes de handler não precisarem assinar um JWT só para exercitar a regra.
func ContextParaTeste(ctx context.Context, userID uuid.UUID, email, cargo string, expiraEm time.Time) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
	ctx = context.WithValue(ctx, userEmailKey, email)
	ctx = context.WithValue(ctx, userCargoKey, cargo)
	ctx = context.WithValue(ctx, tokenExpKey, expiraEm)
	return ctx
}
```

- [ ] **Step 4: Implementar o handler**

`backend/internal/handler/salario_lock.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/auth"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// cargosQuePodemVerSalario são os cargos com acesso a valores salariais. A
// conta admin passa independente do cargo.
var cargosQuePodemVerSalario = map[string]bool{
	"coordenador": true,
	"gerente":     true,
	"diretor":     true,
}

type SalarioLockStore interface {
	BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error)
}

type SalarioLockHandler struct {
	store        SalarioLockStore
	tokenService *auth.TokenService
	adminEmail   string
	logger       *zap.Logger
}

func NewSalarioLockHandler(store SalarioLockStore, tokenService *auth.TokenService, adminEmail string, logger *zap.Logger) *SalarioLockHandler {
	return &SalarioLockHandler{store: store, tokenService: tokenService, adminEmail: adminEmail, logger: logger}
}

// Desbloquear troca senha correta por um token com a claim de salários. O token
// novo herda a expiração do atual: destravar não renova a sessão.
func (h *SalarioLockHandler) Desbloquear(w http.ResponseWriter, r *http.Request) {
	email := middleware.UserEmailFromContext(r.Context())
	cargo := middleware.UserCargoFromContext(r.Context())

	if email != h.adminEmail && !cargosQuePodemVerSalario[cargo] {
		respondError(w, http.StatusForbidden, "seu cargo não permite ver valores salariais")
		return
	}

	var req struct {
		Senha string `json:"senha"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Senha == "" {
		respondError(w, http.StatusBadRequest, "informe a senha")
		return
	}

	usuario, err := h.store.BuscarPorEmail(r.Context(), email)
	if err != nil {
		h.logger.Error("failed to find usuario for salary unlock", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro interno")
		return
	}
	if usuario == nil || usuario.SenhaHash == nil {
		respondError(w, http.StatusUnauthorized, "senha incorreta")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*usuario.SenhaHash), []byte(req.Senha)); err != nil {
		respondError(w, http.StatusUnauthorized, "senha incorreta")
		return
	}

	h.responderComToken(w, usuario.ID, email, cargo, true, middleware.TokenExpiraEm(r.Context()))
}

// Travar devolve o token sem a claim. Não pede senha: fechar a cortina é sempre
// permitido.
func (h *SalarioLockHandler) Travar(w http.ResponseWriter, r *http.Request) {
	h.responderComToken(w,
		middleware.UserIDFromContext(r.Context()),
		middleware.UserEmailFromContext(r.Context()),
		middleware.UserCargoFromContext(r.Context()),
		false,
		middleware.TokenExpiraEm(r.Context()),
	)
}

func (h *SalarioLockHandler) responderComToken(w http.ResponseWriter, userID uuid.UUID, email, cargo string, salarios bool, expiraEm time.Time) {
	token, err := h.tokenService.GenerateTokenComExpiracao(userID, email, cargo, salarios, expiraEm)
	if err != nil {
		h.logger.Error("failed to generate salary token", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro ao gerar token")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"token": token})
}
```

Acrescentar `"github.com/google/uuid"` ao bloco de import.

As funções de leitura de contexto usadas acima existem com estes nomes exatos em
`backend/internal/middleware/auth.go`: `UserIDFromContext` (linha 57),
`UserEmailFromContext` (62) e `UserCargoFromContext` (67).

- [ ] **Step 5: Rodar os testes para ver passar**

Run: `cd backend && go test ./internal/handler/ -run "Desbloquear|Travar" -v`
Expected: PASS nos cinco testes.

- [ ] **Step 6: Ligar no main**

Em `backend/cmd/api/main.go`, junto dos outros handlers (perto de `authHandler := handler.NewAuthHandler(...)`, linha 87):

```go
	salarioLockHandler := handler.NewSalarioLockHandler(usuarioRepo, tokenService, cfg.Auth.AdminEmail, logger)
```

E no grupo de rotas autenticadas, junto das outras rotas de `/auth`:

```go
			r.Post("/auth/desbloquear-salarios", salarioLockHandler.Desbloquear)
			r.Post("/auth/travar-salarios", salarioLockHandler.Travar)
```

As duas precisam ficar **dentro** do grupo que usa `middleware.AuthJWT` — elas leem identidade do contexto.

- [ ] **Step 7: Conferir de ponta a ponta**

Run:
```bash
cd /home/emerson/code/myplanner && ./dev.sh restart
set -a; source .env; set +a
TOKEN=$(curl -s -X POST localhost:9091/api/v1/auth/login -H 'Content-Type: application/json' \
  -d "{\"email\":\"admin@myplanner.local\",\"senha\":\"$PASS_APP\"}" | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
echo "senha errada:"; curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:9091/api/v1/auth/desbloquear-salarios \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"senha":"errada"}'
echo "senha certa:"; curl -s -X POST localhost:9091/api/v1/auth/desbloquear-salarios \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d "{\"senha\":\"$PASS_APP\"}" \
  | python3 -c "import sys,json,base64;t=json.load(sys.stdin)['token'];p=t.split('.')[1];p+='='*(-len(p)%4);print(json.loads(base64.urlsafe_b64decode(p)))"
```
Expected: `401` para senha errada; para a senha certa, o payload impresso contém `'salarios': True`.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/handler/salario_lock.go backend/internal/handler/salario_lock_test.go backend/internal/middleware/auth.go backend/cmd/api/main.go
git commit -m "feat: rotas de desbloquear e travar valores salariais"
```

---

### Task 4: Campos de dinheiro viram ponteiro e somem quando travado (investimentos)

**Files:**
- Modify: `backend/internal/domain/investimento.go:9-14,35-40,55-58`
- Modify: `backend/internal/service/investimento.go:110,142`
- Modify: `backend/internal/handler/investimento.go` (`GetDashboard`, `GetGastosMensais`, `GetHistoricoSalario`)
- Create: `backend/internal/handler/investimento_lock_test.go`

**Interfaces:**
- Consumes: `middleware.PodeVerSalarios(ctx)` e `middleware.ContextParaTeste` (Tasks 2 e 3).
- Produces: `InvestimentoSumario.CustoMensalTotal`, `GastoMensal.CustoTotal` e `SalarioHistorico.Valor` como `*float64` com `omitempty` — consumidos pelo frontend na Task 7.

- [ ] **Step 1: Escrever os testes que falham**

`backend/internal/handler/investimento_lock_test.go`:

```go
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	mw "github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// São dois stores: InvestimentoStore (dashboard, gastos, alocações) e
// MembroFinanceiroStore (escrita e históricos). NewInvestimentoHandler recebe
// os dois, nesta ordem, mais o logger.
type mockInvestimentoStore struct {
	getDashboardFn     func(ctx context.Context, equipeID uuid.UUID) (*domain.InvestimentoDashboard, error)
	getGastosMensaisFn func(ctx context.Context, equipeID uuid.UUID, ano int) (*domain.GastosMensaisResponse, error)
}

func (m *mockInvestimentoStore) GetDashboard(ctx context.Context, equipeID uuid.UUID) (*domain.InvestimentoDashboard, error) {
	return m.getDashboardFn(ctx, equipeID)
}
func (m *mockInvestimentoStore) GetGastosMensais(ctx context.Context, equipeID uuid.UUID, ano int) (*domain.GastosMensaisResponse, error) {
	return m.getGastosMensaisFn(ctx, equipeID, ano)
}
func (m *mockInvestimentoStore) GetAlocacoesProjetos(context.Context, uuid.UUID) (*domain.AlocacoesProjetosResponse, error) {
	return nil, nil
}

type mockMembroFinanceiroStore struct {
	updateSalarioFn       func(ctx context.Context, id uuid.UUID, valor float64) error
	getHistoricoSalarioFn func(ctx context.Context, membroID uuid.UUID) ([]domain.SalarioHistorico, error)
}

func (m *mockMembroFinanceiroStore) UpdateSalario(ctx context.Context, id uuid.UUID, valor float64) error {
	return m.updateSalarioFn(ctx, id, valor)
}
func (m *mockMembroFinanceiroStore) UpdateBancoHoras(context.Context, uuid.UUID, float64) error {
	return nil
}
func (m *mockMembroFinanceiroStore) UpdateDataAdmissao(context.Context, uuid.UUID, *time.Time) error {
	return nil
}
func (m *mockMembroFinanceiroStore) GetHistoricoSalario(ctx context.Context, membroID uuid.UUID) ([]domain.SalarioHistorico, error) {
	return m.getHistoricoSalarioFn(ctx, membroID)
}
func (m *mockMembroFinanceiroStore) GetHistoricoBancoHoras(context.Context, uuid.UUID) ([]domain.BancoHorasHistorico, error) {
	return nil, nil
}

func requestComContexto(metodo, alvo, corpo, paramNome, paramValor string, destravado bool) *http.Request {
	req := httptest.NewRequest(metodo, alvo, strings.NewReader(corpo))
	ctx := mw.ContextParaTeste(req.Context(), uuid.New(), "chefe@myplanner.local", "gerente", time.Now().Add(time.Hour))
	if destravado {
		ctx = mw.ContextDestravadoParaTeste(ctx)
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(paramNome, paramValor)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	return req.WithContext(ctx)
}

func dashboardDeTeste() *domain.InvestimentoDashboard {
	custo := 12345.67
	salario := 5000.00
	return &domain.InvestimentoDashboard{
		Equipe:  domain.EquipeInfo{ID: uuid.New(), Nome: "Time"},
		Sumario: domain.InvestimentoSumario{CustoMensalTotal: &custo, TotalMembros: 3},
		Membros: []domain.MembroInvestimento{{ID: uuid.New(), Nome: "Fulano", Salario: &salario}},
	}
}

// O requisito do F12: travado, o número não pode existir no corpo da resposta.
func TestDashboardTravadoNaoMandaValores(t *testing.T) {
	store := &mockInvestimentoStore{getDashboardFn: func(context.Context, uuid.UUID) (*domain.InvestimentoDashboard, error) {
		return dashboardDeTeste(), nil
	}}
	h := NewInvestimentoHandler(store, &mockMembroFinanceiroStore{}, zap.NewNop())

	w := httptest.NewRecorder()
	h.GetDashboard(w, requestComContexto(http.MethodGet, "/equipes/x/investimentos", "", "id", uuid.NewString(), false))

	corpo := w.Body.String()
	for _, proibido := range []string{"12345.67", "5000", "custo_mensal_total", "\"salario\""} {
		if strings.Contains(corpo, proibido) {
			t.Errorf("corpo travado contém %q: %s", proibido, corpo)
		}
	}
	if !strings.Contains(corpo, "total_membros") {
		t.Errorf("travar não pode esvaziar o resto do dashboard: %s", corpo)
	}
}

func TestDashboardDestravadoMandaValores(t *testing.T) {
	store := &mockInvestimentoStore{getDashboardFn: func(context.Context, uuid.UUID) (*domain.InvestimentoDashboard, error) {
		return dashboardDeTeste(), nil
	}}
	h := NewInvestimentoHandler(store, &mockMembroFinanceiroStore{}, zap.NewNop())

	w := httptest.NewRecorder()
	h.GetDashboard(w, requestComContexto(http.MethodGet, "/equipes/x/investimentos", "", "id", uuid.NewString(), true))

	if !strings.Contains(w.Body.String(), "12345.67") {
		t.Errorf("destravado deveria mandar o custo: %s", w.Body.String())
	}
}

func TestGastosMensaisTravadoNaoMandaValores(t *testing.T) {
	custo := 9876.54
	store := &mockInvestimentoStore{getGastosMensaisFn: func(context.Context, uuid.UUID, int) (*domain.GastosMensaisResponse, error) {
		return &domain.GastosMensaisResponse{Ano: 2026, Meses: []domain.GastoMensal{{Mes: 1, CustoTotal: &custo}}}, nil
	}}
	h := NewInvestimentoHandler(store, &mockMembroFinanceiroStore{}, zap.NewNop())

	w := httptest.NewRecorder()
	h.GetGastosMensais(w, requestComContexto(http.MethodGet, "/equipes/x/investimentos/gastos-mensais", "", "id", uuid.NewString(), false))

	if strings.Contains(w.Body.String(), "9876.54") || strings.Contains(w.Body.String(), "custo_total") {
		t.Errorf("corpo travado contém o gasto: %s", w.Body.String())
	}
}

func TestHistoricoSalarioTravadoVemVazio(t *testing.T) {
	valor := 4321.00
	membroStore := &mockMembroFinanceiroStore{getHistoricoSalarioFn: func(context.Context, uuid.UUID) ([]domain.SalarioHistorico, error) {
		return []domain.SalarioHistorico{{ID: uuid.New(), Valor: &valor}}, nil
	}}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())

	w := httptest.NewRecorder()
	h.GetHistoricoSalario(w, requestComContexto(http.MethodGet, "/membros/x/salario/historico", "", "id", uuid.NewString(), false))

	if strings.Contains(w.Body.String(), "4321") {
		t.Errorf("histórico travado vazou valor: %s", w.Body.String())
	}
}
```

O mock acima implementa só os métodos que estes testes exercitam. Se `InvestimentoStore` tiver outros métodos, acrescentar stubs vazios ao mock — o compilador aponta quais faltam.

- [ ] **Step 2: Acrescentar o helper de contexto destravado**

Em `backend/internal/middleware/auth.go`, junto de `ContextParaTeste`:

```go
// ContextDestravadoParaTeste marca o contexto como podendo ver salários.
func ContextDestravadoParaTeste(ctx context.Context) context.Context {
	return context.WithValue(ctx, salariosKey, true)
}
```

- [ ] **Step 3: Rodar os testes para ver falhar**

Run: `cd backend && go test ./internal/handler/ -run "Travado|Destravado" -v`
Expected: FAIL na compilação — `CustoMensalTotal`, `CustoTotal` e `Valor` ainda são `float64`, não aceitam `&custo`.

- [ ] **Step 4: Trocar os campos para ponteiro**

Em `backend/internal/domain/investimento.go`:

```go
type SalarioHistorico struct {
	ID           uuid.UUID `json:"id"`
	MembroID     uuid.UUID `json:"membro_id"`
	Valor        *float64  `json:"valor,omitempty"`
	DataVigencia time.Time `json:"data_vigencia"`
	CreatedAt    time.Time `json:"created_at"`
}
```

```go
type InvestimentoSumario struct {
	// Ponteiro com omitempty: travado, a chave some do JSON. Zerar seria mentir
	// — "R$ 0,00" é um valor, e o frontend não distinguiria de custo real zero.
	CustoMensalTotal    *float64 `json:"custo_mensal_total,omitempty"`
	TotalMembros        int      `json:"total_membros"`
	TempoCasaMedioMeses int      `json:"tempo_casa_medio_meses"`
	BancoHorasTotal     float64  `json:"banco_horas_total"`
}
```

```go
type GastoMensal struct {
	Mes        int      `json:"mes"`
	CustoTotal *float64 `json:"custo_total,omitempty"`
}
```

Em `backend/internal/service/investimento.go`, ajustar as duas atribuições. Linha ~110:

```go
		custoTotalRef := custoTotal
		// ... dentro da construção do sumário:
			CustoMensalTotal:    &custoTotalRef,
```

Linha ~142, dentro do laço dos meses — a variável precisa ser nova a cada volta, senão todos os meses apontam para o mesmo número:

```go
			custoMesRef := math.Round(custoMes*100) / 100
			// ... na construção do GastoMensal:
				CustoTotal: &custoMesRef,
```

Rodar `cd backend && go build ./...` e corrigir os pontos que o compilador apontar em `repository/investimento.go` e onde mais esses campos forem lidos — o padrão é criar uma variável local e passar o endereço.

- [ ] **Step 5: Limpar os campos nos três handlers**

Em `backend/internal/handler/investimento.go`, dentro de `GetDashboard`, logo antes do `respondJSON`:

```go
	// Travado, o valor não sai do servidor: é o que impede de lê-lo no F12.
	if !middleware.PodeVerSalarios(r.Context()) {
		dashboard.Sumario.CustoMensalTotal = nil
		for i := range dashboard.Membros {
			dashboard.Membros[i].Salario = nil
		}
	}
```

Em `GetGastosMensais`:

```go
	if !middleware.PodeVerSalarios(r.Context()) {
		for i := range resp.Meses {
			resp.Meses[i].CustoTotal = nil
		}
	}
```

Em `GetHistoricoSalario`:

```go
	if !middleware.PodeVerSalarios(r.Context()) {
		historico = []domain.SalarioHistorico{}
	}
```

Ajustar os nomes das variáveis locais aos que já existem em cada handler. Acrescentar o import de `middleware` ao arquivo.

- [ ] **Step 6: Rodar os testes para ver passar**

Run: `cd backend && go test ./internal/handler/ -run "Travado|Destravado" -v`
Expected: PASS nos quatro testes.

- [ ] **Step 7: Rodar a suíte inteira**

Run: `cd backend && go build ./... && go test ./...`
Expected: build limpo e `ok` em todos os pacotes.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/domain/investimento.go backend/internal/service/investimento.go backend/internal/handler/investimento.go backend/internal/handler/investimento_lock_test.go backend/internal/middleware/auth.go
git commit -m "feat: valores de investimento somem do JSON quando travados"
```

---

### Task 5: Salário some das rotas de membro, equipe e import

**Files:**
- Modify: `backend/internal/handler/membro.go` (`List`, `GetByID`, `Search`)
- Modify: `backend/internal/handler/equipe.go:305-317` (`GetMembros`)
- Modify: `backend/internal/handler/import.go` (`Import`, `Sync`)
- Create: `backend/internal/handler/membro_lock_test.go`

**Interfaces:**
- Consumes: `middleware.PodeVerSalarios`, `middleware.ContextParaTeste`, `middleware.ContextDestravadoParaTeste` (Tasks 2 a 4).
- Produces: nada consumido por outra task do backend.

- [ ] **Step 1: Escrever os testes que falham**

`backend/internal/handler/membro_lock_test.go`:

```go
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	mw "github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func membroComSalario() domain.Membro {
	salario := 7777.77
	return domain.Membro{ID: uuid.New(), Nome: "Fulano", Salario: &salario}
}

func requestSimples(alvo string, destravado bool) *http.Request {
	req := httptest.NewRequest(http.MethodGet, alvo, nil)
	ctx := mw.ContextParaTeste(req.Context(), uuid.New(), "chefe@myplanner.local", "gerente", time.Now().Add(time.Hour))
	if destravado {
		ctx = mw.ContextDestravadoParaTeste(ctx)
	}
	return req.WithContext(ctx)
}

func TestListMembrosTravadoNaoMandaSalario(t *testing.T) {
	store := &mockMembroStore{listFn: func(context.Context) ([]domain.Membro, error) {
		return []domain.Membro{membroComSalario()}, nil
	}}
	h := NewMembroHandler(store, zap.NewNop())

	w := httptest.NewRecorder()
	h.List(w, requestSimples("/membros", false))

	if strings.Contains(w.Body.String(), "7777.77") || strings.Contains(w.Body.String(), "\"salario\"") {
		t.Errorf("lista travada vazou salário: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Fulano") {
		t.Errorf("travar não pode esvaziar a lista: %s", w.Body.String())
	}
}

func TestListMembrosDestravadoMandaSalario(t *testing.T) {
	store := &mockMembroStore{listFn: func(context.Context) ([]domain.Membro, error) {
		return []domain.Membro{membroComSalario()}, nil
	}}
	h := NewMembroHandler(store, zap.NewNop())

	w := httptest.NewRecorder()
	h.List(w, requestSimples("/membros", true))

	if !strings.Contains(w.Body.String(), "7777.77") {
		t.Errorf("destravado deveria mandar o salário: %s", w.Body.String())
	}
}

func TestSearchMembrosTravadoNaoMandaSalario(t *testing.T) {
	store := &mockMembroStore{searchFn: func(context.Context, string, bool) ([]domain.Membro, error) {
		return []domain.Membro{membroComSalario()}, nil
	}}
	h := NewMembroHandler(store, zap.NewNop())

	w := httptest.NewRecorder()
	h.Search(w, requestSimples("/membros/search?q=ful", false))

	if strings.Contains(w.Body.String(), "7777.77") {
		t.Errorf("busca travada vazou salário: %s", w.Body.String())
	}
}
```

O `mockMembroStore` já existe em `backend/internal/handler/membro_test.go`; acrescentar o campo `listFn func(ctx context.Context) ([]domain.Membro, error)` ao struct e fazer o método `List` chamá-lo quando não for nil (hoje ele devolve `nil, nil`).

- [ ] **Step 2: Rodar os testes para ver falhar**

Run: `cd backend && go test ./internal/handler/ -run "MembrosTravado|MembrosDestravado" -v`
Expected: FAIL — a lista travada contém `7777.77`.

- [ ] **Step 3: Limpar salário nos handlers de membro**

Em `backend/internal/handler/membro.go`, criar o helper e usá-lo nos três pontos:

```go
// limparSalarios zera o salário quando a requisição não pode ver dinheiro. O
// campo é *float64 com omitempty, então nil faz a chave sumir do JSON.
func limparSalarios(ctx context.Context, membros []domain.Membro) []domain.Membro {
	if middleware.PodeVerSalarios(ctx) {
		return membros
	}
	for i := range membros {
		membros[i].Salario = nil
	}
	return membros
}
```

- Em `List`: `respondJSON(w, http.StatusOK, limparSalarios(r.Context(), membros))`
- Em `Search`: idem.
- Em `GetByID`, antes do `respondJSON` que monta o mapa com `"membro"`:

```go
	if !middleware.PodeVerSalarios(r.Context()) {
		membro.Salario = nil
	}
```

Acrescentar o import de `middleware`.

- [ ] **Step 4: Limpar salário no handler de equipe**

Em `backend/internal/handler/equipe.go`, dentro de `GetMembros` (linha ~311), antes do `respondJSON`:

```go
	if !middleware.PodeVerSalarios(r.Context()) {
		for i := range membros {
			membros[i].Salario = nil
		}
	}
```

Se o tipo devolvido por `GetMembrosEquipe` não for `[]domain.Membro`, ajustar o laço ao campo de salário do tipo real — o compilador aponta.

- [ ] **Step 5: Limpar salário no preview do import**

Em `backend/internal/handler/import.go`, em `Import` e em `Sync`, antes de cada `respondJSON(w, http.StatusOK, result)`:

```go
	if !middleware.PodeVerSalarios(r.Context()) {
		limparSalariosDoPreview(result)
	}
```

E no mesmo arquivo:

```go
// limparSalariosDoPreview tira valor salarial do preview do import, inclusive o
// marcador "salario" em changes — ele sozinho já denunciaria que o valor mudou.
func limparSalariosDoPreview(result *domain.ImportMatchResult) {
	for i := range result.Matched {
		result.Matched[i].Dados.Salario = nil
		result.Matched[i].Changes = semSalario(result.Matched[i].Changes)
	}
	for i := range result.UnmatchedMembros {
		result.UnmatchedMembros[i].Dados.Salario = nil
	}
}

func semSalario(changes []string) []string {
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		if c != "salario" {
			out = append(out, c)
		}
	}
	return out
}
```

Conferir os nomes dos campos contra `backend/internal/domain/import.go` antes de compilar.

- [ ] **Step 6: Rodar os testes para ver passar**

Run: `cd backend && go test ./internal/handler/ -v 2>&1 | tail -20`
Expected: PASS em todos, incluindo os testes de import que já existiam.

- [ ] **Step 7: Conferir contra o servidor**

Run:
```bash
cd /home/emerson/code/myplanner && ./dev.sh restart
set -a; source .env; set +a
TOKEN=$(curl -s -X POST localhost:9091/api/v1/auth/login -H 'Content-Type: application/json' \
  -d "{\"email\":\"admin@myplanner.local\",\"senha\":\"$PASS_APP\"}" | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
echo "travado:"; curl -s "localhost:9091/api/v1/membros?limit=1" -H "Authorization: Bearer $TOKEN" | grep -c salario
DESTRAVADO=$(curl -s -X POST localhost:9091/api/v1/auth/desbloquear-salarios -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d "{\"senha\":\"$PASS_APP\"}" | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
echo "destravado:"; curl -s "localhost:9091/api/v1/membros?limit=1" -H "Authorization: Bearer $DESTRAVADO" | grep -c salario
```
Expected: `0` travado, número maior que zero destravado.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/handler/membro.go backend/internal/handler/equipe.go backend/internal/handler/import.go backend/internal/handler/membro_lock_test.go backend/internal/handler/membro_test.go
git commit -m "feat: salário some das rotas de membro, equipe e preview do import"
```

---

### Task 6: Escrita de salário exige desbloqueio

**Files:**
- Modify: `backend/internal/handler/investimento.go` (`UpdateSalario`)
- Modify: `backend/internal/handler/equipe.go` (`MeritoPromocao`)
- Modify: `backend/internal/handler/import.go` (`Confirmar`)
- Create: `backend/internal/handler/salario_escrita_test.go`

**Interfaces:**
- Consumes: `middleware.PodeVerSalarios`, helpers de contexto de teste (Tasks 2 a 4).
- Produces: nada consumido por outra task.

- [ ] **Step 1: Escrever os testes que falham**

`backend/internal/handler/salario_escrita_test.go`:

```go
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	mw "github.com/emersonpaula83/myplanner/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func requestEscrita(metodo, alvo, corpo, paramValor string) *http.Request {
	req := httptest.NewRequest(metodo, alvo, strings.NewReader(corpo))
	ctx := mw.ContextParaTeste(req.Context(), uuid.New(), "chefe@myplanner.local", "gerente", time.Now().Add(time.Hour))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", paramValor)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	return req.WithContext(ctx)
}

// Travar só a leitura seria teatro: quem abre o F12 monta o PUT na mão.
func TestUpdateSalarioTravadoDa403(t *testing.T) {
	chamou := false
	membroStore := &mockMembroFinanceiroStore{updateSalarioFn: func(context.Context, uuid.UUID, float64) error {
		chamou = true
		return nil
	}}
	h := NewInvestimentoHandler(&mockInvestimentoStore{}, membroStore, zap.NewNop())

	w := httptest.NewRecorder()
	h.UpdateSalario(w, requestEscrita(http.MethodPut, "/membros/x/salario", `{"salario":9000}`, uuid.NewString()))

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, esperava 403", w.Code)
	}
	if chamou {
		t.Error("store foi chamado com a cortina fechada")
	}
}

func TestImportConfirmarTravadoDa403(t *testing.T) {
	chamou := false
	store := &mockImportStore{confirmImportFn: func(context.Context, domain.ConfirmImportRequest) (*domain.ConfirmImportResponse, error) {
		chamou = true
		return &domain.ConfirmImportResponse{}, nil
	}}
	h := newTestImportHandler(store)

	w := httptest.NewRecorder()
	h.Confirmar(w, requestEscrita(http.MethodPost, "/investimentos/import/confirmar", `{"linhas":[{"linha":2}]}`, ""))

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, esperava 403", w.Code)
	}
	if chamou {
		t.Error("import confirmou com a cortina fechada")
	}
}
```

`mockMembroFinanceiroStore` já foi criado na Task 4 com `updateSalarioFn` — este teste só o preenche.

- [ ] **Step 2: Rodar os testes para ver falhar**

Run: `cd backend && go test ./internal/handler/ -run "TravadoDa403" -v`
Expected: FAIL — as rotas respondem 200 e o store é chamado.

- [ ] **Step 3: Recusar escrita sem a claim**

No começo de `UpdateSalario` (`handler/investimento.go`), de `MeritoPromocao` (`handler/equipe.go`) e de `Confirmar` (`handler/import.go`), antes de qualquer leitura de corpo:

```go
	// Alterar salário sem poder vê-lo seria alterar às cegas — e seria o
	// caminho aberto para quem monta a requisição na mão.
	if !middleware.PodeVerSalarios(r.Context()) {
		respondError(w, http.StatusForbidden, "destrave os valores salariais para alterar salário")
		return
	}
```

Acrescentar o import de `middleware` onde faltar.

- [ ] **Step 4: Rodar os testes para ver passar**

Run: `cd backend && go test ./internal/handler/ -v 2>&1 | tail -15`
Expected: PASS em todos.

- [ ] **Step 5: Rodar a suíte inteira**

Run: `cd backend && go build ./... && go test ./...`
Expected: build limpo, todos `ok`.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handler/investimento.go backend/internal/handler/equipe.go backend/internal/handler/import.go backend/internal/handler/salario_escrita_test.go
git commit -m "feat: alterar salário exige cortina aberta"
```

---

### Task 7: Cortina no frontend — máscara, modal e cadeado

**Files:**
- Modify: `frontend/index.html` — CSS junto das regras de investimentos; markup do modal junto de `#merito-modal`; funções novas perto de `formatSalarioBR` (linha ~8292); pontos de exibição nas linhas ~2681, ~2751-2752, ~8357, ~8566, ~8985-8986, ~9028-9030

**Interfaces:**
- Consumes: `POST /auth/desbloquear-salarios`, `POST /auth/travar-salarios` (Task 3); campos ausentes quando travado (Tasks 4 e 5).
- Produces: nada.

- [ ] **Step 1: Adicionar o markup do modal**

Depois do bloco `<div class="modal-overlay" id="merito-modal" …>…</div>`:

```html
<div class="modal-overlay" id="salario-modal" onclick="if(event.target===this)closeSalarioModal()">
  <div class="modal" style="max-width:400px">
    <div class="modal-title">🔒 Valores salariais</div>
    <div style="font-size:13px;color:var(--text-secondary);margin-bottom:12px">Digite sua senha para ver os valores nesta sessão.</div>
    <input class="form-input" type="password" id="salario-senha" placeholder="Sua senha" onkeydown="if(event.key==='Enter')confirmarSenhaSalario()" />
    <div id="salario-erro" style="display:none;color:var(--red);font-size:12px;margin-top:8px"></div>
    <div class="modal-actions">
      <button class="btn-cancel" type="button" onclick="closeSalarioModal()">Cancelar</button>
      <button class="btn-add" type="button" id="salario-confirmar" onclick="confirmarSenhaSalario()">Destravar</button>
    </div>
  </div>
</div>
```

- [ ] **Step 2: Adicionar o CSS**

Junto das regras de investimentos no `<style>`:

```css
.valor-travado { letter-spacing: 2px; color: var(--text-tertiary); }
.btn-cadeado { background: none; border: none; cursor: pointer; font-size: 14px; padding: 0 4px; line-height: 1; }
.btn-cadeado:hover { opacity: .75; }
```

- [ ] **Step 3: Implementar máscara e estado**

Junto de `formatSalarioBR` (linha ~8292):

```javascript
// === CORTINA DE SALÁRIOS ===
// O desbloqueio vive numa claim do token; enquanto travado o backend nem manda
// o número, então aqui não existe valor escondido para o F12 achar.
function salariosDesbloqueados() {
  if (!token) return false;
  try {
    var payload = JSON.parse(atob(token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')));
    return payload.salarios === true;
  } catch (e) { return false; }
}

// valorSalario é o único lugar que decide entre mostrar e mascarar.
function valorSalario(v, vazio) {
  if (!salariosDesbloqueados()) {
    return '<span class="valor-travado">•••••</span>' +
      '<button class="btn-cadeado" type="button" onclick="event.stopPropagation();openSalarioModal()" title="Destravar valores">🔒</button>';
  }
  if (v == null) return vazio || '—';
  return formatSalarioBR(v);
}

function botaoCortina() {
  return salariosDesbloqueados()
    ? '<button class="btn-cadeado" type="button" onclick="travarSalarios()" title="Esconder valores">🔓</button>'
    : '<button class="btn-cadeado" type="button" onclick="openSalarioModal()" title="Destravar valores">🔒</button>';
}

function openSalarioModal() {
  document.getElementById('salario-erro').style.display = 'none';
  document.getElementById('salario-senha').value = '';
  document.getElementById('salario-modal').classList.add('open');
  document.getElementById('salario-senha').focus();
}

function closeSalarioModal() {
  document.getElementById('salario-modal').classList.remove('open');
}

async function confirmarSenhaSalario() {
  var senha = document.getElementById('salario-senha').value;
  var erroEl = document.getElementById('salario-erro');
  var btn = document.getElementById('salario-confirmar');
  if (!senha) { erroEl.textContent = 'Informe sua senha'; erroEl.style.display = 'block'; return; }

  btn.disabled = true;
  try {
    var resp = await api('/auth/desbloquear-salarios', { method: 'POST', body: JSON.stringify({ senha: senha }) });
    token = resp.token;
    localStorage.setItem('myplanner_token', token);
    closeSalarioModal();
    recarregarTelaAtual();
  } catch (e) {
    erroEl.textContent = e.message || 'Não foi possível destravar';
    erroEl.style.display = 'block';
  } finally {
    btn.disabled = false;
  }
}

async function travarSalarios() {
  try {
    var resp = await api('/auth/travar-salarios', { method: 'POST' });
    token = resp.token;
    localStorage.setItem('myplanner_token', token);
    recarregarTelaAtual();
  } catch (e) { alert('Erro ao travar: ' + e.message); }
}

// Recarrega a tela aberta para os valores virem do servidor com o token novo.
function recarregarTelaAtual() {
  if (typeof invCurrentEquipeId !== 'undefined' && invCurrentEquipeId) {
    loadInvestimentos(invCurrentEquipeId);
    return;
  }
  if (typeof currentMembroId !== 'undefined' && currentMembroId) {
    loadMembroDetail(currentMembroId, currentMembroPeriod);
    return;
  }
  if (typeof importMatchResult !== 'undefined' && importMatchResult) {
    refreshImportPreview();
  }
}
```

Conferir o nome da variável do membro aberto:

Run: `grep -n "currentMembroId\|function loadMembroDetail" frontend/index.html | head -5`

Se o nome for outro, ajustar `recarregarTelaAtual` ao nome real.

- [ ] **Step 4: Trocar os pontos de exibição pela máscara**

Substituir, um a um:

- Linha ~8357 (investimento/mês): `formatSalarioBR(sum.custo_mensal_total)` → `valorSalario(sum.custo_mensal_total)`
- Linha ~8566 (salário do membro no card): a expressão `? formatSalarioBR(m.salario)` → `? valorSalario(m.salario)`
- Linha ~2681 (salário atual no modal de mérito): `(s.salario != null ? formatSalarioBR(s.salario) : 'Não definido')` → `valorSalario(s.salario, 'Não definido')`
- Linhas ~2751-2752 (resultado do mérito): `formatSalarioBR(a.salario)` e `formatSalarioBR(d.salario)` → `valorSalario(...)` com o mesmo segundo argumento `'—'`
- Linhas ~8985-8986 e ~9028-9030 (preview do import): trocar as chamadas a `formatSalarioBR` por `valorSalario`, mantendo o texto de fallback já usado ali

No gráfico de gastos mensais (`formatTooltip` nas linhas ~8541 e ~8679), trocar por:

```javascript
      formatTooltip: function(v) { return salariosDesbloqueados() ? formatSalarioBR(v) : '•••••'; }
```

- [ ] **Step 5: Colocar o botão de cortina no cabeçalho de Investimentos**

No cabeçalho da tela de investimentos, junto do título, acrescentar `botaoCortina()` na string de HTML. Localizar o ponto:

Run: `grep -n "inv-stat-card" frontend/index.html | head -3`

O botão entra imediatamente antes do primeiro `inv-stat-card`, dentro do mesmo contêiner do cabeçalho.

- [ ] **Step 6: Travar as duas telas de escrita**

Em `openMeritoModal(membroId)`, como primeira linha do corpo:

```javascript
  if (!salariosDesbloqueados()) { openSalarioModal(); return; }
```

No `renderImportPreviewTable`, o botão de confirmar (`#import-confirm-btn`) fica desabilitado quando travado. Logo depois de montar a tabela, acrescentar:

```javascript
  if (!salariosDesbloqueados()) {
    html += '<div class="fusao-aviso" style="margin-top:12px">Destrave os valores para revisar os salários antes de importar. ' +
      botaoCortina() + '</div>';
  }
```

E em `confirmImportSubmit`, como primeira linha:

```javascript
  if (!salariosDesbloqueados()) { openSalarioModal(); return; }
```

- [ ] **Step 7: Conferir a sintaxe do JS**

Run:
```bash
cd /home/emerson/code/myplanner && python3 - <<'EOF'
import re
html = open('frontend/index.html').read()
blocos = re.findall(r'<script(?![^>]*src=)[^>]*>(.*?)</script>', html, re.S)
open('/tmp/cortina-check.js','w').write('\n;\n'.join(blocos))
EOF
node --check /tmp/cortina-check.js && echo "JS OK"
```
Expected: `JS OK`.

- [ ] **Step 8: Conferir no navegador**

O frontend é servido do disco; basta recarregar `http://localhost:9091`.

1. Entrar em Investimentos: os valores aparecem como `••••• 🔒` e o gráfico de gastos mostra `•••••` no tooltip.
2. Abrir o F12 na aba Network, recarregar a página e conferir a resposta de `/equipes/{id}/investimentos`: não existe `custo_mensal_total` nem `salario` no JSON.
3. Clicar no cadeado, digitar uma senha errada — a mensagem "Senha incorreta" aparece inline.
4. Digitar a senha certa: os valores aparecem, e agora o JSON da mesma rota traz os campos.
5. Clicar no 🔓 do cabeçalho: volta a mascarar sem deslogar.
6. Com a cortina fechada, clicar em ⭐ num membro: abre o modal de senha, não o de mérito.

- [ ] **Step 9: Commit**

```bash
git add frontend/index.html
git commit -m "feat: cortina de salários no frontend com cadeado e modal de senha"
```

---

## Notas de desvio da spec

- A spec fala em `GenerateToken` recebendo o flag. O plano acrescenta
  `GenerateTokenComExpiracao` e deixa `GenerateToken` intacto: preservar a
  expiração restante exige passar a data explicitamente, e assim os dois
  chamadores existentes (`handler/auth.go:85` e `handler/saml_auth.go:97`) não
  mudam.
- A spec não menciona helpers de contexto para teste. O plano acrescenta
  `middleware.ContextParaTeste` e `middleware.ContextDestravadoParaTeste`, sem os
  quais cada teste de handler precisaria assinar um JWT só para exercitar a regra.
