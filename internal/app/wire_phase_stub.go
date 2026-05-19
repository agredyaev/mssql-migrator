//go:build !integration

package app

import (
	"time"

	"reporting-db-migrations/internal/engine"
	"reporting-db-migrations/internal/fs"
)

func applyIntegrationHooks(_ *engine.Engine) {}

func integrationFlushObserver() func(time.Duration) { return nil }

func integrationReportObserver() func(time.Duration) { return nil }

func integrationScanPhaseHook() fs.ScanPhaseObserver { return nil }
