// Package domain defines the pure domain types for the dlqueue service.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
)

// DLQStatus is the lifecycle state of a DLQ event.
type DLQStatus string

// Valid DLQStatus values.
const (
	StatusPending  DLQStatus = "pending"
	StatusReplayed DLQStatus = "replayed"
	StatusResolved DLQStatus = "resolved"
	StatusPoisoned DLQStatus = "poisoned"
)

// Domain errors.
var (
	ErrNotFound          = errors.New("dlq event not found")
	ErrAlreadyResolved   = errors.New("event already resolved")
	ErrInvalidTransition = errors.New("invalid status transition")
)

// DLQEvent represents a single failed event captured from an Argo Events sensor.
type DLQEvent struct {
	ID              string
	EventID         string
	EventType       string
	EventSource     string
	EventSubject    string
	SensorName      string
	FailedTrigger   string
	EventSourceURL  string
	Namespace       string
	OriginalPayload []byte
	PayloadHash     string
	OriginalHeaders map[string][]string
	DataContentType string
	EventTimestamp  time.Time
	Status          DLQStatus
	RetryCount      int
	MaxRetries      int
	LastReplayedAt  *time.Time
	ResolvedAt      *time.Time
	ResolvedBy      string
	Notes           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewDLQEventParams holds inputs for NewDLQEvent.
type NewDLQEventParams struct {
	EventID         string
	EventType       string
	EventSource     string
	EventSubject    string
	SensorName      string
	FailedTrigger   string
	EventSourceURL  string
	Namespace       string
	OriginalPayload []byte
	OriginalHeaders map[string][]string
	DataContentType string
	EventTimestamp  time.Time
	MaxRetries      int
}

// NewDLQEvent constructs a new DLQEvent in the pending state and computes
// the payload hash used for dedup.
func NewDLQEvent(p NewDLQEventParams) (*DLQEvent, error) {
	if p.MaxRetries < 1 {
		p.MaxRetries = 3
	}
	now := time.Now()
	return &DLQEvent{
		ID:              uuid.NewString(),
		EventID:         p.EventID,
		EventType:       p.EventType,
		EventSource:     p.EventSource,
		EventSubject:    p.EventSubject,
		SensorName:      p.SensorName,
		FailedTrigger:   p.FailedTrigger,
		EventSourceURL:  p.EventSourceURL,
		Namespace:       p.Namespace,
		OriginalPayload: p.OriginalPayload,
		PayloadHash:     HashPayload(p.OriginalPayload),
		OriginalHeaders: p.OriginalHeaders,
		DataContentType: p.DataContentType,
		EventTimestamp:  p.EventTimestamp,
		Status:          StatusPending,
		RetryCount:      0,
		MaxRetries:      p.MaxRetries,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

// HashPayload computes a SHA-256 hex digest of the payload bytes. Used as
// part of the dedup composite key.
func HashPayload(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// Replay transitions pending -> replayed. Rejects any other starting state.
func (e *DLQEvent) Replay() error {
	if e.Status != StatusPending {
		return ErrInvalidTransition
	}
	now := time.Now()
	e.Status = StatusReplayed
	e.LastReplayedAt = &now
	e.UpdatedAt = now
	return nil
}

// Resolve transitions replayed or pending -> resolved. Rejects already-resolved;
// rejects poisoned via ErrInvalidTransition.
func (e *DLQEvent) Resolve(by string) error {
	if e.Status == StatusResolved {
		return ErrAlreadyResolved
	}
	if e.Status != StatusReplayed && e.Status != StatusPending {
		return ErrInvalidTransition
	}
	now := time.Now()
	e.Status = StatusResolved
	e.ResolvedAt = &now
	e.ResolvedBy = by
	e.UpdatedAt = now
	return nil
}

// ReIngest is called when a replayed event fails again and arrives back in
// the DLQ. Increments retry_count; transitions to poisoned if the threshold
// is reached, otherwise back to pending.
func (e *DLQEvent) ReIngest() error {
	if e.Status == StatusResolved {
		return ErrAlreadyResolved
	}
	e.RetryCount++
	if e.RetryCount >= e.MaxRetries {
		e.Status = StatusPoisoned
	} else {
		e.Status = StatusPending
	}
	e.UpdatedAt = time.Now()
	return nil
}

// ResetPoison transitions poisoned -> pending and clears retry_count.
// Operator-triggered only.
func (e *DLQEvent) ResetPoison() error {
	if e.Status != StatusPoisoned {
		return ErrInvalidTransition
	}
	e.Status = StatusPending
	e.RetryCount = 0
	e.UpdatedAt = time.Now()
	return nil
}
