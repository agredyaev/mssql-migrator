package types

import (
	"fmt"
	"testing"
)

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

// BenchmarkBuildDualINQuery_500x500 tracks IN placeholder expansion for large
// schema/object lists (same shape as inspector chunking).
func BenchmarkBuildDualINQuery_500x500(b *testing.B) {
	const tpl = "SELECT 1 WHERE s IN ({{schema_list}}) AND o IN ({{object_list}})"
	sc := make([]string, 500)
	oc := make([]string, 500)
	for i := range sc {
		sc[i] = fmt.Sprintf("sch_%03d", i)
	}
	for i := range oc {
		oc[i] = fmt.Sprintf("obj_%03d", i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = BuildDualINQuery(tpl, "{{schema_list}}", sc, "{{object_list}}", oc, 1)
	}
}
