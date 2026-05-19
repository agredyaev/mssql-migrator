package types

import (
	"strconv"
	"sync"
)

// ObjectScopeRef identifies one layout object for OpenJSON-scoped catalog queries.
type ObjectScopeRef struct {
	Schema string
	Kind   string
	Object string
}

var objectScopeJSONPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 512)
		return &b
	},
}

// MarshalObjectScopeJSON builds a JSON array of {"schema","kind","object"} objects.
func MarshalObjectScopeJSON(refs []ObjectScopeRef) string {
	bufPtr := objectScopeJSONPool.Get().(*[]byte)
	b := (*bufPtr)[:0]

	need := 2
	for _, r := range refs {
		need += len(r.Schema) + len(r.Kind) + len(r.Object) + 48
	}
	if cap(b) < need {
		b = make([]byte, 0, need)
	}

	b = append(b, '[')
	for i, r := range refs {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '{')
		b = appendJSONField(b, "schema", r.Schema, false)
		b = appendJSONField(b, "kind", r.Kind, true)
		b = appendJSONField(b, "object", r.Object, true)
		b = append(b, '}')
	}
	b = append(b, ']')

	out := string(b)
	*bufPtr = b[:0]
	objectScopeJSONPool.Put(bufPtr)
	return out
}

func appendJSONField(dst []byte, key, value string, withComma bool) []byte {
	if withComma {
		dst = append(dst, ',')
	}
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, '"', ':')
	dst = strconv.AppendQuote(dst, value)
	return dst
}
