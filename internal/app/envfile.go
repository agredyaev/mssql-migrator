package app

import (
	"fmt"
	"os"
	"strings"
)

type envState struct {
	value   string
	existed bool
}

func resolveEnvFilePath(args []string) string {
	path := strings.TrimSpace(os.Getenv("RM_ENV_FILE"))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--env-file" && i+1 < len(args):
			path = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--env-file="):
			path = strings.TrimSpace(strings.TrimPrefix(arg, "--env-file="))
		}
	}
	return path
}

func applyEnvironmentFile(path string) (func(), error) {
	if strings.TrimSpace(path) == "" {
		return func() {}, nil
	}

	values, err := loadEnvironmentFile(path)
	if err != nil {
		return nil, err
	}

	previous := map[string]envState{}
	applied := make([]string, 0, len(values))
	for key, value := range values {
		current, existed := os.LookupEnv(key)
		if existed {
			continue
		}
		previous[key] = envState{value: current, existed: existed}
		if err := os.Setenv(key, value); err != nil {
			restoreEnvironment(previous, applied)
			return nil, err
		}
		applied = append(applied, key)
	}

	return func() {
		restoreEnvironment(previous, applied)
	}, nil
}

func restoreEnvironment(previous map[string]envState, keys []string) {
	for _, key := range keys {
		state := previous[key]
		if state.existed {
			_ = os.Setenv(key, state.value)
			continue
		}
		_ = os.Unsetenv(key)
	}
}

func loadEnvironmentFile(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) > 0 {
		lines[0] = strings.TrimPrefix(lines[0], "\ufeff")
	}

	values := make(map[string]string)
	for index, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid env file %s line %d: expected KEY=VALUE", path, index+1)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("invalid env file %s line %d: empty key", path, index+1)
		}
		parsedValue, err := parseEnvironmentValue(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid env file %s line %d: %v", path, index+1, err)
		}
		values[key] = parsedValue
	}
	return values, nil
}

func parseEnvironmentValue(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if len(value) == 1 && (value[0] == '\'' || value[0] == '"') {
		return "", fmt.Errorf("unterminated quoted value")
	}
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1], nil
		}
		if value[0] == '"' || value[0] == '\'' {
			return "", fmt.Errorf("unterminated quoted value")
		}
	}
	return value, nil
}
