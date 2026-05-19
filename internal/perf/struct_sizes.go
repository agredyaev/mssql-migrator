package perf

import (
	"sort"
	"unsafe"

	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

// StructSizeEntry is one type at or above FootprintThreshold.
type StructSizeEntry struct {
	Package string `json:"package"`
	Type    string `json:"type"`
	Bytes   int    `json:"bytes"`
}

// CollectStructSizes returns struct sizes >= FootprintThreshold, sorted by bytes desc.
func CollectStructSizes() []StructSizeEntry {
	raw := []StructSizeEntry{
		{"fs", "Object", int(unsafe.Sizeof(fs.Object{}))},
		{"fs", "ObjectStore", int(unsafe.Sizeof(fs.ObjectStore{}))},
		{"fs", "objectRow", fs.ObjectRowByteSize()},
		{"fs", "TransitionScript", int(unsafe.Sizeof(fs.TransitionScript{}))},
		{"fs", "CheckScript", int(unsafe.Sizeof(fs.CheckScript{}))},
		{"fs", "CachedFile", int(unsafe.Sizeof(fs.CachedFile{}))},
		{"fs", "Layout", int(unsafe.Sizeof(fs.Layout{}))},
		{"fs", "Schema", int(unsafe.Sizeof(fs.Schema{}))},
		{"types", "PlannedObject", int(unsafe.Sizeof(types.PlannedObject{}))},
		{"types", "MigrationPlan", int(unsafe.Sizeof(types.MigrationPlan{}))},
		{"types", "ObjectRef", int(unsafe.Sizeof(types.ObjectRef{}))},
		{"types", "PlannedSchema", int(unsafe.Sizeof(types.PlannedSchema{}))},
		{"types", "PlanSummary", int(unsafe.Sizeof(types.PlanSummary{}))},
		{"types", "GitInfo", int(unsafe.Sizeof(types.GitInfo{}))},
		{"db", "Object", int(unsafe.Sizeof(db.Object{}))},
		{"db", "TableColumn", int(unsafe.Sizeof(db.TableColumn{}))},
		{"db", "State", int(unsafe.Sizeof(db.State{}))},
	}
	out := make([]StructSizeEntry, 0, len(raw))
	for _, e := range raw {
		if e.Bytes >= FootprintThreshold {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes > out[j].Bytes
		}
		if out[i].Package != out[j].Package {
			return out[i].Package < out[j].Package
		}
		return out[i].Type < out[j].Type
	})
	return out
}
