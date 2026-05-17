package testutil

import (
	"errors"
	"fmt"
)

type MockRows struct {
	Values [][]any
	pos    int
	Closed bool
}

func NewMockRows(vals [][]any) *MockRows {
	return &MockRows{Values: vals, pos: -1}
}

func (m *MockRows) Scan(dest ...any) error {
	if m.pos < 0 || m.pos >= len(m.Values) {
		return errors.New("no rows")
	}
	row := m.Values[m.pos]
	if len(dest) != len(row) {
		return fmt.Errorf("mockrows: Scan: got %d dest, row has %d values", len(dest), len(row))
	}
	for i, v := range row {
		switch d := dest[i].(type) {
		case *string:
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("mockrows: col %d: want string, got %T", i, v)
			}
			*d = s
		case *int:
			n, ok := v.(int)
			if !ok {
				return fmt.Errorf("mockrows: col %d: want int, got %T", i, v)
			}
			*d = n
		case *bool:
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("mockrows: col %d: want bool, got %T", i, v)
			}
			*d = b
		default:
			return fmt.Errorf("mockrows: col %d: unsupported dest type %T", i, dest[i])
		}
	}
	return nil
}

func (m *MockRows) Next() bool {
	m.pos++
	return m.pos < len(m.Values)
}

func (m *MockRows) Err() error { return nil }

func (m *MockRows) Reset() {
	m.pos = -1
}

func (m *MockRows) Close() error {
	if m.Closed {
		return errors.New("already closed")
	}
	m.Closed = true
	return nil
}
