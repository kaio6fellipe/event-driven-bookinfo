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

// FindByProductID returns paginated reviews for the given product ID.
func (r *ReviewRepository) FindByProductID(ctx context.Context, productID string, offset, limit int) ([]domain.Review, int, error) {
	var total int
	err := r.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM reviews WHERE product_id = $1",
		productID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting reviews for product %s: %w", productID, err)
	}

	rows, err := r.pool.Query(ctx,
		"SELECT id, product_id, reviewer, text FROM reviews WHERE product_id = $1 ORDER BY id LIMIT $2 OFFSET $3",
		productID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("querying reviews for product %s: %w", productID, err)
	}
	defer rows.Close()

	var reviews []domain.Review
	for rows.Next() {
		var review domain.Review
		if err := rows.Scan(&review.ID, &review.ProductID, &review.Reviewer, &review.Text); err != nil {
			return nil, 0, fmt.Errorf("scanning review row: %w", err)
		}
		reviews = append(reviews, review)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating review rows: %w", err)
	}

	return reviews, total, nil
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

// DeleteByID removes a review by its ID.
func (r *ReviewRepository) DeleteByID(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx,
		"DELETE FROM reviews WHERE id = $1",
		id,
	)
	if err != nil {
		return fmt.Errorf("deleting review %s: %w", id, err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return nil
}
