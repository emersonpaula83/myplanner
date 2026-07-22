package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

func escapeILIKE(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

type SkillRepository struct {
	pool *pgxpool.Pool
}

func NewSkillRepository(pool *pgxpool.Pool) *SkillRepository {
	return &SkillRepository{pool: pool}
}

func (r *SkillRepository) List(ctx context.Context, query string) ([]domain.Skill, error) {
	var rows pgx.Rows
	var err error
	if query == "" {
		rows, err = r.pool.Query(ctx, `
			SELECT id, nome, created_at, updated_at FROM skills ORDER BY nome LIMIT 50
		`)
	} else {
		query = escapeILIKE(query)
		rows, err = r.pool.Query(ctx, `
			SELECT id, nome, created_at, updated_at FROM skills
			WHERE nome ILIKE '%' || $1 || '%'
			ORDER BY nome LIMIT 50
		`, query)
	}
	if err != nil {
		return nil, fmt.Errorf("listing skills: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Skill, 0)
	for rows.Next() {
		var s domain.Skill
		if err := rows.Scan(&s.ID, &s.Nome, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning skill: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *SkillRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Skill, error) {
	var s domain.Skill
	err := r.pool.QueryRow(ctx, `
		SELECT id, nome, created_at, updated_at FROM skills WHERE id = $1
	`, id).Scan(&s.ID, &s.Nome, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting skill: %w", err)
	}
	return &s, nil
}

func (r *SkillRepository) Create(ctx context.Context, nome string) (*domain.Skill, error) {
	var s domain.Skill
	err := r.pool.QueryRow(ctx, `
		INSERT INTO skills (nome) VALUES ($1)
		ON CONFLICT ((LOWER(nome))) DO UPDATE SET updated_at = skills.updated_at
		RETURNING id, nome, created_at, updated_at
	`, nome).Scan(&s.ID, &s.Nome, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating skill: %w", err)
	}
	return &s, nil
}

func (r *SkillRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM skills WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting skill: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("skill %s not found", id)
	}
	return nil
}

func (r *SkillRepository) GetMembroSkills(ctx context.Context, membroID uuid.UUID) ([]domain.Skill, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.nome, s.created_at, s.updated_at
		FROM skills s
		INNER JOIN membro_skills ms ON ms.skill_id = s.id
		WHERE ms.membro_id = $1
		ORDER BY s.nome
	`, membroID)
	if err != nil {
		return nil, fmt.Errorf("getting membro skills: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Skill, 0)
	for rows.Next() {
		var s domain.Skill
		if err := rows.Scan(&s.ID, &s.Nome, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning membro skill: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *SkillRepository) AddMembroSkill(ctx context.Context, membroID, skillID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO membro_skills (membro_id, skill_id) VALUES ($1, $2)
		ON CONFLICT (membro_id, skill_id) DO NOTHING
	`, membroID, skillID)
	if err != nil {
		return fmt.Errorf("adding skill to membro: %w", err)
	}
	return nil
}

func (r *SkillRepository) RemoveMembroSkill(ctx context.Context, membroID, skillID uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `
		DELETE FROM membro_skills WHERE membro_id = $1 AND skill_id = $2
	`, membroID, skillID)
	if err != nil {
		return fmt.Errorf("removing skill from membro: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("skill %s not associated with membro %s", skillID, membroID)
	}
	return nil
}
