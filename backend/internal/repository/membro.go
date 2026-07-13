package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
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
