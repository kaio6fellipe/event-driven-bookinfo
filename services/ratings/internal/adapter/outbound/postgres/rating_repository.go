// Package postgres provides a PostgreSQL implementation of the ratings repository.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/ratings/internal/core/domain"
)

// RatingRepository is a PostgreSQL implementation of port.RatingRepository.
type RatingRepository struct {
	pool *pgxpool.Pool
}

// NewRatingRepository creates a new PostgreSQL rating repository.
func NewRatingRepository(pool *pgxpool.Pool) *RatingRepository {
	return &RatingRepository{pool: pool}
}

// FindByProductID returns all ratings for the given product ID.
func (r *RatingRepository) FindByProductID(ctx context.Context, productID string) ([]domain.Rating, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT id, product_id, reviewer, stars FROM ratings WHERE product_id = $1",
		productID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying ratings for product %s: %w", productID, err)
	}
	defer rows.Close()

	var ratings []domain.Rating
	for rows.Next() {
		var rating domain.Rating
		if err := rows.Scan(&rating.ID, &rating.ProductID, &rating.Reviewer, &rating.Stars); err != nil {
			return nil, fmt.Errorf("scanning rating row: %w", err)
		}
		ratings = append(ratings, rating)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rating rows: %w", err)
	}

	return ratings, nil
}

// Save persists a rating in PostgreSQL.
func (r *RatingRepository) Save(ctx context.Context, rating *domain.Rating) error {
	_, err := r.pool.Exec(ctx,
		"INSERT INTO ratings (id, product_id, reviewer, stars) VALUES ($1, $2, $3, $4)",
		rating.ID, rating.ProductID, rating.Reviewer, rating.Stars,
	)
	if err != nil {
		return fmt.Errorf("inserting rating %s: %w", rating.ID, err)
	}

	return nil
}
