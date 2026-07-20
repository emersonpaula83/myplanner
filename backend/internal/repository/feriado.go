package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Feriado struct {
	ID   uuid.UUID `json:"id"`
	Data string    `json:"data"`
	Nome string    `json:"nome"`
	Tipo string    `json:"tipo"`
}

type FeriadoRepository struct {
	pool *pgxpool.Pool
}

func NewFeriadoRepository(pool *pgxpool.Pool) *FeriadoRepository {
	return &FeriadoRepository{pool: pool}
}

func (r *FeriadoRepository) List(ctx context.Context) ([]Feriado, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, data, nome, tipo FROM feriados ORDER BY data DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing feriados: %w", err)
	}
	defer rows.Close()

	result := make([]Feriado, 0)
	for rows.Next() {
		var f Feriado
		var d time.Time
		if err := rows.Scan(&f.ID, &d, &f.Nome, &f.Tipo); err != nil {
			return nil, fmt.Errorf("scanning feriado: %w", err)
		}
		f.Data = d.Format("2006-01-02")
		result = append(result, f)
	}
	return result, rows.Err()
}

func (r *FeriadoRepository) Create(ctx context.Context, data string, nome string) (*Feriado, error) {
	d, err := time.Parse("2006-01-02", data)
	if err != nil {
		return nil, fmt.Errorf("parsing date: %w", err)
	}

	id := uuid.New()
	_, err = r.pool.Exec(ctx, `
		INSERT INTO feriados (id, data, nome, tipo) VALUES ($1, $2, $3, 'custom')
		ON CONFLICT (data) DO UPDATE SET nome = $3
	`, id, d, nome)
	if err != nil {
		return nil, fmt.Errorf("creating feriado: %w", err)
	}

	return &Feriado{ID: id, Data: data, Nome: nome, Tipo: "custom"}, nil
}

func (r *FeriadoRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM feriados WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting feriado: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
