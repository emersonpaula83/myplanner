package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AllocationRepository struct {
	pool *pgxpool.Pool
}

func NewAllocationRepository(pool *pgxpool.Pool) *AllocationRepository {
	return &AllocationRepository{pool: pool}
}

type EpicAllocationRow struct {
	EpicID              uuid.UUID
	NumeroTicket        string
	Resumo              string
	Apelido             *string
	DataLimite          *time.Time
	Prioridade          *string
	TipoDemanda         *string
	Produtos            []string
	TotalFilhas         int
	FilhasComEstimativa int
	HorasEstimadas      float64
	HorasEmSprint       float64
}

type TaskAllocationRow struct {
	TarefaID        uuid.UUID
	NumeroTicket    string
	Resumo          string
	Tipo            string
	TipoDemanda     *string
	Status          string
	EstimativaTempo *int
	SprintID        *uuid.UUID
	SprintNome      *string
	SprintInicio    *time.Time
	SprintFim       *time.Time
	ResponsavelID   *uuid.UUID
	ResponsavelNome *string
}

type PersonAllocationRow struct {
	MembroID       uuid.UUID
	Nome           string
	HorasNoProjeto float64
}

type SprintOptionRow struct {
	ID     uuid.UUID
	JiraID int
	Nome   string
	Inicio time.Time
	Fim    time.Time
	Estado string
}

type TaskPreviousState struct {
	SprintID      *uuid.UUID
	ResponsavelID *uuid.UUID
	Estimativa    *int
}

func (r *AllocationRepository) GetEpicsByEquipeAndProduto(ctx context.Context, equipeID, produtoID uuid.UUID) ([]EpicAllocationRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			e.id,
			e.numero_ticket,
			e.resumo,
			e.apelido,
			e.data_limite::timestamptz,
			e.prioridade,
			COALESCE(e.tipo_demanda,
				CASE
					WHEN e.tipo IN ('Épico', 'Projeto') THEN 'Meta'
					WHEN e.tipo IN ('Spike', 'Implantação', 'Aditivo - Delivery') THEN 'Compromisso'
					ELSE 'Iniciativa'
				END
			),
			COALESCE(
				(SELECT ARRAY_AGG(DISTINCT p.nome ORDER BY p.nome)
				 FROM tarefas c
				 JOIN tarefa_produtos tp ON tp.tarefa_id = c.id
				 JOIN produtos p ON p.id = tp.produto_id
				 WHERE c.parent_id = e.id),
				'{}'
			),
			(SELECT COUNT(*) FROM tarefas c WHERE c.parent_id = e.id AND c.status NOT IN ('Cancelado', 'Rejeitada'))::int,
			(SELECT COUNT(*) FROM tarefas c WHERE c.parent_id = e.id AND c.status NOT IN ('Cancelado', 'Rejeitada') AND c.estimativa_tempo IS NOT NULL AND c.estimativa_tempo > 0)::int,
			COALESCE(
				(SELECT SUM(c.estimativa_tempo) FROM tarefas c WHERE c.parent_id = e.id AND c.status NOT IN ('Cancelado', 'Rejeitada') AND c.estimativa_tempo IS NOT NULL AND c.estimativa_tempo > 0),
				0
			)::float8 / 3600.0,
			COALESCE(
				(SELECT SUM(c.estimativa_tempo) FROM tarefas c
				 JOIN sprints s ON s.id = c.sprint_id
				 WHERE c.parent_id = e.id
				   AND c.status NOT IN ('Cancelado', 'Rejeitada')
				   AND c.estimativa_tempo IS NOT NULL AND c.estimativa_tempo > 0
				   AND s.estado IN ('active', 'future')),
				0
			)::float8 / 3600.0
		FROM tarefas e
		WHERE e.tipo IN ('Épico', 'Epico')
		  AND e.fonte_dados_id IN (
			SELECT DISTINCT m.fonte_dados_id
			FROM equipe_membros em
			JOIN membros m ON em.membro_id = m.id
			WHERE em.equipe_id = $1
		  )
		  AND EXISTS (
			SELECT 1 FROM tarefas c
			JOIN tarefa_produtos tp ON tp.tarefa_id = c.id
			WHERE c.parent_id = e.id AND tp.produto_id = $2
		  )
		  AND e.status NOT IN ('Cancelado', 'Rejeitada', 'Concluído')
		ORDER BY
			CASE e.prioridade
				WHEN 'Highest' THEN 1
				WHEN 'High' THEN 2
				WHEN 'Medium' THEN 3
				WHEN 'Low' THEN 4
				WHEN 'Lowest' THEN 5
				ELSE 6
			END,
			CASE
				WHEN COALESCE(e.tipo_demanda, '') = 'Meta' THEN 1
				WHEN COALESCE(e.tipo_demanda, '') = 'Compromisso' THEN 2
				WHEN COALESCE(e.tipo_demanda, '') = 'Iniciativa' THEN 3
				ELSE 4
			END,
			e.numero_ticket
	`, equipeID, produtoID)
	if err != nil {
		return nil, fmt.Errorf("querying epics: %w", err)
	}
	defer rows.Close()

	result := make([]EpicAllocationRow, 0)
	for rows.Next() {
		var e EpicAllocationRow
		if err := rows.Scan(
			&e.EpicID, &e.NumeroTicket, &e.Resumo, &e.Apelido,
			&e.DataLimite, &e.Prioridade, &e.TipoDemanda, &e.Produtos,
			&e.TotalFilhas, &e.FilhasComEstimativa,
			&e.HorasEstimadas, &e.HorasEmSprint,
		); err != nil {
			return nil, fmt.Errorf("scanning epic: %w", err)
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (r *AllocationRepository) GetEpicTasks(ctx context.Context, epicID uuid.UUID) ([]TaskAllocationRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			t.id, t.numero_ticket, t.resumo, t.tipo, t.tipo_demanda, t.status,
			t.estimativa_tempo,
			t.sprint_id, s.nome, s.data_inicio, s.data_fim,
			t.responsavel_id, m.nome
		FROM tarefas t
		LEFT JOIN sprints s ON s.id = t.sprint_id
		LEFT JOIN membros m ON m.id = t.responsavel_id
		WHERE t.parent_id = $1
		  AND t.status NOT IN ('Cancelado', 'Rejeitada')
		ORDER BY t.numero_ticket
	`, epicID)
	if err != nil {
		return nil, fmt.Errorf("querying epic tasks: %w", err)
	}
	defer rows.Close()

	result := make([]TaskAllocationRow, 0)
	for rows.Next() {
		var t TaskAllocationRow
		if err := rows.Scan(
			&t.TarefaID, &t.NumeroTicket, &t.Resumo, &t.Tipo, &t.TipoDemanda, &t.Status,
			&t.EstimativaTempo,
			&t.SprintID, &t.SprintNome, &t.SprintInicio, &t.SprintFim,
			&t.ResponsavelID, &t.ResponsavelNome,
		); err != nil {
			return nil, fmt.Errorf("scanning task: %w", err)
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func (r *AllocationRepository) GetEpicPeople(ctx context.Context, epicID uuid.UUID) ([]PersonAllocationRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			m.id,
			m.nome,
			COALESCE(SUM(t.estimativa_tempo), 0)::float8 / 3600.0
		FROM tarefas t
		JOIN membros m ON m.id = t.responsavel_id
		WHERE t.parent_id = $1
		  AND t.status NOT IN ('Cancelado', 'Rejeitada')
		GROUP BY m.id, m.nome
		ORDER BY SUM(t.estimativa_tempo) DESC NULLS LAST
	`, epicID)
	if err != nil {
		return nil, fmt.Errorf("querying epic people: %w", err)
	}
	defer rows.Close()

	result := make([]PersonAllocationRow, 0)
	for rows.Next() {
		var p PersonAllocationRow
		if err := rows.Scan(&p.MembroID, &p.Nome, &p.HorasNoProjeto); err != nil {
			return nil, fmt.Errorf("scanning person: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *AllocationRepository) GetTaskPreviousState(ctx context.Context, taskID uuid.UUID) (*TaskPreviousState, error) {
	var s TaskPreviousState
	err := r.pool.QueryRow(ctx, `
		SELECT sprint_id, responsavel_id, estimativa_tempo FROM tarefas WHERE id = $1
	`, taskID).Scan(&s.SprintID, &s.ResponsavelID, &s.Estimativa)
	if err != nil {
		return nil, fmt.Errorf("getting task previous state: %w", err)
	}
	return &s, nil
}

func (r *AllocationRepository) UpdateTaskAllocation(ctx context.Context, taskID, sprintID uuid.UUID, assigneeID *uuid.UUID, estimateSeconds int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tarefas
		SET sprint_id = $2, responsavel_id = $3, estimativa_tempo = $4, updated_at = NOW()
		WHERE id = $1
	`, taskID, sprintID, assigneeID, estimateSeconds)
	if err != nil {
		return fmt.Errorf("updating task allocation: %w", err)
	}
	return nil
}

func (r *AllocationRepository) RollbackTaskAllocation(ctx context.Context, taskID uuid.UUID, prev *TaskPreviousState) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tarefas
		SET sprint_id = $2, responsavel_id = $3, estimativa_tempo = $4, updated_at = NOW()
		WHERE id = $1
	`, taskID, prev.SprintID, prev.ResponsavelID, prev.Estimativa)
	if err != nil {
		return fmt.Errorf("rolling back task allocation: %w", err)
	}
	return nil
}

func (r *AllocationRepository) GetFutureSprintsByEquipe(ctx context.Context, equipeID uuid.UUID) ([]SprintOptionRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT s.id, s.jira_id, s.nome, s.data_inicio, s.data_fim, COALESCE(s.estado, 'future')
		FROM sprints s
		JOIN equipes eq ON eq.board_id = s.board_id
		WHERE eq.id = $1
		  AND s.estado IN ('active', 'future')
		  AND s.data_inicio IS NOT NULL AND s.data_fim IS NOT NULL
		ORDER BY s.data_inicio
	`, equipeID)
	if err != nil {
		return nil, fmt.Errorf("querying future sprints: %w", err)
	}
	defer rows.Close()

	result := make([]SprintOptionRow, 0)
	for rows.Next() {
		var s SprintOptionRow
		if err := rows.Scan(&s.ID, &s.JiraID, &s.Nome, &s.Inicio, &s.Fim, &s.Estado); err != nil {
			return nil, fmt.Errorf("scanning sprint: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *AllocationRepository) CheckGDPTCAncestors(ctx context.Context, epicIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if len(epicIDs) == 0 {
		return make(map[uuid.UUID]bool), nil
	}

	rows, err := r.pool.Query(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT t.id AS original_id, t.id, t.parent_id, t.numero_ticket, 1 AS depth
			FROM tarefas t WHERE t.id = ANY($1)
			UNION ALL
			SELECT a.original_id, p.id, p.parent_id, p.numero_ticket, a.depth + 1
			FROM tarefas p JOIN ancestors a ON p.id = a.parent_id
			WHERE a.depth < 10
		)
		SELECT DISTINCT original_id FROM ancestors
		WHERE numero_ticket LIKE 'GDPTC-%' AND original_id != id
	`, epicIDs)
	if err != nil {
		return nil, fmt.Errorf("querying GDPTC ancestors: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]bool)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning GDPTC id: %w", err)
		}
		result[id] = true
	}
	return result, rows.Err()
}

func (r *AllocationRepository) GetTaskFonteDadosID(ctx context.Context, taskID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT fonte_dados_id FROM tarefas WHERE id = $1`, taskID).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("getting task fonte_dados_id: %w", err)
	}
	return id, nil
}

func (r *AllocationRepository) GetSprintJiraID(ctx context.Context, sprintID uuid.UUID) (int, error) {
	var jiraID int
	err := r.pool.QueryRow(ctx, `SELECT jira_id FROM sprints WHERE id = $1`, sprintID).Scan(&jiraID)
	if err != nil {
		return 0, fmt.Errorf("getting sprint jira_id: %w", err)
	}
	return jiraID, nil
}

func (r *AllocationRepository) GetTaskJiraKey(ctx context.Context, taskID uuid.UUID) (string, error) {
	var key string
	err := r.pool.QueryRow(ctx, `SELECT numero_ticket FROM tarefas WHERE id = $1`, taskID).Scan(&key)
	if err != nil {
		return "", fmt.Errorf("getting task jira key: %w", err)
	}
	return key, nil
}
