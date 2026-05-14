package report

import (
	"encoding/json"
	"os"
	"path/filepath"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/types"
)

type Subscriber struct {
	bus     bus.EventBus
	baseDir string
	stdout  bool
}

func NewSubscriber(b bus.EventBus, baseDir string, stdout bool) *Subscriber {
	s := &Subscriber{bus: b, baseDir: baseDir, stdout: stdout}
	b.Subscribe(types.EventDiffComputed, s.onDiffComputed)
	b.Subscribe(types.EventRunFinished, s.onRunFinished)
	return s
}

func (s *Subscriber) onDiffComputed(payload any) {
	result, ok := payload.(types.DiffResult)
	if !ok || result.Plan == nil {
		return
	}

	data, err := json.MarshalIndent(result.Plan, "", "  ")
	if err != nil {
		return
	}

	path := filepath.Join(s.baseDir, ".plan.json")
	os.WriteFile(path, data, 0644)
}

func (s *Subscriber) onRunFinished(payload any) {
	result, ok := payload.(types.RunFinished)
	if !ok {
		return
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return
	}

	path := filepath.Join(s.baseDir, ".report.json")
	os.WriteFile(path, data, 0644)
}
