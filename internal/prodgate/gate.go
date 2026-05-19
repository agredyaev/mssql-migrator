package prodgate

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// PhaseTimings holds optional wall durations for prod gate reporting.
type PhaseTimings struct {
	ConnectMS  int64 `json:"connect_ms"`
	ScanMS     int64 `json:"scan_ms"`
	InspectMS  int64 `json:"inspect_ms"`
	AuditMS    int64 `json:"audit_ms"`
	DiffMS     int64 `json:"diff_ms"`
	PlanWallMS int64 `json:"plan_wall_ms"`
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
