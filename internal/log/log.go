package log

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

type Logger struct {
	json   bool
	level  Level
	writer io.Writer
	mu     sync.Mutex
}

var levelRanks = map[Level]int{
	LevelDebug: 0,
	LevelInfo:  1,
	LevelWarn:  2,
	LevelError: 3,
}

func New(jsonOutput bool, level string, w io.Writer) *Logger {
	if w == nil {
		w = os.Stderr
	}
	return &Logger{
		json:   jsonOutput,
		level:  normalizeLevel(level),
		writer: w,
	}
}

func (l *Logger) Debug(event, message string) { l.write(LevelDebug, event, message) }
func (l *Logger) Info(event, message string)  { l.write(LevelInfo, event, message) }
func (l *Logger) Warn(event, message string)  { l.write(LevelWarn, event, message) }
func (l *Logger) Error(event, message string) { l.write(LevelError, event, message) }

func (l *Logger) ErrorEnvelope(event, envelope string) {
	l.write(LevelError, event, envelope)
}

func (l *Logger) write(level Level, event, message string) {
	if !l.shouldLog(level) {
		return
	}
	message = Redact(message)
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.json {
		entry := map[string]string{
			"time":    time.Now().UTC().Format(time.RFC3339),
			"level":   string(level),
			"event":   event,
			"message": message,
		}
		data, err := json.Marshal(entry)
		if err != nil {
			fmt.Fprintf(l.writer, `{"error":"marshal failed: %s"}`+"\n", err.Error())
			return
		}
		fmt.Fprintln(l.writer, string(data))
		return
	}

	fmt.Fprintf(l.writer, "%s %-5s %s %s\n",
		time.Now().UTC().Format(time.RFC3339),
		strings.ToUpper(string(level)),
		eventPath(event),
		message,
	)
}

func (l *Logger) shouldLog(level Level) bool {
	return levelRanks[level] >= levelRanks[l.level]
}

func normalizeLevel(level string) Level {
	switch Level(strings.ToLower(level)) {
	case LevelDebug, LevelInfo, LevelWarn, LevelError:
		return Level(strings.ToLower(level))
	default:
		return LevelInfo
	}
}

func eventPath(event string) string {
	event = Redact(event)
	if len(event) <= 48 {
		return event
	}
	runes := []rune(event)
	if len(runes) > 48 {
		return string(runes[:48])
	}
	return event
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

var jsonSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)("(?:password|pwd|token|access[_-]?token|secret|client_secret)"\s*:\s*)"[^"]*"`),
}

var sqlServerURLSecretPattern = regexp.MustCompile(`sqlserver://([^:]+):([^@]+)@`)

var hasSecret = regexp.MustCompile(`(?i)(password|pwd|token|secret|client_secret|sig|signature|sqlserver://)`)

func Redact(value string) string {
	if !hasSecret.MatchString(value) {
		return value
	}
	result := value
	for _, pattern := range secretPatterns {
		result = pattern.ReplaceAllString(result, `${1}***`)
	}
	for _, pattern := range jsonSecretPatterns {
		result = pattern.ReplaceAllString(result, `${1}"***"`)
	}
	result = sqlServerURLSecretPattern.ReplaceAllString(result, `sqlserver://$1:***@`)
	return result
}
