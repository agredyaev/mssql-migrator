package audit

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("RMIG_CATALOG_CACHE", "0")
	os.Exit(m.Run())
}
