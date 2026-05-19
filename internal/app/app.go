package app

import (
	"context"
	"fmt"
	"os"

	"reporting-db-migrations/internal/buildinfo"
	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/errors"
	"reporting-db-migrations/internal/log"
	"reporting-db-migrations/internal/types"
)

type Connector func(ctx context.Context, cfg types.Config) (driver.Conn, error)

func Run(args []string, connect Connector) int {
	return runWithLookup(args, osEnvLookup, connect)
}

func runWithLookup(args []string, lookup envLookupFn, connect Connector) int {
	ctx := context.Background()

	flags, err := parseFlags(args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "rmig: %v\n", err)
		return types.ExitInvalidInput
	}

	if flags.Command == "version" {
		if flags.JSON {
			if err := buildinfo.WriteJSON(os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "rmig: version: %v\n", err)
				return types.ExitInvalidInput
			}
			return 0
		}
		fmt.Fprintln(os.Stdout, buildinfo.Summary())
		return 0
	}

	envFile := flags.EnvFile
	if envFile == "" {
		envFile = ".env"
	}

	env, err := loadEnvFile(envFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rmig: %v\n", err)
		return types.ExitConfigError
	}

	cfg := buildConfig(flags, env, lookup)

	if err := validateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "rmig: %v\n", err)
		return types.ExitConfigError
	}

	logger := log.New(cfg.JSONLogs, cfg.LogLevel, os.Stderr)

	conn, err := connect(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rmig: cannot connect to %s: %v\n", cfg.Server, err)
		return errors.ExitCode(err)
	}
	defer conn.Close()

	b := bus.New()
	auditSub := attachSubscribers(b, conn, cfg, logger)

	eng := wireEngine(b, conn, cfg, logger)
	eng.SetBootstrapChecker(auditSub)

	var execErr error
	switch flags.Command {
	case "plan":
		execErr = eng.Plan(ctx)
	case "migrate":
		execErr = eng.Migrate(ctx)
	case "validate":
		execErr = eng.Validate(ctx)
	case "baseline":
		execErr = eng.Baseline(ctx)
	case "repair-checksum":
		execErr = eng.RepairChecksum(ctx)
	}

	if execErr != nil {
		logger.Error(flags.Command, execErr.Error())
		return errors.ExitCode(execErr)
	}

	logger.Info(flags.Command, "completed successfully")
	return 0
}
