package report

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/types"
)

type Subscriber struct {
	bus      bus.EventBus
	notifier types.ErrorNotifier
	baseDir  string
}

func NewSubscriber(b bus.EventBus, cfg types.Config) *Subscriber {
	s := &Subscriber{
		bus:     b,
		baseDir: cfg.ReportDir,
	}
	b.Subscribe(types.EventDiffComputed, s.onDiffComputed)
	b.Subscribe(types.EventRunFinished, s.onRunFinished)
	return s
}

func (s *Subscriber) SetErrorHandler(fn func(msg string)) {
	s.notifier.SetErrorHandler(fn)
}

func (s *Subscriber) writeJSON(filename string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		s.notifier.Notify("report marshal: " + err.Error())
		return
	}
	path := filepath.Join(s.baseDir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		s.notifier.Notify(fmt.Sprintf("report write %s: %s", path, err.Error()))
	}
}

func (s *Subscriber) onDiffComputed(_ context.Context, payload any) {
	result, ok := bus.ParseDiffResult(payload)
	if !ok || result.Plan == nil {
		return
	}
	s.writeJSON(".plan.json", result.Plan)
}

func (s *Subscriber) onRunFinished(_ context.Context, payload any) {
	result, ok := bus.ParseRunFinished(payload)
	if !ok {
		return
	}
	s.writeJSON(".report.json", result)
}
