// Package domain contains pure domain types for the details service.
package domain

// BookAddedEvent is the domain event emitted after a successful AddDetail.
// Carries the business payload shape consumed by downstream services (e.g. notification).
type BookAddedEvent struct {
	ID             string
	Title          string
	Author         string
	Year           int
	Type           string
	Pages          int
	Publisher      string
	Language       string
	ISBN10         string
	ISBN13         string
	IdempotencyKey string
}
