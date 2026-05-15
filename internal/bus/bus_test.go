package bus

import (
	"context"
	"sync"
	"testing"

	"reporting-db-migrations/internal/types"
)

func TestPublishDeliversToSubscriber(t *testing.T) {
	b := New()
	var received any
	b.Subscribe(types.EventRunStarted, func(_ context.Context, payload any) {
		received = payload
	})

	payload := &types.RunStarted{Command: "plan"}
	b.Publish(context.Background(), types.EventRunStarted, payload)

	if received != payload {
		t.Fatal("subscriber did not receive published payload")
	}
}

func TestPublishDeliversToMultipleSubscribers(t *testing.T) {
	b := New()
	var received1, received2 any
	b.Subscribe(types.EventDiffComputed, func(_ context.Context, payload any) {
		received1 = payload
	})
	b.Subscribe(types.EventDiffComputed, func(_ context.Context, payload any) {
		received2 = payload
	})

	payload := &types.DiffResult{Plan: &types.MigrationPlan{Command: "plan"}}
	b.Publish(context.Background(), types.EventDiffComputed, payload)

	if received1 != payload {
		t.Error("subscriber 1 did not receive payload")
	}
	if received2 != payload {
		t.Error("subscriber 2 did not receive payload")
	}
}

func TestPublishToEventWithNoSubscribers(t *testing.T) {
	b := New()
	b.Publish(context.Background(), types.EventObjectApplied, &types.ObjectEvent{})
}

func TestDifferentEventsAreDeliveredIndependently(t *testing.T) {
	b := New()
	var started, finished any
	b.Subscribe(types.EventRunStarted, func(_ context.Context, payload any) {
		started = payload
	})
	b.Subscribe(types.EventRunFinished, func(_ context.Context, payload any) {
		finished = payload
	})

	startPayload := &types.RunStarted{Command: "migrate"}
	b.Publish(context.Background(), types.EventRunStarted, startPayload)

	if got, ok := finished.(*types.RunFinished); ok {
		t.Errorf("finished subscriber received wrong payload: %v", got)
	}

	finishPayload := &types.RunFinished{Command: "migrate", Result: "success"}
	b.Publish(context.Background(), types.EventRunFinished, finishPayload)

	if started != startPayload {
		t.Error("started subscriber did not receive payload")
	}
	if finished != finishPayload {
		t.Error("finished subscriber did not receive payload")
	}
}

func TestConcurrentPublishAndSubscribe(t *testing.T) {
	b := New()
	var mu sync.Mutex
	var received []any

	b.Subscribe(types.EventRunStarted, func(_ context.Context, payload any) {
		mu.Lock()
		received = append(received, payload)
		mu.Unlock()
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Publish(context.Background(), types.EventRunStarted, &types.RunStarted{Command: "plan"})
		}()
	}
	wg.Wait()

	mu.Lock()
	count := len(received)
	mu.Unlock()
	if count != 100 {
		t.Errorf("expected 100 deliveries, got %d", count)
	}
}

func TestPublishRecoversFromPanicInHandler(t *testing.T) {
	b := New()
	var ok bool

	b.Subscribe(types.EventRunStarted, func(_ context.Context, payload any) {
		panic("boom")
	})
	b.Subscribe(types.EventRunStarted, func(_ context.Context, payload any) {
		ok = true
	})

	b.Publish(context.Background(), types.EventRunStarted, &types.RunStarted{Command: "plan"})

	if !ok {
		t.Error("second handler did not execute after panic in first")
	}
}
