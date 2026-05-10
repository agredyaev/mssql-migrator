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
	state := goParseState{}
	for _, line := range lines {
		matches := goLinePattern.FindStringSubmatch(line)
		if matches != nil && !state.inBlockComment() && !state.inStringLiteral {
			sql := strings.TrimSpace(strings.Join(current, "\n"))
			if sql != "" {
				repeat, err := parseRepeat(matches[1], line)
				if err != nil {
					return nil, err
				}
				batches = append(batches, Batch{SQL: sql, Repeat: repeat})
			}
			current = make([]string, 0)
			continue
		}

		state = advanceGOParseState(line, state)
		if matches == nil {
			current = append(current, line)
			continue
		}
		current = append(current, line)
	}
	if state.inStringLiteral {
		return nil, fmt.Errorf("unterminated SQL string literal")
	}
	if state.inBlockComment() {
		return nil, fmt.Errorf("unterminated SQL block comment")
	}
	last := strings.TrimSpace(strings.Join(current, "\n"))
	if last != "" {
		batches = append(batches, Batch{SQL: last, Repeat: 1})
	}
	return batches, nil
}

type goParseState struct {
	blockCommentDepth int
	inStringLiteral   bool
}

func (s goParseState) inBlockComment() bool {
	return s.blockCommentDepth > 0
}

func advanceGOParseState(line string, state goParseState) goParseState {
	for i := 0; i < len(line); i++ {
		if state.inBlockComment() {
			if i+1 < len(line) && line[i] == '/' && line[i+1] == '*' {
				state.blockCommentDepth++
				i++
				continue
			}
			if i+1 < len(line) && line[i] == '*' && line[i+1] == '/' {
				state.blockCommentDepth--
				i++
			}
			continue
		}
		if state.inStringLiteral {
			if line[i] != '\'' {
				continue
			}
			if i+1 < len(line) && line[i+1] == '\'' {
				i++
				continue
			}
			state.inStringLiteral = false
			continue
		}
		if i+1 < len(line) && line[i] == '-' && line[i+1] == '-' {
			break
		}
		if i+1 < len(line) && line[i] == '/' && line[i+1] == '*' {
			state.blockCommentDepth++
			i++
			continue
		}
		if line[i] == '\'' {
			state.inStringLiteral = true
		}
	}
	return state
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
