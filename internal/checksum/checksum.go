package checksum

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const normalizeSQLBufferSize = 32 * 1024

// sqlNormalizer preserves NormalizeSQL semantics while allowing file hashing to stream.
type sqlNormalizer struct {
	sawCR             bool
	pendingWhitespace []byte
}

func NormalizeSQL(input string) string {
	normalizer := sqlNormalizer{}
	output := make([]byte, 0, len(input))
	for i := 0; i < len(input); i++ {
		normalizer.processByte(input[i], &output)
	}
	normalizer.finish(&output)
	return string(output)
}

func SHA256String(input string) string {
	sum := sha256.New()
	hashNormalizedString(input, sum)
	return hex.EncodeToString(sum.Sum(nil))
}

func SHA256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	sum := sha256.New()
	if err := hashNormalizedReader(file, sum); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func SQLDirHash(root string) (string, error) {
	entries := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".sql" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		h, err := SHA256File(path)
		if err != nil {
			return err
		}
		entries = append(entries, filepath.ToSlash(rel)+":"+h)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:]), nil
}

func hashNormalizedString(input string, sum hash.Hash) {
	normalizer := sqlNormalizer{}
	output := make([]byte, 0, normalizeSQLBufferSize)
	for i := 0; i < len(input); i++ {
		normalizer.processByte(input[i], &output)
		flushHashBuffer(sum, &output, false)
	}
	normalizer.finish(&output)
	flushHashBuffer(sum, &output, true)
}

func hashNormalizedReader(reader io.Reader, sum hash.Hash) error {
	normalizer := sqlNormalizer{}
	input := make([]byte, normalizeSQLBufferSize)
	output := make([]byte, 0, normalizeSQLBufferSize)
	for {
		count, err := reader.Read(input)
		if count > 0 {
			for _, b := range input[:count] {
				normalizer.processByte(b, &output)
				flushHashBuffer(sum, &output, false)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	normalizer.finish(&output)
	flushHashBuffer(sum, &output, true)
	return nil
}

func flushHashBuffer(sum hash.Hash, output *[]byte, force bool) {
	if len(*output) == 0 {
		return
	}
	if !force && len(*output) < normalizeSQLBufferSize {
		return
	}
	_, _ = sum.Write(*output)
	*output = (*output)[:0]
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
