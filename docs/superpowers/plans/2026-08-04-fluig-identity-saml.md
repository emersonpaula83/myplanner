# Fluig Identity SAML SSO — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace email/password login with SAML 2.0 SSO via Fluig Identity, add equipe-based access control (alçada), and implement daily admin password rotation stored in K8s Secrets.

**Architecture:** SAML SP implemented with `crewjam/saml`. Frontend redirects unauthenticated users to IdP. After SAML assertion, backend generates JWT (same as today). Equipe filter middleware replaces current ProjetoFilter — injects allowed equipe_ids into context. Admin password rotated daily via goroutine, stored in K8s Secret (prod) or stdout (dev).

**Tech Stack:** Go 1.25, `crewjam/saml`, `k8s.io/client-go`, pgx, chi, JWT, bcrypt, Vanilla JS frontend

## Global Constraints

- Module path: `github.com/emersonpaula83/myplanner/backend`
- Migration numbering: sequential from 000025
- Admin email fixed: `admin@myplanner.local`
- Frontend: single `frontend/index.html` file (SPA)
- Auth: JWT stateless, 24h expiration
- All new SAML env vars required at startup (fail fast if missing)
- `auth_provider` values: `local` or `saml`

---

### Task 1: Database Migration — auth_provider, senha nullable, usuario_equipes

**Files:**
- Create: `backend/migrations/000025_iam_saml_alcada.up.sql`
- Create: `backend/migrations/000025_iam_saml_alcada.down.sql`

**Interfaces:**
- Produces: `auth_provider` column on `usuarios`, nullable `senha_hash`, `usuario_equipes` table

- [ ] **Step 1: Write up migration**

```sql
-- Allow SAML users without password
ALTER TABLE usuarios ALTER COLUMN senha_hash DROP NOT NULL;

-- Track auth provider per user
ALTER TABLE usuarios ADD COLUMN auth_provider VARCHAR(20) NOT NULL DEFAULT 'local';

-- Equipe-based access control (replaces projeto-based alçada)
CREATE TABLE IF NOT EXISTS usuario_equipes (
    usuario_id  UUID NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    equipe_id   UUID NOT NULL REFERENCES equipes(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (usuario_id, equipe_id)
);

CREATE INDEX IF NOT EXISTS idx_usuario_equipes_usuario ON usuario_equipes(usuario_id);
```

- [ ] **Step 2: Write down migration**

```sql
DROP TABLE IF EXISTS usuario_equipes;
ALTER TABLE usuarios DROP COLUMN IF EXISTS auth_provider;
ALTER TABLE usuarios ALTER COLUMN senha_hash SET NOT NULL;
```

- [ ] **Step 3: Run migration**

```bash
cd backend && go run ./cmd/migrate -direction up
```

Expected: migration 000025 applied successfully.

- [ ] **Step 4: Verify schema**

```bash
docker compose exec -T db psql -U myplanner -d myplanner -c "\d usuarios" | grep -E "auth_provider|senha_hash"
docker compose exec -T db psql -U myplanner -d myplanner -c "\d usuario_equipes"
```

Expected: `senha_hash` nullable, `auth_provider` varchar(20) with default 'local', `usuario_equipes` table exists.

- [ ] **Step 5: Commit**

```bash
git add backend/migrations/000025_iam_saml_alcada.up.sql backend/migrations/000025_iam_saml_alcada.down.sql
git commit -m "feat: add migration for SAML auth_provider, nullable senha_hash, usuario_equipes"
```

---

### Task 2: Domain & Config — SAMLConfig, Usuario changes, AlçadaEquipesRequest

**Files:**
- Modify: `backend/internal/config/config.go` — add `SAMLConfig` struct and loading
- Modify: `backend/internal/domain/usuario.go` — make `SenhaHash` pointer, add `AuthProvider`, add `AlcadaEquipesRequest`

**Interfaces:**
- Consumes: migration from Task 1 (auth_provider column)
- Produces:
  - `config.SAMLConfig{IDPMetadataURL, EntityID, ACSURL, CertFile, KeyFile, FrontendURL string}`
  - `config.AdminSecretConfig{SecretName, SecretNamespace string}`
  - `domain.Usuario.SenhaHash` becomes `*string`
  - `domain.Usuario.AuthProvider` field (string)
  - `domain.AlcadaEquipesRequest{EquipeIDs []uuid.UUID}`

- [ ] **Step 1: Add SAMLConfig and AdminSecretConfig to config.go**

Add structs after `GeminiConfig`:

```go
type SAMLConfig struct {
	IDPMetadataURL string
	EntityID       string
	ACSURL         string
	CertFile       string
	KeyFile        string
	FrontendURL    string
}

type AdminSecretConfig struct {
	SecretName      string
	SecretNamespace string
}
```

Add fields to `Config` struct:

```go
SAML        SAMLConfig
AdminSecret AdminSecretConfig
```

Add defaults in `Load()`:

```go
viper.SetDefault("ADMIN_SECRET_NAME", "myplanner-admin-password")
viper.SetDefault("ADMIN_SECRET_NAMESPACE", "")
```

Add loading in `Load()` return:

```go
SAML: SAMLConfig{
    IDPMetadataURL: viper.GetString("SAML_IDP_METADATA_URL"),
    EntityID:       viper.GetString("SAML_ENTITY_ID"),
    ACSURL:         viper.GetString("SAML_ACS_URL"),
    CertFile:       viper.GetString("SAML_CERT_FILE"),
    KeyFile:        viper.GetString("SAML_KEY_FILE"),
    FrontendURL:    viper.GetString("SAML_FRONTEND_URL"),
},
AdminSecret: AdminSecretConfig{
    SecretName:      viper.GetString("ADMIN_SECRET_NAME"),
    SecretNamespace: viper.GetString("ADMIN_SECRET_NAMESPACE"),
},
```

- [ ] **Step 2: Update domain/usuario.go**

Change `SenhaHash` type from `string` to `*string`:

```go
SenhaHash    *string   `json:"-" db:"senha_hash"`
```

Add `AuthProvider` field:

```go
AuthProvider string    `json:"auth_provider" db:"auth_provider"`
```

Add new request type:

```go
type AlcadaEquipesRequest struct {
	EquipeIDs []uuid.UUID `json:"equipe_ids"`
}
```

- [ ] **Step 3: Fix all compilation errors from SenhaHash type change**

In `handler/auth.go` Login(), change bcrypt compare from:
```go
bcrypt.CompareHashAndPassword([]byte(usuario.SenhaHash), []byte(req.Senha))
```
to:
```go
if usuario.SenhaHash == nil {
    respondError(w, http.StatusUnauthorized, "credenciais inválidas")
    return
}
bcrypt.CompareHashAndPassword([]byte(*usuario.SenhaHash), []byte(req.Senha))
```

In `handler/usuario.go` AlterarSenha(), change:
```go
bcrypt.CompareHashAndPassword([]byte(usuario.SenhaHash), []byte(req.SenhaAtual))
```
to:
```go
if usuario.SenhaHash == nil {
    respondError(w, http.StatusUnauthorized, "usuário sem senha local")
    return
}
bcrypt.CompareHashAndPassword([]byte(*usuario.SenhaHash), []byte(req.SenhaAtual))
```

In `repository/usuario.go`, update all `scanUsuario`/`scanUsuarioRows` and SELECT queries to include `auth_provider`:

```go
SELECT id, nome_completo, apelido, email, senha_hash, cargo, ativo, auth_provider, created_at, updated_at
```

Update scan functions:
```go
func scanUsuario(row pgx.Row) (domain.Usuario, error) {
    var u domain.Usuario
    err := row.Scan(
        &u.ID, &u.NomeCompleto, &u.Apelido, &u.Email, &u.SenhaHash,
        &u.Cargo, &u.Ativo, &u.AuthProvider, &u.CreatedAt, &u.UpdatedAt,
    )
    return u, err
}
```

Same for `scanUsuarioRows`.

- [ ] **Step 4: Build to verify**

```bash
cd backend && go build ./...
```

Expected: compiles cleanly.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/config/config.go backend/internal/domain/usuario.go backend/internal/handler/auth.go backend/internal/handler/usuario.go backend/internal/repository/usuario.go
git commit -m "feat: add SAMLConfig, AdminSecretConfig, auth_provider field, nullable senha_hash"
```

---

### Task 3: Repository — BuscarOuCriarPorEmail, Alçada Equipes CRUD

**Files:**
- Modify: `backend/internal/repository/usuario.go` — add `BuscarOuCriarPorEmail()`, equipe alçada methods

**Interfaces:**
- Consumes: `domain.Usuario` with `AuthProvider` and `*string SenhaHash` from Task 2
- Produces:
  - `BuscarOuCriarPorEmail(ctx, email, nomeCompleto, authProvider string) (*domain.Usuario, error)` — returns existing user or creates new
  - `ListarEquipesPorUsuario(ctx, usuarioID uuid.UUID) ([]EquipeResumo, error)`
  - `AtualizarEquipes(ctx, usuarioID uuid.UUID, equipeIDs []uuid.UUID) error`
  - `BuscarEquipeIDsPorUsuario(ctx, usuarioID uuid.UUID) ([]uuid.UUID, error)`
  - `EquipeResumo{ID uuid.UUID, Nome string}`

- [ ] **Step 1: Write test for BuscarOuCriarPorEmail**

Create `backend/internal/repository/usuario_saml_test.go`:

```go
package repository

import (
    "testing"
)

func TestBuscarOuCriarPorEmail_NewUser(t *testing.T) {
    // Integration test — requires DB. Skip if no DB.
    // Validates: creates user with auth_provider='saml', senha_hash=NULL
    t.Skip("integration test — run with DB")
}
```

- [ ] **Step 2: Add BuscarOuCriarPorEmail to usuario.go**

```go
func (r *UsuarioRepository) BuscarOuCriarPorEmail(ctx context.Context, email, nomeCompleto, authProvider string) (*domain.Usuario, error) {
	u, err := r.BuscarPorEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if u != nil {
		return u, nil
	}

	row := r.pool.QueryRow(ctx, `
		INSERT INTO usuarios (id, nome_completo, apelido, email, senha_hash, cargo, auth_provider)
		VALUES ($1, $2, $3, $4, NULL, 'gerente', $5)
		RETURNING id, nome_completo, apelido, email, senha_hash, cargo, ativo, auth_provider, created_at, updated_at
	`, uuid.New(), nomeCompleto, nomeCompleto, email, authProvider)

	newUser, err := scanUsuario(row)
	if err != nil {
		return nil, fmt.Errorf("creating usuario via SAML: %w", err)
	}
	return &newUser, nil
}
```

- [ ] **Step 3: Add equipe alçada methods**

```go
type EquipeResumo struct {
	ID   uuid.UUID `json:"id"`
	Nome string    `json:"nome"`
}

func (r *UsuarioRepository) BuscarEquipeIDsPorUsuario(ctx context.Context, usuarioID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT equipe_id FROM usuario_equipes WHERE usuario_id = $1
	`, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("querying equipe_ids for usuario %s: %w", usuarioID, err)
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning equipe_id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *UsuarioRepository) ListarEquipesPorUsuario(ctx context.Context, usuarioID uuid.UUID) ([]EquipeResumo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.nome
		FROM equipes e
		INNER JOIN usuario_equipes ue ON ue.equipe_id = e.id
		WHERE ue.usuario_id = $1
		ORDER BY e.nome
	`, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("querying equipes for usuario %s: %w", usuarioID, err)
	}
	defer rows.Close()

	result := make([]EquipeResumo, 0)
	for rows.Next() {
		var e EquipeResumo
		if err := rows.Scan(&e.ID, &e.Nome); err != nil {
			return nil, fmt.Errorf("scanning equipe resumo: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (r *UsuarioRepository) AtualizarEquipes(ctx context.Context, usuarioID uuid.UUID, equipeIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `DELETE FROM usuario_equipes WHERE usuario_id = $1`, usuarioID)
	if err != nil {
		return fmt.Errorf("deleting existing equipes: %w", err)
	}

	for _, eid := range equipeIDs {
		_, err = tx.Exec(ctx, `
			INSERT INTO usuario_equipes (usuario_id, equipe_id) VALUES ($1, $2)
		`, usuarioID, eid)
		if err != nil {
			return fmt.Errorf("inserting equipe %s: %w", eid, err)
		}
	}

	return tx.Commit(ctx)
}
```

- [ ] **Step 4: Build to verify**

```bash
cd backend && go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add backend/internal/repository/usuario.go backend/internal/repository/usuario_saml_test.go
git commit -m "feat: add BuscarOuCriarPorEmail and equipe alçada repository methods"
```

---

### Task 4: SAML Provider — `internal/saml/provider.go`

**Files:**
- Create: `backend/internal/saml/provider.go`

**Interfaces:**
- Consumes: `config.SAMLConfig` from Task 2
- Produces:
  - `SAMLProvider` struct
  - `NewSAMLProvider(cfg config.SAMLConfig) (*SAMLProvider, error)` — loads IdP metadata, configures SP
  - `GetServiceProvider() *saml.ServiceProvider` — returns underlying SP for middleware/handlers
  - `ExtractEmailFromAssertion(assertion *saml.Assertion) (email string, nome string, err error)`

- [ ] **Step 1: Add crewjam/saml dependency**

```bash
cd backend && go get github.com/crewjam/saml
```

- [ ] **Step 2: Write provider.go**

```go
package saml

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	"github.com/emersonpaula83/myplanner/backend/internal/config"
)

type SAMLProvider struct {
	sp *saml.ServiceProvider
}

func NewSAMLProvider(cfg config.SAMLConfig) (*SAMLProvider, error) {
	keyPair, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading SP cert/key: %w", err)
	}

	keyPair.Leaf, err = x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parsing SP certificate: %w", err)
	}

	idpMetadataURL, err := url.Parse(cfg.IDPMetadataURL)
	if err != nil {
		return nil, fmt.Errorf("parsing IDP metadata URL: %w", err)
	}

	idpMetadata, err := samlsp.FetchMetadata(
		http.DefaultClient,
		*idpMetadataURL,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching IDP metadata: %w", err)
	}

	rootURL, err := url.Parse(cfg.ACSURL)
	if err != nil {
		return nil, fmt.Errorf("parsing ACS URL: %w", err)
	}
	// ACS URL is the full path; root is scheme+host
	rootURL.Path = ""
	rootURL.RawQuery = ""

	acsURL, err := url.Parse(cfg.ACSURL)
	if err != nil {
		return nil, fmt.Errorf("parsing ACS URL: %w", err)
	}

	sp := saml.ServiceProvider{
		EntityID:          cfg.EntityID,
		Key:               keyPair.PrivateKey.(*rsa.PrivateKey),
		Certificate:       keyPair.Leaf,
		IDPMetadata:       idpMetadata,
		AcsURL:            *acsURL,
		MetadataURL:       *rootURL,
	}

	return &SAMLProvider{sp: &sp}, nil
}

func (p *SAMLProvider) GetServiceProvider() *saml.ServiceProvider {
	return p.sp
}

func ExtractEmailFromAssertion(assertion *saml.Assertion) (email string, nome string, err error) {
	if assertion == nil || assertion.Subject == nil || assertion.Subject.NameID == nil {
		return "", "", fmt.Errorf("assertion missing NameID")
	}

	email = assertion.Subject.NameID.Value
	if email == "" {
		return "", "", fmt.Errorf("NameID value is empty")
	}

	// Try to get display name from attributes
	for _, stmt := range assertion.AttributeStatements {
		for _, attr := range stmt.Attributes {
			if attr.Name == "displayName" || attr.Name == "cn" || attr.Name == "urn:oid:2.16.840.1.113730.3.1.241" {
				if len(attr.Values) > 0 {
					nome = attr.Values[0].Value
				}
			}
		}
	}

	if nome == "" {
		nome = email
	}

	return email, nome, nil
}
```

- [ ] **Step 3: Build to verify**

```bash
cd backend && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add backend/internal/saml/
git commit -m "feat: add SAML 2.0 Service Provider with crewjam/saml"
```

---

### Task 5: SAML Auth Handler — `internal/handler/saml_auth.go`

**Files:**
- Create: `backend/internal/handler/saml_auth.go`

**Interfaces:**
- Consumes:
  - `saml.SAMLProvider.GetServiceProvider()` from Task 4
  - `saml.ExtractEmailFromAssertion()` from Task 4
  - `UsuarioStore.BuscarOuCriarPorEmail()` (extended interface) from Task 3
  - `auth.TokenService.GenerateToken()` — existing
  - `config.SAMLConfig.FrontendURL` from Task 2
- Produces:
  - `SAMLAuthHandler` struct
  - `NewSAMLAuthHandler(sp *saml.SAMLProvider, store SAMLUserStore, tokenService *auth.TokenService, frontendURL string, logger *zap.Logger) *SAMLAuthHandler`
  - `Login(w, r)` — GET, initiates SAML AuthnRequest redirect
  - `ACS(w, r)` — POST, processes SAML response, creates/finds user, generates JWT, redirects
  - `Metadata(w, r)` — GET, serves SP metadata XML

- [ ] **Step 1: Write saml_auth.go**

```go
package handler

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"

	"github.com/crewjam/saml"
	"github.com/emersonpaula83/myplanner/backend/internal/auth"
	samlpkg "github.com/emersonpaula83/myplanner/backend/internal/saml"
	"go.uber.org/zap"
)

type SAMLUserStore interface {
	BuscarOuCriarPorEmail(ctx context.Context, email, nomeCompleto, authProvider string) (*domain.Usuario, error)
}

type SAMLAuthHandler struct {
	provider     *samlpkg.SAMLProvider
	store        SAMLUserStore
	tokenService *auth.TokenService
	frontendURL  string
	logger       *zap.Logger
}

func NewSAMLAuthHandler(provider *samlpkg.SAMLProvider, store SAMLUserStore, tokenService *auth.TokenService, frontendURL string, logger *zap.Logger) *SAMLAuthHandler {
	return &SAMLAuthHandler{
		provider:     provider,
		store:        store,
		tokenService: tokenService,
		frontendURL:  frontendURL,
		logger:       logger,
	}
}

func (h *SAMLAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	sp := h.provider.GetServiceProvider()
	authnReq, err := sp.MakeAuthenticationRequest(sp.GetSSOBindingLocation(saml.HTTPRedirectBinding), saml.HTTPRedirectBinding, saml.HTTPPostBinding)
	if err != nil {
		h.logger.Error("failed to create SAML AuthnRequest", zap.Error(err))
		http.Error(w, "falha ao iniciar SSO", http.StatusInternalServerError)
		return
	}

	redirectURL, err := authnReq.Redirect("", sp)
	if err != nil {
		h.logger.Error("failed to build SAML redirect URL", zap.Error(err))
		http.Error(w, "falha ao redirecionar para IdP", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

func (h *SAMLAuthHandler) ACS(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.redirectWithError(w, r, "falha ao processar resposta SAML")
		return
	}

	sp := h.provider.GetServiceProvider()

	assertionInfo, err := sp.RetrieveAssertionInfo(r.FormValue("SAMLResponse"))
	if err != nil {
		h.logger.Error("SAML assertion validation failed", zap.Error(err))
		h.redirectWithError(w, r, "autenticação SAML falhou")
		return
	}

	if assertionInfo.WarningInfo.InvalidTime {
		h.logger.Warn("SAML assertion has invalid time")
		h.redirectWithError(w, r, "sessão SAML expirada")
		return
	}

	email := assertionInfo.NameID
	if email == "" {
		h.redirectWithError(w, r, "email não encontrado na resposta SAML")
		return
	}

	nome := email
	for _, attr := range assertionInfo.Values {
		if attr.Name == "displayName" || attr.Name == "cn" {
			if len(attr.Values) > 0 {
				nome = attr.Values[0].Value
			}
		}
	}

	usuario, err := h.store.BuscarOuCriarPorEmail(r.Context(), email, nome, "saml")
	if err != nil {
		h.logger.Error("failed to find/create usuario from SAML", zap.Error(err), zap.String("email", email))
		h.redirectWithError(w, r, "erro ao criar usuário")
		return
	}

	token, err := h.tokenService.GenerateToken(usuario.ID, usuario.Email, usuario.Cargo)
	if err != nil {
		h.logger.Error("failed to generate JWT after SAML", zap.Error(err))
		h.redirectWithError(w, r, "erro ao gerar token")
		return
	}

	redirectURL := fmt.Sprintf("%s/#auth_token=%s", h.frontendURL, url.QueryEscape(token))
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (h *SAMLAuthHandler) Metadata(w http.ResponseWriter, r *http.Request) {
	sp := h.provider.GetServiceProvider()
	metadata := sp.Metadata()

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(metadata)
}

func (h *SAMLAuthHandler) redirectWithError(w http.ResponseWriter, r *http.Request, msg string) {
	redirectURL := fmt.Sprintf("%s/#auth_error=%s", h.frontendURL, url.QueryEscape(msg))
	http.Redirect(w, r, redirectURL, http.StatusFound)
}
```

Note: `crewjam/saml` API may require adjustments based on exact library version. The `RetrieveAssertionInfo` method returns decoded assertion info directly. If library API differs, adapt to use `ParseResponse` + manual assertion extraction.

- [ ] **Step 2: Add missing imports**

Ensure `context` and `domain` imports:
```go
import (
    "context"
    // ...
    "github.com/emersonpaula83/myplanner/backend/internal/domain"
)
```

Note: `domain` import needed for `SAMLUserStore` interface return type reference. However, since `SAMLUserStore` returns `*domain.Usuario`, the import is required.

- [ ] **Step 3: Build to verify**

```bash
cd backend && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handler/saml_auth.go
git commit -m "feat: add SAML auth handler (login, ACS, metadata endpoints)"
```

---

### Task 6: Restrict Login to Local auth_provider

**Files:**
- Modify: `backend/internal/handler/auth.go:42-81` — add auth_provider check in `Login()`

**Interfaces:**
- Consumes: `domain.Usuario.AuthProvider` from Task 2
- Produces: Login endpoint rejects SAML users attempting email/password login

- [ ] **Step 1: Add auth_provider guard in Login()**

After the `usuario == nil` check (line ~60), before the bcrypt compare, add:

```go
if usuario.AuthProvider != "local" {
    respondError(w, http.StatusUnauthorized, "utilize o login via Fluig Identity")
    return
}
```

- [ ] **Step 2: Build to verify**

```bash
cd backend && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handler/auth.go
git commit -m "feat: restrict email/password login to local auth_provider only"
```

---

### Task 7: Admin Password Rotation — `internal/admin/rotation.go`

**Files:**
- Create: `backend/internal/admin/rotation.go`

**Interfaces:**
- Consumes:
  - `config.AuthConfig.AdminEmail` — fixed admin email
  - Repository: needs `BuscarPorEmail()` and `AtualizarSenha()` from `UsuarioStore`
- Produces:
  - `AdminRotator` struct
  - `NewAdminRotator(store AdminPasswordStore, secretWriter SecretWriter, adminEmail string, logger *zap.Logger) *AdminRotator`
  - `Start(ctx context.Context)` — runs rotation immediately, then every 24h
  - `RotateNow(ctx context.Context) error` — generates new password, updates DB, writes to secret store

- [ ] **Step 1: Write rotation.go**

```go
package admin

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

const passwordCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%&*"
const passwordLength = 16

type AdminPasswordStore interface {
	BuscarPorEmail(ctx context.Context, email string) (id uuid.UUID, err error)
	AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error
}

type SecretWriter interface {
	WritePassword(ctx context.Context, password string, expiresAt time.Time) error
}

type AdminRotator struct {
	store       AdminPasswordStore
	writer      SecretWriter
	adminEmail  string
	logger      *zap.Logger
}

func NewAdminRotator(store AdminPasswordStore, writer SecretWriter, adminEmail string, logger *zap.Logger) *AdminRotator {
	return &AdminRotator{
		store:      store,
		writer:     writer,
		adminEmail: adminEmail,
		logger:     logger,
	}
}

func (ar *AdminRotator) Start(ctx context.Context) {
	if err := ar.RotateNow(ctx); err != nil {
		ar.logger.Error("initial admin password rotation failed", zap.Error(err))
	}

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := ar.RotateNow(ctx); err != nil {
				ar.logger.Error("admin password rotation failed", zap.Error(err))
			}
		}
	}
}

func (ar *AdminRotator) RotateNow(ctx context.Context) error {
	password, err := generatePassword()
	if err != nil {
		return fmt.Errorf("generating password: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	adminID, err := ar.store.BuscarPorEmail(ctx, ar.adminEmail)
	if err != nil {
		return fmt.Errorf("finding admin user: %w", err)
	}
	if adminID == uuid.Nil {
		return fmt.Errorf("admin user %s not found", ar.adminEmail)
	}

	if err := ar.store.AtualizarSenha(ctx, adminID, string(hash)); err != nil {
		return fmt.Errorf("updating admin password: %w", err)
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	if err := ar.writer.WritePassword(ctx, password, expiresAt); err != nil {
		return fmt.Errorf("writing password to secret store: %w", err)
	}

	ar.logger.Info("admin password rotated successfully")
	return nil
}

func generatePassword() (string, error) {
	b := make([]byte, passwordLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordCharset))))
		if err != nil {
			return "", err
		}
		b[i] = passwordCharset[n.Int64()]
	}
	return string(b), nil
}
```

- [ ] **Step 2: Create AdminPasswordStoreAdapter**

The `AdminPasswordStore` interface differs from `UsuarioRepository` (returns `id` not full user). Create a simple adapter in the same file:

```go
type RepoAdapter struct {
	repo interface {
		BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error)
		AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error
	}
}

func NewRepoAdapter(repo interface {
	BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error)
	AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error
}) *RepoAdapter {
	return &RepoAdapter{repo: repo}
}

func (a *RepoAdapter) BuscarPorEmail(ctx context.Context, email string) (uuid.UUID, error) {
	u, err := a.repo.BuscarPorEmail(ctx, email)
	if err != nil {
		return uuid.Nil, err
	}
	if u == nil {
		return uuid.Nil, nil
	}
	return u.ID, nil
}

func (a *RepoAdapter) AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error {
	return a.repo.AtualizarSenha(ctx, id, senhaHash)
}
```

Add import for domain:
```go
"github.com/emersonpaula83/myplanner/backend/internal/domain"
```

- [ ] **Step 3: Build to verify**

```bash
cd backend && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add backend/internal/admin/rotation.go
git commit -m "feat: add daily admin password rotation with SecretWriter interface"
```

---

### Task 8: K8s Secret Writer + Stdout Fallback — `internal/admin/k8s_secret.go`

**Files:**
- Create: `backend/internal/admin/k8s_secret.go`

**Interfaces:**
- Consumes: `SecretWriter` interface from Task 7, `config.AdminSecretConfig` from Task 2
- Produces:
  - `K8sSecretWriter` — implements `SecretWriter`, writes to K8s Secret
  - `StdoutSecretWriter` — implements `SecretWriter`, logs password to stdout (dev fallback)
  - `NewSecretWriter(cfg config.AdminSecretConfig, logger *zap.Logger) SecretWriter` — factory: returns K8s writer if in cluster, stdout otherwise

- [ ] **Step 1: Add k8s.io/client-go dependency**

```bash
cd backend && go get k8s.io/client-go@latest k8s.io/apimachinery@latest
```

- [ ] **Step 2: Write k8s_secret.go**

```go
package admin

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/emersonpaula83/myplanner/backend/internal/config"
)

type K8sSecretWriter struct {
	clientset *kubernetes.Clientset
	name      string
	namespace string
	logger    *zap.Logger
}

func newK8sSecretWriter(cfg config.AdminSecretConfig, logger *zap.Logger) (*K8sSecretWriter, error) {
	k8sCfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("getting in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(k8sCfg)
	if err != nil {
		return nil, fmt.Errorf("creating k8s client: %w", err)
	}

	ns := cfg.SecretNamespace
	if ns == "" {
		nsBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			return nil, fmt.Errorf("detecting namespace: %w", err)
		}
		ns = string(nsBytes)
	}

	return &K8sSecretWriter{
		clientset: clientset,
		name:      cfg.SecretName,
		namespace: ns,
		logger:    logger,
	}, nil
}

func (w *K8sSecretWriter) WritePassword(ctx context.Context, password string, expiresAt time.Time) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.name,
			Namespace: w.namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"password":   []byte(password),
			"expires_at": []byte(expiresAt.Format(time.RFC3339)),
		},
	}

	secretsClient := w.clientset.CoreV1().Secrets(w.namespace)

	existing, err := secretsClient.Get(ctx, w.name, metav1.GetOptions{})
	if err != nil {
		// Secret doesn't exist, create it
		_, err = secretsClient.Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating k8s secret: %w", err)
		}
		w.logger.Info("k8s secret created", zap.String("name", w.name))
		return nil
	}

	existing.Data = secret.Data
	_, err = secretsClient.Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating k8s secret: %w", err)
	}
	w.logger.Info("k8s secret updated", zap.String("name", w.name))
	return nil
}

type StdoutSecretWriter struct {
	logger *zap.Logger
}

func (w *StdoutSecretWriter) WritePassword(_ context.Context, password string, expiresAt time.Time) error {
	// In dev mode, print password to stdout
	fmt.Printf("\n========================================\n")
	fmt.Printf("  ADMIN PASSWORD (dev mode)\n")
	fmt.Printf("  Password: %s\n", password)
	fmt.Printf("  Expires:  %s\n", expiresAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("========================================\n\n")
	w.logger.Info("admin password rotated (dev mode, printed to stdout)")
	return nil
}

func NewSecretWriter(cfg config.AdminSecretConfig, logger *zap.Logger) SecretWriter {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		writer, err := newK8sSecretWriter(cfg, logger)
		if err != nil {
			logger.Warn("failed to init K8s secret writer, falling back to stdout", zap.Error(err))
			return &StdoutSecretWriter{logger: logger}
		}
		return writer
	}
	return &StdoutSecretWriter{logger: logger}
}
```

- [ ] **Step 3: Build to verify**

```bash
cd backend && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add backend/internal/admin/k8s_secret.go
git commit -m "feat: add K8s Secret writer + stdout fallback for admin password"
```

---

### Task 9: Equipe Filter Middleware — `internal/middleware/equipe_filter.go`

**Files:**
- Create: `backend/internal/middleware/equipe_filter.go`

**Interfaces:**
- Consumes:
  - `UserIDFromContext()`, `UserEmailFromContext()` from `middleware/auth.go`
  - `BuscarEquipeIDsPorUsuario()` from Task 3
- Produces:
  - `EquipeFilter(fetcher EquipeIDsFetcher, adminEmail string) func(http.Handler) http.Handler`
  - `EquipeIDsFromContext(ctx) []uuid.UUID`
  - Context key `equipeIDsKey` with equipe IDs (empty for non-admin without alçada, all equipes for admin)

- [ ] **Step 1: Write equipe_filter.go**

```go
package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

const equipeIDsKey contextKey = "equipe_ids"

type EquipeIDsFetcher interface {
	BuscarEquipeIDsPorUsuario(ctx context.Context, usuarioID uuid.UUID) ([]uuid.UUID, error)
}

type AllEquipesFetcher interface {
	ListarTodosIDs(ctx context.Context) ([]uuid.UUID, error)
}

func EquipeFilter(fetcher EquipeIDsFetcher, allFetcher AllEquipesFetcher, adminEmail string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := UserIDFromContext(r.Context())
			if userID == uuid.Nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "usuário não autenticado"})
				return
			}

			email := UserEmailFromContext(r.Context())

			var ids []uuid.UUID
			var err error

			if email == adminEmail {
				ids, err = allFetcher.ListarTodosIDs(r.Context())
			} else {
				ids, err = fetcher.BuscarEquipeIDsPorUsuario(r.Context(), userID)
			}

			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "falha ao buscar alçada"})
				return
			}

			ctx := context.WithValue(r.Context(), equipeIDsKey, ids)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func EquipeIDsFromContext(ctx context.Context) []uuid.UUID {
	ids, _ := ctx.Value(equipeIDsKey).([]uuid.UUID)
	return ids
}
```

- [ ] **Step 2: Add ListarTodosIDs to equipe repository**

In `backend/internal/repository/equipe.go`, add:

```go
func (r *EquipeRepository) ListarTodosIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM equipes ORDER BY nome`)
	if err != nil {
		return nil, fmt.Errorf("listing all equipe IDs: %w", err)
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning equipe ID: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
```

- [ ] **Step 3: Build to verify**

```bash
cd backend && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add backend/internal/middleware/equipe_filter.go backend/internal/repository/equipe.go
git commit -m "feat: add EquipeFilter middleware for alçada-based access control"
```

---

### Task 10: Alçada Endpoints in Usuario Handler

**Files:**
- Modify: `backend/internal/handler/usuario.go` — add `ListEquipes()`, `UpdateEquipes()` methods
- Modify: `backend/internal/handler/auth.go` — extend `UsuarioStore` interface

**Interfaces:**
- Consumes:
  - `UsuarioRepository.ListarEquipesPorUsuario()` from Task 3
  - `UsuarioRepository.AtualizarEquipes()` from Task 3
  - `domain.AlcadaEquipesRequest` from Task 2
  - `middleware.UserEmailFromContext()` — to check admin
- Produces:
  - `GET /api/v1/usuarios/{id}/equipes` → `ListEquipes(w, r)`
  - `PUT /api/v1/usuarios/{id}/equipes` → `UpdateEquipes(w, r)` (admin-only)

- [ ] **Step 1: Extend UsuarioStore interface in auth.go**

Add to `UsuarioStore` interface:

```go
BuscarOuCriarPorEmail(ctx context.Context, email, nomeCompleto, authProvider string) (*domain.Usuario, error)
ListarEquipesPorUsuario(ctx context.Context, usuarioID uuid.UUID) ([]repository.EquipeResumo, error)
AtualizarEquipes(ctx context.Context, usuarioID uuid.UUID, equipeIDs []uuid.UUID) error
BuscarEquipeIDsPorUsuario(ctx context.Context, usuarioID uuid.UUID) ([]uuid.UUID, error)
```

Add import:
```go
"github.com/emersonpaula83/myplanner/backend/internal/repository"
```

- [ ] **Step 2: Add handler methods in usuario.go**

```go
func (h *UsuarioHandler) ListEquipes(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	equipes, err := h.store.ListarEquipesPorUsuario(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to list equipes for usuario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar equipes")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"equipes": equipes})
}

func (h *UsuarioHandler) UpdateEquipes(w http.ResponseWriter, r *http.Request) {
	callerEmail := middleware.UserEmailFromContext(r.Context())
	if callerEmail != "admin@myplanner.local" {
		respondError(w, http.StatusForbidden, "somente admin pode alterar alçada")
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req domain.AlcadaEquipesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	if err := h.store.AtualizarEquipes(r.Context(), id, req.EquipeIDs); err != nil {
		h.logger.Error("failed to update equipes", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar equipes")
		return
	}

	equipes, err := h.store.ListarEquipesPorUsuario(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to list equipes after update", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar equipes")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"equipes": equipes})
}
```

Add import for middleware:
```go
"github.com/emersonpaula83/myplanner/backend/internal/middleware"
```

- [ ] **Step 3: Build to verify**

```bash
cd backend && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handler/usuario.go backend/internal/handler/auth.go
git commit -m "feat: add alçada equipe endpoints (list/update, admin-only update)"
```

---

### Task 11: Route Registration + Rotation Startup in main.go

**Files:**
- Modify: `backend/cmd/api/main.go` — register SAML routes, start admin rotation goroutine, swap ProjetoFilter for EquipeFilter

**Interfaces:**
- Consumes:
  - `saml.NewSAMLProvider()` from Task 4
  - `handler.NewSAMLAuthHandler()` from Task 5
  - `admin.NewAdminRotator()`, `admin.NewRepoAdapter()`, `admin.NewSecretWriter()` from Tasks 7-8
  - `middleware.EquipeFilter()` from Task 9
  - All handler alçada methods from Task 10
- Produces: Updated route table with SAML endpoints, admin rotation goroutine

- [ ] **Step 1: Add imports**

```go
"github.com/emersonpaula83/myplanner/backend/internal/admin"
samlpkg "github.com/emersonpaula83/myplanner/backend/internal/saml"
```

- [ ] **Step 2: Initialize SAML provider (optional — only if SAML configured)**

After `tokenService` creation (~line 66), add:

```go
var samlAuthHandler *handler.SAMLAuthHandler
if cfg.SAML.IDPMetadataURL != "" {
    samlProvider, err := samlpkg.NewSAMLProvider(cfg.SAML)
    if err != nil {
        logger.Fatal("failed to init SAML provider", zap.Error(err))
    }
    samlAuthHandler = handler.NewSAMLAuthHandler(samlProvider, usuarioRepo, tokenService, cfg.SAML.FrontendURL, logger)
    logger.Info("SAML SSO configured", zap.String("entity_id", cfg.SAML.EntityID))
} else {
    logger.Warn("SAML_IDP_METADATA_URL not set, SAML SSO disabled")
}
```

- [ ] **Step 3: Start admin rotation goroutine**

After `go schedulerSvc.Start(ctx)` (~line 131), add:

```go
secretWriter := admin.NewSecretWriter(cfg.AdminSecret, logger)
adminStore := admin.NewRepoAdapter(usuarioRepo)
adminRotator := admin.NewAdminRotator(adminStore, secretWriter, cfg.Auth.AdminEmail, logger)
go adminRotator.Start(ctx)
```

- [ ] **Step 4: Register SAML routes (outside auth middleware)**

After the OAuth routes block (~line 177), add:

```go
if samlAuthHandler != nil {
    r.Get("/api/v1/auth/saml/login", samlAuthHandler.Login)
    r.Post("/api/v1/auth/saml/acs", samlAuthHandler.ACS)
    r.Get("/api/v1/auth/saml/metadata", samlAuthHandler.Metadata)
}
```

- [ ] **Step 5: Replace ProjetoFilter with EquipeFilter**

Change line ~184:
```go
r.Use(middleware.ProjetoFilter(usuarioRepo))
```
to:
```go
r.Use(middleware.EquipeFilter(usuarioRepo, equipeRepo, cfg.Auth.AdminEmail))
```

- [ ] **Step 6: Add alçada routes**

In the authenticated group, after existing usuario routes (~line 198), add:

```go
r.Get("/usuarios/{id}/equipes", usuarioHandler.ListEquipes)
r.Put("/usuarios/{id}/equipes", usuarioHandler.UpdateEquipes)
```

- [ ] **Step 7: Build to verify**

```bash
cd backend && go build ./...
```

- [ ] **Step 8: Commit**

```bash
git add backend/cmd/api/main.go
git commit -m "feat: register SAML routes, start admin rotation, swap to EquipeFilter"
```

---

### Task 12: Update Seed to set auth_provider

**Files:**
- Modify: `backend/cmd/seed/main.go` — set `auth_provider = 'local'` for admin

**Interfaces:**
- Consumes: `auth_provider` column from Task 1

- [ ] **Step 1: Update seed INSERT**

Change the INSERT query to include `auth_provider`:

```sql
INSERT INTO usuarios (nome_completo, apelido, email, senha_hash, cargo, auth_provider)
VALUES ('Administrador', 'admin', $1, $2, 'coordenador', 'local')
ON CONFLICT (email) DO UPDATE SET senha_hash = $2, auth_provider = 'local'
```

- [ ] **Step 2: Build to verify**

```bash
cd backend && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add backend/cmd/seed/main.go
git commit -m "feat: set auth_provider=local for admin in seed"
```

---

### Task 13: dev.sh — get-master-pass command

**Files:**
- Modify: `dev.sh` — add `get-master-pass` command

**Interfaces:**
- Consumes: K8s Secret `myplanner-admin-password` from Task 8
- Produces: `./dev.sh get-master-pass` command that retrieves admin password from K8s

- [ ] **Step 1: Add cmd_get_master_pass function**

After `cmd_cleanall()` function (~line 226), add:

```bash
cmd_get_master_pass() {
    log "Buscando senha do admin no K8s..."
    local ns="${ADMIN_SECRET_NAMESPACE:-default}"
    local secret_name="${ADMIN_SECRET_NAME:-myplanner-admin-password}"

    local password
    password=$(kubectl get secret "$secret_name" -n "$ns" \
        -o jsonpath='{.data.password}' 2>/dev/null | base64 -d 2>/dev/null)

    if [ -z "$password" ]; then
        err "Sem acesso ao cluster / aplicação ou falta de credenciais RBAC"
        return 1
    fi

    local expires
    expires=$(kubectl get secret "$secret_name" -n "$ns" \
        -o jsonpath='{.data.expires_at}' 2>/dev/null | base64 -d 2>/dev/null)

    echo ""
    echo -e "  ${CYAN}Admin Password${NC}"
    echo -e "  Email:    admin@myplanner.local"
    echo -e "  Senha:    ${GREEN}$password${NC}"
    if [ -n "$expires" ]; then
        echo -e "  Expira:   $expires"
    fi
    echo ""
}
```

- [ ] **Step 2: Add to case statement**

Add before `help|*)`:

```bash
get-master-pass) cmd_get_master_pass ;;
```

- [ ] **Step 3: Add to help text**

In `cmd_help()`, add:

```bash
echo "  get-master-pass  Busca senha admin no K8s Secret"
```

- [ ] **Step 4: Test locally (expect error — no K8s)**

```bash
./dev.sh get-master-pass
```

Expected: "Sem acesso ao cluster / aplicação ou falta de credenciais RBAC"

- [ ] **Step 5: Commit**

```bash
git add dev.sh
git commit -m "feat: add get-master-pass command to dev.sh"
```

---

### Task 14: Frontend — Login Page (SAML + Admin fallback)

**Files:**
- Modify: `frontend/index.html` — update login UI

**Interfaces:**
- Consumes:
  - `GET /api/v1/auth/saml/login` — SAML SSO redirect
  - `POST /api/v1/auth/login` — admin local login
  - `auth_token` and `auth_error` hash params
- Produces: Login page with "Entrar com Fluig Identity" button + "Acesso administrativo" link expanding email/senha form

- [ ] **Step 1: Find current login form in index.html**

Search for the login form HTML section (look for `id="login"` or similar).

- [ ] **Step 2: Replace login form**

Replace existing login HTML with:

```html
<div id="loginPage" style="display:flex;align-items:center;justify-content:center;min-height:100vh;">
    <div style="max-width:400px;width:100%;padding:2rem;">
        <h1 style="text-align:center;margin-bottom:2rem;">MyPlanner</h1>

        <div id="authError" style="display:none;background:#fee;border:1px solid #f88;padding:0.75rem;border-radius:8px;margin-bottom:1rem;color:#c33;text-align:center;"></div>

        <a href="/api/v1/auth/saml/login" class="btn btn-primary" style="display:block;text-align:center;padding:0.75rem;font-size:1.1rem;margin-bottom:1.5rem;">
            Entrar com Fluig Identity
        </a>

        <div style="text-align:center;">
            <a href="#" onclick="document.getElementById('adminLoginForm').style.display=document.getElementById('adminLoginForm').style.display==='none'?'block':'none';return false;" style="font-size:0.85rem;color:#888;">
                Acesso administrativo
            </a>
        </div>

        <div id="adminLoginForm" style="display:none;margin-top:1rem;">
            <input type="email" id="loginEmail" placeholder="Email" style="width:100%;padding:0.5rem;margin-bottom:0.5rem;border:1px solid #ccc;border-radius:4px;">
            <input type="password" id="loginSenha" placeholder="Senha" style="width:100%;padding:0.5rem;margin-bottom:0.75rem;border:1px solid #ccc;border-radius:4px;">
            <button onclick="doAdminLogin()" class="btn btn-secondary" style="width:100%;padding:0.5rem;">Entrar</button>
        </div>
    </div>
</div>
```

- [ ] **Step 3: Update JS — handle auth_token and auth_error from hash**

Find existing hash handling code (used for OAuth Atlassian). Extend or add near app initialization:

```javascript
function checkAuthHash() {
    var hash = window.location.hash;
    if (hash.indexOf('auth_token=') > -1) {
        var token = hash.split('auth_token=')[1].split('&')[0];
        token = decodeURIComponent(token);
        localStorage.setItem('myplanner_token', token);
        window.location.hash = '';
        return true;
    }
    if (hash.indexOf('auth_error=') > -1) {
        var error = decodeURIComponent(hash.split('auth_error=')[1].split('&')[0]);
        var el = document.getElementById('authError');
        if (el) {
            el.textContent = error;
            el.style.display = 'block';
        }
        window.location.hash = '';
        return false;
    }
    return false;
}
```

Call `checkAuthHash()` at startup, before checking stored token.

- [ ] **Step 4: Update doAdminLogin function**

```javascript
function doAdminLogin() {
    var email = document.getElementById('loginEmail').value;
    var senha = document.getElementById('loginSenha').value;
    if (!email || !senha) return;

    fetch('/api/v1/auth/login', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({email: email, senha: senha})
    })
    .then(function(r) { return r.json().then(function(d) { return {ok: r.ok, data: d}; }); })
    .then(function(res) {
        if (!res.ok) {
            var el = document.getElementById('authError');
            el.textContent = res.data.error || 'Erro ao fazer login';
            el.style.display = 'block';
            return;
        }
        localStorage.setItem('myplanner_token', res.data.token);
        window.location.reload();
    })
    .catch(function() {
        var el = document.getElementById('authError');
        el.textContent = 'Erro de conexão';
        el.style.display = 'block';
    });
}
```

- [ ] **Step 5: Build backend, open browser, verify login page renders**

```bash
./dev.sh restart
```

Open browser at `http://localhost:9091`. Verify:
- "Entrar com Fluig Identity" button visible
- "Acesso administrativo" link toggles email/senha form
- Admin login still works with email/senha

- [ ] **Step 6: Commit**

```bash
git add frontend/index.html
git commit -m "feat: update login page with SAML SSO button + admin fallback"
```

---

### Task 15: Frontend — Alçada Management UI (admin only)

**Files:**
- Modify: `frontend/index.html` — add alçada management section in usuarios view

**Interfaces:**
- Consumes:
  - `GET /api/v1/usuarios` — list all users
  - `GET /api/v1/equipes` — list all equipes
  - `GET /api/v1/usuarios/{id}/equipes` — get user's current equipes
  - `PUT /api/v1/usuarios/{id}/equipes` — update user's equipes
- Produces: Admin-visible section in usuarios management showing equipe multiselect per user

- [ ] **Step 1: Find usuarios management section in index.html**

Search for the usuarios listing/management code.

- [ ] **Step 2: Add alçada column/section**

For each user in the management list, add a multiselect showing their equipes. The multiselect should list all available equipes with checkboxes. Only visible when logged-in user is admin (`admin@myplanner.local`).

Implementation depends on existing UI patterns in the file — follow the established style for multiselects, modals, or inline editing.

Key logic:

```javascript
function loadUsuarioEquipes(usuarioId) {
    return apiFetch('/api/v1/usuarios/' + usuarioId + '/equipes')
        .then(function(data) { return data.equipes || []; });
}

function salvarAlcada(usuarioId, equipeIds) {
    return apiFetch('/api/v1/usuarios/' + usuarioId + '/equipes', {
        method: 'PUT',
        body: JSON.stringify({equipe_ids: equipeIds})
    });
}
```

- [ ] **Step 3: Test in browser**

- Login as admin
- Go to usuarios section
- Verify equipe multiselect appears for each user
- Assign equipes, save, refresh — verify persistence

- [ ] **Step 4: Commit**

```bash
git add frontend/index.html
git commit -m "feat: add alçada equipe management UI for admin"
```

---

### Task 16: Integration Verification

**Files:** None (verification only)

- [ ] **Step 1: Build full backend**

```bash
cd backend && go build ./...
```

- [ ] **Step 2: Run tests**

```bash
cd backend && go test ./...
```

Fix any broken tests from the `SenhaHash` type change or new `auth_provider` column.

- [ ] **Step 3: Run full stack**

```bash
./dev.sh restart
```

- [ ] **Step 4: Verify admin login still works**

Login with `admin@myplanner.local` via "Acesso administrativo" form.

- [ ] **Step 5: Verify SAML metadata endpoint**

```bash
curl -s http://localhost:9091/api/v1/auth/saml/metadata
```

If SAML not configured (no env vars), verify server starts without crashing (SAML disabled warning in logs).

- [ ] **Step 6: Verify password rotation**

Check stdout for admin password printed at startup (dev mode).

- [ ] **Step 7: Verify dev.sh get-master-pass**

```bash
./dev.sh get-master-pass
```

Expected: error message (no K8s in dev).

- [ ] **Step 8: Verify alçada endpoints**

```bash
# Get admin token
TOKEN=$(curl -s http://localhost:9091/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@myplanner.local","senha":"<check stdout>"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

# List equipes for a user
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:9091/api/v1/usuarios/<user-id>/equipes
```

- [ ] **Step 9: Commit any fixes**

```bash
git add -u
git commit -m "fix: integration fixes for IAM module"
```
