package bus

import (
	"context"
	"testing"
	"time"

	"reporting-db-migrations/internal/types"
)

func BenchmarkBusPublish_1Handler_Empty(b *testing.B) {
	benchmarkBusPublish(b, 1, func(context.Context, any) {})
}

func BenchmarkBusPublish_10Handlers_Empty(b *testing.B) {
	benchmarkBusPublish(b, 10, func(context.Context, any) {})
}

func BenchmarkBusPublish_100Handlers_Empty(b *testing.B) {
	benchmarkBusPublish(b, 100, func(context.Context, any) {})
}

func BenchmarkBusPublish_10Handlers_Sleep100ns(b *testing.B) {
	benchmarkBusPublish(b, 10, func(context.Context, any) {
		time.Sleep(100 * time.Nanosecond)
	})
}

func BenchmarkBusPublish_10Handlers_Panic(b *testing.B) {
	benchmarkBusPublish(b, 10, func(context.Context, any) {
		panic("bench")
	})
}

func benchmarkBusPublish(b *testing.B, n int, h func(context.Context, any)) {
	b.Helper()
	ev := types.EventRunStarted
	bus := New()
	for i := 0; i < n; i++ {
		bus.Subscribe(ev, h)
	}
	payload := &types.RunStarted{Command: "plan"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish(context.Background(), ev, payload)
	}
}
