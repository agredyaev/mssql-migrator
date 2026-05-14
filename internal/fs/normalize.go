package fs

type sqlNormalizer struct {
	sawCR             bool
	pendingWhitespace []byte
}

func normalizeSQL(input string) string {
	n := sqlNormalizer{}
	output := make([]byte, 0, len(input))
	for i := 0; i < len(input); i++ {
		n.processByte(input[i], &output)
	}
	n.finish(&output)
	return string(output)
}

func (n *sqlNormalizer) processByte(b byte, output *[]byte) {
	if n.sawCR {
		if b == '\n' {
			n.emitNewline(output)
			n.sawCR = false
			return
		}
		n.emitNewline(output)
		n.sawCR = false
	}

	switch b {
	case '\r':
		n.sawCR = true
	case '\n':
		n.emitNewline(output)
	case ' ', '\t':
		n.pendingWhitespace = append(n.pendingWhitespace, b)
	default:
		*output = append(*output, n.pendingWhitespace...)
		n.pendingWhitespace = n.pendingWhitespace[:0]
		*output = append(*output, b)
	}
}

func (n *sqlNormalizer) finish(output *[]byte) {
	if n.sawCR {
		n.emitNewline(output)
		n.sawCR = false
	}
	n.pendingWhitespace = n.pendingWhitespace[:0]
}

func (n *sqlNormalizer) emitNewline(output *[]byte) {
	n.pendingWhitespace = n.pendingWhitespace[:0]
	*output = append(*output, '\n')
}
