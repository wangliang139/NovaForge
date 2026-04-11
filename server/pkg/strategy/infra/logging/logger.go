package logging

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
)

// Logger is a minimal pluggable logger for strategy console output.
//
// This is intentionally Printf-style to make it easy to adapt existing loggers
// (stdlib log.Logger, file logger, etc).
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

type nopLogger struct{}

func (nopLogger) Debugf(string, ...any) {}
func (nopLogger) Infof(string, ...any)  {}
func (nopLogger) Warnf(string, ...any)  {}
func (nopLogger) Errorf(string, ...any) {}

// Nop returns a logger that discards all logs.
func Nop() Logger { return nopLogger{} }

type stdLoggerAdapter struct {
	l *log.Logger
}

func (a stdLoggerAdapter) Debugf(format string, args ...any) {
	a.l.Printf("[DEBUG] "+format, args...)
}

func (a stdLoggerAdapter) Infof(format string, args ...any) {
	a.l.Printf("[INFO] "+format, args...)
}

func (a stdLoggerAdapter) Warnf(format string, args ...any) {
	a.l.Printf("[WARN] "+format, args...)
}

func (a stdLoggerAdapter) Errorf(format string, args ...any) {
	a.l.Printf("[ERROR] "+format, args...)
}

// FromStdLogger adapts a stdlib *log.Logger to Logger.
//
// If l is nil, it returns a Nop logger.
func NewStdLogger(l *log.Logger) Logger {
	if l == nil {
		return Nop()
	}
	return stdLoggerAdapter{l: l}
}

func NewZeroLogger(opts ...LoggerOption) Logger {
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	zerolog.TimeFieldFormat = time.RFC3339Nano

	cfg := &loggerConfig{
		level: "debug",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	var writer io.Writer
	writer = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	if cfg.writeToFile {
		fileWriter, err := os.OpenFile(cfg.fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			// 文件打开失败，只使用控制台输出，记录警告
			zlog.Warn().Err(err).Str("file", cfg.fileName).Msg("failed to open log file, using console only")
		} else {
			writer = io.MultiWriter(writer, fileWriter)
		}
	}
	logger := zlog.With().Caller().Logger()
	logger = logger.Output(writer)
	logger = logger.Level(zerolog.DebugLevel)
	if cfg.level != "" {
		level, err := zerolog.ParseLevel(cfg.level)
		if err != nil {
			level = zerolog.DebugLevel
		}
		logger = logger.Level(level)
	}
	if cfg.module != "" {
		logger = logger.With().Str("module", cfg.module).Logger()
	}
	return &zerologAdapter{l: &logger}
}

type zerologAdapter struct {
	l *zerolog.Logger
}

func (a zerologAdapter) Debugf(format string, args ...any) {
	a.l.Debug().Msgf(format, args...)
}

func (a zerologAdapter) Infof(format string, args ...any) {
	a.l.Info().Msgf(format, args...)
}

func (a zerologAdapter) Warnf(format string, args ...any) {
	a.l.Warn().Msgf(format, args...)
}

func (a zerologAdapter) Errorf(format string, args ...any) {
	a.l.Error().Msgf(format, args...)
}

func NewCombinedLogger(loggers ...Logger) Logger {
	if len(loggers) == 0 {
		return Nop()
	}
	return &combinedLoggerAdapter{loggers: loggers}
}

type combinedLoggerAdapter struct {
	loggers []Logger
}

func (a combinedLoggerAdapter) Debugf(format string, args ...any) {
	for _, logger := range a.loggers {
		logger.Debugf(format, args...)
	}
}

func (a combinedLoggerAdapter) Infof(format string, args ...any) {
	for _, logger := range a.loggers {
		logger.Infof(format, args...)
	}
}

func (a combinedLoggerAdapter) Warnf(format string, args ...any) {
	for _, logger := range a.loggers {
		logger.Warnf(format, args...)
	}
}

func (a combinedLoggerAdapter) Errorf(format string, args ...any) {
	for _, logger := range a.loggers {
		logger.Errorf(format, args...)
	}
}

type sinkLoggerAdapter struct {
	storage Storage
	timeFn  func() time.Time
}

func NewSinkLogger(storage Storage, timeFn func() time.Time) Logger {
	if timeFn == nil {
		timeFn = time.Now
	}
	return &sinkLoggerAdapter{storage: storage, timeFn: timeFn}
}

func (a sinkLoggerAdapter) Debugf(format string, args ...any) {
	a.write("debug", format, args...)
}

func (a sinkLoggerAdapter) Infof(format string, args ...any) {
	a.write("info", format, args...)
}

func (a sinkLoggerAdapter) Warnf(format string, args ...any) {
	a.write("warn", format, args...)
}

func (a sinkLoggerAdapter) Errorf(format string, args ...any) {
	a.write("error", format, args...)
}

func (a sinkLoggerAdapter) write(level string, format string, args ...any) {
	a.storage.Write(context.Background(), Entry{
		Ts:      a.timeFn(),
		Level:   level,
		Message: fmt.Sprintf(format, args...),
	})
}

type LoggerOption func(*loggerConfig)

type loggerConfig struct {
	level string

	module string

	writeToFile bool
	fileName    string
}

func WithLevel(level string) LoggerOption {
	return func(opt *loggerConfig) {
		opt.level = level
	}
}

func WithModule(module string) LoggerOption {
	return func(opt *loggerConfig) {
		opt.module = module
	}
}

func WithWriteToFile(fileName string) LoggerOption {
	return func(opt *loggerConfig) {
		opt.writeToFile = true
		opt.fileName = fileName
	}
}
