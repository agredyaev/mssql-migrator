//go:build integration

package app

import (
	"time"

	"reporting-db-migrations/internal/engine"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/prodgate"
)

var (
	testPhaseObserver   engine.PhaseObserver
	integrationTimings  *prodgate.PhaseTimings
	integrationOnFlush  func(time.Duration)
	integrationOnReport func(time.Duration)
)

func applyIntegrationHooks(eng *engine.Engine) {
	if testPhaseObserver != nil {
		eng.SetPhaseObserver(testPhaseObserver)
	}
}

func enableIntegrationPhaseTrace(timings *prodgate.PhaseTimings) {
	integrationTimings = timings
	testPhaseObserver = func(phase string, d time.Duration) {
		ms := prodgate.DurMS(d)
		switch phase {
		case "scan":
			timings.ScanMS = ms
		case "scan_walk":
			timings.ScanWalkMS = ms
		case "scan_git":
			timings.ScanGitMS = ms
		case "scan_checksums":
			timings.ScanChecksumsMS = ms
		case "ensure":
			timings.EnsureMS = ms
		case "inspect":
			timings.InspectMS = ms
		case "checksums":
			timings.ChecksumsMS = ms
		case "parallel_wall":
			timings.ParallelWallMS = ms
		case "diff":
			timings.DiffMS = ms
		case "apply":
			timings.ApplyMS = ms
		case "engine":
			timings.EngineMS = ms
		}
	}
	integrationOnFlush = func(d time.Duration) {
		timings.AuditFlushMS = prodgate.DurMS(d)
	}
	integrationOnReport = func(d time.Duration) {
		timings.ReportWriteMS += prodgate.DurMS(d)
	}
}

func disableIntegrationPhaseTrace() {
	integrationTimings = nil
	testPhaseObserver = nil
	integrationOnFlush = nil
	integrationOnReport = nil
}

func integrationFlushObserver() func(time.Duration) { return integrationOnFlush }

func integrationReportObserver() func(time.Duration) { return integrationOnReport }

func integrationScanPhaseHook() fs.ScanPhaseObserver {
	if integrationTimings == nil {
		return nil
	}
	return func(phase string, d time.Duration) {
		ms := prodgate.DurMS(d)
		switch phase {
		case "scan_walk":
			integrationTimings.ScanWalkMS = ms
		case "scan_git":
			integrationTimings.ScanGitMS = ms
		case "scan_checksums":
			integrationTimings.ScanChecksumsMS = ms
		}
	}
}
