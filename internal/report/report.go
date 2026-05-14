package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/types"
)

type Subscriber struct {
	bus     bus.EventBus
	baseDir string
	stdout  bool
	errf    func(msg string)
}

func NewSubscriber(b bus.EventBus, cfg types.Config) *Subscriber {
	s := &Subscriber{
		bus:     b,
		baseDir: cfg.ReportDir,
		stdout:  cfg.PlanJSON,
		errf:    func(msg string) {},
	}
	b.Subscribe(types.EventDiffComputed, s.onDiffComputed)
	b.Subscribe(types.EventRunFinished, s.onRunFinished)
	return s
}

func (s *Subscriber) SetErrorHandler(fn func(msg string)) {
	s.errf = fn
}

func (s *Subscriber) onDiffComputed(payload any) {
	result, ok := payload.(*types.DiffResult)
	if !ok || result.Plan == nil {
		return
	}

	data, err := json.MarshalIndent(result.Plan, "", "  ")
	if err != nil {
		s.errf("report marshal plan: " + err.Error())
		return
	}

	path := filepath.Join(s.baseDir, ".plan.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		s.errf(fmt.Sprintf("report write %s: %s", path, err.Error()))
	}
}

func (s *Subscriber) onRunFinished(payload any) {
	result, ok := payload.(*types.RunFinished)
	if !ok {
		return
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		s.errf("report marshal report: " + err.Error())
		return
	}

	path := filepath.Join(s.baseDir, ".report.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		s.errf(fmt.Sprintf("report write %s: %s", path, err.Error()))
	}
}
