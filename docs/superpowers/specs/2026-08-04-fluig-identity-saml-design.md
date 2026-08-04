# Integração Fluig Identity — SAML 2.0 SSO

## Resumo

Substituir login email/senha por SSO via Fluig Identity da TOTVS usando SAML 2.0. MyPlanner = Service Provider (SP), Fluig Identity = Identity Provider (IdP). Após autenticação SAML, sistema gera JWT interno para manter sessão.

## Fluxo de Autenticação

```
Usuário acessa MyPlanner
  → Frontend checa JWT no localStorage
    → Sem JWT ou expirado?
      → Redirect para /api/v1/auth/saml/login
        → Backend gera SAML AuthnRequest
          → Redirect pro Fluig Identity (IdP)
            → Usuário autentica no Fluig
              → Fluig POST SAML Response para /api/v1/auth/saml/acs
                → Backend valida assertion + assinatura XML
                  → Extrai email do NameID
                    → Match na tabela usuarios por email
                      → Não existe? Cria usuário automaticamente
                      → Existe? Usa existente
                    → Gera JWT interno (mesmo formato atual)
                      → Redirect pro frontend com /#auth_token=xxx
                        → Frontend salva JWT, carrega app
```

JWT interno mantido após SAML validar — evita mudar middleware existente.

## Backend — Componentes

### Novo: `internal/saml/provider.go`

- `SAMLConfig` — struct com IdP metadata URL, entity ID, ACS URL, cert, key
- `SAMLProvider` — encapsula `crewjam/saml` ServiceProvider
- `NewSAMLProvider(cfg SAMLConfig)` — carrega metadata do IdP (HTTP fetch ou XML local), configura SP
- `MakeAuthnRequest()` — gera AuthnRequest para redirect ao IdP
- `ValidateResponse(samlResponse string)` — valida SAML response, extrai email + nome dos atributos

### Novo: `internal/handler/saml_auth.go`

Endpoints (sem auth middleware):

| Método | Path | Função |
|--------|------|--------|
| GET | `/api/v1/auth/saml/login` | Inicia SSO — gera AuthnRequest, redirect pro IdP |
| POST | `/api/v1/auth/saml/acs` | Assertion Consumer Service — recebe SAML response, valida, busca/cria usuário, gera JWT, redirect `/#auth_token=xxx` |
| GET | `/api/v1/auth/saml/metadata` | Expõe SP metadata XML para configurar no Fluig Identity |

### Lógica do ACS

1. Receber `SAMLResponse` do POST (form-encoded)
2. Validar assinatura XML contra certificado do IdP
3. Verificar expiração, audience, destination
4. Extrair email do NameID (formato `emailAddress`)
5. Buscar usuário por email em `usuarios`
6. Se não existe: criar com `nome_completo` do atributo SAML (ou NameID), `cargo` default `gerente`, `auth_provider = 'saml'`, `senha_hash = NULL`
7. Gerar JWT (mesmo `TokenService.GenerateToken()` existente)
8. Redirect: `{FRONTEND_URL}/#auth_token={jwt}`

### Mudanças em existentes

- `config/config.go` — adicionar `SAMLConfig`
- `main.go` — registrar routes SAML (fora do auth middleware)
- `handler/auth.go` — restringir `Login()` apenas para `auth_provider = 'local'` (admin)
- `repository/usuario.go` — adicionar `BuscarOuCriarPorEmail(email, nome, authProvider)` — busca por email, cria se não existe

## Frontend

### Tela de login

Dois modos de acesso:

1. **Botão principal:** "Entrar com Fluig Identity" → redirect `/api/v1/auth/saml/login`
2. **Link discreto abaixo:** "Acesso administrativo" → expande form email/senha (para admin local)

O form email/senha chama `POST /api/v1/auth/login` (mantido apenas para admin). Validação backend rejeita login local se `auth_provider != 'local'`.

### Retorno do SAML

Estender lógica existente de hash params (já usada para OAuth Atlassian):
- Ler `auth_token` do `window.location.hash`
- Salvar em `localStorage` como `myplanner_token`
- Limpar hash, carregar app

### Erros

- SAML falha → redirect `/#auth_error=mensagem`
- Frontend mostra erro na tela de login se `auth_error` presente no hash

### Logout

- `logout()` limpa token local (sem mudança)
- Sem SLO (Single Logout) — v1. Sessão Fluig Identity separada

## Migration

```sql
-- Permitir usuários sem senha (autenticação via SAML)
ALTER TABLE usuarios ALTER COLUMN senha_hash DROP NOT NULL;

-- Indicar provedor de autenticação
ALTER TABLE usuarios ADD COLUMN auth_provider VARCHAR(20) NOT NULL DEFAULT 'local';
```

Valores de `auth_provider`: `local` (email/senha legado), `saml` (Fluig Identity).

## Configuração

### Envs novas

| Variável | Obrigatório | Default | Descrição |
|----------|-------------|---------|-----------|
| `SAML_IDP_METADATA_URL` | Sim | — | URL do metadata XML do Fluig Identity |
| `SAML_ENTITY_ID` | Sim | — | Identificador do SP (ex: `https://myplanner.totvs.com`) |
| `SAML_ACS_URL` | Sim | — | URL completa do ACS endpoint |
| `SAML_CERT_FILE` | Sim | — | Path do certificado X.509 do SP (PEM) |
| `SAML_KEY_FILE` | Sim | — | Path da chave privada do SP (PEM) |
| `SAML_FRONTEND_URL` | Sim | — | URL base do frontend para redirect pós-auth |

Startup falha se variáveis obrigatórias ausentes.

### Configuração no Fluig Identity (manual)

1. Acessar painel admin Fluig Identity
2. Registrar novo aplicativo SAML
3. Informar Entity ID do MyPlanner
4. Informar ACS URL do MyPlanner
5. Copiar URL do metadata XML do IdP (ou baixar XML)
6. Garantir que NameID = email do usuário (formato `emailAddress`)

O endpoint `GET /api/v1/auth/saml/metadata` gera o XML do SP automaticamente — pode ser importado diretamente no Fluig Identity.

## Alçada por Equipe

### Conceito

Cada usuário tem acesso a 1 ou mais equipes. Sem alçada = sem acesso a dados. Projetos vinculados às equipes da alçada ficam visíveis automaticamente.

O usuário admin (`admin@myplanner.local`) tem acesso total — não precisa de alçada, vê tudo.

Somente o admin pode gerenciar alçadas (atribuir/remover equipes de usuários).

### Schema

```sql
CREATE TABLE usuario_equipes (
    usuario_id  UUID NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
    equipe_id   UUID NOT NULL REFERENCES equipes(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (usuario_id, equipe_id)
);

CREATE INDEX idx_usuario_equipes_usuario ON usuario_equipes(usuario_id);
```

Substitui tabela `usuario_projetos` da spec IAM antiga.

### Filtro de Visibilidade

Middleware `EquipeFilter` — injeta `equipeIDs []uuid.UUID` no context a partir da alçada do usuário. Admin bypassa (recebe todas equipes).

| Módulo | Como filtra |
|--------|------------|
| Sprints / Capacidade | Só mostra sprints de equipes na alçada |
| Timeline | Só mostra dados de equipes na alçada |
| Alocação / Projetos | Épicos vinculados a equipes na alçada |
| Pessoas | Membros de equipes na alçada |
| Fonte de Dados | Sem filtro (configuração do sistema) |

Aplicação nas queries via `WHERE equipe_id = ANY($1)` onde `$1` = equipe_ids da alçada.

### API — Alçada

Somente admin autenticado. Middleware verifica `cargo = 'admin'` ou `email = 'admin@myplanner.local'`.

| Método | Path | Descrição |
|--------|------|-----------|
| GET | `/api/v1/usuarios/{id}/equipes` | Lista equipes na alçada do usuário |
| PUT | `/api/v1/usuarios/{id}/equipes` | Substitui lista de equipes (body: `{"equipe_ids": [...]}`) |

### Frontend — Gestão de Alçada

Tela de gestão de usuários (acesso admin):
- Lista de usuários com suas equipes
- Para cada usuário: multiselect de equipes disponíveis
- Salvar chama `PUT /usuarios/{id}/equipes`

## Admin — Rotação de Senha via K8s Secret

### Usuário Admin

- Email fixo: `admin@myplanner.local`
- `auth_provider = 'local'` — admin sempre faz login via email/senha (não via SAML)
- Acesso total a tudo (bypass de alçada)
- Senha rotacionada automaticamente todo dia

### Rotação Automática

Goroutine no backend (`internal/admin/rotation.go`):
- Executa na inicialização e depois a cada 24h (ticker)
- Gera senha aleatória forte: 16 chars, letras maiúsculas/minúsculas + números + símbolos
- Atualiza `senha_hash` do admin no banco via bcrypt
- Escreve senha plaintext no K8s Secret `myplanner-admin-password` (namespace do pod) via `client-go`
- Loga rotação (sem logar a senha em si)

### K8s Secret

O backend cria/atualiza o Secret automaticamente:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: myplanner-admin-password
  namespace: <namespace-do-pod>
type: Opaque
data:
  password: <base64-da-senha>
  expires_at: <base64-da-data-expiração>
```

### Recuperar a Senha

```bash
kubectl get secret myplanner-admin-password -n <namespace> \
  -o jsonpath='{.data.password}' | base64 -d
```

Quem tem acesso ao namespace K8s consegue ler. Controle de acesso = RBAC do K8s (já existente).

### RBAC Necessário

ServiceAccount do pod precisa de permissão para criar/atualizar Secrets no namespace:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: myplanner-secret-writer
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    resourceNames: ["myplanner-admin-password"]
    verbs: ["get", "create", "update", "patch"]
```

### Fallback fora do K8s

Se `KUBERNETES_SERVICE_HOST` não existir (dev local, Docker), o backend loga a senha no stdout na inicialização e a cada rotação. Em produção (K8s), escreve no Secret e não loga.

### Envs

| Variável | Obrigatório | Default | Descrição |
|----------|-------------|---------|-----------|
| `ADMIN_SECRET_NAME` | Não | `myplanner-admin-password` | Nome do K8s Secret |
| `ADMIN_SECRET_NAMESPACE` | Não | namespace do pod | Namespace do Secret |

## Dependências Go

- `github.com/crewjam/saml` — implementação SP SAML 2.0
- `k8s.io/client-go` — interação com K8s API (escrita de Secrets)
- `golang.org/x/crypto/bcrypt` — hash de senhas (já existe)
- `github.com/golang-jwt/jwt/v5` — JWT (já existe)

## Impacto em Arquivos

| Arquivo | Mudança |
|---------|---------|
| `internal/saml/provider.go` | **Novo** — SAMLProvider, config, validação |
| `internal/handler/saml_auth.go` | **Novo** — endpoints login, ACS, metadata |
| `internal/admin/k8s_secret.go` | **Novo** — escrita de senha no K8s Secret |
| `internal/handler/usuario.go` | **Alterar** — endpoints de alçada por equipe |
| `internal/admin/rotation.go` | **Novo** — goroutine rotação de senha |
| `internal/middleware/equipe_filter.go` | **Novo** — middleware filtro por alçada |
| `internal/config/config.go` | Adicionar SAMLConfig |
| `cmd/api/main.go` | Registrar routes SAML, iniciar goroutine rotação |
| `internal/handler/auth.go` | Restringir Login() para auth_provider=local |
| `internal/repository/usuario.go` | BuscarOuCriarPorEmail(), alçada equipes, admin password |
| `migrations/` | senha nullable, auth_provider, usuario_equipes, admin_password_plain |
| `frontend/index.html` | Tela de login SAML, gestão de alçada (admin) |

## Fora do Escopo

- OIDC como alternativa (v2 — se Fluig Identity usar OIDC, adaptar SAMLProvider)
- Single Logout (SLO) — v2
- Mapeamento de grupos/roles do Fluig → cargos do MyPlanner
- Login email/senha para usuários não-admin (somente SAML)
- Auditoria de acessos (v2)
