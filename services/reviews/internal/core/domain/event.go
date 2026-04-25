package domain

// ReviewSubmittedEvent is emitted after a successful SubmitReview.
type ReviewSubmittedEvent struct {
	ID             string
	ProductID      string
	Reviewer       string
	Text           string
	IdempotencyKey string
}

// ReviewDeletedEvent is emitted after a successful DeleteReview.
type ReviewDeletedEvent struct {
	ReviewID       string
	ProductID      string
	IdempotencyKey string
}
