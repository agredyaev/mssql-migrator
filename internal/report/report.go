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
	errf    func(msg string)
}

func NewSubscriber(b bus.EventBus, cfg types.Config) *Subscriber {
	s := &Subscriber{
		bus:     b,
		baseDir: cfg.ReportDir,
		errf:    func(msg string) {},
	}
	b.Subscribe(types.EventDiffComputed, s.onDiffComputed)
	b.Subscribe(types.EventRunFinished, s.onRunFinished)
	return s
}

func (s *Subscriber) SetErrorHandler(fn func(msg string)) {
	s.errf = fn
}

func (s *Subscriber) writeJSON(filename string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		s.errf("report marshal: " + err.Error())
		return
	}
	path := filepath.Join(s.baseDir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		s.errf(fmt.Sprintf("report write %s: %s", path, err.Error()))
	}
}

func (s *Subscriber) onDiffComputed(payload any) {
	result, ok := payload.(*types.DiffResult)
	if !ok || result.Plan == nil {
		return
	}
	s.writeJSON(".plan.json", result.Plan)
}

func (s *Subscriber) onRunFinished(payload any) {
	result, ok := payload.(*types.RunFinished)
	if !ok {
		return
	}
	s.writeJSON(".report.json", result)
}
