package config

import (
	"errors"
	"os"
	"path/filepath"
	"reporting-db-migrations/internal/contracts"
	"strings"
	"testing"
)

func TestValidateCommonAllowsIntegratedAuthWithoutUserAndPassword(t *testing.T) {
	cfg := Config{Env: "prod", Server: "server", Database: "db", DBAuth: DBAuthIntegrated, UpdatePolicy: UpdatePolicyNone, TransactionMode: TransactionModeScript}
	if err := cfg.ValidateCommon(); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
}

func TestValidateCommonRejectsUnknownDBAuth(t *testing.T) {
	cfg := Config{Env: "prod", Server: "server", Database: "db", DBAuth: "something-else", User: "user", Password: "password", UpdatePolicy: UpdatePolicyNone, TransactionMode: TransactionModeScript}
	err := cfg.ValidateCommon()
	if err == nil || !strings.Contains(err.Error(), "allowed: sql, integrated") {
		t.Fatalf("expected invalid auth error, got %v", err)
	}
}

func TestLoadRequiresSQLRootAndBaseForPlanCommandsLater(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
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

func TestLoadClassifiesInvalidTimeoutFlagAsInvalidInput(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")

	_, err := Load(Input{Env: "prod", CommandTimeout: "not-a-duration", commandTimeoutFromFlag: true})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, contracts.ErrInvalidInput) {
		t.Fatalf("expected invalid input sentinel, got %v", err)
	}
}

func TestLoadClassifiesInvalidTimeoutEnvironmentAsConfigError(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	t.Setenv("RM_TIMEOUT", "not-a-duration")

	_, err := Load(Input{Env: "prod", CommandTimeout: Getenv("RM_TIMEOUT", "900s")})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, contracts.ErrConfig) {
		t.Fatalf("expected config sentinel, got %v", err)
	}
}

func TestLoadRejectsInvalidEncryptEnvironmentAsConfigError(t *testing.T) {
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	t.Setenv("RM_DB_ENCRYPT", "maybe")

	_, err := Load(Input{Env: "prod"})
	if err == nil {
		t.Fatal("expected encrypt error")
	}
	if !errors.Is(err, contracts.ErrConfig) {
		t.Fatalf("expected config sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "RM_DB_ENCRYPT") {
		t.Fatalf("expected env key in error, got %v", err)
	}
}

func TestGetenvBoolRejectsInvalidValue(t *testing.T) {
	t.Setenv("RM_DB_ENCRYPT", "maybe")
	_, err := GetenvBool("RM_DB_ENCRYPT", true)
	if err == nil {
		t.Fatal("expected bool parse error")
	}
	if !strings.Contains(err.Error(), "RM_DB_ENCRYPT") {
		t.Fatalf("expected key in error, got %v", err)
	}
}

func TestValidateForCommandDoesNotRequireManagedSchemasForValidate(t *testing.T) {
	root := t.TempDir()
	base := "dwh"
	if err := osMkdirAll(joinEffectiveBasePath(root, base)); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	t.Setenv("RM_DB_SERVER", "server")
	t.Setenv("RM_DB_DATABASE", "db")
	t.Setenv("RM_DB_USER", "user")
	t.Setenv("RM_DB_PASSWORD", "password")
	cfg := Config{Env: "prod", Server: "server", Database: "db", User: "user", Password: "password", SQLRoot: root, SQLBase: base, EffectiveBasePath: joinEffectiveBasePath(root, base), UpdatePolicy: UpdatePolicyNone, TransactionMode: TransactionModeScript}
	if err := cfg.ValidateForCommand("validate"); err != nil {
		t.Fatalf("unexpected validate config error: %v", err)
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
	root := t.TempDir()
	base := "dwh"
	if err := osMkdirAll(joinEffectiveBasePath(root, base)); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	cfg := Config{Env: "prod", Server: "server", Database: "db", User: "user", Password: "password", SQLRoot: root, SQLBase: base, EffectiveBasePath: joinEffectiveBasePath(root, base), UpdatePolicy: UpdatePolicyNone, TransactionMode: TransactionModeScript}
	cfg.applyRepositoryDefaults()
	err := cfg.ValidateForCommand("plan")
	if err == nil || !strings.Contains(err.Error(), "RM_GIT_COMMIT") {
		t.Fatalf("expected git commit validation error, got %v", err)
	}
}

func TestApplyRepositoryDefaultsUsesSingleSQLBase(t *testing.T) {
	root := t.TempDir()
	if err := osMkdirAll(filepath.Join(root, "dwh")); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	cfg := Config{SQLRoot: root}
	cfg.applyRepositoryDefaults()
	if cfg.SQLBase != "dwh" {
		t.Fatalf("expected detected base dwh, got %q", cfg.SQLBase)
	}
}

func TestApplyRepositoryDefaultsDoesNotGuessMultipleSQLBases(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"dwh", "mart"} {
		if err := osMkdirAll(filepath.Join(root, name)); err != nil {
			t.Fatalf("mkdir base %s: %v", name, err)
		}
	}
	cfg := Config{SQLRoot: root}
	cfg.applyRepositoryDefaults()
	if cfg.SQLBase != "" {
		t.Fatalf("expected empty base when multiple candidates exist, got %q", cfg.SQLBase)
	}
}

func TestApplyRepositoryDefaultsUsesGitHeadCommit(t *testing.T) {
	root := t.TempDir()
	if err := osMkdirAll(filepath.Join(root, ".git", "refs", "heads")); err != nil {
		t.Fatalf("mkdir git refs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "refs", "heads", "main"), []byte("deadbeef\n"), 0o644); err != nil {
		t.Fatalf("write ref: %v", err)
	}
	cfg := Config{SQLRoot: root}
	cfg.applyRepositoryDefaults()
	if cfg.GitCommit != "deadbeef" {
		t.Fatalf("expected detected git commit, got %q", cfg.GitCommit)
	}
}

func TestApplyRepositoryDefaultsUsesSQLBaseAsDatabase(t *testing.T) {
	cfg := Config{SQLBase: "dactests"}
	cfg.applyRepositoryDefaults()
	if cfg.Database != "dactests" {
		t.Fatalf("expected database to default from sql base, got %q", cfg.Database)
	}
}

func TestApplyRepositoryDefaultsKeepsExplicitDatabase(t *testing.T) {
	cfg := Config{SQLBase: "dactests", Database: "reportingdb"}
	cfg.applyRepositoryDefaults()
	if cfg.Database != "reportingdb" {
		t.Fatalf("expected explicit database to win, got %q", cfg.Database)
	}
}

func TestValidateForCommandRequiresConfirmForBaseline(t *testing.T) {
	root := t.TempDir()
	base := "dwh"
	if err := osMkdirAll(joinEffectiveBasePath(root, base)); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	cfg := Config{Env: "prod", Server: "server", Database: "db", User: "user", Password: "password", SQLRoot: root, SQLBase: base, EffectiveBasePath: joinEffectiveBasePath(root, base), GitCommit: "abc", UpdatePolicy: UpdatePolicyNone, TransactionMode: TransactionModeScript}
	err := cfg.ValidateForCommand("baseline")
	if err == nil || !strings.Contains(err.Error(), "confirm flag required") {
		t.Fatalf("expected confirm validation error, got %v", err)
	}
	if !errors.Is(err, contracts.ErrInvalidInput) {
		t.Fatalf("expected invalid input sentinel, got %v", err)
	}
}

func TestValidateForCommandRequiresScriptForRepairChecksum(t *testing.T) {
	root := t.TempDir()
	base := "dwh"
	if err := osMkdirAll(joinEffectiveBasePath(root, base)); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	cfg := Config{Env: "prod", Server: "server", Database: "db", User: "user", Password: "password", SQLRoot: root, SQLBase: base, EffectiveBasePath: joinEffectiveBasePath(root, base), GitCommit: "abc", Confirm: true, UpdatePolicy: UpdatePolicyNone, TransactionMode: TransactionModeScript}
	err := cfg.ValidateForCommand("repair-checksum")
	if err == nil || !strings.Contains(err.Error(), "--script is required") {
		t.Fatalf("expected script validation error, got %v", err)
	}
	if !errors.Is(err, contracts.ErrInvalidInput) {
		t.Fatalf("expected invalid input sentinel, got %v", err)
	}
}

func TestValidateCommonWrapsMissingConfigAsConfigError(t *testing.T) {
	cfg := Config{UpdatePolicy: UpdatePolicyNone, TransactionMode: TransactionModeScript}
	err := cfg.ValidateCommon()
	if err == nil {
		t.Fatal("expected missing config error")
	}
	if !errors.Is(err, contracts.ErrConfig) {
		t.Fatalf("expected config sentinel, got %v", err)
	}
}

func TestValidateSQLSelectionWrapsPathErrorsAsInvalidInput(t *testing.T) {
	cfg := Config{SQLRoot: t.TempDir(), SQLBase: ""}
	err := cfg.ValidateSQLSelection()
	if err == nil {
		t.Fatal("expected sql selection error")
	}
	if !errors.Is(err, contracts.ErrInvalidInput) {
		t.Fatalf("expected invalid input sentinel, got %v", err)
	}
}

func TestValidateRejectsUnknownEnvironment(t *testing.T) {
	cfg := Config{Env: "stage", Server: "server", Database: "db", User: "user", Password: "password", UpdatePolicy: UpdatePolicyNone, TransactionMode: TransactionModeScript}
	err := cfg.ValidateCommon()
	if err == nil || !strings.Contains(err.Error(), "allowed: pred, prod") {
		t.Fatalf("expected invalid environment error, got %v", err)
	}
}

func TestValidateSQLSelectionRejectsPathTraversalBase(t *testing.T) {
	root := t.TempDir()
	cfg := Config{SQLRoot: root, SQLBase: "../bad"}
	err := cfg.ValidateSQLSelection()
	if err == nil || !strings.Contains(err.Error(), "invalid_or_missing_base_selection") {
		t.Fatalf("expected base selection error, got %v", err)
	}
}

func TestValidateSQLSelectionRejectsAbsoluteBase(t *testing.T) {
	root := t.TempDir()
	cfg := Config{SQLRoot: root, SQLBase: filepath.Join(root, "dwh")}
	err := cfg.ValidateSQLSelection()
	if err == nil || !strings.Contains(err.Error(), "must not be an absolute path") {
		t.Fatalf("expected absolute base error, got %v", err)
	}
}

func TestValidateSQLSelectionRejectsSeparatorBase(t *testing.T) {
	root := t.TempDir()
	cfg := Config{SQLRoot: root, SQLBase: "bad/base"}
	err := cfg.ValidateSQLSelection()
	if err == nil || !strings.Contains(err.Error(), "single directory name") {
		t.Fatalf("expected separator base error, got %v", err)
	}
}

func TestValidateSQLSelectionRejectsEmptyBase(t *testing.T) {
	root := t.TempDir()
	cfg := Config{SQLRoot: root, SQLBase: ""}
	err := cfg.ValidateSQLSelection()
	if err == nil || !strings.Contains(err.Error(), "RM_SQL_BASE is required") {
		t.Fatalf("expected empty base error, got %v", err)
	}
}

func TestValidateSQLSelectionRejectsSQLRootFile(t *testing.T) {
	rootFile := filepath.Join(t.TempDir(), "sql-package.tar.gz")
	if err := os.WriteFile(rootFile, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{SQLRoot: rootFile, SQLBase: "dwh"}
	err := cfg.ValidateSQLSelection()
	if err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("expected root file error, got %v", err)
	}
}

func TestValidateCommonRejectsInvalidUpdatePolicy(t *testing.T) {
	cfg := Config{Env: "prod", Server: "server", Database: "db", User: "user", Password: "password", UpdatePolicy: "wrong", TransactionMode: TransactionModeScript}
	err := cfg.ValidateCommon()
	if err == nil || !strings.Contains(err.Error(), "invalid_update_policy") {
		t.Fatalf("expected invalid update policy error, got %v", err)
	}
	if !errors.Is(err, contracts.ErrInvalidInput) {
		t.Fatalf("expected invalid input sentinel, got %v", err)
	}
}

func TestValidateCommonRejectsInvalidTransactionMode(t *testing.T) {
	cfg := Config{Env: "prod", Server: "server", Database: "db", User: "user", Password: "password", UpdatePolicy: UpdatePolicyNone, TransactionMode: "wrong"}
	err := cfg.ValidateCommon()
	if err == nil || !strings.Contains(err.Error(), "invalid_transaction_mode") {
		t.Fatalf("expected invalid transaction mode error, got %v", err)
	}
	if !errors.Is(err, contracts.ErrInvalidInput) {
		t.Fatalf("expected invalid input sentinel, got %v", err)
	}
}

func TestWrapErrorsUseContractsSentinels(t *testing.T) {
	configErr := wrapConfigError(errors.New("missing required config: RM_GIT_COMMIT"))
	if !errors.Is(configErr, contracts.ErrConfig) {
		t.Fatalf("expected config sentinel, got %v", configErr)
	}
	invalidInputErr := wrapInvalidInputError(errors.New("--script is required"))
	if !errors.Is(invalidInputErr, contracts.ErrInvalidInput) {
		t.Fatalf("expected invalid input sentinel, got %v", invalidInputErr)
	}
}

func osMkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}
