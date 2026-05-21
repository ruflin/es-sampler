package sampler

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// HelpText describes the CLI surface.
const HelpText = `
es-sampler - Copy documents from source Elasticsearch to destination.

Usage:
  es-sampler [options]

Source (required):
  SOURCE_ELASTICSEARCH_HOST    or  --source-host       Source cluster URL
  SOURCE_ELASTICSEARCH_API_KEY or  --source-api-key    Source API key

Destination:
  Configured via env vars (defaults: http://localhost:9200, elastic, changeme).
  ELASTICSEARCH_HOST             Destination cluster URL
  ELASTICSEARCH_API_KEY  or  --dest-api-key            Destination API key (takes precedence over username/password)
  ELASTICSEARCH_USERNAME                               Destination username (default: elastic)
  ELASTICSEARCH_PASSWORD                               Destination password (default: changeme)

Set env vars in the shell (e.g. export SOURCE_ELASTICSEARCH_HOST=...) or pass inline; CLI flags override env.

Sync options:
  SYNC_INDEX_PATTERN           or  --index-pattern    (default: logs*)
  SYNC_SIZE                    or  --size             (default: 100)
  SYNC_INTERVAL_SECONDS        or  --interval         (default: 1)
  SYNC_LOOKBACK                or  --lookback         Go duration (e.g. 15m, 24h). Search window ends at now. (default: 24h)
  SYNC_RANDOM_SEED             or  --random-seed      Fixed seed for sampling (omit for run-based seed)
  SYNC_TARGET_INDEX            or  --target-index     Single target index/stream (default: preserve original index names)
  SYNC_BATCH_SIZE              or  --batch-size       (default: same as --size)
  SYNC_REQUEST_TIMEOUT         or  --request-timeout  Go duration for per-request timeout (default: 30s)

Other:
  --env-file                   Path to a dotenv file to load before parsing env vars (default: .env)
  --no-verify-certs            Disable TLS verification
  --verbose, -v                Verbose logging
  --help, -h                   This help
`

// rawFlags captures the raw CLI flag values (strings, pointer for presence).
type rawFlags struct {
	help           bool
	verbose        bool
	noVerifyCerts  bool
	sourceHost     *string
	sourceAPIKey   *string
	destAPIKey     *string
	indexPattern   *string
	size           *string
	interval       *string
	lookback       *string
	randomSeed     *string
	targetIndex    *string
	batchSize      *string
	requestTimeout *string
}

// parseArgs is a small custom parser supporting `--flag`, `--flag=value`,
// `--flag value`, and shorts `-h`, `-v`. Unknown flags produce an error.
func parseArgs(argv []string) (*rawFlags, error) {
	f := &rawFlags{}

	setString := func(target **string, raw string, hasEqualsValue bool, nextArg func() (string, bool)) error {
		if hasEqualsValue {
			v := raw
			*target = &v
			return nil
		}
		v, ok := nextArg()
		if !ok {
			return fmt.Errorf("missing value for flag")
		}
		*target = &v
		return nil
	}

	i := 0
	for i < len(argv) {
		arg := argv[i]
		i++
		next := func() (string, bool) {
			if i >= len(argv) {
				return "", false
			}
			v := argv[i]
			i++
			return v, true
		}

		var name, value string
		hasEq := false
		switch {
		case strings.HasPrefix(arg, "--"):
			name = arg[2:]
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				value = name[eq+1:]
				name = name[:eq]
				hasEq = true
			}
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			name = arg[1:]
		default:
			return nil, fmt.Errorf("unexpected positional argument: %q", arg)
		}

		switch name {
		case "help", "h":
			f.help = true
		case "verbose", "v":
			f.verbose = true
		case "no-verify-certs":
			f.noVerifyCerts = true
		case "source-host":
			if err := setString(&f.sourceHost, value, hasEq, next); err != nil {
				return nil, fmt.Errorf("--%s: %w", name, err)
			}
		case "source-api-key":
			if err := setString(&f.sourceAPIKey, value, hasEq, next); err != nil {
				return nil, fmt.Errorf("--%s: %w", name, err)
			}
		case "dest-api-key":
			if err := setString(&f.destAPIKey, value, hasEq, next); err != nil {
				return nil, fmt.Errorf("--%s: %w", name, err)
			}
		case "index-pattern":
			if err := setString(&f.indexPattern, value, hasEq, next); err != nil {
				return nil, fmt.Errorf("--%s: %w", name, err)
			}
		case "size":
			if err := setString(&f.size, value, hasEq, next); err != nil {
				return nil, fmt.Errorf("--%s: %w", name, err)
			}
		case "interval":
			if err := setString(&f.interval, value, hasEq, next); err != nil {
				return nil, fmt.Errorf("--%s: %w", name, err)
			}
		case "lookback":
			if err := setString(&f.lookback, value, hasEq, next); err != nil {
				return nil, fmt.Errorf("--%s: %w", name, err)
			}
		case "random-seed":
			if err := setString(&f.randomSeed, value, hasEq, next); err != nil {
				return nil, fmt.Errorf("--%s: %w", name, err)
			}
		case "target-index":
			if err := setString(&f.targetIndex, value, hasEq, next); err != nil {
				return nil, fmt.Errorf("--%s: %w", name, err)
			}
		case "batch-size":
			if err := setString(&f.batchSize, value, hasEq, next); err != nil {
				return nil, fmt.Errorf("--%s: %w", name, err)
			}
		case "request-timeout":
			if err := setString(&f.requestTimeout, value, hasEq, next); err != nil {
				return nil, fmt.Errorf("--%s: %w", name, err)
			}
		case "env-file":
			// Parsed here so it isn't reported as unknown; the value is
			// consumed in main.go before ParseConfig runs.
			var sink *string
			if err := setString(&sink, value, hasEq, next); err != nil {
				return nil, fmt.Errorf("--%s: %w", name, err)
			}
		default:
			return nil, fmt.Errorf("unknown flag: --%s", name)
		}
	}

	return f, nil
}

// envOr returns the CLI flag value when non-empty, then env var, then fallback.
func envOr(cliValue *string, envVar, fallback string) string {
	if cliValue != nil {
		if trimmed := strings.TrimSpace(*cliValue); trimmed != "" {
			return trimmed
		}
	}
	if env := strings.TrimSpace(os.Getenv(envVar)); env != "" {
		return env
	}
	return fallback
}

// ParseConfig resolves a Config from argv + env vars. If argv contains
// `--help`/`-h`, returns ErrHelpRequested without parsing further. Callers are
// responsible for printing HelpText.
func ParseConfig(argv []string) (*Config, error) {
	for _, a := range argv {
		if a == "--help" || a == "-h" {
			return nil, ErrHelpRequested
		}
	}

	raw, err := parseArgs(argv)
	if err != nil {
		return nil, err
	}
	if raw.help {
		return nil, ErrHelpRequested
	}

	sourceHost := envOr(raw.sourceHost, "SOURCE_ELASTICSEARCH_HOST", "")
	sourceAPIKey := envOr(raw.sourceAPIKey, "SOURCE_ELASTICSEARCH_API_KEY", "")
	indexPattern := envOr(raw.indexPattern, "SYNC_INDEX_PATTERN", "logs*")
	sizeStr := envOr(raw.size, "SYNC_SIZE", "100")
	intervalStr := envOr(raw.interval, "SYNC_INTERVAL_SECONDS", "1")
	lookbackStr := envOr(raw.lookback, "SYNC_LOOKBACK", "24h")
	randomSeedStr := envOr(raw.randomSeed, "SYNC_RANDOM_SEED", "")
	targetIndex := envOr(raw.targetIndex, "SYNC_TARGET_INDEX", "")
	// batch size defaults to SYNC_SIZE when neither flag nor env is set.
	batchSizeStr := envOr(raw.batchSize, "SYNC_BATCH_SIZE", sizeStr)
	requestTimeoutStr := envOr(raw.requestTimeout, "SYNC_REQUEST_TIMEOUT", "30s")

	destHost := envOr(nil, "ELASTICSEARCH_HOST", "http://localhost:9200")
	destAPIKey := envOr(raw.destAPIKey, "ELASTICSEARCH_API_KEY", "")
	destUser := envOr(nil, "ELASTICSEARCH_USERNAME", "elastic")
	destPass := envOr(nil, "ELASTICSEARCH_PASSWORD", "changeme")

	if sourceHost == "" || sourceAPIKey == "" {
		return nil, fmt.Errorf("Error: SOURCE_ELASTICSEARCH_HOST and SOURCE_ELASTICSEARCH_API_KEY (or --source-host and --source-api-key) are required.")
	}

	size, err := strconv.Atoi(sizeStr)
	if err != nil || size < 1 {
		return nil, fmt.Errorf("Error: --size must be a positive integer.")
	}

	batchSize, err := strconv.Atoi(batchSizeStr)
	if err != nil || batchSize < 1 {
		return nil, fmt.Errorf("Error: --batch-size must be a positive integer.")
	}

	var randomSeed *int64
	if randomSeedStr != "" {
		n, err := strconv.ParseInt(randomSeedStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("Error: --random-seed must be an integer.")
		}
		randomSeed = &n
	}

	intervalSeconds, err := strconv.ParseFloat(intervalStr, 64)
	if err != nil {
		return nil, fmt.Errorf("Error: --interval must be a number.")
	}

	lookback, err := time.ParseDuration(lookbackStr)
	if err != nil || lookback <= 0 {
		return nil, fmt.Errorf("Error: --lookback must be a positive Go duration (e.g. 15m, 24h).")
	}

	requestTimeout, err := time.ParseDuration(requestTimeoutStr)
	if err != nil || requestTimeout <= 0 {
		return nil, fmt.Errorf("Error: --request-timeout must be a positive Go duration (e.g. 30s, 1m).")
	}

	cfg := &Config{
		SourceHost:     strings.TrimRight(sourceHost, "/"),
		SourceAPIKey:   sourceAPIKey,
		DestHost:       strings.TrimRight(destHost, "/"),
		DestAPIKey:     destAPIKey,
		DestUsername:   destUser,
		DestPassword:   destPass,
		IndexPattern:   indexPattern,
		Size:           size,
		Interval:       time.Duration(intervalSeconds * float64(time.Second)),
		Lookback:       lookback,
		RandomSeed:     randomSeed,
		TargetIndex:    targetIndex,
		BatchSize:      batchSize,
		RequestTimeout: requestTimeout,
		NoVerifyCerts:  raw.noVerifyCerts,
		Verbose:        raw.verbose,
	}
	return cfg, nil
}
