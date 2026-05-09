package db

import (
	"net/url"
	"testing"

	"reporting-db-migrations/internal/config"
)

func TestBuildDSNSQLAuthIncludesUserPassword(t *testing.T) {
	dsn := buildDSN(config.Config{
		Server:                 "server",
		Port:                   "1433",
		Database:               "db",
		DBAuth:                 config.DBAuthSQL,
		User:                   "sql_user",
		Password:               "secret",
		Encrypt:                true,
		TrustServerCertificate: false,
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if u.User == nil {
		t.Fatal("expected user info in DSN")
	}
	if got := u.User.Username(); got != "sql_user" {
		t.Fatalf("expected sql_user, got %q", got)
	}
	password, ok := u.User.Password()
	if !ok || password != "secret" {
		t.Fatalf("expected password secret, got ok=%t value=%q", ok, password)
	}
	query := u.Query()
	if got := query.Get("authenticator"); got != "" {
		t.Fatalf("expected no authenticator for sql auth, got %q", got)
	}
}

func TestBuildDSNIntegratedAuthOmitsPasswordAndSetsWinSSPI(t *testing.T) {
	dsn := buildDSN(config.Config{
		Server:                 "server",
		Port:                   "1433",
		Database:               "db",
		DBAuth:                 config.DBAuthIntegrated,
		Encrypt:                false,
		TrustServerCertificate: true,
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if u.User != nil {
		t.Fatalf("expected no user info for integrated auth, got %q", u.User.String())
	}
	query := u.Query()
	if got := query.Get("authenticator"); got != "winsspi" {
		t.Fatalf("expected winsspi authenticator, got %q", got)
	}
	if got := query.Get("database"); got != "db" {
		t.Fatalf("expected database db, got %q", got)
	}
	if got := query.Get("TrustServerCertificate"); got != "true" {
		t.Fatalf("expected TrustServerCertificate=true, got %q", got)
	}
	if got := query.Get("encrypt"); got != "false" {
		t.Fatalf("expected encrypt=false, got %q", got)
	}
}

func TestBuildDSNIntegratedAuthKeepsExplicitWindowsUser(t *testing.T) {
	dsn := buildDSN(config.Config{
		Server:   "server",
		Port:     "1433",
		Database: "db",
		DBAuth:   config.DBAuthIntegrated,
		User:     `CORP\\user`,
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if u.User == nil {
		t.Fatal("expected user info for explicit integrated user")
	}
	if got := u.User.Username(); got != `CORP\\user` {
		t.Fatalf("expected explicit windows user, got %q", got)
	}
	if _, ok := u.User.Password(); ok {
		t.Fatal("did not expect password for integrated auth")
	}
}
