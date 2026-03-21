package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Message struct {
	ID             uuid.UUID        `json:"id"`
	ConversationID uuid.UUID        `json:"conversation_id"`
	Role           string           `json:"role"`
	Content        *string          `json:"content,omitempty"`
	ToolCalls      *json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID     *string          `json:"tool_call_id,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
}
