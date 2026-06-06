package logger

import (
	"os"
	"strings"

	"log/slog"
)

// levelVar backs every handler so the level can be set once at startup (and, in
// principle, changed at runtime) via SetLevel. It defaults to slog.LevelInfo.
var levelVar = new(slog.LevelVar)

var (
	stdoutHandler           = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: levelVar})
	stderrHandler           = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: levelVar})
	stderrHandlerWithSource = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: levelVar, AddSource: true})

	// Stdout sends logs to stdout.
	Stdout = slog.New(stdoutHandler)
	// Stderr sends logs to stderr.
	Stderr = slog.New(stderrHandler)
	// StderrWithSource sends logs to stderr with source location attached.
	StderrWithSource = slog.New(stderrHandlerWithSource)
)

// SetLevel sets the global log level from a string (debug/info/warn/error).
// Unrecognized or empty values leave the level at its default (info).
func SetLevel(level string) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		levelVar.Set(slog.LevelDebug)
	case "warn", "warning":
		levelVar.Set(slog.LevelWarn)
	case "error":
		levelVar.Set(slog.LevelError)
	case "", "info":
		levelVar.Set(slog.LevelInfo)
	default:
		levelVar.Set(slog.LevelInfo)
	}
}
