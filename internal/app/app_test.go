package app

import (
	"bytes"
	"reporting-db-migrations/internal/contracts"
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
	for _, expected := range []string{"baseline --env prod --up-to V010 --confirm", "repair-checksum --env prod --script R002__views.sql --confirm"} {
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
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	t.Setenv("RM_MANAGED_SCHEMAS", "reporting")
	t.Setenv("RM_GIT_COMMIT", "deadbeef")
	_, err := parseCommandConfig("migrate", []string{"--env", "prod"})
	if err == nil || !strings.Contains(err.Error(), "--plan-file is required") {
		t.Fatalf("expected plan-file error, got %v", err)
	}
}

func TestParseCommandConfigRequiresManagedSchemasForValidate(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	t.Setenv("RM_MANAGED_SCHEMAS", "")
	_, err := parseCommandConfig("validate", []string{"--env", "prod"})
	if err == nil || !strings.Contains(err.Error(), "RM_MANAGED_SCHEMAS") {
		t.Fatalf("expected managed schemas error, got %v", err)
	}
}

func TestParseCommandConfigAllowsMissingManagedSchemasForPlan(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	t.Setenv("RM_GIT_COMMIT", "deadbeef")
	t.Setenv("RM_MANAGED_SCHEMAS", "")
	if _, err := parseCommandConfig("plan", []string{"--env", "prod"}); err != nil {
		t.Fatalf("unexpected plan config error: %v", err)
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
	t.Setenv("RM_MANAGED_SCHEMAS", "reporting")
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	runtime := Runtime{BuildInfo: BuildInfo{Version: "1.0.0", Commit: "abc"}, Stdout: &stdout, Stderr: &stderr}
	code := runtime.Run([]string{"rmig", "info", "--env", "prod"})
	if code == contracts.ExitInvalidInput && strings.Contains(stdout.String(), "not implemented") {
		t.Fatalf("runtime fell back to stub handler: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if code != contracts.ExitConnError {
		t.Fatalf("expected connection error from real handler, got %d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
