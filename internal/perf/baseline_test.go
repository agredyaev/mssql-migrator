package perf

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func baselinePath() string {
	return filepath.Join("..", "..", "internal", "app", "testdata", "perf", "footprint_baseline.json")
}

func TestStructSizeReport(t *testing.T) {
	sizes := CollectStructSizes()
	for _, e := range sizes {
		t.Logf("%s.%s: %d bytes (threshold %d)", e.Package, e.Type, e.Bytes, FootprintThreshold)
	}
	if len(sizes) == 0 {
		t.Fatal("expected at least one struct at or above threshold")
	}
	// Largest layout struct is typically types.MigrationPlan after phase 4 DOD shrinks fs.Object.
	if sizes[0].Package == "types" && sizes[0].Type == "MigrationPlan" {
		return
	}
	t.Logf("note: largest struct is %s.%s (%d B)", sizes[0].Package, sizes[0].Type, sizes[0].Bytes)
}

func TestFootprintBaselineMatch(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	path := baselinePath()
	baseline, err := ReadJSONFile(path)
	if err != nil {
		t.Skipf("baseline %s: %v (run make bench-footprint-update-baseline)", path, err)
	}
	got := CollectStructSizes()
	if len(got) != len(baseline.StructSizes) {
		t.Fatalf("struct size count: got %d, baseline %d (update baseline if intentional)", len(got), len(baseline.StructSizes))
	}
	for i := range got {
		if got[i] != baseline.StructSizes[i] {
			t.Fatalf("struct_sizes[%d]: got %+v, want %+v", i, got[i], baseline.StructSizes[i])
		}
	}
}

func TestFootprintBenchmarkRegression(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	if os.Getenv("RMIG_FOOTPRINT_BENCH") != "1" {
		t.Skip("set RMIG_FOOTPRINT_BENCH=1 to run benchmark regression (slow)")
	}
	path := baselinePath()
	baseline, err := ReadJSONFile(path)
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	current := RunFootprintBenchmarks()
	for _, c := range current {
		t.Logf("%s: %dns/op %d allocs/op %d B/op", c.Name, c.NsPerOp, c.AllocsPerOp, c.BytesPerOp)
	}
	const tolerance = 0.15
	if msgs := CompareBenchmarks(baseline.Benchmarks, current, tolerance); len(msgs) > 0 {
		for _, m := range msgs {
			t.Errorf("regression: %s", m)
		}
	}
}

func TestUpdateFootprintBaseline(t *testing.T) {
	if os.Getenv("RMIG_FOOTPRINT_UPDATE_BASELINE") != "1" {
		t.Skip("set RMIG_FOOTPRINT_UPDATE_BASELINE=1 to rewrite footprint_baseline.json")
	}
	benches := RunFootprintBenchmarks()
	for _, c := range benches {
		t.Logf("%s: %dns/op %d allocs/op %d B/op", c.Name, c.NsPerOp, c.AllocsPerOp, c.BytesPerOp)
	}
	b := NewFootprintBaseline(benches)
	path := baselinePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSONFile(path, b); err != nil {
		t.Fatal(err)
	}
	t.Logf("updated %s (%s/%s %s)", path, b.GOOS, b.GOARCH, runtime.Version())

	if art := os.Getenv("RMIG_FOOTPRINT_ARTIFACTS"); art != "" {
		_ = os.MkdirAll(art, 0o755)
		_ = WriteJSONFile(filepath.Join(art, "footprint_struct_sizes.json"), b)
	}
}
