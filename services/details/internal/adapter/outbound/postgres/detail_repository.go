// Package postgres provides a PostgreSQL implementation of the details repository.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/details/internal/core/domain"
)

// DetailRepository is a PostgreSQL implementation of port.DetailRepository.
type DetailRepository struct {
	pool *pgxpool.Pool
}

// NewDetailRepository creates a new PostgreSQL detail repository.
func NewDetailRepository(pool *pgxpool.Pool) *DetailRepository {
	return &DetailRepository{pool: pool}
}

// FindByID returns a detail by its ID. Returns nil, nil if not found.
func (r *DetailRepository) FindByID(ctx context.Context, id string) (*domain.Detail, error) {
	var d domain.Detail
	err := r.pool.QueryRow(ctx,
		"SELECT id, title, author, year, type, pages, publisher, language, isbn10, isbn13 FROM details WHERE id = $1",
		id,
	).Scan(&d.ID, &d.Title, &d.Author, &d.Year, &d.Type, &d.Pages, &d.Publisher, &d.Language, &d.ISBN10, &d.ISBN13)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying detail %s: %w", id, err)
	}

	return &d, nil
}

// Save persists a detail in PostgreSQL.
func (r *DetailRepository) Save(ctx context.Context, detail *domain.Detail) error {
	_, err := r.pool.Exec(ctx,
		"INSERT INTO details (id, title, author, year, type, pages, publisher, language, isbn10, isbn13) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)",
		detail.ID, detail.Title, detail.Author, detail.Year, detail.Type, detail.Pages, detail.Publisher, detail.Language, detail.ISBN10, detail.ISBN13,
	)
	if err != nil {
		return fmt.Errorf("inserting detail %s: %w", detail.ID, err)
	}

	return nil
}

// FindAll returns all stored details.
func (r *DetailRepository) FindAll(ctx context.Context) ([]*domain.Detail, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT id, title, author, year, type, pages, publisher, language, isbn10, isbn13 FROM details ORDER BY title",
	)
	if err != nil {
		return nil, fmt.Errorf("querying all details: %w", err)
	}
	defer rows.Close()

	var details []*domain.Detail
	for rows.Next() {
		var d domain.Detail
		if err := rows.Scan(&d.ID, &d.Title, &d.Author, &d.Year, &d.Type, &d.Pages, &d.Publisher, &d.Language, &d.ISBN10, &d.ISBN13); err != nil {
			return nil, fmt.Errorf("scanning detail row: %w", err)
		}
		details = append(details, &d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating detail rows: %w", err)
	}

	return details, nil
}
