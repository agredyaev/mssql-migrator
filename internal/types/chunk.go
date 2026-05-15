package types

import (
	"strings"
)

func ChunkKeys(keys []string, size int) [][]string {
	if len(keys) == 0 {
		return nil
	}
	var chunks [][]string
	for i := 0; i < len(keys); i += size {
		end := i + size
		if end > len(keys) {
			end = len(keys)
		}
		chunks = append(chunks, keys[i:end])
	}
	return chunks
}

func BuildINQuery(template, placeholder string, keys []string) (string, []any) {
	parts := make([]string, len(keys))
	args := make([]any, len(keys))
	for i, k := range keys {
		parts[i] = "?"
		args[i] = k
	}
	query := strings.Replace(template, placeholder, strings.Join(parts, ", "), -1)
	return query, args
}
