package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
	"reporting-db-migrations/internal/failures"
	"reporting-db-migrations/internal/logger"
	"reporting-db-migrations/internal/migrator"
)

type BuildInfo struct {
	Version string
	Commit  string
}

type Handler interface {
	Info(ctx context.Context, cfg config.Config, log logger.Logger) error
	Plan(ctx context.Context, cfg config.Config, log logger.Logger) (contracts.MigrationPlan, error)
	Migrate(ctx context.Context, cfg config.Config, log logger.Logger) error
	Validate(ctx context.Context, cfg config.Config, log logger.Logger) error
	Baseline(ctx context.Context, cfg config.Config, log logger.Logger) error
	RepairChecksum(ctx context.Context, cfg config.Config, log logger.Logger) error
}

type Runtime struct {
	BuildInfo BuildInfo
	Handler   Handler
	Stdout    io.Writer
	Stderr    io.Writer
}

func Run(args []string, build BuildInfo) int {
	return defaultRuntime(build).Run(args)
}

func defaultRuntime(build BuildInfo) Runtime {
	return Runtime{BuildInfo: build, Handler: migrator.Handler{}, Stdout: os.Stdout, Stderr: os.Stderr}
}

func (r Runtime) Run(args []string) int {
	stdout := writerOrDefault(r.Stdout, os.Stdout)
	stderr := writerOrDefault(r.Stderr, os.Stderr)

	if len(args) < 2 {
		printUsage(stdout)
		return contracts.ExitConfigError
	}

	command := args[1]
	if command == "version" {
		fmt.Fprintf(stdout, "rmig %s commit=%s\n", r.BuildInfo.Version, r.BuildInfo.Commit)
		return contracts.ExitOK
	}

	switch command {
	case "info", "plan", "migrate", "validate", "baseline", "repair-checksum":
		return r.dispatch(command, args[2:], stdout, stderr)
	default:
		failure := failures.BuildWithCause(config.Config{}, "config_failed", contracts.ErrInvalidInput, fmt.Errorf("unknown command: %s", command))
		fmt.Fprintln(stderr, failure.Error)
		printUsage(stdout)
		return contracts.ExitConfigError
	}
}

func (r Runtime) dispatch(command string, args []string, stdout io.Writer, stderr io.Writer) int {
	cfg, err := parseCommandConfig(command, args)
	if err != nil {
		failure := failures.BuildWithCause(config.Config{}, "config_failed", contracts.ErrInvalidInput, err)
		fmt.Fprintln(stderr, failure.Error)
		return contracts.ExitConfigError
	}
	cfg.ToolVersion = r.BuildInfo.Version
	cfg.ToolCommit = r.BuildInfo.Commit

	logWriter := stdout
	if command == contracts.CommandPlan && cfg.PlanJSON {
		logWriter = stderr
	}
	log := logger.New(logger.Options{JSON: cfg.JSONLogs, Level: cfg.LogLevel, Writer: logWriter})
	ctx, cancel := context.WithTimeout(context.Background(), cfg.CommandTimeout)
	defer cancel()

	handler := r.Handler
	if handler == nil {
		handler = migrator.Handler{}
	}

	switch command {
	case "info":
		return exitCode(cfg, handler.Info(ctx, cfg, log), log, "info_failed")
	case "plan":
		plan, err := handler.Plan(ctx, cfg, log)
		if err != nil {
			return exitCode(cfg, err, log, "plan_failed")
		}
		if cfg.PlanJSON {
			if err := writePlanJSON(stdout, plan); err != nil {
				return exitCode(cfg, err, log, "plan_output_failed")
			}
		}
		if plan.Blocked {
			failure := failures.BuildPlanBlocked(cfg, plan)
			log.Error("plan_failed", failure.Error)
			return contracts.ExitChecksumMismatch
		}
		return contracts.ExitOK
	case "migrate":
		return exitCode(cfg, handler.Migrate(ctx, cfg, log), log, "migration_failed")
	case "validate":
		return exitCode(cfg, handler.Validate(ctx, cfg, log), log, "validation_failed")
	case "baseline":
		return exitCode(cfg, handler.Baseline(ctx, cfg, log), log, "baseline_failed")
	case "repair-checksum":
		return exitCode(cfg, handler.RepairChecksum(ctx, cfg, log), log, "repair_checksum_failed")
	default:
		return contracts.ExitConfigError
	}
}

func parseCommandConfig(command string, args []string) (config.Config, error) {
	restoreEnv, err := applyEnvironmentFile(resolveEnvFilePath(args))
	if err != nil {
		return config.Config{}, err
	}
	defer restoreEnv()

	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	input := config.Input{}
	var envFile string
	flags.StringVar(&input.Env, "env", config.Getenv("RM_ENV", ""), "target environment")
	flags.StringVar(&input.SQLRoot, "sql-root", config.Getenv("RM_SQL_ROOT", ""), "SQL scripts root directory")
	flags.StringVar(&input.SQLBase, "sql-base", config.Getenv("RM_SQL_BASE", ""), "SQL base directory under the root")
	flags.StringVar(&input.ReportDir, "report-dir", config.Getenv("RM_REPORT_DIR", "./reports"), "report output directory")
	flags.StringVar(&input.LogLevel, "log-level", config.Getenv("RM_LOG_LEVEL", "info"), "log level")
	flags.BoolVar(&input.JSONLogs, "json-logs", config.GetenvBool("RM_JSON_LOGS", false), "emit JSON logs")
	flags.BoolVar(&input.PlanJSON, "json", config.GetenvBool("RM_PLAN_JSON", false), "emit plan JSON to stdout")
	flags.StringVar(&input.CommandTimeout, "timeout", config.Getenv("RM_TIMEOUT", "900s"), "command timeout")
	flags.StringVar(&input.ScriptTimeout, "script-timeout", config.Getenv("RM_SCRIPT_TIMEOUT", "600s"), "per-script timeout")
	flags.StringVar(&input.LockTimeout, "lock-timeout", config.Getenv("RM_LOCK_TIMEOUT", "60s"), "SQL app lock timeout")
	flags.StringVar(&envFile, "env-file", config.Getenv("RM_ENV_FILE", ""), "optional env file with RM_* values")
	_ = envFile
	flags.StringVar(&input.PlanFile, "plan-file", config.Getenv("RM_PLAN_FILE", ""), "approved migration plan file")
	flags.StringVar(&input.RepairTarget, "script", config.Getenv("RM_REPAIR_SCRIPT", ""), "repo object path or normalized key to repair")
	flags.StringVar(&input.UpdatePolicy, "update-policy", config.Getenv("RM_UPDATE_POLICY", config.UpdatePolicyNone), "existing object update policy")
	flags.StringVar(&input.TransactionMode, "transaction-mode", config.Getenv("RM_TRANSACTION_MODE", config.TransactionModeScript), "transaction mode")
	flags.BoolVar(&input.Confirm, "confirm", config.GetenvBool("RM_CONFIRM", false), "confirm destructive command")
	flags.BoolVar(&input.SkipValidate, "skip-validate", config.GetenvBool("RM_SKIP_VALIDATE", false), "skip validation after migrate")

	if err := flags.Parse(args); err != nil {
		return config.Config{}, err
	}
	cfg, err := config.Load(input)
	if err != nil {
		return config.Config{}, err
	}
	if command == contracts.CommandMigrate && cfg.PlanFile == "" {
		return config.Config{}, fmt.Errorf("--plan-file is required")
	}
	if err := cfg.ValidateForCommand(command); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func exitCode(cfg config.Config, err error, log logger.Logger, event string) int {
	if err == nil {
		return contracts.ExitOK
	}
	failure := failures.Build(cfg, event, err)
	log.Error(event, failure.Error)

	switch {
	case errors.Is(err, contracts.ErrConfig):
		return contracts.ExitConfigError
	case errors.Is(err, contracts.ErrConnection):
		return contracts.ExitConnError
	case errors.Is(err, contracts.ErrChecksumMismatch):
		return contracts.ExitChecksumMismatch
	case errors.Is(err, contracts.ErrSQLExecution):
		return contracts.ExitSQLExecution
	case errors.Is(err, contracts.ErrValidation):
		return contracts.ExitValidation
	case errors.Is(err, contracts.ErrLockTimeout):
		return contracts.ExitLockTimeout
	case errors.Is(err, contracts.ErrInvalidInput):
		return contracts.ExitInvalidInput
	case errors.Is(err, contracts.ErrCriticalState):
		return contracts.ExitCriticalState
	default:
		return contracts.ExitGeneralError
	}
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "Usage:")
	fmt.Fprintln(writer, "  rmig version")
	fmt.Fprintln(writer, "  env values: pred, prod")
	fmt.Fprintln(writer, "  rmig info --env prod")
	fmt.Fprintln(writer, "  rmig plan --env prod --sql-root ./sql --sql-base dwh")
	fmt.Fprintln(writer, "  optional: --env-file path/to/.env or RM_ENV_FILE=path/to/.env")
	fmt.Fprintln(writer, "  rmig migrate --env prod --sql-root ./sql --sql-base dwh --plan-file reports/migration-plan.json")
	fmt.Fprintln(writer, "  rmig validate --env prod --sql-root ./sql --sql-base dwh")
	fmt.Fprintln(writer, "  rmig baseline --env prod --sql-root ./sql --sql-base dwh --confirm")
	fmt.Fprintln(writer, "  rmig repair-checksum --env prod --sql-root ./sql --sql-base dwh --script reporting/views/monthly.sql --confirm")
}

func writerOrDefault(writer io.Writer, fallback io.Writer) io.Writer {
	if writer == nil {
		return fallback
	}
	return writer
}

func writePlanJSON(writer io.Writer, plan contracts.MigrationPlan) error {
	b, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = writer.Write(b)
	return err
}
