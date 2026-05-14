package app

import (
	"context"
	"fmt"
	"os"

	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/errors"
	"reporting-db-migrations/internal/log"
	"reporting-db-migrations/internal/types"
)

func Run(args []string) int {
	return runWithLookup(args, osEnvLookup)
}

func runWithLookup(args []string, lookup envLookupFn) int {
	ctx := context.Background()

	flags, err := parseFlags(args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "rmig: %v\n", err)
		return types.ExitInvalidInput
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

	conn, err := openConn(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rmig: %v\n", err)
		return errors.ExitCode(err)
	}
	defer conn.Close()

	b := bus.New()
	attachSubscribers(b, conn, cfg, logger)

	eng, err := wireEngine(b, conn, cfg, logger)
	if err != nil {
		logger.Error("init", err.Error())
		return errors.ExitCode(err)
	}

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
