package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/contracts"
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
		fmt.Fprintf(stderr, "unknown command: %s\n", command)
		printUsage(stdout)
		return contracts.ExitConfigError
	}
}

func (r Runtime) dispatch(command string, args []string, stdout io.Writer, stderr io.Writer) int {
	cfg, err := parseCommandConfig(command, args)
	if err != nil {
		fmt.Fprintf(stderr, "config error: %v\n", err)
		return contracts.ExitConfigError
	}
	cfg.ToolVersion = r.BuildInfo.Version
	cfg.ToolCommit = r.BuildInfo.Commit

	log := logger.New(logger.Options{JSON: cfg.JSONLogs, Level: cfg.LogLevel, Writer: stdout})
	ctx, cancel := context.WithTimeout(context.Background(), cfg.CommandTimeout)
	defer cancel()

	handler := r.Handler
	if handler == nil {
		handler = migrator.Handler{}
	}

	switch command {
	case "info":
		return exitCode(handler.Info(ctx, cfg, log), log, "info_failed")
	case "plan":
		plan, err := handler.Plan(ctx, cfg, log)
		if err != nil {
			return exitCode(err, log, "plan_failed")
		}
		if plan.Blocked {
			return contracts.ExitChecksumMismatch
		}
		return contracts.ExitOK
	case "migrate":
		return exitCode(handler.Migrate(ctx, cfg, log), log, "migration_failed")
	case "validate":
		return exitCode(handler.Validate(ctx, cfg, log), log, "validation_failed")
	case "baseline":
		return exitCode(handler.Baseline(ctx, cfg, log), log, "baseline_failed")
	case "repair-checksum":
		return exitCode(handler.RepairChecksum(ctx, cfg, log), log, "repair_checksum_failed")
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
	flags.StringVar(&input.SQLDir, "sql-dir", config.Getenv("RM_SQL_DIR", "./sql"), "SQL root directory")
	flags.StringVar(&input.ReportDir, "report-dir", config.Getenv("RM_REPORT_DIR", "./reports"), "report output directory")
	flags.StringVar(&input.LogLevel, "log-level", config.Getenv("RM_LOG_LEVEL", "info"), "log level")
	flags.BoolVar(&input.JSONLogs, "json-logs", config.GetenvBool("RM_JSON_LOGS", false), "emit JSON logs")
	flags.StringVar(&input.CommandTimeout, "timeout", config.Getenv("RM_TIMEOUT", "900s"), "command timeout")
	flags.StringVar(&input.ScriptTimeout, "script-timeout", config.Getenv("RM_SCRIPT_TIMEOUT", "600s"), "per-script timeout")
	flags.StringVar(&input.LockTimeout, "lock-timeout", config.Getenv("RM_LOCK_TIMEOUT", "60s"), "SQL app lock timeout")
	flags.StringVar(&envFile, "env-file", config.Getenv("RM_ENV_FILE", ""), "optional env file with RM_* values")
	_ = envFile
	flags.StringVar(&input.PlanFile, "plan-file", config.Getenv("RM_PLAN_FILE", ""), "approved migration plan file")
	flags.StringVar(&input.BaselineUpTo, "up-to", config.Getenv("RM_BASELINE_UP_TO", ""), "baseline up to version")
	flags.StringVar(&input.RepairScript, "script", config.Getenv("RM_REPAIR_SCRIPT", ""), "script to repair checksum for")
	flags.BoolVar(&input.Confirm, "confirm", config.GetenvBool("RM_CONFIRM", false), "confirm destructive command")
	flags.BoolVar(&input.SkipValidate, "skip-validate", config.GetenvBool("RM_SKIP_VALIDATE", false), "skip validation after migrate")

	if err := flags.Parse(args); err != nil {
		return config.Config{}, err
	}
	cfg, err := config.Load(input)
	if err != nil {
		return config.Config{}, err
	}
	if err := cfg.ValidateForCommand(command); err != nil {
		return config.Config{}, err
	}
	if command == "migrate" && cfg.PlanFile == "" {
		return config.Config{}, fmt.Errorf("--plan-file is required")
	}
	return cfg, nil
}

func exitCode(err error, log logger.Logger, event string) int {
	if err == nil {
		return contracts.ExitOK
	}
	log.Error(event, err.Error())

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
	fmt.Fprintln(writer, "  rmig plan --env prod")
	fmt.Fprintln(writer, "  optional: --env-file path/to/.env or RM_ENV_FILE=path/to/.env")
	fmt.Fprintln(writer, "  rmig migrate --env prod --plan-file reports/migration-plan.json")
	fmt.Fprintln(writer, "  rmig validate --env prod")
	fmt.Fprintln(writer, "  rmig baseline --env prod --up-to V010 --confirm")
	fmt.Fprintln(writer, "  rmig repair-checksum --env prod --script R002__views.sql --confirm")
}

func writerOrDefault(writer io.Writer, fallback io.Writer) io.Writer {
	if writer == nil {
		return fallback
	}
	return writer
}
