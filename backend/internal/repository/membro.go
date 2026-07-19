package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
)

type MembroRepository struct {
	pool *pgxpool.Pool
}

func NewMembroRepository(pool *pgxpool.Pool) *MembroRepository {
	return &MembroRepository{pool: pool}
}

func (r *MembroRepository) List(ctx context.Context) ([]domain.Membro, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo, created_at, updated_at
		FROM membros
		WHERE ativo = true
		ORDER BY nome
	`)
	if err != nil {
		return nil, fmt.Errorf("listing membros: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Membro, 0)
	for rows.Next() {
		var m domain.Membro
		if err := rows.Scan(&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email, &m.AvatarURL, &m.Team, &m.Ativo, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning membro: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (r *MembroRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Membro, error) {
	var m domain.Membro
	err := r.pool.QueryRow(ctx, `
		SELECT id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo, created_at, updated_at
		FROM membros WHERE id = $1
	`, id).Scan(&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email, &m.AvatarURL, &m.Team, &m.Ativo, &m.CreatedAt, &m.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting membro: %w", err)
	}
	return &m, nil
}

func (r *MembroRepository) ListDisponibilidade(ctx context.Context, membroID uuid.UUID) ([]domain.Disponibilidade, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, membro_id, tipo, data_inicio, data_fim, descricao, criado_por, created_at, updated_at
		FROM disponibilidade
		WHERE membro_id = $1
		ORDER BY data_inicio DESC
	`, membroID)
	if err != nil {
		return nil, fmt.Errorf("listing disponibilidade: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Disponibilidade, 0)
	for rows.Next() {
		var d domain.Disponibilidade
		if err := rows.Scan(&d.ID, &d.MembroID, &d.Tipo, &d.DataInicio, &d.DataFim, &d.Descricao, &d.CriadoPor, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning disponibilidade: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (r *MembroRepository) CreateDisponibilidade(ctx context.Context, d *domain.Disponibilidade) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO disponibilidade (id, membro_id, tipo, data_inicio, data_fim, descricao, criado_por)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, d.ID, d.MembroID, d.Tipo, d.DataInicio, d.DataFim, d.Descricao, d.CriadoPor)
	if err != nil {
		return fmt.Errorf("creating disponibilidade: %w", err)
	}
	return nil
}

func (r *MembroRepository) UpdateDisponibilidade(ctx context.Context, id uuid.UUID, tipo string, dataInicio, dataFim pgtype.Date, descricao *string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE disponibilidade SET tipo = $2, data_inicio = $3, data_fim = $4, descricao = $5, updated_at = NOW()
		WHERE id = $1
	`, id, tipo, dataInicio, dataFim, descricao)
	if err != nil {
		return fmt.Errorf("updating disponibilidade: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("disponibilidade %s not found", id)
	}
	return nil
}

func (r *MembroRepository) DeleteDisponibilidade(ctx context.Context, id uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM disponibilidade WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting disponibilidade: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("disponibilidade %s not found", id)
	}
	return nil
}

func (r *MembroRepository) GetMembroStats(ctx context.Context, membroID uuid.UUID, inicio, fim time.Time) (*domain.MembroStats, error) {
	var stats domain.MembroStats

	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tarefas WHERE responsavel_id = $1
		  AND COALESCE(data_atualizado, data_criacao) >= $2
		  AND COALESCE(data_atualizado, data_criacao) < $3
	`, membroID, inicio, fim).Scan(&stats.TotalTarefas)
	if err != nil {
		return nil, fmt.Errorf("counting tarefas: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tarefas WHERE responsavel_id = $1 AND status_categoria = 'done'
		  AND COALESCE(data_atualizado, data_criacao) >= $2
		  AND COALESCE(data_atualizado, data_criacao) < $3
	`, membroID, inicio, fim).Scan(&stats.TarefasConcluidas)
	if err != nil {
		return nil, fmt.Errorf("counting tarefas concluidas: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tarefas WHERE responsavel_id = $1 AND status_categoria = 'indeterminate'
		  AND COALESCE(data_atualizado, data_criacao) >= $2
		  AND COALESCE(data_atualizado, data_criacao) < $3
	`, membroID, inicio, fim).Scan(&stats.TarefasEmAndamento)
	if err != nil {
		return nil, fmt.Errorf("counting tarefas em andamento: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(
			GREATEST(0,
				LEAST(data_fim, $3::date) - GREATEST(data_inicio, $2::date) + 1
			)
		), 0)
		FROM disponibilidade
		WHERE membro_id = $1 AND data_inicio <= $3 AND data_fim >= $2
	`, membroID, inicio, fim).Scan(&stats.DiasAusenteAno)
	if err != nil {
		return nil, fmt.Errorf("counting dias ausente: %w", err)
	}

	var segundosEstimados int64
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(estimativa_tempo), 0) FROM tarefas
		WHERE responsavel_id = $1 AND status_categoria != 'done'
		  AND COALESCE(data_atualizado, data_criacao) >= $2
		  AND COALESCE(data_atualizado, data_criacao) < $3
	`, membroID, inicio, fim).Scan(&segundosEstimados)
	if err != nil {
		return nil, fmt.Errorf("counting horas estimadas: %w", err)
	}
	stats.TotalHorasEstimadas = float64(segundosEstimados) / 3600.0

	return &stats, nil
}

func (r *MembroRepository) UpdateTeam(ctx context.Context, id uuid.UUID, team string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE membros SET team = $2, updated_at = NOW() WHERE id = $1
	`, id, team)
	if err != nil {
		return fmt.Errorf("updating membro team: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("membro %s not found", id)
	}
	return nil
}

func (r *MembroRepository) Search(ctx context.Context, query string) ([]domain.Membro, error) {
	pattern := "%" + query + "%"
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, jira_account_id, nome, email, avatar_url, team, ativo, created_at, updated_at
		FROM membros
		WHERE ativo = true AND (nome ILIKE $1 OR email ILIKE $1)
		ORDER BY nome
		LIMIT 50
	`, pattern)
	if err != nil {
		return nil, fmt.Errorf("searching membros: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Membro, 0)
	for rows.Next() {
		var m domain.Membro
		if err := rows.Scan(&m.ID, &m.FonteDadosID, &m.JiraAccountID, &m.Nome, &m.Email, &m.AvatarURL, &m.Team, &m.Ativo, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning membro: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (r *MembroRepository) ListTeams(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT team FROM membros WHERE team IS NOT NULL AND ativo = true ORDER BY team
	`)
	if err != nil {
		return nil, fmt.Errorf("listing teams: %w", err)
	}
	defer rows.Close()

	teams := make([]string, 0)
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scanning team: %w", err)
		}
		teams = append(teams, t)
	}
	return teams, rows.Err()
}
