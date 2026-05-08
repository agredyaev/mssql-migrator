package state

import "time"

type Attempt struct {
	ScriptName                                                                string
	ScriptType                                                                string
	Checksum                                                                  string
	AppliedAt                                                                 time.Time
	ExecutionMS                                                               int
	Success                                                                   bool
	ErrorMessage, GitCommit, GitBranch, PipelineRunID, PipelineURL, AppliedBy string
}

type State struct {
	SuccessByScript map[string]Attempt
	Failures        []Attempt
}

func New(attempts []Attempt) State {
	s := State{SuccessByScript: map[string]Attempt{}, Failures: []Attempt{}}
	for _, a := range attempts {
		if a.Success {
			s.SuccessByScript[a.ScriptName] = a
		} else {
			s.Failures = append(s.Failures, a)
		}
	}
	return s
}
