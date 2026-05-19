package prodgate

import (
	"encoding/hex"
	"encoding/json"
	"os"

	"reporting-db-migrations/internal/types"
)

const SnapshotVersion = "1"

// PlanSnapshot is a compact, comparable view of a MigrationPlan for incremental gates.
type PlanSnapshot struct {
	Version    string                 `json:"version"`
	Blocked    bool                   `json:"blocked"`
	LayoutHash string                 `json:"layout_hash,omitempty"`
	Objects    map[string]ObjectEntry `json:"objects"`
}

// ObjectEntry holds business-meaningful fields per normalized object key.
type ObjectEntry struct {
	ObjectPath    string `json:"object_path"`
	PlannedAction string `json:"planned_action"`
	ChecksumHex   string `json:"checksum_hex"`
	Exists        bool   `json:"exists"`
}

// SnapshotFromPlan builds a PlanSnapshot from a MigrationPlan.
func SnapshotFromPlan(plan *types.MigrationPlan) PlanSnapshot {
	if plan == nil {
		return PlanSnapshot{Version: SnapshotVersion, Objects: map[string]ObjectEntry{}}
	}
	objects := make(map[string]ObjectEntry, len(plan.Objects))
	for _, obj := range plan.Objects {
		key := obj.NormalizedKey
		if key == "" {
			key = types.NormalizedKey(obj.SchemaName, obj.Kind, obj.ObjectName)
		}
		objects[key] = ObjectEntry{
			ObjectPath:    obj.ObjectPath,
			PlannedAction: obj.PlannedAction,
			ChecksumHex:   hex.EncodeToString(obj.Checksum[:]),
			Exists:        obj.Exists,
		}
	}
	return PlanSnapshot{
		Version:    SnapshotVersion,
		Blocked:    plan.Blocked,
		LayoutHash: plan.LayoutHash,
		Objects:    objects,
	}
}

// WriteJSONFile writes snapshot to path (mode 0644).
func WriteJSONFile(path string, snap PlanSnapshot) error {
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ReadJSONFile loads a PlanSnapshot from path.
func ReadJSONFile(path string) (PlanSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PlanSnapshot{}, err
	}
	var snap PlanSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return PlanSnapshot{}, err
	}
	if snap.Objects == nil {
		snap.Objects = map[string]ObjectEntry{}
	}
	return snap, nil
}
