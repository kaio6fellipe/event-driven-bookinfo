package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/adapter/outbound/memory"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/domain"
	"github.com/kaio6fellipe/event-driven-bookinfo/services/dlqueue/internal/core/port"
)

func newEvent(t *testing.T) *domain.DLQEvent {
	t.Helper()
	e, err := domain.NewDLQEvent(domain.NewDLQEventParams{
		EventID:         "evt-1",
		SensorName:      "sensor-a",
		FailedTrigger:   "trigger-a",
		OriginalPayload: []byte(`{"k":"v"}`),
		EventSource:     "source-a",
		MaxRetries:      3,
		EventTimestamp:  time.Now(),
	})
	if err != nil {
		t.Fatalf("NewDLQEvent err = %v", err)
	}
	return e
}

func TestMemory_SaveFindByID(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDLQRepository()
	e := newEvent(t)

	if err := repo.Save(ctx, e); err != nil {
		t.Fatalf("Save err = %v", err)
	}

	got, err := repo.FindByID(ctx, e.ID)
	if err != nil {
		t.Fatalf("FindByID err = %v", err)
	}
	if got.ID != e.ID {
		t.Errorf("got.ID = %q, want %q", got.ID, e.ID)
	}
}

func TestMemory_FindByID_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDLQRepository()
	_, err := repo.FindByID(ctx, "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestMemory_FindByNaturalKey(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDLQRepository()
	e := newEvent(t)
	_ = repo.Save(ctx, e)

	got, err := repo.FindByNaturalKey(ctx, e.SensorName, e.FailedTrigger, e.PayloadHash)
	if err != nil {
		t.Fatalf("FindByNaturalKey err = %v", err)
	}
	if got.ID != e.ID {
		t.Errorf("got.ID = %q, want %q", got.ID, e.ID)
	}

	_, err = repo.FindByNaturalKey(ctx, "other", e.FailedTrigger, e.PayloadHash)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for wrong sensor")
	}
}

func TestMemory_Update(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDLQRepository()
	e := newEvent(t)
	_ = repo.Save(ctx, e)

	_ = e.Replay()
	if err := repo.Update(ctx, e); err != nil {
		t.Fatalf("Update err = %v", err)
	}

	got, _ := repo.FindByID(ctx, e.ID)
	if got.Status != domain.StatusReplayed {
		t.Errorf("status = %v, want replayed", got.Status)
	}
}

func TestMemory_List_FilterByStatus(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewDLQRepository()

	// Create 3 events with distinct sensor/trigger so natural-key is unique.
	for i := 0; i < 3; i++ {
		e, err := domain.NewDLQEvent(domain.NewDLQEventParams{
			EventID:         "evt-" + string(rune('a'+i)),
			SensorName:      "sensor-" + string(rune('a'+i)),
			FailedTrigger:   "trigger-" + string(rune('a'+i)),
			OriginalPayload: []byte(`{"i":` + string(rune('0'+i)) + `}`),
			EventSource:     "source-a",
			MaxRetries:      3,
			EventTimestamp:  time.Now(),
		})
		if err != nil {
			t.Fatalf("NewDLQEvent err = %v", err)
		}
		_ = repo.Save(ctx, e)
	}
	// Replay one of them.
	all, _, _ := repo.List(ctx, port.ListFilter{Limit: 10})
	_ = all[0].Replay()
	_ = repo.Update(ctx, &all[0])

	pending, total, err := repo.List(ctx, port.ListFilter{Status: string(domain.StatusPending), Limit: 10})
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if total != 2 {
		t.Errorf("pending total = %d, want 2", total)
	}
	if len(pending) != 2 {
		t.Errorf("pending len = %d, want 2", len(pending))
	}
}
