package sampler

import (
	"fmt"
	"time"
)

// Logger is the minimal logging interface used throughout the package.
// Implementations must be safe to call with a single format-free message.
type Logger func(msg string)

// NewLogger returns a Logger. When verbose, each line is prefixed with an
// ISO-8601 UTC timestamp in `[<iso>] message` form.
func NewLogger(verbose bool) Logger {
	if verbose {
		return func(msg string) {
			now := time.Now().UTC()
			ts := now.Format("2006-01-02T15:04:05.000Z")
			fmt.Printf("[%s] %s\n", ts, msg)
		}
	}
	return func(msg string) { fmt.Println(msg) }
}

// Logf is a tiny printf helper on top of Logger.
func (l Logger) Logf(format string, args ...any) {
	l(fmt.Sprintf(format, args...))
}
