// Package slogx wraps log/slog with convenience helpers so callers only
// need a single import for structured logging.
//
// It re-exports the most commonly used slog types and functions, adds
// context-based logger passing, format configuration (JSON, pretty, text),
// and a small set of attribute helpers.
package slogx

import (
	"bytes"
	"context"
	"encoding"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

// Level is a named slog level that implements flag.Value and
// encoding.TextMarshaler / encoding.TextUnmarshaler, mirroring how Format
// works so callers can register it directly with flag parsing libraries.
//
// It also implements slog.Leveler so it can be passed to
// slog.HandlerOptions.Level and similar interfaces.
type Level slog.Level

var (
	_ encoding.TextUnmarshaler = (*Level)(nil)
	_ encoding.TextMarshaler   = Level(0)
	_ slog.Leveler             = Level(0)
)

// Level implements slog.Leveler.
func (l Level) Level() slog.Level { return slog.Level(l) }

// MarshalText emits the lowercase level name ("info", "warn+2") to match how
// Format renders and to keep flag defaults looking like the values callers
// type.
func (l Level) MarshalText() ([]byte, error) {
	b, err := slog.Level(l).MarshalText()
	if err != nil {
		return nil, err
	}
	return bytes.ToLower(b), nil
}

// UnmarshalText accepts any casing ("info", "INFO", "Info"), including the
// offset forms slog supports such as "warn+2".
func (l *Level) UnmarshalText(text []byte) error {
	var sl slog.Level
	if err := sl.UnmarshalText(bytes.ToUpper(text)); err != nil {
		return err
	}
	*l = Level(sl)
	return nil
}

// String and Set make Level a flag.Value; both delegate to MarshalText /
// UnmarshalText so the accepted values and output are consistent.
func (l Level) String() string {
	b, _ := l.MarshalText()
	return string(b)
}

func (l *Level) Set(s string) error { return l.UnmarshalText([]byte(s)) }

// Re-export slog levels so callers don't need to import both packages.
const (
	LevelDebug Level = Level(slog.LevelDebug)
	LevelInfo  Level = Level(slog.LevelInfo)
	LevelWarn  Level = Level(slog.LevelWarn)
	LevelError Level = Level(slog.LevelError)
)

// Re-export common slog attribute constructors.
var (
	String   = slog.String
	Int      = slog.Int
	Int64    = slog.Int64
	Bool     = slog.Bool
	Float64  = slog.Float64
	Duration = slog.Duration
	Time     = slog.Time
	Any      = slog.Any
	Group    = slog.Group
)

// Err creates an slog.Attr for an error value, keyed as "err".
func Err(err error) slog.Attr {
	return slog.Any("err", err)
}

// Format selects the log output format.
type Format int

const (
	FormatJSON Format = iota
	FormatPretty
	FormatText
)

var (
	_ encoding.TextUnmarshaler = (*Format)(nil)
	_ encoding.TextMarshaler   = Format(0)
)

// String and Set make Format a flag.Value; both delegate to MarshalText /
// UnmarshalText so the accepted values and output are consistent.
func (f Format) String() string {
	b, _ := f.MarshalText()
	return string(b)
}

func (f *Format) Set(s string) error { return f.UnmarshalText([]byte(s)) }

func (f *Format) UnmarshalText(text []byte) error {
	switch string(bytes.ToLower(text)) {
	case "json":
		*f = FormatJSON
	case "pretty":
		*f = FormatPretty
	case "text":
		*f = FormatText
	default:
		return fmt.Errorf("unknown log format %q", text)
	}
	return nil
}

func (f Format) MarshalText() ([]byte, error) {
	switch f {
	case FormatJSON:
		return []byte("json"), nil
	case FormatPretty:
		return []byte("pretty"), nil
	case FormatText:
		return []byte("text"), nil
	default:
		return nil, fmt.Errorf("unknown log format %d", f)
	}
}

// Logger is a type alias for slog.Logger so callers that already import slogx
// for Level/Format don't need a second import just for the pointer type.
type Logger = slog.Logger

// New creates a logger for the given format and level. It also sets the slog
// default.
func New(format Format, level Level) *Logger {
	var handler slog.Handler

	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	}

	switch format {
	case FormatPretty:
		handler = tint.NewHandler(os.Stdout, &tint.Options{
			AddSource:  true,
			Level:      level,
			TimeFormat: time.Kitchen,
		})
	case FormatText:
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

type contextKey string

const loggerKey contextKey = "slogx_logger"

// FromContext retrieves a logger from the context. Returns slog.Default()
// if none is set.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

// WithLogger attaches a logger to a context.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}
