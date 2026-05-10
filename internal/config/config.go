package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"reporting-db-migrations/internal/commands"
)

type Input struct {
	Env, SQLRoot, SQLBase, ReportDir, LogLevel, CommandTimeout, ScriptTimeout, LockTimeout, EnvFile, PlanFile string
	RepairTarget, UpdatePolicy, TransactionMode                                                               string
	JSONLogs, SkipValidate, Confirm, PlanJSON                                                                 bool
}

type Config struct {
	Env, SQLRoot, SQLBase, EffectiveBasePath, ReportDir, LogLevel string
	JSONLogs, SkipValidate, Confirm, PlanJSON                     bool
	CommandTimeout, ScriptTimeout, LockTimeout                    time.Duration
	PlanFile, RepairTarget                                        string
	Server, Port, Database, DBAuth, User, Password                string
	Encrypt, TrustServerCertificate                               bool
	GitCommit, GitBranch, PipelineRunID, PipelineURL, Actor       string
	ToolVersion, ToolCommit                                       string
	ComparisonMode, UpdatePolicy, TransactionMode                 string
}

const (
	DBAuthSQL        = "sql"
	DBAuthIntegrated = "integrated"

	ComparisonModeCaseInsensitive = "case_insensitive"

	UpdatePolicyNone         = "none"
	UpdatePolicyModulesOnly  = "modules_only"
	UpdatePolicyAllSupported = "all_supported"

	TransactionModeScript = "script"
	TransactionModeNone   = "none"
)

var allowedEnvironments = map[string]struct{}{
	"pred": {},
	"prod": {},
}

var allowedDBAuthModes = map[string]struct{}{
	DBAuthSQL:        {},
	DBAuthIntegrated: {},
}

var allowedUpdatePolicies = map[string]struct{}{
	UpdatePolicyNone:         {},
	UpdatePolicyModulesOnly:  {},
	UpdatePolicyAllSupported: {},
}

var allowedTransactionModes = map[string]struct{}{
	TransactionModeScript: {},
	TransactionModeNone:   {},
}

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
		Env:             normalizeEnv(input.Env),
		SQLRoot:         strings.TrimSpace(input.SQLRoot),
		SQLBase:         strings.TrimSpace(input.SQLBase),
		ReportDir:       def(input.ReportDir, "./reports"),
		LogLevel:        def(input.LogLevel, "info"),
		JSONLogs:        input.JSONLogs,
		SkipValidate:    input.SkipValidate,
		Confirm:         input.Confirm,
		PlanJSON:        input.PlanJSON,
		CommandTimeout:  commandTimeout,
		ScriptTimeout:   scriptTimeout,
		LockTimeout:     lockTimeout,
		PlanFile:        input.PlanFile,
		RepairTarget:    strings.TrimSpace(input.RepairTarget),
		ComparisonMode:  ComparisonModeCaseInsensitive,
		UpdatePolicy:    normalizeUpdatePolicy(def(input.UpdatePolicy, UpdatePolicyNone)),
		TransactionMode: normalizeTransactionMode(def(input.TransactionMode, TransactionModeScript)),
	}
	cfg.loadEnvironment()
	cfg.EffectiveBasePath = joinEffectiveBasePath(cfg.SQLRoot, cfg.SQLBase)
	return cfg, cfg.ValidateCommon()
}

func (cfg Config) Validate() error {
	if err := cfg.ValidateCommon(); err != nil {
		return err
	}
	return cfg.ValidateSQLSelection()
}

func (cfg Config) ValidateCommon() error {
	authMode := cfg.DBAuthMode()
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
	if authMode == DBAuthSQL {
		if cfg.User == "" {
			missing = append(missing, "RM_DB_USER")
		}
		if cfg.Password == "" {
			missing = append(missing, "RM_DB_PASSWORD")
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	if err := cfg.ValidateEnvironment(); err != nil {
		return err
	}
	if err := cfg.ValidateDBAuth(); err != nil {
		return err
	}
	if err := cfg.ValidateUpdatePolicy(); err != nil {
		return err
	}
	if err := cfg.ValidateTransactionMode(); err != nil {
		return err
	}
	return nil
}

func (cfg Config) ValidateEnvironment() error {
	if _, ok := allowedEnvironments[cfg.Env]; !ok {
		return fmt.Errorf("invalid environment: %s (allowed: pred, prod)", cfg.Env)
	}
	return nil
}

func (cfg Config) ValidateDBAuth() error {
	if _, ok := allowedDBAuthModes[cfg.DBAuthMode()]; !ok {
		return fmt.Errorf("invalid RM_DB_AUTH: %s (allowed: sql, integrated)", strings.TrimSpace(cfg.DBAuth))
	}
	return nil
}

func (cfg Config) ValidateGitCommit() error {
	if strings.TrimSpace(cfg.GitCommit) == "" {
		return fmt.Errorf("missing required config: RM_GIT_COMMIT")
	}
	return nil
}

func (cfg Config) ValidateUpdatePolicy() error {
	if _, ok := allowedUpdatePolicies[cfg.UpdatePolicy]; !ok {
		return fmt.Errorf("invalid_update_policy: %s", cfg.UpdatePolicy)
	}
	return nil
}

func (cfg Config) ValidateTransactionMode() error {
	if _, ok := allowedTransactionModes[cfg.TransactionMode]; !ok {
		return fmt.Errorf("invalid_transaction_mode: %s", cfg.TransactionMode)
	}
	return nil
}

func (cfg Config) ValidateSQLSelection() error {
	if strings.TrimSpace(cfg.SQLRoot) == "" {
		return fmt.Errorf("invalid_or_missing_sql_scripts_root: RM_SQL_ROOT is required")
	}
	rootInfo, err := os.Stat(cfg.SQLRoot)
	if err != nil {
		return fmt.Errorf("invalid_or_missing_sql_scripts_root: %v", err)
	}
	if !rootInfo.IsDir() {
		return fmt.Errorf("invalid_or_missing_sql_scripts_root: %s is not a directory", cfg.SQLRoot)
	}
	if err := validateSQLBaseName(cfg.SQLBase); err != nil {
		return err
	}
	baseInfo, err := os.Stat(cfg.SelectedBasePath())
	if err != nil {
		return fmt.Errorf("invalid_or_missing_base_selection: %v", err)
	}
	if !baseInfo.IsDir() {
		return fmt.Errorf("invalid_or_missing_base_selection: %s is not a directory", cfg.SelectedBasePath())
	}
	return nil
}

func (cfg Config) ValidateForCommand(command string) error {
	spec, ok := commands.Lookup(command)
	if !ok {
		return fmt.Errorf("unknown command: %s", command)
	}
	if err := cfg.ValidateCommon(); err != nil {
		return err
	}
	if spec.RequiresSQLSelection {
		if err := cfg.ValidateSQLSelection(); err != nil {
			return err
		}
	}
	if spec.RequiresGitCommit {
		if err := cfg.ValidateGitCommit(); err != nil {
			return err
		}
	}
	if spec.RequiresPlanFile && strings.TrimSpace(cfg.PlanFile) == "" {
		return fmt.Errorf("--plan-file is required")
	}
	if spec.RequiresConfirm && !cfg.Confirm {
		return fmt.Errorf("confirm flag required")
	}
	return nil
}

func (cfg Config) DBAuthMode() string {
	return normalizeDBAuth(cfg.DBAuth)
}

func (cfg Config) MaskedTarget() string {
	target := fmt.Sprintf("server=%s;port=%s;database=%s;auth=%s", cfg.Server, cfg.Port, cfg.Database, cfg.DBAuthMode())
	if cfg.DBAuthMode() == DBAuthIntegrated {
		user := cfg.User
		if user == "" {
			user = "current-session"
		}
		return target + ";user=" + user
	}
	return target + ";user=" + cfg.User + ";password=***"
}

func (cfg Config) SelectedBasePath() string {
	if cfg.EffectiveBasePath != "" {
		return cfg.EffectiveBasePath
	}
	return joinEffectiveBasePath(cfg.SQLRoot, cfg.SQLBase)
}

func (cfg *Config) loadEnvironment() {
	cfg.Server = strings.TrimSpace(os.Getenv("RM_DB_SERVER"))
	cfg.Port = def(os.Getenv("RM_DB_PORT"), "1433")
	cfg.Database = strings.TrimSpace(os.Getenv("RM_DB_DATABASE"))
	cfg.DBAuth = normalizeDBAuth(os.Getenv("RM_DB_AUTH"))
	cfg.User = strings.TrimSpace(os.Getenv("RM_DB_USER"))
	cfg.Password = os.Getenv("RM_DB_PASSWORD")
	cfg.Encrypt = GetenvBool("RM_DB_ENCRYPT", true)
	cfg.TrustServerCertificate = GetenvBool("RM_DB_TRUST_SERVER_CERTIFICATE", false)
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

func def(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizeEnv(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeDBAuth(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return DBAuthSQL
	}
	return value
}

func normalizeUpdatePolicy(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return UpdatePolicyNone
	}
	return value
}

func normalizeTransactionMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return TransactionModeScript
	}
	return value
}

func validateSQLBaseName(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("invalid_or_missing_base_selection: RM_SQL_BASE is required")
	}
	if filepath.IsAbs(value) {
		return fmt.Errorf("invalid_or_missing_base_selection: RM_SQL_BASE must not be an absolute path")
	}
	if value == "." || value == ".." {
		return fmt.Errorf("invalid_or_missing_base_selection: RM_SQL_BASE must not contain path traversal segments")
	}
	if strings.Contains(value, "/") || strings.Contains(value, `\`) {
		return fmt.Errorf("invalid_or_missing_base_selection: RM_SQL_BASE must be a single directory name")
	}
	return nil
}

func joinEffectiveBasePath(root string, base string) string {
	root = strings.TrimSpace(root)
	base = strings.TrimSpace(base)
	if root == "" || base == "" {
		return ""
	}
	return filepath.Join(root, base)
}
