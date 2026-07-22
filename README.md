# MyPlanner

Ferramenta de planejamento e gestao de capacidade de equipes, integrada com Jira.

Sincroniza projetos, sprints, tarefas e membros do Jira, e oferece dashboards de capacidade, timeline (Gantt), burndown e gestao de ausencias.

## Stack

| Camada   | Tecnologia                              |
|----------|-----------------------------------------|
| Backend  | Go 1.25 (chi, pgx, JWT, zap)           |
| Frontend | Vanilla JS (SPA single-file)            |
| Banco    | PostgreSQL 16                           |
| Infra    | Docker Compose                          |
| IA       | Google Gemini (opcional, analise de capacidade) |

## Inicio Rapido

```bash
# 1. Copiar variaveis de ambiente
cp .env.example .env
# Preencher PASS_DB, PASS_APP, JIRA_USER_EMAIL, JIRA_API_TOKEN

# 2. Subir tudo (DB + migrate + seed + server)
./dev.sh up
```

Acesse `http://localhost:8080`. Login padrao: `admin@myplanner.local` / senha definida em `PASS_APP`.

## Comandos

### dev.sh (recomendado)

```bash
./dev.sh up        # Stack completa
./dev.sh down      # Parar tudo
./dev.sh restart   # Rebuild + restart backend
./dev.sh status    # Status dos servicos
./dev.sh logs      # Tail logs do servidor
./dev.sh test      # Rodar testes
./dev.sh clean     # Limpar dados sincronizados (preserva usuarios)
```

### Makefile

```bash
make db            # Subir PostgreSQL
make dev           # Backend em modo dev
make build         # Compilar backend
make test          # Testes
make migrate-up    # Rodar migracoes
make seed          # Criar dados iniciais
```

## Estrutura do Projeto

```
myplanner/
  backend/
    cmd/
      api/           # Entrypoint do servidor
      migrate/       # Runner de migracoes
      seed/          # Seed de dados iniciais
    internal/
      auth/          # JWT token service
      config/        # Configuracao (viper)
      domain/        # Modelos de dominio
      handler/       # HTTP handlers
      jira/          # Cliente Jira (token + OAuth)
      middleware/    # Auth, filtro de projetos
      repository/    # Acesso a dados (pgx)
      service/       # Logica de negocio (equalizer, sync)
    migrations/      # SQL migrations (golang-migrate)
  frontend/
    index.html       # SPA completo (HTML + CSS + JS)
  docker-compose.yml # PostgreSQL 16
  dev.sh             # Script de dev (up/down/status/clean)
  Makefile
```

## Funcionalidades

- **Sync Jira** — sincronizacao automatica e manual de projetos, sprints, tarefas e membros
- **Sprints** — visualizacao de capacidade por membro, tarefas nao planejadas, burndown chart
- **Timeline** — Gantt de projetos (epicos) e ausencias de membros, capacidade mensal
- **Equipes** — gestao de membros, resumo de equipe
- **Skills** — catalogo global de skills (tags tecnicas) com associacao N:N a membros, autocomplete e criacao inline
- **Equalizer** — redistribuicao automatica de tarefas entre membros da sprint, algoritmo greedy com visualizacao before/after e apply via Jira
- **Ausencias** — ferias, licencas, dayoffs com impacto automatico na capacidade
- **Feriados** — CRUD de feriados nacionais, descontados dos dias uteis
- **Desligamento** — membros desligados sao excluidos automaticamente dos calculos
- **Analise IA** — analise de capacidade mensal via Google Gemini (requer `GEMINI_API_KEY`)
- **Auth** — JWT com controle de acesso por projeto

## Configuracao

Variaveis de ambiente (`.env`):

| Variavel | Descricao |
|----------|-----------|
| `PASS_DB` | Senha do PostgreSQL |
| `PASS_APP` | Senha do admin |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_NAME` | Conexao com banco |
| `JIRA_BASE_URL` | URL da instancia Jira |
| `JIRA_AUTH_TYPE` | `token` ou `oauth` |
| `JIRA_USER_EMAIL` | Email do usuario Jira |
| `JIRA_API_TOKEN` | API token do Jira |
| `JWT_SECRET` | Chave de assinatura JWT |
| `GEMINI_API_KEY` | API key Google Gemini (opcional) |
| `SYNC_INTERVAL_MINUTES` | Intervalo de sync automatico (default: 30) |

## Testes

```bash
make test
# ou
./dev.sh test
```

## Licenca

Uso interno.
