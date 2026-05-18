package bus

import (
	"context"
	"sync"

	"reporting-db-migrations/internal/types"
)

type EventBus interface {
	Publish(ctx context.Context, event types.Event, payload any)
	Subscribe(event types.Event, handler func(ctx context.Context, payload any))
	// HasHandlers reports whether Subscribe registered any handler for the event.
	// Hot paths use this to avoid building payloads nobody observes.
	HasHandlers(event types.Event) bool
}

type Bus struct {
	handlers map[types.Event][]func(ctx context.Context, payload any)
	mu       sync.Mutex
}

func New() *Bus {
	return &Bus{
		handlers: make(map[types.Event][]func(ctx context.Context, payload any)),
	}
}

func (b *Bus) HasHandlers(event types.Event) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.handlers[event]) > 0
}

func (b *Bus) Publish(ctx context.Context, event types.Event, payload any) {
	b.mu.Lock()
	handlers := b.handlers[event]
	if len(handlers) == 0 {
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()

	for _, h := range handlers {
		invokeBusHandler(ctx, h, payload)
	}
}

func invokeBusHandler(ctx context.Context, h func(context.Context, any), payload any) {
	defer func() { recover() }()
	h(ctx, payload)
}

func (b *Bus) Subscribe(event types.Event, handler func(ctx context.Context, payload any)) {
	b.mu.Lock()
	b.handlers[event] = append(b.handlers[event], handler)
	b.mu.Unlock()
}
