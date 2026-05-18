package testutil

import (
	"context"
	"fmt"
	"sync/atomic"

	"reporting-db-migrations/internal/driver"
)

type MockConn struct {
	QueryCount   atomic.Int32
	ExecCount    atomic.Int32
	QueryErr     error
	ExecErr      error
	Queries      []MockQuery
	ExecQueries  []string
	RowsByPrefix map[string]*MockRows
	FailN        int
	execN        int
}

type MockQuery struct {
	Query       string
	Args        []any
	StringArgs1 []string
	StringArgs2 []string
}

type MockResult struct{}

func (m *MockResult) RowsAffected() (int64, error) { return 0, nil }

func (m *MockConn) QueryContext(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	m.QueryCount.Add(1)
	m.Queries = append(m.Queries, MockQuery{Query: query, Args: args})
	if m.QueryErr != nil {
		return nil, m.QueryErr
	}
	if m.RowsByPrefix != nil {
		for prefix, r := range m.RowsByPrefix {
			if len(query) >= len(prefix) && query[:len(prefix)] == prefix {
				r.Reset()
				return r, nil
			}
		}
	}
	return NewMockRows(nil), nil
}

func (m *MockConn) QueryStringsContext(ctx context.Context, query string, args []string) (driver.Rows, error) {
	m.QueryCount.Add(1)
	m.Queries = append(m.Queries, MockQuery{Query: query, StringArgs1: args})
	if m.QueryErr != nil {
		return nil, m.QueryErr
	}
	if m.RowsByPrefix != nil {
		for prefix, r := range m.RowsByPrefix {
			if len(query) >= len(prefix) && query[:len(prefix)] == prefix {
				r.Reset()
				return r, nil
			}
		}
	}
	return NewMockRows(nil), nil
}

func (m *MockConn) QueryStringSlicesContext(ctx context.Context, query string, args1 []string, args2 []string) (driver.Rows, error) {
	m.QueryCount.Add(1)
	m.Queries = append(m.Queries, MockQuery{Query: query, StringArgs1: args1, StringArgs2: args2})
	if m.QueryErr != nil {
		return nil, m.QueryErr
	}
	if m.RowsByPrefix != nil {
		for prefix, r := range m.RowsByPrefix {
			if len(query) >= len(prefix) && query[:len(prefix)] == prefix {
				r.Reset()
				return r, nil
			}
		}
	}
	return NewMockRows(nil), nil
}

func (m *MockConn) ExecContext(ctx context.Context, query string, args ...any) (driver.Result, error) {
	m.execN++
	m.ExecCount.Add(1)
	m.Queries = append(m.Queries, MockQuery{Query: query, Args: args})
	m.ExecQueries = append(m.ExecQueries, query)
	if m.FailN > 0 && m.execN <= m.FailN {
		return nil, fmt.Errorf("injected error after %d calls", m.execN)
	}
	if m.ExecErr != nil {
		return nil, m.ExecErr
	}
	return &MockResult{}, nil
}

func (m *MockConn) Ping(ctx context.Context) error { return nil }
func (m *MockConn) Close() error                   { return nil }
