package prodgate

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// PhaseTimings holds optional wall durations for plan/CLI profiling (milliseconds).
type PhaseTimings struct {
	ConnectMS       int64 `json:"connect_ms,omitempty"`
	ScanMS          int64 `json:"scan_ms,omitempty"`
	ScanWalkMS      int64 `json:"scan_walk_ms,omitempty"`
	ScanGitMS       int64 `json:"scan_git_ms,omitempty"`
	ScanChecksumsMS int64 `json:"scan_checksums_ms,omitempty"`
	FetchMS         int64 `json:"fetch_ms,omitempty"`
	InspectMS       int64 `json:"inspect_ms,omitempty"`
	ChecksumsMS     int64 `json:"checksums_ms,omitempty"`
	EnsureMS        int64 `json:"ensure_ms,omitempty"`
	ParallelWallMS  int64 `json:"parallel_wall_ms,omitempty"`
	// AuditMS is ensure_ms + checksums_ms (legacy gate SLO field; not parallel overlap).
	AuditMS       int64 `json:"audit_ms,omitempty"`
	DiffMS        int64 `json:"diff_ms,omitempty"`
	PlanWallMS    int64 `json:"plan_wall_ms,omitempty"`
	CLIWallMS     int64 `json:"cli_wall_ms,omitempty"`
	EngineMS      int64 `json:"engine_ms,omitempty"`
	ReportWriteMS int64 `json:"report_write_ms,omitempty"`
	ApplyMS       int64 `json:"apply_ms,omitempty"`
	AuditFlushMS  int64 `json:"audit_flush_ms,omitempty"`
}

// GateInput bundles snapshot comparison and optional SLO checks.
type GateInput struct {
	Baseline         PlanSnapshot
	Current          PlanSnapshot
	DeltaKeys        map[string]struct{}
	StrictUnexpected bool
	Timings          PhaseTimings
	MaxPlanWallMS    int64 // 0 = no SLO
}

// GateResult is the go/no-go verdict for operators.
type GateResult struct {
	Go            bool          `json:"go"`
	Messages      []string      `json:"messages,omitempty"`
	Compare       CompareResult `json:"compare"`
	Timings       PhaseTimings  `json:"timings,omitempty"`
	DeltaKeyCount int           `json:"delta_key_count"`
}

// Evaluate runs incremental comparison and optional wall-time SLO.
func Evaluate(in GateInput) GateResult {
	opts := CompareOptions{
		DeltaKeys:        in.DeltaKeys,
		StrictUnexpected: in.StrictUnexpected,
	}
	cmp := CompareSnapshots(in.Baseline, in.Current, opts)
	out := GateResult{
		Go:            cmp.Go,
		Messages:      append([]string(nil), cmp.Messages...),
		Compare:       cmp,
		Timings:       in.Timings,
		DeltaKeyCount: len(in.DeltaKeys),
	}
	if in.MaxPlanWallMS > 0 && in.Timings.PlanWallMS > in.MaxPlanWallMS {
		out.Go = false
		out.Messages = append(out.Messages, fmt.Sprintf("plan wall SLO exceeded: %dms > %dms", in.Timings.PlanWallMS, in.MaxPlanWallMS))
	}
	return out
}

// MaxPlanWallMSFromEnv reads RMIG_GATE_MAX_PLAN_WALL_MS (milliseconds).
func MaxPlanWallMSFromEnv() int64 {
	raw := strings.TrimSpace(os.Getenv("RMIG_GATE_MAX_PLAN_WALL_MS"))
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// DurMS converts duration to integer milliseconds for reports.
func DurMS(d time.Duration) int64 {
	return d.Milliseconds()
}
