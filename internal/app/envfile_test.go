package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEnvFilePathPrefersCLIOverEnvironment(t *testing.T) {
	t.Setenv("RM_ENV_FILE", "from-env")
	path := resolveEnvFilePath([]string{"--env-file", "from-cli"})
	if path != "from-cli" {
		t.Fatalf("expected CLI env file path, got %q", path)
	}
}

func TestResolveEnvFilePathSupportsEqualsForm(t *testing.T) {
	path := resolveEnvFilePath([]string{"--env-file=from-cli"})
	if path != "from-cli" {
		t.Fatalf("expected equals env file path, got %q", path)
	}
}

func TestLoadEnvironmentFileParsesCommentsQuotesAndExport(t *testing.T) {
	path := writeEnvFile(t, "test.env", "# comment\nRM_DB_SERVER=server\nexport RM_DB_AUTH=integrated\nRM_SQL_BASE=\"dwh\"\nRM_SQL_ROOT='C:\\\\sql dir'\n")
	values, err := loadEnvironmentFile(path)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if values["RM_DB_SERVER"] != "server" {
		t.Fatalf("expected RM_DB_SERVER=server, got %q", values["RM_DB_SERVER"])
	}
	if values["RM_DB_AUTH"] != "integrated" {
		t.Fatalf("expected integrated auth, got %q", values["RM_DB_AUTH"])
	}
	if values["RM_SQL_BASE"] != "dwh" {
		t.Fatalf("expected sql base, got %q", values["RM_SQL_BASE"])
	}
	if values["RM_SQL_ROOT"] != `C:\\sql dir` {
		t.Fatalf("expected quoted windows path, got %q", values["RM_SQL_ROOT"])
	}
}

func TestLoadEnvironmentFileRejectsInvalidLine(t *testing.T) {
	path := writeEnvFile(t, "invalid.env", "RM_DB_SERVER\n")
	if _, err := loadEnvironmentFile(path); err == nil {
		t.Fatal("expected invalid env file error")
	}
}

func TestApplyEnvironmentFileDoesNotOverrideExistingEnvironment(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "existing-server")
	path := writeEnvFile(t, "override.env", "RM_DB_SERVER=file-server\nRM_DB_DATABASE=file-db\n")
	restore, err := applyEnvironmentFile(path)
	if err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}
	defer restore()
	if got := os.Getenv("RM_DB_SERVER"); got != "existing-server" {
		t.Fatalf("expected existing server to win, got %q", got)
	}
	if got := os.Getenv("RM_DB_DATABASE"); got != "file-db" {
		t.Fatalf("expected file database to apply, got %q", got)
	}
	restore()
	if got := os.Getenv("RM_DB_DATABASE"); got != "" {
		t.Fatalf("expected file database to be restored, got %q", got)
	}
}

func TestApplyEnvironmentFileDoesNotOverrideEmptyExistingEnvironment(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "")
	path := writeEnvFile(t, "override.env", "RM_DB_SERVER=file-server\n")
	restore, err := applyEnvironmentFile(path)
	if err != nil {
		t.Fatalf("unexpected apply error: %v", err)
	}
	defer restore()
	if got := os.Getenv("RM_DB_SERVER"); got != "" {
		t.Fatalf("expected empty process env to win, got %q", got)
	}
}

func TestParseCommandConfigLoadsValuesFromEnvFileWhenEnabled(t *testing.T) {
	root, base := createSQLLayout(t)
	path := writeEnvFile(t, "config.env", "RM_DB_SERVER=file-server\nRM_DB_DATABASE=file-db\nRM_DB_AUTH=integrated\nRM_GIT_COMMIT=file-commit\nRM_SQL_ROOT="+root+"\nRM_SQL_BASE="+base+"\n")
	cfg, err := parseCommandConfig("validate", []string{"--env", "pred", "--env-file", path})
	if err != nil {
		t.Fatalf("unexpected config error: %v", err)
	}
	if cfg.Server != "file-server" {
		t.Fatalf("expected file-server, got %q", cfg.Server)
	}
	if cfg.Database != "file-db" {
		t.Fatalf("expected file-db, got %q", cfg.Database)
	}
	if cfg.DBAuthMode() != "integrated" {
		t.Fatalf("expected integrated auth, got %q", cfg.DBAuthMode())
	}
	if cfg.SQLRoot != root || cfg.SQLBase != base {
		t.Fatalf("unexpected sql selection: root=%q base=%q", cfg.SQLRoot, cfg.SQLBase)
	}
}

func TestParseCommandConfigProcessEnvOverridesEnvFile(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "process-server")
	t.Setenv("RM_DB_DATABASE", "process-db")
	t.Setenv("RM_DB_USER", "process-user")
	t.Setenv("RM_DB_PASSWORD", "process-password")
	root, base := createSQLLayout(t)
	path := writeEnvFile(t, "config.env", "RM_DB_SERVER=file-server\nRM_DB_DATABASE=file-db\nRM_DB_USER=file-user\nRM_DB_PASSWORD=file-password\nRM_SQL_ROOT="+root+"\nRM_SQL_BASE="+base+"\n")
	cfg, err := parseCommandConfig("info", []string{"--env", "prod", "--env-file", path})
	if err != nil {
		t.Fatalf("unexpected config error: %v", err)
	}
	if cfg.Server != "process-server" {
		t.Fatalf("expected process env server, got %q", cfg.Server)
	}
	if cfg.Database != "process-db" {
		t.Fatalf("expected process env database, got %q", cfg.Database)
	}
	if cfg.User != "process-user" {
		t.Fatalf("expected process env user, got %q", cfg.User)
	}
}

func TestParseCommandConfigWithoutEnvFileKeepsCurrentBehavior(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	cfg, err := parseCommandConfig("info", []string{"--env", "prod"})
	if err != nil {
		t.Fatalf("unexpected config error: %v", err)
	}
	if cfg.Server != "server" || cfg.Database != "db" || cfg.User != "user" {
		t.Fatalf("unexpected config loaded without env file: %#v", cfg)
	}
}

func TestParseCommandConfigSupportsEnvFileFromEnvironment(t *testing.T) {
	root, base := createSQLLayout(t)
	path := writeEnvFile(t, "config.env", "RM_DB_SERVER=file-server\nRM_DB_DATABASE=file-db\nRM_DB_AUTH=integrated\nRM_GIT_COMMIT=file-commit\nRM_SQL_ROOT="+root+"\nRM_SQL_BASE="+base+"\n")
	t.Setenv("RM_ENV_FILE", path)
	cfg, err := parseCommandConfig("validate", []string{"--env", "pred"})
	if err != nil {
		t.Fatalf("unexpected config error: %v", err)
	}
	if cfg.Server != "file-server" {
		t.Fatalf("expected env file server, got %q", cfg.Server)
	}
}

func writeEnvFile(t *testing.T, name string, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	return path
}
