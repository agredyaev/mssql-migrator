package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

type Options struct {
	JSON   bool
	Level  string
	Writer io.Writer
}

type Logger struct {
	json   bool
	level  Level
	writer io.Writer
	mu     *sync.Mutex
}

func New(options Options) Logger {
	writer := options.Writer
	if writer == nil {
		writer = os.Stdout
	}
	return Logger{json: options.JSON, level: normalizeLevel(options.Level), writer: writer, mu: &sync.Mutex{}}
}

func (l *Logger) Debug(event, message string) { l.write(LevelDebug, event, message) }
func (l *Logger) Info(event, message string)  { l.write(LevelInfo, event, message) }
func (l *Logger) Warn(event, message string)  { l.write(LevelWarn, event, message) }
func (l *Logger) Error(event, message string) { l.write(LevelError, event, message) }

func (l *Logger) write(level Level, event, message string) {
	if !l.enabled(level) {
		return
	}
	message = Redact(message)
	if l.mu == nil {
		l.mu = &sync.Mutex{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.json {
		payload, _ := json.Marshal(map[string]string{
			"time":    time.Now().UTC().Format(time.RFC3339),
			"level":   string(level),
			"event":   event,
			"message": message,
		})
		fmt.Fprintln(l.writer, string(payload))
		return
	}
	fmt.Fprintf(l.writer, "%s %s: %s\n", strings.ToUpper(string(level)), event, message)
}

func (l *Logger) enabled(level Level) bool {
	order := map[Level]int{LevelDebug: 10, LevelInfo: 20, LevelWarn: 30, LevelError: 40}
	return order[level] >= order[l.level]
}

func normalizeLevel(value string) Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password\s*=\s*)("[^"]*"|'[^']*'|[^;\r\n]+)`),
	regexp.MustCompile(`(?i)(pwd\s*=\s*)("[^"]*"|'[^']*'|[^;\r\n]+)`),
	regexp.MustCompile(`(?i)(token\s*=\s*)("[^"]*"|'[^']*'|[^;\r\n]+)`),
	regexp.MustCompile(`(?i)(access[_-]?token\s*=\s*)("[^"]*"|'[^']*'|[^;\r\n]+)`),
	regexp.MustCompile(`(?i)(secret\s*=\s*)("[^"]*"|'[^']*'|[^;\r\n]+)`),
	regexp.MustCompile(`(?i)(client_secret\s*=\s*)("[^"]*"|'[^']*'|[^;\r\n]+)`),
	regexp.MustCompile(`(?i)([?&][^=&\s]*(?:token|secret|sig|signature)[^=&\s]*=)[^&\s]+`),
}

var sqlServerURLSecretPattern = regexp.MustCompile(`sqlserver://([^:]+):([^@]+)@`)

func Redact(value string) string {
	result := value
	for _, pattern := range secretPatterns {
		result = pattern.ReplaceAllString(result, `${1}***`)
	}
	result = sqlServerURLSecretPattern.ReplaceAllString(result, `sqlserver://$1:***@`)
	return result
}
