package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ConfigRepository struct {
	pool *pgxpool.Pool
}

func NewConfigRepository(pool *pgxpool.Pool) *ConfigRepository {
	return &ConfigRepository{pool: pool}
}

func (r *ConfigRepository) GetConfig(ctx context.Context, chave string) (string, error) {
	var valor string
	err := r.pool.QueryRow(ctx,
		`SELECT valor FROM configuracoes WHERE chave = $1`, chave,
	).Scan(&valor)
	if err != nil {
		return "", fmt.Errorf("getting config %s: %w", chave, err)
	}
	return valor, nil
}

func (r *ConfigRepository) SetConfig(ctx context.Context, chave, valor string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO configuracoes (chave, valor, atualizado_em)
		VALUES ($1, $2, NOW())
		ON CONFLICT (chave) DO UPDATE SET valor = $2, atualizado_em = NOW()
	`, chave, valor)
	if err != nil {
		return fmt.Errorf("setting config %s: %w", chave, err)
	}
	return nil
}

func (r *ConfigRepository) ConfigExists(ctx context.Context, chave string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM configuracoes WHERE chave = $1)`, chave,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking config %s: %w", chave, err)
	}
	return exists, nil
}
