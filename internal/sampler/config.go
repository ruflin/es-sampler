package sampler

import (
	"errors"
	"time"
)

// ErrHelpRequested is returned by ParseConfig when the user passed --help/-h.
var ErrHelpRequested = errors.New("help requested")

// Config holds the resolved runtime configuration for a sync run.
type Config struct {
	SourceHost    string
	SourceAPIKey  string
	DestHost      string
	DestAPIKey    string
	DestUsername  string
	DestPassword  string
	IndexPattern  string
	Size          int
	Interval      time.Duration
	Lookback      time.Duration
	RandomSeed    *int64
	TargetIndex   string
	BatchSize     int
	NoVerifyCerts bool
	Verbose       bool

	// RunSeed is populated by Run when RandomSeed is nil so every cycle gets a
	// stable but non-deterministic seed based on the run start.
	RunSeed int64
}
