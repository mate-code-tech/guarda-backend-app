package model

import (
	"time"

	"github.com/google/uuid"
)

type Guest struct {
	ID            uuid.UUID `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	PreferredMode string    `json:"preferred_mode"`
}
