package app

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"reporting-db-migrations/internal/types"
)

func loadEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read env file %s: %w", path, err)
	}
	defer f.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		env[key] = value
	}
	return env, scanner.Err()
}

type envLookupFn func(string) (string, bool)

func osEnvLookup(key string) (string, bool) { return os.LookupEnv(key) }

func buildConfig(flags cliFlags, env map[string]string, lookup envLookupFn) types.Config {
	cfg := types.Config{}
	lookupStr := func(key string) string {
		if v, ok := env[key]; ok {
			return v
		}
		if lookup != nil {
			if v, ok := lookup(key); ok {
				return v
			}
		}
		return ""
	}

	cfg.SQLRoot = lookupStr("RM_SQL_ROOT")
	cfg.SQLBase = lookupStr("RM_SQL_BASE")
	cfg.ReportDir = lookupStr("RM_REPORT_DIR")
	cfg.LogLevel = lookupStr("RM_LOG_LEVEL")
	cfg.Server = lookupStr("RM_DB_SERVER")
	cfg.Port = lookupStr("RM_DB_PORT")
	cfg.Database = lookupStr("RM_DB_DATABASE")
	cfg.DBAuth = lookupStr("RM_DB_AUTH")
	cfg.User = lookupStr("RM_DB_USER")
	cfg.Password = lookupStr("RM_DB_PASSWORD")
	cfg.GitCommit = lookupStr("RM_GIT_COMMIT")
	cfg.GitBranch = lookupStr("RM_GIT_BRANCH")
	cfg.PipelineRunID = lookupStr("RM_PIPELINE_RUN_ID")
	cfg.PipelineURL = lookupStr("RM_PIPELINE_URL")
	cfg.Actor = lookupStr("RM_ACTOR")
	cfg.ToolVersion = lookupStr("RM_TOOL_VERSION")
	cfg.ToolCommit = lookupStr("RM_TOOL_COMMIT")
	cfg.UpdatePolicy = lookupStr("RM_UPDATE_POLICY")
	cfg.TransactionMode = lookupStr("RM_TRANSACTION_MODE")

	if v := lookupStr("RM_DB_ENCRYPT"); v != "" {
		cfg.Encrypt, _ = strconv.ParseBool(v)
	}
	if v := lookupStr("RM_DB_TRUST_SERVER_CERTIFICATE"); v != "" {
		cfg.TrustServerCertificate, _ = strconv.ParseBool(v)
	}
	if v := lookupStr("RM_COMMAND_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.CommandTimeout = d
		}
	}
	if v := lookupStr("RM_SCRIPT_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.ScriptTimeout = d
		}
	}
	if v := lookupStr("RM_LOCK_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.LockTimeout = d
		}
	}

	cfg.JSONLogs = flags.JSON

	return cfg
}

func validateConfig(cfg types.Config) error {
	var missing []string
	if cfg.Server == "" {
		missing = append(missing, "RM_DB_SERVER")
	}
	if cfg.Database == "" {
		missing = append(missing, "RM_DB_DATABASE")
	}
	if len(missing) > 0 {
		return fmt.Errorf("configuration error: missing required variables: %s", strings.Join(missing, ", "))
	}
	return nil
}
