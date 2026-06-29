package event

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestInMemoryBus_PublishSubscribe(t *testing.T) {
	bus := NewInMemoryBus()

	var mu sync.Mutex
	var received []Event

	unsub, err := bus.Subscribe(func(ctx context.Context, e Event) error {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
		return nil
	}, TypeTaskCreated, TypePlanningStarted)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsub()

	ctx := context.Background()
	e1 := Event{ID: "1", Type: TypeTaskCreated, Timestamp: time.Now(), RunID: "run-1"}
	e2 := Event{ID: "2", Type: TypePlanningStarted, Timestamp: time.Now(), RunID: "run-1"}

	if err := bus.Publish(ctx, &e1); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if err := bus.Publish(ctx, &e2); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	mu.Lock()
	if len(received) != 2 {
		t.Fatalf("got %d events, want 2", len(received))
	}
	if received[0].ID != "1" {
		t.Errorf("event[0].ID = %q, want %q", received[0].ID, "1")
	}
	if received[1].ID != "2" {
		t.Errorf("event[1].ID = %q, want %q", received[1].ID, "2")
	}
	mu.Unlock()
}

func TestInMemoryBus_FilterByType(t *testing.T) {
	bus := NewInMemoryBus()

	var mu sync.Mutex
	var received []Event

	unsub, err := bus.Subscribe(func(ctx context.Context, e Event) error {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
		return nil
	}, TypeToolStarted)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsub()

	ctx := context.Background()
	_ = bus.Publish(ctx, &Event{ID: "1", Type: TypeTaskCreated})
	_ = bus.Publish(ctx, &Event{ID: "2", Type: TypeToolStarted})
	_ = bus.Publish(ctx, &Event{ID: "3", Type: TypeToolFinished})

	mu.Lock()
	if len(received) != 1 {
		t.Fatalf("got %d events, want 1 (only ToolStarted should match)", len(received))
	}
	if received[0].ID != "2" {
		t.Errorf("event.ID = %q, want %q", received[0].ID, "2")
	}
	mu.Unlock()
}

func TestInMemoryBus_SubscribeAll(t *testing.T) {
	bus := NewInMemoryBus()

	var count int
	unsub, err := bus.Subscribe(func(ctx context.Context, e Event) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsub()

	ctx := context.Background()
	_ = bus.Publish(ctx, &Event{Type: TypeTaskCreated})
	_ = bus.Publish(ctx, &Event{Type: TypeRunCompleted})

	if count != 2 {
		t.Fatalf("got %d events, want 2", count)
	}
}

func TestInMemoryBus_Unsubscribe(t *testing.T) {
	bus := NewInMemoryBus()

	var count int
	unsub, _ := bus.Subscribe(func(ctx context.Context, e Event) error {
		count++
		return nil
	})

	ctx := context.Background()
	_ = bus.Publish(ctx, &Event{Type: TypeTaskCreated})
	unsub()
	_ = bus.Publish(ctx, &Event{Type: TypeRunCompleted})

	if count != 1 {
		t.Fatalf("got %d events, want 1 (after unsubscribe)", count)
	}
}

func TestInMemoryBus_Close(t *testing.T) {
	bus := NewInMemoryBus()

	var count int
	_, _ = bus.Subscribe(func(ctx context.Context, e Event) error {
		count++
		return nil
	})

	_ = bus.Close()

	ctx := context.Background()
	_ = bus.Publish(ctx, &Event{Type: TypeTaskCreated})

	if count != 0 {
		t.Fatalf("got %d events, want 0 (after close)", count)
	}
}

func TestFileStore_HandlerAndReplay(t *testing.T) {
	store, err := NewFileStore(t.TempDir() + "/events.jsonl")
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	handler := store.Handler()

	e1 := Event{ID: "1", Type: TypeTaskCreated, RunID: "run-1"}
	e2 := Event{ID: "2", Type: TypeRunCompleted, RunID: "run-1"}

	if err := handler(ctx, e1); err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if err := handler(ctx, e2); err != nil {
		t.Fatalf("handler() error = %v", err)
	}

	var replayed []Event
	err = store.Replay(ctx, func(ctx context.Context, e Event) error {
		replayed = append(replayed, e)
		return nil
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if len(replayed) != 2 {
		t.Fatalf("got %d events, want 2", len(replayed))
	}
	if replayed[0].ID != "1" {
		t.Errorf("replayed[0].ID = %q, want %q", replayed[0].ID, "1")
	}
	if replayed[1].ID != "2" {
		t.Errorf("replayed[1].ID = %q, want %q", replayed[1].ID, "2")
	}
}
