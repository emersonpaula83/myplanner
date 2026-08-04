#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
BACKEND="$ROOT/backend"
PID_FILE="/tmp/myplanner.pid"
BIN="/tmp/myplanner-dev"

# Exportar variáveis do .env para docker compose
if [ -f "$ROOT/.env" ]; then
    set -a
    source "$ROOT/.env"
    set +a
fi

PORT="${SERVER_PORT:-9091}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log()  { echo -e "${GREEN}[myplanner]${NC} $1"; }
warn() { echo -e "${YELLOW}[myplanner]${NC} $1"; }
err()  { echo -e "${RED}[myplanner]${NC} $1"; }

stop_server() {
    # 1) Kill by PID file
    if [ -f "$PID_FILE" ]; then
        local pid
        pid=$(cat "$PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null
            sleep 1
            if kill -0 "$pid" 2>/dev/null; then
                kill -9 "$pid" 2>/dev/null
            fi
            log "Servidor parado (PID $pid)"
        fi
        rm -f "$PID_FILE"
    fi
    # 2) Fallback: kill any myplanner-dev still listening on $PORT
    local port_pid
    port_pid=$(lsof -ti:"$PORT" -sTCP:LISTEN 2>/dev/null || true)
    if [ -n "$port_pid" ]; then
        kill $port_pid 2>/dev/null || true
        sleep 1
        # Force kill survivors
        for p in $port_pid; do
            if kill -0 "$p" 2>/dev/null; then
                kill -9 "$p" 2>/dev/null
            fi
        done
        log "Processos residuais na porta $PORT eliminados"
    fi
    # 3) Verify port is free
    if lsof -ti:"$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
        err "AVISO: porta $PORT ainda ocupada após stop"
    fi
}

ensure_no_local_postgres() {
    if ss -tlnp 2>/dev/null | grep -q "127.0.0.1:5432"; then
        warn "Postgres local detectado na porta 5432, parando..."
        systemctl stop postgresql 2>/dev/null || true
        sleep 1
    fi
}

cmd_db() {
    log "Subindo PostgreSQL..."
    ensure_no_local_postgres
    docker compose -f "$ROOT/docker-compose.yml" up -d db
    log "Aguardando PostgreSQL ficar pronto..."
    for i in $(seq 1 60); do
        if docker compose -f "$ROOT/docker-compose.yml" exec -T db pg_isready -U myplanner -d myplanner >/dev/null 2>&1; then
            log "PostgreSQL pronto!"
            return 0
        fi
        sleep 1
    done
    err "PostgreSQL não ficou pronto em 60s"
    docker compose -f "$ROOT/docker-compose.yml" logs db --tail 10
    return 1
}

cmd_migrate() {
    log "Rodando migrations..."
    cd "$BACKEND" && go run ./cmd/migrate -direction up
    log "Migrations aplicadas!"
}

cmd_seed() {
    log "Criando usuário admin..."
    cd "$BACKEND" && go run ./cmd/seed
    log "Seed concluído!"
}

cmd_build() {
    log "Compilando backend..."
    cd "$BACKEND" && go build -o "$BIN" ./cmd/api
    log "Build OK → $BIN"
}

cmd_ensure_db() {
    if ! docker compose -f "$ROOT/docker-compose.yml" exec -T db pg_isready -U myplanner -d myplanner >/dev/null 2>&1; then
        warn "PostgreSQL fora do ar, subindo..."
        cmd_db
    fi
}

cmd_start() {
    cmd_ensure_db
    stop_server
    cmd_build
    LOG_FILE="/tmp/myplanner.log"
    log "Iniciando servidor em http://localhost:$PORT ..."
    cd "$BACKEND" && "$BIN" >> "$LOG_FILE" 2>&1 &
    echo $! > "$PID_FILE"
    sleep 2
    if curl -sf http://localhost:$PORT/health >/dev/null 2>&1; then
        log "Servidor rodando! PID $(cat "$PID_FILE")"
    else
        err "Servidor não respondeu no /health"
        return 1
    fi
}

cmd_stop() {
    log "Parando tudo..."
    stop_server
    docker compose -f "$ROOT/docker-compose.yml" down 2>/dev/null || true
    log "Tudo parado."
}

cmd_restart() {
    log "Reiniciando..."
    cmd_start
}

cmd_status() {
    echo -e "${CYAN}=== MyPlanner Status ===${NC}"
    echo ""

    # DB
    if docker compose -f "$ROOT/docker-compose.yml" exec -T db pg_isready -U myplanner -d myplanner >/dev/null 2>&1; then
        echo -e "  PostgreSQL:  ${GREEN}●${NC} rodando"
    else
        echo -e "  PostgreSQL:  ${RED}●${NC} parado"
    fi

    # Server
    if curl -sf http://localhost:$PORT/health >/dev/null 2>&1; then
        local pid="?"
        [ -f "$PID_FILE" ] && pid=$(cat "$PID_FILE")
        echo -e "  Backend:     ${GREEN}●${NC} rodando (PID $pid)"
    else
        echo -e "  Backend:     ${RED}●${NC} parado"
    fi

    # Sync
    local token
    token=$(curl -sf http://localhost:$PORT/api/v1/auth/login \
        -H 'Content-Type: application/json' \
        -d '{"email":"admin@myplanner.local","senha":"Totvs@123"}' 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null || true)

    if [ -n "$token" ]; then
        local fontes
        fontes=$(curl -sf -H "Authorization: Bearer $token" http://localhost:$PORT/api/v1/fontes 2>/dev/null || echo "[]")
        local count
        count=$(echo "$fontes" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
        echo -e "  Fontes JIRA: ${CYAN}$count${NC} configurada(s)"

        if [ "$count" != "0" ]; then
            echo "$fontes" | python3 -c "
import sys, json
fontes = json.load(sys.stdin)
for f in fontes:
    sync = f.get('ultimo_sync', None)
    status = sync[:16].replace('T',' ') if sync else 'nunca'
    print(f\"    → {f['nome']}: último sync {status}\")
" 2>/dev/null || true
        fi
    fi
    echo ""
}

cmd_logs() {
    local log_file="/tmp/myplanner.log"
    if [ -f "$log_file" ]; then
        tail -f "$log_file"
    else
        warn "Sem arquivo de log. Inicie com: ./dev.sh start"
    fi
}

cmd_test() {
    log "Rodando testes..."
    cd "$BACKEND" && go test ./...
    log "Testes OK!"
}

cmd_clean() {
    warn "Limpando TODOS os dados do banco (mantém estrutura e fonte_dados)..."
    read -rp "Tem certeza? (s/N) " confirm
    if [[ "$confirm" != "s" && "$confirm" != "S" ]]; then
        log "Cancelado."
        return 0
    fi
    docker compose -f "$ROOT/docker-compose.yml" exec -T db psql -U myplanner -d myplanner -c \
        "TRUNCATE tarefas, tarefa_produtos, sprints, projetos, membros, sync_logs, disponibilidade, equipes, equipe_membros, limites_alerta, produtos, usuario_projetos CASCADE;"
    log "Dados sincronizados limpos! (usuarios e fonte_dados preservados)"
}

cmd_cleanall() {
    warn "Limpando TUDO (incluindo fonte_dados e migrações)..."
    read -rp "Tem certeza? Vai precisar rodar migrate de novo. (s/N) " confirm
    if [[ "$confirm" != "s" && "$confirm" != "S" ]]; then
        log "Cancelado."
        return 0
    fi
    docker compose -f "$ROOT/docker-compose.yml" exec -T db psql -U myplanner -d myplanner -c \
        "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
    log "Schema zerado! Rode: ./dev.sh migrate"
}

cmd_get_master_pass() {
    log "Buscando senha do admin no K8s..."
    local ns="${ADMIN_SECRET_NAMESPACE:-default}"
    local secret_name="${ADMIN_SECRET_NAME:-myplanner-admin-password}"

    local password
    password=$(kubectl get secret "$secret_name" -n "$ns" \
        -o jsonpath='{.data.password}' 2>/dev/null | base64 -d 2>/dev/null || true)

    if [ -z "$password" ]; then
        err "Sem acesso ao cluster / aplicação ou falta de credenciais RBAC"
        return 1
    fi

    local expires
    expires=$(kubectl get secret "$secret_name" -n "$ns" \
        -o jsonpath='{.data.expires_at}' 2>/dev/null | base64 -d 2>/dev/null || true)

    echo ""
    echo -e "  ${CYAN}Admin Password${NC}"
    echo -e "  Email:    admin@myplanner.local"
    echo -e "  Senha:    ${GREEN}$password${NC}"
    if [ -n "$expires" ]; then
        echo -e "  Expira:   $expires"
    fi
    echo ""
}

cmd_up() {
    log "Subindo tudo..."
    cmd_db
    cmd_migrate
    cmd_seed
    cmd_start
    echo ""
    cmd_status
    log "Acesse: http://localhost:$PORT"
    log "Login: admin@myplanner.local / Totvs@123"
}

cmd_down() {
    cmd_stop
}

cmd_help() {
    echo -e "${CYAN}MyPlanner — Dev Script${NC}"
    echo ""
    echo "Uso: ./dev.sh <comando>"
    echo ""
    echo "Comandos:"
    echo "  up        Sobe tudo (DB + migrate + server)"
    echo "  down      Alias para stop"
    echo "  start     Compila e inicia o servidor"
    echo "  stop      Para tudo (servidor + Docker)"
    echo "  restart   Recompila e reinicia"
    echo "  db        Sobe só o PostgreSQL"
    echo "  migrate   Roda migrations"
    echo "  seed      Cria usuário admin (usa PASS_APP do .env)"
    echo "  build     Compila o backend"
    echo "  test      Roda testes"
    echo "  status    Mostra status dos serviços"
    echo "  logs      Mostra logs do servidor (tail -f)"
    echo "  clean     Limpa dados (mantém estrutura e fonte_dados)"
    echo "  cleanall  Zera schema completo (precisa migrate depois)"
    echo "  get-master-pass  Busca senha admin no K8s Secret"
    echo "  help      Mostra esta ajuda"
    echo ""
}

case "${1:-help}" in
    up)      cmd_up ;;
    down)    cmd_down ;;
    start)   cmd_start ;;
    stop)    cmd_stop ;;
    restart) cmd_restart ;;
    db)      cmd_db ;;
    migrate) cmd_migrate ;;
    seed)    cmd_seed ;;
    build)   cmd_build ;;
    test)    cmd_test ;;
    status)  cmd_status ;;
    logs)    cmd_logs ;;
    clean)   cmd_clean ;;
    cleanall) cmd_cleanall ;;
    get-master-pass) cmd_get_master_pass ;;
    help|*)  cmd_help ;;
esac
