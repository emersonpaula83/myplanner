package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

type SyncRepository struct {
	pool *pgxpool.Pool
}

func NewSyncRepository(pool *pgxpool.Pool) *SyncRepository {
	return &SyncRepository{pool: pool}
}

type UpsertTarefaParams struct {
	FonteDadosID       uuid.UUID
	ProjetoID          uuid.UUID
	JiraID             string
	NumeroTicket       string
	Resumo             string
	Tipo               string
	Status             string
	Prioridade         *string
	EstimativaPontos   *float64
	EstimativaTempo    *int
	TempoGasto         *int
	ResponsavelID      *uuid.UUID
	RelatorID          *uuid.UUID
	Team               *string
	SprintID           *uuid.UUID
	DataCriacao        time.Time
	DataLimite         *pgtype.Date
	DataResolvido      *time.Time
	DataAtualizado     *time.Time
	TipoDemanda        *string
	DataComponente     *pgtype.Date
	StatusCategoria    *string
	CamposExtras       json.RawMessage
	ParentID           *uuid.UUID
	Apelido            *string
	DataInicioExecucao *time.Time
}

type SyncTotals struct {
	Projetos int
	Tarefas  int
	Membros  int
	Sprints  int
}

func (r *SyncRepository) UpsertMembro(ctx context.Context, fonteDadosID uuid.UUID, jiraAccountID, nome string, email, avatarURL, team *string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO membros (id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, true)
		ON CONFLICT (fonte_dados_id, jira_account_id)
		DO UPDATE SET nome = EXCLUDED.nome, email = EXCLUDED.email,
		              avatar_url = EXCLUDED.avatar_url, team = COALESCE(membros.team, EXCLUDED.team),
		              ativo = true, updated_at = NOW()
		RETURNING id
	`, fonteDadosID, jiraAccountID, nome, email, avatarURL, team).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upserting membro %s: %w", jiraAccountID, err)
	}
	return id, nil
}

func (r *SyncRepository) UpsertProjeto(ctx context.Context, fonteDadosID uuid.UUID, jiraID, chave, nome string, descricao *string, leadID *uuid.UUID, categoria *string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO projetos (id, fonte_dados_id, jira_id, chave, nome, descricao, lead_id, categoria, ativo)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, true)
		ON CONFLICT (fonte_dados_id, jira_id)
		DO UPDATE SET chave = EXCLUDED.chave, nome = EXCLUDED.nome,
		              descricao = EXCLUDED.descricao, lead_id = EXCLUDED.lead_id,
		              categoria = EXCLUDED.categoria, ativo = true, updated_at = NOW()
		RETURNING id
	`, fonteDadosID, jiraID, chave, nome, descricao, leadID, categoria).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upserting projeto %s: %w", chave, err)
	}
	return id, nil
}

func (r *SyncRepository) UpsertSprint(ctx context.Context, fonteDadosID uuid.UUID, jiraID int, nome string, estado *string, dataInicio, dataFim, dataConclusao *time.Time, boardID *int, projetoID *uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO sprints (id, fonte_dados_id, jira_id, nome, estado, data_inicio, data_fim, data_conclusao, board_id, projeto_id)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (fonte_dados_id, jira_id)
		DO UPDATE SET nome = EXCLUDED.nome, estado = EXCLUDED.estado,
		              data_inicio = EXCLUDED.data_inicio, data_fim = EXCLUDED.data_fim,
		              data_conclusao = EXCLUDED.data_conclusao, board_id = EXCLUDED.board_id,
		              projeto_id = EXCLUDED.projeto_id, updated_at = NOW()
		RETURNING id
	`, fonteDadosID, jiraID, nome, estado, dataInicio, dataFim, dataConclusao, boardID, projetoID).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upserting sprint %d: %w", jiraID, err)
	}
	return id, nil
}

func (r *SyncRepository) UpsertTarefa(ctx context.Context, t *UpsertTarefaParams) (uuid.UUID, error) {
	ce := t.CamposExtras
	if ce == nil {
		ce = json.RawMessage(`{}`)
	}
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO tarefas (id, fonte_dados_id, projeto_id, jira_id, numero_ticket, resumo,
		                     tipo, status, prioridade, estimativa_pontos, estimativa_tempo, tempo_gasto,
		                     responsavel_id, relator_id, team, sprint_id, data_criacao, data_limite,
		                     data_resolvido, data_atualizado, tipo_demanda, data_componente,
		                     status_categoria, campos_extras, parent_id, apelido, data_inicio_execucao)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
		        $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26)
		ON CONFLICT (fonte_dados_id, jira_id)
		DO UPDATE SET resumo = EXCLUDED.resumo, tipo = EXCLUDED.tipo, status = EXCLUDED.status,
		              prioridade = EXCLUDED.prioridade, estimativa_pontos = EXCLUDED.estimativa_pontos,
		              estimativa_tempo = EXCLUDED.estimativa_tempo, tempo_gasto = EXCLUDED.tempo_gasto,
		              responsavel_id = EXCLUDED.responsavel_id, relator_id = EXCLUDED.relator_id,
		              team = EXCLUDED.team, sprint_id = EXCLUDED.sprint_id,
		              data_limite = EXCLUDED.data_limite, data_resolvido = EXCLUDED.data_resolvido,
		              data_atualizado = EXCLUDED.data_atualizado, tipo_demanda = EXCLUDED.tipo_demanda,
		              data_componente = EXCLUDED.data_componente, status_categoria = EXCLUDED.status_categoria,
		              parent_id = EXCLUDED.parent_id, apelido = EXCLUDED.apelido,
		              data_inicio_execucao = EXCLUDED.data_inicio_execucao,
		              campos_extras = EXCLUDED.campos_extras, updated_at = NOW()
		RETURNING id
	`, t.FonteDadosID, t.ProjetoID, t.JiraID, t.NumeroTicket, t.Resumo,
		t.Tipo, t.Status, t.Prioridade, t.EstimativaPontos, t.EstimativaTempo, t.TempoGasto,
		t.ResponsavelID, t.RelatorID, t.Team, t.SprintID, t.DataCriacao, t.DataLimite,
		t.DataResolvido, t.DataAtualizado, t.TipoDemanda, t.DataComponente,
		t.StatusCategoria, ce, t.ParentID, t.Apelido, t.DataInicioExecucao).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upserting tarefa %s: %w", t.NumeroTicket, err)
	}
	return id, nil
}

func (r *SyncRepository) LookupTarefaIDByJiraID(ctx context.Context, fonteDadosID uuid.UUID, jiraID string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT id FROM tarefas WHERE fonte_dados_id = $1 AND jira_id = $2
	`, fonteDadosID, jiraID).Scan(&id)
	if err == pgx.ErrNoRows {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("looking up tarefa by jira_id %s: %w", jiraID, err)
	}
	return id, nil
}

func (r *SyncRepository) UpdateTarefaParent(ctx context.Context, tarefaID, parentID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tarefas SET parent_id = $2, updated_at = NOW() WHERE id = $1
	`, tarefaID, parentID)
	if err != nil {
		return fmt.Errorf("updating parent_id for tarefa %s: %w", tarefaID, err)
	}
	return nil
}

func (r *SyncRepository) UpsertProduto(ctx context.Context, fonteDadosID uuid.UUID, jiraID, nome string, descricao *string, projetoID *uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO produtos (id, fonte_dados_id, jira_id, nome, descricao, projeto_id, ativo)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, true)
		ON CONFLICT (fonte_dados_id, jira_id)
		DO UPDATE SET nome = EXCLUDED.nome, descricao = EXCLUDED.descricao,
		              projeto_id = EXCLUDED.projeto_id, ativo = true, updated_at = NOW()
		RETURNING id
	`, fonteDadosID, jiraID, nome, descricao, projetoID).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upserting produto %s: %w", nome, err)
	}
	return id, nil
}

func (r *SyncRepository) LinkTarefaProduto(ctx context.Context, tarefaID, produtoID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO tarefa_produtos (tarefa_id, produto_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, tarefaID, produtoID)
	if err != nil {
		return fmt.Errorf("linking tarefa %s to produto %s: %w", tarefaID, produtoID, err)
	}
	return nil
}

func (r *SyncRepository) CreateSyncLog(ctx context.Context, log *domain.SyncLog) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sync_logs (id, fonte_dados_id, tipo, status, iniciado_em, mensagem)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, log.ID, log.FonteDadosID, log.Tipo, log.Status, log.IniciadoEm, log.Mensagem)
	if err != nil {
		return fmt.Errorf("creating sync log: %w", err)
	}
	return nil
}

func (r *SyncRepository) UpdateSyncLogTotals(ctx context.Context, id uuid.UUID, totals SyncTotals) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE sync_logs
		SET total_projetos = $2, total_tarefas = $3, total_membros = $4, total_sprints = $5
		WHERE id = $1
	`, id, totals.Projetos, totals.Tarefas, totals.Membros, totals.Sprints)
	if err != nil {
		return fmt.Errorf("updating sync log totals %s: %w", id, err)
	}
	return nil
}

func (r *SyncRepository) UpdateSyncLog(ctx context.Context, id uuid.UUID, status string, finalizadoEm time.Time, totals SyncTotals, erros json.RawMessage, mensagem *string) error {
	if erros == nil {
		erros = json.RawMessage(`[]`)
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE sync_logs
		SET status = $2, finalizado_em = $3, total_projetos = $4, total_tarefas = $5,
		    total_membros = $6, total_sprints = $7, erros = $8, mensagem = $9
		WHERE id = $1
	`, id, status, finalizadoEm, totals.Projetos, totals.Tarefas, totals.Membros, totals.Sprints, erros, mensagem)
	if err != nil {
		return fmt.Errorf("updating sync log %s: %w", id, err)
	}
	return nil
}

func (r *SyncRepository) GetLatestSyncLog(ctx context.Context, fonteDadosID uuid.UUID) (*domain.SyncLog, error) {
	var log domain.SyncLog
	err := r.pool.QueryRow(ctx, `
		SELECT id, fonte_dados_id, tipo, status, iniciado_em, finalizado_em,
		       total_projetos, total_tarefas, total_membros, total_sprints,
		       erros, mensagem, created_at
		FROM sync_logs
		WHERE fonte_dados_id = $1
		ORDER BY iniciado_em DESC
		LIMIT 1
	`, fonteDadosID).Scan(
		&log.ID, &log.FonteDadosID, &log.Tipo, &log.Status, &log.IniciadoEm, &log.FinalizadoEm,
		&log.TotalProjetos, &log.TotalTarefas, &log.TotalMembros, &log.TotalSprints,
		&log.Erros, &log.Mensagem, &log.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting latest sync log: %w", err)
	}
	return &log, nil
}

func (r *SyncRepository) ListSyncLogs(ctx context.Context, fonteDadosID uuid.UUID, limit int) ([]domain.SyncLog, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, tipo, status, iniciado_em, finalizado_em,
		       total_projetos, total_tarefas, total_membros, total_sprints,
		       erros, mensagem, created_at
		FROM sync_logs
		WHERE fonte_dados_id = $1
		ORDER BY iniciado_em DESC
		LIMIT $2
	`, fonteDadosID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing sync logs: %w", err)
	}
	defer rows.Close()

	logs := make([]domain.SyncLog, 0)
	for rows.Next() {
		var log domain.SyncLog
		if err := rows.Scan(
			&log.ID, &log.FonteDadosID, &log.Tipo, &log.Status, &log.IniciadoEm, &log.FinalizadoEm,
			&log.TotalProjetos, &log.TotalTarefas, &log.TotalMembros, &log.TotalSprints,
			&log.Erros, &log.Mensagem, &log.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning sync log: %w", err)
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *SyncRepository) GetProjectKeysForSync(ctx context.Context, fonteDadosID uuid.UUID) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT p.chave
		FROM projetos p
		INNER JOIN tarefas t ON t.projeto_id = p.id
		INNER JOIN equipe_membros em ON em.membro_id = t.responsavel_id
		WHERE p.fonte_dados_id = $1 AND p.ativo = true
	`, fonteDadosID)
	if err != nil {
		return nil, fmt.Errorf("getting project keys for sync: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("scanning project key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (r *SyncRepository) GetFonteDadosAtivas(ctx context.Context) ([]domain.FonteDados, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, nome, tipo, base_url, auth_type, api_token, user_email,
		       oauth2_client_id, oauth2_client_secret, oauth2_access_token,
		       oauth2_refresh_token, oauth2_token_expiry, custom_field_map,
		       ativo, ultimo_sync, created_at, updated_at
		FROM fonte_dados
		WHERE ativo = true AND tipo = 'jira'
		ORDER BY nome
	`)
	if err != nil {
		return nil, fmt.Errorf("getting active fonte dados: %w", err)
	}
	defer rows.Close()

	result := make([]domain.FonteDados, 0)
	for rows.Next() {
		fd, err := scanFonteDados(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning fonte dados: %w", err)
		}
		result = append(result, fd)
	}
	return result, rows.Err()
}
