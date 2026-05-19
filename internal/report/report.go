package report

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/types"
)

type Subscriber struct {
	bus           bus.EventBus
	notifier      types.ErrorNotifier
	baseDir       string
	syncDisk      bool
	pendingPlan   *types.MigrationPlan
	writeObserver func(time.Duration)
}

func NewSubscriber(b bus.EventBus, cfg types.Config) *Subscriber {
	s := &Subscriber{
		bus:      b,
		baseDir:  cfg.ReportDir,
		syncDisk: cfg.ReportSync,
	}
	b.Subscribe(types.EventDiffComputed, s.onDiffComputed)
	b.Subscribe(types.EventRunFinished, s.onRunFinished)
	return s
}

func (s *Subscriber) SetErrorHandler(fn func(msg string)) {
	s.notifier.SetErrorHandler(fn)
}

// SetWriteObserver is optional; called after each report file write (integration profiling).
func (s *Subscriber) SetWriteObserver(fn func(time.Duration)) {
	s.writeObserver = fn
}

func (s *Subscriber) writeJSON(filename string, v any) {
	start := time.Now()
	path := filepath.Join(s.baseDir, filename)
	f, err := os.Create(path)
	if err != nil {
		s.notifier.Notify(fmt.Sprintf("report create %s: %s", path, err.Error()))
		return
	}
	defer f.Close()

	bw := bufio.NewWriter(f)
	enc := json.NewEncoder(bw)
	if err := enc.Encode(v); err != nil {
		s.notifier.Notify(fmt.Sprintf("report marshal: %s", err.Error()))
		return
	}
	if err := bw.Flush(); err != nil {
		s.notifier.Notify(fmt.Sprintf("report flush %s: %s", path, err.Error()))
		return
	}
	if s.syncDisk {
		if err := f.Sync(); err != nil {
			s.notifier.Notify(fmt.Sprintf("report sync %s: %s", path, err.Error()))
		}
	}
	if s.writeObserver != nil {
		s.writeObserver(time.Since(start))
	}
}

func (s *Subscriber) onDiffComputed(_ context.Context, payload any) {
	result, ok := bus.ParseDiffResult(payload)
	if !ok || result.Plan == nil {
		return
	}
	s.pendingPlan = result.Plan
}

func (s *Subscriber) onRunFinished(_ context.Context, payload any) {
	result, ok := bus.ParseRunFinished(payload)
	if !ok {
		return
	}
	if s.pendingPlan != nil {
		s.writeJSON(".plan.json", s.pendingPlan)
		s.pendingPlan = nil
	}
	s.writeJSON(".report.json", result)
}
