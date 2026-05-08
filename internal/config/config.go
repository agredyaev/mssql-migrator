package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Input struct {
	Env, SQLDir, ReportDir, LogLevel, CommandTimeout, ScriptTimeout, LockTimeout, PlanFile string
	BaselineUpTo, RepairScript                                                             string
	JSONLogs, SkipValidate, Confirm                                                        bool
}

type Config struct {
	Env, SQLDir, ReportDir, LogLevel                        string
	JSONLogs, SkipValidate                                  bool
	Confirm                                                 bool
	CommandTimeout, ScriptTimeout, LockTimeout              time.Duration
	PlanFile, BaselineUpTo, RepairScript                    string
	Server, Port, Database, User, Password                  string
	Encrypt, TrustServerCertificate                         bool
	ManagedSchemas                                          []string
	GitCommit, GitBranch, PipelineRunID, PipelineURL, Actor string
	ToolVersion, ToolCommit                                 string
}

var managedSchemaPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func Getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func GetenvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err == nil {
		return parsed
	}
	switch strings.ToLower(value) {
	case "yes", "y", "1":
		return true
	case "no", "n", "0":
		return false
	default:
		return fallback
	}
}

func Load(input Input) (Config, error) {
	commandTimeout, err := parseDuration("timeout", def(input.CommandTimeout, "900s"))
	if err != nil {
		return Config{}, err
	}
	scriptTimeout, err := parseDuration("script-timeout", def(input.ScriptTimeout, "600s"))
	if err != nil {
		return Config{}, err
	}
	lockTimeout, err := parseDuration("lock-timeout", def(input.LockTimeout, "60s"))
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Env:            strings.TrimSpace(input.Env),
		SQLDir:         def(input.SQLDir, "./sql"),
		ReportDir:      def(input.ReportDir, "./reports"),
		LogLevel:       def(input.LogLevel, "info"),
		JSONLogs:       input.JSONLogs,
		SkipValidate:   input.SkipValidate,
		Confirm:        input.Confirm,
		CommandTimeout: commandTimeout,
		ScriptTimeout:  scriptTimeout,
		LockTimeout:    lockTimeout,
		PlanFile:       input.PlanFile,
		BaselineUpTo:   strings.TrimSpace(input.BaselineUpTo),
		RepairScript:   strings.TrimSpace(input.RepairScript),
	}
	cfg.loadEnvironment()
	return cfg, cfg.ValidateCommon()
}

func (cfg Config) Validate() error {
	if err := cfg.ValidateCommon(); err != nil {
		return err
	}
	return cfg.ValidateManagedSchemas()
}

func (cfg Config) ValidateCommon() error {
	missing := []string{}
	if cfg.Env == "" {
		missing = append(missing, "--env or RM_ENV")
	}
	if cfg.Server == "" {
		missing = append(missing, "RM_DB_SERVER")
	}
	if cfg.Database == "" {
		missing = append(missing, "RM_DB_DATABASE")
	}
	if cfg.User == "" {
		missing = append(missing, "RM_DB_USER")
	}
	if cfg.Password == "" {
		missing = append(missing, "RM_DB_PASSWORD")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (cfg Config) ValidateManagedSchemas() error {
	if len(cfg.ManagedSchemas) == 0 {
		return fmt.Errorf("missing required config: RM_MANAGED_SCHEMAS")
	}
	for _, schema := range cfg.ManagedSchemas {
		if !managedSchemaPattern.MatchString(schema) {
			return fmt.Errorf("invalid managed schema name: %s", schema)
		}
	}
	return nil
}

func (cfg Config) ValidateGitCommit() error {
	if strings.TrimSpace(cfg.GitCommit) == "" {
		return fmt.Errorf("missing required config: RM_GIT_COMMIT")
	}
	return nil
}

func (cfg Config) ValidateForCommand(command string) error {
	if err := cfg.ValidateCommon(); err != nil {
		return err
	}
	switch command {
	case "plan":
		return cfg.ValidateGitCommit()
	case "validate":
		return cfg.ValidateManagedSchemas()
	case "migrate":
		if err := cfg.ValidateGitCommit(); err != nil {
			return err
		}
		if !cfg.SkipValidate {
			return cfg.ValidateManagedSchemas()
		}
	case "baseline", "repair-checksum":
		return cfg.ValidateGitCommit()
	}
	return nil
}

func (cfg Config) MaskedTarget() string {
	return fmt.Sprintf("server=%s;port=%s;database=%s;user=%s;password=***", cfg.Server, cfg.Port, cfg.Database, cfg.User)
}

func (cfg *Config) loadEnvironment() {
	cfg.Server = strings.TrimSpace(os.Getenv("RM_DB_SERVER"))
	cfg.Port = def(os.Getenv("RM_DB_PORT"), "1433")
	cfg.Database = strings.TrimSpace(os.Getenv("RM_DB_DATABASE"))
	cfg.User = strings.TrimSpace(os.Getenv("RM_DB_USER"))
	cfg.Password = os.Getenv("RM_DB_PASSWORD")
	cfg.Encrypt = GetenvBool("RM_DB_ENCRYPT", true)
	cfg.TrustServerCertificate = GetenvBool("RM_DB_TRUST_SERVER_CERTIFICATE", false)
	cfg.ManagedSchemas = splitCSV(os.Getenv("RM_MANAGED_SCHEMAS"))
	cfg.GitCommit = os.Getenv("RM_GIT_COMMIT")
	cfg.GitBranch = os.Getenv("RM_GIT_BRANCH")
	cfg.PipelineRunID = os.Getenv("RM_PIPELINE_RUN_ID")
	cfg.PipelineURL = os.Getenv("RM_PIPELINE_URL")
	cfg.Actor = os.Getenv("RM_ACTOR")
}

func parseDuration(name, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("invalid %s: must be greater than zero", name)
	}
	return duration, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func def(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
