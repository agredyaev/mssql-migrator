package testutil

import "errors"

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
	for i, v := range m.Values[m.pos] {
		switch d := dest[i].(type) {
		case *string:
			s, ok := v.(string)
			if !ok {
				return errors.New("mock scan: expected string value")
			}
			*d = s
		case *int:
			n, ok := v.(int)
			if !ok {
				return errors.New("mock scan: expected int value")
			}
			*d = n
		case *bool:
			b, ok := v.(bool)
			if !ok {
				return errors.New("mock scan: expected bool value")
			}
			*d = b
		default:
			return errors.New("mock scan: unsupported destination type")
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
