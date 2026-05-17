package fs

import "strings"

func normalizeSQL(input string) string {
	var b strings.Builder
	b.Grow(len(input))
	sawCR := false
	wsBuf := make([]byte, 0, 16)

	for i := 0; i < len(input); i++ {
		ch := input[i]

		if sawCR {
			sawCR = false
			wsBuf = wsBuf[:0]
			if ch == '\n' {
				b.WriteByte('\n')
				continue
			}
			b.WriteByte('\n')
		}

		switch ch {
		case '\r':
			sawCR = true
		case '\n':
			b.WriteByte('\n')
			wsBuf = wsBuf[:0]
		case ' ', '\t':
			wsBuf = append(wsBuf, ch)
		default:
			b.Write(wsBuf)
			wsBuf = wsBuf[:0]
			start := i
			for i < len(input) && input[i] > ' ' {
				i++
			}
			b.WriteString(input[start:i])
			i--
		}
	}

	if sawCR {
		b.WriteByte('\n')
	}

	return b.String()
}
