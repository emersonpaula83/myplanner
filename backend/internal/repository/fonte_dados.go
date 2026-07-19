package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

type FonteDadosRepository struct {
	pool *pgxpool.Pool
}

func NewFonteDadosRepository(pool *pgxpool.Pool) *FonteDadosRepository {
	return &FonteDadosRepository{pool: pool}
}

type CreateFonteDadosRequest struct {
	Nome               string          `json:"nome"`
	Tipo               string          `json:"tipo"`
	BaseURL            string          `json:"base_url"`
	AuthType           string          `json:"auth_type"`
	APIToken           *string         `json:"api_token"`
	UserEmail          *string         `json:"user_email"`
	OAuth2AccessToken  *string         `json:"-"`
	OAuth2RefreshToken *string         `json:"-"`
	OAuth2TokenExpiry  *time.Time      `json:"-"`
	CustomFieldMap     json.RawMessage `json:"custom_field_map"`
}

type UpdateFonteDadosRequest struct {
	Nome           *string         `json:"nome"`
	BaseURL        *string         `json:"base_url"`
	AuthType       *string         `json:"auth_type"`
	APIToken       *string         `json:"api_token"`
	UserEmail      *string         `json:"user_email"`
	CustomFieldMap json.RawMessage `json:"custom_field_map"`
}

func (r *FonteDadosRepository) List(ctx context.Context) ([]domain.FonteDados, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, nome, tipo, base_url, auth_type, api_token, user_email,
		       oauth2_client_id, oauth2_client_secret, oauth2_access_token,
		       oauth2_refresh_token, oauth2_token_expiry, custom_field_map,
		       ativo, ultimo_sync, created_at, updated_at
		FROM fonte_dados
		WHERE ativo = true
		ORDER BY nome
	`)
	if err != nil {
		return nil, fmt.Errorf("querying fonte_dados: %w", err)
	}
	defer rows.Close()

	var result []domain.FonteDados
	for rows.Next() {
		fd, err := scanFonteDados(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning fonte_dados: %w", err)
		}
		result = append(result, fd)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating fonte_dados rows: %w", err)
	}

	return result, nil
}

func (r *FonteDadosRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.FonteDados, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, nome, tipo, base_url, auth_type, api_token, user_email,
		       oauth2_client_id, oauth2_client_secret, oauth2_access_token,
		       oauth2_refresh_token, oauth2_token_expiry, custom_field_map,
		       ativo, ultimo_sync, created_at, updated_at
		FROM fonte_dados
		WHERE id = $1
	`, id)

	fd, err := scanFonteDadosRow(row)
	if err != nil {
		return nil, fmt.Errorf("getting fonte_dados %s: %w", id, err)
	}

	return &fd, nil
}

func (r *FonteDadosRepository) Create(ctx context.Context, req *CreateFonteDadosRequest) (*domain.FonteDados, error) {
	cfm := req.CustomFieldMap
	if cfm == nil {
		cfm = json.RawMessage(`{}`)
	}

	row := r.pool.QueryRow(ctx, `
		INSERT INTO fonte_dados (id, nome, tipo, base_url, auth_type, api_token, user_email,
		       oauth2_access_token, oauth2_refresh_token, oauth2_token_expiry, custom_field_map)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, nome, tipo, base_url, auth_type, api_token, user_email,
		          oauth2_client_id, oauth2_client_secret, oauth2_access_token,
		          oauth2_refresh_token, oauth2_token_expiry, custom_field_map,
		          ativo, ultimo_sync, created_at, updated_at
	`, uuid.New(), req.Nome, req.Tipo, req.BaseURL, req.AuthType, req.APIToken, req.UserEmail,
		req.OAuth2AccessToken, req.OAuth2RefreshToken, req.OAuth2TokenExpiry, cfm)

	fd, err := scanFonteDadosRow(row)
	if err != nil {
		return nil, fmt.Errorf("creating fonte_dados: %w", err)
	}

	return &fd, nil
}

func (r *FonteDadosRepository) Update(ctx context.Context, id uuid.UUID, req *UpdateFonteDadosRequest) (*domain.FonteDados, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE fonte_dados
		SET nome = COALESCE($2, nome),
		    base_url = COALESCE($3, base_url),
		    auth_type = COALESCE($4, auth_type),
		    api_token = COALESCE($5, api_token),
		    user_email = COALESCE($6, user_email),
		    custom_field_map = COALESCE($7, custom_field_map),
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, nome, tipo, base_url, auth_type, api_token, user_email,
		          oauth2_client_id, oauth2_client_secret, oauth2_access_token,
		          oauth2_refresh_token, oauth2_token_expiry, custom_field_map,
		          ativo, ultimo_sync, created_at, updated_at
	`, id, req.Nome, req.BaseURL, req.AuthType, req.APIToken, req.UserEmail, req.CustomFieldMap)

	fd, err := scanFonteDadosRow(row)
	if err != nil {
		return nil, fmt.Errorf("updating fonte_dados %s: %w", id, err)
	}

	return &fd, nil
}

func (r *FonteDadosRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE fonte_dados
		SET ativo = false, updated_at = NOW()
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("soft-deleting fonte_dados %s: %w", id, err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("fonte_dados %s not found", id)
	}

	return nil
}

func (r *FonteDadosRepository) UpdateUltimoSync(ctx context.Context, id uuid.UUID, syncTime time.Time) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE fonte_dados
		SET ultimo_sync = $2, updated_at = NOW()
		WHERE id = $1
	`, id, syncTime)
	if err != nil {
		return fmt.Errorf("updating ultimo_sync for fonte_dados %s: %w", id, err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("fonte_dados %s not found", id)
	}

	return nil
}

func (r *FonteDadosRepository) SaveOAuthTokens(ctx context.Context, id uuid.UUID, baseURL, accessToken, refreshToken string, expiry time.Time) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE fonte_dados
		SET base_url = $2, auth_type = 'oauth2',
		    oauth2_access_token = $3, oauth2_refresh_token = $4,
		    oauth2_token_expiry = $5, updated_at = NOW()
		WHERE id = $1
	`, id, baseURL, accessToken, refreshToken, expiry)
	if err != nil {
		return fmt.Errorf("saving oauth tokens for fonte_dados %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("fonte_dados %s not found", id)
	}
	return nil
}

func scanFonteDados(rows pgx.Rows) (domain.FonteDados, error) {
	var fd domain.FonteDados
	err := rows.Scan(
		&fd.ID, &fd.Nome, &fd.Tipo, &fd.BaseURL, &fd.AuthType,
		&fd.APIToken, &fd.UserEmail,
		&fd.OAuth2ClientID, &fd.OAuth2ClientSecret, &fd.OAuth2AccessToken,
		&fd.OAuth2RefreshToken, &fd.OAuth2TokenExpiry, &fd.CustomFieldMap,
		&fd.Ativo, &fd.UltimoSync, &fd.CreatedAt, &fd.UpdatedAt,
	)
	return fd, err
}

func scanFonteDadosRow(row pgx.Row) (domain.FonteDados, error) {
	var fd domain.FonteDados
	err := row.Scan(
		&fd.ID, &fd.Nome, &fd.Tipo, &fd.BaseURL, &fd.AuthType,
		&fd.APIToken, &fd.UserEmail,
		&fd.OAuth2ClientID, &fd.OAuth2ClientSecret, &fd.OAuth2AccessToken,
		&fd.OAuth2RefreshToken, &fd.OAuth2TokenExpiry, &fd.CustomFieldMap,
		&fd.Ativo, &fd.UltimoSync, &fd.CreatedAt, &fd.UpdatedAt,
	)
	return fd, err
}
