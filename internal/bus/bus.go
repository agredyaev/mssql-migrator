package bus

import (
	"context"
	"sync"

	"reporting-db-migrations/internal/types"
)

type EventBus interface {
	Publish(ctx context.Context, event types.Event, payload any)
	Subscribe(event types.Event, handler func(ctx context.Context, payload any))
}

type Bus struct {
	mu       sync.Mutex
	handlers map[types.Event][]func(ctx context.Context, payload any)
}

func New() *Bus {
	return &Bus{
		handlers: make(map[types.Event][]func(ctx context.Context, payload any)),
	}
}

func (b *Bus) Publish(ctx context.Context, event types.Event, payload any) {
	b.mu.Lock()
	handlers := b.handlers[event]
	b.mu.Unlock()

	for _, h := range handlers {
		func() {
			defer func() { recover() }()
			h(ctx, payload)
		}()
	}
}

func (b *Bus) Subscribe(event types.Event, handler func(ctx context.Context, payload any)) {
	b.mu.Lock()
	b.handlers[event] = append(b.handlers[event], handler)
	b.mu.Unlock()
}
