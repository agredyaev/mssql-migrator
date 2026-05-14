package app

import (
	"fmt"
	"strings"
)

type cliFlags struct {
	Command string
	EnvFile string
	JSON    bool
}

func parseFlags(args []string) (cliFlags, error) {
	if len(args) == 0 {
		return cliFlags{}, fmt.Errorf("missing command; expected one of: plan, migrate, validate, baseline, repair-checksum")
	}

	var flags cliFlags

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--env":
			i++
			if i >= len(args) {
				return cliFlags{}, fmt.Errorf("--env requires a path argument")
			}
			flags.EnvFile = args[i]
		case "--json":
			flags.JSON = true
		case "--help", "-h":
			return cliFlags{}, fmt.Errorf("show help:\n%s", usageText)
		default:
			if strings.HasPrefix(arg, "-") {
				return cliFlags{}, fmt.Errorf("unknown flag: %s\n%s", arg, usageText)
			}
			if flags.Command != "" {
				return cliFlags{}, fmt.Errorf("unexpected argument after command %q: %s", flags.Command, arg)
			}
			flags.Command = arg
		}
	}

	if flags.Command == "" {
		return cliFlags{}, fmt.Errorf("missing command; expected one of: plan, migrate, validate, baseline, repair-checksum\n%s", usageText)
	}

	switch flags.Command {
	case "plan", "migrate", "validate", "baseline", "repair-checksum":
	default:
		return cliFlags{}, fmt.Errorf("unknown command: %s\n%s", flags.Command, usageText)
	}

	return flags, nil
}

const usageText = `Usage: rmig [--env <path>] [--json] <command>

Commands:
  plan              Scan, inspect, compute diff, write plan report
  migrate           Full migration (plan + scaffold + apply)
  validate          Validate all checks
  baseline          Mark all objects as adopted (baseline)
  repair-checksum   Repair checksum mismatches

Flags:
  --env <path>      Path to .env file (default: .env)
  --json            Enable JSON log output
  -h, --help        Show this help`
