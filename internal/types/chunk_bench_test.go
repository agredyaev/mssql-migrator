package types

import "testing"

// BenchmarkChunkKeys measures allocation overhead for IN-list chunking used by
// db inspector queries (many small chunks when parameter limit is large).
func BenchmarkChunkKeys_10k_2100(b *testing.B) {
	keys := make([]string, 10000)
	for i := range keys {
		keys[i] = "k"
	}
	const chunk = 2100
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ChunkKeys(keys, chunk)
	}
}
