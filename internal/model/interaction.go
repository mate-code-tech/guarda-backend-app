package model

import (
	"time"

	"github.com/google/uuid"
)

type Interaction struct {
	ID             uuid.UUID `json:"id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	DrugA          string    `json:"drug_a"`
	DrugB          string    `json:"drug_b"`
	Severity       string    `json:"severity"`
	Description    *string   `json:"description,omitempty"`
	Recommendation *string   `json:"recommendation,omitempty"`
	Source         string    `json:"source"`
	CreatedAt      time.Time `json:"created_at"`
}
