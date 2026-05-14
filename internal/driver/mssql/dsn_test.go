package mssql

import (
	"net/url"
	"strings"
	"testing"

	"reporting-db-migrations/internal/types"
)

func parseDSN(dsn string) url.Values {
	u, err := url.Parse(dsn)
	if err != nil {
		return url.Values{}
	}
	return u.Query()
}

func TestBuildDSN_Defaults(t *testing.T) {
	cfg := types.Config{
		Server:   "localhost",
		Port:     "1433",
		Database: "test",
		User:     "sa",
		Password: "pwd",
		Encrypt:  false,
	}
	dsn := BuildDSN(cfg)
	if !strings.HasPrefix(dsn, "sqlserver://sa:pwd@localhost:1433") {
		t.Errorf("unexpected DSN prefix: %s", dsn)
	}
	q := parseDSN(dsn)
	if q.Get("database") != "test" {
		t.Errorf("database = %q", q.Get("database"))
	}
	if q.Get("encrypt") != "false" {
		t.Errorf("encrypt = %q", q.Get("encrypt"))
	}
	if q.Get("app name") != "rmig" {
		t.Errorf("app name = %q", q.Get("app name"))
	}
}

func TestBuildDSN_IntegratedAuth(t *testing.T) {
	cfg := types.Config{
		Server:                 "localhost",
		Port:                   "1433",
		Database:               "test",
		DBAuth:                 "integrated",
		Encrypt:                true,
		TrustServerCertificate: true,
	}
	dsn := BuildDSN(cfg)
	q := parseDSN(dsn)
	if q.Get("authenticator") != "winsspi" {
		t.Errorf("authenticator = %q", q.Get("authenticator"))
	}
	if q.Get("encrypt") != "true" {
		t.Errorf("encrypt = %q", q.Get("encrypt"))
	}
	if q.Get("TrustServerCertificate") != "true" {
		t.Errorf("TrustServerCertificate = %q", q.Get("TrustServerCertificate"))
	}
}

func TestBuildDSN_NoPassword(t *testing.T) {
	cfg := types.Config{
		Server:   "localhost",
		Port:     "1433",
		Database: "test",
	}
	dsn := BuildDSN(cfg)
	if !strings.HasPrefix(dsn, "sqlserver://localhost:1433") {
		t.Errorf("unexpected DSN: %s", dsn)
	}
}
