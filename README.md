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

Acesse `http://localhost:9091`. Login via Fluig Identity (SSO) ou acesso admin: `admin@myplanner.local` (senha rotacionada diariamente, ver stdout do servidor em dev).

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
      admin/         # Rotacao de senha, K8s Secret
      auth/          # JWT token service
      config/        # Configuracao (env vars)
      domain/        # Modelos de dominio
      handler/       # HTTP handlers
      jira/          # Cliente Jira (token + OAuth)
      middleware/    # Auth JWT, filtro de alcada por equipe
      repository/    # Acesso a dados (pgx)
      saml/          # SAML 2.0 Service Provider (Fluig Identity)
      service/       # Logica de negocio (equalizer, sync, alocacao)
    migrations/      # SQL migrations (golang-migrate)
  frontend/
    index.html       # SPA completo (HTML + CSS + JS)
  docker-compose.yml # PostgreSQL 16
  dev.sh             # Script de dev (up/down/status/clean)
  Makefile
```

## Modulos

### IAM (Identidade e Acesso)

- **SSO via Fluig Identity** — autenticacao SAML 2.0 com Fluig Identity da TOTVS como IdP. Sem login email/senha para usuarios comuns
- **Admin local** — usuario admin (`admin@myplanner.local`) com login email/senha. Senha rotacionada diariamente e armazenada em K8s Secret
- **Alcada por equipe** — cada usuario ve somente dados das equipes que o admin liberou. Filtro automatico em todos os modulos (sprints, timeline, alocacao, pessoas). Admin ve tudo
- **JWT stateless** — apos autenticacao (SAML ou local), sessao mantida via JWT com expiracao de 24h

### Alocacao de Projetos

- **Cards de projetos** — visualizacao por tipo de demanda (Meta, Compromisso, Iniciativa) com barras de progresso (% estimado e % planejado)
- **Status automatico** — 4 status calculados: Concluido (todas tarefas done/canceladas), Em Andamento (tarefas em progresso), Nao Iniciado (tudo em backlog), Desconsiderado (manual)
- **Filtros** — Em Andamento, Em Atraso (data limite < hoje), Concluidos, Desconsiderados, Todos. Desconsiderados com opacidade reduzida no filtro Todos
- **Modal de detalhes** — secoes accordion (Nao Alocadas, Estimadas sem Pessoa, Planejadas, Concluidas) com alocacao de sprint, pessoa e estimativa por tarefa
- **Gantt do projeto** — timeline visual com barras por tarefa e linha de data limite
- **Desconsiderar projeto** — fluxo com motivo e data, substituindo antigo "Encerrar"

### Sprint Review

- **Relatorio por sprint** — estatisticas de entrega (GDPTC), destaques, metricas por membro
- **Graficos interativos** — pizza por tipo de demanda e por epico, com tooltips clicaveis mostrando tickets, descricao e relator
- **Exportacao** — PDF e imagem do relatorio completo
- **Analise IA** — analise automatica de capacidade e entregas via Google Gemini

### Sprints e Capacidade

- **Capacidade por membro** — horas disponiveis vs alocadas, percentual de ocupacao
- **Tarefas nao planejadas** — identificacao de tarefas sem sprint ou estimativa
- **Burndown chart** — grafico de evolucao da sprint

### Timeline

- **Gantt de sprints** — visualizacao anual com barras de alocacao por sprint. Tooltip com % e horas (alocacao e livre)
- **Checkpoints** — marcos visuais na timeline com cores e resumos
- **Filtragem por equipe** — board_id isolado para evitar vazamento entre times
- **Analise IA** — analise de capacidade mensal via Google Gemini

### Sync Jira

- **Sincronizacao** — automatica (intervalo configuravel) e manual de projetos, sprints, tarefas e membros
- **Auto-deteccao** — custom fields (tipo de demanda) detectados automaticamente
- **Preservacao** — dados manuais (apelido, data inicio, data limite) preservados durante sync via COALESCE
- **Upsert completo** — todos campos atualizados: responsavel, relator, estimativa, sprint, epico pai, tipo, tipo de demanda, status

### Equipes

- **Gestao de membros** — cargos (dev, QA, scrum master, gerente, etc), associacao de produtos
- **Board ID** — configuravel por equipe para isolamento de sprints do Jira
- **Resumo** — visao consolidada da equipe

### Outras Funcionalidades

- **Disclaimers** — modal de ressalvas por sprint com graficos de pizza interativos
- **Skills** — catalogo global de skills tecnicas com associacao N:N a membros
- **Equalizer** — redistribuicao automatica de tarefas (algoritmo greedy) com visualizacao before/after e apply via Jira
- **Ausencias** — ferias, licencas, dayoffs com impacto automatico na capacidade
- **Feriados** — CRUD de feriados nacionais, descontados dos dias uteis
- **Desligamento** — membros desligados excluidos automaticamente dos calculos
- **UX** — favicon e logo SVG, datas formato brasileiro, dark mode completo com exportacao

## Configuracao

Variaveis de ambiente (`.env`):

| Variavel | Descricao |
|----------|-----------|
| `PASS_DB` | Senha do PostgreSQL |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_NAME` | Conexao com banco |
| `JWT_SECRET` | Chave de assinatura JWT |
| `JIRA_BASE_URL` | URL da instancia Jira |
| `JIRA_AUTH_TYPE` | `token` ou `oauth` |
| `JIRA_USER_EMAIL` | Email do usuario Jira |
| `JIRA_API_TOKEN` | API token do Jira |
| `SAML_IDP_METADATA_URL` | URL do metadata XML do Fluig Identity |
| `SAML_ENTITY_ID` | Identificador do SP |
| `SAML_ACS_URL` | URL do Assertion Consumer Service |
| `SAML_CERT_FILE` | Path do certificado X.509 do SP |
| `SAML_KEY_FILE` | Path da chave privada do SP |
| `SAML_FRONTEND_URL` | URL base do frontend para redirect pos-auth |
| `GEMINI_API_KEY` | API key Google Gemini (opcional) |
| `SYNC_INTERVAL_MINUTES` | Intervalo de sync automatico (default: 30) |

Senha do admin rotacionada diariamente. Em K8s, armazenada no Secret `myplanner-admin-password`. Em dev local, logada no stdout.

## Testes

```bash
make test
# ou
./dev.sh test
```

## Licenca

Uso interno.
