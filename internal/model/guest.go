package model

import (
	"time"

	"github.com/google/uuid"
)

type Guest struct {
	ID                 uuid.UUID `json:"id"`
	CreatedAt          time.Time `json:"created_at"`
	PreferredMode      string    `json:"preferred_mode"`
	Name               *string   `json:"name,omitempty"`
	Age                *int      `json:"age,omitempty"`
	Conditions         []string  `json:"conditions,omitempty"`
	Allergies          []string  `json:"allergies,omitempty"`
	ConsultationReason *string   `json:"consultation_reason,omitempty"`
	IsForSelf          bool      `json:"is_for_self"`
}
