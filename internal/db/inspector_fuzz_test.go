package db

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"reporting-db-migrations/internal/fs"
)

func FuzzScopeKey_stringStableUnderRepeat(f *testing.F) {
	f.Add("s1", "o1", "t1", "c1")
	f.Fuzz(func(t *testing.T, s1, o1, t1, c1 string) {
		const max = 128
		if len(s1) > max || len(o1) > max || len(t1) > max || len(c1) > max {
			t.Skip()
		}
		layout := fs.Layout{
			Schemas:     []fs.Schema{{NormalizedName: s1}},
			Objects:     []*fs.Object{{NormalizedKey: o1}},
			Transitions: []*fs.TransitionScript{{NormalizedKey: t1}},
			Checks:      []*fs.CheckScript{{Path: c1}},
		}
		a := scopeKey(layout)
		b := scopeKey(layout)
		if a != b {
			t.Fatalf("non-deterministic scopeKey: %q vs %q", a, b)
		}
		da := scopeKeySHA256Hex(a)
		db := scopeKeySHA256Hex(b)
		if da != db {
			t.Fatalf("non-deterministic scopeKeySHA256Hex: %q vs %q", da, db)
		}
	})
}

// FuzzScopeKey_digestMatchesSHA256 checks scopeKeySHA256Hex against crypto/sha256
// for every non-empty canonical string produced from bounded fuzz inputs.
func FuzzScopeKey_digestMatchesSHA256(f *testing.F) {
	f.Add("s1", "o1", "t1", "c1")
	f.Fuzz(func(t *testing.T, s1, o1, t1, c1 string) {
		const max = 128
		if len(s1) > max || len(o1) > max || len(t1) > max || len(c1) > max {
			t.Skip()
		}
		layout := fs.Layout{
			Schemas:     []fs.Schema{{NormalizedName: s1}},
			Objects:     []*fs.Object{{NormalizedKey: o1}},
			Transitions: []*fs.TransitionScript{{NormalizedKey: t1}},
			Checks:      []*fs.CheckScript{{Path: c1}},
		}
		canon := scopeKey(layout)
		got := scopeKeySHA256Hex(canon)
		var want string
		if canon == "" {
			want = ""
		} else {
			sum := sha256.Sum256([]byte(canon))
			want = hex.EncodeToString(sum[:])
		}
		if got != want {
			t.Fatalf("scopeKeySHA256Hex(%q) = %q, want %q", canon, got, want)
		}
	})
}
