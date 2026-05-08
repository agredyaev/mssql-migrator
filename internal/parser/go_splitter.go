package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var goLinePattern = regexp.MustCompile(`(?i)^\s*GO(?:\s+([0-9]+))?\s*$`)

func SplitGO(content string) ([]Batch, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	batches := make([]Batch, 0)
	current := make([]string, 0)
	for _, line := range lines {
		matches := goLinePattern.FindStringSubmatch(line)
		if matches == nil {
			current = append(current, line)
			continue
		}
		sql := strings.TrimSpace(strings.Join(current, "\n"))
		if sql != "" {
			repeat, err := parseRepeat(matches[1], line)
			if err != nil {
				return nil, err
			}
			batches = append(batches, Batch{SQL: sql, Repeat: repeat})
		}
		current = make([]string, 0)
	}
	last := strings.TrimSpace(strings.Join(current, "\n"))
	if last != "" {
		batches = append(batches, Batch{SQL: last, Repeat: 1})
	}
	return batches, nil
}

func parseRepeat(value string, line string) (int, error) {
	if value == "" {
		return 1, nil
	}
	repeat, err := strconv.Atoi(value)
	if err != nil || repeat < 1 {
		return 0, fmt.Errorf("invalid GO repeat: %s", line)
	}
	return repeat, nil
}
