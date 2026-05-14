package bus

import (
	"sync"

	"reporting-db-migrations/internal/types"
)

type EventBus interface {
	Publish(event types.Event, payload any)
	Subscribe(event types.Event, handler func(payload any))
}

type Bus struct {
	mu       sync.Mutex
	handlers map[types.Event][]func(payload any)
}

func New() *Bus {
	return &Bus{
		handlers: make(map[types.Event][]func(payload any)),
	}
}

func (b *Bus) Publish(event types.Event, payload any) {
	b.mu.Lock()
	handlers := b.handlers[event]
	b.mu.Unlock()

	for _, h := range handlers {
		h(payload)
	}
}

func (b *Bus) Subscribe(event types.Event, handler func(payload any)) {
	b.mu.Lock()
	b.handlers[event] = append(b.handlers[event], handler)
	b.mu.Unlock()
}
