package prodgate

import (
	"encoding/json"
	"os"

	"reporting-db-migrations/internal/types"
)

// E2EScenarioReport is the wire format for Go↔Rust scenario e2e (behavior + phase timings).
type E2EScenarioReport struct {
	Scenario     string         `json:"scenario"`
	SetupSteps   []string       `json:"setup_steps,omitempty"`
	Timings      PhaseTimings   `json:"timings"`
	Io           DbIoProfile    `json:"io"`
	Snapshot     PlanSnapshot   `json:"snapshot"`
	ActionCounts map[string]int `json:"action_counts"`
}

// E2EApplyReport is the wire format for migrate/apply outcome e2e.
type E2EApplyReport struct {
	Scenario        string   `json:"scenario"`
	SetupSteps      []string `json:"setup_steps,omitempty"`
	Applied         int      `json:"applied"`
	Failed          int      `json:"failed"`
	Skipped         int      `json:"skipped"`
	Errors          []string `json:"errors,omitempty"`
	AuditObjectRows int      `json:"audit_object_rows"`
}

// E2EGateReport is the wire format for prod gate e2e (Go↔Rust).
type E2EGateReport struct {
	Scenario   string       `json:"scenario"`
	SetupSteps []string     `json:"setup_steps,omitempty"`
	GateGo     bool         `json:"gate_go"`
	Messages   []string     `json:"messages,omitempty"`
	Snapshot   PlanSnapshot `json:"snapshot"`
	Gate       GateResult   `json:"gate"`
}

// E2EBlockedReport is the wire format for blocked migrate + scaffold e2e.
type E2EBlockedReport struct {
	Scenario      string   `json:"scenario"`
	SetupSteps    []string `json:"setup_steps,omitempty"`
	ExitCode      int      `json:"exit_code"`
	Blocked       bool     `json:"blocked"`
	Blockers      []string `json:"blockers,omitempty"`
	ScaffoldPaths []string `json:"scaffold_paths,omitempty"`
}

// ActionCountsFromPlan tallies planned_action values (scenario behavior summary).
func ActionCountsFromPlan(plan *types.MigrationPlan) map[string]int {
	out := map[string]int{}
	if plan == nil {
		return out
	}
	for _, obj := range plan.Objects {
		out[obj.PlannedAction]++
	}
	return out
}

// BuildE2EScenarioReport builds a comparable report for pipeline-level e2e.
func BuildE2EScenarioReport(scenario string, plan *types.MigrationPlan, timings PhaseTimings, io DbIoProfile) E2EScenarioReport {
	return E2EScenarioReport{
		Scenario:     scenario,
		Timings:      timings,
		Io:           io,
		Snapshot:     SnapshotFromPlan(plan),
		ActionCounts: ActionCountsFromPlan(plan),
	}
}

// WriteE2EReportFile writes report JSON (mode 0644).
func WriteE2EReportFile(path string, rep E2EScenarioReport) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// BuildE2EApplyReport builds apply outcome report for e2e.
func BuildE2EApplyReport(scenario string, applied, failed, skipped, auditRows int, errors []string) E2EApplyReport {
	return E2EApplyReport{
		Scenario:        scenario,
		Applied:         applied,
		Failed:          failed,
		Skipped:         skipped,
		Errors:          errors,
		AuditObjectRows: auditRows,
	}
}

// BuildE2EGateReport builds gate e2e report from Evaluate result.
func BuildE2EGateReport(scenario string, snap PlanSnapshot, result GateResult) E2EGateReport {
	return E2EGateReport{
		Scenario: scenario,
		GateGo:   result.Go,
		Messages: append([]string(nil), result.Messages...),
		Snapshot: snap,
		Gate:     result,
	}
}

// WriteE2EApplyReportFile writes E2EApplyReport JSON.
func WriteE2EApplyReportFile(path string, rep E2EApplyReport) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ReadE2EApplyReportFile loads E2EApplyReport from path.
func ReadE2EApplyReportFile(path string) (E2EApplyReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return E2EApplyReport{}, err
	}
	var rep E2EApplyReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return E2EApplyReport{}, err
	}
	return rep, nil
}

// WriteE2EGateReportFile writes E2EGateReport JSON.
func WriteE2EGateReportFile(path string, rep E2EGateReport) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ReadE2EGateReportFile loads E2EGateReport from path.
func ReadE2EGateReportFile(path string) (E2EGateReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return E2EGateReport{}, err
	}
	var rep E2EGateReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return E2EGateReport{}, err
	}
	if rep.Snapshot.Objects == nil {
		rep.Snapshot.Objects = map[string]ObjectEntry{}
	}
	return rep, nil
}

// WriteE2EBlockedReportFile writes E2EBlockedReport JSON.
func WriteE2EBlockedReportFile(path string, rep E2EBlockedReport) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ReadE2EBlockedReportFile loads E2EBlockedReport from path.
func ReadE2EBlockedReportFile(path string) (E2EBlockedReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return E2EBlockedReport{}, err
	}
	var rep E2EBlockedReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return E2EBlockedReport{}, err
	}
	return rep, nil
}

// ReadE2EReportFile loads E2EScenarioReport from path.
func ReadE2EReportFile(path string) (E2EScenarioReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return E2EScenarioReport{}, err
	}
	var rep E2EScenarioReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return E2EScenarioReport{}, err
	}
	if rep.ActionCounts == nil {
		rep.ActionCounts = map[string]int{}
	}
	if rep.Snapshot.Objects == nil {
		rep.Snapshot.Objects = map[string]ObjectEntry{}
	}
	return rep, nil
}
