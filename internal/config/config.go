package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"reporting-db-migrations/internal/commands"
	"reporting-db-migrations/internal/contracts"
)

type Input struct {
	Env, SQLRoot, SQLBase, ReportDir, LogLevel, CommandTimeout, ScriptTimeout, LockTimeout, EnvFile, PlanFile string
	RepairTarget, UpdatePolicy, TransactionMode                                                               string
	JSONLogs, SkipValidate, Confirm, PlanJSON                                                                 bool
	commandTimeoutFromFlag, scriptTimeoutFromFlag, lockTimeoutFromFlag                                        bool
	jsonLogsFromFlag, skipValidateFromFlag, confirmFromFlag, planJSONFromFlag                                 bool
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

func GetenvBool(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err == nil {
		return parsed, nil
	}
	switch strings.ToLower(value) {
	case "yes", "y":
		return true, nil
	case "no", "n":
		return false, nil
	default:
		return false, fmt.Errorf("invalid %s: %q (allowed: true, false, 1, 0, yes, no, y, n)", key, value)
	}
}

func Load(input Input) (Config, error) {
	commandTimeout, err := loadDurationInput("timeout", "RM_TIMEOUT", input.CommandTimeout, "900s", input.commandTimeoutFromFlag)
	if err != nil {
		return Config{}, err
	}
	scriptTimeout, err := loadDurationInput("script-timeout", "RM_SCRIPT_TIMEOUT", input.ScriptTimeout, "600s", input.scriptTimeoutFromFlag)
	if err != nil {
		return Config{}, err
	}
	lockTimeout, err := loadDurationInput("lock-timeout", "RM_LOCK_TIMEOUT", input.LockTimeout, "60s", input.lockTimeoutFromFlag)
	if err != nil {
		return Config{}, err
	}
	jsonLogs, err := loadBoolInput("RM_JSON_LOGS", input.JSONLogs, false, input.jsonLogsFromFlag)
	if err != nil {
		return Config{}, wrapConfigError(err)
	}
	planJSON, err := loadBoolInput("RM_PLAN_JSON", input.PlanJSON, false, input.planJSONFromFlag)
	if err != nil {
		return Config{}, wrapConfigError(err)
	}
	confirm, err := loadBoolInput("RM_CONFIRM", input.Confirm, false, input.confirmFromFlag)
	if err != nil {
		return Config{}, wrapConfigError(err)
	}
	skipValidate, err := loadBoolInput("RM_SKIP_VALIDATE", input.SkipValidate, false, input.skipValidateFromFlag)
	if err != nil {
		return Config{}, wrapConfigError(err)
	}

	cfg := Config{
		Env:             normalizeEnv(input.Env),
		SQLRoot:         strings.TrimSpace(input.SQLRoot),
		SQLBase:         strings.TrimSpace(input.SQLBase),
		ReportDir:       strings.TrimSpace(input.ReportDir),
		LogLevel:        def(input.LogLevel, "info"),
		JSONLogs:        jsonLogs,
		SkipValidate:    skipValidate,
		Confirm:         confirm,
		PlanJSON:        planJSON,
		CommandTimeout:  commandTimeout,
		ScriptTimeout:   scriptTimeout,
		LockTimeout:     lockTimeout,
		PlanFile:        input.PlanFile,
		RepairTarget:    strings.TrimSpace(input.RepairTarget),
		ComparisonMode:  ComparisonModeCaseInsensitive,
		UpdatePolicy:    normalizeUpdatePolicy(def(input.UpdatePolicy, UpdatePolicyModulesOnly)),
		TransactionMode: normalizeTransactionMode(def(input.TransactionMode, TransactionModeScript)),
	}
	if err := cfg.loadEnvironment(); err != nil {
		return Config{}, err
	}
	cfg.applyRepositoryDefaults()
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
		return wrapConfigError(fmt.Errorf("missing required config: %s", strings.Join(missing, ", ")))
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
		return wrapConfigError(fmt.Errorf("invalid environment: %s (allowed: pred, prod)", cfg.Env))
	}
	return nil
}

func (cfg Config) ValidateDBAuth() error {
	if _, ok := allowedDBAuthModes[cfg.DBAuthMode()]; !ok {
		return wrapConfigError(fmt.Errorf("invalid RM_DB_AUTH: %s (allowed: sql, integrated)", strings.TrimSpace(cfg.DBAuth)))
	}
	return nil
}

func (cfg Config) ValidateGitCommit() error {
	if strings.TrimSpace(cfg.GitCommit) == "" {
		return wrapConfigError(fmt.Errorf("missing required config: RM_GIT_COMMIT"))
	}
	return nil
}

func (cfg Config) ValidateUpdatePolicy() error {
	if _, ok := allowedUpdatePolicies[cfg.UpdatePolicy]; !ok {
		return wrapInvalidInputError(fmt.Errorf("invalid_update_policy: %s", cfg.UpdatePolicy))
	}
	return nil
}

func (cfg Config) ValidateTransactionMode() error {
	if _, ok := allowedTransactionModes[cfg.TransactionMode]; !ok {
		return wrapInvalidInputError(fmt.Errorf("invalid_transaction_mode: %s", cfg.TransactionMode))
	}
	return nil
}

func (cfg Config) ValidateSQLSelection() error {
	if strings.TrimSpace(cfg.SQLRoot) == "" {
		return wrapInvalidInputError(fmt.Errorf("invalid_or_missing_sql_scripts_root: RM_SQL_ROOT is required"))
	}
	rootInfo, err := os.Stat(cfg.SQLRoot)
	if err != nil {
		return wrapInvalidInputError(fmt.Errorf("invalid_or_missing_sql_scripts_root: %v", err))
	}
	if !rootInfo.IsDir() {
		return wrapInvalidInputError(fmt.Errorf("invalid_or_missing_sql_scripts_root: %s is not a directory", cfg.SQLRoot))
	}
	if err := validateSQLBaseName(cfg.SQLBase); err != nil {
		return wrapInvalidInputError(err)
	}
	baseInfo, err := os.Stat(cfg.SelectedBasePath())
	if err != nil {
		return wrapInvalidInputError(fmt.Errorf("invalid_or_missing_base_selection: %v", err))
	}
	if !baseInfo.IsDir() {
		return wrapInvalidInputError(fmt.Errorf("invalid_or_missing_base_selection: %s is not a directory", cfg.SelectedBasePath()))
	}
	return nil
}

func (cfg Config) ValidateForCommand(command string) error {
	spec, ok := commands.Lookup(command)
	if !ok {
		return wrapInvalidInputError(fmt.Errorf("unknown command: %s", command))
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
	if spec.RequiresRepairTarget && strings.TrimSpace(cfg.RepairTarget) == "" {
		return wrapInvalidInputError(fmt.Errorf("--script is required"))
	}
	if spec.RequiresConfirm && !cfg.Confirm {
		return wrapInvalidInputError(fmt.Errorf("confirm flag required"))
	}
	return nil
}

func wrapConfigError(err error) error {
	if err == nil || errors.Is(err, contracts.ErrConfig) {
		return err
	}
	return contracts.Wrap(contracts.ErrConfig, err)
}

func wrapInvalidInputError(err error) error {
	if err == nil || errors.Is(err, contracts.ErrInvalidInput) {
		return err
	}
	return contracts.Wrap(contracts.ErrInvalidInput, err)
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

func (cfg *Config) loadEnvironment() error {
	cfg.Server = strings.TrimSpace(os.Getenv("RM_DB_SERVER"))
	cfg.Port = def(os.Getenv("RM_DB_PORT"), "1433")
	cfg.Database = strings.TrimSpace(os.Getenv("RM_DB_DATABASE"))
	cfg.DBAuth = normalizeDBAuth(os.Getenv("RM_DB_AUTH"))
	cfg.User = strings.TrimSpace(os.Getenv("RM_DB_USER"))
	cfg.Password = os.Getenv("RM_DB_PASSWORD")
	encrypt, err := GetenvBool("RM_DB_ENCRYPT", true)
	if err != nil {
		return wrapConfigError(err)
	}
	trustServerCertificate, err := GetenvBool("RM_DB_TRUST_SERVER_CERTIFICATE", false)
	if err != nil {
		return wrapConfigError(err)
	}
	cfg.Encrypt = encrypt
	cfg.TrustServerCertificate = trustServerCertificate
	cfg.GitCommit = os.Getenv("RM_GIT_COMMIT")
	cfg.GitBranch = os.Getenv("RM_GIT_BRANCH")
	cfg.PipelineRunID = os.Getenv("RM_PIPELINE_RUN_ID")
	cfg.PipelineURL = os.Getenv("RM_PIPELINE_URL")
	cfg.Actor = os.Getenv("RM_ACTOR")
	return nil
}

func (cfg *Config) applyRepositoryDefaults() {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.SQLBase) == "" {
		cfg.SQLBase = detectSingleSQLBase(cfg.SQLRoot)
	}
	if strings.TrimSpace(cfg.Database) == "" {
		cfg.Database = strings.TrimSpace(cfg.SQLBase)
	}
	if strings.TrimSpace(cfg.GitCommit) == "" {
		cfg.GitCommit = detectGitCommit(cfg.SQLRoot)
	}
}

func detectSingleSQLBase(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	bases := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		bases = append(bases, entry.Name())
	}
	if len(bases) != 1 {
		return ""
	}
	return bases[0]
}

func detectGitCommit(root string) string {
	for dir := detectGitStart(root); strings.TrimSpace(dir) != ""; dir = filepath.Dir(dir) {
		if value := readGitHead(dir); value != "" {
			return value
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return ""
}

func detectGitStart(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return ""
		}
		return cwd
	}
	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		return filepath.Dir(root)
	}
	return root
}

func readGitHead(dir string) string {
	headPath := filepath.Join(dir, ".git", "HEAD")
	head, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(head))
	if line == "" {
		return ""
	}
	if !strings.HasPrefix(line, "ref: ") {
		return line
	}
	refPath := filepath.Join(dir, ".git", filepath.FromSlash(strings.TrimSpace(strings.TrimPrefix(line, "ref: "))))
	ref, err := os.ReadFile(refPath)
	if err == nil {
		return strings.TrimSpace(string(ref))
	}
	packedRefs, err := os.ReadFile(filepath.Join(dir, ".git", "packed-refs"))
	if err != nil {
		return ""
	}
	refName := strings.TrimSpace(strings.TrimPrefix(line, "ref: "))
	for _, entry := range strings.Split(string(packedRefs), "\n") {
		entry = strings.TrimSpace(entry)
		if entry == "" || strings.HasPrefix(entry, "#") || strings.HasPrefix(entry, "^") {
			continue
		}
		hash, name, ok := strings.Cut(entry, " ")
		if ok && strings.TrimSpace(name) == refName {
			return strings.TrimSpace(hash)
		}
	}
	return ""
}

func loadBoolInput(envKey string, value bool, fallback bool, fromFlag bool) (bool, error) {
	if fromFlag {
		return value, nil
	}
	return GetenvBool(envKey, fallback)
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

func loadDurationInput(name string, envKey string, value string, fallback string, fromFlag bool) (time.Duration, error) {
	duration, err := parseDuration(name, def(value, fallback))
	if err == nil {
		return duration, nil
	}
	if fromFlag {
		return 0, wrapInvalidInputError(err)
	}
	if raw, ok := os.LookupEnv(envKey); ok && strings.TrimSpace(raw) != "" {
		return 0, wrapConfigError(err)
	}
	return 0, wrapConfigError(err)
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
