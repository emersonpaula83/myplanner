# Sync Progress — Contadores Incrementais Inline

**Data:** 2026-07-19
**Escopo:** Frontend only — sem mudança no backend

## Contexto

O sync de projetos JIRA é assíncrono: `POST /sync/trigger` dispara goroutine e retorna imediato. Frontend faz polling a cada 2s no `GET /sync/status` que retorna contadores (`total_tarefas`, `total_membros`, `total_sprints`) e `status` (`running`/`success`/`partial`/`error`).

Problema: frontend mostra "Sincronizando..." estático durante todo o processo. Contadores só aparecem ao finalizar. Usuário não vê progresso.

## Design

### Comportamento durante sync

Card da fonte de dados mostra:
- Texto do label: "Sincronizando TCDV..." (inclui project key)
- Barra indeterminate animada (shimmer CSS) abaixo do sync status
- Contadores live com prefixo ↻ atualizando a cada poll
- Quando valor muda entre polls: animação bump (scale 1.1 + accent por 300ms)
- Botão do projeto mostra contagem incremental: "47 tarefas..."

### Ao finalizar

- Barra fade-out (opacity 0 → remove DOM)
- Classe `.syncing` removida dos stats (↻ desaparece)
- Contadores fazem bump final pro valor definitivo
- Dot muda cor (verde/âmbar/vermelho)
- Label: "Sincronizado" / "Parcial" / "Erro"

## CSS (3 regras novas)

### `.sync-progress-bar`
- Container: width 100%, height 3px, border-radius 2px, background var(--border-subtle), overflow hidden, margin 6px 0
- Inner: width 40%, height 100%, background linear-gradient(90deg, var(--accent), var(--blue))
- Animação: `@keyframes shimmer` — translateX(-100%) → translateX(250%), duration 1.5s, infinite
- Transição: opacity 0.3s para fade-out ao finalizar

### `.fonte-card-stats.syncing span::before`
- Content: "↻ "
- Color: var(--blue)

### `.counter-bump`
- Animação: scale(1.1) + color var(--accent), duration 300ms, ease-out
- Via `@keyframes bump`: 0% scale(1) → 50% scale(1.1) + accent → 100% scale(1)

## JS — mudanças em `startSyncPolling`

### Ao iniciar (antes do setInterval)
1. Injetar `.sync-progress-bar` no card após `.fonte-card-sync`
2. Adicionar classe `.syncing` ao `.fonte-card-stats`
3. Label text: "Sincronizando {projectKey}..."
4. Inicializar `prev = { tarefas: 0, membros: 0, sprints: 0 }`

### A cada poll (dentro do setInterval)
1. Comparar `st.total_tarefas` vs `prev.tarefas` — se diferente, aplicar `.counter-bump` no span correspondente, remover após 300ms
2. Mesmo para membros e sprints
3. Atualizar innerHTML dos stats com valores novos
4. Atualizar `prev`
5. Botão: já mostra `st.total_tarefas + ' tarefas...'` (mantém comportamento atual)

### Ao finalizar (status !== 'running')
1. Barra: `opacity = 0`, remove do DOM após 300ms
2. Remover `.syncing` do stats
3. Bump final nos contadores
4. Dot/label: comportamento existente mantido

## Arquivos modificados

- `frontend/index.html` — CSS (3 regras) + JS (`startSyncPolling`)

## Fora de escopo

- Mudanças no backend
- WebSocket/SSE
- Progress bar deterministic (% real)
- Notificações push
