package catalog

import (
	"context"
	"errors"
	"testing"

	"reporting-db-migrations/internal/contracts"
)

func TestReadRejectsNilConnection(t *testing.T) {
	_, err := Read(context.Background(), nil)
	if err == nil {
		t.Fatal("expected nil connection error")
	}
	if !errors.Is(err, contracts.ErrCriticalState) {
		t.Fatalf("expected critical state sentinel, got %v", err)
	}
	if err.Error() == contracts.ErrCriticalState.Error() {
		t.Fatalf("expected descriptive wrapped error, got %v", err)
	}
}
