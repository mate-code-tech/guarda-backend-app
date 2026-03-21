package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/guarda/backend/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GuestRepo struct {
	pool *pgxpool.Pool
}

func NewGuestRepo(pool *pgxpool.Pool) *GuestRepo {
	return &GuestRepo{pool: pool}
}

func (r *GuestRepo) Create(ctx context.Context, g *model.Guest) error {
	_, err := r.pool.Exec(ctx,
		"INSERT INTO guests (id, preferred_mode) VALUES ($1, $2)",
		g.ID, g.PreferredMode)
	return err
}

func (r *GuestRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Guest, error) {
	g := &model.Guest{}
	err := r.pool.QueryRow(ctx,
		"SELECT id, created_at, preferred_mode, name, age, conditions, allergies, consultation_reason, is_for_self FROM guests WHERE id = $1", id).
		Scan(&g.ID, &g.CreatedAt, &g.PreferredMode, &g.Name, &g.Age, &g.Conditions, &g.Allergies, &g.ConsultationReason, &g.IsForSelf)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (r *GuestRepo) UpdateProfile(ctx context.Context, id uuid.UUID, name *string, age *int, conditions []string, allergies []string, consultationReason *string, isForSelf *bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE guests SET
			name = COALESCE($2, name),
			age = COALESCE($3, age),
			conditions = CASE WHEN $4::text[] IS NOT NULL AND array_length($4::text[], 1) > 0 THEN $4 ELSE conditions END,
			allergies = CASE WHEN $5::text[] IS NOT NULL AND array_length($5::text[], 1) > 0 THEN $5 ELSE allergies END,
			consultation_reason = COALESCE($6, consultation_reason),
			is_for_self = COALESCE($7, is_for_self)
		WHERE id = $1`,
		id, name, age, conditions, allergies, consultationReason, isForSelf)
	return err
}
