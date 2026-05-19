package prodgate

import (
	"fmt"
	"sort"
)

// KeyChange describes a difference for one normalized object key.
type KeyChange struct {
	Key      string
	Field    string
	Baseline string
	Current  string
}

// CompareOptions controls incremental plan comparison.
type CompareOptions struct {
	// DeltaKeys limits which keys may differ between baseline and current.
	// When empty and StrictUnexpected is true, any difference is unexpected.
	DeltaKeys map[string]struct{}
	// StrictUnexpected: differences outside DeltaKeys fail the gate.
	StrictUnexpected bool
}

// CompareResult is the outcome of baseline vs current on the delta policy.
type CompareResult struct {
	Go                     bool
	Messages               []string
	DeltaChanges           []KeyChange
	UnexpectedOutsideDelta []KeyChange
	MissingInCurrent       []string
	MissingInBaseline      []string
}

// CompareSnapshots compares baseline and current plan snapshots under opts.
func CompareSnapshots(baseline, current PlanSnapshot, opts CompareOptions) CompareResult {
	res := CompareResult{Go: true}

	if current.Blocked && !baseline.Blocked {
		res.Go = false
		res.Messages = append(res.Messages, "plan became blocked")
	}

	allKeys := make(map[string]struct{})
	for k := range baseline.Objects {
		allKeys[k] = struct{}{}
	}
	for k := range current.Objects {
		allKeys[k] = struct{}{}
	}

	for key := range allKeys {
		bObj, bOK := baseline.Objects[key]
		cObj, cOK := current.Objects[key]
		if !bOK {
			res.MissingInBaseline = append(res.MissingInBaseline, key)
			changes := objectDiff(key, ObjectEntry{}, cObj)
			res.recordChange(key, changes, opts, &res)
			continue
		}
		if !cOK {
			res.MissingInCurrent = append(res.MissingInCurrent, key)
			changes := objectDiff(key, bObj, ObjectEntry{})
			res.recordChange(key, changes, opts, &res)
			continue
		}
		changes := objectDiff(key, bObj, cObj)
		if len(changes) == 0 {
			continue
		}
		res.recordChange(key, changes, opts, &res)
	}

	sort.Strings(res.MissingInBaseline)
	sort.Strings(res.MissingInCurrent)

	if len(res.UnexpectedOutsideDelta) > 0 || len(res.MissingInBaseline) > 0 || len(res.MissingInCurrent) > 0 {
		if opts.StrictUnexpected {
			res.Go = false
		}
	}
	if current.Blocked {
		res.Go = false
	}

	if !res.Go && len(res.Messages) == 0 {
		res.Messages = append(res.Messages, "incremental plan gate failed")
	}
	return res
}

func (res *CompareResult) recordChange(key string, changes []KeyChange, opts CompareOptions, out *CompareResult) {
	if len(changes) == 0 {
		return
	}
	_, inDelta := opts.DeltaKeys[key]
	if len(opts.DeltaKeys) == 0 || inDelta {
		out.DeltaChanges = append(out.DeltaChanges, changes...)
		for _, ch := range changes {
			if isRiskyAction(ch.Current) {
				out.Go = false
				out.Messages = append(out.Messages, fmt.Sprintf("risky action for %s: %s", key, ch.Current))
			}
		}
		return
	}
	if opts.StrictUnexpected {
		out.UnexpectedOutsideDelta = append(out.UnexpectedOutsideDelta, changes...)
		out.Go = false
		out.Messages = append(out.Messages, fmt.Sprintf("unexpected plan change outside delta: %s", key))
	}
}

func objectDiff(key string, b, c ObjectEntry) []KeyChange {
	var changes []KeyChange
	if b.PlannedAction != c.PlannedAction {
		changes = append(changes, KeyChange{Key: key, Field: "planned_action", Baseline: b.PlannedAction, Current: c.PlannedAction})
	}
	if b.ChecksumHex != c.ChecksumHex {
		changes = append(changes, KeyChange{Key: key, Field: "checksum_hex", Baseline: b.ChecksumHex, Current: c.ChecksumHex})
	}
	if b.Exists != c.Exists {
		changes = append(changes, KeyChange{Key: key, Field: "exists", Baseline: fmt.Sprintf("%v", b.Exists), Current: fmt.Sprintf("%v", c.Exists)})
	}
	if b.ObjectPath != c.ObjectPath && (b.ObjectPath != "" || c.ObjectPath != "") {
		changes = append(changes, KeyChange{Key: key, Field: "object_path", Baseline: b.ObjectPath, Current: c.ObjectPath})
	}
	return changes
}

func isRiskyAction(action string) bool {
	switch action {
	case "fail", "reprocess_changed_blocked":
		return true
	default:
		return false
	}
}
