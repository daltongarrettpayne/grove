// Package log configures grove's global structured logger.
// All application code should call slog.Info / slog.Debug / slog.Error directly
// after Setup has run. This package only exists to wire up the initial handler.
package log

import (
	"context"
	"io"
	"log/slog"
	"os"
)

// Setup installs a text-format handler on the global slog logger at the
// requested level. Call once in the CLI's PersistentPreRunE.
//
// Logs go to stderr so they don't pollute stdout, which is reserved for
// machine-readable output (JSON, TSV picker rows, status segments).
//
// If fileOut is non-nil, a JSON handler writing to that writer is added
// alongside the stderr handler via a multiHandler. The file handler always
// runs at LevelDebug regardless of the requested level, so every invocation
// is fully captured in the log file.
func Setup(level string, fileOut io.Writer) {
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

	stderrHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})

	if fileOut == nil {
		slog.SetDefault(slog.New(stderrHandler))
		return
	}

	fileHandler := slog.NewJSONHandler(fileOut, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(multiHandler{stderrHandler, fileHandler}))
}

// multiHandler fans out a single log record to multiple slog.Handler instances.
type multiHandler []slog.Handler

func (m multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	// Accept the record if any handler would process it. Each handler gates
	// on its own minimum level inside Handle, so we take the union here.
	for _, h := range m {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m {
		if h.Enabled(ctx, r.Level) {
			_ = h.Handle(ctx, r)
		}
	}
	return nil
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(multiHandler, len(m))
	for i, h := range m {
		out[i] = h.WithAttrs(attrs)
	}
	return out
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	out := make(multiHandler, len(m))
	for i, h := range m {
		out[i] = h.WithGroup(name)
	}
	return out
}
