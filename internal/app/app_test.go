package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reporting-db-migrations/internal/types"
)

func TestParseFlags_Commands(t *testing.T) {
	tests := []struct {
		name    string
		wantCmd string
		args    []string
		wantErr bool
	}{
		{name: "plan", args: []string{"plan"}, wantCmd: "plan"},
		{name: "migrate", args: []string{"migrate"}, wantCmd: "migrate"},
		{name: "validate", args: []string{"validate"}, wantCmd: "validate"},
		{name: "baseline", args: []string{"baseline"}, wantCmd: "baseline"},
		{name: "repair-checksum", args: []string{"repair-checksum"}, wantCmd: "repair-checksum"},
		{name: "version", args: []string{"version"}, wantCmd: "version"},
		{name: "unknown command", args: []string{"unknown"}, wantErr: true},
		{name: "no args", args: []string{}, wantErr: true},
		{name: "extra arg after command", args: []string{"plan", "extra"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := parseFlags(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if flags.Command != tt.wantCmd {
				t.Errorf("command = %q, want %q", flags.Command, tt.wantCmd)
			}
		})
	}
}

func TestParseFlags_Options(t *testing.T) {
	tests := []struct {
		name     string
		wantCmd  string
		wantEnv  string
		args     []string
		wantJSON bool
		wantErr  bool
	}{
		{
			name:    "plan with --env",
			args:    []string{"--env", "/tmp/.env", "plan"},
			wantCmd: "plan",
			wantEnv: "/tmp/.env",
		},
		{
			name:     "migrate with --json",
			args:     []string{"--json", "migrate"},
			wantCmd:  "migrate",
			wantJSON: true,
		},
		{
			name:     "plan with both flags",
			args:     []string{"--env", "prod.env", "--json", "plan"},
			wantCmd:  "plan",
			wantEnv:  "prod.env",
			wantJSON: true,
		},
		{
			name:     "command before flags is ok",
			args:     []string{"plan", "--json"},
			wantCmd:  "plan",
			wantJSON: true,
		},
		{
			name:    "--env without value",
			args:    []string{"--env"},
			wantErr: true,
		},
		{
			name:    "unknown flag",
			args:    []string{"--foo", "plan"},
			wantErr: true,
		},
		{
			name:    "--help",
			args:    []string{"--help"},
			wantErr: true,
		},
		{
			name:    "-h",
			args:    []string{"-h"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := parseFlags(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if flags.Command != tt.wantCmd {
				t.Errorf("command = %q, want %q", flags.Command, tt.wantCmd)
			}
			if flags.EnvFile != tt.wantEnv {
				t.Errorf("env file = %q, want %q", flags.EnvFile, tt.wantEnv)
			}
			if flags.JSON != tt.wantJSON {
				t.Errorf("json = %v, want %v", flags.JSON, tt.wantJSON)
			}
		})
	}
}

func TestLoadEnvFile(t *testing.T) {
	t.Run("valid env file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		content := "RM_DB_SERVER=localhost\nRM_DB_DATABASE=testdb\nRM_SQL_ROOT=/sql\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		env, err := loadEnvFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if env["RM_DB_SERVER"] != "localhost" {
			t.Errorf("RM_DB_SERVER = %q, want localhost", env["RM_DB_SERVER"])
		}
		if env["RM_DB_DATABASE"] != "testdb" {
			t.Errorf("RM_DB_DATABASE = %q, want testdb", env["RM_DB_DATABASE"])
		}
		if env["RM_SQL_ROOT"] != "/sql" {
			t.Errorf("RM_SQL_ROOT = %q, want /sql", env["RM_SQL_ROOT"])
		}
	})

	t.Run("missing file returns nil", func(t *testing.T) {
		env, err := loadEnvFile("/nonexistent/.env")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if env != nil {
			t.Errorf("expected nil env for missing file")
		}
	})

	t.Run("quoted values stripped", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		content := "RM_DB_SERVER=\"localhost\"\nRM_DB_USER='admin'\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		env, err := loadEnvFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if env["RM_DB_SERVER"] != "localhost" {
			t.Errorf("RM_DB_SERVER = %q, want localhost (quotes stripped)", env["RM_DB_SERVER"])
		}
		if env["RM_DB_USER"] != "admin" {
			t.Errorf("RM_DB_USER = %q, want admin (quotes stripped)", env["RM_DB_USER"])
		}
	})

	t.Run("comments and blanks ignored", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		content := "# this is a comment\n\nRM_DB_SERVER=localhost\n# another comment\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		env, err := loadEnvFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(env) != 1 {
			t.Errorf("expected 1 key, got %d: %v", len(env), env)
		}
		if env["RM_DB_SERVER"] != "localhost" {
			t.Errorf("RM_DB_SERVER = %q, want localhost", env["RM_DB_SERVER"])
		}
	})
}

func TestBuildConfig_EnvPrecedence(t *testing.T) {
	tests := []struct {
		env    map[string]string
		lookup func(string) (string, bool)
		want   map[string]string
		name   string
		flags  cliFlags
	}{
		{
			name:  "env file values",
			flags: cliFlags{Command: "plan"},
			env: map[string]string{
				"RM_DB_SERVER": "envfile-host",
			},
			lookup: nil,
			want:   map[string]string{"RM_DB_SERVER": "envfile-host"},
		},
		{
			name:  "os env overrides env file",
			flags: cliFlags{Command: "plan"},
			env: map[string]string{
				"RM_DB_SERVER": "envfile-host",
			},
			lookup: func(k string) (string, bool) {
				if k == "RM_DB_SERVER" {
					return "osenv-host", true
				}
				return "", false
			},
			want: map[string]string{"RM_DB_SERVER": "envfile-host"},
		},
		{
			name:  "os env only",
			flags: cliFlags{Command: "plan"},
			env:   nil,
			lookup: func(k string) (string, bool) {
				if k == "RM_DB_DATABASE" {
					return "mydb", true
				}
				return "", false
			},
			want: map[string]string{"RM_DB_DATABASE": "mydb"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := buildConfig(tt.flags, tt.env, tt.lookup)
			for envKey, wantVal := range tt.want {
				got := configField(cfg, envKey)
				if got != wantVal {
					t.Errorf("%s = %q, want %q", envKey, got, wantVal)
				}
			}
		})
	}
}

func TestBuildConfig_JSONFlag(t *testing.T) {
	cfg := buildConfig(cliFlags{JSON: true}, nil, nil)
	if !cfg.JSONLogs {
		t.Error("expected JSONLogs=true")
	}

	cfg = buildConfig(cliFlags{JSON: false}, nil, nil)
	if cfg.JSONLogs {
		t.Error("expected JSONLogs=false")
	}
}

func TestBuildConfig_PlanRepairFromEnv(t *testing.T) {
	env := map[string]string{
		"RM_PLAN_FILE":     "/tmp/plan.json",
		"RM_REPAIR_SCRIPT": "reporting/views/monthly.sql",
	}
	cfg := buildConfig(cliFlags{}, env, nil)
	if cfg.PlanFile != "/tmp/plan.json" {
		t.Errorf("PlanFile = %q, want /tmp/plan.json", cfg.PlanFile)
	}
	if cfg.RepairTarget != "reporting/views/monthly.sql" {
		t.Errorf("RepairTarget = %q, want reporting/views/monthly.sql", cfg.RepairTarget)
	}
}

func TestBuildConfig_BooleanEnvVars(t *testing.T) {
	tests := []struct {
		name  string
		env   map[string]string
		field string
		want  bool
	}{
		{name: "encrypt true", env: map[string]string{"RM_DB_ENCRYPT": "true"}, field: "Encrypt", want: true},
		{name: "encrypt false", env: map[string]string{"RM_DB_ENCRYPT": "false"}, field: "Encrypt", want: false},
		{name: "encrypt 0", env: map[string]string{"RM_DB_ENCRYPT": "0"}, field: "Encrypt", want: false},
		{name: "encrypt empty", env: map[string]string{"RM_DB_ENCRYPT": ""}, field: "Encrypt", want: false},
		{name: "encrypt TRUE", env: map[string]string{"RM_DB_ENCRYPT": "TRUE"}, field: "Encrypt", want: true},
		{name: "encrypt xyz", env: map[string]string{"RM_DB_ENCRYPT": "xyz"}, field: "Encrypt", want: false},
		{name: "trust cert true", env: map[string]string{"RM_DB_TRUST_SERVER_CERTIFICATE": "true"}, field: "TrustServerCertificate", want: true},
		{name: "trust cert 1", env: map[string]string{"RM_DB_TRUST_SERVER_CERTIFICATE": "1"}, field: "TrustServerCertificate", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := buildConfig(cliFlags{}, tt.env, nil)
			var got bool
			switch tt.field {
			case "Encrypt":
				got = cfg.Encrypt
			case "TrustServerCertificate":
				got = cfg.TrustServerCertificate
			}
			if got != tt.want {
				t.Errorf("%s = %v, want %v", tt.field, got, tt.want)
			}
		})
	}
}

func TestBuildConfig_DurationEnvVars(t *testing.T) {
	env := map[string]string{
		"RM_COMMAND_TIMEOUT": "30s",
		"RM_SCRIPT_TIMEOUT":  "5m",
		"RM_LOCK_TIMEOUT":    "10s",
	}
	cfg := buildConfig(cliFlags{}, env, nil)
	if cfg.CommandTimeout.String() != "30s" {
		t.Errorf("CommandTimeout = %v, want 30s", cfg.CommandTimeout)
	}
	if cfg.ScriptTimeout.String() != "5m0s" {
		t.Errorf("ScriptTimeout = %v, want 5m0s", cfg.ScriptTimeout)
	}
	if cfg.LockTimeout.String() != "10s" {
		t.Errorf("LockTimeout = %v, want 10s", cfg.LockTimeout)
	}
}

func TestBuildConfig_DurationEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "zero", env: map[string]string{"RM_COMMAND_TIMEOUT": "0s"}, want: "0s"},
		{name: "empty", env: map[string]string{"RM_COMMAND_TIMEOUT": ""}, want: "0s"},
		{name: "invalid", env: map[string]string{"RM_COMMAND_TIMEOUT": "abc"}, want: "0s"},
		{name: "negative", env: map[string]string{"RM_COMMAND_TIMEOUT": "-1s"}, want: "-1s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := buildConfig(cliFlags{}, tt.env, nil)
			if cfg.CommandTimeout.String() != tt.want {
				t.Errorf("CommandTimeout = %v, want %v", cfg.CommandTimeout, tt.want)
			}
		})
	}
}

func TestValidateConfig_MissingRequired(t *testing.T) {
	tests := []struct {
		name    string
		cfg     types.Config
		wantErr bool
	}{
		{
			name:    "all present",
			cfg:     types.Config{Server: "localhost", Database: "testdb", SQLRoot: "/sql", SQLBase: "dwh"},
			wantErr: false,
		},
		{
			name:    "missing server",
			cfg:     types.Config{Database: "testdb", SQLRoot: "/sql", SQLBase: "dwh"},
			wantErr: true,
		},
		{
			name:    "missing database",
			cfg:     types.Config{Server: "localhost", SQLRoot: "/sql", SQLBase: "dwh"},
			wantErr: true,
		},
		{
			name:    "missing sql root",
			cfg:     types.Config{Server: "localhost", Database: "db", SQLBase: "dwh"},
			wantErr: true,
		},
		{
			name:    "missing sql base",
			cfg:     types.Config{Server: "localhost", Database: "db", SQLRoot: "/sql"},
			wantErr: true,
		},
		{
			name:    "both missing",
			cfg:     types.Config{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRunWithLookup_InvalidFlags(t *testing.T) {
	code := runWithLookup([]string{"rmig", "--unknown"}, nil, nil)
	if code != types.ExitInvalidInput {
		t.Errorf("exit code = %d, want %d (ExitInvalidInput)", code, types.ExitInvalidInput)
	}
}

func TestRunWithLookup_UnknownCommand(t *testing.T) {
	code := runWithLookup([]string{"rmig", "unknown"}, nil, nil)
	if code != types.ExitInvalidInput {
		t.Errorf("exit code = %d, want %d (ExitInvalidInput)", code, types.ExitInvalidInput)
	}
}

func TestRunWithLookup_Version(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	code := runWithLookup([]string{"rmig", "version"}, nil, nil)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = old
	out, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	s := strings.TrimSpace(string(out))
	if !strings.HasPrefix(s, "rmig ") {
		t.Fatalf("stdout = %q, want prefix \"rmig \"", s)
	}
}

func TestRunWithLookup_VersionJSON(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	code := runWithLookup([]string{"rmig", "--json", "version"}, nil, nil)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = old
	out, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(string(out), `"version"`) || !strings.Contains(string(out), `"commit"`) {
		t.Fatalf("stdout = %q, want JSON with version and commit", string(out))
	}
}

func TestRunWithLookup_VersionSkipsEnvFile(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	code := runWithLookup([]string{"rmig", "--env", "/nonexistent/.env", "version"}, nil, nil)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = old
	if _, err := io.ReadAll(r); err != nil {
		t.Fatal(err)
	}
	r.Close()
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (version must not read .env)", code)
	}
}

func TestRunWithLookup_MissingRequiredConfig(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("# no RM_* keys\n"), 0644); err != nil {
		t.Fatal(err)
	}
	code := runWithLookup([]string{"rmig", "--env", envPath, "plan"}, func(k string) (string, bool) {
		return "", false
	}, nil)
	if code != types.ExitConfigError {
		t.Errorf("exit code = %d, want %d (ExitConfigError)", code, types.ExitConfigError)
	}
}

func TestRunWithLookup_MissingEnvFile(t *testing.T) {
	code := runWithLookup([]string{"rmig", "--env", "/nonexistent/.env", "plan"}, func(k string) (string, bool) {
		return "", false
	}, nil)
	if code != types.ExitConfigError {
		t.Errorf("exit code = %d, want %d (ExitConfigError)", code, types.ExitConfigError)
	}
}

func configField(cfg types.Config, key string) string {
	switch key {
	case "RM_SQL_ROOT":
		return cfg.SQLRoot
	case "RM_SQL_BASE":
		return cfg.SQLBase
	case "RM_REPORT_DIR":
		return cfg.ReportDir
	case "RM_LOG_LEVEL":
		return cfg.LogLevel
	case "RM_DB_SERVER":
		return cfg.Server
	case "RM_DB_PORT":
		return cfg.Port
	case "RM_DB_DATABASE":
		return cfg.Database
	case "RM_DB_AUTH":
		return cfg.DBAuth
	case "RM_DB_USER":
		return cfg.User
	case "RM_DB_PASSWORD":
		return cfg.Password
	case "RM_GIT_COMMIT":
		return cfg.GitCommit
	case "RM_GIT_BRANCH":
		return cfg.GitBranch
	case "RM_PIPELINE_RUN_ID":
		return cfg.PipelineRunID
	case "RM_PIPELINE_URL":
		return cfg.PipelineURL
	case "RM_ACTOR":
		return cfg.Actor
	case "RM_TOOL_VERSION":
		return cfg.ToolVersion
	case "RM_TOOL_COMMIT":
		return cfg.ToolCommit
	case "RM_UPDATE_POLICY":
		return cfg.UpdatePolicy
	case "RM_TRANSACTION_MODE":
		return cfg.TransactionMode
	case "RM_PLAN_FILE":
		return cfg.PlanFile
	case "RM_REPAIR_SCRIPT":
		return cfg.RepairTarget
	default:
		return ""
	}
}
