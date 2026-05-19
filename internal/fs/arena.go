package fs

import (
	"unsafe"
)

// stringArena deduplicates strings into one backing buffer (phase 4 DOD).
type stringArena struct {
	buf    []byte
	index  map[string]uint32
	unique int
}

func newStringArena(capHint int) *stringArena {
	return &stringArena{
		buf:   make([]byte, 0, capHint),
		index: make(map[string]uint32, capHint/4),
	}
}

func (a *stringArena) Intern(s string) string {
	if s == "" {
		return ""
	}
	if off, ok := a.index[s]; ok {
		return a.at(off, len(s))
	}
	off := uint32(len(a.buf))
	a.buf = append(a.buf, s...)
	a.index[s] = off
	a.unique++
	return a.at(off, len(s))
}

func (a *stringArena) at(off uint32, n int) string {
	if n == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(a.buf[off:]), n)
}

// Len returns unique string count interned.
func (a *stringArena) Len() int { return a.unique }

// Bytes returns arena backing size.
func (a *stringArena) Bytes() int { return len(a.buf) }

// internLayoutStrings replaces duplicate metadata strings on layout objects in place.
func internLayoutStrings(layout *Layout) {
	if len(layout.Objects) == 0 {
		return
	}
	est := len(layout.Objects) * 64
	arena := newStringArena(est)
	for _, obj := range layout.Objects {
		if obj == nil {
			continue
		}
		obj.Path = arena.Intern(obj.Path)
		obj.DatabaseName = arena.Intern(obj.DatabaseName)
		obj.SchemaName = arena.Intern(obj.SchemaName)
		obj.NormalizedSchemaName = arena.Intern(obj.NormalizedSchemaName)
		obj.Kind = arena.Intern(obj.Kind)
		obj.ObjectName = arena.Intern(obj.ObjectName)
		obj.ParentName = arena.Intern(obj.ParentName)
		obj.ParentNormalizedKey = arena.Intern(obj.ParentNormalizedKey)
		obj.NormalizedKey = arena.Intern(obj.NormalizedKey)
	}
	for _, ts := range layout.Transitions {
		if ts == nil {
			continue
		}
		ts.Path = arena.Intern(ts.Path)
		ts.DatabaseName = arena.Intern(ts.DatabaseName)
		ts.SchemaName = arena.Intern(ts.SchemaName)
		ts.TableName = arena.Intern(ts.TableName)
		ts.NormalizedKey = arena.Intern(ts.NormalizedKey)
		ts.Ordinal = arena.Intern(ts.Ordinal)
		ts.Commit = arena.Intern(ts.Commit)
		ts.Slug = arena.Intern(ts.Slug)
	}
	for _, ch := range layout.Checks {
		if ch == nil {
			continue
		}
		ch.Path = arena.Intern(ch.Path)
		ch.DatabaseName = arena.Intern(ch.DatabaseName)
		ch.SchemaName = arena.Intern(ch.SchemaName)
		ch.Name = arena.Intern(ch.Name)
	}
	for i := range layout.Schemas {
		layout.Schemas[i].DatabaseName = arena.Intern(layout.Schemas[i].DatabaseName)
		layout.Schemas[i].Name = arena.Intern(layout.Schemas[i].Name)
		layout.Schemas[i].NormalizedName = arena.Intern(layout.Schemas[i].NormalizedName)
	}
	layout.stringArena = arena
}
