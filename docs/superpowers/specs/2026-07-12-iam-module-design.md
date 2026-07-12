# Módulo IAM — Design Spec

## Visão Geral

Sistema de autenticação e controle de acesso por projeto. Cadastro de usuários com cargo, login via email+senha com JWT stateless, e "Alçada de Projetos" que restringe visibilidade de dados em todos os módulos.

**Princípio:** Cargo (Coordenador, Gerente, Gerente de Projetos) é metadata informativa — sem hierarquia de permissões. Controle de acesso é exclusivamente pela alçada de projetos JIRA (tabela `projetos`) atribuída ao usuário.

## Requisitos Funcionais

### Cadastro de Usuários

Campos:
- **Nome completo** (obrigatório, max 255 chars)
- **Apelido** (obrigatório, único, max 50 chars) — identificação visual, não usado pra login
- **Email** (obrigatório, único, formato válido) — usado pra login
- **Senha** (obrigatório, mínimo 8 chars) — armazenada como bcrypt hash (cost 12)
- **Cargo** (obrigatório): `coordenador`, `gerente` ou `gerente_projetos`
- **Ativo** (boolean, default true)

Qualquer usuário logado pode criar/editar outros usuários.

### Usuário Admin (Seed)

Primeiro usuário da aplicação, criado via migration:
- Nome completo: "Administrador"
- Apelido: "admin"
- Email: configurável via env `ADMIN_EMAIL` (default: `admin@tcloud.local`)
- Senha: `Totvs@123` (bcrypt hash gerado na migration)
- Cargo: `coordenador`
- Recebe acesso a todos os projetos existentes na base

### Login

Login via email + senha. Retorna JWT token stateless.

- Assinatura: HMAC-SHA256 com secret via env `JWT_SECRET`
- Expiração: 24h (configurável via `JWT_EXPIRATION_HOURS`)
- Sem refresh token (v1). Re-login quando expirar.

### Alçada de Projetos

Menu que permite cadastrar quais projetos JIRA (tabela `projetos`) um usuário pode visualizar. Controle N:N — um usuário pode ter acesso a múltiplos projetos, e um projeto pode ser visível para múltiplos usuários.

**Sem alçada cadastrada = sem acesso a dados.** Exceção: admin seed recebe todos projetos automaticamente.

### Filtro de Visibilidade

Middleware que aplica automaticamente a alçada do usuário em todos os módulos:

| Módulo | Como filtra |
|--------|------------|
| Equipe | Só mostra membros/tarefas de projetos na alçada |
| Pessoas | Só mostra pessoas que têm tarefas em projetos na alçada |
| Timeline Capacidade | Só mostra épicos de projetos na alçada |
| Dashboard | Métricas calculadas apenas sobre projetos na alçada |
| Fonte de Dados | Sem filtro (configuração do sistema) |

Aplicação nas queries via cláusula `WHERE projeto_id = ANY($1)` onde `$1` = projeto_ids da alçada do usuário.

## Schema

### Tabela `usuarios`

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
```

### Tabela `usuario_projetos`

```sql
CREATE TABLE usuario_projetos (
    usuario_id  UUID NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    projeto_id  UUID NOT NULL REFERENCES projetos(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (usuario_id, projeto_id)
);

CREATE INDEX idx_usuario_projetos_usuario ON usuario_projetos(usuario_id);
```

### Seed Admin

```sql
INSERT INTO usuarios (nome_completo, apelido, email, senha_hash, cargo)
VALUES ('Administrador', 'admin', 'admin@tcloud.local', '$2a$12$...', 'coordenador');

INSERT INTO usuario_projetos (usuario_id, projeto_id)
SELECT u.id, p.id FROM usuarios u, projetos p WHERE u.apelido = 'admin';
```

Email padrão `admin@tcloud.local` configurável via env `ADMIN_EMAIL`. Hash bcrypt da senha `Totvs@123` pré-computado como constante na migration SQL (bcrypt hashes são determinísticos dado o salt — gerar hash uma vez com `bcrypt.GenerateFromPassword([]byte("Totvs@123"), 12)` e inserir o resultado literal na migration).

## API Endpoints

### POST /api/v1/auth/login

**Não requer autenticação.**

**Body:**
```json
{
  "email": "admin@tcloud.local",
  "senha": "Totvs@123"
}
```

**Response 200:**
```json
{
  "token": "eyJhbG...",
  "usuario": {
    "id": "uuid",
    "nome_completo": "Administrador",
    "apelido": "admin",
    "email": "admin@tcloud.local",
    "cargo": "coordenador"
  }
}
```

**Response 401:**
```json
{ "error": "credenciais inválidas" }
```

**JWT payload:**
```json
{
  "sub": "uuid-do-usuario",
  "email": "admin@tcloud.local",
  "cargo": "coordenador",
  "exp": 1752537600,
  "iat": 1752451200
}
```

### GET /api/v1/usuarios

**Response 200:**
```json
{
  "usuarios": [
    {
      "id": "uuid",
      "nome_completo": "Administrador",
      "apelido": "admin",
      "email": "admin@tcloud.local",
      "cargo": "coordenador",
      "ativo": true,
      "created_at": "2026-07-12T00:00:00Z"
    }
  ]
}
```

### POST /api/v1/usuarios

**Body:**
```json
{
  "nome_completo": "João Silva",
  "apelido": "joao",
  "email": "joao@totvs.com",
  "senha": "MinhaS3nh@",
  "cargo": "gerente"
}
```

**Validações:**
- `email`: formato válido, único no sistema
- `apelido`: único, max 50 chars
- `senha`: mínimo 8 caracteres
- `cargo`: deve ser `coordenador`, `gerente` ou `gerente_projetos`

**Response 201:** Usuário criado (sem `senha_hash` no response).

### GET /api/v1/usuarios/{id}

**Response 200:** Usuário com campos públicos.

### PUT /api/v1/usuarios/{id}

**Body (parcial aceito):**
```json
{
  "nome_completo": "João da Silva",
  "apelido": "joaosilva",
  "email": "joao.silva@totvs.com",
  "cargo": "gerente_projetos",
  "ativo": false
}
```

Mesmas validações do POST. Não altera senha (endpoint separado).

### PUT /api/v1/usuarios/{id}/senha

**Body:**
```json
{
  "senha_atual": "antiga123",
  "nova_senha": "Nova@456"
}
```

**Validações:**
- `senha_atual` deve coincidir com hash atual
- `nova_senha` mínimo 8 chars

**Response 200:** `{ "message": "senha alterada" }`
**Response 401:** `{ "error": "senha atual incorreta" }`

### GET /api/v1/usuarios/{id}/projetos

**Response 200:**
```json
{
  "projetos": [
    {"id": "uuid-1", "chave": "BACK", "nome": "Backend"},
    {"id": "uuid-2", "chave": "MOBILE", "nome": "Mobile App"}
  ]
}
```

### PUT /api/v1/usuarios/{id}/projetos

Substitui lista completa de projetos (replace, não patch):

**Body:**
```json
{
  "projeto_ids": ["uuid-1", "uuid-2", "uuid-3"]
}
```

**Response 200:** Lista atualizada de projetos (mesmo formato do GET).

**Validação:** Todos os UUIDs devem existir na tabela `projetos`.

## Arquitetura de Componentes

```
cmd/api/main.go ─── routes + middleware chain
│
├── middleware/auth.go
│   └── AuthJWT() ── valida token, extrai user_id pro context
│
├── middleware/projeto_filter.go
│   └── ProjetoFilter() ── busca alçada, injeta projeto_ids no context
│
├── handler/auth.go
│   └── POST /auth/login → Login()
│
├── handler/usuario.go
│   ├── GET    /usuarios           → List()
│   ├── POST   /usuarios           → Create()
│   ├── GET    /usuarios/{id}      → GetByID()
│   ├── PUT    /usuarios/{id}      → Update()
│   ├── PUT    /usuarios/{id}/senha    → AlterarSenha()
│   ├── GET    /usuarios/{id}/projetos → ListProjetos()
│   └── PUT    /usuarios/{id}/projetos → UpdateProjetos()
│
├── repository/usuario.go
│   ├── BuscarPorEmail()
│   ├── BuscarPorID()
│   ├── ListarTodos()
│   ├── Criar()
│   ├── Atualizar()
│   ├── AtualizarSenha()
│   ├── ListarProjetos()
│   ├── AtualizarProjetos()
│   └── BuscarProjetoIDsPorUsuario()
│
└── domain/usuario.go
    ├── Usuario (struct)
    ├── LoginRequest / LoginResponse
    ├── AlterarSenhaRequest
    └── AlcadaProjetosRequest
```

**Cadeia de middleware:**
```
Request → CORS → Logger → Recoverer → RequestID → AuthJWT → ProjetoFilter → Handler
```

Routes excluídas do auth: `POST /auth/login`, `GET /health`.

## Configuração

Adicionar em `config.go`:

```go
type AuthConfig struct {
    JWTSecret         string `env:"JWT_SECRET"`
    JWTExpirationHours int   `env:"JWT_EXPIRATION_HOURS" envDefault:"24"`
    AdminEmail        string `env:"ADMIN_EMAIL" envDefault:"admin@tcloud.local"`
}
```

**Env vars novas:**
- `JWT_SECRET` (obrigatório, sem default — falha no startup se ausente)
- `JWT_EXPIRATION_HOURS` (default: 24)
- `ADMIN_EMAIL` (default: `admin@tcloud.local`)

## Dependências Go

- `golang.org/x/crypto/bcrypt` — hash de senhas
- `github.com/golang-jwt/jwt/v5` — JWT sign/verify

## Impacto em Código Existente

- `main.go`: adicionar middleware chain (AuthJWT + ProjetoFilter), registrar novas routes
- `config/config.go`: adicionar `AuthConfig`
- Repositories futuros (equipe, timeline, etc): receber `projetoIDs []uuid.UUID` como parâmetro nas queries
- `repository/fonte_dados.go`: sem alteração (acesso livre, config do sistema)

## Fora do Escopo

- Refresh token (v2)
- Reset de senha por email (v2)
- MFA (v2)
- Permissões diferenciadas por cargo (cargo é metadata)
- Auto-registro de usuários (somente criação por usuário logado)
