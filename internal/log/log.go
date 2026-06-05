// Package log configures grove's global structured logger.
// All application code should call slog.Info / slog.Debug / slog.Error directly
// after Setup has run. This package only exists to wire up the initial handler.
package log

import (
	"log/slog"
	"os"
)

// Setup installs a text-format handler on the global slog logger at the
// requested level. Call once in the CLI's PersistentPreRunE.
//
// Logs go to stderr so they don't pollute stdout, which is reserved for
// machine-readable output (JSON, TSV picker rows, status segments).
func Setup(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(h))
}
