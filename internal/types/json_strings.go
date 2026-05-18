package types

import (
	"strconv"
	"sync"
)

var jsonStringSlicePool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 256)
		return &b
	},
}

// MarshalStringSliceJSON builds a JSON string array without reflection-heavy
// encoding/json machinery. Strings are still escaped correctly via
// strconv.AppendQuote.
func MarshalStringSliceJSON(values []string) string {
	bufPtr := jsonStringSlicePool.Get().(*[]byte)
	b := (*bufPtr)[:0]

	need := 2
	for _, v := range values {
		need += len(v) + 3
	}
	if cap(b) < need {
		b = make([]byte, 0, need)
	}

	b = append(b, '[')
	for i, v := range values {
		if i > 0 {
			b = append(b, ',')
		}
		b = strconv.AppendQuote(b, v)
	}
	b = append(b, ']')

	out := string(b)
	*bufPtr = b[:0]
	jsonStringSlicePool.Put(bufPtr)
	return out
}
