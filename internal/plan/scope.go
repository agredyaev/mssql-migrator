package plan

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/prodgate"
	"reporting-db-migrations/internal/types"
)

// BuildInspectScope classifies layout objects for scoped SQL catalog inspection.
// checksums must be loaded for all layout keys (from audit) before calling.
func BuildInspectScope(
	layout fs.Layout,
	changedPaths []string,
	fullInspect bool,
	checksums map[string][32]byte,
) db.InspectScope {
	if fullInspect {
		return db.InspectScope{FullInspect: true}
	}

	delta := prodgate.KeysForChangedPaths(layout, changedPaths)
	delta = prodgate.ExpandDeltaClosure(layout, delta)

	hotKeys := make(map[string]struct{})
	stable := make(map[string]db.Object)

	for _, obj := range layout.Objects {
		if obj == nil {
			continue
		}
		key := obj.NormalizedKey
		if _, inDelta := delta[key]; inDelta {
			hotKeys[key] = struct{}{}
			continue
		}
		fileCS, err := obj.Checksum()
		if err != nil {
			hotKeys[key] = struct{}{}
			continue
		}
		prior, ok := checksums[key]
		if !ok || prior == ([32]byte{}) || fileCS != prior {
			hotKeys[key] = struct{}{}
			continue
		}
		stable[key] = db.Object{
			SchemaName: obj.SchemaName,
			Kind:       obj.Kind,
			ObjectName: obj.ObjectName,
			ParentName: obj.ParentName,
		}
	}

	promoteSpotCheckKeys(hotKeys, stable, layout)

	if len(hotKeys) == len(layout.Objects) {
		return db.InspectScope{FullInspect: true}
	}

	refs := objectRefsForKeys(layout, hotKeys)
	return db.InspectScope{
		FullInspect:   false,
		HotRefs:       refs,
		StableObjects: stable,
	}
}

func objectRefsForKeys(layout fs.Layout, keys map[string]struct{}) []types.ObjectScopeRef {
	if len(keys) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(keys))
	out := make([]types.ObjectScopeRef, 0, len(keys))
	for _, obj := range layout.Objects {
		if obj == nil {
			continue
		}
		if _, ok := keys[obj.NormalizedKey]; !ok {
			continue
		}
		if _, dup := seen[obj.NormalizedKey]; dup {
			continue
		}
		seen[obj.NormalizedKey] = struct{}{}
		out = append(out, types.ObjectScopeRef{
			Schema: obj.SchemaName,
			Kind:   obj.Kind,
			Object: obj.ObjectName,
		})
	}
	return out
}

// promoteSpotCheckKeys moves up to n stable keys into hot for SQL EXISTS verification (phase 3).
func promoteSpotCheckKeys(hotKeys map[string]struct{}, stable map[string]db.Object, layout fs.Layout) {
	n := db.SpotCheckCountFromEnv()
	if n <= 0 || len(stable) == 0 {
		return
	}
	keys := make([]string, 0, len(stable))
	for k := range stable {
		keys = append(keys, k)
	}
	digest := db.LayoutDigest(layout)
	sort.Slice(keys, func(i, j int) bool {
		return spotCheckRank(keys[i], digest) < spotCheckRank(keys[j], digest)
	})
	if n > len(keys) {
		n = len(keys)
	}
	for i := 0; i < n; i++ {
		k := keys[i]
		delete(stable, k)
		hotKeys[k] = struct{}{}
	}
}

func spotCheckRank(key, layoutDigest string) uint64 {
	sum := sha256.Sum256([]byte(layoutDigest + "\x00" + key))
	return binary.LittleEndian.Uint64(sum[:8])
}
