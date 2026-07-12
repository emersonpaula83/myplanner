package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/totvs/tcloud-planner/backend/internal/domain"
)

type UsuarioRepository struct {
	pool *pgxpool.Pool
}

func NewUsuarioRepository(pool *pgxpool.Pool) *UsuarioRepository {
	return &UsuarioRepository{pool: pool}
}

func (r *UsuarioRepository) BuscarPorEmail(ctx context.Context, email string) (*domain.Usuario, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, nome_completo, apelido, email, senha_hash, cargo, ativo, created_at, updated_at
		FROM usuarios
		WHERE email = $1 AND ativo = true
	`, email)

	u, err := scanUsuario(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying usuario by email: %w", err)
	}
	return &u, nil
}

func (r *UsuarioRepository) BuscarPorID(ctx context.Context, id uuid.UUID) (*domain.Usuario, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, nome_completo, apelido, email, senha_hash, cargo, ativo, created_at, updated_at
		FROM usuarios
		WHERE id = $1
	`, id)

	u, err := scanUsuario(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying usuario by id: %w", err)
	}
	return &u, nil
}

func (r *UsuarioRepository) ListarTodos(ctx context.Context) ([]domain.Usuario, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, nome_completo, apelido, email, senha_hash, cargo, ativo, created_at, updated_at
		FROM usuarios
		ORDER BY nome_completo
	`)
	if err != nil {
		return nil, fmt.Errorf("querying usuarios: %w", err)
	}
	defer rows.Close()

	result := make([]domain.Usuario, 0)
	for rows.Next() {
		u, err := scanUsuarioRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning usuario: %w", err)
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

func (r *UsuarioRepository) Criar(ctx context.Context, req *domain.CriarUsuarioRequest, senhaHash string) (*domain.Usuario, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO usuarios (id, nome_completo, apelido, email, senha_hash, cargo)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, nome_completo, apelido, email, senha_hash, cargo, ativo, created_at, updated_at
	`, uuid.New(), req.NomeCompleto, req.Apelido, req.Email, senhaHash, req.Cargo)

	u, err := scanUsuario(row)
	if err != nil {
		return nil, fmt.Errorf("creating usuario: %w", err)
	}
	return &u, nil
}

func (r *UsuarioRepository) Atualizar(ctx context.Context, id uuid.UUID, req *domain.AtualizarUsuarioRequest) (*domain.Usuario, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE usuarios
		SET nome_completo = COALESCE($2, nome_completo),
		    apelido = COALESCE($3, apelido),
		    email = COALESCE($4, email),
		    cargo = COALESCE($5, cargo),
		    ativo = COALESCE($6, ativo),
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, nome_completo, apelido, email, senha_hash, cargo, ativo, created_at, updated_at
	`, id, req.NomeCompleto, req.Apelido, req.Email, req.Cargo, req.Ativo)

	u, err := scanUsuario(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("updating usuario %s: %w", id, err)
	}
	return &u, nil
}

func (r *UsuarioRepository) AtualizarSenha(ctx context.Context, id uuid.UUID, senhaHash string) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE usuarios
		SET senha_hash = $2, updated_at = NOW()
		WHERE id = $1
	`, id, senhaHash)
	if err != nil {
		return fmt.Errorf("updating senha for usuario %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("usuario %s not found", id)
	}
	return nil
}

func (r *UsuarioRepository) ListarProjetos(ctx context.Context, usuarioID uuid.UUID) ([]domain.ProjetoResumo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.chave, p.nome
		FROM projetos p
		INNER JOIN usuario_projetos up ON up.projeto_id = p.id
		WHERE up.usuario_id = $1
		ORDER BY p.nome
	`, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("querying projetos for usuario %s: %w", usuarioID, err)
	}
	defer rows.Close()

	result := make([]domain.ProjetoResumo, 0)
	for rows.Next() {
		var p domain.ProjetoResumo
		if err := rows.Scan(&p.ID, &p.Chave, &p.Nome); err != nil {
			return nil, fmt.Errorf("scanning projeto resumo: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *UsuarioRepository) AtualizarProjetos(ctx context.Context, usuarioID uuid.UUID, projetoIDs []uuid.UUID) ([]domain.ProjetoResumo, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `DELETE FROM usuario_projetos WHERE usuario_id = $1`, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("deleting existing projetos: %w", err)
	}

	for _, pid := range projetoIDs {
		_, err = tx.Exec(ctx, `
			INSERT INTO usuario_projetos (usuario_id, projeto_id) VALUES ($1, $2)
		`, usuarioID, pid)
		if err != nil {
			return nil, fmt.Errorf("inserting projeto %s: %w", pid, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return r.ListarProjetos(ctx, usuarioID)
}

func (r *UsuarioRepository) BuscarProjetoIDsPorUsuario(ctx context.Context, usuarioID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT projeto_id FROM usuario_projetos WHERE usuario_id = $1
	`, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("querying projeto_ids for usuario %s: %w", usuarioID, err)
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning projeto_id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func scanUsuario(row pgx.Row) (domain.Usuario, error) {
	var u domain.Usuario
	err := row.Scan(
		&u.ID, &u.NomeCompleto, &u.Apelido, &u.Email, &u.SenhaHash,
		&u.Cargo, &u.Ativo, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

func scanUsuarioRows(rows pgx.Rows) (domain.Usuario, error) {
	var u domain.Usuario
	err := rows.Scan(
		&u.ID, &u.NomeCompleto, &u.Apelido, &u.Email, &u.SenhaHash,
		&u.Cargo, &u.Ativo, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}
