package config

import "testing"

func TestValidateRejectsInvalidManagedSchema(t *testing.T) {
	cfg := Config{Env: "prod", Server: "server", Database: "db", User: "user", Password: "password", ManagedSchemas: []string{"reporting", "bad-name"}}
	if cfg.Validate() == nil {
		t.Fatal("expected invalid schema error")
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

func TestValidateForCommandRequiresManagedSchemasForValidate(t *testing.T) {
	cfg := Config{Env: "prod", Server: "server", Database: "db", User: "user", Password: "password"}
	if err := cfg.ValidateForCommand("validate"); err == nil {
		t.Fatal("expected missing RM_MANAGED_SCHEMAS error")
	}
}

func TestValidateForCommandRequiresGitCommitForPlan(t *testing.T) {
	cfg := Config{Env: "prod", Server: "server", Database: "db", User: "user", Password: "password"}
	if err := cfg.ValidateForCommand("plan"); err == nil {
		t.Fatal("expected missing RM_GIT_COMMIT error")
	}
}
