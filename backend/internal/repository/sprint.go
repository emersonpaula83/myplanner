package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

type SprintRepository struct {
	pool *pgxpool.Pool
}

func NewSprintRepository(pool *pgxpool.Pool) *SprintRepository {
	return &SprintRepository{pool: pool}
}

type SprintListItem struct {
	ID           uuid.UUID  `json:"id"`
	Nome         string     `json:"nome"`
	Estado       *string    `json:"estado"`
	DataInicio   *time.Time `json:"data_inicio"`
	DataFim      *time.Time `json:"data_fim"`
	TotalTarefas int        `json:"total_tarefas"`
	ProjetoChave *string    `json:"projeto_chave,omitempty"`
	ProjetoNome  *string    `json:"projeto_nome,omitempty"`
}

type ProjetoComSprints struct {
	ID    uuid.UUID `json:"id"`
	Chave string    `json:"chave"`
	Nome  string    `json:"nome"`
}

func (r *SprintRepository) ListProjetosComSprints(ctx context.Context, equipeID *uuid.UUID) ([]ProjetoComSprints, error) {
	var query string
	var args []interface{}

	if equipeID != nil {
		query = `
			SELECT DISTINCT p.id, p.chave, p.nome
			FROM projetos p
			INNER JOIN sprints s ON s.projeto_id = p.id
			INNER JOIN tarefas t ON t.projeto_id = p.id
			INNER JOIN equipe_membros em ON em.membro_id = t.responsavel_id
			WHERE p.ativo = true AND em.equipe_id = $1
			ORDER BY p.nome
		`
		args = []interface{}{*equipeID}
	} else {
		query = `
			SELECT DISTINCT p.id, p.chave, p.nome
			FROM projetos p
			INNER JOIN sprints s ON s.projeto_id = p.id
			WHERE p.ativo = true
			ORDER BY p.nome
		`
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing projetos com sprints: %w", err)
	}
	defer rows.Close()

	result := make([]ProjetoComSprints, 0)
	for rows.Next() {
		var p ProjetoComSprints
		if err := rows.Scan(&p.ID, &p.Chave, &p.Nome); err != nil {
			return nil, fmt.Errorf("scanning projeto: %w", err)
		}
		result = append(result, p)
	}
	return result, nil
}

func (r *SprintRepository) ListByProjeto(ctx context.Context, projetoID uuid.UUID, estado *string) ([]SprintListItem, error) {
	query := `
		SELECT s.id, s.nome, s.estado, s.data_inicio, s.data_fim,
		       COUNT(t.id) AS total_tarefas
		FROM sprints s
		LEFT JOIN tarefas t ON t.sprint_id = s.id
		WHERE s.projeto_id = $1
	`
	args := []interface{}{projetoID}

	if estado != nil && *estado != "" {
		query += " AND s.estado = $2"
		args = append(args, *estado)
	}

	query += `
		GROUP BY s.id, s.nome, s.estado, s.data_inicio, s.data_fim
		ORDER BY s.data_inicio DESC NULLS LAST
	`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing sprints by projeto: %w", err)
	}
	defer rows.Close()

	result := make([]SprintListItem, 0)
	for rows.Next() {
		var item SprintListItem
		if err := rows.Scan(&item.ID, &item.Nome, &item.Estado, &item.DataInicio, &item.DataFim, &item.TotalTarefas); err != nil {
			return nil, fmt.Errorf("scanning sprint: %w", err)
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *SprintRepository) ListSprints(ctx context.Context, equipeID *uuid.UUID, estado *string) ([]SprintListItem, error) {
	query := `
		SELECT s.id, s.nome, s.estado, s.data_inicio, s.data_fim,
		       (SELECT COUNT(*) FROM tarefas t WHERE t.sprint_id = s.id) AS total_tarefas,
		       p.chave, p.nome
		FROM sprints s
		INNER JOIN projetos p ON p.id = s.projeto_id
		WHERE 1=1
	`
	args := make([]interface{}, 0)
	argN := 1

	if equipeID != nil {
		query += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM tarefas t2
			INNER JOIN equipe_membros em ON em.membro_id = t2.responsavel_id
			WHERE t2.sprint_id = s.id AND em.equipe_id = $%d
		)`, argN)
		args = append(args, *equipeID)
		argN++
	}

	if estado != nil && *estado != "" {
		query += fmt.Sprintf(" AND s.estado = $%d", argN)
		args = append(args, *estado)
		argN++
	}

	query += " ORDER BY s.data_inicio DESC NULLS LAST"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing sprints: %w", err)
	}
	defer rows.Close()

	result := make([]SprintListItem, 0)
	for rows.Next() {
		var item SprintListItem
		if err := rows.Scan(&item.ID, &item.Nome, &item.Estado, &item.DataInicio, &item.DataFim, &item.TotalTarefas, &item.ProjetoChave, &item.ProjetoNome); err != nil {
			return nil, fmt.Errorf("scanning sprint: %w", err)
		}
		result = append(result, item)
	}
	return result, nil
}

func (r *SprintRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Sprint, error) {
	var s domain.Sprint
	err := r.pool.QueryRow(ctx, `
		SELECT id, fonte_dados_id, projeto_id, jira_id, nome, estado, data_inicio, data_fim, data_conclusao, board_id, created_at, updated_at
		FROM sprints WHERE id = $1
	`, id).Scan(&s.ID, &s.FonteDadosID, &s.ProjetoID, &s.JiraID, &s.Nome, &s.Estado, &s.DataInicio, &s.DataFim, &s.DataConclusao, &s.BoardID, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting sprint %s: %w", id, err)
	}
	return &s, nil
}

type TarefaCapacity struct {
	ResponsavelID uuid.UUID
	Segundos      int64
}

func (r *SprintRepository) GetTarefasCapacityBySprint(ctx context.Context, sprintID uuid.UUID) ([]TarefaCapacity, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT responsavel_id, COALESCE(estimativa_tempo, 0)
		FROM tarefas
		WHERE sprint_id = $1 AND responsavel_id IS NOT NULL
	`, sprintID)
	if err != nil {
		return nil, fmt.Errorf("getting tarefas capacity: %w", err)
	}
	defer rows.Close()

	result := make([]TarefaCapacity, 0)
	for rows.Next() {
		var tc TarefaCapacity
		if err := rows.Scan(&tc.ResponsavelID, &tc.Segundos); err != nil {
			return nil, fmt.Errorf("scanning tarefa capacity: %w", err)
		}
		result = append(result, tc)
	}
	return result, nil
}

type MembroInfo struct {
	ID        uuid.UUID
	Nome      string
	AvatarURL *string
}

func (r *SprintRepository) GetMembrosFromSprint(ctx context.Context, sprintID uuid.UUID) ([]MembroInfo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT m.id, m.nome, m.avatar_url
		FROM membros m
		INNER JOIN tarefas t ON t.responsavel_id = m.id
		WHERE t.sprint_id = $1
		ORDER BY m.nome
	`, sprintID)
	if err != nil {
		return nil, fmt.Errorf("getting membros from sprint: %w", err)
	}
	defer rows.Close()

	result := make([]MembroInfo, 0)
	for rows.Next() {
		var m MembroInfo
		if err := rows.Scan(&m.ID, &m.Nome, &m.AvatarURL); err != nil {
			return nil, fmt.Errorf("scanning membro info: %w", err)
		}
		result = append(result, m)
	}
	return result, nil
}

type AusenciaRecord struct {
	MembroID   uuid.UUID
	Tipo       string
	DataInicio time.Time
	DataFim    time.Time
}

func (r *SprintRepository) GetAusenciasNoPeriodo(ctx context.Context, membroIDs []uuid.UUID, inicio, fim time.Time) ([]AusenciaRecord, error) {
	if len(membroIDs) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT membro_id, tipo, data_inicio, data_fim
		FROM disponibilidade
		WHERE membro_id = ANY($1)
		  AND data_inicio <= $3
		  AND data_fim >= $2
	`, membroIDs, inicio, fim)
	if err != nil {
		return nil, fmt.Errorf("getting ausencias: %w", err)
	}
	defer rows.Close()

	result := make([]AusenciaRecord, 0)
	for rows.Next() {
		var a AusenciaRecord
		if err := rows.Scan(&a.MembroID, &a.Tipo, &a.DataInicio, &a.DataFim); err != nil {
			return nil, fmt.Errorf("scanning ausencia: %w", err)
		}
		result = append(result, a)
	}
	return result, nil
}
