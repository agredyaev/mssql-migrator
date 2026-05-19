package prodgate

import (
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

// ExpandDeltaClosure adds dependency keys (parent tables, transitions) to a path-derived delta set.
func ExpandDeltaClosure(layout fs.Layout, deltaKeys map[string]struct{}) map[string]struct{} {
	if len(deltaKeys) == 0 {
		return deltaKeys
	}
	out := make(map[string]struct{}, len(deltaKeys)+8)
	for k := range deltaKeys {
		out[k] = struct{}{}
	}
	transByKey := transitionsByNormalizedKey(layout)
	for {
		added := 0
		for _, obj := range layout.Objects {
			if obj == nil {
				continue
			}
			if _, hot := out[obj.NormalizedKey]; !hot {
				continue
			}
			if obj.Kind == "triggers" && obj.ParentNormalizedKey != "" {
				if _, ok := out[obj.ParentNormalizedKey]; !ok {
					out[obj.ParentNormalizedKey] = struct{}{}
					added++
				}
			}
		}
		for _, ts := range layout.Transitions {
			if ts == nil {
				continue
			}
			if _, hot := out[ts.NormalizedKey]; !hot {
				continue
			}
			tableKey := types.NormalizedKey(ts.SchemaName, "tables", ts.TableName)
			if _, ok := out[tableKey]; !ok {
				out[tableKey] = struct{}{}
				added++
			}
		}
		for key := range out {
			for _, ts := range transByKey[key] {
				if _, ok := out[ts.NormalizedKey]; !ok {
					out[ts.NormalizedKey] = struct{}{}
					added++
				}
			}
		}
		if added == 0 {
			break
		}
	}
	return out
}

func transitionsByNormalizedKey(layout fs.Layout) map[string][]*fs.TransitionScript {
	m := make(map[string][]*fs.TransitionScript)
	for _, ts := range layout.Transitions {
		if ts == nil {
			continue
		}
		tableKey := types.NormalizedKey(ts.SchemaName, "tables", ts.TableName)
		m[tableKey] = append(m[tableKey], ts)
	}
	return m
}
