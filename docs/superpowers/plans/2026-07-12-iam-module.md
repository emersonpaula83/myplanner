# IAM Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add authentication (JWT) and project-level access control (Alçada de Projetos) to the tCloud Planner backend.

**Architecture:** Handler → Repository → Domain layering following existing patterns. JWT stateless auth via middleware chain. Two new tables (`usuarios`, `usuario_projetos`) with admin seed. Middleware injects user identity and project visibility into request context; downstream handlers/repositories use context values to filter data.

**Tech Stack:** Go 1.25, chi/v5, pgx/v5, bcrypt (golang.org/x/crypto), golang-jwt/jwt/v5, zap logger, viper config.

## Global Constraints

- Password hashing: bcrypt cost 12
- JWT signing: HMAC-SHA256 via `JWT_SECRET` env var (required — app must refuse to start if empty)
- JWT expiration: configurable via `JWT_EXPIRATION_HOURS` (default 24)
- Admin seed email: configurable via `ADMIN_EMAIL` (default `admin@tcloud.local`)
- Cargo values: exactly `coordenador`, `gerente`, `gerente_projetos` — enforced by DB CHECK and handler validation
- Senha mínima: 8 characters
- Apelido: unique, max 50 chars
- Email: unique, valid format
- `SenhaHash` field MUST use `json:"-"` tag — never serialize password hash to API responses
- Follow existing codebase patterns: interface-based handler dependencies, `respondJSON`/`respondError` helpers, `pgxpool` for DB access
- All new files under `backend/internal/`
- Migration sequence: 000003 (follows existing 000001 + 000002)

## File Structure

```
backend/
├── cmd/api/main.go                          (modify — wire routes + middleware)
├── internal/
│   ├── auth/
│   │   ├── jwt.go                           (create — token service)
│   │   └── jwt_test.go                      (create)
│   ├── config/
│   │   └── config.go                        (modify — add AuthConfig)
│   ├── domain/
│   │   └── usuario.go                       (create — types)
│   ├── handler/
│   │   ├── auth.go                          (create — login endpoint)
│   │   ├── auth_test.go                     (create)
│   │   ├── usuario.go                       (create — CRUD + alçada)
│   │   └── usuario_test.go                  (create)
│   ├── middleware/
│   │   ├── auth.go                          (create — JWT validation)
│   │   ├── auth_test.go                     (create)
│   │   ├── projeto_filter.go                (create — inject projeto_ids)
│   │   └── projeto_filter_test.go           (create)
│   └── repository/
│       └── usuario.go                       (create — DB access)
├── migrations/
│   ├── 000003_iam.up.sql                    (create)
│   └── 000003_iam.down.sql                  (create)
```

---

### Task 1: Domain Types + Config

**Files:**
- Create: `backend/internal/domain/usuario.go`
- Modify: `backend/internal/config/config.go`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `domain.Usuario` struct — used by Tasks 3-7
  - `domain.LoginRequest`, `domain.LoginResponse` — used by Task 6
  - `domain.CriarUsuarioRequest` — used by Tasks 4, 7
  - `domain.AtualizarUsuarioRequest` — used by Tasks 4, 7
  - `domain.AlterarSenhaRequest` — used by Task 7
  - `domain.AlcadaProjetosRequest` — used by Tasks 4, 7
  - `domain.ProjetoResumo` — used by Tasks 4, 7
  - `config.AuthConfig` — used by Tasks 3, 8

- [ ] **Step 1: Create `backend/internal/domain/usuario.go`**

```go
package domain

import (
	"time"

	"github.com/google/uuid"
)

type Usuario struct {
	ID           uuid.UUID `json:"id" db:"id"`
	NomeCompleto string    `json:"nome_completo" db:"nome_completo"`
	Apelido      string    `json:"apelido" db:"apelido"`
	Email        string    `json:"email" db:"email"`
	SenhaHash    string    `json:"-" db:"senha_hash"`
	Cargo        string    `json:"cargo" db:"cargo"`
	Ativo        bool      `json:"ativo" db:"ativo"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type LoginRequest struct {
	Email string `json:"email"`
	Senha string `json:"senha"`
}

type LoginResponse struct {
	Token   string  `json:"token"`
	Usuario Usuario `json:"usuario"`
}

type CriarUsuarioRequest struct {
	NomeCompleto string `json:"nome_completo"`
	Apelido      string `json:"apelido"`
	Email        string `json:"email"`
	Senha        string `json:"senha"`
	Cargo        string `json:"cargo"`
}

type AtualizarUsuarioRequest struct {
	NomeCompleto *string `json:"nome_completo"`
	Apelido      *string `json:"apelido"`
	Email        *string `json:"email"`
	Cargo        *string `json:"cargo"`
	Ativo        *bool   `json:"ativo"`
}

type AlterarSenhaRequest struct {
	SenhaAtual string `json:"senha_atual"`
	NovaSenha  string `json:"nova_senha"`
}

type AlcadaProjetosRequest struct {
	ProjetoIDs []uuid.UUID `json:"projeto_ids"`
}

type ProjetoResumo struct {
	ID    uuid.UUID `json:"id"`
	Chave string    `json:"chave"`
	Nome  string    `json:"nome"`
}
```

- [ ] **Step 2: Add `AuthConfig` to `backend/internal/config/config.go`**

Add the `AuthConfig` struct and field to `Config`:

```go
// Add to Config struct:
type Config struct {
	DB     DBConfig
	Server ServerConfig
	Jira   JiraConfig
	Sync   SyncConfig
	Log    LogConfig
	Auth   AuthConfig
}

// Add new struct:
type AuthConfig struct {
	JWTSecret          string
	JWTExpirationHours int
	AdminEmail         string
}
```

Add defaults and loading in `Load()`:

```go
// Add defaults after existing defaults:
viper.SetDefault("JWT_EXPIRATION_HOURS", 24)
viper.SetDefault("ADMIN_EMAIL", "admin@tcloud.local")

// Add to returned Config:
Auth: AuthConfig{
	JWTSecret:          viper.GetString("JWT_SECRET"),
	JWTExpirationHours: viper.GetInt("JWT_EXPIRATION_HOURS"),
	AdminEmail:         viper.GetString("ADMIN_EMAIL"),
},
```

- [ ] **Step 3: Verify compilation**

Run: `cd backend && go build ./...`
Expected: BUILD SUCCESS (no errors)

- [ ] **Step 4: Commit**

```bash
git add backend/internal/domain/usuario.go backend/internal/config/config.go
git commit -m "feat(iam): add domain types and auth config"
```

---

### Task 2: Migration — IAM Tables + Admin Seed

**Files:**
- Create: `backend/migrations/000003_iam.up.sql`
- Create: `backend/migrations/000003_iam.down.sql`

**Interfaces:**
- Consumes: nothing
- Produces: `usuarios` and `usuario_projetos` tables in PostgreSQL, admin seed row

- [ ] **Step 1: Create `backend/migrations/000003_iam.up.sql`**

```sql
CREATE TABLE usuarios (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nome_completo  VARCHAR(255) NOT NULL,
    apelido        VARCHAR(50) NOT NULL UNIQUE,
    email          VARCHAR(255) NOT NULL UNIQUE,
    senha_hash     VARCHAR(255) NOT NULL,
    cargo          VARCHAR(50) NOT NULL CHECK (cargo IN ('coordenador', 'gerente', 'gerente_projetos')),
    ativo          BOOLEAN NOT NULL DEFAULT true,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_usuarios_email ON usuarios(email);

CREATE TABLE usuario_projetos (
    usuario_id  UUID NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    projeto_id  UUID NOT NULL REFERENCES projetos(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (usuario_id, projeto_id)
);

CREATE INDEX idx_usuario_projetos_usuario ON usuario_projetos(usuario_id);

-- Seed admin user
-- Password: Totvs@123 (bcrypt cost 12)
INSERT INTO usuarios (nome_completo, apelido, email, senha_hash, cargo)
VALUES (
    'Administrador',
    'admin',
    'admin@tcloud.local',
    '$2a$12$YD27E7brWZvrrq0lVpbsouDUIi3UiwgjT6NsiIOQGPzwDBlvC5DYK',
    'coordenador'
);

-- Grant admin access to all existing projects
INSERT INTO usuario_projetos (usuario_id, projeto_id)
SELECT u.id, p.id
FROM usuarios u, projetos p
WHERE u.apelido = 'admin';
```

- [ ] **Step 2: Create `backend/migrations/000003_iam.down.sql`**

```sql
DROP TABLE IF EXISTS usuario_projetos;
DROP TABLE IF EXISTS usuarios;
```

- [ ] **Step 3: Run migration**

Run: `cd backend && go run cmd/migrate/main.go -direction=up`
Expected: `migration up completed successfully` (or `no changes` if already applied)

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/000003_iam.up.sql backend/migrations/000003_iam.down.sql
git commit -m "feat(iam): add usuarios and usuario_projetos tables with admin seed"
```

---

### Task 3: JWT Token Service

**Files:**
- Create: `backend/internal/auth/jwt.go`
- Create: `backend/internal/auth/jwt_test.go`

**Interfaces:**
- Consumes: `config.AuthConfig` (from Task 1) for secret and expiration
- Produces:
  - `auth.TokenService` struct — used by Tasks 5, 6, 8
  - `auth.NewTokenService(secret string, expirationHours int) *TokenService`
  - `(*TokenService).GenerateToken(userID uuid.UUID, email, cargo string) (string, error)`
  - `(*TokenService).ValidateToken(tokenString string) (*Claims, error)`
  - `auth.Claims` struct with fields `Email string`, `Cargo string`, embedded `jwt.RegisteredClaims`

- [ ] **Step 1: Write failing test `backend/internal/auth/jwt_test.go`**

```go
package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerateAndValidateToken(t *testing.T) {
	ts := NewTokenService("test-secret-key-minimum-32-chars!!", 24)

	userID := uuid.New()
	email := "test@example.com"
	cargo := "coordenador"

	token, err := ts.GenerateToken(userID, email, cargo)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateToken returned empty token")
	}

	claims, err := ts.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.Subject != userID.String() {
		t.Errorf("subject = %q, want %q", claims.Subject, userID.String())
	}
	if claims.Email != email {
		t.Errorf("email = %q, want %q", claims.Email, email)
	}
	if claims.Cargo != cargo {
		t.Errorf("cargo = %q, want %q", claims.Cargo, cargo)
	}
}

func TestValidateToken_InvalidSignature(t *testing.T) {
	ts1 := NewTokenService("secret-key-one-32-chars-long!!!!", 24)
	ts2 := NewTokenService("secret-key-two-32-chars-long!!!!", 24)

	token, err := ts1.GenerateToken(uuid.New(), "test@example.com", "gerente")
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	_, err = ts2.ValidateToken(token)
	if err == nil {
		t.Fatal("ValidateToken should fail with wrong secret")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	ts := &TokenService{
		secret:     []byte("test-secret-key-minimum-32-chars!!"),
		expiration: -1 * time.Hour,
	}

	token, err := ts.GenerateToken(uuid.New(), "test@example.com", "gerente")
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	_, err = ts.ValidateToken(token)
	if err == nil {
		t.Fatal("ValidateToken should fail with expired token")
	}
}

func TestValidateToken_Malformed(t *testing.T) {
	ts := NewTokenService("test-secret-key-minimum-32-chars!!", 24)

	_, err := ts.ValidateToken("not.a.valid.token")
	if err == nil {
		t.Fatal("ValidateToken should fail with malformed token")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/auth/...`
Expected: FAIL (package doesn't exist yet)

- [ ] **Step 3: Implement `backend/internal/auth/jwt.go`**

```go
package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	Email string `json:"email"`
	Cargo string `json:"cargo"`
	jwt.RegisteredClaims
}

type TokenService struct {
	secret     []byte
	expiration time.Duration
}

func NewTokenService(secret string, expirationHours int) *TokenService {
	return &TokenService{
		secret:     []byte(secret),
		expiration: time.Duration(expirationHours) * time.Hour,
	}
}

func (ts *TokenService) GenerateToken(userID uuid.UUID, email, cargo string) (string, error) {
	now := time.Now()
	claims := Claims{
		Email: email,
		Cargo: cargo,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ts.expiration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(ts.secret)
}

func (ts *TokenService) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return ts.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/auth/... -v`
Expected: PASS (all 4 tests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/auth/jwt.go backend/internal/auth/jwt_test.go
git commit -m "feat(iam): add JWT token service with tests"
```

---

### Task 4: Repository — Usuario CRUD + Alçada

**Files:**
- Create: `backend/internal/repository/usuario.go`

**Interfaces:**
- Consumes:
  - `domain.Usuario` (from Task 1)
  - `domain.CriarUsuarioRequest` (from Task 1)
  - `domain.AtualizarUsuarioRequest` (from Task 1)
  - `domain.ProjetoResumo` (from Task 1)
- Produces:
  - `repository.UsuarioRepository` struct — used by Tasks 6-8
  - `repository.NewUsuarioRepository(pool *pgxpool.Pool) *UsuarioRepository`
  - `(*UsuarioRepository).BuscarPorEmail(ctx, email string) (*domain.Usuario, error)`
  - `(*UsuarioRepository).BuscarPorID(ctx, id uuid.UUID) (*domain.Usuario, error)`
  - `(*UsuarioRepository).ListarTodos(ctx) ([]domain.Usuario, error)`
  - `(*UsuarioRepository).Criar(ctx, req *domain.CriarUsuarioRequest, senhaHash string) (*domain.Usuario, error)`
  - `(*UsuarioRepository).Atualizar(ctx, id uuid.UUID, req *domain.AtualizarUsuarioRequest) (*domain.Usuario, error)`
  - `(*UsuarioRepository).AtualizarSenha(ctx, id uuid.UUID, senhaHash string) error`
  - `(*UsuarioRepository).ListarProjetos(ctx, usuarioID uuid.UUID) ([]domain.ProjetoResumo, error)`
  - `(*UsuarioRepository).AtualizarProjetos(ctx, usuarioID uuid.UUID, projetoIDs []uuid.UUID) ([]domain.ProjetoResumo, error)`
  - `(*UsuarioRepository).BuscarProjetoIDsPorUsuario(ctx, usuarioID uuid.UUID) ([]uuid.UUID, error)`

- [ ] **Step 1: Create `backend/internal/repository/usuario.go`**

```go
package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
)

type UsuarioRepository struct {
	pool *pgxpool.Pool
}

func NewUsuarioRepository(pool *pgxpool.Pool) *UsuarioRepository {
	return &UsuarioRepository{pool: pool}
}

func (r *UsuarioRepository) BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, nome_completo, apelido, email, senha_hash, cargo, ativo, created_at, updated_at
		FROM usuarios
		WHERE email = $1 AND ativo = true
	`, email)

	u, err := scanUsuario(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying usuario by email: %w", err)
	}
	return &u, nil
}

func (r *UsuarioRepository) BuscarPorID(ctx context.Context, id uuid.UUID) (*domain.Usuario, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, nome_completo, apelido, email, senha_hash, cargo, ativo, created_at, updated_at
		FROM usuarios
		WHERE id = $1
	`, id)

	u, err := scanUsuario(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying usuario by id: %w", err)
	}
	return &u, nil
}

func (r *UsuarioRepository) ListarTodos(ctx context.Context) ([]domain.Usuario, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, nome_completo, apelido, email, senha_hash, cargo, ativo, created_at, updated_at
		FROM usuarios
		ORDER BY nome_completo
	`)
	if err != nil {
		return nil, fmt.Errorf("querying usuarios: %w", err)
	}
	defer rows.Close()

	var result []domain.Usuario
	for rows.Next() {
		u, err := scanUsuarioRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning usuario: %w", err)
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

func (r *UsuarioRepository) Criar(ctx context.Context, req *domain.CriarUsuarioRequest, senhaHash string) (*domain.Usuario, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO usuarios (id, nome_completo, apelido, email, senha_hash, cargo)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, nome_completo, apelido, email, senha_hash, cargo, ativo, created_at, updated_at
	`, uuid.New(), req.NomeCompleto, req.Apelido, req.Email, senhaHash, req.Cargo)

	u, err := scanUsuario(row)
	if err != nil {
		return nil, fmt.Errorf("creating usuario: %w", err)
	}
	return &u, nil
}

func (r *UsuarioRepository) Atualizar(ctx context.Context, id uuid.UUID, req *domain.AtualizarUsuarioRequest) (*domain.Usuario, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE usuarios
		SET nome_completo = COALESCE($2, nome_completo),
		    apelido = COALESCE($3, apelido),
		    email = COALESCE($4, email),
		    cargo = COALESCE($5, cargo),
		    ativo = COALESCE($6, ativo),
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, nome_completo, apelido, email, senha_hash, cargo, ativo, created_at, updated_at
	`, id, req.NomeCompleto, req.Apelido, req.Email, req.Cargo, req.Ativo)

	u, err := scanUsuario(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("updating usuario %s: %w", id, err)
	}
	return &u, nil
}

func (r *UsuarioRepository) AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE usuarios
		SET senha_hash = $2, updated_at = NOW()
		WHERE id = $1
	`, id, senhaHash)
	if err != nil {
		return fmt.Errorf("updating senha for usuario %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("usuario %s not found", id)
	}
	return nil
}

func (r *UsuarioRepository) ListarProjetos(ctx context.Context, usuarioID uuid.UUID) ([]domain.ProjetoResumo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.chave, p.nome
		FROM projetos p
		INNER JOIN usuario_projetos up ON up.projeto_id = p.id
		WHERE up.usuario_id = $1
		ORDER BY p.nome
	`, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("querying projetos for usuario %s: %w", usuarioID, err)
	}
	defer rows.Close()

	var result []domain.ProjetoResumo
	for rows.Next() {
		var p domain.ProjetoResumo
		if err := rows.Scan(&p.ID, &p.Chave, &p.Nome); err != nil {
			return nil, fmt.Errorf("scanning projeto resumo: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *UsuarioRepository) AtualizarProjetos(ctx context.Context, usuarioID uuid.UUID, projetoIDs []uuid.UUID) ([]domain.ProjetoResumo, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `DELETE FROM usuario_projetos WHERE usuario_id = $1`, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("deleting existing projetos: %w", err)
	}

	for _, pid := range projetoIDs {
		_, err = tx.Exec(ctx, `
			INSERT INTO usuario_projetos (usuario_id, projeto_id) VALUES ($1, $2)
		`, usuarioID, pid)
		if err != nil {
			return nil, fmt.Errorf("inserting projeto %s: %w", pid, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return r.ListarProjetos(ctx, usuarioID)
}

func (r *UsuarioRepository) BuscarProjetoIDsPorUsuario(ctx context.Context, usuarioID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT projeto_id FROM usuario_projetos WHERE usuario_id = $1
	`, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("querying projeto_ids for usuario %s: %w", usuarioID, err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning projeto_id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func scanUsuario(row pgx.Row) (domain.Usuario, error) {
	var u domain.Usuario
	err := row.Scan(
		&u.ID, &u.NomeCompleto, &u.Apelido, &u.Email, &u.SenhaHash,
		&u.Cargo, &u.Ativo, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

func scanUsuarioRows(rows pgx.Rows) (domain.Usuario, error) {
	var u domain.Usuario
	err := rows.Scan(
		&u.ID, &u.NomeCompleto, &u.Apelido, &u.Email, &u.SenhaHash,
		&u.Cargo, &u.Ativo, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd backend && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 3: Commit**

```bash
git add backend/internal/repository/usuario.go
git commit -m "feat(iam): add usuario repository with CRUD and alçada methods"
```

---

### Task 5: Middleware — AuthJWT + ProjetoFilter

**Files:**
- Create: `backend/internal/middleware/auth.go`
- Create: `backend/internal/middleware/projeto_filter.go`
- Create: `backend/internal/middleware/auth_test.go`
- Create: `backend/internal/middleware/projeto_filter_test.go`

**Interfaces:**
- Consumes:
  - `auth.TokenService` (from Task 3) — `ValidateToken(tokenString string) (*Claims, error)`
  - `auth.Claims` (from Task 3) — has `Subject string` (via `RegisteredClaims`), `Email string`, `Cargo string`
- Produces:
  - `middleware.AuthJWT(tokenService *auth.TokenService) func(http.Handler) http.Handler` — used by Task 8
  - `middleware.ProjetoFilter(fetcher ProjetoIDsFetcher) func(http.Handler) http.Handler` — used by Task 8
  - `middleware.ProjetoIDsFetcher` interface — `BuscarProjetoIDsPorUsuario(ctx, uuid.UUID) ([]uuid.UUID, error)`
  - `middleware.UserIDFromContext(ctx) uuid.UUID` — used by Tasks 6, 7
  - `middleware.ProjetoIDsFromContext(ctx) []uuid.UUID` — used by future modules

- [ ] **Step 1: Write failing tests `backend/internal/middleware/auth_test.go`**

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/auth"
)

func TestAuthJWT_ValidToken(t *testing.T) {
	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	userID := uuid.New()
	token, _ := ts.GenerateToken(userID, "test@example.com", "coordenador")

	var capturedUserID uuid.UUID
	handler := AuthJWT(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if capturedUserID != userID {
		t.Errorf("userID = %s, want %s", capturedUserID, userID)
	}
}

func TestAuthJWT_MissingToken(t *testing.T) {
	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	handler := AuthJWT(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAuthJWT_InvalidToken(t *testing.T) {
	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	handler := AuthJWT(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAuthJWT_WrongScheme(t *testing.T) {
	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	token, _ := ts.GenerateToken(uuid.New(), "test@example.com", "gerente")

	handler := AuthJWT(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd backend && go test ./internal/middleware/...`
Expected: FAIL (package doesn't exist yet)

- [ ] **Step 3: Implement `backend/internal/middleware/auth.go`**

```go
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/auth"
)

type contextKey string

const (
	userIDKey    contextKey = "user_id"
	userEmailKey contextKey = "user_email"
	userCargoKey contextKey = "user_cargo"
)

func AuthJWT(tokenService *auth.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				respondUnauthorized(w, "token não fornecido")
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				respondUnauthorized(w, "formato de authorization inválido")
				return
			}

			claims, err := tokenService.ValidateToken(parts[1])
			if err != nil {
				respondUnauthorized(w, "token inválido ou expirado")
				return
			}

			userID, err := uuid.Parse(claims.Subject)
			if err != nil {
				respondUnauthorized(w, "token inválido")
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, userID)
			ctx = context.WithValue(ctx, userEmailKey, claims.Email)
			ctx = context.WithValue(ctx, userCargoKey, claims.Cargo)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserIDFromContext(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(userIDKey).(uuid.UUID)
	return id
}

func UserEmailFromContext(ctx context.Context) string {
	email, _ := ctx.Value(userEmailKey).(string)
	return email
}

func UserCargoFromContext(ctx context.Context) string {
	cargo, _ := ctx.Value(userCargoKey).(string)
	return cargo
}

func respondUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
```

- [ ] **Step 4: Run auth middleware tests**

Run: `cd backend && go test ./internal/middleware/... -v -run TestAuthJWT`
Expected: PASS (all 4 tests)

- [ ] **Step 5: Write failing tests `backend/internal/middleware/projeto_filter_test.go`**

```go
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type mockFetcher struct {
	ids []uuid.UUID
	err error
}

func (m *mockFetcher) BuscarProjetoIDsPorUsuario(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	return m.ids, m.err
}

func TestProjetoFilter_InjectsIDs(t *testing.T) {
	projetoIDs := []uuid.UUID{uuid.New(), uuid.New()}
	fetcher := &mockFetcher{ids: projetoIDs}
	userID := uuid.New()

	var capturedIDs []uuid.UUID
	handler := ProjetoFilter(fetcher)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIDs = ProjetoIDsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), userIDKey, userID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if len(capturedIDs) != len(projetoIDs) {
		t.Errorf("got %d projeto IDs, want %d", len(capturedIDs), len(projetoIDs))
	}
}

func TestProjetoFilter_EmptyAlcada(t *testing.T) {
	fetcher := &mockFetcher{ids: nil}
	userID := uuid.New()

	var capturedIDs []uuid.UUID
	handler := ProjetoFilter(fetcher)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIDs = ProjetoIDsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), userIDKey, userID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d — empty alçada must not block", rr.Code, http.StatusOK)
	}
	if capturedIDs != nil {
		t.Errorf("expected nil projeto IDs for empty alçada, got %v", capturedIDs)
	}
}

func TestProjetoFilter_FetcherError(t *testing.T) {
	fetcher := &mockFetcher{err: fmt.Errorf("db connection failed")}
	userID := uuid.New()

	handler := ProjetoFilter(fetcher)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called on fetcher error")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), userIDKey, userID)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}
```

- [ ] **Step 6: Implement `backend/internal/middleware/projeto_filter.go`**

```go
package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

const projetoIDsKey contextKey = "projeto_ids"

type ProjetoIDsFetcher interface {
	BuscarProjetoIDsPorUsuario(ctx context.Context, usuarioID uuid.UUID) ([]uuid.UUID, error)
}

func ProjetoFilter(fetcher ProjetoIDsFetcher) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := UserIDFromContext(r.Context())
			if userID == uuid.Nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "usuário não autenticado"})
				return
			}

			ids, err := fetcher.BuscarProjetoIDsPorUsuario(r.Context(), userID)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "falha ao buscar alçada"})
				return
			}

			ctx := context.WithValue(r.Context(), projetoIDsKey, ids)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ProjetoIDsFromContext(ctx context.Context) []uuid.UUID {
	ids, _ := ctx.Value(projetoIDsKey).([]uuid.UUID)
	return ids
}
```

- [ ] **Step 7: Run all middleware tests**

Run: `cd backend && go test ./internal/middleware/... -v`
Expected: PASS (all 7 tests)

- [ ] **Step 8: Commit**

```bash
git add backend/internal/middleware/auth.go backend/internal/middleware/auth_test.go \
       backend/internal/middleware/projeto_filter.go backend/internal/middleware/projeto_filter_test.go
git commit -m "feat(iam): add AuthJWT and ProjetoFilter middleware with tests"
```

---

### Task 6: Auth Handler — Login

**Files:**
- Create: `backend/internal/handler/auth.go`
- Create: `backend/internal/handler/auth_test.go`

**Interfaces:**
- Consumes:
  - `domain.LoginRequest` (from Task 1) — fields: `Email string`, `Senha string`
  - `domain.LoginResponse` (from Task 1) — fields: `Token string`, `Usuario domain.Usuario`
  - `domain.Usuario` (from Task 1) — field `SenhaHash string` (json:"-")
  - `auth.TokenService` (from Task 3) — `GenerateToken(userID uuid.UUID, email, cargo string) (string, error)`
  - `respondJSON`, `respondError` from `handler/response.go`
- Produces:
  - `handler.AuthHandler` struct — used by Task 8
  - `handler.NewAuthHandler(store UsuarioStore, tokenService *auth.TokenService, logger *zap.Logger) *AuthHandler`
  - `(*AuthHandler).Login(w http.ResponseWriter, r *http.Request)` — used by Task 8

**Note:** `UsuarioStore` interface is defined in Task 7 (`handler/usuario.go`). Since both files are in the same `handler` package, the interface is visible to `auth.go`. Task 7 must be implemented before or together with this task for compilation. **Alternative:** the implementer may define `UsuarioStore` in `handler/usuario.go` first (just the interface, no handler), then implement this task. OR: define the interface in this task's file temporarily and move it to `handler/usuario.go` in Task 7. The recommended approach: **define `UsuarioStore` interface in this task's `handler/auth.go`** since auth needs it first. Task 7's `handler/usuario.go` uses the same interface from the same package — no duplication needed.

- [ ] **Step 1: Write failing test `backend/internal/handler/auth_test.go`**

```go
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/auth"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"go.uber.org/zap"
)

type mockUsuarioStore struct {
	buscarPorEmailFn        func(ctx context.Context, email string) (*domain.Usuario, error)
	buscarPorIDFn           func(ctx context.Context, id uuid.UUID) (*domain.Usuario, error)
	listarTodosFn           func(ctx context.Context) ([]domain.Usuario, error)
	criarFn                 func(ctx context.Context, req *domain.CriarUsuarioRequest, senhaHash string) (*domain.Usuario, error)
	atualizarFn             func(ctx context.Context, id uuid.UUID, req *domain.AtualizarUsuarioRequest) (*domain.Usuario, error)
	atualizarSenhaFn        func(ctx context.Context, id uuid.UUID, senhaHash string) error
	listarProjetosFn        func(ctx context.Context, usuarioID uuid.UUID) ([]domain.ProjetoResumo, error)
	atualizarProjetosFn     func(ctx context.Context, usuarioID uuid.UUID, projetoIDs []uuid.UUID) ([]domain.ProjetoResumo, error)
	buscarProjetoIDsFn      func(ctx context.Context, usuarioID uuid.UUID) ([]uuid.UUID, error)
}

func (m *mockUsuarioStore) BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error) {
	return m.buscarPorEmailFn(ctx, email)
}
func (m *mockUsuarioStore) BuscarPorID(ctx context.Context, id uuid.UUID) (*domain.Usuario, error) {
	return m.buscarPorIDFn(ctx, id)
}
func (m *mockUsuarioStore) ListarTodos(ctx context.Context) ([]domain.Usuario, error) {
	return m.listarTodosFn(ctx)
}
func (m *mockUsuarioStore) Criar(ctx context.Context, req *domain.CriarUsuarioRequest, senhaHash string) (*domain.Usuario, error) {
	return m.criarFn(ctx, req, senhaHash)
}
func (m *mockUsuarioStore) Atualizar(ctx context.Context, id uuid.UUID, req *domain.AtualizarUsuarioRequest) (*domain.Usuario, error) {
	return m.atualizarFn(ctx, id, req)
}
func (m *mockUsuarioStore) AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error {
	return m.atualizarSenhaFn(ctx, id, senhaHash)
}
func (m *mockUsuarioStore) ListarProjetos(ctx context.Context, usuarioID uuid.UUID) ([]domain.ProjetoResumo, error) {
	return m.listarProjetosFn(ctx, usuarioID)
}
func (m *mockUsuarioStore) AtualizarProjetos(ctx context.Context, usuarioID uuid.UUID, projetoIDs []uuid.UUID) ([]domain.ProjetoResumo, error) {
	return m.atualizarProjetosFn(ctx, usuarioID, projetoIDs)
}
func (m *mockUsuarioStore) BuscarProjetoIDsPorUsuario(ctx context.Context, usuarioID uuid.UUID) ([]uuid.UUID, error) {
	return m.buscarProjetoIDsFn(ctx, usuarioID)
}

func TestLogin_Success(t *testing.T) {
	userID := uuid.New()
	// Hash of "Totvs@123" with cost 12
	senhaHash := "$2a$12$YD27E7brWZvrrq0lVpbsouDUIi3UiwgjT6NsiIOQGPzwDBlvC5DYK"

	store := &mockUsuarioStore{
		buscarPorEmailFn: func(_ context.Context, email string) (*domain.Usuario, error) {
			return &domain.Usuario{
				ID:           userID,
				NomeCompleto: "Administrador",
				Apelido:      "admin",
				Email:        email,
				SenhaHash:    senhaHash,
				Cargo:        "coordenador",
				Ativo:        true,
			}, nil
		},
	}

	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	logger := zap.NewNop()
	h := NewAuthHandler(store, ts, logger)

	body := `{"email":"admin@tcloud.local","senha":"Totvs@123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp domain.LoginResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.Usuario.ID != userID {
		t.Errorf("usuario.id = %s, want %s", resp.Usuario.ID, userID)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	senhaHash := "$2a$12$YD27E7brWZvrrq0lVpbsouDUIi3UiwgjT6NsiIOQGPzwDBlvC5DYK"

	store := &mockUsuarioStore{
		buscarPorEmailFn: func(_ context.Context, _ string) (*domain.Usuario, error) {
			return &domain.Usuario{SenhaHash: senhaHash, Ativo: true}, nil
		},
	}

	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	h := NewAuthHandler(store, ts, zap.NewNop())

	body := `{"email":"admin@tcloud.local","senha":"wrong-password"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	store := &mockUsuarioStore{
		buscarPorEmailFn: func(_ context.Context, _ string) (*domain.Usuario, error) {
			return nil, nil
		},
	}

	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	h := NewAuthHandler(store, ts, zap.NewNop())

	body := `{"email":"nobody@example.com","senha":"whatever"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestLogin_EmptyBody(t *testing.T) {
	store := &mockUsuarioStore{}
	ts := auth.NewTokenService("test-secret-key-minimum-32-chars!!", 24)
	h := NewAuthHandler(store, ts, zap.NewNop())

	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString("{}"))
	rr := httptest.NewRecorder()

	h.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/handler/... -run TestLogin`
Expected: FAIL (NewAuthHandler not defined)

- [ ] **Step 3: Implement `backend/internal/handler/auth.go`**

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/auth"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type UsuarioStore interface {
	BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error)
	BuscarPorID(ctx context.Context, id uuid.UUID) (*domain.Usuario, error)
	ListarTodos(ctx context.Context) ([]domain.Usuario, error)
	Criar(ctx context.Context, req *domain.CriarUsuarioRequest, senhaHash string) (*domain.Usuario, error)
	Atualizar(ctx context.Context, id uuid.UUID, req *domain.AtualizarUsuarioRequest) (*domain.Usuario, error)
	AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error
	ListarProjetos(ctx context.Context, usuarioID uuid.UUID) ([]domain.ProjetoResumo, error)
	AtualizarProjetos(ctx context.Context, usuarioID uuid.UUID, projetoIDs []uuid.UUID) ([]domain.ProjetoResumo, error)
	BuscarProjetoIDsPorUsuario(ctx context.Context, usuarioID uuid.UUID) ([]uuid.UUID, error)
}

type AuthHandler struct {
	store        UsuarioStore
	tokenService *auth.TokenService
	logger       *zap.Logger
}

func NewAuthHandler(store UsuarioStore, tokenService *auth.TokenService, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		store:        store,
		tokenService: tokenService,
		logger:       logger,
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req domain.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	if req.Email == "" || req.Senha == "" {
		respondError(w, http.StatusBadRequest, "email e senha são obrigatórios")
		return
	}

	usuario, err := h.store.BuscarPorEmail(r.Context(), req.Email)
	if err != nil {
		h.logger.Error("failed to find usuario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro interno")
		return
	}
	if usuario == nil {
		respondError(w, http.StatusUnauthorized, "credenciais inválidas")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(usuario.SenhaHash), []byte(req.Senha)); err != nil {
		respondError(w, http.StatusUnauthorized, "credenciais inválidas")
		return
	}

	token, err := h.tokenService.GenerateToken(usuario.ID, usuario.Email, usuario.Cargo)
	if err != nil {
		h.logger.Error("failed to generate token", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro ao gerar token")
		return
	}

	respondJSON(w, http.StatusOK, domain.LoginResponse{
		Token:   token,
		Usuario: *usuario,
	})
}
```

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./internal/handler/... -v -run TestLogin`
Expected: PASS (all 4 tests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/auth.go backend/internal/handler/auth_test.go
git commit -m "feat(iam): add login handler with JWT token generation"
```

---

### Task 7: Usuario Handler — CRUD + Alçada Endpoints

**Files:**
- Create: `backend/internal/handler/usuario.go`
- Create: `backend/internal/handler/usuario_test.go`

**Interfaces:**
- Consumes:
  - `UsuarioStore` interface (from Task 6, same package)
  - `domain.CriarUsuarioRequest` (from Task 1)
  - `domain.AtualizarUsuarioRequest` (from Task 1)
  - `domain.AlterarSenhaRequest` (from Task 1)
  - `domain.AlcadaProjetosRequest` (from Task 1)
  - `domain.ProjetoResumo` (from Task 1)
  - `middleware.UserIDFromContext` (from Task 5)
  - `respondJSON`, `respondError` from `handler/response.go`
- Produces:
  - `handler.UsuarioHandler` struct — used by Task 8
  - `handler.NewUsuarioHandler(store UsuarioStore, logger *zap.Logger) *UsuarioHandler`
  - `(*UsuarioHandler).List(w, r)` — GET /api/v1/usuarios
  - `(*UsuarioHandler).Create(w, r)` — POST /api/v1/usuarios
  - `(*UsuarioHandler).GetByID(w, r)` — GET /api/v1/usuarios/{id}
  - `(*UsuarioHandler).Update(w, r)` — PUT /api/v1/usuarios/{id}
  - `(*UsuarioHandler).AlterarSenha(w, r)` — PUT /api/v1/usuarios/{id}/senha
  - `(*UsuarioHandler).ListProjetos(w, r)` — GET /api/v1/usuarios/{id}/projetos
  - `(*UsuarioHandler).UpdateProjetos(w, r)` — PUT /api/v1/usuarios/{id}/projetos

- [ ] **Step 1: Write failing test `backend/internal/handler/usuario_test.go`**

```go
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"go.uber.org/zap"
)

func TestUsuarioHandler_List(t *testing.T) {
	store := &mockUsuarioStore{
		listarTodosFn: func(_ context.Context) ([]domain.Usuario, error) {
			return []domain.Usuario{
				{ID: uuid.New(), NomeCompleto: "Admin", Apelido: "admin", Email: "admin@tcloud.local", Cargo: "coordenador", Ativo: true},
			}, nil
		},
	}

	h := NewUsuarioHandler(store, zap.NewNop())
	req := httptest.NewRequest("GET", "/api/v1/usuarios", nil)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp map[string][]domain.Usuario
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp["usuarios"]) != 1 {
		t.Errorf("got %d usuarios, want 1", len(resp["usuarios"]))
	}
}

func TestUsuarioHandler_Create(t *testing.T) {
	createdID := uuid.New()
	store := &mockUsuarioStore{
		criarFn: func(_ context.Context, req *domain.CriarUsuarioRequest, senhaHash string) (*domain.Usuario, error) {
			return &domain.Usuario{
				ID:           createdID,
				NomeCompleto: req.NomeCompleto,
				Apelido:      req.Apelido,
				Email:        req.Email,
				Cargo:        req.Cargo,
				Ativo:        true,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}, nil
		},
	}

	h := NewUsuarioHandler(store, zap.NewNop())
	body := `{"nome_completo":"João Silva","apelido":"joao","email":"joao@totvs.com","senha":"MinhaS3nh@","cargo":"gerente"}`
	req := httptest.NewRequest("POST", "/api/v1/usuarios", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}
}

func TestUsuarioHandler_Create_InvalidCargo(t *testing.T) {
	store := &mockUsuarioStore{}
	h := NewUsuarioHandler(store, zap.NewNop())

	body := `{"nome_completo":"João","apelido":"joao","email":"joao@totvs.com","senha":"MinhaS3nh@","cargo":"diretor"}`
	req := httptest.NewRequest("POST", "/api/v1/usuarios", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUsuarioHandler_Create_ShortPassword(t *testing.T) {
	store := &mockUsuarioStore{}
	h := NewUsuarioHandler(store, zap.NewNop())

	body := `{"nome_completo":"João","apelido":"joao","email":"joao@totvs.com","senha":"123","cargo":"gerente"}`
	req := httptest.NewRequest("POST", "/api/v1/usuarios", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestUsuarioHandler_GetByID(t *testing.T) {
	userID := uuid.New()
	store := &mockUsuarioStore{
		buscarPorIDFn: func(_ context.Context, id uuid.UUID) (*domain.Usuario, error) {
			return &domain.Usuario{
				ID:           id,
				NomeCompleto: "Admin",
				Apelido:      "admin",
				Email:        "admin@tcloud.local",
				Cargo:        "coordenador",
				Ativo:        true,
			}, nil
		},
	}

	h := NewUsuarioHandler(store, zap.NewNop())

	r := chi.NewRouter()
	r.Get("/api/v1/usuarios/{id}", h.GetByID)

	req := httptest.NewRequest("GET", "/api/v1/usuarios/"+userID.String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestUsuarioHandler_AlterarSenha_WrongCurrent(t *testing.T) {
	store := &mockUsuarioStore{
		buscarPorIDFn: func(_ context.Context, _ uuid.UUID) (*domain.Usuario, error) {
			return &domain.Usuario{
				ID:        uuid.New(),
				SenhaHash: "$2a$12$YD27E7brWZvrrq0lVpbsouDUIi3UiwgjT6NsiIOQGPzwDBlvC5DYK",
			}, nil
		},
	}

	h := NewUsuarioHandler(store, zap.NewNop())

	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}/senha", h.AlterarSenha)

	body := `{"senha_atual":"wrong","nova_senha":"NewPass123"}`
	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+uuid.New().String()+"/senha", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestUsuarioHandler_UpdateProjetos(t *testing.T) {
	projetoID := uuid.New()
	store := &mockUsuarioStore{
		atualizarProjetosFn: func(_ context.Context, _ uuid.UUID, ids []uuid.UUID) ([]domain.ProjetoResumo, error) {
			return []domain.ProjetoResumo{
				{ID: ids[0], Chave: "BACK", Nome: "Backend"},
			}, nil
		},
	}

	h := NewUsuarioHandler(store, zap.NewNop())

	r := chi.NewRouter()
	r.Put("/api/v1/usuarios/{id}/projetos", h.UpdateProjetos)

	body, _ := json.Marshal(domain.AlcadaProjetosRequest{ProjetoIDs: []uuid.UUID{projetoID}})
	req := httptest.NewRequest("PUT", "/api/v1/usuarios/"+uuid.New().String()+"/projetos", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/handler/... -run TestUsuarioHandler`
Expected: FAIL (NewUsuarioHandler not defined)

- [ ] **Step 3: Implement `backend/internal/handler/usuario.go`**

```go
package handler

import (
	"encoding/json"
	"net/http"
	"net/mail"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

var cargosValidos = map[string]bool{
	"coordenador":      true,
	"gerente":          true,
	"gerente_projetos": true,
}

type UsuarioHandler struct {
	store  UsuarioStore
	logger *zap.Logger
}

func NewUsuarioHandler(store UsuarioStore, logger *zap.Logger) *UsuarioHandler {
	return &UsuarioHandler{store: store, logger: logger}
}

func (h *UsuarioHandler) List(w http.ResponseWriter, r *http.Request) {
	usuarios, err := h.store.ListarTodos(r.Context())
	if err != nil {
		h.logger.Error("failed to list usuarios", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar usuários")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"usuarios": usuarios})
}

func (h *UsuarioHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req domain.CriarUsuarioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	if req.NomeCompleto == "" {
		respondError(w, http.StatusBadRequest, "nome_completo é obrigatório")
		return
	}
	if req.Apelido == "" || len(req.Apelido) > 50 {
		respondError(w, http.StatusBadRequest, "apelido é obrigatório (max 50 chars)")
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil || req.Email == "" {
		respondError(w, http.StatusBadRequest, "email inválido")
		return
	}
	if len(req.Senha) < 8 {
		respondError(w, http.StatusBadRequest, "senha deve ter no mínimo 8 caracteres")
		return
	}
	if !cargosValidos[req.Cargo] {
		respondError(w, http.StatusBadRequest, "cargo inválido: deve ser coordenador, gerente ou gerente_projetos")
		return
	}

	senhaHash, err := bcrypt.GenerateFromPassword([]byte(req.Senha), 12)
	if err != nil {
		h.logger.Error("failed to hash password", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro interno")
		return
	}

	usuario, err := h.store.Criar(r.Context(), &req, string(senhaHash))
	if err != nil {
		h.logger.Error("failed to create usuario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao criar usuário")
		return
	}

	respondJSON(w, http.StatusCreated, usuario)
}

func (h *UsuarioHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	usuario, err := h.store.BuscarPorID(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get usuario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao buscar usuário")
		return
	}
	if usuario == nil {
		respondError(w, http.StatusNotFound, "usuário não encontrado")
		return
	}

	respondJSON(w, http.StatusOK, usuario)
}

func (h *UsuarioHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req domain.AtualizarUsuarioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	if req.Apelido != nil && (len(*req.Apelido) == 0 || len(*req.Apelido) > 50) {
		respondError(w, http.StatusBadRequest, "apelido inválido (1-50 chars)")
		return
	}
	if req.Email != nil {
		if _, err := mail.ParseAddress(*req.Email); err != nil {
			respondError(w, http.StatusBadRequest, "email inválido")
			return
		}
	}
	if req.Cargo != nil && !cargosValidos[*req.Cargo] {
		respondError(w, http.StatusBadRequest, "cargo inválido: deve ser coordenador, gerente ou gerente_projetos")
		return
	}

	usuario, err := h.store.Atualizar(r.Context(), id, &req)
	if err != nil {
		h.logger.Error("failed to update usuario", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar usuário")
		return
	}
	if usuario == nil {
		respondError(w, http.StatusNotFound, "usuário não encontrado")
		return
	}

	respondJSON(w, http.StatusOK, usuario)
}

func (h *UsuarioHandler) AlterarSenha(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req domain.AlterarSenhaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	if len(req.NovaSenha) < 8 {
		respondError(w, http.StatusBadRequest, "nova senha deve ter no mínimo 8 caracteres")
		return
	}

	usuario, err := h.store.BuscarPorID(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get usuario for password change", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro interno")
		return
	}
	if usuario == nil {
		respondError(w, http.StatusNotFound, "usuário não encontrado")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(usuario.SenhaHash), []byte(req.SenhaAtual)); err != nil {
		respondError(w, http.StatusUnauthorized, "senha atual incorreta")
		return
	}

	novaHash, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), 12)
	if err != nil {
		h.logger.Error("failed to hash new password", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "erro interno")
		return
	}

	if err := h.store.AtualizarSenha(r.Context(), id, string(novaHash)); err != nil {
		h.logger.Error("failed to update password", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao alterar senha")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "senha alterada"})
}

func (h *UsuarioHandler) ListProjetos(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	projetos, err := h.store.ListarProjetos(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to list projetos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao listar projetos")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"projetos": projetos})
}

func (h *UsuarioHandler) UpdateProjetos(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "id inválido")
		return
	}

	var req domain.AlcadaProjetosRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return
	}

	projetos, err := h.store.AtualizarProjetos(r.Context(), id, req.ProjetoIDs)
	if err != nil {
		h.logger.Error("failed to update projetos", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "falha ao atualizar projetos")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"projetos": projetos})
}
```

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./internal/handler/... -v`
Expected: PASS (all Login + UsuarioHandler tests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/usuario.go backend/internal/handler/usuario_test.go
git commit -m "feat(iam): add usuario handler with CRUD and alçada endpoints"
```

---

### Task 8: Wire Routes in main.go

**Files:**
- Modify: `backend/cmd/api/main.go`

**Interfaces:**
- Consumes:
  - `config.AuthConfig` (from Task 1) — fields: `JWTSecret string`, `JWTExpirationHours int`
  - `auth.NewTokenService(secret string, expirationHours int) *TokenService` (from Task 3)
  - `repository.NewUsuarioRepository(pool *pgxpool.Pool) *UsuarioRepository` (from Task 4)
  - `middleware.AuthJWT(tokenService *auth.TokenService) func(http.Handler) http.Handler` (from Task 5)
  - `middleware.ProjetoFilter(fetcher ProjetoIDsFetcher) func(http.Handler) http.Handler` (from Task 5)
  - `handler.NewAuthHandler(store UsuarioStore, tokenService *auth.TokenService, logger *zap.Logger) *AuthHandler` (from Task 6)
  - `handler.NewUsuarioHandler(store UsuarioStore, logger *zap.Logger) *UsuarioHandler` (from Task 7)
- Produces: fully wired application with auth middleware

- [ ] **Step 1: Modify `backend/cmd/api/main.go`**

Replace the full file content with:

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/totvs/tcloud-planner/backend/internal/auth"
	"github.com/totvs/tcloud-planner/backend/internal/config"
	"github.com/totvs/tcloud-planner/backend/internal/handler"
	"github.com/totvs/tcloud-planner/backend/internal/middleware"
	"github.com/totvs/tcloud-planner/backend/internal/repository"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	if cfg.Auth.JWTSecret == "" {
		fmt.Fprintf(os.Stderr, "JWT_SECRET is required\n")
		os.Exit(1)
	}

	var logger *zap.Logger
	if cfg.Log.Level == "debug" {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DB.DSN())
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Fatal("failed to ping database", zap.Error(err))
	}
	logger.Info("connected to database")

	tokenService := auth.NewTokenService(cfg.Auth.JWTSecret, cfg.Auth.JWTExpirationHours)

	fonteDadosRepo := repository.NewFonteDadosRepository(pool)
	usuarioRepo := repository.NewUsuarioRepository(pool)

	fonteDadosHandler := handler.NewFonteDadosHandler(fonteDadosRepo, logger)
	authHandler := handler.NewAuthHandler(usuarioRepo, tokenService, logger)
	usuarioHandler := handler.NewUsuarioHandler(usuarioRepo, logger)

	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", authHandler.Login)

		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthJWT(tokenService))
			r.Use(middleware.ProjetoFilter(usuarioRepo))

			r.Get("/fontes", fonteDadosHandler.List)
			r.Post("/fontes", fonteDadosHandler.Create)
			r.Get("/fontes/{id}", fonteDadosHandler.GetByID)
			r.Put("/fontes/{id}", fonteDadosHandler.Update)
			r.Delete("/fontes/{id}", fonteDadosHandler.Delete)

			r.Get("/usuarios", usuarioHandler.List)
			r.Post("/usuarios", usuarioHandler.Create)
			r.Get("/usuarios/{id}", usuarioHandler.GetByID)
			r.Put("/usuarios/{id}", usuarioHandler.Update)
			r.Put("/usuarios/{id}/senha", usuarioHandler.AlterarSenha)
			r.Get("/usuarios/{id}/projetos", usuarioHandler.ListProjetos)
			r.Put("/usuarios/{id}/projetos", usuarioHandler.UpdateProjetos)
		})
	})

	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("starting server", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Info("shutting down server", zap.String("signal", sig.String()))

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Fatal("server forced to shutdown", zap.Error(err))
	}

	logger.Info("server stopped")
}
```

**Key changes from original:**
- Import `auth`, `middleware` packages
- JWT_SECRET validation on startup
- Create `tokenService`, `usuarioRepo`, `authHandler`, `usuarioHandler`
- Move fontes routes inside authenticated group
- Add `/auth/login` as public route
- Add all `/usuarios` routes inside authenticated group
- Rename chi middleware import to `chimw` to avoid conflict with our `middleware` package

- [ ] **Step 2: Verify compilation**

Run: `cd backend && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 3: Run all tests**

Run: `cd backend && go test ./... -v`
Expected: PASS (all auth, middleware, and handler tests)

- [ ] **Step 4: Commit**

```bash
git add backend/cmd/api/main.go backend/go.mod backend/go.sum
git commit -m "feat(iam): wire auth middleware and IAM routes in main.go"
```

---

## Self-Review Checklist

### 1. Spec Coverage
| Spec Requirement | Task |
|---|---|
| Tabela `usuarios` com campos especificados | Task 2 |
| Tabela `usuario_projetos` (N:N) | Task 2 |
| Seed admin (apelido=admin, senha=Totvs@123, cargo=coordenador) | Task 2 |
| Admin email configurável via `ADMIN_EMAIL` env | Task 1 (config) |
| Login via email+senha → JWT | Task 6 |
| JWT HMAC-SHA256 com `JWT_SECRET` | Task 3 |
| JWT expiração 24h (configurável) | Task 1 + Task 3 |
| `GET /usuarios` | Task 7 |
| `POST /usuarios` com validações | Task 7 |
| `GET /usuarios/{id}` | Task 7 |
| `PUT /usuarios/{id}` parcial | Task 7 |
| `PUT /usuarios/{id}/senha` | Task 7 |
| `GET /usuarios/{id}/projetos` | Task 7 |
| `PUT /usuarios/{id}/projetos` replace | Task 7 |
| Middleware AuthJWT | Task 5 |
| Middleware ProjetoFilter | Task 5 |
| `SenhaHash` nunca exposta em responses (`json:"-"`) | Task 1 |
| Cargo metadata only (sem hierarquia) | Task 7 (validation only) |
| Routes excluídas do auth: login + health | Task 8 |
| `AuthConfig` em config.go | Task 1 |
| Dependências: bcrypt + jwt | Task 1 (go get) |
| Cadeia: CORS → Logger → Recoverer → RequestID → AuthJWT → ProjetoFilter → Handler | Task 8 |

### 2. Placeholder Scan
No TBDs, TODOs, or "implement later" found.

### 3. Type Consistency
- `UsuarioStore` interface defined in Task 6 (`handler/auth.go`), used by Tasks 6, 7, 8 — same package, consistent
- `domain.Usuario`, `domain.ProjetoResumo` — consistent across Tasks 1, 4, 6, 7
- `auth.TokenService` — consistent across Tasks 3, 5, 6, 8
- `middleware.UserIDFromContext`, `middleware.ProjetoIDsFromContext` — defined in Task 5, used by Task 7 and future modules
- `ProjetoIDsFetcher` interface in Task 5 — satisfied by `UsuarioRepository` from Task 4 (has `BuscarProjetoIDsPorUsuario`)
- `contextKey` type — private to middleware package, used consistently
