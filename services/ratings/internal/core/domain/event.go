package domain

// RatingSubmittedEvent is emitted after a successful SubmitRating.
type RatingSubmittedEvent struct {
	ID             string
	ProductID      string
	Reviewer       string
	Stars          int
	IdempotencyKey string
}
