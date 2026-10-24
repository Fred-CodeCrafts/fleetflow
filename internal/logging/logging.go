// Package logging builds the process's logger: one place that decides the format,
// the level and the attributes every line carries.
//
// It exists because the choice was previously made in two places at once. Request
// lines went through a *slog.Logger while startup lines went through the standard
// log package, so one process emitted two formats and no level could turn either
// down. A deployment reading its own logs should see one shape.
//
// The logger is slog from the standard library. That is a deliberate choice rather
// than a default: it is structured, it costs no dependency, it is the same logger
// the net/http and database ecosystems now emit through, and slog.Handler is an
// interface, so a different backend is a constructor here and nothing else. A
// third-party logger would buy speed this program does not need at the price of a
// dependency it would have to justify.
package logging

import (
	"io"
	"log/slog"

	"fleetflow/internal/config"
)

// Format is the shape of a log line.
type Format string

const (
	// FormatText is for a human reading a terminal: one readable line each.
	FormatText Format = "text"
	// FormatJSON is for a machine reading a log stream, which is what Cloud Run
	// does: it parses the level out of the line and groups by the attributes.
	FormatJSON Format = "json"
)

// Options is everything the logger needs. The zero value is a development logger
// at info, which is the right thing for a test that forgot to say otherwise.
type Options struct {
	// Level is one of config.LogDebug..config.LogError. Anything else is treated
	// as info: a log level is not worth refusing to start over, and starting
	// loudly is better than not starting.
	Level string
	// Format is the shape of a line.
	Format Format
	// Env is the deployment the process believes it is in. It goes on every line,
	// so a log stream that collects several deployments can still be read.
	Env string
}

// New returns a logger writing to w.
func New(w io.Writer, opts Options) *slog.Logger {
	handlerOpts := &slog.HandlerOptions{Level: levelOf(opts.Level)}

	var handler slog.Handler
	switch opts.Format {
	case FormatJSON:
		handler = slog.NewJSONHandler(w, handlerOpts)
	default:
		handler = slog.NewTextHandler(w, handlerOpts)
	}

	logger := slog.New(handler)
	if opts.Env != "" {
		logger = logger.With(slog.String("env", opts.Env))
	}
	return logger
}

// FormatFor returns the format a deployment of env should use. Production logs are
// collected by a machine, so they are JSON; everywhere else a person is watching a
// terminal and a readable line is worth more than a parseable one.
func FormatFor(env string) Format {
	if env == config.EnvProduction {
		return FormatJSON
	}
	return FormatText
}

// levelOf maps the configuration names onto slog levels. An unknown name falls back
// to info rather than to the zero value, which is debug: a typo in LOG_LEVEL must
// not quietly turn on every line the program can produce.
func levelOf(name string) slog.Level {
	switch name {
	case config.LogDebug:
		return slog.LevelDebug
	case config.LogWarn:
		return slog.LevelWarn
	case config.LogError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
