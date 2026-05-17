package fs

import "sync"

var normalizePool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, 4096)
		return &b
	},
}

func normalizeSQLBytes(input string, b []byte) []byte {
	sawCR := false
	var wsBufArr [256]byte
	wsBuf := wsBufArr[:0]

	for i := 0; i < len(input); i++ {
		ch := input[i]

		if sawCR {
			sawCR = false
			wsBuf = wsBuf[:0]
			if ch == '\n' {
				b = append(b, '\n')
				continue
			}
			b = append(b, '\n')
		}

		switch ch {
		case '\r':
			sawCR = true
		case '\n':
			b = append(b, '\n')
			wsBuf = wsBuf[:0]
		case ' ', '\t':
			wsBuf = append(wsBuf, ch)
		default:
			b = append(b, wsBuf...)
			wsBuf = wsBuf[:0]
			start := i
			for i < len(input) && input[i] > ' ' {
				i++
			}
			b = append(b, input[start:i]...)
			i--
		}
	}

	if sawCR {
		b = append(b, '\n')
	}

	return b
}
