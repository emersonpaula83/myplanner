package domain

import (
	"time"

	"github.com/google/uuid"
)

type SyncSchedule struct {
	ID           uuid.UUID `json:"id" db:"id"`
	FonteDadosID uuid.UUID `json:"fonte_dados_id" db:"fonte_dados_id"`
	ProjectKeys  []string  `json:"project_keys" db:"project_keys"`
	Horarios     []string  `json:"horarios" db:"horarios"`
	Ativo        bool      `json:"ativo" db:"ativo"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}
