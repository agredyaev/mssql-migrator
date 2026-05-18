package bus

import "reporting-db-migrations/internal/types"

// ParseObjectAppliedPayload extracts object-applied bus payloads published as
// either a single *ObjectEvent or a batch []*ObjectEvent. ok is false if the
// dynamic type does not match either shape.
func ParseObjectAppliedPayload(payload any) (events []*types.ObjectEvent, ok bool) {
	switch ev := payload.(type) {
	case *types.ObjectEvent:
		return []*types.ObjectEvent{ev}, true
	case []*types.ObjectEvent:
		return ev, true
	default:
		return nil, false
	}
}

// ParseObjectFailedPayload extracts object-failed bus payloads published as
// either a single *FailureEvent or a batch []*FailureEvent. ok is false if
// the dynamic type does not match either shape.
func ParseObjectFailedPayload(payload any) (failures []*types.FailureEvent, ok bool) {
	switch ev := payload.(type) {
	case *types.FailureEvent:
		return []*types.FailureEvent{ev}, true
	case []*types.FailureEvent:
		return ev, true
	default:
		return nil, false
	}
}

// ParseDiffResult returns the payload as *DiffResult, or ok false.
func ParseDiffResult(payload any) (result *types.DiffResult, ok bool) {
	result, ok = payload.(*types.DiffResult)
	return result, ok
}

// ParseRunFinished returns the payload as *RunFinished, or ok false.
func ParseRunFinished(payload any) (finished *types.RunFinished, ok bool) {
	finished, ok = payload.(*types.RunFinished)
	return finished, ok
}

// ParseValidationResult returns the payload as *ValidationResult, or ok false.
func ParseValidationResult(payload any) (result *types.ValidationResult, ok bool) {
	result, ok = payload.(*types.ValidationResult)
	return result, ok
}
