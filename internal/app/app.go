package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"reporting-db-migrations/internal/commands"
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

	spec, ok := commands.Lookup(command)
	if ok {
		return r.dispatch(spec, args[2:], stdout, stderr)
	}
	outcome := failures.EvaluateWithCause(config.Config{}, "config_failed", contracts.ErrInvalidInput, fmt.Errorf("unknown command: %s", command))
	fmt.Fprintln(stderr, outcome.Failure.Error)
	printUsage(stdout)
	return outcome.ExitCode
}

func (r Runtime) dispatch(spec commands.Spec, args []string, stdout io.Writer, stderr io.Writer) int {
	cfg, err := parseCommandConfig(spec.Name, args)
	if err != nil {
		outcome := failures.Evaluate(config.Config{}, "config_failed", err)
		fmt.Fprintln(stderr, outcome.Failure.Error)
		return outcome.ExitCode
	}
	cfg.ToolVersion = r.BuildInfo.Version
	cfg.ToolCommit = r.BuildInfo.Commit

	logWriter := stdout
	if spec.Name == contracts.CommandPlan && cfg.PlanJSON {
		logWriter = stderr
	}
	log := logger.New(logger.Options{JSON: cfg.JSONLogs, Level: cfg.LogLevel, Writer: logWriter})
	ctx, cancel := context.WithTimeout(context.Background(), cfg.CommandTimeout)
	defer cancel()

	handler := r.Handler
	if handler == nil {
		handler = migrator.Handler{}
	}

	runner, ok := commandRunners[spec.Name]
	if !ok {
		return contracts.ExitConfigError
	}
	result, err := runner(ctx, handler, cfg, log, stdout)
	if err != nil {
		event := spec.FailureEvent
		var carrier failureEventCarrier
		if errors.As(err, &carrier) {
			event = carrier.FailureEvent()
		}
		outcome := failures.Evaluate(cfg, event, err)
		log.Error(event, outcome.Failure.Error)
		return outcome.ExitCode
	}
	if result.plan != nil && result.plan.Blocked {
		outcome := failures.EvaluatePlanBlocked(cfg, *result.plan)
		log.Error(spec.FailureEvent, outcome.Failure.Error)
		return outcome.ExitCode
	}
	return contracts.ExitOK
}

func parseCommandConfig(command string, args []string) (config.Config, error) {
	restoreEnv, err := applyEnvironmentFile(resolveEnvFilePath(args))
	if err != nil {
		return config.Config{}, contracts.Wrap(contracts.ErrInvalidInput, err)
	}
	defer restoreEnv()

	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	input := config.Input{}
	config.BindFlags(flags, &input)

	if err := flags.Parse(args); err != nil {
		return config.Config{}, contracts.Wrap(contracts.ErrInvalidInput, err)
	}
	cfg, err := config.Load(input)
	if err != nil {
		return config.Config{}, err
	}
	if err := cfg.ValidateForCommand(command); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "Usage:")
	fmt.Fprintln(writer, "  rmig version")
	fmt.Fprintln(writer, "  env values: pred, prod")
	fmt.Fprintln(writer, "  optional: --env-file path/to/.env or RM_ENV_FILE=path/to/.env")
	for _, spec := range commands.Specs() {
		fmt.Fprintf(writer, "  rmig %s\n", spec.Usage)
	}
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
