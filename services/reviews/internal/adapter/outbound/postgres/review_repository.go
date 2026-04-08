// Package postgres provides a PostgreSQL implementation of the reviews repository.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

// ReviewRepository is a PostgreSQL implementation of port.ReviewRepository.
type ReviewRepository struct {
	pool *pgxpool.Pool
}

// NewReviewRepository creates a new PostgreSQL review repository.
func NewReviewRepository(pool *pgxpool.Pool) *ReviewRepository {
	return &ReviewRepository{pool: pool}
}

// FindByProductID returns all reviews for the given product ID.
func (r *ReviewRepository) FindByProductID(ctx context.Context, productID string) ([]domain.Review, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT id, product_id, reviewer, text FROM reviews WHERE product_id = $1",
		productID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying reviews for product %s: %w", productID, err)
	}
	defer rows.Close()

	var reviews []domain.Review
	for rows.Next() {
		var review domain.Review
		if err := rows.Scan(&review.ID, &review.ProductID, &review.Reviewer, &review.Text); err != nil {
			return nil, fmt.Errorf("scanning review row: %w", err)
		}
		reviews = append(reviews, review)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating review rows: %w", err)
	}

	return reviews, nil
}

// Save persists a review in PostgreSQL.
func (r *ReviewRepository) Save(ctx context.Context, review *domain.Review) error {
	_, err := r.pool.Exec(ctx,
		"INSERT INTO reviews (id, product_id, reviewer, text) VALUES ($1, $2, $3, $4)",
		review.ID, review.ProductID, review.Reviewer, review.Text,
	)
	if err != nil {
		return fmt.Errorf("inserting review %s: %w", review.ID, err)
	}

	return nil
}
