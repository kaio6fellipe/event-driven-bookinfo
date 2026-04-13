// Package postgres provides a PostgreSQL implementation of port.DLQRepository.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/port"
)

// DLQRepository persists DLQ events in PostgreSQL.
type DLQRepository struct {
	pool *pgxpool.Pool
}

// NewDLQRepository constructs a new DLQRepository.
func NewDLQRepository(pool *pgxpool.Pool) *DLQRepository {
	return &DLQRepository{pool: pool}
}

const insertSQL = `
INSERT INTO dlq_events (
    id, event_id, event_type, event_source, event_subject,
    sensor_name, failed_trigger, eventsource_url, namespace,
    original_payload, payload_hash, original_headers,
    datacontenttype, event_timestamp, status,
    retry_count, max_retries, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9,
    $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
)
`

// Save implements port.DLQRepository.
func (r *DLQRepository) Save(ctx context.Context, e *domain.DLQEvent) error {
	headers, err := json.Marshal(e.OriginalHeaders)
	if err != nil {
		return fmt.Errorf("marshaling headers: %w", err)
	}
	_, err = r.pool.Exec(ctx, insertSQL,
		e.ID, e.EventID, e.EventType, e.EventSource, e.EventSubject,
		e.SensorName, e.FailedTrigger, e.EventSourceURL, e.Namespace,
		e.OriginalPayload, e.PayloadHash, headers,
		e.DataContentType, e.EventTimestamp, string(e.Status),
		e.RetryCount, e.MaxRetries, e.CreatedAt, e.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting dlq event: %w", err)
	}
	return nil
}

const selectCols = `
    id, event_id, event_type, event_source, event_subject,
    sensor_name, failed_trigger, eventsource_url, namespace,
    original_payload, payload_hash, original_headers,
    datacontenttype, event_timestamp, status,
    retry_count, max_retries, last_replayed_at, resolved_at,
    resolved_by, notes, created_at, updated_at
`

func scanEvent(row pgx.Row) (*domain.DLQEvent, error) {
	var e domain.DLQEvent
	var statusStr string
	var headersJSON []byte
	var resolvedBy, notes *string
	err := row.Scan(
		&e.ID, &e.EventID, &e.EventType, &e.EventSource, &e.EventSubject,
		&e.SensorName, &e.FailedTrigger, &e.EventSourceURL, &e.Namespace,
		&e.OriginalPayload, &e.PayloadHash, &headersJSON,
		&e.DataContentType, &e.EventTimestamp, &statusStr,
		&e.RetryCount, &e.MaxRetries, &e.LastReplayedAt, &e.ResolvedAt,
		&resolvedBy, &notes, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	e.Status = domain.DLQStatus(statusStr)
	if len(headersJSON) > 0 {
		if err := json.Unmarshal(headersJSON, &e.OriginalHeaders); err != nil {
			return nil, fmt.Errorf("unmarshaling headers: %w", err)
		}
	}
	if resolvedBy != nil {
		e.ResolvedBy = *resolvedBy
	}
	if notes != nil {
		e.Notes = *notes
	}
	return &e, nil
}

// FindByID implements port.DLQRepository.
func (r *DLQRepository) FindByID(ctx context.Context, id string) (*domain.DLQEvent, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+selectCols+` FROM dlq_events WHERE id = $1`, id)
	e, err := scanEvent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finding dlq event: %w", err)
	}
	return e, nil
}

// FindByNaturalKey implements port.DLQRepository.
func (r *DLQRepository) FindByNaturalKey(ctx context.Context, sensorName, failedTrigger, payloadHash string) (*domain.DLQEvent, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+selectCols+` FROM dlq_events
		 WHERE sensor_name = $1 AND failed_trigger = $2 AND payload_hash = $3`,
		sensorName, failedTrigger, payloadHash)
	e, err := scanEvent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finding by natural key: %w", err)
	}
	return e, nil
}

// List implements port.DLQRepository.
func (r *DLQRepository) List(ctx context.Context, f port.ListFilter) ([]domain.DLQEvent, int, error) {
	var conds []string
	var args []any
	argn := 1
	add := func(cond string, v any) {
		conds = append(conds, strings.ReplaceAll(cond, "$?", fmt.Sprintf("$%d", argn)))
		args = append(args, v)
		argn++
	}
	if f.Status != "" {
		add("status = $?", f.Status)
	}
	if f.EventSource != "" {
		add("event_source = $?", f.EventSource)
	}
	if f.SensorName != "" {
		add("sensor_name = $?", f.SensorName)
	}
	if f.FailedTrigger != "" {
		add("failed_trigger = $?", f.FailedTrigger)
	}
	if f.CreatedAfter != nil {
		add("created_at >= $?", *f.CreatedAfter)
	}
	if f.CreatedBefore != nil {
		add("created_at <= $?", *f.CreatedBefore)
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM dlq_events`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting dlq events: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	limitArg := fmt.Sprintf("$%d", argn)
	args = append(args, limit)
	argn++
	offsetArg := fmt.Sprintf("$%d", argn)
	args = append(args, f.Offset)

	rows, err := r.pool.Query(ctx,
		`SELECT `+selectCols+` FROM dlq_events`+where+
			` ORDER BY created_at DESC LIMIT `+limitArg+` OFFSET `+offsetArg,
		args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing dlq events: %w", err)
	}
	defer rows.Close()

	var out []domain.DLQEvent
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning dlq event: %w", err)
		}
		out = append(out, *e)
	}
	return out, total, rows.Err()
}

// Update implements port.DLQRepository.
func (r *DLQRepository) Update(ctx context.Context, e *domain.DLQEvent) error {
	headers, err := json.Marshal(e.OriginalHeaders)
	if err != nil {
		return fmt.Errorf("marshaling headers: %w", err)
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE dlq_events SET
             status = $2, retry_count = $3, max_retries = $4,
             last_replayed_at = $5, resolved_at = $6, resolved_by = $7,
             notes = $8, original_headers = $9, updated_at = $10
         WHERE id = $1`,
		e.ID, string(e.Status), e.RetryCount, e.MaxRetries,
		e.LastReplayedAt, e.ResolvedAt, e.ResolvedBy,
		e.Notes, headers, e.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("updating dlq event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
