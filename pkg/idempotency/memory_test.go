package idempotency_test

import (
	"context"
	"sync"
	"testing"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/idempotency"
)

func TestMemoryStore_CheckAndRecord(t *testing.T) {
	ctx := context.Background()
	s := idempotency.NewMemoryStore()

	seen, err := s.CheckAndRecord(ctx, "key-1")
	if err != nil {
		t.Fatalf("first call err = %v", err)
	}
	if seen {
		t.Error("first call: expected seen=false, got true")
	}

	seen, err = s.CheckAndRecord(ctx, "key-1")
	if err != nil {
		t.Fatalf("second call err = %v", err)
	}
	if !seen {
		t.Error("second call: expected seen=true, got false")
	}
}

func TestMemoryStore_Concurrent(t *testing.T) {
	ctx := context.Background()
	s := idempotency.NewMemoryStore()

	var wg sync.WaitGroup
	seenCount := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			seen, err := s.CheckAndRecord(ctx, "shared-key")
			if err != nil {
				t.Errorf("err = %v", err)
				return
			}
			seenCount <- seen
		}()
	}
	wg.Wait()
	close(seenCount)

	var firstFalse, laterTrue int
	for seen := range seenCount {
		if seen {
			laterTrue++
		} else {
			firstFalse++
		}
	}
	if firstFalse != 1 {
		t.Errorf("expected exactly one first-writer, got %d", firstFalse)
	}
	if laterTrue != 99 {
		t.Errorf("expected 99 already-processed, got %d", laterTrue)
	}
}
