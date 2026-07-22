package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/emersonpaula83/myplanner/backend/internal/domain"
)

type SyncScheduleRepository struct {
	pool *pgxpool.Pool
}

func NewSyncScheduleRepository(pool *pgxpool.Pool) *SyncScheduleRepository {
	return &SyncScheduleRepository{pool: pool}
}

func (r *SyncScheduleRepository) Upsert(ctx context.Context, fonteID uuid.UUID, projectKeys []string, horarios []string) (*domain.SyncSchedule, error) {
	keysJSON, err := json.Marshal(projectKeys)
	if err != nil {
		return nil, fmt.Errorf("marshaling project_keys: %w", err)
	}
	horariosJSON, err := json.Marshal(horarios)
	if err != nil {
		return nil, fmt.Errorf("marshaling horarios: %w", err)
	}

	var s domain.SyncSchedule
	var keysRaw, horariosRaw []byte
	err = r.pool.QueryRow(ctx, `
		INSERT INTO sync_schedules (fonte_dados_id, project_keys, horarios)
		VALUES ($1, $2, $3)
		ON CONFLICT (fonte_dados_id)
		DO UPDATE SET project_keys = EXCLUDED.project_keys, horarios = EXCLUDED.horarios, updated_at = NOW()
		RETURNING id, fonte_dados_id, project_keys, horarios, ativo, created_at, updated_at
	`, fonteID, keysJSON, horariosJSON).Scan(&s.ID, &s.FonteDadosID, &keysRaw, &horariosRaw, &s.Ativo, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upserting sync schedule: %w", err)
	}
	json.Unmarshal(keysRaw, &s.ProjectKeys)
	json.Unmarshal(horariosRaw, &s.Horarios)
	return &s, nil
}

func (r *SyncScheduleRepository) Delete(ctx context.Context, fonteID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM sync_schedules WHERE fonte_dados_id = $1
	`, fonteID)
	if err != nil {
		return fmt.Errorf("deleting sync schedule: %w", err)
	}
	return nil
}

func (r *SyncScheduleRepository) GetByFonte(ctx context.Context, fonteID uuid.UUID) (*domain.SyncSchedule, error) {
	var s domain.SyncSchedule
	var keysRaw, horariosRaw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, fonte_dados_id, project_keys, horarios, ativo, created_at, updated_at
		FROM sync_schedules WHERE fonte_dados_id = $1
	`, fonteID).Scan(&s.ID, &s.FonteDadosID, &keysRaw, &horariosRaw, &s.Ativo, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting sync schedule: %w", err)
	}
	json.Unmarshal(keysRaw, &s.ProjectKeys)
	json.Unmarshal(horariosRaw, &s.Horarios)
	return &s, nil
}

func (r *SyncScheduleRepository) GetDueSchedules(ctx context.Context, horaMinuto string) ([]domain.SyncSchedule, error) {
	quoted, _ := json.Marshal(horaMinuto)
	rows, err := r.pool.Query(ctx, `
		SELECT id, fonte_dados_id, project_keys, horarios, ativo, created_at, updated_at
		FROM sync_schedules
		WHERE ativo = true AND horarios @> $1::jsonb
	`, string(quoted))
	if err != nil {
		return nil, fmt.Errorf("getting due schedules: %w", err)
	}
	defer rows.Close()

	var schedules []domain.SyncSchedule
	for rows.Next() {
		var s domain.SyncSchedule
		var keysRaw, horariosRaw []byte
		if err := rows.Scan(&s.ID, &s.FonteDadosID, &keysRaw, &horariosRaw, &s.Ativo, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning due schedule: %w", err)
		}
		json.Unmarshal(keysRaw, &s.ProjectKeys)
		json.Unmarshal(horariosRaw, &s.Horarios)
		schedules = append(schedules, s)
	}
	return schedules, nil
}

func (r *SyncScheduleRepository) SetAtivo(ctx context.Context, id uuid.UUID, ativo bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE sync_schedules SET ativo = $2, updated_at = NOW() WHERE id = $1
	`, id, ativo)
	if err != nil {
		return fmt.Errorf("toggling sync schedule: %w", err)
	}
	return nil
}
