package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/guarda/backend/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ConversationRepo struct {
	pool *pgxpool.Pool
}

func NewConversationRepo(pool *pgxpool.Pool) *ConversationRepo {
	return &ConversationRepo{pool: pool}
}

func (r *ConversationRepo) Create(ctx context.Context, c *model.Conversation) error {
	return r.pool.QueryRow(ctx,
		"INSERT INTO conversations (guest_id, flow_type, status) VALUES ($1, $2, $3) RETURNING id, created_at, updated_at",
		c.GuestID, c.FlowType, c.Status).
		Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (r *ConversationRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Conversation, error) {
	c := &model.Conversation{}
	err := r.pool.QueryRow(ctx,
		"SELECT id, guest_id, flow_type, status, created_at, updated_at FROM conversations WHERE id = $1", id).
		Scan(&c.ID, &c.GuestID, &c.FlowType, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}
