package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/emersonpaula83/myplanner/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PlanningRepository struct {
	pool *pgxpool.Pool
}

func NewPlanningRepository(pool *pgxpool.Pool) *PlanningRepository {
	return &PlanningRepository{pool: pool}
}

func (r *PlanningRepository) GetNextSprint(ctx context.Context, boardID int, currentDataInicio time.Time) (*domain.Sprint, error) {
	var s domain.Sprint
	err := r.pool.QueryRow(ctx, `
		SELECT id, fonte_dados_id, projeto_id, jira_id, nome, estado,
		       data_inicio, data_fim, data_conclusao, board_id, created_at, updated_at
		FROM sprints
		WHERE board_id = $1 AND data_inicio > $2 AND estado IN ('future', 'active')
		ORDER BY data_inicio ASC
		LIMIT 1
	`, boardID, currentDataInicio).Scan(
		&s.ID, &s.FonteDadosID, &s.ProjetoID, &s.JiraID, &s.Nome, &s.Estado,
		&s.DataInicio, &s.DataFim, &s.DataConclusao, &s.BoardID, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting next sprint for board %d: %w", boardID, err)
	}
	return &s, nil
}

type PlanningTarefa struct {
	ID              uuid.UUID  `json:"id"`
	NumeroTicket    string     `json:"numero_ticket"`
	Resumo          string     `json:"resumo"`
	Tipo            string     `json:"tipo"`
	Status          string     `json:"status"`
	Prioridade      *string    `json:"prioridade"`
	EstimativaTempo *int       `json:"estimativa_tempo"`
	TipoDemanda     *string    `json:"tipo_demanda"`
	ResponsavelID   *uuid.UUID `json:"responsavel_id"`
	ProjetoID       uuid.UUID  `json:"projeto_id"`
	ProjetoChave    string     `json:"projeto_chave"`
}

func (r *PlanningRepository) GetAllTarefasBySprint(ctx context.Context, sprintID uuid.UUID) ([]PlanningTarefa, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.status, t.prioridade,
		       t.estimativa_tempo, t.tipo_demanda, t.responsavel_id,
		       p.id, p.chave
		FROM tarefas t
		INNER JOIN projetos p ON p.id = t.projeto_id
		WHERE t.sprint_id = $1 AND t.removido_em IS NULL
		ORDER BY t.numero_ticket
	`, sprintID)
	if err != nil {
		return nil, fmt.Errorf("getting planning tarefas: %w", err)
	}
	defer rows.Close()

	var result []PlanningTarefa
	for rows.Next() {
		var t PlanningTarefa
		if err := rows.Scan(&t.ID, &t.NumeroTicket, &t.Resumo, &t.Tipo, &t.Status,
			&t.Prioridade, &t.EstimativaTempo, &t.TipoDemanda, &t.ResponsavelID,
			&t.ProjetoID, &t.ProjetoChave); err != nil {
			return nil, fmt.Errorf("scanning planning tarefa: %w", err)
		}
		result = append(result, t)
	}
	return result, nil
}

func (r *PlanningRepository) UpdateTarefaEstimativa(ctx context.Context, tarefaID uuid.UUID, segundos int) error {
	_, err := r.pool.Exec(ctx, `UPDATE tarefas SET estimativa_tempo = $2, updated_at = NOW() WHERE id = $1`, tarefaID, segundos)
	if err != nil {
		return fmt.Errorf("updating estimativa for %s: %w", tarefaID, err)
	}
	return nil
}

func (r *PlanningRepository) UpdateTarefaTipoDemanda(ctx context.Context, tarefaID uuid.UUID, valor string) error {
	_, err := r.pool.Exec(ctx, `UPDATE tarefas SET tipo_demanda = $2, updated_at = NOW() WHERE id = $1`, tarefaID, valor)
	if err != nil {
		return fmt.Errorf("updating tipo_demanda for %s: %w", tarefaID, err)
	}
	return nil
}

func (r *PlanningRepository) UpdateTarefaResponsavel(ctx context.Context, tarefaID uuid.UUID, responsavelID *uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE tarefas SET responsavel_id = $2, updated_at = NOW() WHERE id = $1`, tarefaID, responsavelID)
	if err != nil {
		return fmt.Errorf("updating responsavel for %s: %w", tarefaID, err)
	}
	return nil
}

func (r *PlanningRepository) MoveTarefaToSprint(ctx context.Context, tarefaID uuid.UUID, sprintID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE tarefas SET sprint_id = $2, updated_at = NOW() WHERE id = $1`, tarefaID, sprintID)
	if err != nil {
		return fmt.Errorf("moving tarefa %s to sprint %s: %w", tarefaID, sprintID, err)
	}
	return nil
}

func (r *PlanningRepository) RemoveTarefaFromSprint(ctx context.Context, tarefaID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE tarefas SET sprint_id = NULL, updated_at = NOW() WHERE id = $1`, tarefaID)
	if err != nil {
		return fmt.Errorf("removing tarefa %s from sprint: %w", tarefaID, err)
	}
	return nil
}

func (r *PlanningRepository) GetSprintJiraID(ctx context.Context, sprintID uuid.UUID) (int, error) {
	var jiraID int
	err := r.pool.QueryRow(ctx, `SELECT jira_id FROM sprints WHERE id = $1`, sprintID).Scan(&jiraID)
	if err != nil {
		return 0, fmt.Errorf("getting sprint jira_id for %s: %w", sprintID, err)
	}
	return jiraID, nil
}

type SearchTarefaResult struct {
	ID              uuid.UUID  `json:"id"`
	NumeroTicket    string     `json:"key"`
	Resumo          string     `json:"resumo"`
	Tipo            string     `json:"tipo"`
	Status          string     `json:"status"`
	Prioridade      *string    `json:"prioridade"`
	SprintID        *uuid.UUID `json:"-"`
	ResponsavelNome *string    `json:"responsavel_nome"`
}

func (r *PlanningRepository) SearchTarefasByKeys(ctx context.Context, projetoID uuid.UUID, keys []string) ([]SearchTarefaResult, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.status, t.prioridade, t.sprint_id, m.nome
		FROM tarefas t
		LEFT JOIN membros m ON m.id = t.responsavel_id
		WHERE t.projeto_id = $1 AND t.numero_ticket = ANY($2) AND t.removido_em IS NULL
	`, projetoID, keys)
	if err != nil {
		return nil, fmt.Errorf("searching tarefas by keys: %w", err)
	}
	defer rows.Close()

	var result []SearchTarefaResult
	for rows.Next() {
		var t SearchTarefaResult
		if err := rows.Scan(&t.ID, &t.NumeroTicket, &t.Resumo, &t.Tipo, &t.Status, &t.Prioridade, &t.SprintID, &t.ResponsavelNome); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating search results: %w", err)
	}
	return result, nil
}

func (r *PlanningRepository) UpsertTarefaFromJira(ctx context.Context, t *UpsertTarefaParams) (uuid.UUID, error) {
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
		                     status_categoria, campos_extras, parent_id, apelido, data_inicio_execucao,
		                     data_entrada_sprint, marcacao)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
		        $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28)
		ON CONFLICT (fonte_dados_id, jira_id)
		DO UPDATE SET resumo = EXCLUDED.resumo, tipo = EXCLUDED.tipo, status = EXCLUDED.status,
		              prioridade = EXCLUDED.prioridade, estimativa_pontos = EXCLUDED.estimativa_pontos,
		              estimativa_tempo = EXCLUDED.estimativa_tempo, tempo_gasto = EXCLUDED.tempo_gasto,
		              responsavel_id = EXCLUDED.responsavel_id, relator_id = EXCLUDED.relator_id,
		              team = EXCLUDED.team,
		              data_resolvido = EXCLUDED.data_resolvido,
		              data_atualizado = EXCLUDED.data_atualizado, tipo_demanda = EXCLUDED.tipo_demanda,
		              status_categoria = EXCLUDED.status_categoria,
		              campos_extras = EXCLUDED.campos_extras, updated_at = NOW()
		RETURNING id
	`, t.FonteDadosID, t.ProjetoID, t.JiraID, t.NumeroTicket, t.Resumo,
		t.Tipo, t.Status, t.Prioridade, t.EstimativaPontos, t.EstimativaTempo, t.TempoGasto,
		t.ResponsavelID, t.RelatorID, t.Team, t.SprintID, t.DataCriacao, t.DataLimite,
		t.DataResolvido, t.DataAtualizado, t.TipoDemanda, t.DataComponente,
		t.StatusCategoria, ce, t.ParentID, t.Apelido, t.DataInicioExecucao,
		t.DataEntradaSprint, t.Marcacao).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upserting tarefa from jira %s: %w", t.NumeroTicket, err)
	}
	return id, nil
}

func (r *PlanningRepository) MoveTarefasToSprint(ctx context.Context, sprintID uuid.UUID, tarefaIDs []uuid.UUID) error {
	if len(tarefaIDs) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE tarefas SET sprint_id = $1, data_entrada_sprint = NOW(), updated_at = NOW()
		WHERE id = ANY($2)
	`, sprintID, tarefaIDs)
	if err != nil {
		return fmt.Errorf("moving tarefas to sprint %s: %w", sprintID, err)
	}
	return nil
}

func (r *PlanningRepository) GetTarefasByIDs(ctx context.Context, ids []uuid.UUID) ([]PlanningTarefa, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.numero_ticket, t.resumo, t.tipo, t.status, t.prioridade,
		       t.estimativa_tempo, t.tipo_demanda, t.responsavel_id,
		       p.id, p.chave
		FROM tarefas t
		INNER JOIN projetos p ON p.id = t.projeto_id
		WHERE t.id = ANY($1) AND t.removido_em IS NULL
		ORDER BY t.numero_ticket
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("getting tarefas by ids: %w", err)
	}
	defer rows.Close()

	var result []PlanningTarefa
	for rows.Next() {
		var t PlanningTarefa
		if err := rows.Scan(&t.ID, &t.NumeroTicket, &t.Resumo, &t.Tipo, &t.Status,
			&t.Prioridade, &t.EstimativaTempo, &t.TipoDemanda, &t.ResponsavelID,
			&t.ProjetoID, &t.ProjetoChave); err != nil {
			return nil, fmt.Errorf("scanning tarefa by id: %w", err)
		}
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tarefas by ids: %w", err)
	}
	return result, nil
}

func (r *PlanningRepository) GetProjetoChaveByID(ctx context.Context, projetoID uuid.UUID) (string, uuid.UUID, error) {
	var chave string
	var fonteDadosID uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT chave, fonte_dados_id FROM projetos WHERE id = $1`, projetoID).Scan(&chave, &fonteDadosID)
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("getting projeto chave for %s: %w", projetoID, err)
	}
	return chave, fonteDadosID, nil
}
