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
		"SELECT id, created_at, preferred_mode FROM guests WHERE id = $1", id).
		Scan(&g.ID, &g.CreatedAt, &g.PreferredMode)
	if err != nil {
		return nil, err
	}
	return g, nil
}
