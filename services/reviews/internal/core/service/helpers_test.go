package service_test

import (
	"context"
	"sync"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/reviews/internal/core/domain"
)

type fakeReviewPublisher struct {
	mu        sync.Mutex
	submitted []domain.ReviewSubmittedEvent
	deleted   []domain.ReviewDeletedEvent
	forceErr  error
}

func (f *fakeReviewPublisher) PublishReviewSubmitted(_ context.Context, evt domain.ReviewSubmittedEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submitted = append(f.submitted, evt)
	return f.forceErr
}

func (f *fakeReviewPublisher) PublishReviewDeleted(_ context.Context, evt domain.ReviewDeletedEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, evt)
	return f.forceErr
}
