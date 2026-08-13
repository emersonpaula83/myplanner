package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ImportConfig struct {
	ID         uuid.UUID
	Tipo       string
	URL        *string
	Gid        *string
	UltimoSync *time.Time
}

type ImportConfigRepository struct {
	pool *pgxpool.Pool
}

func NewImportConfigRepository(pool *pgxpool.Pool) *ImportConfigRepository {
	return &ImportConfigRepository{pool: pool}
}

func (r *ImportConfigRepository) Get(ctx context.Context) (*ImportConfig, error) {
	var c ImportConfig
	err := r.pool.QueryRow(ctx, `
		SELECT id, tipo, url, gid, ultimo_sync FROM import_configs
		ORDER BY created_at DESC LIMIT 1
	`).Scan(&c.ID, &c.Tipo, &c.URL, &c.Gid, &c.UltimoSync)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting import config: %w", err)
	}
	return &c, nil
}

func (r *ImportConfigRepository) Save(ctx context.Context, tipo string, url, gid *string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM import_configs`); err != nil {
		return fmt.Errorf("clearing import config: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO import_configs (tipo, url, gid) VALUES ($1, $2, $3)
	`, tipo, url, gid); err != nil {
		return fmt.Errorf("saving import config: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *ImportConfigRepository) UpdateUltimoSync(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `UPDATE import_configs SET ultimo_sync = NOW()`)
	if err != nil {
		return fmt.Errorf("updating ultimo_sync: %w", err)
	}
	return nil
}
