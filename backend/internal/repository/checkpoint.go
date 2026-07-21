package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Checkpoint struct {
	ID         uuid.UUID  `json:"id"`
	EquipeID   *uuid.UUID `json:"equipe_id"`
	Nome       string     `json:"nome"`
	Resumo     string     `json:"resumo"`
	DataInicio string     `json:"data_inicio"`
	DataFim    *string    `json:"data_fim"`
	Cor        string     `json:"cor"`
}

type CheckpointRepository struct {
	pool *pgxpool.Pool
}

func NewCheckpointRepository(pool *pgxpool.Pool) *CheckpointRepository {
	return &CheckpointRepository{pool: pool}
}

func (r *CheckpointRepository) List(ctx context.Context, equipeID *uuid.UUID, ano int) ([]Checkpoint, error) {
	yearStart := time.Date(ano, 1, 1, 0, 0, 0, 0, time.UTC)
	yearEnd := time.Date(ano, 12, 31, 0, 0, 0, 0, time.UTC)

	rows, err := r.pool.Query(ctx, `
		SELECT id, equipe_id, nome, resumo, data_inicio, data_fim, cor
		FROM checkpoints
		WHERE (equipe_id = $1 OR equipe_id IS NULL)
		AND data_inicio <= $3
		AND (data_fim >= $2 OR (data_fim IS NULL AND data_inicio >= $2))
		ORDER BY data_inicio
	`, equipeID, yearStart, yearEnd)
	if err != nil {
		return nil, fmt.Errorf("listing checkpoints: %w", err)
	}
	defer rows.Close()

	result := make([]Checkpoint, 0)
	for rows.Next() {
		var c Checkpoint
		var di time.Time
		var df *time.Time
		if err := rows.Scan(&c.ID, &c.EquipeID, &c.Nome, &c.Resumo, &di, &df, &c.Cor); err != nil {
			return nil, fmt.Errorf("scanning checkpoint: %w", err)
		}
		c.DataInicio = di.Format("2006-01-02")
		if df != nil {
			s := df.Format("2006-01-02")
			c.DataFim = &s
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (r *CheckpointRepository) Create(ctx context.Context, equipeID *uuid.UUID, nome, resumo, dataInicio string, dataFim *string, cor string) (*Checkpoint, error) {
	di, err := time.Parse("2006-01-02", dataInicio)
	if err != nil {
		return nil, fmt.Errorf("parsing data_inicio: %w", err)
	}

	var df *time.Time
	if dataFim != nil && *dataFim != "" {
		t, err := time.Parse("2006-01-02", *dataFim)
		if err != nil {
			return nil, fmt.Errorf("parsing data_fim: %w", err)
		}
		df = &t
	}

	id := uuid.New()
	_, err = r.pool.Exec(ctx, `
		INSERT INTO checkpoints (id, equipe_id, nome, resumo, data_inicio, data_fim, cor)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, equipeID, nome, resumo, di, df, cor)
	if err != nil {
		return nil, fmt.Errorf("creating checkpoint: %w", err)
	}

	return &Checkpoint{
		ID: id, EquipeID: equipeID, Nome: nome, Resumo: resumo,
		DataInicio: dataInicio, DataFim: dataFim, Cor: cor,
	}, nil
}

func (r *CheckpointRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM checkpoints WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting checkpoint: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
