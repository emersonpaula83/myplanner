package domain

import (
	"time"

	"github.com/google/uuid"
)

type Skill struct {
	ID        uuid.UUID `json:"id"`
	Nome      string    `json:"nome"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
