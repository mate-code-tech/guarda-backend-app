package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/guarda/backend/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InteractionRepo struct {
	pool *pgxpool.Pool
}

func NewInteractionRepo(pool *pgxpool.Pool) *InteractionRepo {
	return &InteractionRepo{pool: pool}
}

func (r *InteractionRepo) Create(ctx context.Context, i *model.Interaction) error {
	return r.pool.QueryRow(ctx,
		"INSERT INTO interactions (conversation_id, drug_a, drug_b, severity, description, recommendation, source) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at",
		i.ConversationID, i.DrugA, i.DrugB, i.Severity, i.Description, i.Recommendation, i.Source).
		Scan(&i.ID, &i.CreatedAt)
}

func (r *InteractionRepo) GetByConversation(ctx context.Context, convID uuid.UUID) ([]model.Interaction, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT id, conversation_id, drug_a, drug_b, severity, description, recommendation, source, created_at FROM interactions WHERE conversation_id = $1 ORDER BY created_at ASC", convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var interactions []model.Interaction
	for rows.Next() {
		var i model.Interaction
		if err := rows.Scan(&i.ID, &i.ConversationID, &i.DrugA, &i.DrugB, &i.Severity, &i.Description, &i.Recommendation, &i.Source, &i.CreatedAt); err != nil {
			return nil, err
		}
		interactions = append(interactions, i)
	}
	return interactions, nil
}
