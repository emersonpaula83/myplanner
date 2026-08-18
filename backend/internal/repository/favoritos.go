package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FavoritosRepository struct {
	pool *pgxpool.Pool
}

func NewFavoritosRepository(pool *pgxpool.Pool) *FavoritosRepository {
	return &FavoritosRepository{pool: pool}
}

func (r *FavoritosRepository) List(ctx context.Context, usuarioID uuid.UUID, fonteDadosID uuid.UUID) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT project_key FROM usuario_projeto_favoritos
		WHERE usuario_id = $1 AND fonte_dados_id = $2
		ORDER BY project_key
	`, usuarioID, fonteDadosID)
	if err != nil {
		return nil, fmt.Errorf("listing favoritos: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scanning favorito key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating favoritos: %w", err)
	}
	if keys == nil {
		keys = []string{}
	}
	return keys, nil
}

func (r *FavoritosRepository) Replace(ctx context.Context, usuarioID uuid.UUID, fonteDadosID uuid.UUID, projectKeys []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `DELETE FROM usuario_projeto_favoritos WHERE usuario_id = $1 AND fonte_dados_id = $2`, usuarioID, fonteDadosID)
	if err != nil {
		return fmt.Errorf("deleting existing favoritos: %w", err)
	}

	for _, key := range projectKeys {
		_, err = tx.Exec(ctx, `
			INSERT INTO usuario_projeto_favoritos (usuario_id, fonte_dados_id, project_key)
			VALUES ($1, $2, $3)
		`, usuarioID, fonteDadosID, key)
		if err != nil {
			return fmt.Errorf("inserting favorito %s: %w", key, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}
