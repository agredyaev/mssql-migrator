package db

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Unit tests count SQL round-trips; persistent catalog cache adds extra queries.
	_ = os.Setenv("RMIG_CATALOG_CACHE", "0")
	os.Exit(m.Run())
}
