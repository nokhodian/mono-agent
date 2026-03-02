package util

import (
	"io"
	"os"

	"github.com/rs/zerolog"
)

// NewLogger creates a zerolog.Logger that writes to stderr (with pretty console
// output) and optionally to a log file. When verbose is true the level is set
// to Debug; otherwise Info.
func NewLogger(verbose bool, logFile string) zerolog.Logger {
	var writers []io.Writer

	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "15:04:05",
	}
	writers = append(writers, consoleWriter)

	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			writers = append(writers, f)
		}
	}

	level := zerolog.InfoLevel
	if verbose {
		level = zerolog.DebugLevel
	}

	multi := zerolog.MultiLevelWriter(writers...)
	return zerolog.New(multi).Level(level).With().Timestamp().Logger()
}
