package bus

import (
	"testing"

	"reporting-db-migrations/internal/types"
)

func TestPublishDeliversToSubscriber(t *testing.T) {
	b := New()
	var received any
	b.Subscribe(types.EventRunStarted, func(payload any) {
		received = payload
	})

	payload := &types.RunStarted{Command: "plan"}
	b.Publish(types.EventRunStarted, payload)

	if received != payload {
		t.Fatal("subscriber did not receive published payload")
	}
}

func TestPublishDeliversToMultipleSubscribers(t *testing.T) {
	b := New()
	var received1, received2 any
	b.Subscribe(types.EventDiffComputed, func(payload any) {
		received1 = payload
	})
	b.Subscribe(types.EventDiffComputed, func(payload any) {
		received2 = payload
	})

	payload := &types.DiffResult{Plan: &types.MigrationPlan{Command: "plan"}}
	b.Publish(types.EventDiffComputed, payload)

	if received1 != payload {
		t.Error("subscriber 1 did not receive payload")
	}
	if received2 != payload {
		t.Error("subscriber 2 did not receive payload")
	}
}

func TestPublishToEventWithNoSubscribers(t *testing.T) {
	b := New()
	b.Publish(types.EventObjectApplied, &types.ObjectEvent{})
}

func TestDifferentEventsAreDeliveredIndependently(t *testing.T) {
	b := New()
	var started, finished any
	b.Subscribe(types.EventRunStarted, func(payload any) {
		started = payload
	})
	b.Subscribe(types.EventRunFinished, func(payload any) {
		finished = payload
	})

	startPayload := &types.RunStarted{Command: "migrate"}
	b.Publish(types.EventRunStarted, startPayload)

	if got, ok := finished.(*types.RunFinished); ok {
		t.Errorf("finished subscriber received wrong payload: %v", got)
	}

	finishPayload := &types.RunFinished{Command: "migrate", Result: "success"}
	b.Publish(types.EventRunFinished, finishPayload)

	if started != startPayload {
		t.Error("started subscriber did not receive payload")
	}
	if finished != finishPayload {
		t.Error("finished subscriber did not receive payload")
	}
}
