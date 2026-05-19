package perf

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
)

const BaselineVersion = 1

// BenchEntry holds one go test -bench result.
type BenchEntry struct {
	Name        string  `json:"name"`
	NsPerOp     int64   `json:"ns_per_op"`
	AllocsPerOp int64   `json:"allocs_per_op"`
	BytesPerOp  int64   `json:"bytes_per_op"`
	MBPerSec    float64 `json:"mb_per_sec,omitempty"`
}

// FootprintBaseline is the committed performance / footprint snapshot.
type FootprintBaseline struct {
	Version        int               `json:"version"`
	GoVersion      string            `json:"go_version"`
	GOOS           string            `json:"goos"`
	GOARCH         string            `json:"goarch"`
	ThresholdBytes int               `json:"threshold_bytes"`
	StructSizes    []StructSizeEntry `json:"struct_sizes"`
	Benchmarks     []BenchEntry      `json:"benchmarks"`
}

// NewFootprintBaseline builds a baseline from current struct sizes and bench results.
func NewFootprintBaseline(benches []BenchEntry) FootprintBaseline {
	return FootprintBaseline{
		Version:        BaselineVersion,
		GoVersion:      runtime.Version(),
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		ThresholdBytes: FootprintThreshold,
		StructSizes:    CollectStructSizes(),
		Benchmarks:     benches,
	}
}

// WriteJSONFile writes baseline to path (mode 0644).
func WriteJSONFile(path string, b FootprintBaseline) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ReadJSONFile loads a FootprintBaseline from path.
func ReadJSONFile(path string) (FootprintBaseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FootprintBaseline{}, err
	}
	var b FootprintBaseline
	if err := json.Unmarshal(data, &b); err != nil {
		return FootprintBaseline{}, err
	}
	if b.Version == 0 {
		b.Version = BaselineVersion
	}
	return b, nil
}

// CompareBenchmarks returns human-readable diffs when current exceeds baseline tolerances.
// tolerance is a fraction (0.10 = 10% regression allowed).
func CompareBenchmarks(baseline, current []BenchEntry, tolerance float64) []string {
	byName := make(map[string]BenchEntry, len(baseline))
	for _, e := range baseline {
		byName[e.Name] = e
	}
	var msgs []string
	for _, cur := range current {
		base, ok := byName[cur.Name]
		if !ok {
			msgs = append(msgs, fmt.Sprintf("benchmark %q: new (not in baseline)", cur.Name))
			continue
		}
		if base.NsPerOp > 0 {
			ratio := float64(cur.NsPerOp-base.NsPerOp) / float64(base.NsPerOp)
			if ratio > tolerance {
				msgs = append(msgs, fmt.Sprintf("benchmark %q: ns/op %d -> %d (+%.1f%%)", cur.Name, base.NsPerOp, cur.NsPerOp, ratio*100))
			}
		}
		if base.AllocsPerOp > 0 {
			ratio := float64(cur.AllocsPerOp-base.AllocsPerOp) / float64(base.AllocsPerOp)
			if ratio > tolerance {
				msgs = append(msgs, fmt.Sprintf("benchmark %q: allocs/op %d -> %d (+%.1f%%)", cur.Name, base.AllocsPerOp, cur.AllocsPerOp, ratio*100))
			}
		}
		if base.BytesPerOp > 0 {
			ratio := float64(cur.BytesPerOp-base.BytesPerOp) / float64(base.BytesPerOp)
			if ratio > tolerance {
				msgs = append(msgs, fmt.Sprintf("benchmark %q: B/op %d -> %d (+%.1f%%)", cur.Name, base.BytesPerOp, cur.BytesPerOp, ratio*100))
			}
		}
	}
	return msgs
}
