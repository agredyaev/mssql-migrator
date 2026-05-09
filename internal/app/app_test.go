package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/migrator"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	stdout := bytes.Buffer{}
	runtime := Runtime{BuildInfo: BuildInfo{Version: "4.0.0", Commit: "abc"}, Stdout: &stdout}
	code := runtime.Run([]string{"rmig", "version"})
	if code != contracts.ExitOK {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if stdout.String() != "rmig 4.0.0 commit=abc\n" {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestUsageIncludesV11Commands(t *testing.T) {
	buffer := bytes.Buffer{}
	printUsage(&buffer)
	output := buffer.String()
	for _, expected := range []string{"baseline --env prod --sql-root ./sql --sql-base dwh --confirm", "repair-checksum --env prod --sql-root ./sql --sql-base dwh --script reporting/views/monthly.sql --confirm"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("usage missing %q: %s", expected, output)
		}
	}
}

func TestUsageListsSupportedEnvironments(t *testing.T) {
	buffer := bytes.Buffer{}
	printUsage(&buffer)
	if !strings.Contains(buffer.String(), "env values: pred, prod") {
		t.Fatalf("usage missing supported environments: %s", buffer.String())
	}
}

func TestDefaultRuntimeUsesRealMigratorHandler(t *testing.T) {
	runtime := defaultRuntime(BuildInfo{Version: "1.0.0", Commit: "abc"})
	handler, ok := runtime.Handler.(migrator.Handler)
	if !ok {
		t.Fatalf("expected migrator handler, got %T", runtime.Handler)
	}
	if handler != (migrator.Handler{}) {
		t.Fatalf("unexpected handler value: %#v", handler)
	}
}

func TestParseCommandConfigRequiresPlanFileForMigrate(t *testing.T) {
	root, base := createSQLLayout(t)
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	t.Setenv("RM_GIT_COMMIT", "deadbeef")
	_, err := parseCommandConfig("migrate", []string{"--env", "prod", "--sql-root", root, "--sql-base", base})
	if err == nil || !strings.Contains(err.Error(), "--plan-file is required") {
		t.Fatalf("expected plan-file error, got %v", err)
	}
}

func TestParseCommandConfigDoesNotRequireManagedSchemasForValidate(t *testing.T) {
	root, base := createSQLLayout(t)
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	_, err := parseCommandConfig("validate", []string{"--env", "prod", "--sql-root", root, "--sql-base", base})
	if err != nil {
		t.Fatalf("unexpected validate config error: %v", err)
	}
}

func TestParseCommandConfigAllowsMissingManagedSchemasForPlan(t *testing.T) {
	root, base := createSQLLayout(t)
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	t.Setenv("RM_GIT_COMMIT", "deadbeef")
	if _, err := parseCommandConfig("plan", []string{"--env", "prod", "--sql-root", root, "--sql-base", base}); err != nil {
		t.Fatalf("unexpected plan config error: %v", err)
	}
}

func TestParseCommandConfigRequiresGitCommitForPlan(t *testing.T) {
	root, base := createSQLLayout(t)
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	t.Setenv("RM_GIT_COMMIT", "")
	_, err := parseCommandConfig("plan", []string{"--env", "prod", "--sql-root", root, "--sql-base", base})
	if err == nil || !strings.Contains(err.Error(), "RM_GIT_COMMIT") {
		t.Fatalf("expected git commit error, got %v", err)
	}
}

func TestRunWritesConfigFailureEnvelopeToStderr(t *testing.T) {
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	runtime := Runtime{BuildInfo: BuildInfo{Version: "1.0.0", Commit: "abc"}, Stdout: &stdout, Stderr: &stderr}
	code := runtime.Run([]string{"rmig", "plan", "--env", "prod"})
	if code != contracts.ExitConfigError {
		t.Fatalf("expected config error exit, got %d", code)
	}
	output := stderr.String()
	if !strings.Contains(output, "ERROR config_failed:") || !strings.Contains(output, "sql_root=-") || !strings.Contains(output, "base=-") || !strings.Contains(output, "class=invalid input") {
		t.Fatalf("expected config envelope on stderr, got %q", output)
	}
}

func TestParseCommandConfigAllowsPredEnvironment(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")

	cfg, err := parseCommandConfig("info", []string{"--env", "PRED"})
	if err != nil {
		t.Fatalf("unexpected config error: %v", err)
	}
	if cfg.Env != "pred" {
		t.Fatalf("expected normalized env, got %q", cfg.Env)
	}
}

func TestParseCommandConfigRejectsUnknownEnvironment(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")

	_, err := parseCommandConfig("info", []string{"--env", "stage"})
	if err == nil || !strings.Contains(err.Error(), "allowed: pred, prod") {
		t.Fatalf("expected invalid environment error, got %v", err)
	}
}

func TestRunUsesMigratorWhenHandlerNil(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "127.0.0.1")
	t.Setenv("RM_DB_PORT", "1")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	root, base := createSQLLayout(t)
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	runtime := Runtime{BuildInfo: BuildInfo{Version: "1.0.0", Commit: "abc"}, Stdout: &stdout, Stderr: &stderr}
	code := runtime.Run([]string{"rmig", "info", "--env", "prod", "--sql-root", root, "--sql-base", base})
	if code == contracts.ExitInvalidInput && strings.Contains(stdout.String(), "not implemented") {
		t.Fatalf("runtime fell back to stub handler: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if code != contracts.ExitConnError {
		t.Fatalf("expected connection error from real handler, got %d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestPlanJSONWritesJSONToStdout(t *testing.T) {
	root, base := createSQLLayout(t)
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	t.Setenv("RM_GIT_COMMIT", "deadbeef")
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	runtime := Runtime{BuildInfo: BuildInfo{Version: "1.0.0", Commit: "abc"}, Handler: planOnlyHandler{}, Stdout: &stdout, Stderr: &stderr}
	code := runtime.Run([]string{"rmig", "plan", "--env", "prod", "--sql-root", root, "--sql-base", base, "--json"})
	if code != contracts.ExitOK {
		t.Fatalf("unexpected exit code: %d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"schema_version"`) {
		t.Fatalf("expected json on stdout, got %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "INFO ") || strings.Contains(stdout.String(), "ERROR ") {
		t.Fatalf("expected stdout to contain only json payload, got %q", stdout.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid json stdout, got %v payload=%q", err, stdout.String())
	}
	for _, key := range []string{"schema_version", "command", "sql_root", "base", "effective_base_path", "target", "comparison_mode", "update_policy", "transaction_mode", "rollback", "summary", "schemas", "objects", "failures"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected key %q in stdout payload: %s", key, stdout.String())
		}
	}
}

func TestBlockedPlanReturnsChecksumMismatchExit(t *testing.T) {
	root, base := createSQLLayout(t)
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	t.Setenv("RM_GIT_COMMIT", "deadbeef")
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	runtime := Runtime{BuildInfo: BuildInfo{Version: "1.0.0", Commit: "abc"}, Handler: blockedPlanHandler{}, Stdout: &stdout, Stderr: &stderr}
	code := runtime.Run([]string{"rmig", "plan", "--env", "prod", "--sql-root", root, "--sql-base", base})
	if code != contracts.ExitChecksumMismatch {
		t.Fatalf("expected checksum mismatch exit, got %d", code)
	}
}

type blockedPlanHandler struct{}

func (blockedPlanHandler) Info(context.Context, config.Config, logger.Logger) error { return nil }

func (blockedPlanHandler) Plan(context.Context, config.Config, logger.Logger) (contracts.MigrationPlan, error) {
	return contracts.MigrationPlan{SchemaVersion: "v8", Command: "plan", Blocked: true}, nil
}

func (blockedPlanHandler) Migrate(context.Context, config.Config, logger.Logger) error { return nil }

func (blockedPlanHandler) Validate(context.Context, config.Config, logger.Logger) error { return nil }

func (blockedPlanHandler) Baseline(context.Context, config.Config, logger.Logger) error { return nil }

func (blockedPlanHandler) RepairChecksum(context.Context, config.Config, logger.Logger) error {
	return nil
}

type planOnlyHandler struct{}

func (planOnlyHandler) Info(context.Context, config.Config, logger.Logger) error { return nil }

func (planOnlyHandler) Plan(_ context.Context, cfg config.Config, _ logger.Logger) (contracts.MigrationPlan, error) {
	return contracts.MigrationPlan{
		SchemaVersion:     "v8",
		Command:           "plan",
		Tool:              "rmig",
		ToolVersion:       cfg.ToolVersion,
		ToolCommit:        cfg.ToolCommit,
		GitCommit:         cfg.GitCommit,
		SQLRoot:           cfg.SQLRoot,
		Base:              cfg.SQLBase,
		EffectiveBasePath: cfg.SelectedBasePath(),
		LayoutHash:        "hash",
		Target:            contracts.PlanTarget{Environment: cfg.Env, Database: cfg.Database},
	}, nil
}

func (planOnlyHandler) Migrate(context.Context, config.Config, logger.Logger) error { return nil }

func (planOnlyHandler) Validate(context.Context, config.Config, logger.Logger) error { return nil }

func (planOnlyHandler) Baseline(context.Context, config.Config, logger.Logger) error { return nil }

func (planOnlyHandler) RepairChecksum(context.Context, config.Config, logger.Logger) error {
	return nil
}

func createSQLLayout(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	base := "dwh"
	path := filepath.Join(root, base, "reporting", "views")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir sql layout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "monthly.sql"), []byte("SELECT 1;"), 0o644); err != nil {
		t.Fatalf("write sql file: %v", err)
	}
	return root, base
}
