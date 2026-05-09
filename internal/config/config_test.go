package config

import (
	"strings"
	"testing"
)

func TestValidateRejectsInvalidManagedSchema(t *testing.T) {
	cfg := Config{Env: "prod", Server: "server", Database: "db", User: "user", Password: "password", ManagedSchemas: []string{"reporting", "bad-name"}}
	if cfg.Validate() == nil {
		t.Fatal("expected invalid schema error")
	}
}

func TestValidateCommonAllowsIntegratedAuthWithoutUserAndPassword(t *testing.T) {
	cfg := Config{Env: "prod", Server: "server", Database: "db", DBAuth: DBAuthIntegrated}
	if err := cfg.ValidateCommon(); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
}

func TestValidateCommonRejectsUnknownDBAuth(t *testing.T) {
	cfg := Config{Env: "prod", Server: "server", Database: "db", DBAuth: "something-else", User: "user", Password: "password"}
	err := cfg.ValidateCommon()
	if err == nil || !strings.Contains(err.Error(), "allowed: sql, integrated") {
		t.Fatalf("expected invalid auth error, got %v", err)
	}
}

func TestLoadAllowsMissingManagedSchemasFromEnvironment(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	t.Setenv("RM_MANAGED_SCHEMAS", "")
	if _, err := Load(Input{Env: "prod"}); err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
}

func TestLoadNormalizesPredEnvironment(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")

	cfg, err := Load(Input{Env: "PRED"})
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if cfg.Env != "pred" {
		t.Fatalf("expected normalized env, got %q", cfg.Env)
	}
}

func TestLoadDefaultsDBAuthToSQL(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")

	cfg, err := Load(Input{Env: "prod"})
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if cfg.DBAuthMode() != DBAuthSQL {
		t.Fatalf("expected sql auth, got %q", cfg.DBAuthMode())
	}
}

func TestLoadAllowsIntegratedAuthFromEnvironment(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_AUTH", "INTEGRATED")
	t.Setenv("RM_DB_USER", "")
	t.Setenv("RM_DB_PASSWORD", "")

	cfg, err := Load(Input{Env: "prod"})
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if cfg.DBAuthMode() != DBAuthIntegrated {
		t.Fatalf("expected integrated auth, got %q", cfg.DBAuthMode())
	}
}

func TestValidateForCommandRequiresManagedSchemasForValidate(t *testing.T) {
	cfg := Config{Env: "prod", Server: "server", Database: "db", User: "user", Password: "password"}
	if err := cfg.ValidateForCommand("validate"); err == nil {
		t.Fatal("expected missing RM_MANAGED_SCHEMAS error")
	}
}

func TestMaskedTargetOmitsPasswordForIntegratedAuth(t *testing.T) {
	cfg := Config{Server: "server", Port: "1433", Database: "db", DBAuth: DBAuthIntegrated}
	masked := cfg.MaskedTarget()
	if !strings.Contains(masked, "auth=integrated") {
		t.Fatalf("expected integrated auth marker, got %q", masked)
	}
	if strings.Contains(masked, "password=") {
		t.Fatalf("did not expect password in masked target: %q", masked)
	}
}

func TestValidateForCommandRequiresGitCommitForPlan(t *testing.T) {
	cfg := Config{Env: "prod", Server: "server", Database: "db", User: "user", Password: "password"}
	if err := cfg.ValidateForCommand("plan"); err == nil {
		t.Fatal("expected missing RM_GIT_COMMIT error")
	}
}

func TestValidateRejectsUnknownEnvironment(t *testing.T) {
	cfg := Config{Env: "stage", Server: "server", Database: "db", User: "user", Password: "password"}
	err := cfg.ValidateCommon()
	if err == nil || !strings.Contains(err.Error(), "allowed: pred, prod") {
		t.Fatalf("expected invalid environment error, got %v", err)
	}
}
