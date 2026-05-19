package fs

import "unsafe"

// ObjectStore is a dense SoA index over layout objects (phase 4 DOD).
// Metadata strings live in layout.stringArena; file state in Object.file pointers.
type ObjectStore struct {
	rows      []objectRow
	keyIndex  map[string]uint32 // normalized_key -> row id (1-based)
	objects   []*Object         // same pointers as Layout.Objects
	fileCount int
}

type objectRow struct {
	schemaID uint16
	kindCode uint8
	flags    uint8
	fileIdx  uint32
}

// ObjectRowByteSize is unsafe.Sizeof(objectRow) for footprint reporting.
func ObjectRowByteSize() int {
	return int(unsafe.Sizeof(objectRow{}))
}

const (
	objFlagNoTransaction uint8 = 1 << iota
	objFlagStatValid
)

var kindCodes = map[string]uint8{
	"types": 1, "sequences": 2, "tables": 3, "synonyms": 4, "indexes": 5,
	"views": 6, "functions": 7, "procedures": 8, "triggers": 9,
}

// Len returns object count in the store.
func (s *ObjectStore) Len() int {
	if s == nil {
		return 0
	}
	return len(s.rows)
}

// Object returns the i-th layout object (0-based).
func (s *ObjectStore) Object(i int) *Object {
	if s == nil || i < 0 || i >= len(s.objects) {
		return nil
	}
	return s.objects[i]
}

// KeyIndex returns row id for normalized key or 0 if missing.
func (s *ObjectStore) KeyIndex(normalizedKey string) uint32 {
	if s == nil {
		return 0
	}
	return s.keyIndex[normalizedKey]
}

func buildObjectStore(layout *Layout) *ObjectStore {
	n := len(layout.Objects)
	if n == 0 {
		return nil
	}
	store := &ObjectStore{
		rows:     make([]objectRow, n),
		keyIndex: make(map[string]uint32, n),
		objects:  layout.Objects,
	}
	for i, obj := range layout.Objects {
		if obj == nil {
			continue
		}
		var flags uint8
		if obj.NoTransaction {
			flags |= objFlagNoTransaction
		}
		if obj.objectStatForByteCacheValid {
			flags |= objFlagStatValid
		}
		kind := kindCodes[obj.Kind]
		store.rows[i] = objectRow{
			schemaID: 0, // reserved for future schema table
			kindCode: kind,
			flags:    flags,
			fileIdx:  uint32(i),
		}
		store.keyIndex[obj.NormalizedKey] = uint32(i + 1)
		if obj.File != nil {
			store.fileCount++
		}
	}
	return store
}
