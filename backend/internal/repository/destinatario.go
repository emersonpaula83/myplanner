package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Destinatario struct {
	ID        uuid.UUID `json:"id"`
	EquipeID  uuid.UUID `json:"equipe_id"`
	Tipo      string    `json:"tipo"`
	Valor     string    `json:"valor"`
	Nome      *string   `json:"nome,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type DestinatarioRepository struct {
	pool *pgxpool.Pool
}

func NewDestinatarioRepository(pool *pgxpool.Pool) *DestinatarioRepository {
	return &DestinatarioRepository{pool: pool}
}

func (r *DestinatarioRepository) ListByEquipe(ctx context.Context, equipeID uuid.UUID) ([]Destinatario, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, equipe_id, tipo, valor, nome, created_at
		FROM review_destinatarios
		WHERE equipe_id = $1
		ORDER BY tipo, nome, valor
	`, equipeID)
	if err != nil {
		return nil, fmt.Errorf("listing destinatarios: %w", err)
	}
	defer rows.Close()

	var result []Destinatario
	for rows.Next() {
		var d Destinatario
		if err := rows.Scan(&d.ID, &d.EquipeID, &d.Tipo, &d.Valor, &d.Nome, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning destinatario: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (r *DestinatarioRepository) Create(ctx context.Context, equipeID uuid.UUID, tipo, valor string, nome *string) (*Destinatario, error) {
	var d Destinatario
	err := r.pool.QueryRow(ctx, `
		INSERT INTO review_destinatarios (equipe_id, tipo, valor, nome)
		VALUES ($1, $2, $3, $4)
		RETURNING id, equipe_id, tipo, valor, nome, created_at
	`, equipeID, tipo, valor, nome).Scan(&d.ID, &d.EquipeID, &d.Tipo, &d.Valor, &d.Nome, &d.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating destinatario: %w", err)
	}
	return &d, nil
}

func (r *DestinatarioRepository) Delete(ctx context.Context, id uuid.UUID, equipeID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM review_destinatarios WHERE id = $1 AND equipe_id = $2`, id, equipeID)
	if err != nil {
		return fmt.Errorf("deleting destinatario: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("destinatario not found")
	}
	return nil
}

func (r *DestinatarioRepository) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]Destinatario, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, equipe_id, tipo, valor, nome, created_at
		FROM review_destinatarios
		WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("getting destinatarios by ids: %w", err)
	}
	defer rows.Close()

	var result []Destinatario
	for rows.Next() {
		var d Destinatario
		if err := rows.Scan(&d.ID, &d.EquipeID, &d.Tipo, &d.Valor, &d.Nome, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning destinatario: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}
