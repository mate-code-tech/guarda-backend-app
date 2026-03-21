package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/guarda/backend/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MessageRepo struct {
	pool *pgxpool.Pool
}

func NewMessageRepo(pool *pgxpool.Pool) *MessageRepo {
	return &MessageRepo{pool: pool}
}

func (r *MessageRepo) Create(ctx context.Context, m *model.Message) error {
	return r.pool.QueryRow(ctx,
		"INSERT INTO messages (conversation_id, role, content, tool_calls, tool_call_id) VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at",
		m.ConversationID, m.Role, m.Content, m.ToolCalls, m.ToolCallID).
		Scan(&m.ID, &m.CreatedAt)
}

func (r *MessageRepo) GetByConversation(ctx context.Context, convID uuid.UUID) ([]model.Message, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT id, conversation_id, role, content, tool_calls, tool_call_id, created_at FROM messages WHERE conversation_id = $1 ORDER BY created_at ASC", convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []model.Message
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.ToolCalls, &m.ToolCallID, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, nil
}
